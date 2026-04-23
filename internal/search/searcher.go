package search

import (
	"context"
	"fmt"
	"time"

	"github.com/jeffWelling/commentarr/internal/indexer"
	"github.com/jeffWelling/commentarr/internal/queue"
	"github.com/jeffWelling/commentarr/internal/schedule"
	"github.com/jeffWelling/commentarr/internal/title"
	"github.com/jeffWelling/commentarr/internal/verify"
)

// Searcher orchestrates one pass through the wanted queue: pick titles
// due for search, query every configured indexer with commentary-biased
// variants, dedupe, score, persist candidates, advance next_search_at.
type Searcher struct {
	indexers []indexer.Indexer
	verifier *verify.Verifier
	repo     *Repo
	queue    *queue.Queue
	titles   title.Repo
	limit    int
}

// NewSearcher returns a Searcher.
func NewSearcher(indexers []indexer.Indexer, v *verify.Verifier, repo *Repo, q *queue.Queue, titles title.Repo, limit int) *Searcher {
	if limit <= 0 {
		limit = 100
	}
	return &Searcher{indexers: indexers, verifier: v, repo: repo, queue: q, titles: titles, limit: limit}
}

// SearchDue runs one search pass for every wanted title whose
// next_search_at has elapsed as of now. Returns the number of titles
// actually processed (not the number of candidates persisted).
func (s *Searcher) SearchDue(ctx context.Context, now time.Time) (int, error) {
	due, err := s.queue.DueForSearch(ctx, now)
	if err != nil {
		return 0, fmt.Errorf("due for search: %w", err)
	}

	processed := 0
	librarySize, _ := s.estimateLibrarySize(ctx)
	for _, e := range due {
		if err := ctx.Err(); err != nil {
			return processed, err
		}
		t, err := s.titles.FindByID(ctx, e.TitleID)
		if err != nil {
			return processed, fmt.Errorf("find title %s: %w", e.TitleID, err)
		}
		if err := s.searchOne(ctx, t); err != nil {
			return processed, err
		}

		seed := hashSeed(t.ID, now.UnixNano())
		interval := schedule.NextSearchInterval(librarySize, seed)
		if err := s.queue.UpdateNextSearchAt(ctx, t.ID, now.Add(interval)); err != nil {
			return processed, fmt.Errorf("advance next_search_at %s: %w", t.ID, err)
		}
		processed++
	}
	return processed, nil
}

func (s *Searcher) searchOne(ctx context.Context, t title.Title) error {
	queries := indexer.BuildQueries(t.DisplayName, t.Year, t.Season, t.Episode)
	dedup := indexer.NewDeduper()
	var all []indexer.Release

	for _, idx := range s.indexers {
		for _, q := range queries {
			released, err := idx.Search(ctx, indexer.Query{
				Title:      q,
				Year:       t.Year,
				Limit:      s.limit,
				Categories: []int{categoryFor(t.Kind)},
			})
			if err != nil {
				// One flaky indexer/query shouldn't kill the whole title.
				// Metrics already emitted inside the adapter.
				continue
			}
			for _, r := range released {
				if !dedup.Seen(r) {
					all = append(all, r)
				}
			}
		}
	}

	if len(all) == 0 {
		return s.queue.IncrementSearchMiss(ctx, t.ID)
	}
	scored := s.verifier.Score(all)
	return s.repo.SaveCandidates(ctx, t.ID, scored)
}

func (s *Searcher) estimateLibrarySize(ctx context.Context) (int, error) {
	titles, err := s.titles.List(ctx)
	if err != nil {
		return 0, err
	}
	return len(titles), nil
}

func categoryFor(k title.Kind) int {
	switch k {
	case title.KindEpisode:
		return 5000
	default:
		return 2000
	}
}

// hashSeed mixes a stable title id with the current time so cadence
// jitter differs per title without being fully random per call.
func hashSeed(titleID string, ts int64) int64 {
	var s int64 = ts
	for _, b := range []byte(titleID) {
		s = s*31 + int64(b)
	}
	return s
}
