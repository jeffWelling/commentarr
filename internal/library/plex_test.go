package library

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jeffWelling/commentarr/internal/title"
)

const plexSectionsXML = `<?xml version="1.0"?>
<MediaContainer>
  <Directory key="1" type="movie" />
  <Directory key="2" type="show" />
  <Directory key="3" type="music" />
</MediaContainer>`

const plexMoviesXML = `<?xml version="1.0"?>
<MediaContainer>
  <Video ratingKey="100" title="The Thing" year="1982" type="movie" guid="plex://movie/abc">
    <Media><Part file="/media/movies/The Thing (1982).mkv" /></Media>
  </Video>
  <Video ratingKey="101" title="Inception" year="2010" type="movie">
    <Media><Part file="/media/movies/Inception (2010).mkv" /></Media>
  </Video>
</MediaContainer>`

const plexShowsXML = `<?xml version="1.0"?>
<MediaContainer>
  <Video ratingKey="200" title="Pilot" grandparentTitle="Breaking Bad" parentIndex="1" index="1" type="episode">
    <Media><Part file="/media/tv/bb/s01e01.mkv" /></Media>
  </Video>
</MediaContainer>`

func newPlexServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/library/sections", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Plex-Token") != "tok" {
			http.Error(w, "unauth", http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(plexSectionsXML))
	})
	mux.HandleFunc("/library/sections/1/all", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(plexMoviesXML))
	})
	mux.HandleFunc("/library/sections/2/all", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(plexShowsXML))
	})
	mux.HandleFunc("/library/sections/1/refresh", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/library/sections/2/refresh", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	return httptest.NewServer(mux)
}

func TestPlex_ListMoviesAndEpisodes(t *testing.T) {
	srv := newPlexServer(t)
	defer srv.Close()
	src := NewPlexSource(PlexConfig{BaseURL: srv.URL, Token: "tok", Name: "plex-test"})
	titles, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(titles) != 3 {
		t.Fatalf("expected 3 titles (2 movies + 1 episode), got %d: %+v", len(titles), titles)
	}

	byName := map[string]title.Title{}
	for _, tt := range titles {
		byName[tt.DisplayName] = tt
	}
	thing, ok := byName["The Thing"]
	if !ok {
		t.Fatalf("The Thing not found: %+v", byName)
	}
	if thing.Kind != title.KindMovie || thing.Year != 1982 || !strings.Contains(thing.FilePath, "The Thing") {
		t.Fatalf("The Thing wrong: %+v", thing)
	}

	ep, ok := byName["Breaking Bad - S01E01"]
	if !ok {
		t.Fatalf("Breaking Bad episode not found: %+v", byName)
	}
	if ep.Kind != title.KindEpisode || ep.Season != 1 || ep.Episode != 1 {
		t.Fatalf("episode wrong: %+v", ep)
	}
	if ep.SeriesID != "plex-series:Breaking Bad" {
		t.Fatalf("SeriesID: %q", ep.SeriesID)
	}
}

func TestPlex_AuthFailure(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/library/sections", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauth", http.StatusUnauthorized)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	src := NewPlexSource(PlexConfig{BaseURL: srv.URL, Token: "bad", Name: "plex-test"})
	_, err := src.List(context.Background())
	if err == nil {
		t.Fatal("expected auth error")
	}
}

func TestPlex_SkipsMusicSections(t *testing.T) {
	srv := newPlexServer(t)
	defer srv.Close()
	src := NewPlexSource(PlexConfig{BaseURL: srv.URL, Token: "tok", Name: "plex-test"})
	titles, err := src.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	// If we had tried to fetch section 3 (music), we'd have gotten a 404
	// since the handler isn't defined. The fact that we got a clean
	// result means non-movie/show sections were skipped.
	if len(titles) == 0 {
		t.Fatal("expected some titles")
	}
}
