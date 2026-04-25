CREATE TABLE download_jobs (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  client_name     TEXT NOT NULL,
  client_job_id   TEXT NOT NULL,
  title_id        TEXT NOT NULL,
  release_title   TEXT NOT NULL DEFAULT '',
  edition         TEXT NOT NULL DEFAULT '',
  added_at        TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
  imported_at     TIMESTAMP,
  status          TEXT NOT NULL DEFAULT 'queued',
  outcome         TEXT NOT NULL DEFAULT '',
  UNIQUE (client_name, client_job_id)
);

CREATE INDEX download_jobs_title ON download_jobs (title_id);
CREATE INDEX download_jobs_status ON download_jobs (status);
