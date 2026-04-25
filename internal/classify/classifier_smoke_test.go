package classify

import (
	"strings"
	"testing"

	classifier "github.com/jeffWelling/commentary-classifier"
)

// uniformCorrMatrix returns an n×n matrix with high off-diagonal
// correlation (0.9). High correlation tells the cluster scorer that
// every track sounds like every other track — i.e., none is an
// outlier. That keeps the cluster signal neutral so the synthetic
// metadata-label scenarios actually exercise the metadata-trust path
// instead of getting hijacked by a "this track is an outlier so it
// must be commentary" inference.
func uniformCorrMatrix(n int) [][]float64 {
	m := make([][]float64, n)
	for i := range m {
		m[i] = make([]float64, n)
		for j := range m[i] {
			if i == j {
				m[i][j] = 1.0
			} else {
				m[i][j] = 0.9
			}
		}
	}
	return m
}

// TestClassifierSmoke_MetadataLabeledCommentary catches upstream
// commentary-classifier API changes that would break Commentarr's
// integration. Two scenarios cribbed from group_a / group_b in the
// benchmark fixture format:
//
//   - group A: track titles with no commentary keyword → no track
//     should be flagged commentary
//   - group B: track 1 labeled "Commentary by Director" → that track
//     should be flagged commentary, others should not
//
// We don't depend on real audio / ffprobe — pass synthetic feature
// values matching the typical pattern (commentary tracks have higher
// silence ratio, lower channel count). The metadata-label trust path
// is the dominant signal in v1, so this smoke covers the path that
// matters most.
func TestClassifierSmoke_MetadataLabeledCommentary(t *testing.T) {
	t.Run("group_a no commentary labels", func(t *testing.T) {
		analyses := []classifier.TrackAnalysis{
			{TrackIndex: 0, MetadataLabel: "English DTS-HD MA", Channels: 6, Bitrate: 1500000, SilenceRatio: 0.05, DynamicRange: 30, SpectralCentroid: 1500, RMSEnergyStd: 0.2},
			{TrackIndex: 1, MetadataLabel: "English AC3", Channels: 2, Bitrate: 192000, SilenceRatio: 0.06, DynamicRange: 28, SpectralCentroid: 1450, RMSEnergyStd: 0.18},
		}
		results := classifier.ClassifyTracks(analyses, uniformCorrMatrix(len(analyses)), nil, true)
		if len(results) != 2 {
			t.Fatalf("expected 2 results, got %d", len(results))
		}
		for _, r := range results {
			if r.Recommendation == "commentary" {
				t.Errorf("track %d wrongly flagged commentary; reasoning: %s", r.TrackIndex, r.Reasoning)
			}
		}
	})

	t.Run("group_b track 1 labeled commentary", func(t *testing.T) {
		analyses := []classifier.TrackAnalysis{
			{TrackIndex: 0, MetadataLabel: "English DTS-HD MA 7.1", Channels: 8, Bitrate: 3000000, SilenceRatio: 0.04, DynamicRange: 32, SpectralCentroid: 1600, RMSEnergyStd: 0.21},
			{TrackIndex: 1, MetadataLabel: "English Commentary by Director", Channels: 2, Bitrate: 192000, SilenceRatio: 0.18, DynamicRange: 18, SpectralCentroid: 800, RMSEnergyStd: 0.10},
			{TrackIndex: 2, MetadataLabel: "English AC3 Descriptive", Channels: 2, Bitrate: 192000, SilenceRatio: 0.07, DynamicRange: 25, SpectralCentroid: 1400, RMSEnergyStd: 0.15},
		}
		results := classifier.ClassifyTracks(analyses, uniformCorrMatrix(len(analyses)), nil, true)
		if len(results) != 3 {
			t.Fatalf("expected 3 results, got %d", len(results))
		}
		commentaryFound := false
		for _, r := range results {
			if r.TrackIndex == 1 {
				if r.Recommendation != "commentary" {
					t.Errorf("labeled commentary track was missed: %s (confidence=%.2f, reasoning=%s)",
						r.Recommendation, r.CommentaryConfidence, r.Reasoning)
				}
				if !strings.Contains(strings.ToLower(r.Reasoning), "commentary") {
					t.Errorf("commentary reasoning missing the word 'commentary': %q", r.Reasoning)
				}
				commentaryFound = true
				continue
			}
			if r.Recommendation == "commentary" {
				t.Errorf("non-commentary track %d wrongly flagged: %s", r.TrackIndex, r.Reasoning)
			}
		}
		if !commentaryFound {
			t.Error("track 1 (the labeled commentary) was not in the results")
		}
	})
}

// TestClassifierSmoke_EmptyInputReturnsNil pins the empty-input
// behaviour. The Commentarr-side service relies on len(results)==0
// meaning "no commentary"; an upstream change that returned a single
// "no_commentary" placeholder would silently break the wanted-queue
// gating.
func TestClassifierSmoke_EmptyInputReturnsNil(t *testing.T) {
	if got := classifier.ClassifyTracks(nil, nil, nil, true); got != nil {
		t.Errorf("expected nil for empty input, got %+v", got)
	}
}
