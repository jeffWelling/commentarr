CREATE TABLE releases (
  identity       TEXT PRIMARY KEY,
  infohash       TEXT,
  url            TEXT,
  title          TEXT NOT NULL,
  size_bytes     INTEGER NOT NULL DEFAULT 0,
  seeders        INTEGER NOT NULL DEFAULT 0,
  leechers       INTEGER NOT NULL DEFAULT 0,
  indexer        TEXT NOT NULL,
  protocol       TEXT NOT NULL DEFAULT 'torrent',
  published_at   DATETIME,
  first_seen_at  DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);
CREATE INDEX idx_releases_indexer ON releases(indexer);
CREATE INDEX idx_releases_infohash ON releases(infohash) WHERE infohash IS NOT NULL;
