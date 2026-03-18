param(
    [string]$Target = "W",
    [string]$CredentialsPath = "./credentials.json",
    [int]$IntervalSeconds = 3,
    [int]$BindingCheckInterval = 180,
    [string]$ISP = "mobile",
    [string]$LogPath = ""
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
Set-Location $repoRoot

$stateDir = Join-Path $repoRoot "dist\w-guard"
$logsDir = Join-Path $stateDir "logs"
New-Item -ItemType Directory -Force -Path $stateDir | Out-Null
New-Item -ItemType Directory -Force -Path $logsDir | Out-Null

if ([string]::IsNullOrWhiteSpace($LogPath)) {
    $timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
    $LogPath = Join-Path $logsDir "w-guard-$timestamp.log"
}

$currentLogFile = Join-Path $stateDir "current-log.txt"
Set-Content -Path $currentLogFile -Value $LogPath -Encoding utf8

try {
    $Host.UI.RawUI.WindowTitle = "NJUPT W Guard"
}
catch {
}

Write-Host "Starting NJUPT W guard in this terminal..." -ForegroundColor Cyan
Write-Host "Repository: $repoRoot"
Write-Host "Log file: $LogPath"
Write-Host "Target: $Target"
Write-Host "Credentials: $CredentialsPath"
Write-Host "IntervalSeconds: $IntervalSeconds"
Write-Host "BindingCheckInterval: $BindingCheckInterval"
Write-Host "ISP: $ISP"
Write-Host ""

& uv run .\legacy\experimental\njupt_w_guard.py daemon `
    --target $Target `
    --credentials $CredentialsPath `
    --interval-seconds $IntervalSeconds `
    --binding-check-interval $BindingCheckInterval `
    --isp $ISP 2>&1 | Tee-Object -FilePath $LogPath

$exitCode = if ($LASTEXITCODE -ne $null) { $LASTEXITCODE } else { 0 }

Write-Host ""
Write-Host "Guard exited with code $exitCode" -ForegroundColor Yellow
exit $exitCode
