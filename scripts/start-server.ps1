param([switch]$Elevated, [switch]$Example, [switch]$Displays)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$exe = Join-Path $projectRoot 'bin/jonsbo.exe'
if (-not (Test-Path -LiteralPath $exe)) { throw 'Run scripts/build.ps1 first.' }
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss-fff'
$log = Join-Path $projectRoot "bin/server-$stamp.log"
$arguments = @('serve', '--log-file', ('"' + $log + '"'))
if ($Example) { $arguments += '--example' }
if ($Displays) { $arguments += '--all' }
$launch = @{FilePath=$exe; ArgumentList=$arguments; WorkingDirectory=$projectRoot; WindowStyle='Hidden'; PassThru=$true}
if ($Elevated) { $launch.Verb='RunAs' }
$p = Start-Process @launch
if ($p.WaitForExit(750)) {
    $detail = Get-Content -LiteralPath $log -Raw -ErrorAction SilentlyContinue
    throw "Server exited during startup (exit $($p.ExitCode)): $detail"
}
Write-Output "Started API process $($p.Id). Stop with scripts/stop-monitor.ps1 -Server."
Write-Output "Log: $log"
