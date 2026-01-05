-- Migration 010: Add missing indexes for performance
BEGIN;

-- avatar_history precisa de índice em (user_id, hash_avatar) para a query de dedup
CREATE INDEX IF NOT EXISTS idx_avatar_history_user_hash 
  ON avatar_history(user_id, hash_avatar);

CREATE INDEX IF NOT EXISTS idx_avatar_history_user_id 
  ON avatar_history(user_id);

CREATE INDEX IF NOT EXISTS idx_avatar_history_changed_at 
  ON avatar_history(changed_at DESC);

-- bio_history precisa de índice para query de dedup
CREATE INDEX IF NOT EXISTS idx_bio_history_user_content 
  ON bio_history(user_id, bio_content);

CREATE INDEX IF NOT EXISTS idx_bio_history_user_id 
  ON bio_history(user_id);

-- username_history precisa de índice para query de dedup
CREATE INDEX IF NOT EXISTS idx_username_history_user_id 
  ON username_history(user_id);

CREATE INDEX IF NOT EXISTS idx_username_history_user_combo 
  ON username_history(user_id, username, discriminator, global_name);

-- users table indices
CREATE INDEX IF NOT EXISTS idx_users_last_updated 
  ON users(last_updated_at DESC);

COMMIT;
