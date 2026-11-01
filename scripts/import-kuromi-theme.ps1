param(
    [string]$Serial,
    [uri]$Api = 'http://127.0.0.1:8787',
    [string]$TokenFile = (Join-Path $env:LOCALAPPDATA 'JonsboResurrection/api-token'),
    [switch]$Apply
)
$ErrorActionPreference = 'Stop'
if (-not $Api.IsLoopback -or $Api.Scheme -ne 'http' -or $Api.UserInfo -or $Api.Query -or $Api.Fragment) {
    throw 'Use the local controller HTTP address; credentials are never sent to a remote host.'
}
$root = Split-Path $PSScriptRoot -Parent
$theme = Join-Path $root 'themes/kuromi-alive'
$layout = Get-Content -LiteralPath (Join-Path $theme 'layout.json') -Raw | ConvertFrom-Json
$headers = @{ Authorization = 'Bearer ' + (Get-Content -LiteralPath $TokenFile -Raw).Trim() }
$base = $Api.AbsoluteUri.TrimEnd('/')
$catalog = Invoke-RestMethod "$base/v1/configurator?width=240&height=240" -Headers $headers
if (-not @($catalog.themes | Where-Object id -eq 'cyclops-orbit').Count) {
    throw 'This controller does not support Thermal orbit. Install a build containing the Kuromi theme support first.'
}
$displays = Invoke-RestMethod "$base/v1/displays" -Headers $headers
if ($Apply) {
    $targets = @($displays | Where-Object { $_.kind -eq 'cyclops' -and (-not $Serial -or $_.serial -eq $Serial) })
    if ($targets.Count -ne 1) { throw 'Specify -Serial for exactly one connected Cyclops display.' }
}
# Reuse this versioned asset on repeated imports; never consume another media slot.
$asset = @($catalog.assets | Where-Object { $_.name -eq 'Kuromi expressions v1' -and $_.width -eq 600 -and $_.height -eq 200 -and $_.frame_count -eq 1 }) | Select-Object -First 1
if (-not $asset) {
    $payload = @{name='Kuromi expressions v1';fps=1;frames=@([Convert]::ToBase64String([IO.File]::ReadAllBytes((Join-Path $theme 'expressions.png'))))} | ConvertTo-Json
    $asset = Invoke-RestMethod "$base/v1/configurator/assets" -Method Post -Headers $headers -ContentType 'application/json' -Body $payload
}
$layout.overlays[0].asset_id = $asset.id
$body = $layout | ConvertTo-Json -Depth 12
$null = Invoke-RestMethod "$base/v1/configurator/layouts/$($layout.id)" -Method Put -Headers $headers -ContentType 'application/json' -Body $body
if ($Apply) {
    $payload = @{serial=$targets[0].serial;layout_id=$layout.id;rotation=$targets[0].assignment.rotation} | ConvertTo-Json
    $null = Invoke-RestMethod "$base/v1/configurator/apply" -Method Post -Headers $headers -ContentType 'application/json' -Body $payload
    Write-Output 'Kuromi - Alive saved and applied.'
} else {
    Write-Output 'Kuromi - Alive saved. Select it in Display Studio, or rerun with -Apply.'
}
Write-Output 'For 8 FPS, use frame_interval: 125ms in the local configuration and restart the controller. Sensor polling remains independent.'
