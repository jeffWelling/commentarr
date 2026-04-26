package download

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type qbitState struct {
	mu       sync.Mutex
	torrents map[string]Status
}

// newQBitServer returns an httptest.Server whose handler mimics enough
// of qBit's Web API v2 for the adapter under test.
func newQBitServer(t *testing.T) (*httptest.Server, *qbitState) {
	t.Helper()
	state := &qbitState{torrents: map[string]Status{}}
	mux := http.NewServeMux()

	mux.HandleFunc("/api/v2/auth/login", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		if r.Form.Get("username") == "admin" && r.Form.Get("password") == "pw" {
			http.SetCookie(w, &http.Cookie{Name: "SID", Value: "token", Path: "/"})
			_, _ = w.Write([]byte("Ok."))
			return
		}
		http.Error(w, "bad", http.StatusForbidden)
	})

	mux.HandleFunc("/api/v2/torrents/add", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("SID"); err != nil || c.Value != "token" {
			http.Error(w, "unauth", http.StatusForbidden)
			return
		}
		_ = r.ParseMultipartForm(1 << 20)
		urls := r.FormValue("urls")
		cat := r.FormValue("category")
		state.mu.Lock()
		hash := "h" + string(rune('a'+len(state.torrents)))
		state.torrents[hash] = Status{
			ClientJobID: hash, State: StateQueued, Category: cat, Name: urls,
		}
		state.mu.Unlock()
		_, _ = w.Write([]byte("Ok."))
	})

	mux.HandleFunc("/api/v2/torrents/info", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("SID"); err != nil || c.Value != "token" {
			http.Error(w, "unauth", http.StatusForbidden)
			return
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		cat := r.URL.Query().Get("category")
		hashes := r.URL.Query().Get("hashes")
		var out []map[string]interface{}
		emit := func(h string, st Status) {
			out = append(out, map[string]interface{}{
				"hash":          h,
				"name":          st.Name,
				"category":      st.Category,
				"state":         string(st.State),
				"size":          st.SizeBytes,
				"progress":      st.Progress,
				"content_path":  st.SavePath,
				"completion_on": st.CompletedAt.Unix(),
			})
		}
		if hashes != "" {
			for _, h := range strings.Split(hashes, "|") {
				if st, ok := state.torrents[h]; ok {
					emit(h, st)
				}
			}
		} else if cat != "" {
			for h, st := range state.torrents {
				if st.Category == cat {
					emit(h, st)
				}
			}
		} else {
			for h, st := range state.torrents {
				emit(h, st)
			}
		}
		_ = json.NewEncoder(w).Encode(out)
	})

	mux.HandleFunc("/api/v2/torrents/delete", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("SID"); err != nil || c.Value != "token" {
			http.Error(w, "unauth", http.StatusForbidden)
			return
		}
		state.mu.Lock()
		defer state.mu.Unlock()
		for _, h := range strings.Split(r.URL.Query().Get("hashes"), "|") {
			delete(state.torrents, h)
		}
		_, _ = w.Write([]byte("Ok."))
	})

	return httptest.NewServer(mux), state
}

func TestQBit_AddAndStatus(t *testing.T) {
	srv, state := newQBitServer(t)
	defer srv.Close()
	c := NewQBittorrent(QBittorrentConfig{
		BaseURL: srv.URL, Username: "admin", Password: "pw", Name: "qbit-test",
	})

	id, err := c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:foo", Category: "commentarr"})
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	state.mu.Lock()
	if _, ok := state.torrents[id]; !ok {
		state.mu.Unlock()
		t.Fatalf("server did not record job %q", id)
	}
	s := state.torrents[id]
	s.State = StateDownloading
	s.Progress = 0.5
	state.torrents[id] = s
	state.mu.Unlock()

	got, err := c.Status(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if got.State != StateDownloading || got.Progress != 0.5 {
		t.Fatalf("unexpected status: %+v", got)
	}
}

func TestQBit_Remove(t *testing.T) {
	srv, state := newQBitServer(t)
	defer srv.Close()
	c := NewQBittorrent(QBittorrentConfig{BaseURL: srv.URL, Username: "admin", Password: "pw", Name: "qbit"})

	id, _ := c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:x", Category: "commentarr"})
	if err := c.Remove(context.Background(), id, true); err != nil {
		t.Fatal(err)
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	if _, ok := state.torrents[id]; ok {
		t.Fatal("server should have removed torrent")
	}
}

func TestQBit_AuthFailure(t *testing.T) {
	srv, _ := newQBitServer(t)
	defer srv.Close()
	c := NewQBittorrent(QBittorrentConfig{BaseURL: srv.URL, Username: "admin", Password: "wrong", Name: "qbit"})
	_, err := c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:x"})
	if err == nil {
		t.Fatal("expected auth failure")
	}
}

func TestState_qbitMapping(t *testing.T) {
	cases := map[string]State{
		"queuedDL":     StateQueued,
		"downloading":  StateDownloading,
		"stalledDL":    StateDownloading,
		"uploading":    StateCompleted,
		"stalledUP":    StateCompleted,
		// pausedUP is the qBit 4.x term for "download done, seeding paused".
		// stoppedUP is the qBit 5.x rename. Both must trigger the importer
		// chain — surfaced live when qBit 5.x reported stoppedUP at
		// completion and the watcher misclassified it as StateOther.
		"pausedUP":     StateCompleted,
		"stoppedUP":    StateCompleted,
		"pausedDL":     StatePaused,
		"stoppedDL":    StatePaused,
		"error":        StateError,
		"missingFiles": StateError,
		"somethingNew": StateOther,
	}
	for in, want := range cases {
		if got := mapQBitState(in); got != want {
			t.Errorf("mapQBitState(%q)=%q want %q", in, got, want)
		}
	}
	_ = time.Now
}
