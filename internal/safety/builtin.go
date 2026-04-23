package safety

// BuiltinConfig toggles and thresholds the hard-coded safety rules from
// DESIGN.md § 5.8. Every toggle is off by default; callers opt in.
type BuiltinConfig struct {
	ClassifierConfidenceThreshold float64
	RequireAudioTracksGE          bool
	RequireVideoBitratePct        bool
	VideoBitrateMinRatio          float64
	RequireMagicMatch             bool
}

// Violation records a single failed rule.
type Violation struct {
	Rule   string
	Detail string
}

// Result is the output of EvaluateBuiltin + EvaluateCEL (see cel.go).
type Result struct {
	Violations []Violation
}

// Passed reports whether no violations were recorded.
func (r Result) Passed() bool { return len(r.Violations) == 0 }

// EvaluateBuiltin runs the built-in safety checks against facts.
func EvaluateBuiltin(f Facts, cfg BuiltinConfig) Result {
	var r Result

	if cfg.ClassifierConfidenceThreshold > 0 && f.ClassifierConfidence < cfg.ClassifierConfidenceThreshold {
		r.Violations = append(r.Violations, Violation{
			Rule:   "classifier_confidence",
			Detail: "confidence below threshold",
		})
	}
	if cfg.RequireAudioTracksGE && f.AudioTrackDelta() < 0 {
		r.Violations = append(r.Violations, Violation{
			Rule:   "audio_track_count",
			Detail: "new file has fewer audio tracks than original",
		})
	}
	if cfg.RequireVideoBitratePct {
		min := cfg.VideoBitrateMinRatio
		if min <= 0 {
			min = 0.8
		}
		if ratio := f.VideoBitrateRatio(); ratio > 0 && ratio < min {
			r.Violations = append(r.Violations, Violation{
				Rule:   "video_bitrate",
				Detail: "new/original bitrate ratio below threshold",
			})
		}
	}
	if cfg.RequireMagicMatch && !f.FileMagicMatchesExtension {
		r.Violations = append(r.Violations, Violation{
			Rule:   "file_magic_matches_extension",
			Detail: "magic bytes do not match file extension",
		})
	}

	return r
}
