package trash

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jeffWelling/commentarr/internal/db"
)

func newTrash(t *testing.T) (*Trash, string) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return New(d, Config{Retention: time.Hour, AutoPurge: true}), t.TempDir()
}

func TestTrash_RecordsItem(t *testing.T) {
	tr, tmp := newTrash(t)
	ctx := context.Background()
	trashedPath := filepath.Join(tmp, "trashed-file.mkv")
	if err := os.WriteFile(trashedPath, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := tr.Record(ctx, "lib1", "/original/path.mkv", trashedPath, "replace"); err != nil {
		t.Fatal(err)
	}
	items, err := tr.List(ctx, "lib1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].OriginalPath != "/original/path.mkv" {
		t.Fatalf("unexpected: %+v", items)
	}
}

func TestTrash_PurgeRemovesOldItems(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	tmp := t.TempDir()
	oldPath := filepath.Join(tmp, "old.mkv")
	_ = os.WriteFile(oldPath, []byte("old"), 0o644)

	tr := New(d, Config{Retention: 10 * time.Millisecond, AutoPurge: true})
	ctx := context.Background()
	if err := tr.Record(ctx, "lib1", "/old", oldPath, "replace"); err != nil {
		t.Fatal(err)
	}

	time.Sleep(30 * time.Millisecond)
	n, err := tr.PurgeExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 purged, got %d", n)
	}
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatal("trashed file should be gone after purge")
	}
	items, _ := tr.List(ctx, "lib1")
	if len(items) != 0 {
		t.Fatalf("purged items should not list: %+v", items)
	}
}

func TestTrash_PurgeSkipsWhenAutoPurgeDisabled(t *testing.T) {
	d, _ := db.Open(":memory:")
	defer d.Close()
	_ = db.Migrate(d, "../../migrations")
	tr := New(d, Config{Retention: time.Millisecond, AutoPurge: false})
	ctx := context.Background()
	_ = tr.Record(ctx, "lib", "/a", "/trash/a", "replace")
	time.Sleep(10 * time.Millisecond)

	n, err := tr.PurgeExpired(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("AutoPurge=false should purge nothing, got %d", n)
	}
}
