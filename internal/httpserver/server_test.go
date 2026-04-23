package httpserver

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNew_ServesHealthz(t *testing.T) {
	s := New(Config{})
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Fatalf("expected 'ok', got %q", w.Body.String())
	}
}

func TestNew_ServesReadyz(t *testing.T) {
	s := New(Config{})
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestNew_ServesMetrics(t *testing.T) {
	s := New(Config{})
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "go_") {
		t.Fatalf("expected Prometheus metrics text, got %q", truncate(w.Body.String(), 200))
	}
}

func TestRequestIDMiddleware_SetsHeader(t *testing.T) {
	s := New(Config{})
	s.Mount("/api/ping", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/ping", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if rid := w.Header().Get("X-Request-Id"); rid == "" {
		t.Fatal("request-id header should be set on every response")
	}
}

func TestPanicMiddleware_Recovers(t *testing.T) {
	s := New(Config{})
	s.Router().Get("/boom", func(w http.ResponseWriter, r *http.Request) {
		panic("surprise")
	})
	req := httptest.NewRequest(http.MethodGet, "/boom", nil)
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 after panic, got %d", w.Code)
	}
}

func TestStart_Shutdown(t *testing.T) {
	s := New(Config{Addr: "127.0.0.1:0"})
	errCh := make(chan error, 1)
	go func() { errCh <- s.Start() }()
	if err := s.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := <-errCh; err != nil && err != http.ErrServerClosed {
		// ListenAndServe on port 0 can give a transient error on shutdown
		// race; tolerate common cases.
		if !strings.Contains(err.Error(), "Server closed") {
			t.Fatalf("unexpected Start() error: %v", err)
		}
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
