-- Migration 006: Complete tracking - salvar todas as informacoes possiveis
BEGIN;

-- historico de chamadas de voz
CREATE TABLE IF NOT EXISTS voice_sessions (
  id SERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  guild_id TEXT NOT NULL,
  channel_id TEXT NOT NULL,
  channel_name TEXT,
  joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  left_at TIMESTAMPTZ,
  duration_seconds INTEGER,
  was_muted BOOLEAN DEFAULT FALSE,
  was_deafened BOOLEAN DEFAULT FALSE,
  was_streaming BOOLEAN DEFAULT FALSE,
  was_video BOOLEAN DEFAULT FALSE
);

-- participantes de chamadas (quem estava junto)
CREATE TABLE IF NOT EXISTS voice_participants (
  id SERIAL PRIMARY KEY,
  session_id INTEGER REFERENCES voice_sessions(id) ON DELETE CASCADE,
  user_id TEXT NOT NULL,
  joined_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- estatisticas de voz por usuario
CREATE TABLE IF NOT EXISTS voice_stats (
  id SERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  guild_id TEXT NOT NULL,
  total_sessions INTEGER DEFAULT 0,
  total_duration_seconds BIGINT DEFAULT 0,
  last_session_at TIMESTAMPTZ,
  UNIQUE(user_id, guild_id)
);

-- historico de status/presenca
CREATE TABLE IF NOT EXISTS presence_history (
  id SERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  guild_id TEXT,
  status TEXT NOT NULL, -- online, offline, idle, dnd, invisible
  changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- historico de atividades (jogos, spotify, custom status, etc)
CREATE TABLE IF NOT EXISTS activity_history (
  id SERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  activity_type INTEGER NOT NULL, -- 0=playing, 1=streaming, 2=listening, 3=watching, 4=custom, 5=competing
  name TEXT NOT NULL,
  details TEXT,
  state TEXT,
  url TEXT,
  application_id TEXT,
  started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  ended_at TIMESTAMPTZ,
  -- para spotify
  spotify_track_id TEXT,
  spotify_artist TEXT,
  spotify_album TEXT
);

-- historico de banners
CREATE TABLE IF NOT EXISTS banner_history (
  id SERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  banner_hash TEXT,
  banner_color TEXT, -- accent color hex
  url_cdn TEXT,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- historico de nicknames por servidor
CREATE TABLE IF NOT EXISTS nickname_history (
  id SERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  guild_id TEXT NOT NULL,
  nickname TEXT,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- historico de clans/badges
CREATE TABLE IF NOT EXISTS clan_history (
  id SERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  clan_tag TEXT,
  clan_identity_guild_id TEXT,
  badge TEXT,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- mensagens enviadas (para estatisticas)
CREATE TABLE IF NOT EXISTS message_stats (
  id SERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  guild_id TEXT NOT NULL,
  channel_id TEXT NOT NULL,
  message_count INTEGER DEFAULT 0,
  last_message_at TIMESTAMPTZ,
  first_message_at TIMESTAMPTZ,
  UNIQUE(user_id, guild_id, channel_id)
);

-- historico de decoracoes de avatar
CREATE TABLE IF NOT EXISTS avatar_decoration_history (
  id SERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  decoration_asset TEXT,
  decoration_sku_id TEXT,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- campos extras na tabela users
ALTER TABLE users 
  ADD COLUMN IF NOT EXISTS accent_color INTEGER,
  ADD COLUMN IF NOT EXISTS premium_type INTEGER,
  ADD COLUMN IF NOT EXISTS public_flags INTEGER,
  ADD COLUMN IF NOT EXISTS bot BOOLEAN DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS is_system BOOLEAN DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS mfa_enabled BOOLEAN,
  ADD COLUMN IF NOT EXISTS locale TEXT,
  ADD COLUMN IF NOT EXISTS verified BOOLEAN,
  ADD COLUMN IF NOT EXISTS email TEXT,
  ADD COLUMN IF NOT EXISTS flags INTEGER,
  ADD COLUMN IF NOT EXISTS avatar_decoration_data JSONB,
  ADD COLUMN IF NOT EXISTS clan_data JSONB;

-- indices para performance
CREATE INDEX IF NOT EXISTS idx_voice_sessions_user_id ON voice_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_voice_sessions_guild_id ON voice_sessions(guild_id);
CREATE INDEX IF NOT EXISTS idx_voice_sessions_joined_at ON voice_sessions(joined_at);

CREATE INDEX IF NOT EXISTS idx_voice_stats_user_id ON voice_stats(user_id);
CREATE INDEX IF NOT EXISTS idx_voice_stats_guild_id ON voice_stats(guild_id);

CREATE INDEX IF NOT EXISTS idx_presence_history_user_id ON presence_history(user_id);
CREATE INDEX IF NOT EXISTS idx_presence_history_changed_at ON presence_history(changed_at);

CREATE INDEX IF NOT EXISTS idx_activity_history_user_id ON activity_history(user_id);
CREATE INDEX IF NOT EXISTS idx_activity_history_name ON activity_history(name);
CREATE INDEX IF NOT EXISTS idx_activity_history_started_at ON activity_history(started_at);

CREATE INDEX IF NOT EXISTS idx_banner_history_user_id ON banner_history(user_id);

CREATE INDEX IF NOT EXISTS idx_nickname_history_user_id ON nickname_history(user_id);
CREATE INDEX IF NOT EXISTS idx_nickname_history_guild_id ON nickname_history(guild_id);

CREATE INDEX IF NOT EXISTS idx_clan_history_user_id ON clan_history(user_id);

CREATE INDEX IF NOT EXISTS idx_message_stats_user_id ON message_stats(user_id);
CREATE INDEX IF NOT EXISTS idx_message_stats_guild_id ON message_stats(guild_id);

CREATE INDEX IF NOT EXISTS idx_avatar_decoration_history_user_id ON avatar_decoration_history(user_id);

COMMIT;

