package daemon

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestDaemon_RunsTickAtInterval(t *testing.T) {
	var count int32
	d := New(Config{
		Ticks: []Tick{
			{Interval: 10 * time.Millisecond, Fn: func(ctx context.Context) {
				atomic.AddInt32(&count, 1)
			}},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Millisecond)
	defer cancel()
	d.Run(ctx)
	if got := atomic.LoadInt32(&count); got < 3 {
		t.Fatalf("expected ≥3 ticks in 45ms at 10ms interval, got %d", got)
	}
}

func TestDaemon_StopsOnContextCancel(t *testing.T) {
	d := New(Config{
		Ticks: []Tick{
			{Interval: 5 * time.Millisecond, Fn: func(ctx context.Context) {}},
		},
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		d.Run(ctx)
		close(done)
	}()
	time.Sleep(15 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("daemon did not return after ctx cancel")
	}
}

func TestDaemon_MultipleTicksInterleave(t *testing.T) {
	var a, b int32
	d := New(Config{
		Ticks: []Tick{
			{Name: "a", Interval: 7 * time.Millisecond, Fn: func(ctx context.Context) {
				atomic.AddInt32(&a, 1)
			}},
			{Name: "b", Interval: 13 * time.Millisecond, Fn: func(ctx context.Context) {
				atomic.AddInt32(&b, 1)
			}},
		},
	})
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Millisecond)
	defer cancel()
	d.Run(ctx)
	if atomic.LoadInt32(&a) == 0 || atomic.LoadInt32(&b) == 0 {
		t.Fatalf("both ticks should have fired: a=%d b=%d", a, b)
	}
}
