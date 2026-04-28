package search

import (
	"context"
	"errors"
	"fmt"
	"log"

	"github.com/jeffWelling/commentarr/internal/download"
	"github.com/jeffWelling/commentarr/internal/metrics"
	"github.com/jeffWelling/commentarr/internal/webhook"
)

// Picker walks the wanted queue, finds titles whose top likely-commentary
// candidate hasn't been submitted yet, and queues one download per title.
//
// "Hasn't been submitted yet" means: no row in download_jobs for that
// title id whose status is anything other than "error". A previous error
// is allowed to be retried; a queued/completed/imported job blocks
// further submissions for the same title.
type Picker struct {
	candidates   *Repo
	jobs         *download.JobRepo
	client       download.DownloadClient
	dispatcher   *webhook.Dispatcher // optional; nil disables OnGrab dispatch
	category     string
	threshold    int
	maxSizeBytes int64 // 0 = no cap
	dryRun       bool
	logf         func(format string, args ...any) // overridable for tests
}

// NewPicker returns a Picker. dispatcher is optional — pass nil if
// the caller doesn't want the OnGrab webhook fired (e.g., a one-shot
// CLI invocation).
func NewPicker(candidates *Repo, jobs *download.JobRepo, client download.DownloadClient, dispatcher *webhook.Dispatcher, category string, threshold int) *Picker {
	if threshold <= 0 {
		threshold = 8
	}
	if category == "" {
		category = "commentarr"
	}
	return &Picker{
		candidates: candidates, jobs: jobs, client: client,
		dispatcher: dispatcher,
		category:   category, threshold: threshold,
		logf: log.Printf,
	}
}

// WithMaxSize returns the same Picker with a size cap on candidates.
// Candidates whose Release.SizeBytes exceeds the cap are skipped, so
// the picker prefers a same-scored release that actually fits the
// operator's bandwidth/disk budget. Passing 0 (or negative) disables
// the cap. Live homelab probe: Brazil's score-15 UHD candidates were
// 60GB+ while score-15 1080p versions were ~5GB; without a cap the
// picker can't tell them apart.
func (p *Picker) WithMaxSize(bytes int64) *Picker {
	if bytes <= 0 {
		bytes = 0
	}
	p.maxSizeBytes = bytes
	return p
}

// WithDryRun returns the same Picker configured for dry-run mode. In
// dry-run, PickAndQueueOne logs what *would* be queued, increments
// PickerDecisionsTotal{decision="dry_run"}, and skips the download
// client's Add() + the JobRepo Save() calls. Used by `serve -dry-run`
// to smoke-test against real Prowlarr + qBit without queueing or
// modifying anything.
func (p *Picker) WithDryRun(b bool) *Picker {
	p.dryRun = b
	return p
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
	pick, ok := selectBest(cands, p.threshold, p.maxSizeBytes)
	if !ok {
		metrics.PickerDecisionsTotal.WithLabelValues("skipped_no_candidate").Inc()
		return "", false, nil
	}

	magnet := magnetOrURL(pick)
	if magnet == "" {
		metrics.PickerDecisionsTotal.WithLabelValues("skipped_no_candidate").Inc()
		return "", false, nil
	}
	if p.dryRun {
		// Caller treats this as a no-op (ok=false). The dry_run
		// metric + log line are how the operator sees what would
		// have happened.
		metrics.PickerDecisionsTotal.WithLabelValues("dry_run").Inc()
		p.logf("DRY-RUN: would queue title=%s release=%q score=%d magnet=%s indexer=%s",
			titleID, pick.Release.Title, pick.Score, magnet, pick.Release.Indexer)
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
	if p.dispatcher != nil {
		_ = p.dispatcher.Dispatch(ctx, webhook.EventGrab, map[string]interface{}{
			"title_id":      titleID,
			"client":        p.client.Name(),
			"client_job_id": jobID,
			"release_title": pick.Release.Title,
			"score":         pick.Score,
			"indexer":       pick.Release.Indexer,
		})
	}
	return jobID, true, nil
}

// hasInflightJob returns true when at least one non-errored job already
// covers titleID. Backed by JobRepo.HasInflightForTitle so this is one
// indexed query, not an N×status scan.
func (p *Picker) hasInflightJob(ctx context.Context, titleID string) (bool, error) {
	return p.jobs.HasInflightForTitle(ctx, titleID)
}

// selectBest picks the highest-scoring likely-commentary candidate at
// or above threshold, optionally bounded by maxSizeBytes. When the
// size cap is set, candidates above the cap are skipped; the loop
// keeps going so a smaller same-score candidate (or a smaller next-
// score-down candidate) can still be picked. Candidates are already
// sorted desc by score, so a below-threshold candidate triggers a
// no-pick result regardless of size.
func selectBest(cs []Candidate, threshold int, maxSizeBytes int64) (Candidate, bool) {
	for _, c := range cs {
		if !c.LikelyCommentary {
			continue
		}
		if c.Score < threshold {
			return Candidate{}, false // sorted desc — nothing else will qualify
		}
		if maxSizeBytes > 0 && c.Release.SizeBytes > maxSizeBytes {
			continue // too big — try the next candidate at this score, then lower scores
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
