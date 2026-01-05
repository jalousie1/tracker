-- Migration 012: Add account info columns to tokens table
-- When a token is added, we fetch @me to get the real account info
BEGIN;

ALTER TABLE tokens
  ADD COLUMN IF NOT EXISTS account_user_id TEXT,
  ADD COLUMN IF NOT EXISTS account_username TEXT,
  ADD COLUMN IF NOT EXISTS account_display_name TEXT,
  ADD COLUMN IF NOT EXISTS account_avatar TEXT;

-- Create index for account lookup
CREATE INDEX IF NOT EXISTS idx_tokens_account_user_id ON tokens(account_user_id);

COMMIT;
