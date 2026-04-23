package queue

import (
	"context"
	"testing"
	"time"

	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/title"
)

func newTestQueue(t *testing.T) (*Queue, title.Repo) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return New(d), title.NewRepo(d)
}

func TestQueue_MarkWantedAfterVerdict(t *testing.T) {
	q, titles := newTestQueue(t)
	ctx := context.Background()
	tt := title.Title{ID: "w:1", Kind: title.KindMovie, DisplayName: "A", FilePath: "/a.mkv"}
	if err := titles.Insert(ctx, tt); err != nil {
		t.Fatal(err)
	}

	if err := q.MarkWanted(ctx, "w:1"); err != nil {
		t.Fatalf("MarkWanted: %v", err)
	}

	got, err := q.Get(ctx, "w:1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != StatusWanted {
		t.Fatalf("expected StatusWanted got %q", got.Status)
	}
}

func TestQueue_ListWantedReturnsOnlyWanted(t *testing.T) {
	q, titles := newTestQueue(t)
	ctx := context.Background()
	for _, id := range []string{"a", "b", "c"} {
		if err := titles.Insert(ctx, title.Title{ID: id, Kind: title.KindMovie, DisplayName: id, FilePath: "/" + id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := q.MarkWanted(ctx, "a"); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkWanted(ctx, "b"); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkResolved(ctx, "b"); err != nil {
		t.Fatal(err)
	}

	wanted, err := q.ListByStatus(ctx, StatusWanted)
	if err != nil {
		t.Fatal(err)
	}
	if len(wanted) != 1 || wanted[0].TitleID != "a" {
		t.Fatalf("expected [a], got %+v", wanted)
	}
}

func TestQueue_UpdateNextSearchAt(t *testing.T) {
	q, titles := newTestQueue(t)
	ctx := context.Background()
	if err := titles.Insert(ctx, title.Title{ID: "t", Kind: title.KindMovie, DisplayName: "T", FilePath: "/t"}); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkWanted(ctx, "t"); err != nil {
		t.Fatal(err)
	}
	when := time.Now().Add(7 * 24 * time.Hour).UTC().Truncate(time.Second)
	if err := q.UpdateNextSearchAt(ctx, "t", when); err != nil {
		t.Fatal(err)
	}
	got, err := q.Get(ctx, "t")
	if err != nil {
		t.Fatal(err)
	}
	if !got.NextSearchAt.Equal(when) {
		t.Fatalf("expected %v got %v", when, got.NextSearchAt)
	}
}

func TestQueue_DueForSearch_RespectsNextSearchAt(t *testing.T) {
	q, titles := newTestQueue(t)
	ctx := context.Background()
	for _, id := range []string{"past", "future", "null"} {
		if err := titles.Insert(ctx, title.Title{ID: id, Kind: title.KindMovie, DisplayName: id, FilePath: "/" + id}); err != nil {
			t.Fatal(err)
		}
		if err := q.MarkWanted(ctx, id); err != nil {
			t.Fatal(err)
		}
	}
	now := time.Now().UTC().Truncate(time.Second)
	if err := q.UpdateNextSearchAt(ctx, "past", now.Add(-1*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := q.UpdateNextSearchAt(ctx, "future", now.Add(24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	// "null" keeps next_search_at NULL — should be due.

	due, err := q.DueForSearch(ctx, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(due) != 2 {
		t.Fatalf("expected 2 due (past + null), got %d: %+v", len(due), due)
	}
	gotIDs := map[string]bool{}
	for _, e := range due {
		gotIDs[e.TitleID] = true
	}
	if !gotIDs["past"] || !gotIDs["null"] || gotIDs["future"] {
		t.Fatalf("wrong due set: %+v", gotIDs)
	}
}

func TestQueue_IncrementSearchMiss(t *testing.T) {
	q, titles := newTestQueue(t)
	ctx := context.Background()
	if err := titles.Insert(ctx, title.Title{ID: "m", Kind: title.KindMovie, DisplayName: "M", FilePath: "/m"}); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkWanted(ctx, "m"); err != nil {
		t.Fatal(err)
	}
	if err := q.IncrementSearchMiss(ctx, "m"); err != nil {
		t.Fatal(err)
	}
	if err := q.IncrementSearchMiss(ctx, "m"); err != nil {
		t.Fatal(err)
	}
	got, _ := q.Get(ctx, "m")
	if got.SearchMisses != 2 {
		t.Fatalf("expected 2 misses got %d", got.SearchMisses)
	}
	if got.LastSearchedAt.IsZero() {
		t.Fatal("LastSearchedAt should be set after IncrementSearchMiss")
	}
}
