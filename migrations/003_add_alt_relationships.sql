-- Migration 003: Add alt_relationships table and indexes
BEGIN;

CREATE TABLE IF NOT EXISTS alt_relationships (
  id SERIAL PRIMARY KEY,
  user_a TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  user_b TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  confidence_score DECIMAL(3,2) NOT NULL CHECK (confidence_score >= 0.00 AND confidence_score <= 1.00),
  detection_method TEXT,
  detected_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(user_a, user_b)
);

CREATE INDEX IF NOT EXISTS idx_alt_relationships_user_a 
  ON alt_relationships(user_a);

CREATE INDEX IF NOT EXISTS idx_alt_relationships_user_b 
  ON alt_relationships(user_b);

CREATE INDEX IF NOT EXISTS idx_alt_relationships_confidence 
  ON alt_relationships(confidence_score DESC);

CREATE INDEX IF NOT EXISTS idx_alt_relationships_detected_at 
  ON alt_relationships(detected_at DESC);

-- Add index for connected_accounts external_id (partial index)
CREATE INDEX IF NOT EXISTS idx_connected_accounts_external_id 
  ON connected_accounts(external_id, type) 
  WHERE external_id IS NOT NULL;

-- Add index for username_history changed_at
CREATE INDEX IF NOT EXISTS idx_username_history_changed_at 
  ON username_history(changed_at DESC);

-- Add last_seen_at to connected_accounts if not exists
ALTER TABLE connected_accounts 
  ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMPTZ;

-- Update last_seen_at to observed_at if null
UPDATE connected_accounts 
SET last_seen_at = observed_at 
WHERE last_seen_at IS NULL;

COMMIT;

