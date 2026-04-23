package safety

import "testing"

func TestBuiltin_ClassifierThreshold(t *testing.T) {
	cfg := BuiltinConfig{ClassifierConfidenceThreshold: 0.85}
	facts := Facts{ClassifierConfidence: 0.9, ClassifierCommentaryTrackCount: 1}
	if res := EvaluateBuiltin(facts, cfg); res.Passed() != true {
		t.Fatalf("0.9 confidence should pass threshold 0.85, got %+v", res)
	}

	facts.ClassifierConfidence = 0.7
	res := EvaluateBuiltin(facts, cfg)
	if res.Passed() {
		t.Fatal("0.7 confidence should fail threshold 0.85")
	}
	foundRule := false
	for _, v := range res.Violations {
		if v.Rule == "classifier_confidence" {
			foundRule = true
		}
	}
	if !foundRule {
		t.Fatalf("expected classifier_confidence violation in %+v", res.Violations)
	}
}

func TestBuiltin_AudioTrackCountMinimum(t *testing.T) {
	cfg := BuiltinConfig{
		ClassifierConfidenceThreshold: 0.85,
		RequireAudioTracksGE:          true,
	}
	facts := Facts{
		ClassifierConfidence:           0.9,
		ClassifierCommentaryTrackCount: 1,
		AudioTrackCount:                1,
		OriginalAudioTrackCount:        2,
	}
	res := EvaluateBuiltin(facts, cfg)
	if res.Passed() {
		t.Fatalf("fewer audio tracks than original should fail: %+v", res)
	}
}

func TestBuiltin_VideoBitrate80Pct(t *testing.T) {
	cfg := BuiltinConfig{
		ClassifierConfidenceThreshold: 0.85,
		RequireVideoBitratePct:        true,
		VideoBitrateMinRatio:          0.8,
	}
	facts := Facts{
		ClassifierConfidence:     0.95,
		VideoBitrateMbps:         20,
		OriginalVideoBitrateMbps: 70,
	}
	res := EvaluateBuiltin(facts, cfg)
	if res.Passed() {
		t.Fatalf("29%% of original bitrate should fail 80%% threshold: %+v", res)
	}

	facts.VideoBitrateMbps = 65
	res = EvaluateBuiltin(facts, cfg)
	if !res.Passed() {
		t.Fatalf("93%% of original should pass 80%% threshold: %+v", res)
	}
}

func TestBuiltin_MagicByteCheck(t *testing.T) {
	cfg := BuiltinConfig{
		ClassifierConfidenceThreshold: 0.85,
		RequireMagicMatch:             true,
	}
	facts := Facts{
		ClassifierConfidence:      0.95,
		FileMagicMatchesExtension: false,
	}
	res := EvaluateBuiltin(facts, cfg)
	if res.Passed() {
		t.Fatalf("magic mismatch should fail: %+v", res)
	}
}

func TestBuiltin_AllDisabledPassesEverything(t *testing.T) {
	cfg := BuiltinConfig{}
	facts := Facts{}
	res := EvaluateBuiltin(facts, cfg)
	if !res.Passed() {
		t.Fatalf("empty config should pass empty facts, got %+v", res)
	}
}
