package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/jeffWelling/commentarr/internal/auth"
	"github.com/jeffWelling/commentarr/internal/db"
)

func TestBootstrapAdmin_NoEnvIsNoOp(t *testing.T) {
	t.Setenv("COMMENTARR_ADMIN_USERNAME", "")
	t.Setenv("COMMENTARR_ADMIN_PASSWORD", "")
	repo := newTestAuthRepo(t)

	if err := bootstrapAdmin(repo); err != nil {
		t.Fatalf("bootstrapAdmin: %v", err)
	}
	_, err := repo.Admin(context.Background())
	if err == nil {
		t.Fatal("expected no admin, got one")
	}
}

func TestBootstrapAdmin_SetsAdminAndHashesPassword(t *testing.T) {
	t.Setenv("COMMENTARR_ADMIN_USERNAME", "commander")
	t.Setenv("COMMENTARR_ADMIN_PASSWORD", "engage-warp-9")
	repo := newTestAuthRepo(t)

	if err := bootstrapAdmin(repo); err != nil {
		t.Fatalf("bootstrapAdmin: %v", err)
	}
	a, err := repo.Admin(context.Background())
	if err != nil {
		t.Fatalf("load admin: %v", err)
	}
	if a.Username != "commander" {
		t.Errorf("username: got %q want commander", a.Username)
	}
	if a.PasswordHash == "engage-warp-9" {
		t.Error("password was stored in plaintext")
	}
	if !auth.VerifyPassword(a.PasswordHash, "engage-warp-9") {
		t.Error("stored hash does not verify against the plaintext")
	}
}

func TestBootstrapAdmin_IdempotentOnRestart(t *testing.T) {
	t.Setenv("COMMENTARR_ADMIN_USERNAME", "commander")
	t.Setenv("COMMENTARR_ADMIN_PASSWORD", "engage-warp-9")
	repo := newTestAuthRepo(t)

	if err := bootstrapAdmin(repo); err != nil {
		t.Fatal(err)
	}
	if err := bootstrapAdmin(repo); err != nil {
		t.Fatalf("second bootstrap: %v", err)
	}
	if _, err := repo.Admin(context.Background()); err != nil {
		t.Fatalf("admin should still exist: %v", err)
	}
}

func TestBootstrapAdmin_MissingHalfIsNoOp(t *testing.T) {
	t.Setenv("COMMENTARR_ADMIN_USERNAME", "commander")
	t.Setenv("COMMENTARR_ADMIN_PASSWORD", "")
	repo := newTestAuthRepo(t)

	if err := bootstrapAdmin(repo); err != nil {
		t.Fatal(err)
	}
	_, err := repo.Admin(context.Background())
	if err == nil {
		t.Fatal("expected no admin when only username set")
	}
}

func newTestAuthRepo(t *testing.T) *auth.Repo {
	t.Helper()
	dir := t.TempDir()
	dsn := filepath.Join(dir, "test.db")
	d, err := db.Open(dsn)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { _ = d.Close() })

	migrations, err := resolveMigrations()
	if err != nil {
		t.Fatalf("resolve migrations: %v", err)
	}
	if err := db.Migrate(d, migrations); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return auth.NewRepo(d)
}

// resolveMigrations walks upward from the test binary until it finds a
// migrations/ directory. Needed because `go test` runs with cwd =
// cmd/commentarr/, not the repo root.
func resolveMigrations() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for dir := wd; dir != "/"; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, "migrations")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, nil
		}
	}
	return "", errors.New("migrations directory not found")
}
