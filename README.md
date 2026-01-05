# Identity Archive Tracker

Sistema OSINT completo para rastreamento de identidades Discord com histórico detalhado, detecção de contas alternativas, monitoramento de presença e atividades em tempo real.

## Descrição

O Identity Archive é uma plataforma de rastreamento que coleta e armazena dados históricos de usuários Discord através de múltiplas fontes:

- **Gateway WebSocket**: Conexão direta com o Discord Gateway via user tokens para receber eventos em tempo real
- **Discord API**: Busca on-demand de perfis via API oficial
- **Fontes Públicas**: Scraping de sites públicos como discord.id, discordlookup.com, lantern.rest, japi.rest
- **Scraping de Servidores**: Coleta de membros, mensagens e atividades de servidores compartilhados

## Funcionalidades Principais

### Rastreamento de Dados
- Histórico completo de mudanças de username, discriminator e global name
- Histórico de avatares com upload para R2/S3
- Histórico de biografias e banners
- Histórico de nicknames por servidor
- Rastreamento de presença (online, offline, idle, dnd)
- Rastreamento de atividades (jogos, streaming, Spotify, etc)
- Histórico de mensagens em servidores compartilhados
- Histórico de conexões de voz com duração e metadados
- Rastreamento de parceiros de voz frequentes
- Histórico de decorações de avatar e clãs

### Detecção e Análise
- Detecção automática de contas alternativas (alts) baseada em múltiplos fatores
- Busca avançada com suporte a fuzzy matching (pg_trgm)
- Análise de relacionamentos entre contas
- Timeline completa de eventos por usuário

### Infraestrutura
- Processamento assíncrono de eventos com workers configuráveis
- Cache Redis para otimização de queries
- Rate limiting inteligente para evitar detecção
- Suporte a múltiplos tokens simultâneos
- Criptografia AES-256-GCM para tokens armazenados
- Fingerprinting de tokens para detecção de duplicatas

## Arquitetura

O sistema é composto por dois componentes principais:

### 1. Worker (`cmd/worker/main.go`)
- Conecta user tokens ao Discord Gateway via WebSocket
- Processa eventos em tempo real (PRESENCE_UPDATE, MESSAGE_CREATE, etc)
- Executa scraping inicial de servidores
- Processa eventos de voz, presença e atividades
- Job de detecção de alts em background
- Job de retry de upload de avatares

### 2. API (`main.go` - modo unificado)
- Servidor HTTP REST para consulta de dados
- Endpoints de busca e perfil
- Painel administrativo para gerenciamento de tokens
- Refresh on-demand de dados via Discord API
- Integração com Gateway Manager para dados em tempo real

### Frontend (`web/`)
- Interface web moderna construída com Next.js 16
- Visualização de perfis com timeline interativa
- Busca de usuários
- Visualização de histórico completo (username, avatar, bio, presença, atividades, mensagens, voz)
- Painel administrativo para gerenciamento de tokens

## Requisitos

### Backend
- Go 1.24+
- PostgreSQL 16+ com extensão `pg_trgm`
- Redis 7+
- Docker e Docker Compose (para desenvolvimento local)

### Frontend
- Node.js 20+
- npm ou yarn

### Opcional
- Cloudflare R2 ou AWS S3 (para armazenamento de avatares)
- Bot Token do Discord (para busca de usuários sem user token)

## Instalação

### 1. Clone o repositório

```bash
git clone <repository-url>
cd tracker
```

### 2. Configure o banco de dados

O projeto inclui um `docker-compose.yml` para facilitar o desenvolvimento local:

```bash
docker-compose up -d
```

Isso iniciará:
- PostgreSQL na porta 5433
- Redis na porta 6379

### 3. Execute as migrações

As migrações são executadas automaticamente pelo script `start.ps1`, mas você pode executá-las manualmente:

```bash
# Windows (PowerShell)
.\run_migration.ps1

# Linux/Mac
for f in migrations/*.sql; do
  psql $DB_DSN -f "$f"
done
```

### 4. Configure variáveis de ambiente

Crie um arquivo `.env` na raiz do projeto:

```env
# Banco de dados
DB_DSN=postgres://postgres:postgres@127.0.0.1:5433/tracker?sslmode=disable
REDIS_DSN=redis://127.0.0.1:6379/0

# Segurança
ENCRYPTION_KEY=<gerar com: openssl rand -base64 32>
ADMIN_SECRET_KEY=<sua-chave-secreta-admin>
CORS_ORIGINS=http://localhost:3000

# Discord
BOT_TOKEN=<token-do-bot-opcional>

# Performance
EVENT_WORKER_COUNT=25
DISCORD_ENABLE_GUILD_SUBSCRIPTIONS=true
DISCORD_REQUEST_MEMBER_PRESENCES=false
DISCORD_SCRAPE_INITIAL_GUILD_MEMBERS=true
DISCORD_MAX_CONCURRENT_GUILD_SCRAPES=1
DISCORD_SCRAPE_QUERY_DELAY_MS=350

# Storage (opcional)
R2_ENDPOINT=<endpoint-r2>
R2_BUCKET=<bucket-name>
R2_KEYS={"access_key_id":"...","secret_access_key":"...","public_url":"..."}

# Servidor
HTTP_ADDR=:8080
LOG_LEVEL=info
```

### 5. Gere chave de criptografia

```bash
openssl rand -base64 32
```

Copie o resultado para `ENCRYPTION_KEY` no `.env`.

## Execução

### Modo Desenvolvimento (Windows)

O script `start.ps1` automatiza todo o processo:

```powershell
.\start.ps1
```

O script:
1. Verifica se o Docker está rodando
2. Inicia os containers (PostgreSQL + Redis)
3. Aguarda o banco ficar pronto
4. Executa todas as migrações automaticamente
5. Inicia a API na porta 8080 (ou porta especificada)

### Modo Desenvolvimento (Linux/Mac)

```bash
# Iniciar containers
docker-compose up -d

# Executar migrações
for f in migrations/*.sql; do
  psql $DB_DSN -f "$f"
done

# Iniciar API
go run main.go
```

### Frontend

```bash
cd web
npm install
npm run dev
```

O frontend estará disponível em `http://localhost:3000`.

### Build de Produção

```bash
# Backend
go build -o bin/api main.go

# Frontend
cd web
npm run build
npm start
```

## Adicionar Tokens

### Via Script (Linux/Mac)

```bash
./scripts/add_token.sh "USER_TOKEN" "OWNER_DISCORD_ID" "ADMIN_KEY"
```

### Via API

```bash
curl -X POST http://localhost:8080/api/v1/admin/tokens \
  -H "Content-Type: application/json" \
  -H "X-Admin-Key: YOUR_ADMIN_KEY" \
  -d '{
    "token": "USER_TOKEN",
    "owner_user_id": "DISCORD_ID"
  }'
```

### Via Painel Admin

Acesse `http://localhost:3000/admin` e use a chave admin configurada.

## API Endpoints

### Públicos

#### `GET /api/v1/profile/:discord_id`
Retorna o perfil completo de um usuário com todo o histórico.

**Query Parameters:**
- `refresh` (opcional): Se `true`, força atualização dos dados via Discord API

**Resposta:**
```json
{
  "user": {
    "id": "123456789",
    "status": "active",
    "created_at": "2024-01-01T00:00:00Z"
  },
  "user_history": [...],
  "username_history": [...],
  "avatar_history": [...],
  "bio_history": [...],
  "connections": [...],
  "guilds": [...],
  "voice_history": [...],
  "presence_history": [...],
  "activity_history": [...],
  "messages": [...],
  "voice_partners": [...],
  "banner_history": [...],
  "clan_history": [...],
  "avatar_decoration_history": [...]
}
```

#### `GET /api/v1/search?q=...&limit=...&offset=...`
Busca usuários por username, global name ou nickname.

**Query Parameters:**
- `q` (obrigatório): Termo de busca
- `limit` (opcional, padrão: 50): Número de resultados
- `offset` (opcional, padrão: 0): Offset para paginação

#### `GET /api/v1/alt-check/:discord_id`
Detecta contas alternativas relacionadas.

**Resposta:**
```json
{
  "related_ids": ["123", "456"],
  "confidence": 0.85
}
```

#### `GET /api/v1/health`
Health check do serviço.

### Administrativos (requer `X-Admin-Key` header)

#### `POST /api/v1/admin/tokens`
Adiciona um novo token.

**Body:**
```json
{
  "token": "USER_TOKEN",
  "owner_user_id": "DISCORD_ID"
}
```

#### `GET /api/v1/admin/tokens`
Lista todos os tokens.

#### `GET /api/v1/admin/tokens/:id`
Obtém detalhes de um token específico.

#### `DELETE /api/v1/admin/tokens/:id`
Remove um token.

## Estrutura do Projeto

```
tracker/
├── cmd/
│   ├── api/          # Servidor API (legado, não usado)
│   └── worker/       # Worker standalone (legado, não usado)
├── internal/
│   ├── api/          # Handlers e middlewares da API REST
│   ├── config/       # Configuração e carregamento de env vars
│   ├── db/           # Conexão e pool do PostgreSQL
│   ├── discord/      # Gateway, TokenManager, Scraper, UserFetcher
│   ├── external/     # Fontes públicas de dados (scraping)
│   ├── logging/      # Sistema de logging estruturado
│   ├── models/       # Modelos de dados
│   ├── processor/    # Processamento de eventos, Alt Detector
│   ├── redis/        # Cliente Redis
│   ├── security/     # Criptografia, rate limiting, snowflake parsing
│   └── storage/      # Upload de avatares (R2/S3)
├── migrations/       # Migrações SQL do banco de dados
├── web/             # Frontend Next.js
│   ├── src/
│   │   ├── app/     # Páginas e rotas
│   │   ├── components/  # Componentes React
│   │   └── lib/     # Utilitários e tipos
├── main.go          # Ponto de entrada principal (API + Worker unificado)
├── start.ps1        # Script de inicialização (Windows)
├── stop.ps1         # Script de parada
├── docker-compose.yml
└── README.md
```

## Fluxo de Dados

### Coleta de Dados

1. **Gateway Connection**: Tokens são conectados ao Discord Gateway via WebSocket
2. **Event Processing**: Eventos são recebidos e enfileirados no Redis
3. **Worker Processing**: Workers processam eventos assincronamente
4. **Database Storage**: Dados são persistidos no PostgreSQL
5. **Cache**: Respostas de perfil são cacheadas no Redis

### Atualização de Dados "Fresh"

1. **On-Demand Refresh**: Quando `?refresh=1` é passado na API, o sistema:
   - Tenta buscar dados atualizados via Discord API (User Token ou Bot Token)
   - Se falhar, tenta usar dados do Gateway em tempo real
   - Atualiza o cache Redis

2. **Background Jobs**:
   - `PublicCollectorJob`: Atualiza dados de usuários conhecidos a cada 24h via fontes públicas
   - `AltDetectorJob`: Detecta relacionamentos entre contas em background
   - `AvatarRetryJob`: Retenta upload de avatares que falharam

### Rastreamento de Presença e Atividade

- **PRESENCE_UPDATE**: Eventos de presença são capturados via Gateway e armazenados em `presence_history`
- **ACTIVITY_UPDATE**: Atividades (jogos, streaming, etc) são rastreadas e armazenadas em `activity_history`
- Dados são atualizados em tempo real quando a conta está sendo trackeada via Gateway

## Segurança

### Criptografia
- Tokens são criptografados com AES-256-GCM antes de serem armazenados
- Chave de criptografia deve ter exatamente 32 bytes (256 bits)

### Rate Limiting
- Rate limiting por IP e endpoint
- Delays configuráveis entre requisições de scraping
- Controle de concorrência para evitar rate limits do Discord

### Validação
- Validação de Snowflake IDs (Discord IDs)
- Sanitização de inputs
- Prepared statements para prevenir SQL injection
- CORS configurável

### Token Management
- Fingerprinting de tokens para detectar duplicatas
- Status de tokens (ativo, banido, suspenso)
- Rastreamento de owner de cada token

## Troubleshooting

### Docker não está rodando
```powershell
# Verificar status
docker ps

# Iniciar Docker Desktop
```

### Porta já em uso
```powershell
# Verificar processo usando a porta
netstat -ano | findstr :8080

# Matar processo (substitua PID)
taskkill /PID <PID> /F
```

### Migrações falhando
```powershell
# Verificar logs do container
docker logs identityarchive-postgres

# Executar migrações manualmente
docker exec -i identityarchive-postgres psql -U postgres -d tracker < migrations/001_extend_tokens.sql
```

### Tokens não conectando
- Verifique se o token é válido
- Verifique logs para erros de autenticação
- Tokens podem estar banidos ou invalidados pelo Discord

### Rate Limits
- Ajuste `DISCORD_SCRAPE_QUERY_DELAY_MS` para valores maiores
- Reduza `DISCORD_MAX_CONCURRENT_GUILD_SCRAPES`
- Desabilite `DISCORD_REQUEST_MEMBER_PRESENCES` se necessário

## Desenvolvimento

### Adicionar Nova Migration

1. Crie arquivo em `migrations/` com formato `XXX_description.sql`
2. Execute via `run_migration.ps1` ou manualmente
3. Teste a migration em ambiente de desenvolvimento

### Estrutura de Eventos

Eventos do Discord Gateway são processados em `internal/processor/event_handlers.go`:
- `PRESENCE_UPDATE`: Atualiza presença do usuário
- `MESSAGE_CREATE`: Armazena mensagens
- `VOICE_STATE_UPDATE`: Rastreia conexões de voz
- `USER_UPDATE`: Atualiza dados do perfil
- `GUILD_MEMBER_UPDATE`: Atualiza nicknames e dados de membro

### Adicionar Nova Fonte de Dados

1. Implemente interface `external.Source` em `internal/external/sources.go`
2. Registre a fonte em `external.NewSourceManager()`
3. A fonte será usada automaticamente pelo `PublicCollectorJob`

## Licença

[Especificar licença]

## Contribuindo

[Instruções de contribuição]
