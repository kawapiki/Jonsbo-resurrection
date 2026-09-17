$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$localGo = Join-Path $projectRoot '.tools/go/bin/go.exe'
$go = if (Test-Path $localGo) { $localGo } else { (Get-Command go -ErrorAction Stop).Source }
$previousCache = $env:GOCACHE
$previousToolchain = $env:GOTOOLCHAIN
Push-Location $projectRoot
try {
    $env:GOCACHE = Join-Path $projectRoot '.tools/gocache'
    $env:GOTOOLCHAIN = 'local'
    & $go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed.' }
    & $go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
    & $go build -trimpath -o bin/jonsbo.exe ./cmd/jonsbo
    if ($LASTEXITCODE -ne 0) { throw 'Build failed.' }
    Write-Output "Built $(Join-Path $projectRoot 'bin/jonsbo.exe')"
} finally {
    $env:GOCACHE = $previousCache
    $env:GOTOOLCHAIN = $previousToolchain
    Pop-Location
}
