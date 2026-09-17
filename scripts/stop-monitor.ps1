param([switch]$Server)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
$exe = Join-Path $projectRoot 'bin/jonsbo.exe'
$stopArguments = @('stop')
if ($Server) { $stopArguments += '--server' }
$p = Start-Process -FilePath $exe -ArgumentList $stopArguments -WindowStyle Hidden -PassThru
if (-not $p.WaitForExit(15000)) { throw 'Stop command has not exited; check the monitor status.' }
if ($p.ExitCode -eq 0) { Write-Output 'Monitor stop requested.'; return }
Write-Output 'Requesting administrator access to stop the elevated monitor.'
$p = Start-Process -FilePath $exe -ArgumentList $stopArguments -Verb RunAs -WindowStyle Hidden -PassThru
if (-not $p.WaitForExit(15000)) { throw 'Elevated stop command has not exited.' }
if ($p.ExitCode -ne 0) { throw 'No accessible running monitor was found.' }
Write-Output 'Monitor stop requested.'
