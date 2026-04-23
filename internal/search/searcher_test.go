package search

import (
	"context"
	"testing"
	"time"

	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/indexer"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/verify"
)

// stubIndexer returns a canned set of releases; tracks queries it was asked.
type stubIndexer struct {
	name     string
	releases []indexer.Release
	queries  []string
}

func (s *stubIndexer) Name() string { return s.name }

func (s *stubIndexer) Search(ctx context.Context, q indexer.Query) ([]indexer.Release, error) {
	s.queries = append(s.queries, q.Title)
	return append([]indexer.Release(nil), s.releases...), nil
}

func newSearcherEnv(t *testing.T) (*Searcher, *queue.Queue, title.Repo, *Repo, *stubIndexer) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	titles := title.NewRepo(d)
	q := queue.New(d)
	repo := NewRepo(d)

	idx := &stubIndexer{
		name: "stub",
		releases: []indexer.Release{
			{InfoHash: "h1", Title: "Movie.2020.Criterion.Commentary", Indexer: "stub", Seeders: 20},
			{InfoHash: "h2", Title: "Movie.2020.WEBRip", Indexer: "stub", Seeders: 5},
		},
	}
	v := verify.NewVerifier(verify.DefaultRules(), 8)
	s := NewSearcher([]indexer.Indexer{idx}, v, repo, q, titles, 100)
	return s, q, titles, repo, idx
}

func TestSearcher_SearchDue_InsertsCandidates(t *testing.T) {
	s, q, titles, repo, _ := newSearcherEnv(t)
	ctx := context.Background()

	if err := titles.Insert(ctx, title.Title{ID: "t:1", Kind: title.KindMovie, DisplayName: "Movie", Year: 2020, FilePath: "/m.mkv"}); err != nil {
		t.Fatal(err)
	}
	if err := q.MarkWanted(ctx, "t:1"); err != nil {
		t.Fatal(err)
	}

	n, err := s.SearchDue(ctx, time.Now())
	if err != nil {
		t.Fatalf("SearchDue: %v", err)
	}
	if n != 1 {
		t.Fatalf("expected 1 title processed, got %d", n)
	}

	cands, err := repo.ListCandidates(ctx, "t:1")
	if err != nil {
		t.Fatal(err)
	}
	if len(cands) != 2 {
		t.Fatalf("expected 2 candidates, got %d: %+v", len(cands), cands)
	}
	if !cands[0].LikelyCommentary || cands[0].Release.InfoHash != "h1" {
		t.Fatalf("top candidate wrong: %+v", cands[0])
	}
}

func TestSearcher_SearchDue_AdvancesNextSearchAt(t *testing.T) {
	s, q, titles, _, _ := newSearcherEnv(t)
	ctx := context.Background()
	_ = titles.Insert(ctx, title.Title{ID: "t:2", Kind: title.KindMovie, DisplayName: "X", FilePath: "/x"})
	_ = q.MarkWanted(ctx, "t:2")

	now := time.Now().UTC().Truncate(time.Second)
	if _, err := s.SearchDue(ctx, now); err != nil {
		t.Fatal(err)
	}
	entry, err := q.Get(ctx, "t:2")
	if err != nil {
		t.Fatal(err)
	}
	if entry.NextSearchAt.Before(now.Add(time.Hour)) {
		t.Fatalf("next_search_at should be well in the future, got %v (now=%v)", entry.NextSearchAt, now)
	}
}

func TestSearcher_SearchDue_SkipsTitlesNotDue(t *testing.T) {
	s, q, titles, _, _ := newSearcherEnv(t)
	ctx := context.Background()
	_ = titles.Insert(ctx, title.Title{ID: "t:3", Kind: title.KindMovie, DisplayName: "X", FilePath: "/x"})
	_ = q.MarkWanted(ctx, "t:3")
	_ = q.UpdateNextSearchAt(ctx, "t:3", time.Now().Add(24*time.Hour))

	n, err := s.SearchDue(ctx, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 processed (none due), got %d", n)
	}
}

func TestSearcher_FansOutQueries(t *testing.T) {
	s, q, titles, _, idx := newSearcherEnv(t)
	ctx := context.Background()
	_ = titles.Insert(ctx, title.Title{ID: "t:4", Kind: title.KindMovie, DisplayName: "Movie", Year: 2020, FilePath: "/m"})
	_ = q.MarkWanted(ctx, "t:4")

	if _, err := s.SearchDue(ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	if len(idx.queries) != 5 {
		t.Fatalf("expected 5 query variants issued, got %d: %v", len(idx.queries), idx.queries)
	}
}
