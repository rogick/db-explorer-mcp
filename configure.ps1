$scriptDir = $PSScriptRoot
if (-not $scriptDir) {
    $scriptDir = (Get-Location).Path
}

& (Join-Path $scriptDir "configure-claude-code.ps1")
