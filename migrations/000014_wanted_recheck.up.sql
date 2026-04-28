-- Q8B: re-search resolved titles after a configurable interval. A
-- Criterion edition might come out years after a regular BD; without
-- this, Commentarr would never notice.
--
-- next_recheck_at:
--   * NULL on rows that pre-date this migration (treated as "never recheck"
--     by DueForRecheck — operator can MarkResolved again to set one)
--   * set to now + ResolveRecheckInterval by queue.MarkResolved going forward
--   * advanced after each recheck cycle by the searcher

ALTER TABLE wanted ADD COLUMN next_recheck_at DATETIME;

-- Index covers the DueForRecheck query: status = 'resolved' AND
-- next_recheck_at <= now. Partial index keeps it small (no rows where
-- next_recheck_at IS NULL).
CREATE INDEX idx_wanted_recheck ON wanted(status, next_recheck_at)
  WHERE next_recheck_at IS NOT NULL;
