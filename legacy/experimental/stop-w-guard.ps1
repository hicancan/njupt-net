$ErrorActionPreference = "Stop"

$repoRoot = Resolve-Path (Join-Path $PSScriptRoot "..\..")
$stateDir = Join-Path $repoRoot "dist\w-guard"
$terminalPidFile = Join-Path $stateDir "terminal.pid"
$workerPidFile = Join-Path $stateDir "worker.pid"

if (-not (Test-Path $terminalPidFile)) {
    Write-Host "No running NJUPT W guard terminal was found." -ForegroundColor Yellow
    exit 0
}

$rawPid = (Get-Content $terminalPidFile -Raw).Trim()
if ($rawPid -notmatch '^\d+$') {
    Remove-Item $terminalPidFile -ErrorAction SilentlyContinue
    Write-Host "Invalid terminal pid file was removed." -ForegroundColor Yellow
    exit 0
}

$targetPid = [int]$rawPid
$proc = Get-Process -Id $targetPid -ErrorAction SilentlyContinue
if ($proc) {
    Stop-Process -Id $targetPid -Force
    Write-Host "Stopped NJUPT W guard terminal PID $targetPid." -ForegroundColor Green
}
else {
    Write-Host "NJUPT W guard terminal PID $targetPid is not running." -ForegroundColor Yellow
}

Remove-Item $terminalPidFile -ErrorAction SilentlyContinue
Remove-Item $workerPidFile -ErrorAction SilentlyContinue
