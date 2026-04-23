// Package title owns the Title entity and its persistence.
package title

import (
	"context"
	"database/sql"
	"fmt"
)

// Kind distinguishes movies from TV episodes.
type Kind string

const (
	KindMovie   Kind = "movie"
	KindEpisode Kind = "episode"
)

// Title represents one owned piece of content.
type Title struct {
	ID          string
	Kind        Kind
	DisplayName string
	Year        int
	TMDBID      string
	IMDBID      string
	SeriesID    string
	Season      int
	Episode     int
	FilePath    string
}

// Repo persists Title rows.
type Repo interface {
	Insert(ctx context.Context, t Title) error
	FindByID(ctx context.Context, id string) (Title, error)
	List(ctx context.Context) ([]Title, error)
}

type titleRepo struct{ db *sql.DB }

// NewRepo returns a Repo backed by the given *sql.DB.
func NewRepo(d *sql.DB) *titleRepo { return &titleRepo{db: d} }

func (r *titleRepo) Insert(ctx context.Context, t Title) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO titles
		(id, kind, display_name, year, tmdb_id, imdb_id, series_id, season, episode, file_path)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, string(t.Kind), t.DisplayName, t.Year, t.TMDBID, t.IMDBID,
		nullableString(t.SeriesID), nullableInt(t.Season), nullableInt(t.Episode), t.FilePath)
	if err != nil {
		return fmt.Errorf("insert title %s: %w", t.ID, err)
	}
	return nil
}

func (r *titleRepo) FindByID(ctx context.Context, id string) (Title, error) {
	var t Title
	var kind string
	var series sql.NullString
	var season, episode sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT id, kind, display_name, year, tmdb_id, imdb_id,
		       series_id, season, episode, file_path
		FROM titles WHERE id = ?`, id).
		Scan(&t.ID, &kind, &t.DisplayName, &t.Year, &t.TMDBID, &t.IMDBID,
			&series, &season, &episode, &t.FilePath)
	if err != nil {
		return Title{}, fmt.Errorf("find title %s: %w", id, err)
	}
	t.Kind = Kind(kind)
	t.SeriesID = series.String
	t.Season = int(season.Int64)
	t.Episode = int(episode.Int64)
	return t, nil
}

func (r *titleRepo) List(ctx context.Context) ([]Title, error) {
	// Single query returns every column we need. Don't do FindByID in the
	// row-iteration loop — that would deadlock when the connection pool is
	// capped at 1 (see db.Open for the ":memory:" case).
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, kind, display_name, year, tmdb_id, imdb_id,
		       series_id, season, episode, file_path
		FROM titles ORDER BY display_name`)
	if err != nil {
		return nil, fmt.Errorf("list titles: %w", err)
	}
	defer rows.Close()

	var out []Title
	for rows.Next() {
		var t Title
		var kind string
		var series sql.NullString
		var season, episode sql.NullInt64
		if err := rows.Scan(&t.ID, &kind, &t.DisplayName, &t.Year, &t.TMDBID, &t.IMDBID,
			&series, &season, &episode, &t.FilePath); err != nil {
			return nil, fmt.Errorf("scan title row: %w", err)
		}
		t.Kind = Kind(kind)
		t.SeriesID = series.String
		t.Season = int(season.Int64)
		t.Episode = int(episode.Int64)
		out = append(out, t)
	}
	return out, rows.Err()
}

func nullableString(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func nullableInt(i int) interface{} {
	if i == 0 {
		return nil
	}
	return i
}
