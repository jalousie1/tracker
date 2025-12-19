-- Migration 004: Add public data fields to users table
BEGIN;

-- adicionar campos para dados públicos
ALTER TABLE users 
  ADD COLUMN IF NOT EXISTS public_data_source TEXT,
  ADD COLUMN IF NOT EXISTS last_public_fetch TIMESTAMPTZ,
  ADD COLUMN IF NOT EXISTS banner_hash TEXT,
  ADD COLUMN IF NOT EXISTS banner_url TEXT;

-- criar indice para busca por fonte de dados
CREATE INDEX IF NOT EXISTS idx_users_public_data_source 
  ON users(public_data_source) 
  WHERE public_data_source IS NOT NULL;

-- criar indice para busca por ultima atualizacao publica
CREATE INDEX IF NOT EXISTS idx_users_last_public_fetch 
  ON users(last_public_fetch) 
  WHERE last_public_fetch IS NOT NULL;

COMMIT;

