package verify

import (
	"testing"

	"github.com/jeffWelling/commentarr/internal/indexer"
)

func TestVerifier_ScoreReleases(t *testing.T) {
	v := NewVerifier(DefaultRules(), 8)
	in := []indexer.Release{
		{InfoHash: "a", Title: "Movie.2020.Criterion.Collection", SizeBytes: 20 << 30},
		{InfoHash: "b", Title: "Movie.2020.WEBRip.x264", SizeBytes: 800_000_000},
		{InfoHash: "c", Title: "Movie.2020.Commentary.Director.Edition", SizeBytes: 12 << 30},
	}
	got := v.Score(in)
	if len(got) != 3 {
		t.Fatalf("expected 3 scored releases, got %d", len(got))
	}
	for _, s := range got {
		if s.Release.InfoHash == "a" || s.Release.InfoHash == "c" {
			if !s.LikelyCommentary {
				t.Errorf("%s should be LikelyCommentary: %+v", s.Release.InfoHash, s)
			}
		}
		if s.Release.InfoHash == "b" {
			if s.LikelyCommentary {
				t.Errorf("plain webrip should not be LikelyCommentary: %+v", s)
			}
		}
	}
}

func TestVerifier_SortedByScoreDesc(t *testing.T) {
	v := NewVerifier(DefaultRules(), 8)
	in := []indexer.Release{
		{InfoHash: "low", Title: "Movie.2020.x264"},
		{InfoHash: "high", Title: "Movie.2020.Criterion.Commentary.Remastered"},
		{InfoHash: "mid", Title: "Movie.2020.Special.Edition"},
	}
	got := v.Score(in)
	if got[0].Release.InfoHash != "high" || got[2].Release.InfoHash != "low" {
		t.Fatalf("not sorted by score desc: %+v", got)
	}
}
