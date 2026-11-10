# Finish the already-authorized host-side browser-image download and install it.
$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
Push-Location $repo
try {
    $downloads = @(Get-CimInstance Win32_Process -Filter "Name = 'crane.exe'" | Where-Object { $_.CommandLine -like '*firecrawl-browser.tar*' })
    foreach ($download in $downloads) {
        Write-Output 'Waiting for the pinned browser-image download.'
        Wait-Process -Id $download.ProcessId -ErrorAction SilentlyContinue
    }
    $archive = Join-Path $repo '.cache\firecrawl-browser.tar'
    if (-not (Test-Path -LiteralPath $archive)) { throw 'Browser archive is absent; download the pinned image first.' }
    docker load -i $archive
    if ($LASTEXITCODE -ne 0) { throw 'Browser image import failed; the archive may be incomplete.' }
    docker pull ghcr.io/firecrawl/playwright-service@sha256:eacaf2c256a6ff7fd1869e470d2969e97a037ff6c75ac01d38784d138a33f8eb
    if ($LASTEXITCODE -ne 0) { throw 'Could not register the pinned browser image.' }
    docker compose --progress quiet -f infra/firecrawl/compose.yaml up -d --no-deps --pull never playwright-service
    if ($LASTEXITCODE -ne 0) { throw 'Browser service startup failed.' }
    $ready = $false
    for ($attempt = 0; $attempt -lt 30; $attempt++) {
        docker compose -f infra/firecrawl/compose.yaml exec -T api node -e 'fetch("http://playwright-service:3000/health",{signal:AbortSignal.timeout(2000)}).then(r=>process.exit(r.ok?0:1)).catch(()=>process.exit(1))'
        if ($LASTEXITCODE -eq 0) { $ready = $true; break }
        Start-Sleep -Seconds 2
    }
    if (-not $ready) { throw 'Browser service did not pass its health check.' }
    @{ browser_health_verified = $true; model_configured = $false; verified_at = (Get-Date).ToUniversalTime().ToString('o') } | ConvertTo-Json | Set-Content -LiteralPath .cache/firecrawl-browser-ready.json -Encoding utf8
    Write-Output 'Pinned browser service installed and healthy. Real listing layouts still require verification.'
} finally { Pop-Location }
