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

func TestQueue_MarkResolvedWithRecheck_SchedulesRecheck(t *testing.T) {
	q, titles := newTestQueue(t)
	ctx := context.Background()
	tt := title.Title{ID: "r:1", Kind: title.KindMovie, DisplayName: "Resolved", FilePath: "/r.mkv"}
	if err := titles.Insert(ctx, tt); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkResolvedWithRecheck(ctx, tt.ID, 90*24*time.Hour); err != nil {
		t.Fatal(err)
	}
	// Within the recheck window: nothing due.
	if got, _ := q.DueForRecheck(ctx, time.Now()); len(got) != 0 {
		t.Errorf("expected 0 due-for-recheck immediately after mark, got %d", len(got))
	}
	// Past the recheck window: the title shows up.
	if got, _ := q.DueForRecheck(ctx, time.Now().Add(91*24*time.Hour)); len(got) != 1 {
		t.Errorf("expected 1 due-for-recheck after window expired, got %d", len(got))
	}
}

func TestQueue_MarkResolvedWithRecheck_ZeroIntervalFallsBack(t *testing.T) {
	// after<=0 must behave like the plain MarkResolved — title is
	// resolved but next_recheck_at stays NULL so DueForRecheck never
	// surfaces it. Operators who don't want re-search opt out by
	// setting -recheck-interval=0.
	q, titles := newTestQueue(t)
	ctx := context.Background()
	tt := title.Title{ID: "r:2", Kind: title.KindMovie, DisplayName: "OptOut", FilePath: "/r2.mkv"}
	_ = titles.Insert(ctx, tt)
	if err := q.MarkResolvedWithRecheck(ctx, tt.ID, 0); err != nil {
		t.Fatal(err)
	}
	if got, _ := q.DueForRecheck(ctx, time.Now().Add(10*365*24*time.Hour)); len(got) != 0 {
		t.Errorf("interval=0 should mean no recheck ever; got %d", len(got))
	}
	// But the title IS marked resolved.
	e, err := q.Get(ctx, tt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if e.Status != StatusResolved {
		t.Errorf("expected resolved status, got %q", e.Status)
	}
}

func TestQueue_UpdateNextRecheckAtAdvancesWindow(t *testing.T) {
	q, titles := newTestQueue(t)
	ctx := context.Background()
	tt := title.Title{ID: "r:3", Kind: title.KindMovie, DisplayName: "C", FilePath: "/c.mkv"}
	_ = titles.Insert(ctx, tt)
	_ = q.MarkResolvedWithRecheck(ctx, tt.ID, time.Microsecond)
	time.Sleep(2 * time.Millisecond)

	// Initially due (1µs window already elapsed).
	if got, _ := q.DueForRecheck(ctx, time.Now()); len(got) != 1 {
		t.Fatal("expected the title to be due before advance")
	}
	// Advance the recheck window.
	if err := q.UpdateNextRecheckAt(ctx, tt.ID, time.Now().Add(1*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if got, _ := q.DueForRecheck(ctx, time.Now()); len(got) != 0 {
		t.Errorf("expected 0 due after advancing recheck window, got %d", len(got))
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
