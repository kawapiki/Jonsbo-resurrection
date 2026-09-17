param([switch]$Elevated)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$exe = Join-Path $projectRoot 'bin/jonsbo.exe'
if (-not (Test-Path -LiteralPath $exe)) { throw 'Run scripts/build.ps1 first.' }
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
$log = Join-Path $projectRoot "bin/monitor-$stamp.log"
$arguments = @('monitor', '--all', '--log-file', ('"' + $log + '"'))
$launch = @{FilePath=$exe; ArgumentList=$arguments; WorkingDirectory=$projectRoot; WindowStyle='Hidden'; PassThru=$true}
if ($Elevated) { $launch.Verb = 'RunAs' }
$process = Start-Process @launch
if ($process.WaitForExit(750)) {
    $detail = Get-Content -LiteralPath $log -Raw -ErrorAction SilentlyContinue
    throw "Monitor exited during startup (exit $($process.ExitCode)): $detail"
}
Write-Output "Started monitor process $($process.Id). Stop with scripts/stop-monitor.ps1."
Write-Output "Log: $log"
if (-not $Elevated) { Write-Output 'CPU temperature requires administrator access. Use -Elevated for all readings.' }
