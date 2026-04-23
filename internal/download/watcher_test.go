package download

import (
	"context"
	"sync"
	"testing"
	"time"
)

func TestWatcher_FiresOnCompletion(t *testing.T) {
	client := &fakeClient{name: "fake", jobs: map[string]Status{
		"j1": {ClientJobID: "j1", State: StateDownloading, Category: "commentarr"},
	}}
	events := make(chan Event, 4)
	w := NewWatcher([]DownloadClient{client}, "commentarr", 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		w.Run(ctx, events)
	}()

	time.Sleep(25 * time.Millisecond)
	client.jobs["j1"] = Status{ClientJobID: "j1", State: StateCompleted, Category: "commentarr", CompletedAt: time.Now().UTC()}

	select {
	case ev := <-events:
		if ev.Kind != EventCompleted || ev.Status.ClientJobID != "j1" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("watcher did not emit a completion event in time")
	}

	cancel()
	wg.Wait()
}

func TestWatcher_SkipsOtherCategories(t *testing.T) {
	client := &fakeClient{name: "fake", jobs: map[string]Status{
		"j1": {ClientJobID: "j1", State: StateCompleted, Category: "other"},
	}}
	events := make(chan Event, 4)
	w := NewWatcher([]DownloadClient{client}, "commentarr", 10*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	w.Run(ctx, events)

	select {
	case ev := <-events:
		t.Fatalf("expected no event for non-matching category, got %+v", ev)
	default:
	}
}
