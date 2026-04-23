package indexer

import (
	"context"
	"errors"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimitConfig configures a token-bucket rate limiter fronting an
// indexer. Prowlarr itself throttles per-indexer; this is our polite
// client-side layer on top.
type RateLimitConfig struct {
	RequestsPerMinute int
	Burst             int
}

// RateLimiter wraps a token bucket. Wait blocks until a token is
// available or ctx is cancelled.
type RateLimiter struct {
	bucket *rate.Limiter
}

// NewRateLimiter returns a configured limiter. A zero RequestsPerMinute
// yields an unlimited limiter (useful for tests that don't care about
// rates).
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	if cfg.RequestsPerMinute <= 0 {
		return &RateLimiter{bucket: rate.NewLimiter(rate.Inf, 1)}
	}
	burst := cfg.Burst
	if burst <= 0 {
		burst = 1
	}
	perSec := rate.Limit(float64(cfg.RequestsPerMinute) / 60.0)
	return &RateLimiter{bucket: rate.NewLimiter(perSec, burst)}
}

// Wait blocks until a token is available.
func (r *RateLimiter) Wait(ctx context.Context) error {
	return r.bucket.Wait(ctx)
}

// CircuitBreakerConfig controls circuit-breaker behaviour. When
// ConsecutiveFailureThreshold failures occur back-to-back, subsequent
// calls short-circuit for OpenDuration, then transition to half-open
// and retry once. A half-open success closes the circuit.
type CircuitBreakerConfig struct {
	ConsecutiveFailureThreshold int
	OpenDuration                time.Duration
}

// ErrCircuitOpen is returned by CircuitBreaker.Do when the circuit is
// tripped and the call was short-circuited.
var ErrCircuitOpen = errors.New("circuit open")

type circuitState int

const (
	circuitClosed circuitState = iota
	circuitOpen
	circuitHalfOpen
)

// CircuitBreaker implements the minimal breaker semantics we need.
type CircuitBreaker struct {
	cfg CircuitBreakerConfig

	mu          sync.Mutex
	state       circuitState
	consecutive int
	openedAt    time.Time
}

// NewCircuitBreaker returns a breaker in the closed state.
func NewCircuitBreaker(cfg CircuitBreakerConfig) *CircuitBreaker {
	if cfg.ConsecutiveFailureThreshold <= 0 {
		cfg.ConsecutiveFailureThreshold = 5
	}
	if cfg.OpenDuration <= 0 {
		cfg.OpenDuration = 1 * time.Hour
	}
	return &CircuitBreaker{cfg: cfg, state: circuitClosed}
}

// State reports the current circuit state (0=closed, 1=open, 2=half-open).
func (c *CircuitBreaker) State() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return int(c.state)
}

// Do runs fn unless the circuit is open. Returns ErrCircuitOpen without
// calling fn when the circuit is tripped.
func (c *CircuitBreaker) Do(fn func() error) error {
	c.mu.Lock()
	if c.state == circuitOpen && time.Since(c.openedAt) >= c.cfg.OpenDuration {
		c.state = circuitHalfOpen
	}
	if c.state == circuitOpen {
		c.mu.Unlock()
		return ErrCircuitOpen
	}
	c.mu.Unlock()

	err := fn()

	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		if c.state == circuitHalfOpen {
			c.state = circuitOpen
			c.openedAt = time.Now()
			return err
		}
		c.consecutive++
		if c.consecutive >= c.cfg.ConsecutiveFailureThreshold {
			c.state = circuitOpen
			c.openedAt = time.Now()
		}
		return err
	}
	c.consecutive = 0
	if c.state == circuitHalfOpen {
		c.state = circuitClosed
	}
	return nil
}
