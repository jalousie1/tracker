-- Migration 013: Add guild_name to voice_sessions
BEGIN;

ALTER TABLE voice_sessions ADD COLUMN IF NOT EXISTS guild_name TEXT;

COMMIT;
