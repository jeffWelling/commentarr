package library

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jeffWelling/commentarr/internal/title"
)

// JellyfinConfig configures a Jellyfin (or Emby — same API) backend.
type JellyfinConfig struct {
	BaseURL string // e.g. https://jellyfin.home.example.com:8096
	APIKey  string // API key; header name depends on flavour (see Mode)
	UserID  string // user id — Items endpoint is user-scoped
	Name    string // metric/log label
	Timeout time.Duration
	// EmbyMode — when true, uses X-Emby-Token header; otherwise
	// X-Emby-Token also works for Jellyfin (compatible) but we
	// default to X-MediaBrowser-Token for pure Jellyfin.
	EmbyMode bool
}

type jellyfinSource struct {
	cfg JellyfinConfig
	hc  *http.Client
}

// NewJellyfinSource returns a LibrarySource backed by the given
// Jellyfin server.
func NewJellyfinSource(cfg JellyfinConfig) LibrarySource {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &jellyfinSource{cfg: cfg, hc: &http.Client{Timeout: timeout}}
}

func (j *jellyfinSource) Name() string { return j.cfg.Name }

// jellyfinItem is the subset of Jellyfin's Item DTO we need.
type jellyfinItem struct {
	Id               string `json:"Id"`
	Name             string `json:"Name"`
	Type             string `json:"Type"` // "Movie" | "Episode"
	ProductionYear   int    `json:"ProductionYear"`
	Path             string `json:"Path"`
	SeriesName       string `json:"SeriesName"`
	ParentIndexNumber int   `json:"ParentIndexNumber"` // season
	IndexNumber       int   `json:"IndexNumber"`        // episode
	SeriesId          string `json:"SeriesId"`
}

type jellyfinItemsResp struct {
	Items []jellyfinItem `json:"Items"`
}

// List enumerates every movie + episode the user has access to.
func (j *jellyfinSource) List(ctx context.Context) ([]title.Title, error) {
	items, err := j.fetchItems(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]title.Title, 0, len(items))
	for _, it := range items {
		out = append(out, jellyfinItemToTitle(it))
	}
	return out, nil
}

// Refresh triggers a library rescan. Jellyfin's /Library/Refresh
// refreshes everything; targeted rescans require item IDs.
func (j *jellyfinSource) Refresh(ctx context.Context, _ string) error {
	u := strings.TrimRight(j.cfg.BaseURL, "/") + "/Library/Refresh"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return err
	}
	j.setAuth(req)
	resp, err := j.hc.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	return nil
}

func (j *jellyfinSource) setAuth(req *http.Request) {
	header := "X-MediaBrowser-Token"
	if j.cfg.EmbyMode {
		header = "X-Emby-Token"
	}
	req.Header.Set(header, j.cfg.APIKey)
	req.Header.Set("Accept", "application/json")
}

func (j *jellyfinSource) fetchItems(ctx context.Context) ([]jellyfinItem, error) {
	u, _ := url.Parse(strings.TrimRight(j.cfg.BaseURL, "/") + "/Users/" + url.PathEscape(j.cfg.UserID) + "/Items")
	qs := u.Query()
	qs.Set("Recursive", "true")
	qs.Set("IncludeItemTypes", "Movie,Episode")
	qs.Set("Fields", "Path,ProductionYear,SeriesName,SeriesId,ParentIndexNumber,IndexNumber")
	u.RawQuery = qs.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	j.setAuth(req)

	resp, err := j.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jellyfin items: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("jellyfin items %d: %s", resp.StatusCode, body)
	}
	var out jellyfinItemsResp
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("jellyfin decode: %w", err)
	}
	return out.Items, nil
}

func jellyfinItemToTitle(it jellyfinItem) title.Title {
	kind := title.KindMovie
	displayName := it.Name
	var seriesID string
	if it.Type == "Episode" {
		kind = title.KindEpisode
		displayName = fmt.Sprintf("%s - S%02dE%02d", it.SeriesName, it.ParentIndexNumber, it.IndexNumber)
		seriesID = "jf-series:" + it.SeriesId
	}
	return title.Title{
		ID:          "jf:" + it.Id,
		Kind:        kind,
		DisplayName: displayName,
		Year:        it.ProductionYear,
		SeriesID:    seriesID,
		Season:      it.ParentIndexNumber,
		Episode:     it.IndexNumber,
		FilePath:    it.Path,
	}
}
