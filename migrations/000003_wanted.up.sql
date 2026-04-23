CREATE TABLE wanted (
  title_id         TEXT PRIMARY KEY REFERENCES titles(id) ON DELETE CASCADE,
  status           TEXT NOT NULL CHECK (status IN ('wanted','skipped','resolved')),
  last_searched_at DATETIME,
  next_search_at   DATETIME,
  search_misses    INTEGER NOT NULL DEFAULT 0,
  updated_at       DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_wanted_status_next ON wanted(status, next_search_at);
