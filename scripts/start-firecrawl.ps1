$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
Push-Location $repo
try {
    docker info --format '{{.ServerVersion}}'
    if ($LASTEXITCODE -ne 0) { throw 'Start Docker Desktop first.' }
    docker compose --parallel 1 -f infra/firecrawl/compose.yaml up -d --quiet-pull
    if ($LASTEXITCODE -ne 0) { throw 'Firecrawl startup failed.' }
    Write-Output 'Firecrawl API: http://127.0.0.1:3002 (startup can take a minute).'
    Write-Output 'Run python scripts/verify_local_firecrawl.py to check real scraping.'
} finally { Pop-Location }
