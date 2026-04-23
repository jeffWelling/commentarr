package v1

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/safety"
	"github.com/jeffWelling/commentarr/internal/search"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/trash"
	"github.com/jeffWelling/commentarr/internal/webhook"
)


func TestLibrary_GETTitles(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := db.Migrate(d, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	repo := title.NewRepo(d)
	ctx := context.Background()
	_ = repo.Insert(ctx, title.Title{ID: "t:1", Kind: title.KindMovie, DisplayName: "A", FilePath: "/a"})
	_ = repo.Insert(ctx, title.Title{ID: "t:2", Kind: title.KindMovie, DisplayName: "B", FilePath: "/b"})

	h := NewLibraryHandler(repo)
	req := httptest.NewRequest(http.MethodGet, "/titles", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d — body: %q", w.Code, w.Body.String())
	}
	var out struct {
		Titles []title.Title `json:"titles"`
	}
	if err := json.NewDecoder(w.Body).Decode(&out); err != nil {
		t.Fatal(err)
	}
	if len(out.Titles) != 2 {
		t.Fatalf("expected 2, got %d", len(out.Titles))
	}
}

func TestWanted_GETReturnsOnlyWanted(t *testing.T) {
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if err := db.Migrate(d, "../../../migrations"); err != nil {
		t.Fatal(err)
	}
	tr := title.NewRepo(d)
	q := queue.New(d)
	ctx := context.Background()
	_ = tr.Insert(ctx, title.Title{ID: "a", Kind: title.KindMovie, DisplayName: "A", FilePath: "/a"})
	_ = tr.Insert(ctx, title.Title{ID: "b", Kind: title.KindMovie, DisplayName: "B", FilePath: "/b"})
	_ = q.MarkWanted(ctx, "a")
	_ = q.MarkResolved(ctx, "b")

	h := NewWantedHandler(q, search.NewRepo(d))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var out struct {
		Wanted []struct {
			TitleID string `json:"title_id"`
		} `json:"wanted"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if len(out.Wanted) != 1 || out.Wanted[0].TitleID != "a" {
		t.Fatalf("unexpected wanted: %+v", out.Wanted)
	}
}

func TestIndexers_GETListsStaticConfig(t *testing.T) {
	h := NewIndexerHandler([]IndexerInfo{
		{Name: "prowlarr-primary", Kind: "prowlarr", BaseURL: "http://prowlarr", Enabled: true},
	})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "prowlarr-primary") {
		t.Fatal("response missing indexer name")
	}
}

func TestTrash_ListRequiresLibrary(t *testing.T) {
	d, _ := db.Open(":memory:")
	defer d.Close()
	_ = db.Migrate(d, "../../../migrations")
	h := NewTrashHandler(trash.NewRepo(d))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 without library param, got %d", w.Code)
	}
}

func TestSafety_ValidateRule(t *testing.T) {
	d, _ := db.Open(":memory:")
	defer d.Close()
	_ = db.Migrate(d, "../../../migrations")
	h := NewSafetyHandler(safety.NewProfileRepo(d))

	body, _ := json.Marshal(map[string]string{"expression": "classifier_confidence >= 0.85"})
	req := httptest.NewRequest(http.MethodPost, "/rules/validate", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 for valid CEL, got %d", w.Code)
	}

	body, _ = json.Marshal(map[string]string{"expression": "this is not valid ==="})
	req = httptest.NewRequest(http.MethodPost, "/rules/validate", bytes.NewReader(body))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for bad CEL, got %d", w.Code)
	}
}

func TestWebhooks_SaveAndTest(t *testing.T) {
	d, _ := db.Open(":memory:")
	defer d.Close()
	_ = db.Migrate(d, "../../../migrations")

	repo := webhook.NewRepo(d)
	disp := webhook.NewDispatcher(repo, webhook.DispatcherConfig{})
	h := NewWebhooksHandler(repo, disp)

	body, _ := json.Marshal(webhook.Subscriber{
		ID: "w1", URL: "http://does-not-resolve.example.invalid/", Enabled: false, Events: []webhook.Event{webhook.EventTest},
	})
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/test", nil)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204 (dispatch is fire-and-forget; disabled subscriber is a no-op), got %d", w.Code)
	}
}

