// Package trash records moves-to-trash, exposes the list of current
// items, purges them after a configurable retention period, and emits
// per-library metrics.
package trash

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"time"
)

// Item is one row in trash_items.
type Item struct {
	ID           int64
	Library      string
	OriginalPath string
	TrashPath    string
	SizeBytes    int64
	MovedAt      time.Time
	Reason       string
}

// Repo is the thin SQLite layer.
type Repo struct {
	db *sql.DB
}

// NewRepo returns a Repo.
func NewRepo(d *sql.DB) *Repo { return &Repo{db: d} }

// Insert records a new trashed item.
func (r *Repo) Insert(ctx context.Context, it Item) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO trash_items (library, original_path, trash_path, size_bytes, reason)
		VALUES (?, ?, ?, ?, ?)`,
		it.Library, it.OriginalPath, it.TrashPath, it.SizeBytes, it.Reason)
	if err != nil {
		return fmt.Errorf("trash insert: %w", err)
	}
	return nil
}

// ListByLibrary returns non-purged items for a library.
func (r *Repo) ListByLibrary(ctx context.Context, library string) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library, original_path, trash_path, size_bytes, moved_at, COALESCE(reason,'')
		FROM trash_items
		WHERE library = ? AND purged = FALSE
		ORDER BY moved_at DESC`, library)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Library, &it.OriginalPath, &it.TrashPath, &it.SizeBytes, &it.MovedAt, &it.Reason); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// Expired returns non-purged items older than cutoff.
func (r *Repo) Expired(ctx context.Context, cutoff time.Time) ([]Item, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, library, original_path, trash_path, size_bytes, moved_at, COALESCE(reason,'')
		FROM trash_items
		WHERE purged = FALSE AND moved_at < ?`, cutoff.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Item
	for rows.Next() {
		var it Item
		if err := rows.Scan(&it.ID, &it.Library, &it.OriginalPath, &it.TrashPath, &it.SizeBytes, &it.MovedAt, &it.Reason); err != nil {
			return nil, err
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// MarkPurged flips the purged flag for a record.
func (r *Repo) MarkPurged(ctx context.Context, id int64) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE trash_items SET purged = TRUE, purged_at = CURRENT_TIMESTAMP WHERE id = ?`, id)
	return err
}

// DeleteFileAndMarkPurged removes the on-disk trash file and marks the
// row purged. If the file is already gone, we still mark the row.
func (r *Repo) DeleteFileAndMarkPurged(ctx context.Context, it Item) error {
	if err := os.Remove(it.TrashPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("delete %s: %w", it.TrashPath, err)
	}
	return r.MarkPurged(ctx, it.ID)
}
