package download

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// fakeClient lets tests assert orchestration logic without HTTP.
// All fields are guarded by mu; tests that mutate jobs from one
// goroutine while the watcher reads from another would otherwise race.
type fakeClient struct {
	mu         sync.Mutex
	name       string
	jobs       map[string]Status
	addCalled  int
	removeCall int
}

func (f *fakeClient) Name() string { return f.name }

func (f *fakeClient) Add(ctx context.Context, req AddRequest) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.addCalled++
	id := "job-1"
	if f.jobs == nil {
		f.jobs = map[string]Status{}
	}
	f.jobs[id] = Status{ClientJobID: id, State: StateQueued, Category: req.Category}
	return id, nil
}

func (f *fakeClient) Status(ctx context.Context, id string) (Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.jobs[id]
	if !ok {
		return Status{}, errors.New("not found")
	}
	return s, nil
}

func (f *fakeClient) Remove(ctx context.Context, id string, deleteFiles bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.removeCall++
	delete(f.jobs, id)
	return nil
}

func (f *fakeClient) ListByCategory(ctx context.Context, category string) ([]Status, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]Status, 0, len(f.jobs))
	for _, s := range f.jobs {
		if s.Category == category {
			out = append(out, s)
		}
	}
	return out, nil
}

// setJob lets tests mutate state under the mutex.
func (f *fakeClient) setJob(id string, s Status) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.jobs == nil {
		f.jobs = map[string]Status{}
	}
	f.jobs[id] = s
}

func TestFakeClient_SatisfiesInterface(t *testing.T) {
	var c DownloadClient = &fakeClient{name: "fake"}
	id, err := c.Add(context.Background(), AddRequest{MagnetOrURL: "magnet:?...", Category: "commentarr"})
	if err != nil {
		t.Fatal(err)
	}
	s, err := c.Status(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	if s.State != StateQueued {
		t.Fatalf("expected StateQueued, got %q", s.State)
	}
}

func TestFakeClient_SatisfiesLister(t *testing.T) {
	var c Lister = &fakeClient{name: "fake"}
	_, _ = c.Add(context.Background(), AddRequest{Category: "commentarr"})
	got, err := c.ListByCategory(context.Background(), "commentarr")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d", len(got))
	}
}

func TestState_AllEnumerated(t *testing.T) {
	for _, st := range []State{StateQueued, StateDownloading, StateCompleted, StatePaused, StateError, StateOther} {
		if string(st) == "" {
			t.Fatal("state string must be non-empty")
		}
	}
	_ = time.Now
}
