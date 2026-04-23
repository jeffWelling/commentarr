CREATE TABLE title_candidates (
  title_id         TEXT NOT NULL REFERENCES titles(id) ON DELETE CASCADE,
  release_identity TEXT NOT NULL REFERENCES releases(identity) ON DELETE CASCADE,
  score            INTEGER NOT NULL,
  reasons_json     TEXT NOT NULL,
  likely           BOOLEAN NOT NULL,
  created_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (title_id, release_identity)
);
CREATE INDEX idx_title_candidates_title ON title_candidates(title_id);
CREATE INDEX idx_title_candidates_score ON title_candidates(title_id, score DESC);
