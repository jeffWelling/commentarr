package library

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jeffWelling/commentarr/internal/title"
)

func newJellyfinServer(t *testing.T, wantAuthHeader string) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/Users/u1/Items", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get(wantAuthHeader) != "apikey" {
			http.Error(w, "unauth", http.StatusUnauthorized)
			return
		}
		resp := jellyfinItemsResp{
			Items: []jellyfinItem{
				{
					Id: "m1", Name: "The Thing", Type: "Movie", ProductionYear: 1982,
					Path: "/media/movies/The Thing.mkv",
				},
				{
					Id: "e1", Name: "Pilot", Type: "Episode",
					SeriesName: "Breaking Bad", SeriesId: "s1",
					ParentIndexNumber: 1, IndexNumber: 1,
					Path: "/media/tv/bb/s01e01.mkv", ProductionYear: 2008,
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	return httptest.NewServer(mux)
}

func TestJellyfin_ListMoviesAndEpisodes(t *testing.T) {
	srv := newJellyfinServer(t, "X-MediaBrowser-Token")
	defer srv.Close()
	src := NewJellyfinSource(JellyfinConfig{
		BaseURL: srv.URL, APIKey: "apikey", UserID: "u1", Name: "jf-test",
	})
	titles, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 2 {
		t.Fatalf("expected 2, got %d: %+v", len(titles), titles)
	}
	byKind := map[title.Kind]title.Title{}
	for _, tt := range titles {
		byKind[tt.Kind] = tt
	}
	if byKind[title.KindMovie].DisplayName != "The Thing" {
		t.Fatalf("movie wrong: %+v", byKind[title.KindMovie])
	}
	ep := byKind[title.KindEpisode]
	if ep.DisplayName != "Breaking Bad - S01E01" || ep.Season != 1 || ep.Episode != 1 {
		t.Fatalf("episode wrong: %+v", ep)
	}
}

func TestJellyfin_EmbyModeUsesDifferentHeader(t *testing.T) {
	srv := newJellyfinServer(t, "X-Emby-Token")
	defer srv.Close()
	src := NewJellyfinSource(JellyfinConfig{
		BaseURL: srv.URL, APIKey: "apikey", UserID: "u1", Name: "emby-test",
		EmbyMode: true,
	})
	titles, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 2 {
		t.Fatalf("expected 2, got %d", len(titles))
	}
}
