-- Migration 005: Add guild_members table for tracking which tokens can access which users
BEGIN;

-- tabela para salvar relacao entre membros e guilds
-- permite saber qual token pode buscar qual usuario
CREATE TABLE IF NOT EXISTS guild_members (
  id SERIAL PRIMARY KEY,
  guild_id TEXT NOT NULL,
  user_id TEXT NOT NULL,
  token_id INTEGER NOT NULL,
  joined_at TIMESTAMPTZ,
  discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(guild_id, user_id, token_id)
);

-- indices para buscas rapidas
CREATE INDEX IF NOT EXISTS idx_guild_members_user_id 
  ON guild_members(user_id);

CREATE INDEX IF NOT EXISTS idx_guild_members_guild_id 
  ON guild_members(guild_id);

CREATE INDEX IF NOT EXISTS idx_guild_members_token_id 
  ON guild_members(token_id);

-- indice composto para buscar tokens que podem acessar um usuario
CREATE INDEX IF NOT EXISTS idx_guild_members_user_token 
  ON guild_members(user_id, token_id);

-- tabela para salvar quais guilds cada token tem acesso
CREATE TABLE IF NOT EXISTS token_guilds (
  id SERIAL PRIMARY KEY,
  token_id INTEGER NOT NULL,
  guild_id TEXT NOT NULL,
  guild_name TEXT,
  member_count INTEGER,
  discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_synced_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE(token_id, guild_id)
);

-- indices
CREATE INDEX IF NOT EXISTS idx_token_guilds_token_id 
  ON token_guilds(token_id);

CREATE INDEX IF NOT EXISTS idx_token_guilds_guild_id 
  ON token_guilds(guild_id);

COMMIT;

