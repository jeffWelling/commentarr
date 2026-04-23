package title

import (
	"context"
	"testing"

	"github.com/jeffWelling/commentarr/internal/db"
)

func newTestDB(t *testing.T) *titleRepo {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })

	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return NewRepo(d)
}

func TestRepo_InsertAndFindByID(t *testing.T) {
	r := newTestDB(t)
	tt := Title{
		ID:          "plex:12345",
		Kind:        KindMovie,
		DisplayName: "Example Movie",
		Year:        2020,
		TMDBID:      "mt-111",
		FilePath:    "/media/movies/example.mkv",
	}
	if err := r.Insert(context.Background(), tt); err != nil {
		t.Fatalf("Insert: %v", err)
	}

	got, err := r.FindByID(context.Background(), "plex:12345")
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if got.DisplayName != "Example Movie" || got.Year != 2020 {
		t.Fatalf("unexpected: %+v", got)
	}
	if got.TMDBID != "mt-111" {
		t.Fatalf("expected tmdb_id=mt-111 got %q", got.TMDBID)
	}
}

func TestRepo_FindByID_NotFound(t *testing.T) {
	r := newTestDB(t)
	_, err := r.FindByID(context.Background(), "nope")
	if err == nil {
		t.Fatal("expected error for missing row")
	}
}

func TestRepo_InsertEpisodeWithSeriesID(t *testing.T) {
	r := newTestDB(t)
	tt := Title{
		ID:          "jf:ep:42",
		Kind:        KindEpisode,
		DisplayName: "Show - S01E01",
		SeriesID:    "jf:series:s1",
		Season:      1,
		Episode:     1,
		FilePath:    "/media/tv/show/s01e01.mkv",
	}
	if err := r.Insert(context.Background(), tt); err != nil {
		t.Fatalf("Insert: %v", err)
	}
	got, err := r.FindByID(context.Background(), "jf:ep:42")
	if err != nil {
		t.Fatal(err)
	}
	if got.Kind != KindEpisode || got.SeriesID != "jf:series:s1" || got.Season != 1 || got.Episode != 1 {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestRepo_List_OrderedByDisplayName(t *testing.T) {
	r := newTestDB(t)
	ctx := context.Background()
	for _, name := range []string{"Zebra", "Alpha", "Mike"} {
		if err := r.Insert(ctx, Title{ID: name, Kind: KindMovie, DisplayName: name, FilePath: "/" + name}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := r.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || got[0].DisplayName != "Alpha" || got[2].DisplayName != "Zebra" {
		t.Fatalf("expected alpha/mike/zebra got %v", got)
	}
}
