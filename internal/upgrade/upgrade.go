// Package upgrade detects when a re-search of a resolved title has
// surfaced a candidate scoring higher than the release we already
// imported. Used by the search-tick (which fans the result to a
// webhook) and by /api/v1/upgrades (which renders it in the UI).
//
// Pure read: no DB writes, no webhooks. Callers compose the side
// effects they want.
package upgrade

import (
	"context"
	"fmt"

	"github.com/jeffWelling/commentarr/internal/download"
	"github.com/jeffWelling/commentarr/internal/search"
	"github.com/jeffWelling/commentarr/internal/verify"
)

// Info is one detected upgrade — the imported release vs. the new
// top candidate, with both scores so the caller can render the
// magnitude of the upgrade.
type Info struct {
	TitleID           string
	CurrentRelease    string
	CurrentScore      int
	CandidateRelease  string
	CandidateScore    int
	CandidateIndexer  string
	CandidateInfoHash string
}

// Find walks titleIDs and returns one Info per title where the top
// likely-commentary candidate (score >= threshold) outscores the
// release we last imported. Re-scoring uses verify.DefaultRules,
// matching what the picker did at grab time, so a tie doesn't fire.
//
// Titles with no imported job at all (operator hasn't grabbed
// anything yet, e.g. still wanted) are skipped silently — there's no
// "current" to upgrade FROM.
func Find(ctx context.Context, candRepo *search.Repo, jobRepo *download.JobRepo, titleIDs []string, threshold int) ([]Info, error) {
	rules := verify.DefaultRules()
	var out []Info
	for _, titleID := range titleIDs {
		job, err := jobRepo.LastImportedForTitle(ctx, titleID)
		if err != nil {
			// ErrJobNotFound is the common case (no imported job);
			// keep going.
			continue
		}
		currentScore := verify.ScoreTitle(job.ReleaseTitle, 0, rules).Score

		cands, err := candRepo.ListCandidates(ctx, titleID)
		if err != nil {
			return nil, fmt.Errorf("list candidates %s: %w", titleID, err)
		}
		var top *search.Candidate
		for i := range cands {
			c := &cands[i]
			if !c.LikelyCommentary || c.Score < threshold {
				continue
			}
			top = c
			break
		}
		if top == nil || top.Score <= currentScore {
			continue
		}

		out = append(out, Info{
			TitleID:           titleID,
			CurrentRelease:    job.ReleaseTitle,
			CurrentScore:      currentScore,
			CandidateRelease:  top.Release.Title,
			CandidateScore:    top.Score,
			CandidateIndexer:  top.Release.Indexer,
			CandidateInfoHash: top.Release.InfoHash,
		})
	}
	return out, nil
}
