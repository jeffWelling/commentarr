CREATE TABLE trash_items (
  id              INTEGER PRIMARY KEY AUTOINCREMENT,
  library         TEXT NOT NULL,
  original_path   TEXT NOT NULL,
  trash_path      TEXT NOT NULL UNIQUE,
  size_bytes      INTEGER NOT NULL DEFAULT 0,
  moved_at        DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  purged          BOOLEAN NOT NULL DEFAULT FALSE,
  purged_at       DATETIME,
  reason          TEXT
);
CREATE INDEX idx_trash_moved_at ON trash_items(moved_at) WHERE purged = FALSE;
CREATE INDEX idx_trash_library  ON trash_items(library);
