# script para parar o sistema do tracker
# uso: .\stop.ps1
# para remover dados: .\stop.ps1 -RemoveData

param(
    [switch]$RemoveData,
    [int[]]$ApiPorts = @(8080, 8081)
)

Write-Host "=== Parando Identity Archive Tracker ===" -ForegroundColor Cyan

# parar api(s) rodando nas portas
foreach ($p in $ApiPorts) {
    try {
        $conns = Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue
        foreach ($c in $conns) {
            $owningPid = $c.OwningProcess
            if (-not $owningPid) { continue }

            $procName = "pid_$owningPid"
            try { $procName = (Get-Process -Id $owningPid -ErrorAction Stop).Name } catch {}

            Write-Host "Parando API na porta $p (processo: $procName / PID $owningPid)..." -ForegroundColor Yellow
            Stop-Process -Id $owningPid -Force -ErrorAction SilentlyContinue
        }
    } catch {
        # ignorar falhas de permissao/lookup
    }
}

# parar containers
if ($RemoveData) {
    Write-Host "Parando containers e removendo dados..." -ForegroundColor Yellow
    cmd /c "docker-compose down -v"
    Write-Host "Containers parados e dados removidos." -ForegroundColor Green
} else {
    Write-Host "Parando containers (dados preservados)..." -ForegroundColor Yellow
    cmd /c "docker-compose down"
    Write-Host "Containers parados." -ForegroundColor Green
}

Write-Host ""
Write-Host "Para iniciar novamente: .\start.ps1" -ForegroundColor Cyan

