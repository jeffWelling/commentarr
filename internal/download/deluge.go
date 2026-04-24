package download

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jeffWelling/commentarr/internal/metrics"
)

// DelugeConfig configures the Deluge JSON-RPC adapter (Web UI v2).
type DelugeConfig struct {
	BaseURL  string // e.g. http://deluge:8112
	Password string // Web UI password
	Name     string
	Timeout  time.Duration
}

// Deluge implements DownloadClient + Lister against the Deluge Web UI's
// /json endpoint. Auth is session-cookie based; the adapter logs in on
// first call and transparently re-logs in if the cookie is rejected.
type Deluge struct {
	cfg DelugeConfig
	hc  *http.Client

	reqID atomic.Int64

	mu       sync.Mutex
	loggedIn bool
}

// NewDeluge returns a Deluge adapter.
func NewDeluge(cfg DelugeConfig) *Deluge {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	jar, _ := cookiejar.New(nil)
	return &Deluge{cfg: cfg, hc: &http.Client{Timeout: timeout, Jar: jar}}
}

func (d *Deluge) Name() string { return d.cfg.Name }

type delugeRPCRequest struct {
	Method string        `json:"method"`
	Params []interface{} `json:"params"`
	ID     int64         `json:"id"`
}

type delugeRPCResponse struct {
	Result json.RawMessage  `json:"result"`
	Error  *delugeRPCError  `json:"error"`
	ID     int64            `json:"id"`
}

type delugeRPCError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func (d *Deluge) login(ctx context.Context) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.loggedIn {
		return nil
	}
	raw, err := d.callRaw(ctx, "auth.login", []interface{}{d.cfg.Password})
	if err != nil {
		return fmt.Errorf("deluge login: %w", err)
	}
	var ok bool
	if err := json.Unmarshal(raw, &ok); err != nil {
		return fmt.Errorf("deluge login decode: %w", err)
	}
	if !ok {
		return fmt.Errorf("deluge login rejected")
	}
	d.loggedIn = true
	return nil
}

// callRaw performs one JSON-RPC call and returns the raw result.
func (d *Deluge) callRaw(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	id := d.reqID.Add(1)
	body, err := json.Marshal(delugeRPCRequest{Method: method, Params: params, ID: id})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(d.cfg.BaseURL, "/")+"/json", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("deluge rpc: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("deluge %d: %s", resp.StatusCode, b)
	}
	var rr delugeRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
		return nil, fmt.Errorf("deluge decode: %w", err)
	}
	if rr.Error != nil {
		return nil, fmt.Errorf("deluge rpc %s: %s", method, rr.Error.Message)
	}
	return rr.Result, nil
}

// call logs in if needed then makes the RPC.
func (d *Deluge) call(ctx context.Context, method string, params []interface{}) (json.RawMessage, error) {
	if err := d.login(ctx); err != nil {
		return nil, err
	}
	return d.callRaw(ctx, method, params)
}

// Add enqueues a download. Returns the torrent hash (infohash).
func (d *Deluge) Add(ctx context.Context, r AddRequest) (string, error) {
	opts := map[string]interface{}{}
	if r.SavePath != "" {
		opts["download_location"] = r.SavePath
	}
	if r.Paused {
		opts["add_paused"] = true
	}
	raw, err := d.call(ctx, "core.add_torrent_magnet", []interface{}{r.MagnetOrURL, opts})
	if err != nil {
		return "", err
	}
	var hash string
	if err := json.Unmarshal(raw, &hash); err != nil {
		return "", fmt.Errorf("deluge add decode: %w", err)
	}

	// Apply label (category). label plugin must be enabled server-side
	// — we try and ignore errors.
	if r.Category != "" {
		_, _ = d.call(ctx, "label.set_torrent", []interface{}{hash, r.Category})
	}

	metrics.DownloadsQueuedTotal.WithLabelValues(d.cfg.Name).Inc()
	return hash, nil
}

// delugeTorrentStatus mirrors the subset of fields we consume.
type delugeTorrentStatus struct {
	Hash         string  `json:"hash"`
	Name         string  `json:"name"`
	State        string  `json:"state"`
	TotalSize    int64   `json:"total_size"`
	Progress     float64 `json:"progress"` // 0..100 in Deluge
	SavePath     string  `json:"save_path"`
	CompletedAt  float64 `json:"completed_time"`
	Label        string  `json:"label"`
}

// Status returns a single torrent by hash.
func (d *Deluge) Status(ctx context.Context, id string) (Status, error) {
	raw, err := d.call(ctx, "core.get_torrent_status", []interface{}{id, []string{
		"name", "state", "total_size", "progress", "save_path", "completed_time", "label",
	}})
	if err != nil {
		return Status{}, err
	}
	var ts delugeTorrentStatus
	if err := json.Unmarshal(raw, &ts); err != nil {
		return Status{}, fmt.Errorf("deluge status decode: %w", err)
	}
	ts.Hash = id
	return delugeToStatus(ts), nil
}

// ListByCategory returns torrents with the matching label.
func (d *Deluge) ListByCategory(ctx context.Context, cat string) ([]Status, error) {
	filter := map[string]interface{}{}
	if cat != "" {
		filter["label"] = cat
	}
	raw, err := d.call(ctx, "core.get_torrents_status", []interface{}{filter, []string{
		"name", "state", "total_size", "progress", "save_path", "completed_time", "label",
	}})
	if err != nil {
		return nil, err
	}
	// core.get_torrents_status returns a map keyed by hash.
	var m map[string]delugeTorrentStatus
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("deluge list decode: %w", err)
	}
	out := make([]Status, 0, len(m))
	for hash, ts := range m {
		ts.Hash = hash
		out = append(out, delugeToStatus(ts))
	}
	return out, nil
}

// Remove deletes a torrent.
func (d *Deluge) Remove(ctx context.Context, id string, deleteFiles bool) error {
	_, err := d.call(ctx, "core.remove_torrent", []interface{}{id, deleteFiles})
	return err
}

func delugeToStatus(t delugeTorrentStatus) Status {
	var state State
	switch t.State {
	case "Queued":
		state = StateQueued
	case "Downloading":
		state = StateDownloading
	case "Seeding":
		state = StateCompleted
	case "Paused":
		state = StatePaused
	case "Error":
		state = StateError
	default:
		state = StateOther
	}
	s := Status{
		ClientJobID: t.Hash,
		State:       state,
		Category:    t.Label,
		SavePath:    t.SavePath,
		Name:        t.Name,
		SizeBytes:   t.TotalSize,
		Progress:    t.Progress / 100.0,
	}
	if t.CompletedAt > 0 {
		s.CompletedAt = time.Unix(int64(t.CompletedAt), 0).UTC()
	}
	return s
}
