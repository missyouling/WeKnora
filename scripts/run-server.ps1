# run-server.ps1 - Load .env and start WeKnora backend (server.exe)
# Usage: powershell -ExecutionPolicy Bypass -File scripts\run-server.ps1
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot

# Load .env
$envFile = Join-Path $root ".env"
if (-not (Test-Path $envFile)) { Write-Error ".env not found: $envFile"; exit 1 }
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

if ($env:DB_PASSWORD -eq "__SUPABASE_DB_PASSWORD__" -or [string]::IsNullOrEmpty($env:DB_PASSWORD)) {
    Write-Host "[run-server] ERROR: set DB_PASSWORD in .env first" -ForegroundColor Red
    exit 1
}

Write-Host "[run-server] Starting WeKnora server.exe (DB: $env:DB_HOST:$env:DB_PORT/$env:DB_NAME driver=$env:DB_DRIVER)" -ForegroundColor Cyan
Push-Location $root
try {
    & (Join-Path $root "server.exe")
} finally {
    Pop-Location
}
