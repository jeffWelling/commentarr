// Package queue owns the wanted queue — which titles we're hunting for
// commentary-bearing releases of, and when to look again.
package queue

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Status enumerates the states a queued title can be in.
type Status string

const (
	StatusWanted   Status = "wanted"
	StatusSkipped  Status = "skipped"
	StatusResolved Status = "resolved"
)

// Entry is one row in the wanted queue.
type Entry struct {
	TitleID        string
	Status         Status
	LastSearchedAt time.Time
	NextSearchAt   time.Time
	SearchMisses   int
	UpdatedAt      time.Time
}

// Queue is the wanted-queue repository.
type Queue struct {
	db *sql.DB
}

// New returns a Queue backed by d.
func New(d *sql.DB) *Queue { return &Queue{db: d} }

// MarkWanted upserts the entry to status=wanted.
func (q *Queue) MarkWanted(ctx context.Context, titleID string) error {
	return q.upsertStatus(ctx, titleID, StatusWanted)
}

// MarkSkipped upserts the entry to status=skipped.
func (q *Queue) MarkSkipped(ctx context.Context, titleID string) error {
	return q.upsertStatus(ctx, titleID, StatusSkipped)
}

// MarkResolved upserts the entry to status=resolved.
func (q *Queue) MarkResolved(ctx context.Context, titleID string) error {
	return q.upsertStatus(ctx, titleID, StatusResolved)
}

// MarkResolvedWithRecheck upserts the entry to status=resolved AND
// sets next_recheck_at to now+after. Used after a successful import
// when the operator wants Commentarr to periodically re-search the
// title for upgrade candidates (e.g., a Criterion release that came
// out after we already imported the regular BD). Q8B in
// ~/claude/projects/commentarr/OPEN_QUESTIONS.md.
//
// Pass after<=0 to behave like MarkResolved (no future recheck).
func (q *Queue) MarkResolvedWithRecheck(ctx context.Context, titleID string, after time.Duration) error {
	if after <= 0 {
		return q.upsertStatus(ctx, titleID, StatusResolved)
	}
	when := time.Now().UTC().Add(after)
	_, err := q.db.ExecContext(ctx, `
		INSERT INTO wanted (title_id, status, next_recheck_at, updated_at)
		VALUES (?, ?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(title_id) DO UPDATE SET
		  status = excluded.status,
		  next_recheck_at = excluded.next_recheck_at,
		  updated_at = CURRENT_TIMESTAMP`,
		titleID, string(StatusResolved), when)
	if err != nil {
		return fmt.Errorf("mark resolved with recheck %s: %w", titleID, err)
	}
	return nil
}

func (q *Queue) upsertStatus(ctx context.Context, titleID string, s Status) error {
	_, err := q.db.ExecContext(ctx, `
		INSERT INTO wanted (title_id, status, updated_at)
		VALUES (?, ?, CURRENT_TIMESTAMP)
		ON CONFLICT(title_id) DO UPDATE SET status = excluded.status, updated_at = CURRENT_TIMESTAMP`,
		titleID, string(s))
	if err != nil {
		return fmt.Errorf("upsert status %s->%s: %w", titleID, s, err)
	}
	return nil
}

// UpdateNextSearchAt sets when this title should next be searched.
func (q *Queue) UpdateNextSearchAt(ctx context.Context, titleID string, when time.Time) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE wanted SET next_search_at = ?, updated_at = CURRENT_TIMESTAMP WHERE title_id = ?`,
		when.UTC(), titleID)
	if err != nil {
		return fmt.Errorf("UpdateNextSearchAt %s: %w", titleID, err)
	}
	return nil
}

// IncrementSearchMiss bumps the miss counter and sets last_searched_at=now.
func (q *Queue) IncrementSearchMiss(ctx context.Context, titleID string) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE wanted SET search_misses = search_misses + 1, last_searched_at = CURRENT_TIMESTAMP,
		                  updated_at = CURRENT_TIMESTAMP
		WHERE title_id = ?`, titleID)
	return err
}

// Get returns the entry for titleID.
func (q *Queue) Get(ctx context.Context, titleID string) (Entry, error) {
	var e Entry
	var status string
	var lastSearched, nextSearch sql.NullTime
	err := q.db.QueryRowContext(ctx, `
		SELECT title_id, status, last_searched_at, next_search_at, search_misses, updated_at
		FROM wanted WHERE title_id = ?`, titleID).
		Scan(&e.TitleID, &status, &lastSearched, &nextSearch, &e.SearchMisses, &e.UpdatedAt)
	if err != nil {
		return Entry{}, fmt.Errorf("get wanted %s: %w", titleID, err)
	}
	e.Status = Status(status)
	e.LastSearchedAt = lastSearched.Time
	e.NextSearchAt = nextSearch.Time
	return e, nil
}

// ListByStatus returns every entry with the given status.
func (q *Queue) ListByStatus(ctx context.Context, s Status) ([]Entry, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT title_id, status, last_searched_at, next_search_at, search_misses, updated_at
		FROM wanted WHERE status = ? ORDER BY title_id`, string(s))
	if err != nil {
		return nil, fmt.Errorf("list by status %s: %w", s, err)
	}
	defer rows.Close()

	var out []Entry
	for rows.Next() {
		var e Entry
		var status string
		var lastSearched, nextSearch sql.NullTime
		if err := rows.Scan(&e.TitleID, &status, &lastSearched, &nextSearch, &e.SearchMisses, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Status = Status(status)
		e.LastSearchedAt = lastSearched.Time
		e.NextSearchAt = nextSearch.Time
		out = append(out, e)
	}
	return out, rows.Err()
}

// DueForRecheck returns resolved entries whose next_recheck_at has
// elapsed. Q8B: a Criterion edition might come out after we imported
// a regular BD; periodic re-search keeps the candidate pool current
// without manual intervention.
//
// Caller is expected to UpdateNextRecheckAt after re-searching to
// avoid hammering the indexer every cycle.
func (q *Queue) DueForRecheck(ctx context.Context, now time.Time) ([]Entry, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT title_id, status, last_searched_at, next_search_at, search_misses, updated_at
		FROM wanted
		WHERE status = 'resolved'
		  AND next_recheck_at IS NOT NULL
		  AND next_recheck_at <= ?
		ORDER BY next_recheck_at`, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("due for recheck: %w", err)
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var status string
		var ls, ns sql.NullTime
		if err := rows.Scan(&e.TitleID, &status, &ls, &ns, &e.SearchMisses, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Status = Status(status)
		e.LastSearchedAt = ls.Time
		e.NextSearchAt = ns.Time
		out = append(out, e)
	}
	return out, rows.Err()
}

// UpdateNextRecheckAt advances when this title should next be
// re-checked. Symmetric with UpdateNextSearchAt but for the
// resolved-recheck loop.
func (q *Queue) UpdateNextRecheckAt(ctx context.Context, titleID string, when time.Time) error {
	_, err := q.db.ExecContext(ctx, `
		UPDATE wanted SET next_recheck_at = ?, updated_at = CURRENT_TIMESTAMP WHERE title_id = ?`,
		when.UTC(), titleID)
	if err != nil {
		return fmt.Errorf("UpdateNextRecheckAt %s: %w", titleID, err)
	}
	return nil
}

// DueForSearch returns every wanted entry with next_search_at <= now
// (or null). Used by the scheduler loop in Plan 2.
func (q *Queue) DueForSearch(ctx context.Context, now time.Time) ([]Entry, error) {
	rows, err := q.db.QueryContext(ctx, `
		SELECT title_id, status, last_searched_at, next_search_at, search_misses, updated_at
		FROM wanted
		WHERE status = 'wanted'
		  AND (next_search_at IS NULL OR next_search_at <= ?)
		ORDER BY COALESCE(next_search_at, updated_at)`, now.UTC())
	if err != nil {
		return nil, fmt.Errorf("due for search: %w", err)
	}
	defer rows.Close()
	var out []Entry
	for rows.Next() {
		var e Entry
		var status string
		var ls, ns sql.NullTime
		if err := rows.Scan(&e.TitleID, &status, &ls, &ns, &e.SearchMisses, &e.UpdatedAt); err != nil {
			return nil, err
		}
		e.Status = Status(status)
		e.LastSearchedAt = ls.Time
		e.NextSearchAt = ns.Time
		out = append(out, e)
	}
	return out, rows.Err()
}
