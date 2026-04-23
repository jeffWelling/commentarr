package db

import (
	"context"
	"os"
	"testing"
)

func TestOpen_InMemory(t *testing.T) {
	d, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer d.Close()

	if err := d.PingContext(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}
}

func TestMigrate_AppliesUpMigrations(t *testing.T) {
	tmp := t.TempDir()
	dir := tmp + "/migrations"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/001_example.up.sql",
		[]byte("CREATE TABLE foo (id INTEGER PRIMARY KEY);"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/001_example.down.sql",
		[]byte("DROP TABLE foo;"), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	if err := Migrate(d, dir); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	var name string
	if err := d.QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name='foo'`).Scan(&name); err != nil {
		t.Fatalf("table foo missing after migrate: %v", err)
	}
}

func TestMigrate_IdempotentSecondRun(t *testing.T) {
	tmp := t.TempDir()
	dir := tmp + "/migrations"
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/001_example.up.sql",
		[]byte("CREATE TABLE bar (id INTEGER PRIMARY KEY);"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dir+"/001_example.down.sql",
		[]byte("DROP TABLE bar;"), 0o644); err != nil {
		t.Fatal(err)
	}

	d, err := Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := Migrate(d, dir); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	if err := Migrate(d, dir); err != nil {
		t.Fatalf("second migrate should be no-op, got: %v", err)
	}
}
