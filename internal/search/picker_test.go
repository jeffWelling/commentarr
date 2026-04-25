package search

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/download"
	"github.com/jeffWelling/commentarr/internal/indexer"
	"github.com/jeffWelling/commentarr/internal/metrics"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/verify"

	dto "github.com/prometheus/client_model/go"
)

type fakePickClient struct {
	mu       sync.Mutex
	added    []download.AddRequest
	nextID   int
	failNext bool
}

func (f *fakePickClient) Name() string { return "fake" }
func (f *fakePickClient) Add(_ context.Context, r download.AddRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failNext {
		f.failNext = false
		return "", context.DeadlineExceeded
	}
	f.added = append(f.added, r)
	f.nextID++
	return "job-" + itoa(f.nextID), nil
}
func (f *fakePickClient) Status(context.Context, string) (download.Status, error) {
	return download.Status{}, nil
}
func (f *fakePickClient) Remove(context.Context, string, bool) error { return nil }

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	digits := []byte{}
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}

func TestPicker_PicksHighestScoringLikelyCommentary(t *testing.T) {
	candRepo, jobRepo, titles := newPickerRepos(t)
	persistCands(t, candRepo, titles, "tt-1", []verify.Scored{
		{Release: indexer.Release{Title: "low", InfoHash: "aaa"}, Score: 5, LikelyCommentary: true},
		{Release: indexer.Release{Title: "high", InfoHash: "bbb"}, Score: 12, LikelyCommentary: true},
		{Release: indexer.Release{Title: "not-likely", InfoHash: "ccc"}, Score: 20, LikelyCommentary: false},
	})

	client := &fakePickClient{}
	p := NewPicker(candRepo, jobRepo, client, nil, "commentarr", 8)
	jobID, queued, err := p.PickAndQueueOne(context.Background(), "tt-1")
	if err != nil {
		t.Fatal(err)
	}
	if !queued || jobID == "" {
		t.Fatalf("expected queued job, got id=%q queued=%v", jobID, queued)
	}
	client.mu.Lock()
	defer client.mu.Unlock()
	if len(client.added) != 1 {
		t.Fatalf("expected 1 client add, got %d", len(client.added))
	}
	if client.added[0].MagnetOrURL != "magnet:?xt=urn:btih:bbb" {
		t.Errorf("picked the wrong release: %+v", client.added[0])
	}
	if client.added[0].Category != "commentarr" {
		t.Errorf("category not propagated: %q", client.added[0].Category)
	}
}

func TestPicker_NoQualifyingCandidateIsNoOp(t *testing.T) {
	candRepo, jobRepo, titles := newPickerRepos(t)
	persistCands(t, candRepo, titles, "tt-1", []verify.Scored{
		{Release: indexer.Release{Title: "below", InfoHash: "aaa"}, Score: 3, LikelyCommentary: true},
		{Release: indexer.Release{Title: "above-but-not-likely", InfoHash: "bbb"}, Score: 99, LikelyCommentary: false},
	})

	client := &fakePickClient{}
	p := NewPicker(candRepo, jobRepo, client, nil, "commentarr", 8)
	_, queued, err := p.PickAndQueueOne(context.Background(), "tt-1")
	if err != nil {
		t.Fatal(err)
	}
	if queued {
		t.Fatal("expected no queue")
	}
	if len(client.added) != 0 {
		t.Fatalf("client should not have been called: %+v", client.added)
	}
}

func TestPicker_SkipsTitleWithExistingInflightJob(t *testing.T) {
	candRepo, jobRepo, titles := newPickerRepos(t)
	persistCands(t, candRepo, titles, "tt-1", []verify.Scored{
		{Release: indexer.Release{Title: "good", InfoHash: "aaa"}, Score: 12, LikelyCommentary: true},
	})
	// pre-existing queued job for the same title
	_, _ = jobRepo.Save(context.Background(), download.Job{
		ClientName: "fake", ClientJobID: "prior", TitleID: "tt-1",
	})

	client := &fakePickClient{}
	p := NewPicker(candRepo, jobRepo, client, nil, "commentarr", 8)
	_, queued, err := p.PickAndQueueOne(context.Background(), "tt-1")
	if err != nil {
		t.Fatal(err)
	}
	if queued {
		t.Fatal("should have skipped due to existing queued job")
	}
	if len(client.added) != 0 {
		t.Fatalf("client should not have been called: %+v", client.added)
	}
}

func TestPicker_RetryAllowedAfterErrorJob(t *testing.T) {
	candRepo, jobRepo, titles := newPickerRepos(t)
	persistCands(t, candRepo, titles, "tt-1", []verify.Scored{
		{Release: indexer.Release{Title: "good", InfoHash: "aaa"}, Score: 12, LikelyCommentary: true},
	})
	id, _ := jobRepo.Save(context.Background(), download.Job{
		ClientName: "fake", ClientJobID: "old", TitleID: "tt-1",
	})
	_ = jobRepo.MarkStatus(context.Background(), id, "error", "stalled")

	client := &fakePickClient{}
	p := NewPicker(candRepo, jobRepo, client, nil, "commentarr", 8)
	_, queued, err := p.PickAndQueueOne(context.Background(), "tt-1")
	if err != nil {
		t.Fatal(err)
	}
	if !queued {
		t.Fatal("error'd job should not block retry")
	}
}

func TestPicker_EmitsDecisionMetric(t *testing.T) {
	candRepo, jobRepo, titles := newPickerRepos(t)
	persistCands(t, candRepo, titles, "tt-1", []verify.Scored{
		{Release: indexer.Release{Title: "good", InfoHash: "aaa"}, Score: 12, LikelyCommentary: true},
	})

	before := counterValue(t, "queued")
	p := NewPicker(candRepo, jobRepo, &fakePickClient{}, nil, "commentarr", 8)
	if _, _, err := p.PickAndQueueOne(context.Background(), "tt-1"); err != nil {
		t.Fatal(err)
	}
	if got := counterValue(t, "queued"); got <= before {
		t.Errorf("expected picker_decisions_total{queued} to increment; got before=%v after=%v", before, got)
	}
}

func counterValue(t *testing.T, decision string) float64 {
	t.Helper()
	m := &dto.Metric{}
	if err := metrics.PickerDecisionsTotal.WithLabelValues(decision).Write(m); err != nil {
		t.Fatal(err)
	}
	return m.GetCounter().GetValue()
}

func TestPicker_DryRunSkipsAddAndSave(t *testing.T) {
	candRepo, jobRepo, titles := newPickerRepos(t)
	persistCands(t, candRepo, titles, "tt-1", []verify.Scored{
		{Release: indexer.Release{Title: "good", InfoHash: "aaa"}, Score: 12, LikelyCommentary: true},
	})

	client := &fakePickClient{}
	var logged []string
	p := NewPicker(candRepo, jobRepo, client, nil, "commentarr", 8).WithDryRun(true)
	p.logf = func(format string, args ...any) {
		logged = append(logged, fmt.Sprintf(format, args...))
	}

	jobID, queued, err := p.PickAndQueueOne(context.Background(), "tt-1")
	if err != nil {
		t.Fatal(err)
	}
	if queued || jobID != "" {
		t.Fatalf("dry-run must not return queued; got id=%q queued=%v", jobID, queued)
	}
	if len(client.added) != 0 {
		t.Fatalf("dry-run must not call client.Add; got %+v", client.added)
	}
	// No job row should have been persisted either.
	if _, err := jobRepo.FindByClientJob(context.Background(), "fake", "any"); err == nil {
		t.Fatal("dry-run must not save a job row")
	}
	if len(logged) == 0 {
		t.Fatal("dry-run must log what it would have queued")
	}
	if !strings.Contains(logged[0], "DRY-RUN") || !strings.Contains(logged[0], "tt-1") {
		t.Errorf("log line missing DRY-RUN or title id: %q", logged[0])
	}
}

func TestPicker_DryRunEmitsDryRunMetric(t *testing.T) {
	candRepo, jobRepo, titles := newPickerRepos(t)
	persistCands(t, candRepo, titles, "tt-1", []verify.Scored{
		{Release: indexer.Release{Title: "good", InfoHash: "aaa"}, Score: 12, LikelyCommentary: true},
	})
	before := counterValue(t, "dry_run")
	p := NewPicker(candRepo, jobRepo, &fakePickClient{}, nil, "commentarr", 8).WithDryRun(true)
	p.logf = func(string, ...any) {}
	if _, _, err := p.PickAndQueueOne(context.Background(), "tt-1"); err != nil {
		t.Fatal(err)
	}
	if got := counterValue(t, "dry_run"); got <= before {
		t.Errorf("dry_run counter didn't increment: before=%v after=%v", before, got)
	}
}

func TestPicker_FallsBackToURLWhenInfoHashMissing(t *testing.T) {
	candRepo, jobRepo, titles := newPickerRepos(t)
	persistCands(t, candRepo, titles, "tt-1", []verify.Scored{
		{Release: indexer.Release{Title: "url-only", URL: "http://indexer/dl/abc.torrent"}, Score: 12, LikelyCommentary: true},
	})

	client := &fakePickClient{}
	p := NewPicker(candRepo, jobRepo, client, nil, "commentarr", 8)
	_, queued, err := p.PickAndQueueOne(context.Background(), "tt-1")
	if err != nil {
		t.Fatal(err)
	}
	if !queued {
		t.Fatal("expected queue with url fallback")
	}
	if client.added[0].MagnetOrURL != "http://indexer/dl/abc.torrent" {
		t.Errorf("expected url fallback, got %q", client.added[0].MagnetOrURL)
	}
}

func newPickerRepos(t *testing.T) (*Repo, *download.JobRepo, title.Repo) {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "p.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = d.Close() })
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return NewRepo(d), download.NewJobRepo(d), title.NewRepo(d)
}

func persistCands(t *testing.T, r *Repo, titles title.Repo, titleID string, ss []verify.Scored) {
	t.Helper()
	if err := titles.Insert(context.Background(), title.Title{
		ID: titleID, Kind: title.KindMovie, DisplayName: "test", FilePath: "/m.mkv",
	}); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveCandidates(context.Background(), titleID, ss); err != nil {
		t.Fatal(err)
	}
}
