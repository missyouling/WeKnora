# start-all.ps1 - One-click start/stop/status for WeKnora dev services
# Usage:
#   powershell -ExecutionPolicy Bypass -File scripts\start-all.ps1            # start missing services
#   powershell -ExecutionPolicy Bypass -File scripts\start-all.ps1 start      # same as above
#   powershell -ExecutionPolicy Bypass -File scripts\start-all.ps1 stop       # stop services started by this script
#   powershell -ExecutionPolicy Bypass -File scripts\start-all.ps1 status     # show service status
# Double-click scripts\start-all.bat works too (start / stop / status as arg).

param(
    [ValidateSet("start", "stop", "status")]
    [string]$Action = "start"
)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$dataDir = Join-Path $root "data"
$pidFile = Join-Path $dataDir ".weknora-pids.json"

function Get-LogPath([string]$name) { Join-Path $dataDir ($name + ".log") }

function Test-PortListening([int]$port) {
    return [bool](Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue)
}

function Get-PidByPort([int]$port) {
    $c = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction SilentlyContinue
    if ($c) { return $c[0].OwningProcess }
    return $null
}

function Read-PidFile {
    if (Test-Path $pidFile) {
        try { return (Get-Content $pidFile -Raw | ConvertFrom-Json) } catch { return @{} }
    }
    return @{}
}

function Write-PidFile($obj) {
    $obj | ConvertTo-Json | Set-Content -Path $pidFile -Encoding UTF8
}

# ---- status ----
function Show-Status {
    Write-Host "`n[status] WeKnora dev services" -ForegroundColor Cyan
    $saved = Read-PidFile
    foreach ($svc in @("docreader", "server", "vite")) {
        $port = @{ docreader = 50051; server = 8088; vite = 5173 }[$svc]
        $procId = Get-PidByPort $port
        $savedPid = $saved.$svc
        if ($procId) {
            Write-Host ("  {0,-10} port {1,-6} RUNNING  pid={2}" -f $svc, $port, $procId) -ForegroundColor Green
        } else {
            $extra = if ($savedPid) { " (saved pid $savedPid not listening)" } else { "" }
            Write-Host ("  {0,-10} port {1,-6} STOPPED{2}" -f $svc, $port, $extra) -ForegroundColor Yellow
        }
    }
}

# ---- load .env into process env ----
function Load-EnvFile {
    $envFile = Join-Path $root ".env"
    if (-not (Test-Path $envFile)) { throw ".env not found: $envFile" }
    foreach ($line in Get-Content $envFile) {
        $line = $line.Trim()
        if ($line -eq "" -or $line.StartsWith("#")) { continue }
        if ($line -match "^([^=]+)=(.*)$") {
            $name = $Matches[1].Trim()
            $value = $Matches[2].Trim()
            if ($value.Length -ge 2 -and (($value[0] -eq '"' -and $value[-1] -eq '"') -or ($value[0] -eq "'" -and $value[-1] -eq "'"))) {
                $value = $value.Substring(1, $value.Length - 2)
            }
            [Environment]::SetEnvironmentVariable($name, $value, "Process")
        }
    }
}

# ---- wait for a port to listen ----
function Wait-Port([int]$port, [int]$timeoutSec, [string]$name) {
    $deadline = (Get-Date).AddSeconds($timeoutSec)
    while ((Get-Date) -lt $deadline) {
        if (Test-PortListening $port) { return $true }
        Start-Sleep -Seconds 2
    }
    if ($name -eq "vite") {
        # vite may take longer on first run; check log tail for readiness too
        $log = Get-LogPath "vite"
        if (Test-Path $log) {
            $tail = Get-Content $log -Tail 30 -ErrorAction SilentlyContinue | Out-String
            if ($tail -match "Local:|ready in") { return $true }
        }
    }
    return $false
}

# ---- start one service ----
function Start-One([string]$name) {
    $port = @{ docreader = 50051; server = 8088; vite = 5173 }[$name]
    if (Test-PortListening $port) {
        Write-Host ("[{0}] already running on port {1}, skip" -f $name, $port) -ForegroundColor DarkGray
        return $true
    }
    $log = Get-LogPath $name
    $proc = $null
    switch ($name) {
        "docreader" {
            $py = Join-Path $root "docreader\.venv\Scripts\python.exe"
            if (-not (Test-Path $py)) { throw "docreader venv not found: $py" }
            Write-Host "[docreader] starting (port 50051, log: data\docreader.log)" -ForegroundColor Cyan
            $cmd = "cd /d `"$root`" && set PYTHONPATH=$root && `"$py`" `"$root\docreader\main.py`" >> `"$log`" 2>&1"
            $proc = Start-Process -FilePath "cmd.exe" -ArgumentList "/c", $cmd -WindowStyle Hidden -PassThru
        }
        "server" {
            Load-EnvFile
            if ($env:DB_PASSWORD -eq "__SUPABASE_DB_PASSWORD__" -or [string]::IsNullOrEmpty($env:DB_PASSWORD)) {
                throw "DB_PASSWORD not set correctly in .env"
            }
            Write-Host "[server] starting (port 8088, log: data\server.out.log)" -ForegroundColor Cyan
            $exe = Join-Path $root "server.exe"
            if (-not (Test-Path $exe)) { throw "server.exe not found: $exe" }
            $proc = Start-Process -FilePath "cmd.exe" -ArgumentList "/c", "cd /d `"$root`" && `"$exe`" >> `"$log`" 2>&1" -WindowStyle Hidden -PassThru
        }
        "vite" {
            $nodeCmd = Get-Command node -ErrorAction SilentlyContinue
            if (-not $nodeCmd) { throw "node not found on PATH" }
            $nodeDir = Split-Path -Parent $nodeCmd.Source
            $env:PATH = "$nodeDir;$env:PATH"
            $env:NODE_OPTIONS = "--max-old-space-size=6144"
            $env:VITE_DEV_PROXY_TARGET = "http://localhost:8088"
            $front = Join-Path $root "frontend"
            if (-not (Test-Path (Join-Path $front "package.json"))) { throw "frontend/package.json not found" }
            Write-Host "[vite] starting (port 5173, log: data\vite.log)" -ForegroundColor Cyan
            $cmd = "cd /d `"$front`" && npm run dev >> `"$log`" 2>&1"
            $proc = Start-Process -FilePath "cmd.exe" -ArgumentList "/c", $cmd -WindowStyle Hidden -PassThru
        }
    }
    Start-Sleep -Seconds 3
    $ok = Wait-Port $port 60 $name
    if ($ok) {
        $procId = Get-PidByPort $port
        Write-Host ("[{0}] up on port {1} (pid {2})" -f $name, $port, $procId) -ForegroundColor Green
        return $procId
    } else {
        $tail = ""
        if (Test-Path $log) { $tail = (Get-Content $log -Tail 12 -ErrorAction SilentlyContinue | Out-String) }
        Write-Host ("[{0}] FAILED to listen on {1}. log tail:`n{2}" -f $name, $port, $tail) -ForegroundColor Red
        return $null
    }
}

# ---- stop one service by port (and saved pid) ----
function Stop-One([string]$name) {
    $port = @{ docreader = 50051; server = 8088; vite = 5173 }[$name]
    $saved = Read-PidFile
    $savedPid = $saved.$name
    $listener = Get-PidByPort $port
    $targets = @()
    if ($listener) { $targets += $listener }
    if ($savedPid -and $savedPid -ne $listener) { $targets += $savedPid }
    if ($targets.Count -eq 0) {
        Write-Host ("[{0}] not running" -f $name) -ForegroundColor DarkGray
        return
    }
    foreach ($t in ($targets | Select-Object -Unique)) {
        Stop-Process -Id $t -Force -ErrorAction SilentlyContinue
        Write-Host ("[{0}] stopped pid {1}" -f $name, $t) -ForegroundColor Yellow
    }
}

# ---- actions ----
if ($Action -eq "status") {
    Show-Status
    exit 0
}

if ($Action -eq "stop") {
    Write-Host "[stop] stopping WeKnora services" -ForegroundColor Cyan
    Stop-One "vite"
    Stop-One "docreader"
    Stop-One "server"
    if (Test-Path $pidFile) { Remove-Item $pidFile -Force }
    Start-Sleep -Seconds 2
    Show-Status
    exit 0
}

# ---- start (default) ----
Write-Host "[start] WeKnora one-click startup" -ForegroundColor Cyan
if (-not (Test-Path $dataDir)) { New-Item -ItemType Directory -Path $dataDir -Force | Out-Null }

$pids = Read-PidFile
$pids.docreader = Start-One "docreader"
$pids.server = Start-One "server"
$pids.vite = Start-One "vite"
Write-PidFile $pids

Write-Host "`n[start] done. URLs:" -ForegroundColor Cyan
Write-Host "  frontend : http://localhost:5173" -ForegroundColor Green
Write-Host "  backend  : http://localhost:8088" -ForegroundColor Green
Write-Host "  docreader: 127.0.0.1:50051 (gRPC)" -ForegroundColor Green
Write-Host "  logs     : data\docreader.log / data\server.out.log / data\vite.log"
Show-Status
