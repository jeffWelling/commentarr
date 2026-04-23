CREATE TABLE titles (
  id           TEXT PRIMARY KEY,
  kind         TEXT NOT NULL CHECK (kind IN ('movie','episode')),
  display_name TEXT NOT NULL,
  year         INTEGER,
  tmdb_id      TEXT,
  imdb_id      TEXT,
  series_id    TEXT,
  season       INTEGER,
  episode      INTEGER,
  file_path    TEXT NOT NULL,
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_titles_series ON titles(series_id) WHERE series_id IS NOT NULL;
CREATE INDEX idx_titles_kind ON titles(kind);
