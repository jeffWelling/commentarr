package library

import (
	"context"
	"os"
	"testing"

	"github.com/jeffWelling/commentarr/internal/title"
)

func TestFilesystem_ListWalksRoot(t *testing.T) {
	s := NewFilesystemSource("fs-test", "../../testdata/library")
	got, err := s.List(context.Background())
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 titles, got %d: %+v", len(got), got)
	}

	byName := map[string]title.Title{}
	for _, tt := range got {
		byName[tt.DisplayName] = tt
	}
	m, ok := byName["Movie"]
	if !ok {
		t.Fatalf("Movie not found in list: %+v", byName)
	}
	if m.Kind != title.KindMovie || m.Year != 2020 {
		t.Fatalf("Movie: unexpected %+v", m)
	}

	ep, ok := byName["Series - S01E01"]
	if !ok {
		t.Fatalf("Series episode not found: %+v", byName)
	}
	if ep.Kind != title.KindEpisode || ep.Season != 1 || ep.Episode != 1 {
		t.Fatalf("Episode: unexpected %+v", ep)
	}
	if ep.SeriesID != "fs-series:Series" {
		t.Fatalf("Episode SeriesID: unexpected %q", ep.SeriesID)
	}
}

func TestFilesystem_IgnoresNonVideo(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(tmp+"/README.txt", []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewFilesystemSource("fs-test", tmp)
	got, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("expected 0 titles, got %d", len(got))
	}
}

func TestParsePath_MovieFromFilename(t *testing.T) {
	tt := parsePath("/root", "/root/Inception (2010).mkv")
	if tt.Kind != title.KindMovie || tt.DisplayName != "Inception" || tt.Year != 2010 {
		t.Fatalf("unexpected: %+v", tt)
	}
}

func TestParsePath_MovieFromParentFolder(t *testing.T) {
	tt := parsePath("/root", "/root/The Matrix (1999)/movie.mkv")
	if tt.Kind != title.KindMovie || tt.DisplayName != "The Matrix" || tt.Year != 1999 {
		t.Fatalf("unexpected: %+v", tt)
	}
}

func TestParsePath_EpisodeSxxEyy(t *testing.T) {
	tt := parsePath("/root", "/root/Breaking Bad/Season 02/Breaking.Bad.S02E05.720p.mkv")
	if tt.Kind != title.KindEpisode || tt.Season != 2 || tt.Episode != 5 {
		t.Fatalf("unexpected: %+v", tt)
	}
	if tt.SeriesID != "fs-series:Breaking Bad" {
		t.Fatalf("SeriesID: %q", tt.SeriesID)
	}
}
