CREATE TABLE indexers (
  name                 TEXT PRIMARY KEY,
  kind                 TEXT NOT NULL DEFAULT 'prowlarr',
  base_url             TEXT NOT NULL,
  api_key_encrypted    TEXT NOT NULL,
  requests_per_minute  INTEGER NOT NULL DEFAULT 6,
  burst                INTEGER NOT NULL DEFAULT 3,
  enabled              BOOLEAN NOT NULL DEFAULT TRUE,
  created_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at           DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
