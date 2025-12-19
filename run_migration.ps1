# Script para executar migration 004_add_public_data.sql
# Execute este script quando os containers Docker estiverem rodando

Write-Host "Executando migration 004_add_public_data.sql..." -ForegroundColor Cyan

$migrationFile = "migrations\004_add_public_data.sql"

if (-not (Test-Path $migrationFile)) {
    Write-Host "[ERRO] Arquivo de migration nao encontrado: $migrationFile" -ForegroundColor Red
    exit 1
}

# Verificar se container esta rodando
$containerName = "identityarchive-postgres"
$container = docker ps --filter "name=$containerName" --format "{{.Names}}" | Select-Object -First 1

if (-not $container) {
    Write-Host "[ERRO] Container $containerName nao esta rodando." -ForegroundColor Red
    Write-Host "Execute primeiro: .\start.ps1" -ForegroundColor Yellow
    exit 1
}

Write-Host "Container encontrado: $container" -ForegroundColor Green

# Executar migration
Get-Content $migrationFile | docker exec -i $containerName psql -U postgres -d tracker

if ($LASTEXITCODE -eq 0) {
    Write-Host "[OK] Migration executada com sucesso!" -ForegroundColor Green
} else {
    Write-Host "[ERRO] Falha ao executar migration" -ForegroundColor Red
    exit 1
}

