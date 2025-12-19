-- Migration 001: Extend tokens table for encryption and management
BEGIN;

-- Add new columns to tokens table
ALTER TABLE tokens 
  ADD COLUMN IF NOT EXISTS token_encrypted TEXT,
  ADD COLUMN IF NOT EXISTS user_id TEXT,
  ADD COLUMN IF NOT EXISTS failure_count INT DEFAULT 0,
  ADD COLUMN IF NOT EXISTS suspended_until TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS banned_at TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS last_used TIMESTAMPTZ;

-- Migrate existing token data if token column exists and token_encrypted is empty
-- Note: This assumes tokens will be encrypted later by the application
UPDATE tokens 
SET token_encrypted = token 
WHERE token_encrypted IS NULL AND token IS NOT NULL;

-- Create token_failures table
CREATE TABLE IF NOT EXISTS token_failures (
  id SERIAL PRIMARY KEY,
  token_id INT NOT NULL REFERENCES tokens(id) ON DELETE CASCADE,
  reason TEXT NOT NULL,
  occurred_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_token_failures_token_id 
  ON token_failures(token_id);

CREATE INDEX IF NOT EXISTS idx_token_failures_occurred_at 
  ON token_failures(occurred_at);

COMMIT;

