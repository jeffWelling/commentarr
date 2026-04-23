CREATE TABLE imports (
  id                 INTEGER PRIMARY KEY AUTOINCREMENT,
  title_id           TEXT NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
  release_identity   TEXT NOT NULL REFERENCES releases(identity) ON DELETE CASCADE,
  client_name        TEXT NOT NULL,
  client_job_id      TEXT NOT NULL,
  status             TEXT NOT NULL CHECK (status IN (
    'grabbed','downloading','completed','imported','safety_violation','non_compliant','error')),
  mode               TEXT CHECK (mode IN ('replace','sidecar','separate-library')),
  new_file_path      TEXT,
  old_file_path      TEXT,
  trash_path         TEXT,
  error              TEXT,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_imports_status   ON imports(status);
CREATE INDEX idx_imports_job      ON imports(client_name, client_job_id);
CREATE INDEX idx_imports_title    ON imports(title_id);
