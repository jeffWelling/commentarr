package download

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/jeffWelling/commentarr/internal/db"
)

func TestJobRepo_SaveAndFind(t *testing.T) {
	repo := newTestJobRepo(t)
	id, err := repo.Save(context.Background(), Job{
		ClientName: "qbit", ClientJobID: "abc", TitleID: "tt-1",
		ReleaseTitle: "Some Movie 2020 Criterion BluRay",
		Edition:      "Criterion",
	})
	if err != nil {
		t.Fatal(err)
	}
	if id == 0 {
		t.Fatal("expected non-zero id")
	}
	got, err := repo.FindByClientJob(context.Background(), "qbit", "abc")
	if err != nil {
		t.Fatal(err)
	}
	if got.TitleID != "tt-1" || got.Edition != "Criterion" || got.Status != "queued" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestJobRepo_FindMissingReturnsErr(t *testing.T) {
	repo := newTestJobRepo(t)
	_, err := repo.FindByClientJob(context.Background(), "qbit", "missing")
	if !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("want ErrJobNotFound, got %v", err)
	}
}

func TestJobRepo_SaveIsUpsertOnClientJob(t *testing.T) {
	repo := newTestJobRepo(t)
	_, err := repo.Save(context.Background(), Job{
		ClientName: "qbit", ClientJobID: "x", TitleID: "tt-1", ReleaseTitle: "first",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = repo.Save(context.Background(), Job{
		ClientName: "qbit", ClientJobID: "x", TitleID: "tt-2", ReleaseTitle: "second",
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := repo.FindByClientJob(context.Background(), "qbit", "x")
	if got.TitleID != "tt-2" || got.ReleaseTitle != "second" {
		t.Fatalf("upsert didn't update: %+v", got)
	}
}

func TestJobRepo_MarkStatusSetsImportedAtForTerminal(t *testing.T) {
	repo := newTestJobRepo(t)
	id, _ := repo.Save(context.Background(), Job{ClientName: "qbit", ClientJobID: "x", TitleID: "t"})
	if err := repo.MarkStatus(context.Background(), id, "imported", "placed"); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.FindByClientJob(context.Background(), "qbit", "x")
	if got.Status != "imported" || got.Outcome != "placed" || got.ImportedAt.IsZero() {
		t.Fatalf("expected imported+timestamp set: %+v", got)
	}
}

func TestJobRepo_MarkStatusKeepsImportedAtNullForTransient(t *testing.T) {
	repo := newTestJobRepo(t)
	id, _ := repo.Save(context.Background(), Job{ClientName: "qbit", ClientJobID: "x", TitleID: "t"})
	if err := repo.MarkStatus(context.Background(), id, "completed", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := repo.FindByClientJob(context.Background(), "qbit", "x")
	if got.Status != "completed" || !got.ImportedAt.IsZero() {
		t.Fatalf("transient status should not set imported_at: %+v", got)
	}
}

func TestJobRepo_ListByStatus(t *testing.T) {
	repo := newTestJobRepo(t)
	for _, jid := range []string{"a", "b", "c"} {
		_, _ = repo.Save(context.Background(), Job{ClientName: "qbit", ClientJobID: jid, TitleID: "t"})
	}
	id, _ := repo.Save(context.Background(), Job{ClientName: "qbit", ClientJobID: "imported-one", TitleID: "t"})
	_ = repo.MarkStatus(context.Background(), id, "imported", "")

	queued, err := repo.ListByStatus(context.Background(), "queued")
	if err != nil {
		t.Fatal(err)
	}
	if len(queued) != 3 {
		t.Errorf("expected 3 queued, got %d", len(queued))
	}
	imp, _ := repo.ListByStatus(context.Background(), "imported")
	if len(imp) != 1 {
		t.Errorf("expected 1 imported, got %d", len(imp))
	}
}

func TestJobRepo_HasInflightForTitle(t *testing.T) {
	repo := newTestJobRepo(t)
	ctx := context.Background()

	// no jobs → false
	got, err := repo.HasInflightForTitle(ctx, "tt-1")
	if err != nil || got {
		t.Fatalf("empty: got=%v err=%v", got, err)
	}

	// queued job → true
	id, _ := repo.Save(ctx, Job{ClientName: "qbit", ClientJobID: "a", TitleID: "tt-1"})
	got, _ = repo.HasInflightForTitle(ctx, "tt-1")
	if !got {
		t.Fatal("queued job should count as inflight")
	}

	// errored only → false (allows retry)
	_ = repo.MarkStatus(ctx, id, "error", "stalled")
	got, _ = repo.HasInflightForTitle(ctx, "tt-1")
	if got {
		t.Fatal("errored job should not block")
	}

	// completed → true (counts as inflight; importer hasn't finished yet)
	id2, _ := repo.Save(ctx, Job{ClientName: "qbit", ClientJobID: "b", TitleID: "tt-1"})
	_ = repo.MarkStatus(ctx, id2, "completed", "")
	got, _ = repo.HasInflightForTitle(ctx, "tt-1")
	if !got {
		t.Fatal("completed should count as inflight")
	}

	// other titles aren't affected
	got, _ = repo.HasInflightForTitle(ctx, "tt-other")
	if got {
		t.Fatal("title isolation broken")
	}
}

func TestJobRepo_ListRecentReturnsNewestFirst(t *testing.T) {
	repo := newTestJobRepo(t)
	for _, jid := range []string{"first", "second", "third"} {
		_, _ = repo.Save(context.Background(), Job{
			ClientName: "qbit", ClientJobID: jid, TitleID: "t",
		})
	}
	got, err := repo.ListRecent(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	if got[0].ClientJobID != "third" || got[2].ClientJobID != "first" {
		t.Errorf("expected newest-first ordering, got %v / %v / %v",
			got[0].ClientJobID, got[1].ClientJobID, got[2].ClientJobID)
	}
}

func TestJobRepo_ListRecentRespectsLimit(t *testing.T) {
	repo := newTestJobRepo(t)
	for i := 0; i < 5; i++ {
		_, _ = repo.Save(context.Background(), Job{
			ClientName: "qbit", ClientJobID: "j" + string(rune('0'+i)), TitleID: "t",
		})
	}
	got, _ := repo.ListRecent(context.Background(), 2)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d", len(got))
	}
}

func newTestJobRepo(t *testing.T) *JobRepo {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "jobs.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewJobRepo(d)
}
