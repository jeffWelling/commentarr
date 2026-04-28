package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jeffWelling/commentarr/internal/download"
	"github.com/jeffWelling/commentarr/internal/importer"
	"github.com/jeffWelling/commentarr/internal/indexer"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/search"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/verify"
)

// TestPipeline_EndToEnd drives the full search → pick → watch →
// importer-route chain against httptest backends. The importer itself
// is stubbed (its real classify/place/trash chain needs ffprobe + real
// video files; see internal/importer/integration_test.go for that
// part). What's verified here is the *wiring* between every component
// in serve.go.
func TestPipeline_EndToEnd(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	d := newTestDB(t)
	titles := title.NewRepo(d)
	q := queue.New(d)
	candRepo := search.NewRepo(d)
	jobs := download.NewJobRepo(d)

	// 1. Seed: one wanted title pointing at a real (placeholder) file
	libDir := t.TempDir()
	origPath := filepath.Join(libDir, "test-movie.mkv")
	if err := os.WriteFile(origPath, []byte("placeholder original"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := titles.Insert(ctx, title.Title{
		ID: "tt-1", Kind: title.KindMovie, DisplayName: "Test Movie",
		Year: 2020, FilePath: origPath,
	}); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkWanted(ctx, "tt-1"); err != nil {
		t.Fatal(err)
	}

	// 2. Stand up an httptest Prowlarr that returns one
	// commentary-flavoured release for any query.
	prowlarrSrv := newFakeProwlarr(t)
	defer prowlarrSrv.Close()

	// 3. Stand up a fake qBittorrent. Tracks added torrents; we'll
	// flip one to a completed state by hand to drive the watcher.
	qbitState := newFakeQBit()
	qbitSrv := httptest.NewServer(qbitState.handler())
	defer qbitSrv.Close()

	// 4. Wire up the real adapters + orchestrators.
	rl := indexer.NewRateLimiter(indexer.RateLimitConfig{RequestsPerMinute: 60, Burst: 10})
	cb := indexer.NewCircuitBreaker(indexer.CircuitBreakerConfig{
		ConsecutiveFailureThreshold: 5, OpenDuration: time.Hour,
	})
	idx := indexer.NewProwlarr(indexer.ProwlarrConfig{
		BaseURL: prowlarrSrv.URL, APIKey: "test-key", Name: "prowlarr-test",
	}, rl, cb)
	searcher := search.NewSearcher(
		[]indexer.Indexer{idx},
		verify.NewVerifier(verify.DefaultRules(), 8),
		candRepo, q, titles, 100,
	)
	qbitClient := download.NewQBittorrent(download.QBittorrentConfig{
		BaseURL: qbitSrv.URL, Username: "admin", Password: "pw", Name: "qbit-test",
	})
	picker := search.NewPicker(candRepo, jobs, qbitClient, nil, "commentarr", 8)

	// === STAGE 1: search ===
	processed, err := searcher.SearchDue(ctx, time.Now())
	if err != nil {
		t.Fatalf("SearchDue: %v", err)
	}
	if processed != 1 {
		t.Fatalf("expected 1 title processed, got %d", processed)
	}
	cands, err := candRepo.ListCandidates(ctx, "tt-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) == 0 {
		t.Fatal("search persisted no candidates")
	}
	if !cands[0].LikelyCommentary {
		t.Errorf("top candidate not flagged likely-commentary: score=%d", cands[0].Score)
	}

	// === STAGE 2: pick ===
	jobID, queued, err := picker.PickAndQueueOne(ctx, "tt-1")
	if err != nil {
		t.Fatalf("PickAndQueueOne: %v", err)
	}
	if !queued || jobID == "" {
		t.Fatalf("picker didn't queue: queued=%v id=%q", queued, jobID)
	}
	if !qbitState.has(jobID) {
		t.Fatalf("qbit didn't receive add for job %q", jobID)
	}
	job, err := jobs.FindByClientJob(ctx, "qbit-test", jobID)
	if err != nil {
		t.Fatalf("job not persisted: %v", err)
	}
	if job.Status != "queued" || job.TitleID != "tt-1" {
		t.Errorf("unexpected job: %+v", job)
	}

	// === STAGE 3: simulate qBit completing the download ===
	completedDir := t.TempDir()
	finishedFile := filepath.Join(completedDir, "test-movie-criterion.mkv")
	if err := os.WriteFile(finishedFile, []byte("placeholder downloaded"), 0o644); err != nil {
		t.Fatal(err)
	}
	qbitState.markCompleted(jobID, completedDir)

	// === STAGE 4: watcher emits a completion event ===
	watcher := download.NewWatcher([]download.DownloadClient{qbitClient}, "commentarr", 50*time.Millisecond)
	events := make(chan download.Event, 16)
	go watcher.Run(ctx, events)

	var ev download.Event
	select {
	case ev = <-events:
	case <-time.After(3 * time.Second):
		t.Fatal("watcher never emitted a completion event")
	}
	if ev.Kind != download.EventCompleted {
		t.Fatalf("expected completed event, got %s", ev.Kind)
	}
	if ev.Status.ClientJobID != jobID {
		t.Errorf("event wrong job: got %q want %q", ev.Status.ClientJobID, jobID)
	}
	if ev.Status.SavePath != completedDir {
		t.Errorf("event wrong save path: got %q want %q", ev.Status.SavePath, completedDir)
	}

	// === STAGE 5: route through stubbed importer ===
	imp := &stubImporter{res: importer.Result{
		Outcome: importer.OutcomeSuccess, FinalPath: filepath.Join(libDir, "test-movie-criterion.mkv"),
	}}
	handleEvent(ctx, jobs, titles, q, imp, ev, 0)
	if imp.calls != 1 {
		t.Errorf("importer call count: got %d want 1", imp.calls)
	}

	// Final state assertions
	job, _ = jobs.FindByClientJob(ctx, "qbit-test", jobID)
	if job.Status != "imported" {
		t.Errorf("expected job imported, got %q", job.Status)
	}
	wanted, _ := q.Get(ctx, "tt-1")
	if wanted.Status != queue.StatusResolved {
		t.Errorf("expected queue resolved, got %q", wanted.Status)
	}
}

// --- fakes -----------------------------------------------------------

func newFakeProwlarr(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" {
			http.Error(w, "bad key", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/v1/search" {
			http.NotFound(w, r)
			return
		}
		release := []map[string]interface{}{{
			"title":       "Test.Movie.2020.Criterion.Commentary.1080p.BluRay.x264-CREW",
			"size":        20_000_000_000,
			"infoHash":    "DEADBEEF01",
			"seeders":     50,
			"leechers":    2,
			"indexer":     "Fake Public",
			"publishDate": time.Now().Format(time.RFC3339),
			"protocol":    "torrent",
		}}
		_ = json.NewEncoder(w).Encode(release)
	}))
}

type fakeQBit struct {
	mu       sync.Mutex
	torrents map[string]download.Status
	nextID   int
}

func newFakeQBit() *fakeQBit {
	return &fakeQBit{torrents: map[string]download.Status{}}
}

func (f *fakeQBit) has(id string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.torrents[id]
	return ok
}

func (f *fakeQBit) markCompleted(id, savePath string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	st := f.torrents[id]
	st.State = download.StateCompleted
	st.SavePath = savePath
	st.Progress = 1
	st.CompletedAt = time.Now().UTC()
	f.torrents[id] = st
}

func (f *fakeQBit) handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("username") == "admin" && r.Form.Get("password") == "pw" {
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "token", Path: "/"})
			_, _ = w.Write([]byte("Ok."))
			return
		}
		http.Error(w, "bad", http.StatusForbidden)
	})

	mux.HandleFunc("/api/v2/torrents/add", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("SID"); err != nil || c.Value != "token" {
			http.Error(w, "unauth", http.StatusForbidden)
			return
		}
		_ = r.ParseMultipartForm(1 << 20)
		urls := r.FormValue("urls")
		cat := r.FormValue("category")
		f.mu.Lock()
		f.nextID++
		// Use a deterministic-but-uniqueish hash so failures point to
		// the right add. The qBit adapter computes the hash from the
		// magnet URI rather than reading our response, so the value
		// here just has to round-trip via the listing endpoint.
		hash := computeInfoHashFromMagnet(urls)
		f.torrents[hash] = download.Status{
			ClientJobID: hash, State: download.StateDownloading,
			Category: cat, Name: urls, Progress: 0.1,
		}
		f.mu.Unlock()
		_, _ = w.Write([]byte("Ok."))
	})

	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("SID"); err != nil || c.Value != "token" {
			http.Error(w, "unauth", http.StatusForbidden)
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		cat := r.URL.Query().Get("category")
		hashes := r.URL.Query().Get("hashes")
		var out []map[string]interface{}
		emit := func(h string, st download.Status) {
			row := map[string]interface{}{
				"hash":         h,
				"name":         st.Name,
				"category":     st.Category,
				"state":        qbitStateString(st.State),
				"size":         st.SizeBytes,
				"progress":     st.Progress,
				"content_path": st.SavePath,
			}
			if !st.CompletedAt.IsZero() {
				row["completion_on"] = st.CompletedAt.Unix()
			}
			out = append(out, row)
		}
		if hashes != "" {
			for _, h := range strings.Split(hashes, "|") {
				if st, ok := f.torrents[h]; ok {
					emit(h, st)
				}
			}
		} else if cat != "" {
			for h, st := range f.torrents {
				if st.Category == cat {
					emit(h, st)
				}
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})
	return mux
}

// qbitStateString maps our state enum back to one of qBit's strings
// so the round-trip parses to the same enum value when the adapter
// reads the response.
func qbitStateString(s download.State) string {
	switch s {
	case download.StateQueued:
		return "queuedDL"
	case download.StateDownloading:
		return "downloading"
	case download.StateCompleted:
		return "uploading"
	case download.StatePaused:
		return "pausedDL"
	case download.StateError:
		return "error"
	default:
		return "unknown"
	}
}

// computeInfoHashFromMagnet pulls the btih portion out of a magnet URI.
// The picker hands the client a `magnet:?xt=urn:btih:HEX` URI; that
// HEX is the hash the adapter treats as the job id. Falling back to
// the URL itself keeps URL-based candidates working too.
func computeInfoHashFromMagnet(uri string) string {
	const prefix = "magnet:?xt=urn:btih:"
	if strings.HasPrefix(uri, prefix) {
		rest := uri[len(prefix):]
		if i := strings.IndexAny(rest, "&"); i >= 0 {
			return rest[:i]
		}
		return rest
	}
	return uri
}
