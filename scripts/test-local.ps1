param(
    [string]$ExePath = "./dist/njupt-net-windows-amd64.exe",
    [string]$CredentialsPath = "./credentials.json",
    [string]$IP = "",
    [switch]$IncludeWriteOps,
    [switch]$ReadOnly,
    [switch]$SkipPortal
)

$ErrorActionPreference = "Stop"
Set-Location $PSScriptRoot\..

$passed = 0
$failed = 0

function Write-Section {
    param([string]$Title)
    Write-Host ""
    Write-Host "=== $Title ===" -ForegroundColor Cyan
}

function Invoke-Step {
    param(
        [string]$Name,
        [ScriptBlock]$Action
    )

    Write-Host "[RUN] $Name" -ForegroundColor Yellow
    try {
        & $Action
        Write-Host "[PASS] $Name" -ForegroundColor Green
        $script:passed++
    }
    catch {
        Write-Host "[FAIL] $Name" -ForegroundColor Red
        Write-Host "       $($_.Exception.Message)" -ForegroundColor Red
        $script:failed++
        throw
    }
}

function Require-File {
    param([string]$Path)
    if (-not (Test-Path $Path)) {
        throw "Required file not found: $Path"
    }
}

function Detect-WifiIPv4 {
    $defaultRoutes = Get-NetRoute -AddressFamily IPv4 -DestinationPrefix "0.0.0.0/0" |
        Sort-Object RouteMetric, InterfaceMetric

    foreach ($route in $defaultRoutes) {
        $ips = Get-NetIPAddress -AddressFamily IPv4 -InterfaceIndex $route.InterfaceIndex -ErrorAction SilentlyContinue |
            Where-Object {
                $_.AddressState -eq "Preferred" -and
                $_.IPAddress -notlike "127.*" -and
                $_.IPAddress -notlike "169.254.*" -and
                $_.IPAddress -ne "192.168.137.1"
            }
        foreach ($ip in $ips) {
            if ($ip.IPAddress -like "10.*") {
                return $ip.IPAddress
            }
        }
    }

    return ($defaultRoutes |
        ForEach-Object {
            Get-NetIPAddress -AddressFamily IPv4 -InterfaceIndex $_.InterfaceIndex -ErrorAction SilentlyContinue
        } |
        Where-Object {
            $_.AddressState -eq "Preferred" -and
            $_.IPAddress -notlike "127.*" -and
            $_.IPAddress -notlike "169.254.*"
        } |
        Select-Object -First 1 -ExpandProperty IPAddress)
}

function Exec-Cli {
    param([string[]]$CliArgs)
    & $script:Exe @CliArgs
    if ($LASTEXITCODE -ne 0) {
        throw "Command failed with exit code ${LASTEXITCODE}: ${script:Exe} $($CliArgs -join ' ')"
    }
}

Write-Section "Precheck"
Require-File $ExePath
Require-File $CredentialsPath

$script:Exe = (Resolve-Path $ExePath).Path
$cred = Get-Content $CredentialsPath -Raw | ConvertFrom-Json

if ([string]::IsNullOrWhiteSpace($IP)) {
    $IP = Detect-WifiIPv4
}
if ([string]::IsNullOrWhiteSpace($IP) -and -not $SkipPortal) {
    throw "Cannot auto-detect Wi-Fi IPv4. Please pass -IP explicitly or use -SkipPortal."
}

Write-Host "Executable: $script:Exe"
Write-Host "Using IP: $IP"
Write-Host "IncludeWriteOps: $IncludeWriteOps"
Write-Host "ReadOnly: $ReadOnly"
Write-Host "SkipPortal: $SkipPortal"

Write-Section "Help"
Invoke-Step "root help" { Exec-Cli -CliArgs @("--help") }
Invoke-Step "self help" { Exec-Cli -CliArgs @("self", "--help") }
Invoke-Step "dashboard help" { Exec-Cli -CliArgs @("dashboard", "--help") }
Invoke-Step "service help" { Exec-Cli -CliArgs @("service", "--help") }
Invoke-Step "setting help" { Exec-Cli -CliArgs @("setting", "--help") }
Invoke-Step "bill help" { Exec-Cli -CliArgs @("bill", "--help") }
Invoke-Step "portal help" { Exec-Cli -CliArgs @("portal", "--help") }
Invoke-Step "raw help" { Exec-Cli -CliArgs @("raw", "--help") }

Write-Section "Self"
Invoke-Step "self login (W)" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "self", "login", "--profile", "W")
}
Invoke-Step "self status (W)" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "self", "status", "--profile", "W")
}
Invoke-Step "self doctor (W)" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "self", "doctor", "--profile", "W")
}

Write-Section "Dashboard"
Invoke-Step "dashboard online-list (W)" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "dashboard", "online-list", "--profile", "W")
}
Invoke-Step "dashboard login-history (W)" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "dashboard", "login-history", "--profile", "W")
}
Invoke-Step "dashboard mauth get (W)" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "dashboard", "mauth", "get", "--profile", "W")
}

Write-Section "Service"
Invoke-Step "service binding get (W)" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "service", "binding", "get", "--profile", "W")
}
Invoke-Step "service consume get (W)" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "service", "consume", "get", "--profile", "W")
}
Invoke-Step "service mac list (W)" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "service", "mac", "list", "--profile", "W")
}

Write-Section "Bill"
Invoke-Step "bill online-log (W)" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "bill", "online-log", "--profile", "W")
}

Write-Section "Setting"
Invoke-Step "setting person get (W)" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "setting", "person", "get", "--profile", "W")
}

Write-Section "Raw"
Invoke-Step "raw get login page" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "raw", "get", "/Self/login/?302=LI")
}
Invoke-Step "raw get dashboard with login" {
    Exec-Cli -CliArgs @("--config", $CredentialsPath, "raw", "get", "/Self/dashboard", "--profile", "W", "--login")
}

if (-not $ReadOnly -and -not $SkipPortal) {
    Write-Section "Portal"
    Invoke-Step "portal login (W, mobile)" {
        Exec-Cli -CliArgs @("--config", $CredentialsPath, "portal", "login", "--profile", "W", "--ip", $IP, "--isp", "mobile")
    }
}
elseif (-not $SkipPortal) {
    Write-Host "[SKIP] portal login skipped by ReadOnly" -ForegroundColor DarkYellow
}
else {
    Write-Host "[SKIP] portal login skipped by SkipPortal" -ForegroundColor DarkYellow
}

if ($IncludeWriteOps -and -not $ReadOnly) {
    Write-Section "Write Ops"
    Invoke-Step "service binding set (W mobile fields)" {
        Exec-Cli -CliArgs @(
            "--config", $CredentialsPath,
            "--yes",
            "service", "binding", "set",
            "--profile", "W",
            "--mobile-account", $cred.cmcc.account,
            "--mobile-password", $cred.cmcc.password
        )
    }

    Invoke-Step "service migrate (W -> B)" {
        Exec-Cli -CliArgs @(
            "--config", $CredentialsPath,
            "--yes",
            "service", "migrate",
            "--from-profile", "W",
            "--to-profile", "B",
            "--mobile-account", $cred.cmcc.account,
            "--mobile-password", $cred.cmcc.password
        )
    }
}
elseif ($IncludeWriteOps -and $ReadOnly) {
    Write-Host "[SKIP] write operations disabled because ReadOnly is enabled" -ForegroundColor DarkYellow
}

Write-Section "Summary"
Write-Host "Passed: $passed" -ForegroundColor Green
Write-Host "Failed: $failed" -ForegroundColor Red

if ($failed -gt 0) {
    exit 1
}

exit 0
