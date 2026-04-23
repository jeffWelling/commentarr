package sse

import (
	"testing"
	"time"
)

func TestBroker_PublishToOneClient(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe()
	defer b.Unsubscribe(sub)

	go b.Publish(Event{Kind: "test", Payload: `{"ok":true}`})

	select {
	case ev := <-sub:
		if ev.Kind != "test" {
			t.Fatalf("unexpected event: %+v", ev)
		}
	case <-time.After(100 * time.Millisecond):
		t.Fatal("did not receive event")
	}
}

func TestBroker_FansOutToAllSubscribers(t *testing.T) {
	b := NewBroker()
	a := b.Subscribe()
	c := b.Subscribe()
	defer b.Unsubscribe(a)
	defer b.Unsubscribe(c)

	go b.Publish(Event{Kind: "x"})
	go b.Publish(Event{Kind: "x"})

	got := 0
	deadline := time.After(200 * time.Millisecond)
	for got < 2 {
		select {
		case <-a:
			got++
		case <-c:
			got++
		case <-deadline:
			t.Fatalf("only received %d events on each subscriber", got)
		}
	}
}

func TestBroker_UnsubscribedChannelClosed(t *testing.T) {
	b := NewBroker()
	sub := b.Subscribe()
	b.Unsubscribe(sub)
	if _, ok := <-sub; ok {
		t.Fatal("channel should be closed after Unsubscribe")
	}
}

func TestBroker_DropsOnSlowSubscriber(t *testing.T) {
	b := NewBroker()
	// Don't drain the subscriber — 20 publishes should not block.
	_ = b.Subscribe()

	done := make(chan struct{})
	go func() {
		for i := 0; i < 20; i++ {
			b.Publish(Event{Kind: "flood"})
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("Publish blocked on slow subscriber")
	}
}
