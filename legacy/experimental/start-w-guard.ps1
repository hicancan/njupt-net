param(
    [string]$Target = "W",
    [string]$CredentialsPath = "./credentials.json",
    [int]$IntervalSeconds = 3,
    [int]$BindingCheckInterval = 180,
    [string]$ISP = "mobile"
)

$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$runner = Resolve-Path (Join-Path $PSScriptRoot "run-w-guard.ps1")
$stateDir = Join-Path $repoRoot "dist\w-guard"
$logsDir = Join-Path $stateDir "logs"
$terminalPidFile = Join-Path $stateDir "terminal.pid"
$currentLogFile = Join-Path $stateDir "current-log.txt"

New-Item -ItemType Directory -Force -Path $stateDir | Out-Null
New-Item -ItemType Directory -Force -Path $logsDir | Out-Null

if (Test-Path $terminalPidFile) {
    $existingRaw = (Get-Content $terminalPidFile -Raw).Trim()
    if ($existingRaw -match '^\d+$') {
        $existingPid = [int]$existingRaw
        $existingProc = Get-Process -Id $existingPid -ErrorAction SilentlyContinue
        if ($existingProc) {
            Write-Host "NJUPT W guard is already running in terminal PID $existingPid." -ForegroundColor Yellow
            exit 0
        }
    }
}

$resolvedCredentials = Resolve-Path $CredentialsPath
$timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$logPath = Join-Path $logsDir "w-guard-$timestamp.log"
Set-Content -Path $currentLogFile -Value $logPath -Encoding utf8

$proc = Start-Process `
    -FilePath "powershell.exe" `
    -ArgumentList @(
        "-NoExit",
        "-ExecutionPolicy", "Bypass",
        "-File", $runner.Path,
        "-Target", $Target,
        "-CredentialsPath", $resolvedCredentials.Path,
        "-IntervalSeconds", $IntervalSeconds,
        "-BindingCheckInterval", $BindingCheckInterval,
        "-ISP", $ISP,
        "-LogPath", $logPath
    ) `
    -WorkingDirectory $repoRoot.Path `
    -PassThru

Set-Content -Path $terminalPidFile -Value $proc.Id -Encoding ascii

Write-Host "NJUPT W guard started in a new terminal." -ForegroundColor Green
Write-Host "Terminal PID: $($proc.Id)"
Write-Host "Log file: $logPath"
Write-Host "Stop with: .\legacy\experimental\stop-w-guard.ps1"
