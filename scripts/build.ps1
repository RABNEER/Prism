$ErrorActionPreference = 'Stop'
$repo = Split-Path -Parent $PSScriptRoot
Push-Location $repo
try {
    $env:GOMODCACHE = Join-Path $repo '.cache\gomod'
    $env:GOCACHE = Join-Path $repo '.cache\go-build'
    $env:CARGO_HOME = Join-Path $repo '.cache\cargo'
    $env:CARGO_TARGET_DIR = Join-Path $repo '.cache\cargo-target'
    cargo build --release --workspace --locked
    if ($LASTEXITCODE -ne 0) { throw 'Rust build failed' }
    New-Item -ItemType Directory -Force -Path (Join-Path $repo 'bin') | Out-Null
    Copy-Item -LiteralPath (Join-Path $env:CARGO_TARGET_DIR 'release\prism-matching.exe') -Destination (Join-Path $repo 'bin\prism-matching.exe')
    $env:PRISM_MATCHING_WORKER = Join-Path $repo 'bin\prism-matching.exe'
    cargo test --workspace --locked
    if ($LASTEXITCODE -ne 0) { throw 'Rust tests failed' }
    go test -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go tests failed' }
    go build -o bin/prism-mcp.exe ./cmd/prism-mcp
    if ($LASTEXITCODE -ne 0) { throw 'MCP build failed' }
    go build -o bin/prism-admin.exe ./cmd/prism-admin
    if ($LASTEXITCODE -ne 0) { throw 'Admin build failed' }
} finally { Pop-Location }
