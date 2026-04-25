package download

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// Job records that we asked a download client to fetch a release for a
// title. The (ClientName, ClientJobID) pair is what the watcher matches
// when a job completes; everything else is what the importer needs to
// run the post-download pipeline once the file lands.
type Job struct {
	ID           int64
	ClientName   string
	ClientJobID  string
	TitleID      string
	ReleaseTitle string
	Edition      string
	AddedAt      time.Time
	ImportedAt   time.Time
	Status       string // queued | completed | imported | error
	Outcome      string // optional importer outcome string
}

// ErrJobNotFound is returned when a (client, job-id) lookup misses.
var ErrJobNotFound = errors.New("download job not found")

// JobRepo persists download jobs.
type JobRepo struct {
	db *sql.DB
}

// NewJobRepo returns a JobRepo backed by d.
func NewJobRepo(d *sql.DB) *JobRepo { return &JobRepo{db: d} }

// Save upserts a job by (client, job-id). Returns the row's id.
func (r *JobRepo) Save(ctx context.Context, j Job) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		INSERT INTO download_jobs
		  (client_name, client_job_id, title_id, release_title, edition, status)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(client_name, client_job_id) DO UPDATE SET
		  title_id = excluded.title_id,
		  release_title = excluded.release_title,
		  edition = excluded.edition`,
		j.ClientName, j.ClientJobID, j.TitleID, j.ReleaseTitle, j.Edition, defaultStatus(j.Status))
	if err != nil {
		return 0, fmt.Errorf("save job: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return id, nil
}

// FindByClientJob returns the job tagged with the given client name +
// job id, or ErrJobNotFound.
func (r *JobRepo) FindByClientJob(ctx context.Context, client, jobID string) (Job, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, client_name, client_job_id, title_id, release_title, edition,
		       added_at, imported_at, status, outcome
		FROM download_jobs WHERE client_name = ? AND client_job_id = ?`,
		client, jobID)
	return scanJob(row)
}

// MarkStatus updates the job's status (and optional outcome). When the
// new status is "imported" or "error", imported_at is set to now.
func (r *JobRepo) MarkStatus(ctx context.Context, id int64, status, outcome string) error {
	if status == "imported" || status == "error" {
		_, err := r.db.ExecContext(ctx, `
			UPDATE download_jobs SET status = ?, outcome = ?, imported_at = CURRENT_TIMESTAMP
			WHERE id = ?`, status, outcome, id)
		return err
	}
	_, err := r.db.ExecContext(ctx, `
		UPDATE download_jobs SET status = ?, outcome = ? WHERE id = ?`,
		status, outcome, id)
	return err
}

// ListByStatus returns jobs in the given status, oldest first.
func (r *JobRepo) ListByStatus(ctx context.Context, status string) ([]Job, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, client_name, client_job_id, title_id, release_title, edition,
		       added_at, imported_at, status, outcome
		FROM download_jobs WHERE status = ? ORDER BY added_at`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

type scanner interface {
	Scan(dest ...any) error
}

func scanJob(s scanner) (Job, error) {
	var j Job
	var imported sql.NullTime
	if err := s.Scan(&j.ID, &j.ClientName, &j.ClientJobID, &j.TitleID,
		&j.ReleaseTitle, &j.Edition, &j.AddedAt, &imported, &j.Status, &j.Outcome); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Job{}, ErrJobNotFound
		}
		return Job{}, err
	}
	if imported.Valid {
		j.ImportedAt = imported.Time
	}
	return j, nil
}

func defaultStatus(s string) string {
	if s == "" {
		return "queued"
	}
	return s
}
