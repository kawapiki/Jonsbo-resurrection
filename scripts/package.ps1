param([ValidatePattern('^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$')][string]$Version)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
if (-not $Version) { $Version = (Get-Content (Join-Path $projectRoot 'VERSION') -Raw).Trim() }
if ($Version -notmatch '^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$') { throw 'Invalid semantic version.' }
if ($Version -ne (Get-Content (Join-Path $projectRoot 'VERSION') -Raw).Trim()) { throw 'Package version must match VERSION.' }
$binary = Join-Path $projectRoot 'bin/jonsbo.exe'
if (-not (Test-Path -LiteralPath $binary)) { throw 'Run scripts/build.ps1 first.' }
foreach ($exe in @('bin/jonsbo.exe','bin/JonsboResurrection.exe')) {
    $metadata = (Get-Item -LiteralPath (Join-Path $projectRoot $exe)).VersionInfo
    if ($metadata.ProductVersion -ne $Version) { throw "Rebuild $exe for version $Version before packaging." }
}
$dist = Join-Path $projectRoot 'dist'
New-Item -ItemType Directory -Force -Path $dist | Out-Null
$name = "JonsboResurrection-$Version-windows-amd64"
$archive = Join-Path $dist "$name.zip"
if (Test-Path -LiteralPath $archive) { throw "Archive already exists: $archive" }
$stage = Join-Path $dist ($name + '-' + [guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $stage | Out-Null
$files = @('bin/jonsbo.exe', 'bin/JonsboResurrection.exe', 'VERSION', 'LICENSE', 'README.md', 'CONTRIBUTING.md', 'SECURITY.md', 'THIRD_PARTY_NOTICES.md', 'docs/api.md', 'docs/modules.md', 'docs/architecture.md', 'docs/hardware-monitoring.md', 'docs/releasing.md', 'docs/windows-app.md', 'configs/example.json', 'scripts/start-server.ps1', 'scripts/start-monitor.ps1', 'scripts/stop-monitor.ps1', 'third_party/go/LICENSE', 'third_party/pawnio/COPYING', 'third_party/pawnio/README.md', 'third_party/pawnio/PawnIO.Modules-0.2.11-source.zip')
foreach ($relative in $files) {
    $destination = if ($relative -eq 'bin/JonsboResurrection.exe') { 'JonsboResurrection.exe' } else { $relative }
    $target = Join-Path $stage $destination
    New-Item -ItemType Directory -Force -Path (Split-Path $target -Parent) | Out-Null
    Copy-Item -LiteralPath (Join-Path $projectRoot $relative) -Destination $target
}
Compress-Archive -Path (Join-Path $stage '*') -DestinationPath $archive
$hash = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
Set-Content -LiteralPath ($archive + '.sha256') -Value ($hash + '  ' + [IO.Path]::GetFileName($archive)) -Encoding ascii
Write-Output "Created $archive"
Write-Output "SHA256 $hash"
Write-Output "Staging files retained at $stage for inspection."
