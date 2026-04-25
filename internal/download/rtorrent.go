package download

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jeffWelling/commentarr/internal/metrics"
)

// RTorrentConfig configures the rTorrent XMLRPC adapter.
//
// rTorrent speaks XMLRPC. The typical setup exposes it over HTTP via
// ruTorrent's /RPC2 endpoint or directly via an scgi-to-http proxy. We
// talk HTTP+XMLRPC here; pure SCGI transport is out of scope for v1.
type RTorrentConfig struct {
	BaseURL  string // e.g. http://rtorrent:8080/RPC2
	Username string // optional basic auth
	Password string
	Name     string
	Timeout  time.Duration
}

// RTorrent implements DownloadClient + Lister against rTorrent's XMLRPC.
type RTorrent struct {
	cfg RTorrentConfig
	hc  *http.Client
}

// NewRTorrent returns an RTorrent adapter.
func NewRTorrent(cfg RTorrentConfig) *RTorrent {
	timeout := cfg.Timeout
	if timeout == 0 {
		timeout = 30 * time.Second
	}
	return &RTorrent{cfg: cfg, hc: &http.Client{Timeout: timeout}}
}

func (r *RTorrent) Name() string { return r.cfg.Name }

// Minimal XMLRPC encoding — we only need string, int, array, struct.
type xmlrpcMethodCall struct {
	XMLName    xml.Name `xml:"methodCall"`
	MethodName string   `xml:"methodName"`
	Params     xmlrpcParams
}

type xmlrpcParams struct {
	XMLName xml.Name      `xml:"params"`
	Param   []xmlrpcParam `xml:"param"`
}

type xmlrpcParam struct {
	Value xmlrpcValue
}

type xmlrpcValue struct {
	String  *string         `xml:"string,omitempty"`
	Int     *int            `xml:"i4,omitempty"`
	Array   *xmlrpcArray    `xml:"array,omitempty"`
}

type xmlrpcArray struct {
	Data xmlrpcArrayData `xml:"data"`
}

type xmlrpcArrayData struct {
	Value []xmlrpcValue `xml:"value"`
}

// call issues one XMLRPC method call and returns the raw response body
// for the test/caller to parse (parsing XMLRPC responses is verbose;
// we keep it minimal for our adapter needs).
func (r *RTorrent) call(ctx context.Context, method string, params ...xmlrpcValue) ([]byte, error) {
	mc := xmlrpcMethodCall{MethodName: method}
	for _, p := range params {
		mc.Params.Param = append(mc.Params.Param, xmlrpcParam{Value: p})
	}
	body, err := xml.Marshal(mc)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(r.cfg.BaseURL, "/"), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml")
	if r.cfg.Username != "" {
		req.SetBasicAuth(r.cfg.Username, r.cfg.Password)
	}
	resp, err := r.hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("rtorrent xmlrpc: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 256))
		return nil, fmt.Errorf("rtorrent %d: %s", resp.StatusCode, b)
	}
	return io.ReadAll(resp.Body)
}

// strVal constructs an XMLRPC string value.
func strVal(s string) xmlrpcValue { return xmlrpcValue{String: &s} }

// Add enqueues a download via load.start. rTorrent doesn't return the
// infohash from load.start, so we compute it from the magnet URI or
// derive it from a subsequent listing. For v1 we return the magnet URI
// as a temporary ID; callers that need the real hash can call Status
// followed by a manual match on name.
func (r *RTorrent) Add(ctx context.Context, req AddRequest) (string, error) {
	_, err := r.call(ctx, "load.start", strVal(""), strVal(req.MagnetOrURL))
	if err != nil {
		return "", err
	}
	metrics.DownloadsQueuedTotal.WithLabelValues(r.cfg.Name).Inc()
	return req.MagnetOrURL, nil
}

// Status is intentionally minimal for v1 — rTorrent's XMLRPC response
// is verbose and needs a full parser we haven't justified yet. Calling
// this against real rTorrent returns a stub indicating "unknown state"
// that's safe to consume (won't trigger false completions).
func (r *RTorrent) Status(ctx context.Context, id string) (Status, error) {
	return Status{
		ClientJobID: id,
		State:       StateOther,
		Name:        id,
	}, nil
}

// ListByCategory: rTorrent uses "views" as its category-equivalent.
// v1 returns an empty list — real implementation pending.
func (r *RTorrent) ListByCategory(ctx context.Context, cat string) ([]Status, error) {
	return nil, nil
}

// Remove issues d.erase. rTorrent's erase semantics differ from other
// clients — d.erase removes the torrent metadata but doesn't take a
// "delete files on disk" flag; what's left on disk depends on
// rTorrent's session config. The deleteFiles arg is accepted for
// interface parity but ignored.
func (r *RTorrent) Remove(ctx context.Context, id string, deleteFiles bool) error {
	_ = deleteFiles
	_, err := r.call(ctx, "d.erase", strVal(id))
	return err
}
