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

func TestScoreTitle_DCAbbreviation(t *testing.T) {
	cases := []struct {
		title     string
		wantScore int // criterion (10) + DC (5) where applicable
	}{
		{"Brazil (1985) DC Criterion (1080p BluRay x265 HEVC 10bit Tigole)", 15},
		{"Brazil 1985 Criterion DC 720p BluRay", 15},
		{"Brazil.1985.DC.Criterion.1080p.BluRay", 15},
		// don't false-positive on words containing DC
		{"Brazil 1985 Criterion DCS 1080p", 10},
		{"Brazil 1985 Criterion 5.1 DCEU 1080p", 10},
	}
	for _, tc := range cases {
		t.Run(tc.title, func(t *testing.T) {
			got := ScoreTitle(tc.title, 0, DefaultRules())
			if got.Score != tc.wantScore {
				t.Errorf("score=%d want=%d (reasons=%v)", got.Score, tc.wantScore, got.Reasons)
			}
		})
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
