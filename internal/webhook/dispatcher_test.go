package webhook

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jeffWelling/commentarr/internal/db"
)

func newDispatcher(t *testing.T) (*Dispatcher, *Repo) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	repo := NewRepo(d)
	return NewDispatcher(repo, DispatcherConfig{MaxAttempts: 3, RetryBackoff: 1 * time.Millisecond}), repo
}

// TestDispatcher_ObserverFiresEvenWithoutSubscribers pins the
// behaviour the SSE broker depends on: the observer hook runs on
// EVERY Dispatch, even when no external HTTP subscriber is
// registered. Without this, the Dashboard's recent-activity panel
// stays silent for any setup that doesn't have webhooks configured —
// which is most setups.
func TestDispatcher_ObserverFiresEvenWithoutSubscribers(t *testing.T) {
	disp, _ := newDispatcher(t)
	var got []Event
	disp.AddObserver(func(e Event, _ map[string]interface{}) {
		got = append(got, e)
	})

	if err := disp.Dispatch(context.Background(), EventImport, map[string]interface{}{"x": 1}); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != EventImport {
		t.Fatalf("observer got %v, want [OnImport]", got)
	}
}

// TestDispatcher_ObserverPanicDoesNotKillDispatcher: if a buggy
// observer panics, subsequent observers + the HTTP-fan-out path
// still run.
func TestDispatcher_ObserverPanicDoesNotKillDispatcher(t *testing.T) {
	disp, _ := newDispatcher(t)
	disp.AddObserver(func(_ Event, _ map[string]interface{}) {
		panic("oops")
	})
	var safeRan bool
	disp.AddObserver(func(_ Event, _ map[string]interface{}) {
		safeRan = true
	})

	if err := disp.Dispatch(context.Background(), EventImport, nil); err != nil {
		t.Fatalf("dispatch should swallow observer panics: %v", err)
	}
	if !safeRan {
		t.Fatal("second observer didn't run after first one panicked")
	}
}

func TestDispatcher_DeliversSuccess(t *testing.T) {
	var received Envelope
	var mu sync.Mutex
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &received)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	disp, repo := newDispatcher(t)
	ctx := context.Background()
	_ = repo.SaveSubscriber(ctx, Subscriber{
		ID: "s1", Name: "test", URL: server.URL, Events: []Event{EventImport}, Enabled: true,
	})

	if err := disp.Dispatch(ctx, EventImport, map[string]interface{}{"title_id": "t:1"}); err != nil {
		t.Fatal(err)
	}

	mu.Lock()
	defer mu.Unlock()
	if received.EventType != EventImport {
		t.Fatalf("unexpected event: %+v", received)
	}
	if received.Payload["title_id"] != "t:1" {
		t.Fatalf("payload lost: %+v", received.Payload)
	}
}

func TestDispatcher_RetriesOn500(t *testing.T) {
	var attempts int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt32(&attempts, 1)
		if n < 2 {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	disp, repo := newDispatcher(t)
	ctx := context.Background()
	_ = repo.SaveSubscriber(ctx, Subscriber{
		ID: "s1", URL: server.URL, Events: []Event{EventImport}, Enabled: true,
	})

	if err := disp.Dispatch(ctx, EventImport, map[string]interface{}{"x": 1}); err != nil {
		t.Fatal(err)
	}
	if atomic.LoadInt32(&attempts) < 2 {
		t.Fatalf("expected ≥2 attempts, got %d", attempts)
	}
}

func TestDispatcher_SkipsDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("disabled subscriber should not receive deliveries")
	}))
	defer server.Close()

	disp, repo := newDispatcher(t)
	ctx := context.Background()
	_ = repo.SaveSubscriber(ctx, Subscriber{
		ID: "s1", URL: server.URL, Events: []Event{EventImport}, Enabled: false,
	})
	if err := disp.Dispatch(ctx, EventImport, nil); err != nil {
		t.Fatal(err)
	}
}
