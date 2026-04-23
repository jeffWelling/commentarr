CREATE TABLE safety_rules (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL UNIQUE,
  expression   TEXT NOT NULL,
  action       TEXT NOT NULL CHECK (action IN (
    'block_replace','block_import','warn','log_only')),
  enabled      BOOLEAN NOT NULL DEFAULT TRUE,
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE safety_profiles (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL UNIQUE,
  rule_ids_json TEXT NOT NULL DEFAULT '[]',
  created_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE library_safety_profile (
  library  TEXT PRIMARY KEY,
  profile  TEXT NOT NULL REFERENCES safety_profiles(id) ON DELETE RESTRICT
);
