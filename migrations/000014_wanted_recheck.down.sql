DROP INDEX IF EXISTS idx_wanted_recheck;
ALTER TABLE wanted DROP COLUMN next_recheck_at;
