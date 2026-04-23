package download

import (
	"context"
	"time"
)

// EventKind enumerates the watcher's emitted events.
type EventKind string

const (
	EventCompleted EventKind = "completed"
	EventError     EventKind = "error"
)

// Event is one observation by the watcher.
type Event struct {
	Kind   EventKind
	Client string
	Status Status
}

// Watcher polls every configured DownloadClient for status changes on
// jobs tagged with the target category. It emits one Event per
// completion/error, deduplicated per (client, job-id).
type Watcher struct {
	clients  []DownloadClient
	category string
	interval time.Duration

	seen map[string]map[string]bool
}

// NewWatcher returns a Watcher polling every interval.
func NewWatcher(clients []DownloadClient, category string, interval time.Duration) *Watcher {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &Watcher{
		clients:  clients,
		category: category,
		interval: interval,
		seen:     map[string]map[string]bool{},
	}
}

// Run polls until ctx is cancelled, emitting events on the channel.
// Blocks; run in a goroutine.
func (w *Watcher) Run(ctx context.Context, out chan<- Event) {
	tick := time.NewTicker(w.interval)
	defer tick.Stop()
	for {
		w.pollOnce(ctx, out)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

func (w *Watcher) pollOnce(ctx context.Context, out chan<- Event) {
	for _, c := range w.clients {
		lister, ok := c.(Lister)
		if !ok {
			continue
		}
		jobs, err := lister.ListByCategory(ctx, w.category)
		if err != nil {
			continue
		}
		seen, ok := w.seen[c.Name()]
		if !ok {
			seen = map[string]bool{}
			w.seen[c.Name()] = seen
		}
		for _, j := range jobs {
			if seen[j.ClientJobID] {
				continue
			}
			switch j.State {
			case StateCompleted:
				seen[j.ClientJobID] = true
				select {
				case out <- Event{Kind: EventCompleted, Client: c.Name(), Status: j}:
				case <-ctx.Done():
					return
				}
			case StateError:
				seen[j.ClientJobID] = true
				select {
				case out <- Event{Kind: EventError, Client: c.Name(), Status: j}:
				case <-ctx.Done():
					return
				}
			}
		}
	}
}
