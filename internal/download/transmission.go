package download

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/jeffWelling/commentarr/internal/metrics"
)

// TransmissionConfig configures the Transmission RPC adapter.
type TransmissionConfig struct {
	BaseURL  string // e.g. http://transmission:9091
	Username string // optional (for authenticated instances)
	Password string // optional
	Name     string
	Timeout  time.Duration
}

// Transmission implements DownloadClient + Lister against Transmission's
// RPC-over-HTTP interface. The RPC endpoint is /transmission/rpc; the
// server returns 409 on the first call with an X-Transmission-Session-Id
// we must echo back — we handle that transparently.
type Transmission struct {
	cfg       TransmissionConfig
	hc        *http.Client

	mu        sync.Mutex
	sessionID string
}

// NewTransmission returns a Transmission adapter.
func NewTransmission(cfg TransmissionConfig) *Transmission {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &Transmission{cfg: cfg, hc: &http.Client{Timeout: timeout}}
}

func (t *Transmission) Name() string { return t.cfg.Name }

type rpcRequest struct {
	Method    string                 `json:"method"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

type rpcResponse struct {
	Result    string          `json:"result"`
	Arguments json.RawMessage `json:"arguments"`
}

// rpc makes a single RPC call, handling session-id renegotiation.
func (t *Transmission) rpc(ctx context.Context, req rpcRequest) (json.RawMessage, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	// Two-attempt loop: the first call may return 409 with a new session id.
	for attempt := 0; attempt < 2; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			strings.TrimRight(t.cfg.BaseURL, "/")+"/transmission/rpc",
			bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		httpReq.Header.Set("Content-Type", "application/json")
		if t.cfg.Username != "" {
			httpReq.SetBasicAuth(t.cfg.Username, t.cfg.Password)
		}
		t.mu.Lock()
		sid := t.sessionID
		t.mu.Unlock()
		if sid != "" {
			httpReq.Header.Set("X-Transmission-Session-Id", sid)
		}

		resp, err := t.hc.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("transmission rpc: %w", err)
		}

		if resp.StatusCode == http.StatusConflict {
			newSID := resp.Header.Get("X-Transmission-Session-Id")
			resp.Body.Close()
			if newSID == "" {
				return nil, fmt.Errorf("transmission 409 without session id")
			}
			t.mu.Lock()
			t.sessionID = newSID
			t.mu.Unlock()
			continue
		}

		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
			return nil, fmt.Errorf("transmission %d: %s", resp.StatusCode, b)
		}
		var rr rpcResponse
		if err := json.NewDecoder(resp.Body).Decode(&rr); err != nil {
			return nil, fmt.Errorf("transmission decode: %w", err)
		}
		if rr.Result != "success" {
			return nil, fmt.Errorf("transmission result=%q", rr.Result)
		}
		return rr.Arguments, nil
	}
	return nil, fmt.Errorf("transmission: session handshake failed")
}

// Add enqueues a download.
func (t *Transmission) Add(ctx context.Context, r AddRequest) (string, error) {
	args := map[string]interface{}{"filename": r.MagnetOrURL, "paused": r.Paused}
	if r.SavePath != "" {
		args["download-dir"] = r.SavePath
	}
	raw, err := t.rpc(ctx, rpcRequest{Method: "torrent-add", Arguments: args})
	if err != nil {
		return "", err
	}

	var out struct {
		TorrentAdded     *struct{ HashString string `json:"hashString"` } `json:"torrent-added"`
		TorrentDuplicate *struct{ HashString string `json:"hashString"` } `json:"torrent-duplicate"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("transmission add decode: %w", err)
	}
	var hash string
	switch {
	case out.TorrentAdded != nil:
		hash = out.TorrentAdded.HashString
	case out.TorrentDuplicate != nil:
		hash = out.TorrentDuplicate.HashString
	default:
		return "", fmt.Errorf("transmission add: no hash in response")
	}

	metrics.DownloadsQueuedTotal.WithLabelValues(t.cfg.Name).Inc()

	// Transmission doesn't use string labels; for parity with qBit we
	// rely on save-path to bucket downloads instead of categories.
	// The category arg is a no-op here.
	_ = r.Category
	return hash, nil
}

// transmissionTorrent is the subset of fields we consume.
type transmissionTorrent struct {
	HashString   string  `json:"hashString"`
	Name         string  `json:"name"`
	Status       int     `json:"status"`
	DownloadDir  string  `json:"downloadDir"`
	TotalSize    int64   `json:"totalSize"`
	PercentDone  float64 `json:"percentDone"`
	DoneDate     int64   `json:"doneDate"`
	Error        int     `json:"error"`
	Labels       []string `json:"labels"`
}

// Status returns a single torrent's status by hash.
func (t *Transmission) Status(ctx context.Context, id string) (Status, error) {
	torrents, err := t.getTorrents(ctx, []string{id})
	if err != nil {
		return Status{}, err
	}
	if len(torrents) == 0 {
		return Status{}, fmt.Errorf("transmission: no torrent with hash %q", id)
	}
	return transmissionToStatus(torrents[0]), nil
}

// ListByCategory returns torrents tagged with the given label. Transmission
// supports labels (since 3.00); we map our category to a label.
func (t *Transmission) ListByCategory(ctx context.Context, cat string) ([]Status, error) {
	torrents, err := t.getTorrents(ctx, nil)
	if err != nil {
		return nil, err
	}
	out := make([]Status, 0, len(torrents))
	for _, tr := range torrents {
		hasLabel := cat == ""
		for _, l := range tr.Labels {
			if l == cat {
				hasLabel = true
				break
			}
		}
		if !hasLabel {
			continue
		}
		out = append(out, transmissionToStatus(tr))
	}
	return out, nil
}

func (t *Transmission) getTorrents(ctx context.Context, hashes []string) ([]transmissionTorrent, error) {
	args := map[string]interface{}{
		"fields": []string{
			"hashString", "name", "status", "downloadDir", "totalSize",
			"percentDone", "doneDate", "error", "labels",
		},
	}
	if len(hashes) > 0 {
		args["ids"] = hashes
	}
	raw, err := t.rpc(ctx, rpcRequest{Method: "torrent-get", Arguments: args})
	if err != nil {
		return nil, err
	}
	var out struct {
		Torrents []transmissionTorrent `json:"torrents"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("transmission get decode: %w", err)
	}
	return out.Torrents, nil
}

// Remove deletes a torrent.
func (t *Transmission) Remove(ctx context.Context, id string, deleteFiles bool) error {
	args := map[string]interface{}{
		"ids":               []string{id},
		"delete-local-data": deleteFiles,
	}
	_, err := t.rpc(ctx, rpcRequest{Method: "torrent-remove", Arguments: args})
	return err
}

// Transmission numeric states — from its RPC spec.
const (
	tStatusStopped    = 0
	tStatusCheckWait  = 1
	tStatusCheck      = 2
	tStatusDownloadWait = 3
	tStatusDownload   = 4
	tStatusSeedWait   = 5
	tStatusSeed       = 6
)

func transmissionToStatus(t transmissionTorrent) Status {
	var state State
	switch t.Status {
	case tStatusStopped:
		state = StatePaused
	case tStatusCheckWait, tStatusCheck, tStatusDownloadWait:
		state = StateQueued
	case tStatusDownload:
		state = StateDownloading
	case tStatusSeedWait, tStatusSeed:
		state = StateCompleted
	default:
		state = StateOther
	}
	if t.Error != 0 {
		state = StateError
	}
	s := Status{
		ClientJobID: t.HashString,
		State:       state,
		SavePath:    t.DownloadDir,
		Name:        t.Name,
		SizeBytes:   t.TotalSize,
		Progress:    t.PercentDone,
	}
	if len(t.Labels) > 0 {
		s.Category = t.Labels[0]
	}
	if t.DoneDate > 0 {
		s.CompletedAt = time.Unix(t.DoneDate, 0).UTC()
	}
	return s
}
