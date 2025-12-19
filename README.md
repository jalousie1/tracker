# Identity Archive

Sistema OSINT para rastreamento de identidades Discord com histórico completo, detecção de contas alternativas e API REST.

## Arquitetura

- **Worker**: Conecta userbots ao Discord Gateway, coleta dados e processa eventos em tempo real
- **API**: Expõe endpoints REST para consulta de perfis, busca e detecção de alts

## Requisitos

- Go 1.24+
- PostgreSQL 16+ com extensão `pg_trgm`
- Redis 7+
- Variáveis de ambiente configuradas (ver `.env.example`)

## Configuração

1. Configure as variáveis de ambiente:
   - `DB_DSN`: Connection string do PostgreSQL
   - `REDIS_DSN`: Connection string do Redis (ex: `redis://localhost:6379/0`)
   - `ENCRYPTION_KEY`: Chave de 32 bytes em base64 para criptografia de tokens
   - `ADMIN_SECRET_KEY`: Chave secreta para endpoints admin
   - `CORS_ORIGINS`: Origins permitidos (separados por vírgula)
   - `R2_ENDPOINT`, `R2_BUCKET`, `R2_KEYS`: Configuração do Cloudflare R2 (opcional)

2. Execute as migrações:
   ```bash
   psql $DB_DSN -f migrations/001_extend_tokens.sql
   psql $DB_DSN -f migrations/002_add_guilds.sql
   psql $DB_DSN -f migrations/003_add_alt_relationships.sql
   ```

3. Gere chave de criptografia:
   ```bash
   openssl rand -base64 32
   ```

## Execução

### Worker
```bash
go run cmd/worker/main.go
```

### API
```bash
go run cmd/api/main.go
```

### Build
```bash
go build -o bin/worker cmd/worker/main.go
go build -o bin/api cmd/api/main.go
```

## Adicionar Token

```bash
./scripts/add_token.sh "USER_TOKEN" "OWNER_DISCORD_ID" "ADMIN_KEY"
```

Ou via API:
```bash
curl -X POST http://localhost:8080/api/v1/admin/tokens \
  -H "Content-Type: application/json" \
  -H "X-Admin-Key: YOUR_ADMIN_KEY" \
  -d '{"token": "USER_TOKEN", "owner_user_id": "DISCORD_ID"}'
```

## Endpoints da API

- `GET /api/v1/profile/:discord_id` - Histórico completo do usuário
- `GET /api/v1/search?q=...&limit=...&offset=...` - Busca de usuários
- `GET /api/v1/alt-check/:discord_id` - Detecção de contas alternativas
- `GET /api/v1/health` - Health check
- `POST /api/v1/admin/tokens` - Adicionar token (requer X-Admin-Key)

## Estrutura

- `cmd/worker/` - Worker principal (Gateway + Event Processing)
- `cmd/api/` - Servidor API REST
- `internal/discord/` - Gateway WebSocket, TokenManager, Scraper
- `internal/processor/` - Processamento de eventos, Alt Detector
- `internal/storage/` - Upload de avatares (R2/S3)
- `internal/api/` - Handlers e middlewares da API
- `migrations/` - Migrações SQL

## Segurança

- Tokens criptografados com AES-256-GCM
- Rate limiting por IP/endpoint
- Input validation e sanitização
- Prepared statements (SQL injection prevention)
- CORS configurável

