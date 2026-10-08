[CmdletBinding(SupportsShouldProcess)]
param(
    [Parameter(Mandatory)][ValidatePattern('^[a-p]{32}$')][string]$ExtensionId,
    [Parameter(Mandatory)][string]$ConfigPath,
    [ValidateSet('Chrome','Edge','Both')][string]$Browser = 'Both',
    [string]$BinaryPath
)
$ErrorActionPreference = 'Stop'
if (-not $BinaryPath) { $BinaryPath = Join-Path (Split-Path $PSScriptRoot -Parent) 'bin/jonsbo-ai-bridge.exe' }
$binary = (Resolve-Path -LiteralPath $BinaryPath).Path
$sourcePath = (Resolve-Path -LiteralPath $ConfigPath).Path
if ((Get-Item -LiteralPath $sourcePath).Length -gt 4096) { throw 'Bridge configuration is too large.' }
$source = Get-Content -LiteralPath $sourcePath -Raw | ConvertFrom-Json
$endpoint = [uri]$source.api
$address = $null
if ($endpoint.Scheme -ne 'http' -or -not [Net.IPAddress]::TryParse($endpoint.DnsSafeHost,[ref]$address) -or -not [Net.IPAddress]::IsLoopback($address) -or $endpoint.UserInfo -or $endpoint.Query -or $endpoint.Fragment -or $endpoint.AbsolutePath -ne '/') { throw 'A numeric loopback HTTP endpoint is required.' }
$credential = (Resolve-Path -LiteralPath $source.token_file).Path
$directory = Split-Path $binary -Parent
$hostConfig = Join-Path $directory 'jonsbo-ai-bridge.json'
$manifestPath = Join-Path $directory 'com.jonsbo.subscription_display.json'
$config = @{api=$source.api;token_file=$credential;extension_id=$ExtensionId} | ConvertTo-Json
$manifest = @{name='com.jonsbo.subscription_display';description='Jonsbo visible subscription chat activity';path=$binary;type='stdio';allowed_origins=@("chrome-extension://$ExtensionId/")} | ConvertTo-Json
if ($PSCmdlet.ShouldProcess($hostConfig,'Write paired native-host configuration')) { [IO.File]::WriteAllText($hostConfig,$config,[Text.UTF8Encoding]::new($false)) }
if ($PSCmdlet.ShouldProcess($manifestPath,'Write native messaging manifest')) { [IO.File]::WriteAllText($manifestPath,$manifest,[Text.UTF8Encoding]::new($false)) }
$keys = @()
if ($Browser -in @('Chrome','Both')) { $keys += 'HKCU:\Software\Google\Chrome\NativeMessagingHosts\com.jonsbo.subscription_display' }
if ($Browser -in @('Edge','Both')) { $keys += 'HKCU:\Software\Microsoft\Edge\NativeMessagingHosts\com.jonsbo.subscription_display' }
foreach ($key in $keys) {
    if ($PSCmdlet.ShouldProcess($key,'Register local native host for this Windows user')) {
        New-Item -Path $key -Force | Out-Null
        Set-Item -LiteralPath $key -Value $manifestPath
    }
}
Write-Output 'Native-host registration prepared. Reload the extension and open a provider conversation.'
