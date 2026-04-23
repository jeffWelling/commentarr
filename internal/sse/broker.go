// Package sse broadcasts server-sent events to connected clients. One
// global Broker fan-outs every state-transition event (OnImport,
// OnTrash, etc.) to every subscriber.
package sse

import "sync"

// Event is one fan-out payload.
type Event struct {
	Kind    string
	Payload string // pre-serialized JSON
}

// Broker is a single-process pub/sub for UI event streams.
type Broker struct {
	mu   sync.Mutex
	subs map[chan Event]struct{}
}

// NewBroker returns a new Broker.
func NewBroker() *Broker { return &Broker{subs: map[chan Event]struct{}{}} }

// Subscribe returns a buffered channel that will receive every
// subsequent Publish. Channels are buffered (size 16) to absorb small
// bursts; Publish drops events for slow subscribers rather than blocking.
func (b *Broker) Subscribe() chan Event {
	ch := make(chan Event, 16)
	b.mu.Lock()
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

// Unsubscribe removes ch and closes it. Safe to call from any goroutine;
// idempotent.
func (b *Broker) Unsubscribe(ch chan Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.subs[ch]; !ok {
		return
	}
	delete(b.subs, ch)
	close(ch)
}

// Publish sends e to every subscriber. Non-blocking: a slow subscriber
// drops the event.
func (b *Broker) Publish(e Event) {
	b.mu.Lock()
	subs := make([]chan Event, 0, len(b.subs))
	for ch := range b.subs {
		subs = append(subs, ch)
	}
	b.mu.Unlock()
	for _, ch := range subs {
		select {
		case ch <- e:
		default:
		}
	}
}
