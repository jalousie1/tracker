BEGIN;

CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS users (
  id TEXT PRIMARY KEY,
  status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'suspended', 'banned')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  last_updated_at TIMESTAMPTZ
);

CREATE TABLE IF NOT EXISTS tokens (
  id SERIAL PRIMARY KEY,
  token TEXT NOT NULL UNIQUE,
  status TEXT NOT NULL DEFAULT 'ativo' CHECK (status IN ('ativo', 'banido', 'suspenso')),
  created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS user_history (
  id BIGSERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  username TEXT,
  discriminator TEXT,
  global_name TEXT,
  nickname TEXT,
  avatar_hash TEXT,
  avatar_url TEXT,
  bio_content TEXT,
  observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS username_history (
  id BIGSERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  username TEXT,
  discriminator TEXT,
  global_name TEXT,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS avatar_history (
  id BIGSERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  hash_avatar TEXT,
  url_cdn TEXT,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS bio_history (
  id BIGSERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  bio_content TEXT,
  changed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS connected_accounts (
  id BIGSERIAL PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  type TEXT NOT NULL,
  external_id TEXT,
  name TEXT,
  observed_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  UNIQUE (user_id, type, external_id)
);

CREATE INDEX IF NOT EXISTS idx_username_history_user_id
  ON username_history (user_id);

CREATE INDEX IF NOT EXISTS idx_avatar_history_user_id
  ON avatar_history (user_id);

CREATE INDEX IF NOT EXISTS idx_bio_history_user_id
  ON bio_history (user_id);

CREATE INDEX IF NOT EXISTS idx_connected_accounts_user_id
  ON connected_accounts (user_id);

CREATE INDEX IF NOT EXISTS idx_user_history_user_id
  ON user_history (user_id);

CREATE INDEX IF NOT EXISTS idx_user_history_observed_at
  ON user_history (observed_at);

CREATE INDEX IF NOT EXISTS idx_username_history_username_trgm
  ON username_history USING GIN (username gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_username_history_global_name_trgm
  ON username_history USING GIN (global_name gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_user_history_username_trgm
  ON user_history USING GIN (username gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_user_history_global_name_trgm
  ON user_history USING GIN (global_name gin_trgm_ops);

CREATE INDEX IF NOT EXISTS idx_user_history_nickname_trgm
  ON user_history USING GIN (nickname gin_trgm_ops);

COMMIT;

