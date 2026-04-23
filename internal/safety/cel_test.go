package safety

import "testing"

func TestCEL_CompileAndEvaluate_True(t *testing.T) {
	rule, err := CompileRule("classifier_confidence >= 0.85")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	passed, err := rule.Evaluate(Facts{ClassifierConfidence: 0.9})
	if err != nil {
		t.Fatal(err)
	}
	if !passed {
		t.Fatal("expected rule to pass")
	}
}

func TestCEL_CompileAndEvaluate_False(t *testing.T) {
	rule, err := CompileRule("classifier_confidence >= 0.85")
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	passed, err := rule.Evaluate(Facts{ClassifierConfidence: 0.7})
	if err != nil {
		t.Fatal(err)
	}
	if passed {
		t.Fatal("expected rule to fail")
	}
}

func TestCEL_CompileRejectsBadSyntax(t *testing.T) {
	_, err := CompileRule("not valid cel ===")
	if err == nil {
		t.Fatal("expected compile error")
	}
}

func TestCEL_RejectsNonBoolExpression(t *testing.T) {
	_, err := CompileRule("classifier_confidence + 1.0")
	if err == nil {
		t.Fatal("expected type error for non-bool expression")
	}
}

func TestCEL_AllFieldsExposed(t *testing.T) {
	exprs := []string{
		"classifier_confidence >= 0.0",
		"classifier_commentary_track_count >= 0",
		"audio_track_count >= 0",
		"original_audio_track_count >= 0",
		"video_bitrate_mbps >= 0.0",
		"original_video_bitrate_mbps >= 0.0",
		"container == ''",
		"file_magic_matches_extension == true || file_magic_matches_extension == false",
		"file_size_bytes >= 0",
		"release_title == ''",
		"release_group == ''",
		"indexer == ''",
		"seeders >= 0",
		"duration_seconds >= 0.0",
	}
	for _, e := range exprs {
		if _, err := CompileRule(e); err != nil {
			t.Errorf("expr %q: %v", e, err)
		}
	}
}

func TestEvaluateCEL_CollectsViolations(t *testing.T) {
	r1, _ := CompileRule("classifier_confidence >= 0.85")
	r2, _ := CompileRule("audio_track_count >= original_audio_track_count")
	rules := []CompiledRule{
		{Name: "high_confidence", Compiled: r1, Action: ActionBlockReplace},
		{Name: "gain_tracks", Compiled: r2, Action: ActionBlockReplace},
	}

	res := EvaluateCEL(Facts{ClassifierConfidence: 0.7, AudioTrackCount: 3, OriginalAudioTrackCount: 2}, rules)
	if res.Passed() {
		t.Fatal("should have a violation")
	}
	if len(res.Violations) != 1 || res.Violations[0].Rule != "high_confidence" {
		t.Fatalf("unexpected violations: %+v", res.Violations)
	}
}
