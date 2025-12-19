# script para verificar se as migrations foram aplicadas corretamente
# uso: .\verify_migrations.ps1

Write-Host "=== Verificando Migrations ===" -ForegroundColor Cyan

$containerName = "identityarchive-postgres"

# verificar se container esta rodando
$container = docker ps --filter "name=$containerName" --format "{{.Names}}" 2>$null
if (-not $container) {
    Write-Host "[ERRO] Container $containerName nao esta rodando." -ForegroundColor Red
    Write-Host "Execute primeiro: .\start.ps1" -ForegroundColor Yellow
    exit 1
}

Write-Host "Container encontrado: $container" -ForegroundColor Green
Write-Host ""

# lista de tabelas esperadas
$tables = @(
    "users",
    "tokens",
    "token_failures",
    "user_history",
    "username_history",
    "avatar_history",
    "bio_history",
    "connected_accounts",
    "guilds",
    "alt_relationships",
    "guild_members",
    "token_guilds",
    "voice_sessions",
    "voice_participants",
    "voice_stats",
    "presence_history",
    "activity_history",
    "banner_history",
    "nickname_history",
    "clan_history",
    "message_stats",
    "avatar_decoration_history"
)

Write-Host "Verificando tabelas..." -ForegroundColor Yellow

$missing = @()
$found = @()

foreach ($table in $tables) {
    $result = docker exec $containerName psql -U postgres -d tracker -t -c "SELECT EXISTS (SELECT FROM information_schema.tables WHERE table_name = '$table');" 2>$null
    $exists = $result.Trim() -eq "t"
    
    if ($exists) {
        $found += $table
        Write-Host "  [OK] $table" -ForegroundColor Green
    } else {
        $missing += $table
        Write-Host "  [FALTA] $table" -ForegroundColor Red
    }
}

Write-Host ""

# verificar colunas da tabela users
Write-Host "Verificando colunas da tabela users..." -ForegroundColor Yellow

$userColumns = @(
    "id", "status", "created_at", "last_updated_at",
    "public_data_source", "last_public_fetch", "banner_hash", "banner_url",
    "accent_color", "premium_type", "public_flags", "bot", "is_system",
    "mfa_enabled", "locale", "verified", "email", "flags",
    "avatar_decoration_data", "clan_data"
)

$missingCols = @()
foreach ($col in $userColumns) {
    $result = docker exec $containerName psql -U postgres -d tracker -t -c "SELECT EXISTS (SELECT FROM information_schema.columns WHERE table_name = 'users' AND column_name = '$col');" 2>$null
    $exists = $result.Trim() -eq "t"
    
    if ($exists) {
        Write-Host "  [OK] users.$col" -ForegroundColor Green
    } else {
        $missingCols += $col
        Write-Host "  [FALTA] users.$col" -ForegroundColor Red
    }
}

Write-Host ""

# resumo
Write-Host "=== Resumo ===" -ForegroundColor Cyan
Write-Host "Tabelas encontradas: $($found.Count)/$($tables.Count)" -ForegroundColor $(if ($found.Count -eq $tables.Count) { "Green" } else { "Yellow" })

if ($missing.Count -gt 0) {
    Write-Host "Tabelas faltando: $($missing -join ', ')" -ForegroundColor Red
    Write-Host ""
    Write-Host "Execute as migrations manualmente:" -ForegroundColor Yellow
    Write-Host "docker exec $containerName psql -U postgres -d tracker -f /migrations/005_add_guild_members.sql" -ForegroundColor Gray
    Write-Host "docker exec $containerName psql -U postgres -d tracker -f /migrations/006_complete_tracking.sql" -ForegroundColor Gray
}

if ($missingCols.Count -gt 0) {
    Write-Host "Colunas faltando em users: $($missingCols -join ', ')" -ForegroundColor Red
}

if ($missing.Count -eq 0 -and $missingCols.Count -eq 0) {
    Write-Host ""
    Write-Host "[OK] Todas as migrations foram aplicadas corretamente!" -ForegroundColor Green
    
    # mostrar contagem de registros
    Write-Host ""
    Write-Host "=== Contagem de Registros ===" -ForegroundColor Cyan
    
    $countTables = @("users", "username_history", "avatar_history", "guild_members", "voice_sessions", "presence_history", "activity_history", "message_stats")
    foreach ($table in $countTables) {
        $count = docker exec $containerName psql -U postgres -d tracker -t -c "SELECT COUNT(*) FROM $table;" 2>$null
        $count = $count.Trim()
        Write-Host "  $table`: $count registros" -ForegroundColor Gray
    }
}

Write-Host ""

