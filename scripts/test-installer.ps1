param([string]$Version=(Get-Content (Join-Path (Split-Path $PSScriptRoot -Parent) 'VERSION') -Raw).Trim())
$ErrorActionPreference='Stop'
# This integration test changes HKCU and installed files. Never run against a desktop user's live controller.
if ($env:GITHUB_ACTIONS -ne 'true') { throw 'Installer smoke tests require an isolated GitHub Actions Windows runner.' }
$taskRoot=Split-Path $PSScriptRoot -Parent
$taskSetup=Join-Path $taskRoot "dist/JonsboResurrection-$Version-Setup.exe"
$taskInstall=Join-Path $taskRoot '.tools/installer smoke/app'
$taskData=Join-Path $env:LOCALAPPDATA 'JonsboResurrection'
$taskRunKey='HKCU:\Software\Microsoft\Windows\CurrentVersion\Run'
$taskUninstallKey='HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\{51B2383D-E34E-4F8A-9BC2-B52D142D78B8}_is1'
if ((Test-Path -LiteralPath $taskData) -or (Test-Path -LiteralPath $taskUninstallKey)) { throw 'Runner already contains an application installation or data.' }
function Invoke-Checked([string]$File,[string[]]$Arguments) {
 $taskProcess=Start-Process -FilePath $File -ArgumentList $Arguments -WindowStyle Hidden -Wait -PassThru
 if ($taskProcess.ExitCode -ne 0) { throw "Process failed ($($taskProcess.ExitCode)): $File" }
}
function Install-App {
 Invoke-Checked $taskSetup @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART',('/DIR="'+$taskInstall+'"'))
 foreach ($taskFile in @('JonsboResurrection.exe','bin/jonsbo.exe','bin/jonsbo-ai-observer.exe','bin/jonsbo-ai-bridge.exe','docs/cyclops.md','unins000.exe')) {
  if (!(Test-Path -LiteralPath (Join-Path $taskInstall $taskFile))) { throw "Missing installed file: $taskFile" }
 }
 if ((Get-Item -LiteralPath (Join-Path $taskInstall 'JonsboResurrection.exe')).VersionInfo.ProductVersion -ne $Version) { throw 'Installed version mismatch.' }
 if (!(Test-Path -LiteralPath $taskUninstallKey)) { throw 'Missing Windows uninstall registration.' }
 $taskShortcut=Join-Path ([Environment]::GetFolderPath('Programs')) 'Jonsbo Resurrection/Jonsbo Resurrection.lnk'
 if (!(Test-Path -LiteralPath $taskShortcut)) { throw 'Missing Start menu shortcut.' }
}
function Uninstall-App {
 Invoke-Checked (Join-Path $taskInstall 'unins000.exe') @('/VERYSILENT','/SUPPRESSMSGBOXES','/NORESTART')
 if ((Test-Path -LiteralPath (Join-Path $taskInstall 'JonsboResurrection.exe')) -or (Test-Path -LiteralPath $taskUninstallKey)) { throw 'Uninstall left the application or registration.' }
 if ((Get-Content -LiteralPath (Join-Path $taskData 'preservation-test.txt') -Raw).Trim() -ne 'keep my layouts') { throw 'Uninstall removed user data.' }
}
Install-App
New-Item -ItemType Directory -Path $taskData | Out-Null
Set-Content -LiteralPath (Join-Path $taskData 'preservation-test.txt') 'keep my layouts'
# Simulate upgrading a portable installation whose startup points elsewhere.
Invoke-Checked (Join-Path $taskRoot 'bin/JonsboResurrection.exe') @('startup','enable','--quiet')
Install-App
$taskCommand=(Get-ItemProperty -LiteralPath $taskRunKey -Name JonsboResurrection).JonsboResurrection
if ($taskCommand -ne ('"'+(Join-Path $taskInstall 'JonsboResurrection.exe')+'" tray')) { throw 'Upgrade did not migrate startup.' }
Uninstall-App
if ((Get-ItemProperty -LiteralPath $taskRunKey -Name JonsboResurrection -ErrorAction SilentlyContinue).JonsboResurrection) { throw 'Owned startup entry survived uninstall.' }
Install-App
# The Windows runner is elevated; exercise real Task Scheduler XML and ownership.
Invoke-Checked (Join-Path $taskInstall 'JonsboResurrection.exe') @('startup','enable','--elevated','--quiet')
$taskSID=[Security.Principal.WindowsIdentity]::GetCurrent().User.Value
Install-App
$taskScheduled=Get-ScheduledTask -TaskName ('JonsboResurrection-'+$taskSID)
if ($taskScheduled.Actions.Execute -ne (Join-Path $taskInstall 'JonsboResurrection.exe')) { throw 'Elevated upgrade startup path mismatch.' }
Uninstall-App
if (Get-ScheduledTask -TaskName ('JonsboResurrection-'+$taskSID) -ErrorAction SilentlyContinue) { throw 'Owned elevated startup survived uninstall.' }
Install-App
# A later portable copy owns startup: uninstall must preserve it.
Invoke-Checked (Join-Path $taskRoot 'bin/JonsboResurrection.exe') @('startup','enable','--quiet')
$taskOther=(Get-ItemProperty -LiteralPath $taskRunKey -Name JonsboResurrection).JonsboResurrection
Uninstall-App
if ((Get-ItemProperty -LiteralPath $taskRunKey -Name JonsboResurrection).JonsboResurrection -ne $taskOther) { throw 'Uninstall removed another copy startup.' }
Invoke-Checked (Join-Path $taskRoot 'bin/JonsboResurrection.exe') @('startup','disable-owned','--quiet')
Write-Output 'Installer smoke passed: fresh install, upgrade, portable startup migration, uninstall, other-copy ownership and data preservation.'
