package trash

import (
	"context"
	"database/sql"
	"time"

	"github.com/jeffWelling/commentarr/internal/metrics"
)

// Config controls Trash behaviour.
type Config struct {
	Retention time.Duration
	AutoPurge bool
}

// Trash is the public service.
type Trash struct {
	repo *Repo
	cfg  Config
}

// New returns a Trash service.
func New(db *sql.DB, cfg Config) *Trash {
	return &Trash{repo: NewRepo(db), cfg: cfg}
}

// Record logs a trashed file. Callers have already moved the file; this
// just persists the bookkeeping and updates metrics.
func (t *Trash) Record(ctx context.Context, library, originalPath, trashPath, reason string) error {
	it := Item{Library: library, OriginalPath: originalPath, TrashPath: trashPath, Reason: reason}
	if err := t.repo.Insert(ctx, it); err != nil {
		return err
	}
	metrics.TrashItems.WithLabelValues(library).Inc()
	return nil
}

// List returns current (non-purged) items for a library.
func (t *Trash) List(ctx context.Context, library string) ([]Item, error) {
	return t.repo.ListByLibrary(ctx, library)
}

// PurgeExpired removes items past Retention from disk + flips purged=true.
// No-op when AutoPurge is false or Retention <= 0.
func (t *Trash) PurgeExpired(ctx context.Context) (int, error) {
	if !t.cfg.AutoPurge || t.cfg.Retention <= 0 {
		return 0, nil
	}
	cutoff := time.Now().UTC().Add(-t.cfg.Retention)
	items, err := t.repo.Expired(ctx, cutoff)
	if err != nil {
		return 0, err
	}
	for _, it := range items {
		if err := t.repo.DeleteFileAndMarkPurged(ctx, it); err != nil {
			return 0, err
		}
		metrics.TrashItemsPurgedTotal.WithLabelValues(it.Library).Inc()
		metrics.TrashItems.WithLabelValues(it.Library).Dec()
	}
	return len(items), nil
}
