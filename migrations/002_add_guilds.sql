-- Migration 002: Add guilds table
BEGIN;

CREATE TABLE IF NOT EXISTS guilds (
  guild_id TEXT PRIMARY KEY,
  name TEXT,
  member_count INT,
  discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_guilds_discovered_at 
  ON guilds(discovered_at);

COMMIT;

