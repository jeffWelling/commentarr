CREATE TABLE non_compliant_torrents (
  release_identity TEXT PRIMARY KEY REFERENCES releases(identity) ON DELETE CASCADE,
  reason           TEXT NOT NULL,
  detected_at      DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  revisit_after    DATETIME
);
CREATE INDEX idx_non_compliant_revisit ON non_compliant_torrents(revisit_after)
  WHERE revisit_after IS NOT NULL;
