param([ValidatePattern('^\d+\.\d+\.\d+(-[0-9A-Za-z.-]+)?$')][string]$Version)
$ErrorActionPreference='Stop'
$taskRoot=Split-Path $PSScriptRoot -Parent
if (!$Version) { $Version=(Get-Content (Join-Path $taskRoot 'VERSION') -Raw).Trim() }
if ($Version -ne (Get-Content (Join-Path $taskRoot 'VERSION') -Raw).Trim()) { throw 'Installer version must match VERSION.' }
$taskArchive=Join-Path $taskRoot "dist/JonsboResurrection-$Version-windows-amd64.zip"
if (!(Test-Path -LiteralPath $taskArchive)) { throw 'Build and package the portable release first.' }
$taskExpected=((Get-Content -LiteralPath ($taskArchive+'.sha256') -Raw).Trim() -split '\s+')[0]
if ((Get-FileHash -LiteralPath $taskArchive -Algorithm SHA256).Hash.ToLowerInvariant() -ne $taskExpected) { throw 'Portable package checksum mismatch.' }
$taskPayload=Join-Path $taskRoot ('dist/installer-payload-'+[guid]::NewGuid().ToString('N'))
Expand-Archive -LiteralPath $taskArchive -DestinationPath $taskPayload
foreach ($taskBinary in @('JonsboResurrection.exe','bin/jonsbo.exe')) {
 if ((Get-Item -LiteralPath (Join-Path $taskPayload $taskBinary)).VersionInfo.ProductVersion -ne $Version) { throw 'Payload executable version mismatch.' }
}
$taskInstaller=Join-Path $taskRoot "dist/JonsboResurrection-$Version-Setup.exe"
if (Test-Path -LiteralPath $taskInstaller) { throw 'Installer already exists; do not overwrite release artifacts.' }
$taskCompiler=& (Join-Path $PSScriptRoot 'setup-inno.ps1')
& $taskCompiler /Qp "/DAppVersion=$Version" "/DNumericVersion=$(($Version -split '-')[0])" "/DPayloadDir=$taskPayload" "/DOutputPath=$(Join-Path $taskRoot 'dist')" (Join-Path $taskRoot 'installer/JonsboResurrection.iss')
if ($LASTEXITCODE -ne 0) { throw 'Installer compilation failed.' }
$taskHash=(Get-FileHash -LiteralPath $taskInstaller -Algorithm SHA256).Hash.ToLowerInvariant()
Set-Content -LiteralPath ($taskInstaller+'.sha256') -Value ($taskHash+'  '+[IO.Path]::GetFileName($taskInstaller)) -Encoding ascii
Write-Output "Created $taskInstaller"
Write-Output "SHA256 $taskHash"
