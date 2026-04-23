// Package httpserver wires the chi router, observability middleware,
// health + metrics endpoints, and graceful shutdown. API handler
// packages (internal/api/v1/*) register themselves via Server.Mount.
package httpserver

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

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

	r.Get("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok"))
	})
	r.Get("/readyz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ready"))
	})
	r.Handle("/metrics", promhttp.Handler())

	return &Server{
		cfg:    cfg,
		router: r,
		httpS: &http.Server{
			Addr:         cfg.Addr,
			Handler:      r,
			ReadTimeout:  cfg.ReadTimeout,
			WriteTimeout: cfg.WriteTimeout,
		},
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
