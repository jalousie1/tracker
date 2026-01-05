# script para iniciar o sistema completo do tracker
# uso: .\start.ps1

param(
    [int]$ApiPort = 8080
)

$ErrorActionPreference = "Continue"

try {
    Write-Host "=== Identity Archive Tracker ===" -ForegroundColor Cyan

    # ler ADMIN_SECRET_KEY e BOT_TOKEN do .env se existir
    $adminKey = "admin123"
    $botToken = ""
    if (Test-Path ".\.env") {
        foreach ($line in (Get-Content ".\.env" -ErrorAction SilentlyContinue)) {
            $t = $line.Trim()
            # ignorar comentarios e linhas vazias
            if ($t.StartsWith('#') -or [string]::IsNullOrWhiteSpace($t)) { continue }
            
            if ($t -match "^ADMIN_SECRET_KEY\s*=\s*(.+)$") {
                $val = $Matches[1].Trim()
                # remover aspas
                if (($val.StartsWith('"') -and $val.EndsWith('"')) -or ($val.StartsWith("'") -and $val.EndsWith("'"))) {
                    $val = $val.Substring(1, $val.Length - 2)
                }
                if (-not [string]::IsNullOrWhiteSpace($val)) {
                    $adminKey = $val
                }
            }
            if ($t -match "^BOT_TOKEN\s*=\s*(.+)$") {
                $val = $Matches[1].Trim()
                # remover aspas
                if (($val.StartsWith('"') -and $val.EndsWith('"')) -or ($val.StartsWith("'") -and $val.EndsWith("'"))) {
                    $val = $val.Substring(1, $val.Length - 2)
                }
                if (-not [string]::IsNullOrWhiteSpace($val)) {
                    $botToken = $val
                }
            }
        }
    }

    # verificar se docker esta rodando
    Write-Host "[0/4] Verificando Docker..." -ForegroundColor Yellow
    cmd /c "docker info" 1>$null 2>$null
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERRO] Docker nao esta rodando. Inicie o Docker Desktop primeiro." -ForegroundColor Red
        Write-Host "Pressione qualquer tecla para sair..."
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        exit 1
    }
    Write-Host "Docker OK" -ForegroundColor Green

    Write-Host "[1/4] Iniciando containers (postgres + redis)..." -ForegroundColor Yellow
    cmd /c "docker-compose up -d"
    if ($LASTEXITCODE -ne 0) {
        Write-Host "[ERRO] Falha ao iniciar containers." -ForegroundColor Red
        Write-Host "Pressione qualquer tecla para sair..."
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        exit 1
    }

    # aguardar postgres ficar pronto
    Write-Host "[2/4] Aguardando banco de dados..." -ForegroundColor Yellow
    $maxRetries = 30
    $retries = 0
    do {
        Start-Sleep -Seconds 1
        $retries++
        Write-Host "  Tentativa $retries/$maxRetries..." -ForegroundColor Gray
        cmd /c "docker exec identityarchive-postgres pg_isready -U postgres -d tracker" 1>$null 2>$null
    } while ($LASTEXITCODE -ne 0 -and $retries -lt $maxRetries)

    if ($retries -ge $maxRetries) {
        Write-Host "[ERRO] Banco de dados nao iniciou a tempo." -ForegroundColor Red
        Write-Host "Pressione qualquer tecla para sair..."
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        exit 1
    }
    Write-Host "Banco de dados OK" -ForegroundColor Green

    Write-Host "[3/4] Executando migrations..." -ForegroundColor Yellow
    $migrationFiles = Get-ChildItem -Path ".\\migrations" -Filter "*.sql" -File |
        Where-Object { $_.Name -match '^\d{3}_.+\.sql$' } |
        Sort-Object Name

    if (-not $migrationFiles -or $migrationFiles.Count -eq 0) {
        Write-Host "[ERRO] Nenhuma migration encontrada em .\\migrations" -ForegroundColor Red
        Write-Host "Pressione qualquer tecla para sair..."
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        exit 1
    }

    foreach ($mf in $migrationFiles) {
        $containerPath = "/migrations/$($mf.Name)"
        cmd /c "docker exec identityarchive-postgres psql -U postgres -d tracker -f $containerPath" 1>$null 2>$null
        if ($LASTEXITCODE -ne 0) {
            Write-Host "[ERRO] Falha ao executar migration: $($mf.Name)" -ForegroundColor Red
            Write-Host "Pressione qualquer tecla para sair..."
            $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
            exit 1
        }
    }

    Write-Host "Migrations OK" -ForegroundColor Green

    Write-Host "[4/4] Iniciando API..." -ForegroundColor Yellow

    # checar se a porta ja esta em uso
    $existing = Get-NetTCPConnection -LocalPort $ApiPort -State Listen -ErrorAction SilentlyContinue | Select-Object -First 1
    if ($existing) {
        $owningPid = $existing.OwningProcess
        $procName = ""
        try { $procName = (Get-Process -Id $owningPid -ErrorAction Stop).Name } catch { $procName = "pid_$owningPid" }

        Write-Host "[ERRO] A porta $ApiPort ja esta em uso (processo: $procName / PID $owningPid)." -ForegroundColor Red
        Write-Host "Feche a instancia anterior (Ctrl+C no terminal antigo) e rode de novo." -ForegroundColor Yellow
        Write-Host "Pressione qualquer tecla para sair..."
        $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
        exit 1
    }

    # configurar variaveis de ambiente (valores fixos pro Windows local)
    $env:DB_DSN = "postgres://postgres:postgres@127.0.0.1:5433/tracker?sslmode=disable"
    $env:REDIS_DSN = "redis://127.0.0.1:6379/0"
    $env:ENCRYPTION_KEY = "AQIDBAUGBwgJCgsMDQ4PEBESExQVFhcYGRobHB0eHyA="
    $env:ADMIN_SECRET_KEY = $adminKey
    $env:CORS_ORIGINS = "http://localhost:3000"
    $env:HTTP_ADDR = ":$ApiPort"
    $env:LOG_LEVEL = "info"

    # knobs de performance / ingestao (ajuste se bater rate limit)
    $env:EVENT_WORKER_COUNT = "25"
    $env:DISCORD_ENABLE_GUILD_SUBSCRIPTIONS = "true"
    # presences em chunk deixa o payload bem mais pesado e costuma causar 4008 (rate limited)
    $env:DISCORD_REQUEST_MEMBER_PRESENCES = "false"
    $env:DISCORD_SCRAPE_INITIAL_GUILD_MEMBERS = "true"
    # manter baixo para nao estourar rate limit
    $env:DISCORD_MAX_CONCURRENT_GUILD_SCRAPES = "1"
    $env:DISCORD_SCRAPE_QUERY_DELAY_MS = "350"
    
    # BOT_TOKEN do .env (se estiver configurado)
    if (-not [string]::IsNullOrWhiteSpace($botToken)) {
        $env:BOT_TOKEN = $botToken
        $tokenPreview = $botToken.Substring(0, [Math]::Min(20, $botToken.Length))
        Write-Host "Bot Token: Configurado ($tokenPreview...)" -ForegroundColor Green
    } else {
        Write-Host "Bot Token: Nao configurado (busca limitada a servidores compartilhados)" -ForegroundColor Yellow
    }

    Write-Host ""
    Write-Host "=== Sistema Pronto ===" -ForegroundColor Green
    Write-Host "API: http://localhost:$ApiPort" -ForegroundColor Cyan
    Write-Host "Admin Key: $adminKey" -ForegroundColor Magenta
    Write-Host ""
    Write-Host "Use essa chave no painel admin: $adminKey" -ForegroundColor Yellow
    Write-Host ""
    Write-Host "Pressione Ctrl+C para parar a API" -ForegroundColor Gray
    Write-Host ""

    # iniciar a api
    go run main.go

} catch {
    Write-Host "[ERRO] $($_.Exception.Message)" -ForegroundColor Red
    Write-Host $_.ScriptStackTrace -ForegroundColor Red
} finally {
    Write-Host ""
    Write-Host "API encerrada." -ForegroundColor Yellow
    Write-Host "Pressione qualquer tecla para sair..."
    $null = $Host.UI.RawUI.ReadKey("NoEcho,IncludeKeyDown")
}
