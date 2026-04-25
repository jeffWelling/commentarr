package search

import (
	"context"
	"errors"
	"fmt"

	"github.com/jeffWelling/commentarr/internal/download"
	"github.com/jeffWelling/commentarr/internal/metrics"
)

// Picker walks the wanted queue, finds titles whose top likely-commentary
// candidate hasn't been submitted yet, and queues one download per title.
//
// "Hasn't been submitted yet" means: no row in download_jobs for that
// title id whose status is anything other than "error". A previous error
// is allowed to be retried; a queued/completed/imported job blocks
// further submissions for the same title.
type Picker struct {
	candidates *Repo
	jobs       *download.JobRepo
	client     download.DownloadClient
	category   string
	threshold  int
}

// NewPicker returns a Picker.
func NewPicker(candidates *Repo, jobs *download.JobRepo, client download.DownloadClient, category string, threshold int) *Picker {
	if threshold <= 0 {
		threshold = 8
	}
	if category == "" {
		category = "commentarr"
	}
	return &Picker{
		candidates: candidates, jobs: jobs, client: client,
		category: category, threshold: threshold,
	}
}

// PickAndQueueOne selects the highest-scoring likely-commentary
// candidate for titleID and submits it to the download client. Returns
// (jobID, true) on submission, ("", false) when no candidate qualifies
// or when an existing in-flight job already covers this title.
func (p *Picker) PickAndQueueOne(ctx context.Context, titleID string) (string, bool, error) {
	if existing, err := p.hasInflightJob(ctx, titleID); err != nil {
		metrics.PickerDecisionsTotal.WithLabelValues("error").Inc()
		return "", false, err
	} else if existing {
		metrics.PickerDecisionsTotal.WithLabelValues("skipped_inflight").Inc()
		return "", false, nil
	}

	cands, err := p.candidates.ListCandidates(ctx, titleID)
	if err != nil {
		metrics.PickerDecisionsTotal.WithLabelValues("error").Inc()
		return "", false, fmt.Errorf("list candidates %s: %w", titleID, err)
	}
	pick, ok := selectBest(cands, p.threshold)
	if !ok {
		metrics.PickerDecisionsTotal.WithLabelValues("skipped_no_candidate").Inc()
		return "", false, nil
	}

	magnet := magnetOrURL(pick)
	if magnet == "" {
		metrics.PickerDecisionsTotal.WithLabelValues("skipped_no_candidate").Inc()
		return "", false, nil
	}
	jobID, err := p.client.Add(ctx, download.AddRequest{
		MagnetOrURL: magnet,
		Category:    p.category,
	})
	if err != nil {
		metrics.PickerDecisionsTotal.WithLabelValues("error").Inc()
		return "", false, fmt.Errorf("client.Add: %w", err)
	}
	if _, err := p.jobs.Save(ctx, download.Job{
		ClientName:   p.client.Name(),
		ClientJobID:  jobID,
		TitleID:      titleID,
		ReleaseTitle: pick.Release.Title,
		Status:       "queued",
	}); err != nil {
		metrics.PickerDecisionsTotal.WithLabelValues("error").Inc()
		return jobID, true, fmt.Errorf("save job: %w", err)
	}
	metrics.PickerDecisionsTotal.WithLabelValues("queued").Inc()
	return jobID, true, nil
}

// hasInflightJob returns true when at least one non-errored job already
// covers titleID. Backed by JobRepo.HasInflightForTitle so this is one
// indexed query, not an N×status scan.
func (p *Picker) hasInflightJob(ctx context.Context, titleID string) (bool, error) {
	return p.jobs.HasInflightForTitle(ctx, titleID)
}

// selectBest picks the highest-scoring likely-commentary candidate at
// or above threshold. Candidates are already sorted desc by score.
func selectBest(cs []Candidate, threshold int) (Candidate, bool) {
	for _, c := range cs {
		if !c.LikelyCommentary {
			continue
		}
		if c.Score < threshold {
			return Candidate{}, false // sorted desc — nothing else will qualify
		}
		return c, true
	}
	return Candidate{}, false
}

// magnetOrURL returns the URI we hand to a download client. Magnets
// (built from infohash) win over URLs because every supported client
// accepts magnet links, and the watcher's job-id matching is more
// stable across restarts when the client computed the infohash itself.
func magnetOrURL(c Candidate) string {
	if c.Release.InfoHash != "" {
		return "magnet:?xt=urn:btih:" + c.Release.InfoHash
	}
	return c.Release.URL
}

// ErrNoClient is returned by daemon wiring when something tries to use
// the Picker but no download client is configured.
var ErrNoClient = errors.New("picker: no download client configured")
