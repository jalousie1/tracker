-- Migration 012: Optimization and Scaling
BEGIN;

-- 1. Upgrade IDs to BIGINT to prevent overflow in high-volume tables
ALTER TABLE user_history ALTER COLUMN id TYPE BIGINT;
ALTER TABLE username_history ALTER COLUMN id TYPE BIGINT;
ALTER TABLE avatar_history ALTER COLUMN id TYPE BIGINT;
ALTER TABLE bio_history ALTER COLUMN id TYPE BIGINT;
ALTER TABLE connected_accounts ALTER COLUMN id TYPE BIGINT;
ALTER TABLE alt_relationships ALTER COLUMN id TYPE BIGINT;
ALTER TABLE voice_sessions ALTER COLUMN id TYPE BIGINT;
ALTER TABLE voice_participants ALTER COLUMN id TYPE BIGINT;
ALTER TABLE voice_stats ALTER COLUMN id TYPE BIGINT;
ALTER TABLE presence_history ALTER COLUMN id TYPE BIGINT;
ALTER TABLE activity_history ALTER COLUMN id TYPE BIGINT;
ALTER TABLE banner_history ALTER COLUMN id TYPE BIGINT;
ALTER TABLE nickname_history ALTER COLUMN id TYPE BIGINT;
ALTER TABLE clan_history ALTER COLUMN id TYPE BIGINT;
ALTER TABLE message_stats ALTER COLUMN id TYPE BIGINT;
ALTER TABLE avatar_decoration_history ALTER COLUMN id TYPE BIGINT;
ALTER TABLE messages ALTER COLUMN id TYPE BIGINT;
ALTER TABLE voice_partner_stats ALTER COLUMN id TYPE BIGINT;

-- 2. Add Unique Constraint to connected_accounts for ON CONFLICT support
-- Deduplicate rows before adding the unique constraint (keeps the latest row)
DELETE FROM connected_accounts c1
USING connected_accounts c2
WHERE c1.id < c2.id
  AND c1.user_id = c2.user_id
  AND c1.type = c2.type
  AND (c1.external_id = c2.external_id OR (c1.external_id IS NULL AND c2.external_id IS NULL));

ALTER TABLE connected_accounts 
ADD CONSTRAINT unique_user_type_external UNIQUE (user_id, type, external_id);

-- 3. Optimization Indexes
-- Composite index for frequent profile lookups
CREATE INDEX IF NOT EXISTS idx_messages_guild_channel ON messages(guild_id, channel_id);

-- Index for voice partners lookup
CREATE INDEX IF NOT EXISTS idx_voice_participants_session_user ON voice_participants(session_id, user_id);

-- GIN Index for faster bio content search if it's large (pg_trgm already exists in init.sql)
CREATE INDEX IF NOT EXISTS idx_bio_history_content_trgm ON bio_history USING GIN (bio_content gin_trgm_ops);

-- 4. Cleanup Redundant Records (Example: deduplicate avatar history if hash is same as previous)
-- This logic will be added to the Go processor, but we can do a one-time cleanup.
-- Keeping this commented out as it might be destructive without careful verification under load.
/*
DELETE FROM avatar_history a1
USING avatar_history a2
WHERE a1.user_id = a2.user_id
  AND a1.id > a2.id
  AND a1.hash_avatar = a2.hash_avatar
  AND a1.changed_at - a2.changed_at < interval '1 hour';
*/

COMMIT;
