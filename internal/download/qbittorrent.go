package download

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jeffWelling/commentarr/internal/metrics"
)

// QBittorrentConfig configures the qBittorrent Web API v2 adapter.
type QBittorrentConfig struct {
	BaseURL  string
	Username string
	Password string
	Name     string
	Timeout  time.Duration
}

// QBittorrent implements DownloadClient + Lister against qBit's
// Web API v2. Login is cookie-based; subsequent calls reuse the cookie
// via a cookie jar on the http.Client.
type QBittorrent struct {
	cfg QBittorrentConfig
	hc  *http.Client

	mu       sync.Mutex
	loggedIn bool
}

// NewQBittorrent returns a qBittorrent adapter.
func NewQBittorrent(cfg QBittorrentConfig) *QBittorrent {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	jar, _ := cookiejar.New(nil)
	return &QBittorrent{
		cfg: cfg,
		hc:  &http.Client{Timeout: timeout, Jar: jar},
	}
}

func (q *QBittorrent) Name() string { return q.cfg.Name }

func (q *QBittorrent) login(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.loggedIn {
		return nil
	}
	form := url.Values{"username": {q.cfg.Username}, "password": {q.cfg.Password}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/auth/login",
		strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := q.hc.Do(req)
	if err != nil {
		return fmt.Errorf("qbit login: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(body)), "Ok") {
		return fmt.Errorf("qbit login failed: %d %s", resp.StatusCode, string(body))
	}
	q.loggedIn = true
	return nil
}

// Add uploads the magnet/URL via multipart form and returns the torrent's
// hash. qBit's /torrents/add doesn't return the hash, so we follow up
// with /torrents/info filtered by category and take the newest entry.
func (q *QBittorrent) Add(ctx context.Context, r AddRequest) (string, error) {
	if err := q.login(ctx); err != nil {
		return "", err
	}

	body := &strings.Builder{}
	boundary := "commentarrboundary"
	addField := func(name, val string) {
		fmt.Fprintf(body, "--%s\r\nContent-Disposition: form-data; name=%q\r\n\r\n%s\r\n", boundary, name, val)
	}
	addField("urls", r.MagnetOrURL)
	if r.Category != "" {
		addField("category", r.Category)
	}
	if r.SavePath != "" {
		addField("savepath", r.SavePath)
	}
	if r.Paused {
		addField("paused", "true")
	}
	fmt.Fprintf(body, "--%s--\r\n", boundary)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(q.cfg.BaseURL, "/")+"/api/v2/torrents/add",
		strings.NewReader(body.String()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err := q.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("qbit add: %w", err)
	}
	defer resp.Body.Close()
	buf, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	if resp.StatusCode != http.StatusOK || !strings.HasPrefix(strings.TrimSpace(string(buf)), "Ok") {
		return "", fmt.Errorf("qbit add %d: %s", resp.StatusCode, string(buf))
	}

	metrics.DownloadsQueuedTotal.WithLabelValues(q.cfg.Name).Inc()

	return q.findRecentByCategory(ctx, r.Category)
}

type qbitTorrent struct {
	Hash         string  `json:"hash"`
	Name         string  `json:"name"`
	Category     string  `json:"category"`
	State        string  `json:"state"`
	Size         int64   `json:"size"`
	Progress     float64 `json:"progress"`
	ContentPath  string  `json:"content_path"`
	CompletionOn int64   `json:"completion_on"`
}

func (q *QBittorrent) findRecentByCategory(ctx context.Context, cat string) (string, error) {
	u := strings.TrimRight(q.cfg.BaseURL, "/") + "/api/v2/torrents/info"
	if cat != "" {
		u += "?category=" + url.QueryEscape(cat)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := q.hc.Do(req)
	if err != nil {
		return "", fmt.Errorf("qbit info: %w", err)
	}
	defer resp.Body.Close()
	var ts []qbitTorrent
	if err := json.NewDecoder(resp.Body).Decode(&ts); err != nil {
		return "", fmt.Errorf("decode torrents/info: %w", err)
	}
	if len(ts) == 0 {
		return "", fmt.Errorf("qbit add: no torrent matched category %q after add", cat)
	}
	for _, t := range ts {
		if t.CompletionOn == 0 {
			return t.Hash, nil
		}
	}
	return ts[0].Hash, nil
}

// Status returns the current state of a single torrent.
func (q *QBittorrent) Status(ctx context.Context, id string) (Status, error) {
	if err := q.login(ctx); err != nil {
		return Status{}, err
	}
	u := strings.TrimRight(q.cfg.BaseURL, "/") + "/api/v2/torrents/info?hashes=" + url.QueryEscape(id)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Status{}, err
	}
	resp, err := q.hc.Do(req)
	if err != nil {
		return Status{}, fmt.Errorf("qbit status: %w", err)
	}
	defer resp.Body.Close()
	var ts []qbitTorrent
	if err := json.NewDecoder(resp.Body).Decode(&ts); err != nil {
		return Status{}, fmt.Errorf("decode: %w", err)
	}
	if len(ts) == 0 {
		return Status{}, fmt.Errorf("qbit: no torrent with hash %q", id)
	}
	return toStatus(ts[0]), nil
}

// ListByCategory returns every torrent assigned to the given category.
func (q *QBittorrent) ListByCategory(ctx context.Context, cat string) ([]Status, error) {
	if err := q.login(ctx); err != nil {
		return nil, err
	}
	u := strings.TrimRight(q.cfg.BaseURL, "/") + "/api/v2/torrents/info?category=" + url.QueryEscape(cat)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := q.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("qbit list: %w", err)
	}
	defer resp.Body.Close()
	var ts []qbitTorrent
	if err := json.NewDecoder(resp.Body).Decode(&ts); err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(ts))
	for _, t := range ts {
		out = append(out, toStatus(t))
	}
	return out, nil
}

// Remove deletes a torrent and optionally its files.
func (q *QBittorrent) Remove(ctx context.Context, id string, deleteFiles bool) error {
	if err := q.login(ctx); err != nil {
		return err
	}
	u := strings.TrimRight(q.cfg.BaseURL, "/") + "/api/v2/torrents/delete"
	u += "?hashes=" + url.QueryEscape(id) + "&deleteFiles=" + strconv.FormatBool(deleteFiles)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := q.hc.Do(req)
	if err != nil {
		return fmt.Errorf("qbit delete: %w", err)
	}
	resp.Body.Close()
	return nil
}

func toStatus(t qbitTorrent) Status {
	s := Status{
		ClientJobID: t.Hash,
		State:       mapQBitState(t.State),
		Category:    t.Category,
		SavePath:    t.ContentPath,
		Name:        t.Name,
		SizeBytes:   t.Size,
		Progress:    t.Progress,
	}
	if t.CompletionOn > 0 {
		s.CompletedAt = time.Unix(t.CompletionOn, 0).UTC()
	}
	return s
}

// mapQBitState collapses qBit's rich state vocabulary into our enum.
// qBit 5.x renamed pausedDL→stoppedDL and pausedUP→stoppedUP — both
// vocabularies are accepted here so the watcher works against either.
// stoppedUP / pausedUP are treated as Completed (the download IS
// done; seeding is just paused) rather than Paused, so the importer
// chain runs even if the operator has qBit configured to stop
// seeding immediately at completion.
func mapQBitState(s string) State {
	switch s {
	case "queuedDL", "checkingDL", "allocating":
		return StateQueued
	case "downloading", "stalledDL", "metaDL", "forcedDL":
		return StateDownloading
	case "uploading", "stalledUP", "forcedUP", "queuedUP", "checkingUP",
		"pausedUP", "stoppedUP":
		return StateCompleted
	case "pausedDL", "stoppedDL":
		return StatePaused
	case "error", "missingFiles", "unknown":
		return StateError
	default:
		return StateOther
	}
}
