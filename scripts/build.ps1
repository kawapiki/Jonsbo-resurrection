param([ValidatePattern('^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$')][string]$Version)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
if (-not $Version) { $Version = (Get-Content (Join-Path $projectRoot 'VERSION') -Raw).Trim() }
if ($Version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') { throw 'Invalid semantic version.' }
$localGo = Join-Path $projectRoot '.tools/go/bin/go.exe'
$go = if (Test-Path $localGo) { $localGo } else { (Get-Command go -ErrorAction Stop).Source }
$oldCache, $oldToolchain, $oldBin, $oldPath = $env:GOCACHE, $env:GOTOOLCHAIN, $env:GOBIN, $env:GOPATH
Push-Location $projectRoot
try {
    $env:GOCACHE = Join-Path $projectRoot '.tools/gocache'
    $env:GOTOOLCHAIN = 'local'
    $env:GOBIN = Join-Path $projectRoot '.tools/bin'
    $env:GOPATH = Join-Path $projectRoot '.tools/gopath'
    $resourceTool = Join-Path $env:GOBIN 'go-winres.exe'
    if (-not (Test-Path -LiteralPath $resourceTool)) {
        & $go install github.com/tc-hib/go-winres@v0.3.3
        if ($LASTEXITCODE -ne 0) { throw 'Resource compiler installation failed.' }
    }
    & $resourceTool make --in assets/winres.json --arch amd64 --out cmd/jonsbo/rsrc --file-version $Version --product-version $Version
    if ($LASTEXITCODE -ne 0) { throw 'Windows resource generation failed.' }
    & $go test ./...
    if ($LASTEXITCODE -ne 0) { throw 'Tests failed.' }
    & $go vet ./...
    if ($LASTEXITCODE -ne 0) { throw 'Go vet failed.' }
    $revision = git rev-parse --short HEAD
    if ($LASTEXITCODE -ne 0) { $revision = 'unknown' }
    $flags = "-s -w -X main.version=$Version -X main.revision=$revision"
    & $go build -trimpath -ldflags $flags -o bin/jonsbo.exe ./cmd/jonsbo
    if ($LASTEXITCODE -ne 0) { throw 'Console build failed.' }
    & $go build -trimpath -ldflags "$flags -H windowsgui -X main.guiBuild=true" -o bin/JonsboResurrection.exe ./cmd/jonsbo
    if ($LASTEXITCODE -ne 0) { throw 'Desktop build failed.' }
    Write-Output "Built Windows desktop and CLI version $Version ($revision)."
} finally {
    $env:GOCACHE, $env:GOTOOLCHAIN, $env:GOBIN, $env:GOPATH = $oldCache, $oldToolchain, $oldBin, $oldPath
    Pop-Location
}
