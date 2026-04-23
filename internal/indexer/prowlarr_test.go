package indexer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/jeffWelling/commentarr/internal/metrics"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// prowlarrTestResponse mirrors the subset of Prowlarr's /api/v1/search
// JSON our adapter consumes. We include only the fields the adapter reads.
type prowlarrTestResponse []struct {
	Title       string    `json:"title"`
	Size        int64     `json:"size"`
	DownloadURL string    `json:"downloadUrl"`
	MagnetURL   string    `json:"magnetUrl"`
	InfoHash    string    `json:"infoHash"`
	Seeders     int       `json:"seeders"`
	Leechers    int       `json:"leechers"`
	Indexer     string    `json:"indexer"`
	PublishDate time.Time `json:"publishDate"`
	Protocol    string    `json:"protocol"`
}

func TestProwlarrAdapter_SearchReturnsReleases(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Api-Key") != "test-key" {
			http.Error(w, "bad api key", http.StatusUnauthorized)
			return
		}
		if r.URL.Path != "/api/v1/search" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("query") == "" {
			http.Error(w, "missing query", http.StatusBadRequest)
			return
		}
		resp := prowlarrTestResponse{
			{
				Title:       "The.Matrix.1999.Criterion.1080p.BluRay.x264-CREW",
				Size:        15_000_000_000,
				DownloadURL: "http://ix.example.com/d/abc.torrent",
				InfoHash:    "ABCDEF0123",
				Seeders:     42,
				Leechers:    3,
				Indexer:     "Public Pirates",
				PublishDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
				Protocol:    "torrent",
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	rl := NewRateLimiter(RateLimitConfig{})
	cb := NewCircuitBreaker(CircuitBreakerConfig{ConsecutiveFailureThreshold: 5, OpenDuration: time.Hour})
	p := NewProwlarr(ProwlarrConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Name:    "prowlarr-test",
	}, rl, cb)

	got, err := p.Search(context.Background(), Query{Title: "The Matrix", Year: 1999, Limit: 100})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 release, got %d", len(got))
	}
	r := got[0]
	if r.InfoHash != "abcdef0123" {
		t.Errorf("InfoHash not lowercased: %q", r.InfoHash)
	}
	if r.Title != "The.Matrix.1999.Criterion.1080p.BluRay.x264-CREW" {
		t.Errorf("title: %q", r.Title)
	}
	if r.Seeders != 42 {
		t.Errorf("seeders: %d", r.Seeders)
	}
}

func TestProwlarrAdapter_AuthFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer server.Close()

	rl := NewRateLimiter(RateLimitConfig{})
	cb := NewCircuitBreaker(CircuitBreakerConfig{ConsecutiveFailureThreshold: 5, OpenDuration: time.Hour})
	p := NewProwlarr(ProwlarrConfig{BaseURL: server.URL, APIKey: "bad", Name: "prowlarr-test"}, rl, cb)

	_, err := p.Search(context.Background(), Query{Title: "x"})
	if err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestProwlarr_EmitsSuccessMetric(t *testing.T) {
	metrics.IndexerQueriesTotal.Reset()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(prowlarrTestResponse{})
	}))
	defer server.Close()

	rl := NewRateLimiter(RateLimitConfig{})
	cb := NewCircuitBreaker(CircuitBreakerConfig{ConsecutiveFailureThreshold: 5, OpenDuration: time.Hour})
	p := NewProwlarr(ProwlarrConfig{BaseURL: server.URL, APIKey: "k", Name: "metric-test"}, rl, cb)

	if _, err := p.Search(context.Background(), Query{Title: "x"}); err != nil {
		t.Fatal(err)
	}
	got := testutil.ToFloat64(metrics.IndexerQueriesTotal.WithLabelValues("metric-test", "success"))
	if got != 1 {
		t.Fatalf("expected success counter=1, got %v", got)
	}
}

func TestProwlarrAdapter_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer server.Close()

	rl := NewRateLimiter(RateLimitConfig{})
	cb := NewCircuitBreaker(CircuitBreakerConfig{ConsecutiveFailureThreshold: 3, OpenDuration: 50 * time.Millisecond})
	p := NewProwlarr(ProwlarrConfig{BaseURL: server.URL, APIKey: "k", Name: "prowlarr-test"}, rl, cb)

	for i := 0; i < 3; i++ {
		_, err := p.Search(context.Background(), Query{Title: "x"})
		if err == nil {
			t.Fatalf("call %d: expected error", i)
		}
	}
	_, err := p.Search(context.Background(), Query{Title: "x"})
	if err == nil {
		t.Fatal("expected circuit-open short-circuit")
	}
}
