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
