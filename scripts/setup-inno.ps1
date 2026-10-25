param()
$ErrorActionPreference='Stop'
$taskRoot=Split-Path $PSScriptRoot -Parent
$taskCompiler=Join-Path $taskRoot '.tools/innosetup-6.7.3/ISCC.exe'
if (Test-Path -LiteralPath $taskCompiler) { return $taskCompiler }
$taskDownload=Join-Path $taskRoot '.tools/innosetup-6.7.3.exe'
New-Item -ItemType Directory -Force (Split-Path $taskDownload -Parent) | Out-Null
Invoke-WebRequest 'https://github.com/jrsoftware/issrc/releases/download/is-6_7_3/innosetup-6.7.3.exe' -OutFile $taskDownload
$taskHash=(Get-FileHash -LiteralPath $taskDownload -Algorithm SHA256).Hash.ToLowerInvariant()
if ($taskHash -ne '9c73c3bae7ed48d44112a0f48e66742c00090bdb5bef71d9d3c056c66e97b732') { throw 'Inno Setup compiler checksum mismatch.' }
$taskDirectory=Split-Path $taskCompiler -Parent
$taskProcess=Start-Process -FilePath $taskDownload -ArgumentList @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART','/CURRENTUSER','/NOICONS','/TASKS=""',('/DIR="'+$taskDirectory+'"')) -WindowStyle Hidden -Wait -PassThru
if ($taskProcess.ExitCode -ne 0 -or !(Test-Path -LiteralPath $taskCompiler)) { throw 'Inno Setup compiler installation failed.' }
return $taskCompiler
