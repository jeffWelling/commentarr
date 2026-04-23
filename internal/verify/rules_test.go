package verify

import "testing"

func TestDefaultRules_HighScoreForCriterionCommentary(t *testing.T) {
	rules := DefaultRules()
	got := ScoreTitle("The.Thing.1982.Criterion.Commentary.1080p.BluRay-GROUP", 1<<30, rules)
	if got.Score < 15 {
		t.Fatalf("Criterion+commentary combo should score high, got %d: %+v", got.Score, got.Reasons)
	}
}

func TestDefaultRules_NegativeForWebRip(t *testing.T) {
	rules := DefaultRules()
	got := ScoreTitle("Movie.2020.WEBRip.x264", 800_000_000, rules)
	if got.Score >= 8 {
		t.Fatalf("unenhanced webrip should score below threshold, got %d: %+v", got.Score, got.Reasons)
	}
}

func TestScoreTitle_IncludesReasonsPerMatch(t *testing.T) {
	rules := DefaultRules()
	got := ScoreTitle("Movie.2020.Criterion.Collection.1080p", 10<<30, rules)
	seenCriterion := false
	for _, r := range got.Reasons {
		if r.Rule == "criterion" {
			seenCriterion = true
		}
	}
	if !seenCriterion {
		t.Fatalf("expected criterion reason in %+v", got.Reasons)
	}
}

func TestDefaultRules_CamPenalty(t *testing.T) {
	rules := DefaultRules()
	got := ScoreTitle("Some.Movie.2026.HDCAM.x264", 1<<30, rules)
	if got.Score >= 0 {
		t.Fatalf("cam release should score negative, got %d: %+v", got.Score, got.Reasons)
	}
}
