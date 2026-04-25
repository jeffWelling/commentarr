// Package httpserver wires the chi router, observability middleware,
// health + metrics endpoints, and graceful shutdown. API handler
// packages (internal/api/v1/*) register themselves via Server.Mount.
package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// ReadinessCheck reports whether one component is ready. A non-nil
// error means the check failed and the request returned 503.
type ReadinessCheck func(ctx context.Context) error

// Config configures the server.
type Config struct {
	Addr            string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Server wraps a chi router + http.Server.
type Server struct {
	cfg    Config
	router *chi.Mux
	httpS  *http.Server

	checksMu sync.RWMutex
	checks   map[string]ReadinessCheck
}

// New returns a Server with the default observability stack + built-in
// health/metrics endpoints. Mount API handlers via Server.Mount.
func New(cfg Config) *Server {
	if cfg.Addr == "" {
		cfg.Addr = ":7878"
	}
	if cfg.ReadTimeout == 0 {
		cfg.ReadTimeout = 30 * time.Second
	}
	if cfg.WriteTimeout == 0 {
		cfg.WriteTimeout = 60 * time.Second
	}
	if cfg.ShutdownTimeout == 0 {
		cfg.ShutdownTimeout = 15 * time.Second
	}

	r := chi.NewRouter()
	r.Use(requestIDMiddleware)
	r.Use(panicMiddleware)
	r.Use(loggingMiddleware)
	r.Use(metricsMiddleware)

	s := &Server{
		cfg:    cfg,
		router: r,
		checks: map[string]ReadinessCheck{},
	}
	s.httpS = &http.Server{
		Addr:         cfg.Addr,
		Handler:      r,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	}

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", s.readiness)
	r.Handle("/metrics", promhttp.Handler())

	return s
}

// RegisterReadinessCheck attaches a named check to /readyz. Checks
// run concurrently per request; a single failing check returns 503
// with a body listing the failures by name. With no checks
// registered, /readyz returns 200 "ready" (preserving the previous
// always-ready semantics for tests + bootstrap).
func (s *Server) RegisterReadinessCheck(name string, fn ReadinessCheck) {
	s.checksMu.Lock()
	defer s.checksMu.Unlock()
	s.checks[name] = fn
}

func (s *Server) readiness(w http.ResponseWriter, r *http.Request) {
	s.checksMu.RLock()
	checks := make(map[string]ReadinessCheck, len(s.checks))
	for k, v := range s.checks {
		checks[k] = v
	}
	s.checksMu.RUnlock()

	if len(checks) == 0 {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ready"))
		return
	}

	type result struct {
		name string
		err  error
	}
	out := make(chan result, len(checks))
	var wg sync.WaitGroup
	for name, fn := range checks {
		wg.Add(1)
		go func(name string, fn ReadinessCheck) {
			defer wg.Done()
			out <- result{name: name, err: fn(r.Context())}
		}(name, fn)
	}
	wg.Wait()
	close(out)

	var failures []string
	for res := range out {
		if res.err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", res.name, res.err))
		}
	}
	w.Header().Set("Content-Type", "text/plain")
	if len(failures) == 0 {
		_, _ = w.Write([]byte("ready"))
		return
	}
	sort.Strings(failures)
	w.WriteHeader(http.StatusServiceUnavailable)
	for _, line := range failures {
		_, _ = fmt.Fprintln(w, line)
	}
}

// Mount registers an API handler at the given path pattern.
func (s *Server) Mount(pattern string, h http.Handler) {
	s.router.Mount(pattern, h)
}

// Router returns the underlying chi router.
func (s *Server) Router() chi.Router { return s.router }

// Handler returns the root http.Handler (useful for tests).
func (s *Server) Handler() http.Handler { return s.router }

// Start binds the listener and serves forever.
func (s *Server) Start() error {
	if err := s.httpS.ListenAndServe(); err != nil {
		return fmt.Errorf("http serve: %w", err)
	}
	return nil
}

// Shutdown triggers graceful shutdown.
func (s *Server) Shutdown(ctx context.Context) error {
	c, cancel := context.WithTimeout(ctx, s.cfg.ShutdownTimeout)
	defer cancel()
	return s.httpS.Shutdown(c)
}
