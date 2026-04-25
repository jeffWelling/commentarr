// Package search owns the release/candidate persistence and the
// Searcher orchestrator.
package search

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jeffWelling/commentarr/internal/indexer"
	"github.com/jeffWelling/commentarr/internal/verify"
)

// Candidate is one row returned by ListCandidates.
type Candidate struct {
	TitleID          string
	Release          indexer.Release
	Score            int
	Reasons          []verify.Reason
	LikelyCommentary bool
}

// Repo persists releases + title↔release candidate edges.
type Repo struct {
	db *sql.DB
}

// NewRepo returns a Repo backed by d.
func NewRepo(d *sql.DB) *Repo { return &Repo{db: d} }

// SaveCandidates upserts every release and its candidate edge for a
// given title. A second call for the same (title, release) pair updates
// the score + reasons to the latest values.
func (r *Repo) SaveCandidates(ctx context.Context, titleID string, scored []verify.Scored) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	for _, s := range scored {
		id := s.Release.Identity()
		_, err := tx.ExecContext(ctx, `
			INSERT INTO releases
			  (identity, infohash, url, title, size_bytes, seeders, leechers, indexer, protocol, published_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
			ON CONFLICT(identity) DO UPDATE SET
			  seeders = excluded.seeders,
			  leechers = excluded.leechers,
			  title = excluded.title`,
			id,
			nullIfEmpty(s.Release.InfoHash),
			nullIfEmpty(s.Release.URL),
			s.Release.Title,
			s.Release.SizeBytes,
			s.Release.Seeders,
			s.Release.Leechers,
			s.Release.Indexer,
			s.Release.Protocol,
			nullIfZeroTime(s.Release.PublishedAt),
		)
		if err != nil {
			return fmt.Errorf("upsert release %s: %w", id, err)
		}

		reasonsJSON, err := json.Marshal(s.Reasons)
		if err != nil {
			return fmt.Errorf("marshal reasons: %w", err)
		}
		_, err = tx.ExecContext(ctx, `
			INSERT INTO title_candidates
			  (title_id, release_identity, score, reasons_json, likely)
			VALUES (?, ?, ?, ?, ?)
			ON CONFLICT(title_id, release_identity) DO UPDATE SET
			  score = excluded.score,
			  reasons_json = excluded.reasons_json,
			  likely = excluded.likely`,
			titleID, id, s.Score, string(reasonsJSON), s.LikelyCommentary,
		)
		if err != nil {
			return fmt.Errorf("upsert candidate %s→%s: %w", titleID, id, err)
		}
	}
	return tx.Commit()
}

// ListCandidates returns every candidate for a title, sorted by score
// descending. Within tied scores, healthier swarms come first
// (seeders DESC) so the picker prefers releases that will actually
// download — a top-scored release with a dead swarm is worse than a
// same-scored release with 100 seeders.
func (r *Repo) ListCandidates(ctx context.Context, titleID string) ([]Candidate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.title_id, c.score, c.reasons_json, c.likely,
		       r.infohash, r.url, r.title, r.size_bytes, r.seeders, r.leechers, r.indexer, r.protocol
		FROM title_candidates c
		JOIN releases r ON r.identity = c.release_identity
		WHERE c.title_id = ?
		ORDER BY c.score DESC, r.seeders DESC, c.created_at ASC`, titleID)
	if err != nil {
		return nil, fmt.Errorf("list candidates %s: %w", titleID, err)
	}
	defer rows.Close()

	var out []Candidate
	for rows.Next() {
		var c Candidate
		var reasonsJSON string
		var infohash, url sql.NullString
		if err := rows.Scan(
			&c.TitleID, &c.Score, &reasonsJSON, &c.LikelyCommentary,
			&infohash, &url,
			&c.Release.Title, &c.Release.SizeBytes, &c.Release.Seeders, &c.Release.Leechers,
			&c.Release.Indexer, &c.Release.Protocol,
		); err != nil {
			return nil, err
		}
		c.Release.InfoHash = infohash.String
		c.Release.URL = url.String
		if err := json.Unmarshal([]byte(reasonsJSON), &c.Reasons); err != nil {
			return nil, fmt.Errorf("unmarshal reasons: %w", err)
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullIfZeroTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}
