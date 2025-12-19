-- Migration 007: Add messages table for full message history
BEGIN;

-- tabela de mensagens (armazena conteudo das mensagens)
CREATE TABLE IF NOT EXISTS messages (
  id SERIAL PRIMARY KEY,
  message_id TEXT UNIQUE NOT NULL, -- discord message id
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  guild_id TEXT,
  channel_id TEXT NOT NULL,
  channel_name TEXT,
  content TEXT,
  created_at TIMESTAMPTZ NOT NULL,
  edited_at TIMESTAMPTZ,
  has_attachments BOOLEAN DEFAULT FALSE,
  has_embeds BOOLEAN DEFAULT FALSE,
  reply_to_message_id TEXT,
  reply_to_user_id TEXT
);

-- indices para mensagens
CREATE INDEX IF NOT EXISTS idx_messages_user_id ON messages(user_id);
CREATE INDEX IF NOT EXISTS idx_messages_guild_id ON messages(guild_id);
CREATE INDEX IF NOT EXISTS idx_messages_channel_id ON messages(channel_id);
CREATE INDEX IF NOT EXISTS idx_messages_created_at ON messages(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_messages_user_guild ON messages(user_id, guild_id);

-- melhorar voice_participants para incluir guild e channel
ALTER TABLE voice_participants 
  ADD COLUMN IF NOT EXISTS guild_id TEXT,
  ADD COLUMN IF NOT EXISTS channel_id TEXT,
  ADD COLUMN IF NOT EXISTS left_at TIMESTAMPTZ;

-- tabela para contar pessoas com quem mais faz call
CREATE TABLE IF NOT EXISTS voice_partner_stats (
  id SERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  partner_id TEXT NOT NULL,
  guild_id TEXT NOT NULL,
  total_sessions INTEGER DEFAULT 0,
  total_duration_seconds BIGINT DEFAULT 0,
  last_call_at TIMESTAMPTZ,
  UNIQUE(user_id, partner_id, guild_id)
);

CREATE INDEX IF NOT EXISTS idx_voice_partner_stats_user ON voice_partner_stats(user_id);
CREATE INDEX IF NOT EXISTS idx_voice_partner_stats_partner ON voice_partner_stats(partner_id);

-- adicionar channel_name na voice_sessions se nao existir
ALTER TABLE voice_sessions ADD COLUMN IF NOT EXISTS channel_name TEXT;

-- tabela para cache de canais (nome do canal)
CREATE TABLE IF NOT EXISTS channels (
  channel_id TEXT PRIMARY KEY,
  guild_id TEXT,
  name TEXT,
  type INTEGER, -- 0=text, 2=voice, etc
  discovered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_channels_guild ON channels(guild_id);

COMMIT;
