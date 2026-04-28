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
	"github.com/jeffWelling/commentarr/internal/download"
	"github.com/jeffWelling/commentarr/internal/importer"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/title"
)

type stubImporter struct {
	calls int
	res   importer.Result
	err   error
}

func (s *stubImporter) Import(_ context.Context, _ importer.Request) (importer.Result, error) {
	s.calls++
	return s.res, s.err
}

func newHandleEventFixture(t *testing.T) (*download.JobRepo, title.Repo, *queue.Queue, *sql.DB) {
	t.Helper()
	d := newTestDB(t)
	jobs := download.NewJobRepo(d)
	titles := title.NewRepo(d)
	q := queue.New(d)
	return jobs, titles, q, d
}

func TestHandleEvent_NonCompletedKindIsLogOnly(t *testing.T) {
	jobs, titles, q, _ := newHandleEventFixture(t)
	imp := &stubImporter{}
	handleEvent(context.Background(), jobs, titles, q, imp, download.Event{
		Kind: download.EventError, Client: "qbit",
		Status: download.Status{ClientJobID: "x"},
	})
	if imp.calls != 0 {
		t.Fatal("error events must not invoke importer")
	}
}

func TestHandleEvent_JobNotFoundIsNoOp(t *testing.T) {
	jobs, titles, q, _ := newHandleEventFixture(t)
	imp := &stubImporter{}
	handleEvent(context.Background(), jobs, titles, q, imp, download.Event{
		Kind: download.EventCompleted, Client: "qbit",
		Status: download.Status{ClientJobID: "missing"},
	})
	if imp.calls != 0 {
		t.Fatal("missing job should never reach importer")
	}
}

func TestHandleEvent_TitleMissingMarksJobError(t *testing.T) {
	jobs, titles, q, _ := newHandleEventFixture(t)
	id, _ := jobs.Save(context.Background(), download.Job{
		ClientName: "qbit", ClientJobID: "abc", TitleID: "tt-orphan",
	})
	imp := &stubImporter{}
	handleEvent(context.Background(), jobs, titles, q, imp, download.Event{
		Kind: download.EventCompleted, Client: "qbit",
		Status: download.Status{ClientJobID: "abc", SavePath: "/nowhere"},
	})
	got, _ := jobs.FindByClientJob(context.Background(), "qbit", "abc")
	if got.Status != "error" || got.ID != id {
		t.Fatalf("expected job marked error, got %+v", got)
	}
}

func TestHandleEvent_NoMainVideoMarksJobError(t *testing.T) {
	jobs, titles, q, _ := newHandleEventFixture(t)
	_ = titles.Insert(context.Background(), title.Title{
		ID: "tt-1", Kind: title.KindMovie, DisplayName: "Test", FilePath: "/orig",
	})
	_, _ = jobs.Save(context.Background(), download.Job{
		ClientName: "qbit", ClientJobID: "abc", TitleID: "tt-1",
	})
	dir := t.TempDir() // empty — FindMainVideo will fail
	imp := &stubImporter{}
	handleEvent(context.Background(), jobs, titles, q, imp, download.Event{
		Kind: download.EventCompleted, Client: "qbit",
		Status: download.Status{ClientJobID: "abc", SavePath: dir},
	})
	got, _ := jobs.FindByClientJob(context.Background(), "qbit", "abc")
	if got.Status != "error" {
		t.Fatalf("expected error status, got %q", got.Status)
	}
	if imp.calls != 0 {
		t.Fatal("importer should not run when no main video")
	}
}

func TestHandleEvent_SuccessMarksJobImportedAndQueueResolved(t *testing.T) {
	jobs, titles, q, _ := newHandleEventFixture(t)
	_ = titles.Insert(context.Background(), title.Title{
		ID: "tt-1", Kind: title.KindMovie, DisplayName: "Test", FilePath: "/orig",
	})
	_ = q.MarkWanted(context.Background(), "tt-1")
	_, _ = jobs.Save(context.Background(), download.Job{
		ClientName: "qbit", ClientJobID: "abc", TitleID: "tt-1",
	})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "movie.mkv"), []byte("placeholder"), 0o644); err != nil {
		t.Fatal(err)
	}

	imp := &stubImporter{res: importer.Result{Outcome: importer.OutcomeSuccess, FinalPath: "/lib/movie.mkv"}}
	handleEvent(context.Background(), jobs, titles, q, imp, download.Event{
		Kind: download.EventCompleted, Client: "qbit",
		Status: download.Status{ClientJobID: "abc", SavePath: dir},
	})

	got, _ := jobs.FindByClientJob(context.Background(), "qbit", "abc")
	if got.Status != "imported" {
		t.Fatalf("expected imported, got %q", got.Status)
	}
	wanted, _ := q.Get(context.Background(), "tt-1")
	if wanted.Status != queue.StatusResolved {
		t.Fatalf("expected resolved, got %q", wanted.Status)
	}
}

func TestHandleEvent_NonSuccessOutcomeLeavesQueueWanted(t *testing.T) {
	jobs, titles, q, _ := newHandleEventFixture(t)
	_ = titles.Insert(context.Background(), title.Title{
		ID: "tt-1", Kind: title.KindMovie, DisplayName: "Test", FilePath: "/orig",
	})
	_ = q.MarkWanted(context.Background(), "tt-1")
	_, _ = jobs.Save(context.Background(), download.Job{
		ClientName: "qbit", ClientJobID: "abc", TitleID: "tt-1",
	})
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "movie.mkv"), []byte("x"), 0o644)

	imp := &stubImporter{res: importer.Result{Outcome: importer.OutcomeSafetyViolation}}
	handleEvent(context.Background(), jobs, titles, q, imp, download.Event{
		Kind: download.EventCompleted, Client: "qbit",
		Status: download.Status{ClientJobID: "abc", SavePath: dir},
	})

	wanted, _ := q.Get(context.Background(), "tt-1")
	if wanted.Status != queue.StatusWanted {
		t.Fatalf("expected wanted (so next cycle can try a better candidate), got %q", wanted.Status)
	}
}

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

func TestBuildDownloadClient_DisabledWhenQbitUnconfigured(t *testing.T) {
	cases := []struct {
		name                    string
		url, username, password string
	}{
		{"no url", "", "u", "p"},
		{"no user", "http://qbit", "", "p"},
		{"no pass", "http://qbit", "u", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, ok := buildDownloadClient(tc.url, tc.username, tc.password, "qbit")
			if ok || c != nil {
				t.Fatalf("expected disabled, got c=%v ok=%v", c, ok)
			}
		})
	}
}

func TestBuildDownloadClient_EnabledWhenQbitConfigured(t *testing.T) {
	c, ok := buildDownloadClient("http://qbit.test", "user", "pass", "qbit")
	if !ok || c == nil {
		t.Fatalf("expected enabled, got c=%v ok=%v", c, ok)
	}
	if c.Name() != "qbit" {
		t.Errorf("name not propagated: %q", c.Name())
	}
}

func TestPathTranslator(t *testing.T) {
	cases := []struct {
		name      string
		from, to  string
		in        string
		want      string
	}{
		{"identity when from is empty", "", "", "/downloads/Brazil", "/downloads/Brazil"},
		{"prefix swapped", "/downloads", "/Volumes/downloads", "/downloads/Brazil", "/Volumes/downloads/Brazil"},
		{"non-matching path passes through", "/downloads", "/Volumes/downloads", "/elsewhere/Brazil", "/elsewhere/Brazil"},
		{"empty input stays empty", "/downloads", "/Volumes/downloads", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fn := pathTranslator(tc.from, tc.to)
			if got := fn(tc.in); got != tc.want {
				t.Errorf("translate(%q): got %q want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestStartWatcher_ReturnsBufferedChannel(t *testing.T) {
	c, _ := buildDownloadClient("http://qbit.test", "u", "p", "qbit")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ev := startWatcher(ctx, c, "commentarr", 30*time.Second)
	if ev == nil {
		t.Fatal("expected non-nil channel")
	}
	if cap(ev) < 16 {
		t.Errorf("event channel buffer too small: %d", cap(ev))
	}
}

func TestValidateServeFlags(t *testing.T) {
	cases := []struct {
		name                              string
		mode, separateRoot, trashDir       string
		wantErr                           bool
	}{
		{"sidecar default", "sidecar", "", "", false},
		{"replace without trash", "replace", "", "", true},
		{"replace with trash", "replace", "", "/var/trash", false},
		{"separate without root", "separate-library", "", "", true},
		{"separate with root", "separate-library", "/alt", "", false},
		{"unknown mode", "weird", "", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateServeFlags(tc.mode, tc.separateRoot, tc.trashDir)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
		})
	}
}

func TestBuildPickerTick_ProducesNamedTick(t *testing.T) {
	c, _ := buildDownloadClient("http://qbit.test", "u", "p", "qbit")
	d := newTestDB(t)
	tick := buildPickerTick(d, c, "commentarr", 8, 5*time.Minute, false, nil)
	if tick.Name != "picker" || tick.Interval != 5*time.Minute || tick.Fn == nil {
		t.Fatalf("unexpected tick: %+v", tick)
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
