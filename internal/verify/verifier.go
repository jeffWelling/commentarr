package verify

import (
	"sort"

	"github.com/jeffWelling/commentarr/internal/indexer"
)

// Scored pairs a release with its computed score + reasons.
type Scored struct {
	Release          indexer.Release
	Score            int
	Reasons          []Reason
	LikelyCommentary bool // Score ≥ threshold
}

// Verifier scores releases against a configured ruleset.
type Verifier struct {
	rules     []Rule
	threshold int
}

// NewVerifier returns a Verifier using the given rules and threshold.
func NewVerifier(rules []Rule, threshold int) *Verifier {
	return &Verifier{rules: rules, threshold: threshold}
}

// Score returns every input release scored, sorted by score descending.
func (v *Verifier) Score(in []indexer.Release) []Scored {
	out := make([]Scored, 0, len(in))
	for _, r := range in {
		verdict := ScoreTitle(r.Title, r.SizeBytes, v.rules)
		out = append(out, Scored{
			Release:          r,
			Score:            verdict.Score,
			Reasons:          verdict.Reasons,
			LikelyCommentary: verdict.Score >= v.threshold,
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
