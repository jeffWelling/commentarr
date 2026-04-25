package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

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
	return auth.NewRepo(newTestDB(t))
}

func newTestDB(t *testing.T) *sql.DB {
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
	return d
}

func TestInfoFromProwlarr_EmptyURLReturnsNil(t *testing.T) {
	if got := infoFromProwlarr("", "prowlarr"); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestInfoFromProwlarr_PopulatesFields(t *testing.T) {
	got := infoFromProwlarr("http://prowlarr:9696", "main")
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].BaseURL != "http://prowlarr:9696" || got[0].Name != "main" ||
		got[0].Kind != "prowlarr" || !got[0].Enabled {
		t.Errorf("unexpected entry: %+v", got[0])
	}
}

func TestInfoFromQbit_EmptyURLReturnsNil(t *testing.T) {
	if got := infoFromQbit("", "qbit"); got != nil {
		t.Errorf("expected nil, got %+v", got)
	}
}

func TestBuildSearchTick_DisabledWhenProwlarrUnconfigured(t *testing.T) {
	cases := []struct {
		name     string
		url, key string
		interval time.Duration
	}{
		{"no url", "", "key", time.Minute},
		{"no key", "http://prowlarr", "", time.Minute},
		{"interval zero", "http://prowlarr", "key", 0},
		{"interval negative", "http://prowlarr", "key", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := buildSearchTick(nil, tc.url, tc.key, "p", 6, 3, 8, tc.interval)
			if ok {
				t.Fatal("expected tick to be disabled")
			}
		})
	}
}

func TestBuildWatcher_DisabledWhenQbitUnconfigured(t *testing.T) {
	cases := []struct {
		name                       string
		url, username, password    string
		interval                   time.Duration
	}{
		{"no url", "", "u", "p", time.Second},
		{"no user", "http://qbit", "", "p", time.Second},
		{"no pass", "http://qbit", "u", "", time.Second},
		{"interval zero", "http://qbit", "u", "p", 0},
		{"interval negative", "http://qbit", "u", "p", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w, ev, ok := buildWatcher(tc.url, tc.username, tc.password, "qbit", "commentarr", tc.interval)
			if ok || w != nil || ev != nil {
				t.Fatalf("expected disabled, got w=%v ev=%v ok=%v", w, ev, ok)
			}
		})
	}
}

func TestBuildWatcher_EnabledWhenQbitConfigured(t *testing.T) {
	w, ev, ok := buildWatcher("http://qbit.test", "user", "pass", "qbit", "commentarr", 30*time.Second)
	if !ok {
		t.Fatal("expected enabled")
	}
	if w == nil || ev == nil {
		t.Fatalf("expected non-nil watcher + chan, got w=%v ev=%v", w, ev)
	}
	if cap(ev) < 16 {
		t.Errorf("event channel buffer too small: %d", cap(ev))
	}
}

func TestBuildSearchTick_EnabledWhenProwlarrConfigured(t *testing.T) {
	d := newTestDB(t)
	tick, ok := buildSearchTick(d, "http://prowlarr.test", "abc", "main", 6, 3, 8, time.Minute)
	if !ok {
		t.Fatal("expected tick to be enabled")
	}
	if tick.Name != "search-due" || tick.Interval != time.Minute || tick.Fn == nil {
		t.Fatalf("unexpected tick: %+v", tick)
	}
}

func TestInfoFromQbit_PopulatesFields(t *testing.T) {
	got := infoFromQbit("http://qbit:8080", "hot")
	if len(got) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(got))
	}
	if got[0].Kind != "qbittorrent" || got[0].BaseURL != "http://qbit:8080" ||
		got[0].Name != "hot" || !got[0].Enabled {
		t.Errorf("unexpected entry: %+v", got[0])
	}
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
