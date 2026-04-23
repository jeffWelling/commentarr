package search

import (
	"context"
	"testing"

	"github.com/jeffWelling/commentarr/internal/db"
	"github.com/jeffWelling/commentarr/internal/indexer"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/verify"
)

func newTestRepo(t *testing.T) (*Repo, title.Repo) {
	t.Helper()
	d, err := db.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Close() })
	if err := db.Migrate(d, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	return NewRepo(d), title.NewRepo(d)
}

func TestRepo_SaveCandidatesAndListByTitle(t *testing.T) {
	r, titles := newTestRepo(t)
	ctx := context.Background()
	if err := titles.Insert(ctx, title.Title{ID: "t:1", Kind: title.KindMovie, DisplayName: "M", FilePath: "/m"}); err != nil {
		t.Fatal(err)
	}

	scored := []verify.Scored{
		{
			Release: indexer.Release{
				InfoHash: "abc", Title: "Movie Criterion Commentary", SizeBytes: 10 << 30,
				Seeders: 20, Indexer: "prowlarr-test", Protocol: "torrent",
			},
			Score: 20, LikelyCommentary: true,
			Reasons: []verify.Reason{{Rule: "criterion", Score: 10}, {Rule: "commentary", Score: 10}},
		},
		{
			Release: indexer.Release{
				InfoHash: "def", Title: "Movie Webrip", Indexer: "prowlarr-test",
			},
			Score: -5, LikelyCommentary: false,
			Reasons: []verify.Reason{{Rule: "webrip_penalty", Score: -5}},
		},
	}
	if err := r.SaveCandidates(ctx, "t:1", scored); err != nil {
		t.Fatalf("SaveCandidates: %v", err)
	}

	got, err := r.ListCandidates(ctx, "t:1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 candidates, got %d", len(got))
	}
	if got[0].Score != 20 || got[1].Score != -5 {
		t.Fatalf("expected sorted desc by score, got %+v", got)
	}
	if !got[0].LikelyCommentary {
		t.Fatalf("first candidate should be LikelyCommentary: %+v", got[0])
	}
	if len(got[0].Reasons) != 2 {
		t.Fatalf("reasons should round-trip; got %+v", got[0].Reasons)
	}
}

func TestRepo_SaveCandidates_IdempotentUpsert(t *testing.T) {
	r, titles := newTestRepo(t)
	ctx := context.Background()
	_ = titles.Insert(ctx, title.Title{ID: "t:1", Kind: title.KindMovie, DisplayName: "M", FilePath: "/m"})
	scored := []verify.Scored{
		{
			Release: indexer.Release{InfoHash: "abc", Title: "t1", Indexer: "i"},
			Score:   5,
		},
	}
	if err := r.SaveCandidates(ctx, "t:1", scored); err != nil {
		t.Fatal(err)
	}
	scored[0].Score = 20
	if err := r.SaveCandidates(ctx, "t:1", scored); err != nil {
		t.Fatal(err)
	}
	got, _ := r.ListCandidates(ctx, "t:1")
	if got[0].Score != 20 {
		t.Fatalf("expected upsert to 20, got %d", got[0].Score)
	}
}
