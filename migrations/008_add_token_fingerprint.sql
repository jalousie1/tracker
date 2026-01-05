-- Migration 008: Add stable token fingerprint for deduplication
BEGIN;

ALTER TABLE tokens
  ADD COLUMN IF NOT EXISTS token_fingerprint TEXT;

-- Unique only for non-null fingerprints (allows existing rows to be backfilled by the app)
CREATE UNIQUE INDEX IF NOT EXISTS ux_tokens_token_fingerprint
  ON tokens(token_fingerprint)
  WHERE token_fingerprint IS NOT NULL;

COMMIT;
