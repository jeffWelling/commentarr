package queue

import (
	"context"
	"testing"

	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/title"
)

// Transition matrix asserted below:
//
//   wanted   → wanted   (idempotent re-mark)
//   wanted   → skipped  (user override)
//   wanted   → resolved (successful import)
//   skipped  → wanted   (user undoes skip)
//   resolved → wanted   (verdict rerun finds commentary is gone)

func setupTransitionsQueue(t *testing.T) (*Queue, title.Repo, context.Context) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return New(d), title.NewRepo(d), context.Background()
}

func insertTitle(t *testing.T, titles title.Repo, ctx context.Context, id string) {
	t.Helper()
	if err := titles.Insert(ctx, title.Title{ID: id, Kind: title.KindMovie, DisplayName: id, FilePath: "/" + id}); err != nil {
		t.Fatal(err)
	}
}

func TestTransitions_IdempotentWanted(t *testing.T) {
	q, titles, ctx := setupTransitionsQueue(t)
	insertTitle(t, titles, ctx, "x")
	if err := q.MarkWanted(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkWanted(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	e, err := q.Get(ctx, "x")
	if err != nil {
		t.Fatal(err)
	}
	if e.Status != StatusWanted {
		t.Fatalf("expected wanted, got %q", e.Status)
	}
}

func TestTransitions_WantedToSkipped(t *testing.T) {
	q, titles, ctx := setupTransitionsQueue(t)
	insertTitle(t, titles, ctx, "x")
	if err := q.MarkWanted(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkSkipped(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	e, _ := q.Get(ctx, "x")
	if e.Status != StatusSkipped {
		t.Fatalf("expected skipped got %q", e.Status)
	}
}

func TestTransitions_WantedToResolved(t *testing.T) {
	q, titles, ctx := setupTransitionsQueue(t)
	insertTitle(t, titles, ctx, "x")
	if err := q.MarkWanted(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkResolved(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	e, _ := q.Get(ctx, "x")
	if e.Status != StatusResolved {
		t.Fatalf("expected resolved got %q", e.Status)
	}
}

func TestTransitions_SkippedToWanted(t *testing.T) {
	q, titles, ctx := setupTransitionsQueue(t)
	insertTitle(t, titles, ctx, "x")
	if err := q.MarkSkipped(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkWanted(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	e, _ := q.Get(ctx, "x")
	if e.Status != StatusWanted {
		t.Fatalf("expected wanted got %q", e.Status)
	}
}

func TestTransitions_ResolvedToWanted(t *testing.T) {
	q, titles, ctx := setupTransitionsQueue(t)
	insertTitle(t, titles, ctx, "x")
	if err := q.MarkResolved(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkWanted(ctx, "x"); err != nil {
		t.Fatal(err)
	}
	e, _ := q.Get(ctx, "x")
	if e.Status != StatusWanted {
		t.Fatalf("expected wanted got %q", e.Status)
	}
}
