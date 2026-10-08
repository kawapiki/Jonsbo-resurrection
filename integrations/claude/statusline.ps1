param(
    [Parameter(Mandatory)][string]$HelperPath,
    [Parameter(Mandatory)][string]$Api,
    [Parameter(Mandatory)][string]$TokenFile
)
$ErrorActionPreference = 'Stop'
try {
    $inputText = [Console]::In.ReadToEnd()
    if ($inputText.Length -gt 1048576) { exit 0 }
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $HelperPath
    $start.Arguments = 'statusline --provider claude --api "' + $Api + '" --token-file "' + $TokenFile + '"'
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardInput = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $child = [Diagnostics.Process]::Start($start)
    $errorRead = $child.StandardError.ReadToEndAsync()
    $outputRead = $child.StandardOutput.ReadToEndAsync()
    $child.StandardInput.Write($inputText)
    $child.StandardInput.Close()
    if (-not $child.WaitForExit(3500)) { $child.Kill(); exit 0 }
    [Console]::Out.Write($outputRead.GetAwaiter().GetResult())
    $child.Dispose()
} catch { exit 0 }
