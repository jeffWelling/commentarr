CREATE TABLE title_verdicts (
  title_id           TEXT PRIMARY KEY REFERENCES titles(id) ON DELETE CASCADE,
  has_commentary     BOOLEAN NOT NULL,
  confidence         REAL NOT NULL,
  classifier_version TEXT NOT NULL,
  classified_at      DATETIME NOT NULL
);
