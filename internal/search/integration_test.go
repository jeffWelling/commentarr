package search_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/indexer"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/search"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/verify"
)

// TestSearchEndToEnd asserts the full Plan-2 flow:
// 1. Title in wanted queue
// 2. Prowlarr (httptest) returns two releases
// 3. Searcher runs → dedupes → scores → persists candidates
// 4. Candidates table contains both, ordered by score
// 5. next_search_at advances forward
func TestSearchEndToEnd(t *testing.T) {
	ctx := context.Background()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "key" {
			http.Error(w, "bad key", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"title":       "The.Thing.1982.Criterion.Commentary.1080p-GROUP",
				"size":        15_000_000_000,
				"downloadUrl": "http://ix/a.torrent",
				"infoHash":    "aaaaaa",
				"seeders":     42,
				"indexer":     "PublicPirates",
				"publishDate": "2026-04-20T00:00:00Z",
				"protocol":    "torrent",
			},
			{
				"title":       "The.Thing.1982.WEBRip.x264-NOBODY",
				"size":        800_000_000,
				"downloadUrl": "http://ix/b.torrent",
				"infoHash":    "bbbbbb",
				"seeders":     3,
				"indexer":     "PublicPirates",
				"publishDate": "2026-04-20T00:00:00Z",
				"protocol":    "torrent",
			},
		})
	}))
	defer server.Close()

	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	titles := title.NewRepo(d)
	q := queue.New(d)
	repo := search.NewRepo(d)

	rl := indexer.NewRateLimiter(indexer.RateLimitConfig{})
	cb := indexer.NewCircuitBreaker(indexer.CircuitBreakerConfig{ConsecutiveFailureThreshold: 5, OpenDuration: time.Hour})
	idx := indexer.NewProwlarr(indexer.ProwlarrConfig{BaseURL: server.URL, APIKey: "key", Name: "pw"}, rl, cb)
	searcher := search.NewSearcher(
		[]indexer.Indexer{idx},
		verify.NewVerifier(verify.DefaultRules(), 8),
		repo, q, titles, 100,
	)

	if err := titles.Insert(ctx, title.Title{ID: "t:e2e", Kind: title.KindMovie, DisplayName: "The Thing", Year: 1982, FilePath: "/thing.mkv"}); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkWanted(ctx, "t:e2e"); err != nil {
		t.Fatal(err)
	}

	now := time.Now().UTC().Truncate(time.Second)
	n, err := searcher.SearchDue(ctx, now)
	if err != nil {
		t.Fatalf("SearchDue: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 title processed, got %d", n)
	}

	cands, err := repo.ListCandidates(ctx, "t:e2e")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %+v", len(cands), cands)
	}
	if !cands[0].LikelyCommentary {
		t.Fatalf("top candidate should be likely commentary: %+v", cands[0])
	}
	if cands[0].Release.InfoHash != "aaaaaa" {
		t.Fatalf("top candidate should be Criterion release, got %+v", cands[0])
	}

	entry, err := q.Get(ctx, "t:e2e")
	if err != nil {
		t.Fatal(err)
	}
	if !entry.NextSearchAt.After(now) {
		t.Fatalf("next_search_at should advance, got %v (now=%v)", entry.NextSearchAt, now)
	}
}
