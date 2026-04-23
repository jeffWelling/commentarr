package indexer

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestRateLimiter_AllowsBurst(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 60, Burst: 3})
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	for i := 0; i < 3; i++ {
		start := time.Now()
		if err := rl.Wait(ctx); err != nil {
			t.Fatalf("burst call %d blocked: %v", i, err)
		}
		if elapsed := time.Since(start); elapsed > 10*time.Millisecond {
			t.Fatalf("burst call %d took %v (expected instant)", i, elapsed)
		}
	}
}

func TestRateLimiter_Throttles(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{RequestsPerMinute: 600, Burst: 1})
	ctx := context.Background()
	if err := rl.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	if err := rl.Wait(ctx); err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if elapsed < 50*time.Millisecond {
		t.Fatalf("expected throttle ~100ms, got %v", elapsed)
	}
}

func TestRateLimiter_Unlimited(t *testing.T) {
	rl := NewRateLimiter(RateLimitConfig{}) // zero config = unlimited
	ctx := context.Background()
	start := time.Now()
	for i := 0; i < 10; i++ {
		if err := rl.Wait(ctx); err != nil {
			t.Fatal(err)
		}
	}
	if elapsed := time.Since(start); elapsed > 50*time.Millisecond {
		t.Fatalf("unlimited limiter should be instant, got %v", elapsed)
	}
}

func TestCircuitBreaker_TripsAfterConsecutiveFailures(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		ConsecutiveFailureThreshold: 3,
		OpenDuration:                50 * time.Millisecond,
	})

	boom := errors.New("boom")
	for i := 0; i < 3; i++ {
		_ = cb.Do(func() error { return boom })
	}

	called := false
	err := cb.Do(func() error { called = true; return nil })
	if called {
		t.Fatal("circuit should be open; function must not run")
	}
	if !errors.Is(err, ErrCircuitOpen) {
		t.Fatalf("expected ErrCircuitOpen, got %v", err)
	}

	time.Sleep(75 * time.Millisecond)
	err = cb.Do(func() error { return nil })
	if err != nil {
		t.Fatalf("half-open success call failed: %v", err)
	}
	err = cb.Do(func() error { return nil })
	if err != nil {
		t.Fatalf("closed-circuit call failed: %v", err)
	}
}

func TestCircuitBreaker_ResetsOnSuccess(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{
		ConsecutiveFailureThreshold: 3,
		OpenDuration:                time.Second,
	})
	boom := errors.New("boom")
	_ = cb.Do(func() error { return boom })
	_ = cb.Do(func() error { return boom })
	_ = cb.Do(func() error { return nil })

	_ = cb.Do(func() error { return boom })
	_ = cb.Do(func() error { return boom })
	called := false
	_ = cb.Do(func() error { called = true; return boom })
	if !called {
		t.Fatal("circuit tripped too early — counter did not reset on prior success")
	}
}

func TestCircuitBreaker_StateTransitions(t *testing.T) {
	cb := NewCircuitBreaker(CircuitBreakerConfig{ConsecutiveFailureThreshold: 2, OpenDuration: 20 * time.Millisecond})
	if cb.State() != 0 {
		t.Fatalf("expected closed (0), got %d", cb.State())
	}
	boom := errors.New("x")
	_ = cb.Do(func() error { return boom })
	_ = cb.Do(func() error { return boom })
	if cb.State() != 1 {
		t.Fatalf("expected open (1), got %d", cb.State())
	}
	time.Sleep(30 * time.Millisecond)
	// state transitions on next Do call, not on clock alone
	_ = cb.Do(func() error { return nil })
	if cb.State() != 0 {
		t.Fatalf("expected closed (0) after half-open success, got %d", cb.State())
	}
}
