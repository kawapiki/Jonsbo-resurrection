[CmdletBinding(SupportsShouldProcess)]
param(
    [string]$DataDirectory = (Join-Path $env:LOCALAPPDATA 'JonsboResurrection/ai-subscriptions'),
    [string]$SettingsPath = (Join-Path $env:USERPROFILE '.claude/settings.json'),
    [string]$HelperPath
)
$ErrorActionPreference = 'Stop'
$projectRoot = Split-Path $PSScriptRoot -Parent
if (-not $HelperPath) { $HelperPath = Join-Path $projectRoot 'bin/jonsbo-ai-observer.exe' }
$helper = (Resolve-Path -LiteralPath $HelperPath).Path
$wrapper = (Resolve-Path -LiteralPath (Join-Path $projectRoot 'integrations/claude/statusline.ps1')).Path
$dataDir = (Resolve-Path -LiteralPath $DataDirectory).Path
$bridge = Get-Content -LiteralPath (Join-Path $dataDir 'bridge.json') -Raw | ConvertFrom-Json
$endpoint = [uri]$bridge.api
$ip = $null
if ($endpoint.Scheme -ne 'http' -or -not [Net.IPAddress]::TryParse($endpoint.DnsSafeHost,[ref]$ip) -or -not [Net.IPAddress]::IsLoopback($ip) -or $endpoint.UserInfo -or $endpoint.Query -or $endpoint.Fragment -or $endpoint.AbsolutePath -ne '/') { throw 'The observer requires a numeric loopback HTTP endpoint.' }
$tokenFile = (Resolve-Path -LiteralPath (Join-Path $dataDir 'observer.token')).Path
$token = (Get-Content -LiteralPath $tokenFile -Raw).Trim()
if ($token -notmatch '^[a-fA-F0-9]{64}$') { throw 'Invalid scoped observer credential.' }
$original = if (Test-Path -LiteralPath $SettingsPath) { [IO.File]::ReadAllText($SettingsPath) } else { $null }
$settings = if ($original) { $original | ConvertFrom-Json } else { [PSCustomObject]@{} }
if (-not $settings) { throw 'Invalid Claude settings.' }
$oldCommand = [string]$settings.statusLine.command
if ($oldCommand -and $oldCommand -notmatch 'jonsbo-ai-observer' -and -not $oldCommand.Contains($wrapper.Replace('\','/'))) { throw 'An existing status line must be merged manually; it was preserved.' }
if ($settings.statusLine -and $settings.statusLine.type -ne 'command') { throw 'An existing non-command status line was preserved.' }
if (-not $settings.env) { $settings | Add-Member -NotePropertyName env -NotePropertyValue ([PSCustomObject]@{}) -Force }
$logsEndpoint = $bridge.api.TrimEnd('/') + '/v1/ai/telemetry/claude/v1/logs'
foreach ($key in @('OTEL_EXPORTER_OTLP_LOGS_ENDPOINT','OTEL_EXPORTER_OTLP_ENDPOINT')) {
    $existing = [string]$settings.env.$key
    if ($existing -and $existing -ne $logsEndpoint -and $existing -ne $bridge.api) { throw 'An existing OTLP destination must be merged manually; it was preserved.' }
}
$quote = { param([string]$value) '"' + $value.Replace('\','/') + '"' }
$command = 'powershell.exe -NoLogo -NoProfile -NonInteractive -ExecutionPolicy Bypass -File ' + (& $quote $wrapper) + ' -HelperPath ' + (& $quote $helper) + ' -Api ' + (& $quote ([string]$bridge.api)) + ' -TokenFile ' + (& $quote $tokenFile)
$settings | Add-Member -NotePropertyName statusLine -NotePropertyValue ([PSCustomObject]@{type='command';command=$command}) -Force
$exporters = @(([string]$settings.env.OTEL_LOGS_EXPORTER).Split(',') | Where-Object { $_ -and $_ -ne 'none' })
if ($exporters -notcontains 'otlp') { $exporters += 'otlp' }
$values = @{
    CLAUDE_CODE_ENABLE_TELEMETRY='1'; OTEL_LOGS_EXPORTER=($exporters -join ',');
    OTEL_EXPORTER_OTLP_LOGS_PROTOCOL='http/json'; OTEL_EXPORTER_OTLP_LOGS_ENDPOINT=$logsEndpoint;
    OTEL_EXPORTER_OTLP_LOGS_HEADERS=('Authorization=Bearer ' + $token);
    OTEL_LOG_USER_PROMPTS='0'; OTEL_LOG_ASSISTANT_RESPONSES='0'; OTEL_LOG_TOOL_DETAILS='0';
    OTEL_LOG_TOOL_CONTENT='0'; OTEL_LOG_RAW_API_BODIES='0'
}
if (-not $settings.env.OTEL_METRICS_EXPORTER) { $values.OTEL_METRICS_EXPORTER='none' }
foreach ($key in $values.Keys) { $settings.env | Add-Member -NotePropertyName $key -NotePropertyValue $values[$key] -Force }
$updated = $settings | ConvertTo-Json -Depth 100
if ($PSCmdlet.ShouldProcess($SettingsPath,'Back up and enable local Claude metadata forwarding')) {
    $fullPath = [IO.Path]::GetFullPath($SettingsPath)
    $directory = Split-Path $fullPath -Parent
    [IO.Directory]::CreateDirectory($directory) | Out-Null
    if ($original -and [IO.File]::ReadAllText($fullPath) -ne $original) { throw 'Claude settings changed during setup; retry.' }
    $temporary = Join-Path $directory ('.jonsbo-' + [guid]::NewGuid().ToString('N') + '.tmp')
    try {
        [IO.File]::WriteAllText($temporary,$updated,[Text.UTF8Encoding]::new($false))
        if (Test-Path -LiteralPath $fullPath) {
            $backup = Join-Path $directory ('settings.jonsbo-backup-' + [guid]::NewGuid().ToString('N') + '.json')
            [IO.File]::Replace($temporary,$fullPath,$backup)
            Write-Output ('Settings backup: ' + $backup)
        } else { [IO.File]::Move($temporary,$fullPath) }
    } finally { if (Test-Path -LiteralPath $temporary) { Remove-Item -LiteralPath $temporary } }
}
Write-Output 'Claude metadata forwarding prepared. Restart Claude Code; readings arrive after its next normal response.'
