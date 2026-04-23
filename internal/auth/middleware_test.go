package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAuthed(t *testing.T) (*Repo, string) {
	t.Helper()
	r := newRepo(t)
	ctx := context.Background()
	hash, _ := HashPassword("pw")
	_ = r.SaveAdmin(ctx, "admin", hash)
	key, _ := r.GenerateAPIKey(ctx, "seed")
	return r, key
}

func TestMiddleware_RejectsAnonymous(t *testing.T) {
	r, _ := newAuthed(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := NewMiddleware(r, MiddlewareConfig{})(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestMiddleware_AcceptsAPIKeyHeader(t *testing.T) {
	r, key := newAuthed(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := NewMiddleware(r, MiddlewareConfig{})(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	req.Header.Set("X-Api-Key", key)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMiddleware_AcceptsAPIKeyQuery(t *testing.T) {
	r, key := newAuthed(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := NewMiddleware(r, MiddlewareConfig{})(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything?apikey="+key, nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMiddleware_BypassesLocalCIDR(t *testing.T) {
	r, _ := newAuthed(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := NewMiddleware(r, MiddlewareConfig{LocalBypassCIDRs: []string{"127.0.0.0/8"}})(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	req.RemoteAddr = "127.0.0.1:12345"
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("127.0.0.1 should be allowed with local bypass, got %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	req2.RemoteAddr = "8.8.8.8:12345"
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	if w2.Code != http.StatusUnauthorized {
		t.Fatalf("non-local addr should be 401, got %d", w2.Code)
	}
}

func TestMiddleware_SkipsHealthEndpoints(t *testing.T) {
	r, _ := newAuthed(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := NewMiddleware(r, MiddlewareConfig{})(next)

	for _, path := range []string{"/healthz", "/readyz", "/metrics"} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code == http.StatusUnauthorized {
			t.Errorf("%s must skip auth, got 401", path)
		}
	}
}

func TestMiddleware_ReturnsWWWAuthenticate(t *testing.T) {
	r, _ := newAuthed(t)
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	h := NewMiddleware(r, MiddlewareConfig{})(next)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/anything", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if !strings.Contains(w.Header().Get("WWW-Authenticate"), "X-Api-Key") {
		t.Fatalf("expected WWW-Authenticate hint in 401 response, got %q", w.Header().Get("WWW-Authenticate"))
	}
}
