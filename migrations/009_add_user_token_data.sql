-- Migration 009: Adicionar tabelas para dados de user token
-- User tokens recebem muito mais dados que bots no READY event
BEGIN;

-- Tabela de relacionamentos (amigos) - exclusivo de user tokens
CREATE TABLE IF NOT EXISTS relationships (
  id SERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  friend_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  relationship_type INTEGER NOT NULL, -- 1=friend, 2=blocked, 3=incoming_request, 4=outgoing_request
  nickname TEXT,
  discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_seen_at TIMESTAMPTZ DEFAULT NOW(),
  UNIQUE(user_id, friend_id)
);

CREATE INDEX IF NOT EXISTS idx_relationships_user ON relationships(user_id);
CREATE INDEX IF NOT EXISTS idx_relationships_friend ON relationships(friend_id);
CREATE INDEX IF NOT EXISTS idx_relationships_type ON relationships(relationship_type);

-- Tabela de roles dos guilds
CREATE TABLE IF NOT EXISTS roles (
  role_id TEXT PRIMARY KEY,
  guild_id TEXT NOT NULL,
  name TEXT NOT NULL,
  color INTEGER DEFAULT 0,
  position INTEGER DEFAULT 0,
  permissions TEXT,
  hoist BOOLEAN DEFAULT FALSE,
  mentionable BOOLEAN DEFAULT FALSE,
  icon TEXT,
  discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_roles_guild ON roles(guild_id);

-- Tabela de emojis dos guilds
CREATE TABLE IF NOT EXISTS emojis (
  emoji_id TEXT PRIMARY KEY,
  guild_id TEXT NOT NULL,
  name TEXT NOT NULL,
  animated BOOLEAN DEFAULT FALSE,
  available BOOLEAN DEFAULT TRUE,
  discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_emojis_guild ON emojis(guild_id);

-- Adicionar mais campos na tabela guilds
ALTER TABLE guilds 
  ADD COLUMN IF NOT EXISTS banner TEXT,
  ADD COLUMN IF NOT EXISTS owner_id TEXT,
  ADD COLUMN IF NOT EXISTS description TEXT,
  ADD COLUMN IF NOT EXISTS presence_count INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS premium_tier INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS premium_subscription_count INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS features TEXT[];

-- Adicionar mais campos na tabela channels
ALTER TABLE channels 
  ADD COLUMN IF NOT EXISTS parent_id TEXT,
  ADD COLUMN IF NOT EXISTS position INTEGER DEFAULT 0,
  ADD COLUMN IF NOT EXISTS topic TEXT,
  ADD COLUMN IF NOT EXISTS nsfw BOOLEAN DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS user_limit INTEGER DEFAULT 0;

-- Adicionar mais campos na tabela guild_members
ALTER TABLE guild_members
  ADD COLUMN IF NOT EXISTS nickname TEXT,
  ADD COLUMN IF NOT EXISTS roles TEXT[],
  ADD COLUMN IF NOT EXISTS joined_at TEXT;

-- Criar indice para discovered_at e last_seen_at em guild_members
CREATE INDEX IF NOT EXISTS idx_guild_members_discovered ON guild_members(discovered_at);
CREATE INDEX IF NOT EXISTS idx_guild_members_last_seen ON guild_members(last_seen_at);

COMMIT;
