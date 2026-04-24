package download

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

type delugeServerState struct {
	mu       sync.Mutex
	torrents map[string]delugeTorrentStatus
	lastLabel map[string]string
}

func newDelugeServer(t *testing.T) (*httptest.Server, *delugeServerState) {
	t.Helper()
	state := &delugeServerState{torrents: map[string]delugeTorrentStatus{}, lastLabel: map[string]string{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/json", func(w http.ResponseWriter, r *http.Request) {
		var req delugeRPCRequest
		_ = json.NewDecoder(r.Body).Decode(&req)

		write := func(result interface{}) {
			raw, _ := json.Marshal(result)
			_ = json.NewEncoder(w).Encode(delugeRPCResponse{Result: raw, ID: req.ID})
		}

		state.mu.Lock()
		defer state.mu.Unlock()

		switch req.Method {
		case "auth.login":
			write(true)
		case "core.add_torrent_magnet":
			hash := "deadbeef"
			state.torrents[hash] = delugeTorrentStatus{
				Name: "t1", State: "Downloading", Progress: 50.0, TotalSize: 1 << 30,
			}
			write(hash)
		case "label.set_torrent":
			hash := req.Params[0].(string)
			label := req.Params[1].(string)
			ts := state.torrents[hash]
			ts.Label = label
			state.torrents[hash] = ts
			state.lastLabel[hash] = label
			write(nil)
		case "core.get_torrent_status":
			hash := req.Params[0].(string)
			ts, ok := state.torrents[hash]
			if !ok {
				_ = json.NewEncoder(w).Encode(delugeRPCResponse{
					Error: &delugeRPCError{Message: "not found"}, ID: req.ID,
				})
				return
			}
			write(ts)
		case "core.get_torrents_status":
			write(state.torrents)
		case "core.remove_torrent":
			hash := req.Params[0].(string)
			delete(state.torrents, hash)
			write(true)
		default:
			_ = json.NewEncoder(w).Encode(delugeRPCResponse{
				Error: &delugeRPCError{Message: "unknown method"}, ID: req.ID,
			})
		}
	})
	return httptest.NewServer(mux), state
}

func TestDeluge_AddAndStatus(t *testing.T) {
	srv, _ := newDelugeServer(t)
	defer srv.Close()
	c := NewDeluge(DelugeConfig{BaseURL: srv.URL, Password: "pw", Name: "del-test"})

	id, err := c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:?x", Category: "commentarr"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id != "deadbeef" {
		t.Fatalf("expected deadbeef, got %q", id)
	}
	s, err := c.Status(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if s.State != StateDownloading {
		t.Fatalf("expected downloading, got %s", s.State)
	}
	if s.Progress != 0.5 {
		t.Fatalf("expected progress=0.5 (50%% translated), got %v", s.Progress)
	}
	if s.Category != "commentarr" {
		t.Fatalf("expected category=commentarr, got %q", s.Category)
	}
}

func TestDeluge_ListByCategory(t *testing.T) {
	srv, _ := newDelugeServer(t)
	defer srv.Close()
	c := NewDeluge(DelugeConfig{BaseURL: srv.URL, Password: "pw", Name: "del"})
	_, _ = c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:?x", Category: "commentarr"})

	list, err := c.ListByCategory(context.Background(), "commentarr")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1, got %d", len(list))
	}
}

func TestDeluge_Remove(t *testing.T) {
	srv, state := newDelugeServer(t)
	defer srv.Close()
	c := NewDeluge(DelugeConfig{BaseURL: srv.URL, Password: "pw", Name: "del"})
	id, _ := c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:?x"})
	if err := c.Remove(context.Background(), id, true); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if len(state.torrents) != 0 {
		t.Fatalf("expected no torrents, got %d", len(state.torrents))
	}
}

func TestDeluge_StateMapping(t *testing.T) {
	cases := map[string]State{
		"Queued":      StateQueued,
		"Downloading": StateDownloading,
		"Seeding":     StateCompleted,
		"Paused":      StatePaused,
		"Error":       StateError,
		"Unknown":     StateOther,
	}
	for in, want := range cases {
		got := delugeToStatus(delugeTorrentStatus{State: in})
		if got.State != want {
			t.Errorf("state %q: want %s got %s", in, want, got.State)
		}
	}
}
