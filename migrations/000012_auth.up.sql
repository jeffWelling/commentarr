CREATE TABLE admin_account (
  id                 INTEGER PRIMARY KEY CHECK (id = 1),
  username           TEXT NOT NULL,
  password_hash      TEXT NOT NULL,
  created_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  updated_at         DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE api_keys (
  key_hash    TEXT PRIMARY KEY,
  label       TEXT NOT NULL,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  last_used   DATETIME
);

CREATE TABLE auth_sessions (
  token       TEXT PRIMARY KEY,
  created_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
  expires_at  DATETIME NOT NULL
);
CREATE INDEX idx_sessions_expires ON auth_sessions(expires_at);
