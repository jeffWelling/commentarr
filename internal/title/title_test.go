package title

import (
	"context"
	"testing"
	"time"

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

// TestRepo_FindByID_NullExternalIDs guards against the SQL NULL scan
// panic that surfaced during a real-world dry-run when a title was
// seeded directly via sqlite3 (without going through Insert) and
// tmdb_id / imdb_id / series_id were genuine NULLs. FindByID was
// scanning into plain *string and exploding with "converting NULL to
// string is unsupported." Insert now writes empty strings as NULL via
// nullableString, and the read path uses sql.NullString for all three.
func TestRepo_FindByID_NullExternalIDs(t *testing.T) {
	r := newTestDB(t)
	// Direct sqlite-level seed: emulate a title row created outside Insert.
	if _, err := r.db.ExecContext(context.Background(), `
		INSERT INTO titles (id, kind, display_name, year, file_path)
		VALUES ('seed:1', 'movie', 'Direct Seed', 2020, '/x.mkv')`); err != nil {
		t.Fatal(err)
	}
	got, err := r.FindByID(context.Background(), "seed:1")
	if err != nil {
		t.Fatalf("FindByID with NULL external ids: %v", err)
	}
	if got.DisplayName != "Direct Seed" || got.TMDBID != "" || got.IMDBID != "" || got.SeriesID != "" {
		t.Fatalf("unexpected: %+v", got)
	}
}

// TestRepo_List_NullExternalIDs is the List-side counterpart of the
// above. The same scan bug existed on List too.
func TestRepo_List_NullExternalIDs(t *testing.T) {
	r := newTestDB(t)
	if _, err := r.db.ExecContext(context.Background(), `
		INSERT INTO titles (id, kind, display_name, year, file_path)
		VALUES ('seed:a', 'movie', 'A', 2020, '/a.mkv'),
		       ('seed:b', 'movie', 'B', 2021, '/b.mkv')`); err != nil {
		t.Fatal(err)
	}
	got, err := r.List(context.Background())
	if err != nil {
		t.Fatalf("List with NULL external ids: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 rows, got %d", len(got))
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

func TestRepo_SaveAndGetVerdict(t *testing.T) {
	r := newTestDB(t)
	ctx := context.Background()
	if err := r.Insert(ctx, Title{ID: "tst:1", Kind: KindMovie, DisplayName: "X", FilePath: "/x.mkv"}); err != nil {
		t.Fatal(err)
	}
	v := Verdict{
		TitleID:           "tst:1",
		HasCommentary:     true,
		Confidence:        0.91,
		ClassifierVersion: "v0.1.0",
		ClassifiedAt:      time.Now().UTC().Truncate(time.Second),
	}
	if err := r.SaveVerdict(ctx, v); err != nil {
		t.Fatalf("SaveVerdict: %v", err)
	}
	got, err := r.GetVerdict(ctx, "tst:1")
	if err != nil {
		t.Fatalf("GetVerdict: %v", err)
	}
	if !got.HasCommentary || got.Confidence != 0.91 || got.ClassifierVersion != "v0.1.0" {
		t.Fatalf("unexpected: %+v", got)
	}
}

func TestRepo_SaveVerdict_UpsertsOnConflict(t *testing.T) {
	r := newTestDB(t)
	ctx := context.Background()
	if err := r.Insert(ctx, Title{ID: "u:1", Kind: KindMovie, DisplayName: "U", FilePath: "/u.mkv"}); err != nil {
		t.Fatal(err)
	}
	first := Verdict{TitleID: "u:1", HasCommentary: false, Confidence: 0.5, ClassifierVersion: "v0", ClassifiedAt: time.Now().UTC()}
	second := Verdict{TitleID: "u:1", HasCommentary: true, Confidence: 0.9, ClassifierVersion: "v1", ClassifiedAt: time.Now().UTC().Add(time.Minute)}
	if err := r.SaveVerdict(ctx, first); err != nil {
		t.Fatal(err)
	}
	if err := r.SaveVerdict(ctx, second); err != nil {
		t.Fatal(err)
	}
	got, _ := r.GetVerdict(ctx, "u:1")
	if !got.HasCommentary || got.Confidence != 0.9 || got.ClassifierVersion != "v1" {
		t.Fatalf("upsert did not replace: %+v", got)
	}
}

func TestRepo_GetVerdict_NotFound(t *testing.T) {
	r := newTestDB(t)
	_, err := r.GetVerdict(context.Background(), "missing")
	if err == nil {
		t.Fatal("expected error for missing verdict")
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
