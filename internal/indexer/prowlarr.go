package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jeffWelling/commentarr/internal/metrics"
)

// ProwlarrConfig configures a Prowlarr backend.
type ProwlarrConfig struct {
	BaseURL string
	APIKey  string
	Name    string        // metric label + human-readable identifier
	Timeout time.Duration // per-request HTTP timeout; 0 = 30s default
}

// Prowlarr implements Indexer against Prowlarr's /api/v1/search endpoint.
type Prowlarr struct {
	cfg ProwlarrConfig
	rl  *RateLimiter
	cb  *CircuitBreaker
	hc  *http.Client
}

// NewProwlarr returns a Prowlarr adapter. The rate limiter and circuit
// breaker are passed in so multiple Prowlarr instances against the same
// remote can share a bucket if needed, and so tests can inject
// permissive ones.
func NewProwlarr(cfg ProwlarrConfig, rl *RateLimiter, cb *CircuitBreaker) *Prowlarr {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Prowlarr{
		cfg: cfg,
		rl:  rl,
		cb:  cb,
		hc:  &http.Client{Timeout: timeout},
	}
}

func (p *Prowlarr) Name() string { return p.cfg.Name }

// prowlarrRelease mirrors the JSON shape we consume.
type prowlarrRelease struct {
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

// Search issues a single Prowlarr query with rate limiting + circuit
// breaker + timeout. Metrics are emitted regardless of outcome.
func (p *Prowlarr) Search(ctx context.Context, q Query) ([]Release, error) {
	start := time.Now()
	if err := p.rl.Wait(ctx); err != nil {
		metrics.IndexerQueriesTotal.WithLabelValues(p.cfg.Name, "rate_limited").Inc()
		return nil, fmt.Errorf("rate-limit wait: %w", err)
	}

	var results []Release
	err := p.cb.Do(func() error {
		var err error
		results, err = p.doSearch(ctx, q)
		return err
	})

	metrics.IndexerQueryDurationSeconds.WithLabelValues(p.cfg.Name).Observe(time.Since(start).Seconds())
	metrics.IndexerCircuitState.WithLabelValues(p.cfg.Name).Set(float64(p.cb.State()))

	switch {
	case err == nil:
		metrics.IndexerQueriesTotal.WithLabelValues(p.cfg.Name, "success").Inc()
	case errors.Is(err, ErrCircuitOpen):
		metrics.IndexerQueriesTotal.WithLabelValues(p.cfg.Name, "circuit_open").Inc()
	default:
		metrics.IndexerQueriesTotal.WithLabelValues(p.cfg.Name, "other").Inc()
	}

	return results, err
}

func (p *Prowlarr) doSearch(ctx context.Context, q Query) ([]Release, error) {
	u, err := url.Parse(strings.TrimRight(p.cfg.BaseURL, "/") + "/api/v1/search")
	if err != nil {
		return nil, fmt.Errorf("parse url: %w", err)
	}
	qs := u.Query()
	qs.Set("query", q.Title)
	qs.Set("type", "search")
	if q.Limit > 0 {
		qs.Set("limit", strconv.Itoa(q.Limit))
	}
	for _, c := range q.Categories {
		qs.Add("categories", strconv.Itoa(c))
	}
	u.RawQuery = qs.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("X-Api-Key", p.cfg.APIKey)
	req.Header.Set("Accept", "application/json")

	resp, err := p.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		metrics.IndexerQueriesRejectedByServerTotal.
			WithLabelValues(p.cfg.Name, strconv.Itoa(resp.StatusCode)).Inc()
		return nil, fmt.Errorf("prowlarr %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var raw []prowlarrRelease
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	out := make([]Release, 0, len(raw))
	for _, pr := range raw {
		link := pr.DownloadURL
		if link == "" {
			link = pr.MagnetURL
		}
		out = append(out, Release{
			InfoHash:    strings.ToLower(pr.InfoHash),
			URL:         link,
			Title:       pr.Title,
			SizeBytes:   pr.Size,
			Seeders:     pr.Seeders,
			Leechers:    pr.Leechers,
			Indexer:     pr.Indexer,
			PublishedAt: pr.PublishDate,
			Protocol:    pr.Protocol,
		})
	}
	return out, nil
}
