package download

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestRTorrent_AddCallsLoadStart(t *testing.T) {
	var hits int32
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		w.Header().Set("Content-Type", "text/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><methodResponse><params><param><value><int>0</int></value></param></params></methodResponse>`))
	}))
	defer srv.Close()

	c := NewRTorrent(RTorrentConfig{BaseURL: srv.URL, Name: "rt-test"})
	id, err := c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:?xt=urn:btih:abc"})
	if err != nil {
		t.Fatal(err)
	}
	if id != "magnet:?xt=urn:btih:abc" {
		t.Fatalf("id: %q", id)
	}
	if atomic.LoadInt32(&hits) != 1 {
		t.Fatalf("expected 1 server hit, got %d", hits)
	}
	if !strings.Contains(gotBody, "<methodName>load.start</methodName>") {
		t.Fatalf("request body missing load.start: %s", gotBody)
	}
}

func TestRTorrent_AuthFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauth", http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewRTorrent(RTorrentConfig{BaseURL: srv.URL, Name: "rt-test"})
	_, err := c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:?x"})
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestRTorrent_RemoveIssuesErase(t *testing.T) {
	var gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		gotBody = string(body)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><methodResponse><params><param><value><int>0</int></value></param></params></methodResponse>`))
	}))
	defer srv.Close()
	c := NewRTorrent(RTorrentConfig{BaseURL: srv.URL, Name: "rt"})
	if err := c.Remove(context.Background(), "abc123", true); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gotBody, "d.erase") {
		t.Fatalf("expected d.erase in request: %s", gotBody)
	}
}

func TestRTorrent_StatusReturnsStub(t *testing.T) {
	// rTorrent XMLRPC response parsing isn't implemented yet; Status
	// returns a stub. Caller that wants real state must either wait
	// for Plan 5+ or consume ListByCategory (also stubbed).
	c := NewRTorrent(RTorrentConfig{BaseURL: "http://localhost:9999", Name: "rt"})
	s, err := c.Status(context.Background(), "abc")
	if err != nil {
		t.Fatal(err)
	}
	if s.State != StateOther {
		t.Fatalf("expected StateOther, got %s", s.State)
	}
}
