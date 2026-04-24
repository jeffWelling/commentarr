package download

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type transmissionServer struct {
	mu        sync.Mutex
	sessionID string
	torrents  []transmissionTorrent
}

func newTransmissionServer(t *testing.T) (*httptest.Server, *transmissionServer) {
	t.Helper()
	state := &transmissionServer{sessionID: "handshake-token"}
	mux := http.NewServeMux()
	mux.HandleFunc("/transmission/rpc", func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		defer state.mu.Unlock()

		gotSID := r.Header.Get("X-Transmission-Session-Id")
		if gotSID != state.sessionID {
			w.Header().Set("X-Transmission-Session-Id", state.sessionID)
			http.Error(w, "409 conflict", http.StatusConflict)
			return
		}

		var req rpcRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		switch req.Method {
		case "torrent-add":
			hash := "abc123"
			state.torrents = append(state.torrents, transmissionTorrent{
				HashString: hash, Name: "test-torrent", Status: tStatusDownload,
				PercentDone: 0.25, TotalSize: 1 << 30, Labels: []string{"commentarr"},
			})
			_ = json.NewEncoder(w).Encode(rpcResponse{
				Result: "success",
				Arguments: json.RawMessage(`{"torrent-added":{"hashString":"` + hash + `"}}`),
			})
		case "torrent-get":
			args := map[string]interface{}{"torrents": state.torrents}
			raw, _ := json.Marshal(args)
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success", Arguments: raw})
		case "torrent-remove":
			state.torrents = nil
			_ = json.NewEncoder(w).Encode(rpcResponse{Result: "success", Arguments: json.RawMessage(`{}`)})
		default:
			http.Error(w, "unknown method: "+req.Method, http.StatusBadRequest)
		}
	})
	return httptest.NewServer(mux), state
}

func TestTransmission_AddHandshakesThenSucceeds(t *testing.T) {
	srv, state := newTransmissionServer(t)
	defer srv.Close()
	c := NewTransmission(TransmissionConfig{BaseURL: srv.URL, Name: "tr-test"})

	id, err := c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:?foo"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id != "abc123" {
		t.Fatalf("expected abc123, got %q", id)
	}
	state.mu.Lock()
	n := len(state.torrents)
	state.mu.Unlock()
	if n != 1 {
		t.Fatalf("expected 1 torrent on server, got %d", n)
	}
}

func TestTransmission_StatusMapping(t *testing.T) {
	srv, _ := newTransmissionServer(t)
	defer srv.Close()
	c := NewTransmission(TransmissionConfig{BaseURL: srv.URL, Name: "tr-test"})

	id, err := c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:?foo"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.Status(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if s.State != StateDownloading {
		t.Fatalf("expected downloading, got %s", s.State)
	}
	if s.Progress != 0.25 {
		t.Fatalf("progress: got %v", s.Progress)
	}
}

func TestTransmission_ListByCategory(t *testing.T) {
	srv, _ := newTransmissionServer(t)
	defer srv.Close()
	c := NewTransmission(TransmissionConfig{BaseURL: srv.URL, Name: "tr-test"})
	_, _ = c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:?foo"})

	list, err := c.ListByCategory(context.Background(), "commentarr")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
}

func TestTransmission_Remove(t *testing.T) {
	srv, state := newTransmissionServer(t)
	defer srv.Close()
	c := NewTransmission(TransmissionConfig{BaseURL: srv.URL, Name: "tr-test"})
	id, _ := c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:?foo"})
	if err := c.Remove(context.Background(), id, true); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.torrents) != 0 {
		t.Fatalf("expected remove to clear torrents, got %d", len(state.torrents))
	}
}

func TestTransmission_StateMapping(t *testing.T) {
	cases := map[int]State{
		tStatusStopped:      StatePaused,
		tStatusCheckWait:    StateQueued,
		tStatusCheck:        StateQueued,
		tStatusDownloadWait: StateQueued,
		tStatusDownload:     StateDownloading,
		tStatusSeedWait:     StateCompleted,
		tStatusSeed:         StateCompleted,
	}
	for status, want := range cases {
		got := transmissionToStatus(transmissionTorrent{Status: status})
		if got.State != want {
			t.Errorf("status %d: want %s got %s", status, want, got.State)
		}
	}
}
