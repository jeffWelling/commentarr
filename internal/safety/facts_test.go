package safety

import "testing"

func TestFacts_FromInputs(t *testing.T) {
	f := Facts{
		ClassifierConfidence:           0.92,
		ClassifierCommentaryTrackCount: 1,
		AudioTrackCount:                3,
		OriginalAudioTrackCount:        2,
		VideoBitrateMbps:               25,
		OriginalVideoBitrateMbps:       30,
		Container:                      "mkv",
		FileMagicMatchesExtension:      true,
		ReleaseTitle:                   "X.2020.Criterion.Commentary",
	}
	if !f.HasCommentaryLabeled() {
		t.Fatal("ClassifierCommentaryTrackCount=1 should read as HasCommentaryLabeled")
	}
	if f.AudioTrackDelta() != 1 {
		t.Fatalf("AudioTrackDelta: got %d want 1", f.AudioTrackDelta())
	}
	if f.VideoBitrateRatio() == 0 {
		t.Fatal("VideoBitrateRatio should be computed when both sides present")
	}
}

func TestFacts_VideoBitrateRatio_ZeroIfMissing(t *testing.T) {
	cases := []Facts{
		{VideoBitrateMbps: 10, OriginalVideoBitrateMbps: 0},
		{VideoBitrateMbps: 0, OriginalVideoBitrateMbps: 10},
		{},
	}
	for _, f := range cases {
		if r := f.VideoBitrateRatio(); r != 0 {
			t.Errorf("expected 0 for %+v, got %v", f, r)
		}
	}
}
