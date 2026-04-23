// Package safety evaluates safety rules before imports happen. Built-in
// rules live here; user-defined rules compile from CEL expressions
// against the same Facts struct (see cel.go).
package safety

// Facts is the immutable input to every safety rule. Fill it in once
// per candidate import and pass around — don't mutate.
type Facts struct {
	ClassifierConfidence           float64
	ClassifierCommentaryTrackCount int

	AudioTrackCount         int
	OriginalAudioTrackCount int

	VideoBitrateMbps         float64
	OriginalVideoBitrateMbps float64

	Container                 string
	FileMagicMatchesExtension bool
	FileSizeBytes             int64

	ReleaseTitle    string
	ReleaseGroup    string
	Indexer         string
	Seeders         int
	DurationSeconds float64
}

// HasCommentaryLabeled is a convenience for rules that want to gate on
// the classifier finding at least one commentary-labeled track.
func (f Facts) HasCommentaryLabeled() bool { return f.ClassifierCommentaryTrackCount > 0 }

// AudioTrackDelta is new minus original. Positive = gained tracks.
func (f Facts) AudioTrackDelta() int { return f.AudioTrackCount - f.OriginalAudioTrackCount }

// VideoBitrateRatio is new / original. Zero if either side is zero.
func (f Facts) VideoBitrateRatio() float64 {
	if f.OriginalVideoBitrateMbps <= 0 || f.VideoBitrateMbps <= 0 {
		return 0
	}
	return f.VideoBitrateMbps / f.OriginalVideoBitrateMbps
}
