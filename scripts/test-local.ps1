param(
    [string]$ExePath = ".\dist\njupt-net-windows-amd64.exe",
    [string]$ConfigPath = ".\config.json",
    [string]$IP = "",
    [string]$RouterHost = "immortalwrt",
    [string]$RouterUser = "root",
    [string]$RouterConfigPath = "/etc/njupt-net/config.json",
    [string]$RouterStateDir = "/tmp/njupt-net",
    [switch]$ReadOnly,
    [switch]$SkipPortal
)

$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest
Set-Location (Join-Path $PSScriptRoot "..")

$Results = New-Object System.Collections.Generic.List[object]
$Passed = 0
$Failed = 0
$SemanticPassed = 0
$script:UseInsecureTLS = $false

function Write-Section {
    param([string]$Title)
    Write-Host ""
    Write-Host "=== $Title ===" -ForegroundColor Cyan
}

function Add-Result {
    param(
        [string]$Name,
        [string]$Status,
        [string]$Detail
    )

    $script:Results.Add([pscustomobject]@{
        Name   = $Name
        Status = $Status
        Detail = $Detail
    }) | Out-Null

    switch ($Status) {
        "PASS" { $script:Passed++ }
        "PASS-GUARDED" { $script:Passed++; $script:SemanticPassed++ }
        default { $script:Failed++ }
    }
}

function Get-PropertyValue {
    param(
        [Parameter(Mandatory = $true)]$Object,
        [Parameter(Mandatory = $true)][string]$Name
    )

    if ($null -eq $Object) {
        return $null
    }
    if ($Object -is [System.Collections.IDictionary]) {
        if ($Object.Contains($Name)) {
            return $Object[$Name]
        }
        return $null
    }
    $property = $Object.PSObject.Properties[$Name]
    if ($null -ne $property) {
        return $property.Value
    }
    return $null
}

function Get-NestedValue {
    param(
        [Parameter(Mandatory = $true)]$Object,
        [Parameter(Mandatory = $true)][string[]]$Path
    )

    $cursor = $Object
    foreach ($segment in $Path) {
        $cursor = Get-PropertyValue -Object $cursor -Name $segment
        if ($null -eq $cursor) {
            return $null
        }
    }
    return $cursor
}

function Require-File {
    param([Parameter(Mandatory = $true)][string]$Path)
    if (-not (Test-Path $Path)) {
        throw "Required file not found: $Path"
    }
}

function Detect-WifiIPv4 {
    $defaultRoutes = Get-NetRoute -AddressFamily IPv4 -DestinationPrefix "0.0.0.0/0" -ErrorAction SilentlyContinue |
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

function Get-RouterPortalIP {
    $ssh = Get-Command ssh -ErrorAction Stop
    $target = "$RouterUser@$RouterHost"
    $stdoutFile = [System.IO.Path]::GetTempFileName()
    $stderrFile = [System.IO.Path]::GetTempFileName()
    try {
        & $ssh.Source $target "/usr/bin/njupt-net --config $RouterConfigPath --output json guard status --state-dir $RouterStateDir" 1> $stdoutFile 2> $stderrFile
        if ($LASTEXITCODE -ne 0) {
            $stderr = Get-Content $stderrFile -Raw
            throw "ssh guard status failed: $stderr"
        }
        $payload = Get-Content $stdoutFile -Raw | ConvertFrom-Json
        $selectedIp = Get-NestedValue -Object $payload -Path @("data", "connectivity", "probe", "selectedIp")
        if ([string]::IsNullOrWhiteSpace([string]$selectedIp)) {
            throw "router guard status did not expose connectivity.probe.selectedIp"
        }
        return [string]$selectedIp
    }
    finally {
        Remove-Item $stdoutFile, $stderrFile -ErrorAction SilentlyContinue
    }
}

function Wait-ForInternetHealthy {
    param(
        [int]$TimeoutSeconds = 90,
        [int]$IntervalSeconds = 3
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $response = Invoke-WebRequest -Uri "http://captive.apple.com/hotspot-detect.html" -UseBasicParsing -TimeoutSec 5
            if ($response.StatusCode -eq 200 -and $response.Content -match "Success") {
                return
            }
        }
        catch {
        }
        Start-Sleep -Seconds $IntervalSeconds
    }
    throw "internet did not recover within ${TimeoutSeconds}s"
}

function Invoke-CliJson {
    param([string[]]$CliArgs)

    $stdoutFile = [System.IO.Path]::GetTempFileName()
    $stderrFile = [System.IO.Path]::GetTempFileName()
    try {
        & $script:Exe @CliArgs 1> $stdoutFile 2> $stderrFile
        $exitCode = $LASTEXITCODE
        $stdout = Get-Content $stdoutFile -Raw
        $stderr = Get-Content $stderrFile -Raw
        $json = $null
        if (-not [string]::IsNullOrWhiteSpace($stdout)) {
            try {
                $json = $stdout | ConvertFrom-Json
            }
            catch {
                $jsonStart = -1
                $lines = $stdout -split "`r?`n"
                for ($i = 0; $i -lt $lines.Count; $i++) {
                    if ($lines[$i].TrimStart().StartsWith("{")) {
                        $jsonStart = $i
                        break
                    }
                }
                if ($jsonStart -ge 0) {
                    $candidate = ($lines[$jsonStart..($lines.Count - 1)] -join "`n")
                    try {
                        $json = $candidate | ConvertFrom-Json
                    }
                    catch {
                        throw "command did not emit valid JSON`nstdout:`n$stdout`nstderr:`n$stderr"
                    }
                } else {
                    throw "command emitted no JSON payload`nstderr:`n$stderr"
                }
            }
        }
        return [pscustomobject]@{
            ExitCode = $exitCode
            Stdout   = $stdout
            Stderr   = $stderr
            Json     = $json
            Args     = @($CliArgs)
        }
    }
    finally {
        Remove-Item $stdoutFile, $stderrFile -ErrorAction SilentlyContinue
    }
}

function Require-JsonPayload {
    param($Result)
    if ($null -eq $Result.Json) {
        throw "command emitted no JSON payload`nstderr:`n$($Result.Stderr)"
    }
    return $Result.Json
}

function Assert-ConfirmedSuccess {
    param($Result)
    $json = Require-JsonPayload $Result
    if ($Result.ExitCode -ne 0) {
        throw "expected exit code 0, got $($Result.ExitCode)`nstdout:`n$($Result.Stdout)`nstderr:`n$($Result.Stderr)"
    }
    if (-not (Get-PropertyValue -Object $json -Name "success")) {
        throw "expected success=true, got payload:`n$($Result.Stdout)"
    }
}

function Assert-SemanticResult {
    param(
        $Result,
        [Parameter(Mandatory = $true)][string]$ExpectedLevel,
        [string]$ExpectedProblemCode = "",
        [switch]$ExpectedSuccess
    )

    $json = Require-JsonPayload $Result
    $level = [string](Get-PropertyValue -Object $json -Name "level")
    if ($level -ne $ExpectedLevel) {
        throw "expected level=$ExpectedLevel, got $level`npayload:`n$($Result.Stdout)"
    }
    $success = [bool](Get-PropertyValue -Object $json -Name "success")
    if ($ExpectedSuccess.IsPresent -and -not $success) {
        throw "expected success=true for $ExpectedLevel result`npayload:`n$($Result.Stdout)"
    }
    if (-not $ExpectedSuccess.IsPresent -and $success) {
        throw "expected success=false for $ExpectedLevel result`npayload:`n$($Result.Stdout)"
    }
    if (-not [string]::IsNullOrWhiteSpace($ExpectedProblemCode)) {
        $problems = Get-PropertyValue -Object $json -Name "problems"
        if ($null -eq $problems -or $problems.Count -lt 1) {
            throw "expected problem code $ExpectedProblemCode but payload had no problems`npayload:`n$($Result.Stdout)"
        }
        $code = [string](Get-PropertyValue -Object $problems[0] -Name "code")
        if ($code -ne $ExpectedProblemCode) {
            throw "expected problem code $ExpectedProblemCode, got $code`npayload:`n$($Result.Stdout)"
        }
    }
}

function Invoke-TestCase {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][scriptblock]$Action
    )

    Write-Host "[RUN] $Name" -ForegroundColor Yellow
    try {
        $detail = & $Action
        if ([string]::IsNullOrWhiteSpace([string]$detail)) {
            $detail = "ok"
        }
        Add-Result -Name $Name -Status "PASS" -Detail $detail
        Write-Host "[PASS] $Name" -ForegroundColor Green
    }
    catch {
        Add-Result -Name $Name -Status "FAIL" -Detail $_.Exception.Message
        Write-Host "[FAIL] $Name" -ForegroundColor Red
        Write-Host "       $($_.Exception.Message)" -ForegroundColor Red
    }
}

function Invoke-SemanticPassCase {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][scriptblock]$Action
    )

    Write-Host "[RUN] $Name" -ForegroundColor Yellow
    try {
        $detail = & $Action
        if ([string]::IsNullOrWhiteSpace([string]$detail)) {
            $detail = "semantic pass"
        }
        Add-Result -Name $Name -Status "PASS-GUARDED" -Detail $detail
        Write-Host "[PASS] $Name (semantic)" -ForegroundColor Green
    }
    catch {
        Add-Result -Name $Name -Status "FAIL" -Detail $_.Exception.Message
        Write-Host "[FAIL] $Name" -ForegroundColor Red
        Write-Host "       $($_.Exception.Message)" -ForegroundColor Red
    }
}

function New-RootArgs {
    $args = @("--config", $script:ConfigPath, "--output", "json")
    if ($script:UseInsecureTLS) {
        $args += "--insecure-tls"
    }
    return $args
}

function Get-AccountNames {
    $accounts = Get-PropertyValue -Object $script:ConfigObject -Name "accounts"
    if ($null -eq $accounts) {
        throw "config.json missing accounts object"
    }
    return @($accounts.PSObject.Properties.Name)
}

function Get-BindingState {
    param([string]$Profile)

    $result = Invoke-CliJson ((New-RootArgs) + @("service", "binding", "get", "--profile", $Profile))
    Assert-ConfirmedSuccess $result
    $json = Require-JsonPayload $result
    $data = Get-PropertyValue -Object $json -Name "data"
    return [ordered]@{
        TelecomAccount  = [string](Get-PropertyValue -Object $data -Name "telecomAccount")
        TelecomPassword = [string](Get-PropertyValue -Object $data -Name "telecomPassword")
        MobileAccount   = [string](Get-PropertyValue -Object $data -Name "mobileAccount")
        MobilePassword  = [string](Get-PropertyValue -Object $data -Name "mobilePassword")
    }
}

function Restore-BindingState {
    param(
        [string]$Profile,
        [hashtable]$State
    )

    $args = (New-RootArgs) + @("--yes", "service", "binding", "set", "--profile", $Profile, "--clear-all")
    if (-not [string]::IsNullOrWhiteSpace($State.TelecomAccount)) {
        $args += @("--telecom-account", $State.TelecomAccount)
    }
    if (-not [string]::IsNullOrWhiteSpace($State.TelecomPassword)) {
        $args += @("--telecom-password", $State.TelecomPassword)
    }
    if (-not [string]::IsNullOrWhiteSpace($State.MobileAccount)) {
        $args += @("--mobile-account", $State.MobileAccount)
    }
    if (-not [string]::IsNullOrWhiteSpace($State.MobilePassword)) {
        $args += @("--mobile-password", $State.MobilePassword)
    }
    $result = Invoke-CliJson $args
    Assert-ConfirmedSuccess $result
}

function Get-ConsumeState {
    param([string]$Profile)

    $result = Invoke-CliJson ((New-RootArgs) + @("service", "consume", "get", "--profile", $Profile))
    Assert-ConfirmedSuccess $result
    $json = Require-JsonPayload $result
    $data = Get-PropertyValue -Object $json -Name "data"
    return [ordered]@{
        CurrentLimit    = [string](Get-PropertyValue -Object $data -Name "currentLimit")
        InstallmentFlag = [string](Get-PropertyValue -Object $data -Name "installmentFlag")
    }
}

function Get-OnlineSessions {
    param([string]$Profile)

    $result = Invoke-CliJson ((New-RootArgs) + @("dashboard", "online-list", "--profile", $Profile))
    Assert-ConfirmedSuccess $result
    $json = Require-JsonPayload $result
    $data = Get-PropertyValue -Object $json -Name "data"
    return @($data)
}

function Get-SessionIdForOffline {
    param([string]$Profile)

    $sessions = Get-OnlineSessions -Profile $Profile
    foreach ($session in $sessions) {
        $sessionId = [string](Get-PropertyValue -Object $session -Name "sessionId")
        if (-not [string]::IsNullOrWhiteSpace($sessionId)) {
            return $sessionId
        }
    }
    throw "dashboard online-list returned no force-offline candidate sessionId"
}

function Get-MigrateProfiles {
    $accounts = Get-AccountNames
    $fromProfile = $script:NightProfile
    $toProfile = $script:DayProfile

    if ([string]::IsNullOrWhiteSpace($fromProfile) -or [string]::IsNullOrWhiteSpace($toProfile)) {
        throw "guard.schedule.dayProfile and nightProfile must both be configured"
    }
    if ($fromProfile -eq $toProfile) {
        $fallback = $accounts | Where-Object { $_ -ne $fromProfile } | Select-Object -First 1
        if ([string]::IsNullOrWhiteSpace([string]$fallback)) {
            throw "service migrate requires two distinct configured accounts"
        }
        $toProfile = $fallback
    }
    return [ordered]@{
        From = $fromProfile
        To   = $toProfile
    }
}

function Select-MigrationTargetFields {
    param(
        [hashtable]$SourceState,
        [hashtable]$TargetState
    )

    $selected = [ordered]@{}
    foreach ($field in @("TelecomAccount", "TelecomPassword", "MobileAccount", "MobilePassword")) {
        $value = [string]$SourceState[$field]
        if ([string]::IsNullOrWhiteSpace($value)) {
            $value = [string]$TargetState[$field]
        }
        if (-not [string]::IsNullOrWhiteSpace($value)) {
            $selected[$field] = $value
        }
    }

    if ($selected.Count -eq 0) {
        $cmcc = Get-PropertyValue -Object $script:ConfigObject -Name "cmcc"
        $cmccAccount = [string](Get-PropertyValue -Object $cmcc -Name "account")
        $cmccPassword = [string](Get-PropertyValue -Object $cmcc -Name "password")
        if (-not [string]::IsNullOrWhiteSpace($cmccAccount)) {
            $selected["MobileAccount"] = $cmccAccount
        }
        if (-not [string]::IsNullOrWhiteSpace($cmccPassword)) {
            $selected["MobilePassword"] = $cmccPassword
        }
    }

    if ($selected.Count -eq 0) {
        throw "service migrate could not determine any target FLDEXTRA fields to migrate"
    }
    return $selected
}

Write-Section "Precheck"
Require-File $ExePath
Require-File $ConfigPath

$script:Exe = (Resolve-Path $ExePath).Path
$script:ConfigPath = (Resolve-Path $ConfigPath).Path
$script:ConfigObject = Get-Content $script:ConfigPath -Raw | ConvertFrom-Json
$schedule = Get-NestedValue -Object $script:ConfigObject -Path @("guard", "schedule")
$script:DayProfile = [string](Get-PropertyValue -Object $schedule -Name "dayProfile")
$script:NightProfile = [string](Get-PropertyValue -Object $schedule -Name "nightProfile")
if ([string]::IsNullOrWhiteSpace($script:DayProfile) -or [string]::IsNullOrWhiteSpace($script:NightProfile)) {
    throw "config.json must explicitly define guard.schedule.dayProfile and guard.schedule.nightProfile"
}
if ([string]::IsNullOrWhiteSpace($IP) -and -not $SkipPortal) {
    $IP = Detect-WifiIPv4
    if ([string]::IsNullOrWhiteSpace($IP) -or $IP -notlike "10.*") {
        $IP = Get-RouterPortalIP
    }
}
if ([string]::IsNullOrWhiteSpace($IP) -and -not $SkipPortal) {
    throw "Could not resolve a portal IP from the local machine or router guard status."
}
$portalConfig = Get-PropertyValue -Object $script:ConfigObject -Name "portal"
$portalInsecure = $false
if ($null -ne $portalConfig) {
    $portalInsecure = [bool](Get-PropertyValue -Object $portalConfig -Name "insecureTLS")
}
$script:UseInsecureTLS = $portalInsecure -or (-not [string]::IsNullOrWhiteSpace($IP))

Write-Host "Executable: $script:Exe"
Write-Host "Config: $script:ConfigPath"
Write-Host "Day profile: $script:DayProfile"
Write-Host "Night profile: $script:NightProfile"
if (-not $SkipPortal) {
    Write-Host "Portal IP: $IP"
}
Write-Host "ReadOnly: $ReadOnly"
Write-Host "SkipPortal: $SkipPortal"

Write-Section "Self"
Invoke-TestCase "self login" {
    $result = Invoke-CliJson ((New-RootArgs) + @("self", "login", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "confirmed login"
}
Invoke-TestCase "self status" {
    $result = Invoke-CliJson ((New-RootArgs) + @("self", "status", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "status confirmed"
}
Invoke-TestCase "self doctor" {
    $result = Invoke-CliJson ((New-RootArgs) + @("self", "doctor", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "doctor confirmed"
}
Invoke-TestCase "self logout" {
    $result = Invoke-CliJson ((New-RootArgs) + @("--yes", "self", "logout", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "logout confirmed"
}

Write-Section "Dashboard"
Invoke-TestCase "dashboard online-list" {
    $result = Invoke-CliJson ((New-RootArgs) + @("dashboard", "online-list", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "online-list confirmed"
}
Invoke-TestCase "dashboard login-history" {
    $result = Invoke-CliJson ((New-RootArgs) + @("dashboard", "login-history", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "login-history confirmed"
}
Invoke-TestCase "dashboard refresh-account-raw" {
    $result = Invoke-CliJson ((New-RootArgs) + @("dashboard", "refresh-account-raw", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "refresh-account-raw confirmed"
}
Invoke-TestCase "dashboard mauth get" {
    $result = Invoke-CliJson ((New-RootArgs) + @("dashboard", "mauth", "get", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "mauth get confirmed"
}
if (-not $ReadOnly) {
    Invoke-TestCase "dashboard mauth toggle" {
        $result = Invoke-CliJson ((New-RootArgs) + @("--yes", "dashboard", "mauth", "toggle", "--profile", $script:DayProfile, "--restore"))
        Assert-ConfirmedSuccess $result
        return "mauth toggled with restore"
    }
    Invoke-TestCase "dashboard offline" {
        $sessionId = Get-SessionIdForOffline -Profile $script:DayProfile
        $result = Invoke-CliJson ((New-RootArgs) + @("--yes", "dashboard", "offline", "--profile", $script:DayProfile, $sessionId))
        Assert-ConfirmedSuccess $result
        Wait-ForInternetHealthy
        return "force-offlined session $sessionId"
    }
}
else {
    Add-Result -Name "dashboard mauth toggle" -Status "PASS-GUARDED" -Detail "skipped by ReadOnly"
    Add-Result -Name "dashboard offline" -Status "PASS-GUARDED" -Detail "skipped by ReadOnly"
}

Write-Section "Service"
Invoke-TestCase "service binding get" {
    $result = Invoke-CliJson ((New-RootArgs) + @("service", "binding", "get", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "binding get confirmed"
}
if (-not $ReadOnly) {
    Invoke-TestCase "service binding set" {
        $current = Get-BindingState -Profile $script:DayProfile
        $args = (New-RootArgs) + @("--yes", "service", "binding", "set", "--profile", $script:DayProfile, "--clear-all", "--restore")
        if (-not [string]::IsNullOrWhiteSpace($current.TelecomAccount)) { $args += @("--telecom-account", $current.TelecomAccount) }
        if (-not [string]::IsNullOrWhiteSpace($current.TelecomPassword)) { $args += @("--telecom-password", $current.TelecomPassword) }
        if (-not [string]::IsNullOrWhiteSpace($current.MobileAccount)) { $args += @("--mobile-account", $current.MobileAccount) }
        if (-not [string]::IsNullOrWhiteSpace($current.MobilePassword)) { $args += @("--mobile-password", $current.MobilePassword) }
        $result = Invoke-CliJson $args
        Assert-ConfirmedSuccess $result
        return "binding set exercised with restore"
    }
}
else {
    Add-Result -Name "service binding set" -Status "PASS-GUARDED" -Detail "skipped by ReadOnly"
}
Invoke-TestCase "service consume get" {
    $result = Invoke-CliJson ((New-RootArgs) + @("service", "consume", "get", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "consume get confirmed"
}
if (-not $ReadOnly) {
    Invoke-TestCase "service consume set" {
        $current = Get-ConsumeState -Profile $script:DayProfile
        $limit = $current.CurrentLimit
        if ([string]::IsNullOrWhiteSpace($limit)) {
            $limit = $current.InstallmentFlag
        }
        if ([string]::IsNullOrWhiteSpace($limit)) {
            throw "consume get returned no currentLimit/installmentFlag"
        }
        $result = Invoke-CliJson ((New-RootArgs) + @("--yes", "service", "consume", "set", "--profile", $script:DayProfile, "--limit", $limit, "--restore"))
        Assert-ConfirmedSuccess $result
        return "consume set exercised with restore"
    }
}
else {
    Add-Result -Name "service consume set" -Status "PASS-GUARDED" -Detail "skipped by ReadOnly"
}
Invoke-TestCase "service mac list" {
    $result = Invoke-CliJson ((New-RootArgs) + @("service", "mac", "list", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "mac list confirmed"
}
if (-not $ReadOnly) {
    Invoke-TestCase "service migrate" {
        $profiles = Get-MigrateProfiles
        $fromState = Get-BindingState -Profile $profiles.From
        $toState = Get-BindingState -Profile $profiles.To
        $target = Select-MigrationTargetFields -SourceState $fromState -TargetState $toState
        try {
            $args = (New-RootArgs) + @("--yes", "service", "migrate", "--from-profile", $profiles.From, "--to-profile", $profiles.To)
        if ($target.Contains("TelecomAccount") -and -not [string]::IsNullOrWhiteSpace([string]$target["TelecomAccount"])) { $args += @("--telecom-account", [string]$target["TelecomAccount"]) }
        if ($target.Contains("TelecomPassword") -and -not [string]::IsNullOrWhiteSpace([string]$target["TelecomPassword"])) { $args += @("--telecom-password", [string]$target["TelecomPassword"]) }
        if ($target.Contains("MobileAccount") -and -not [string]::IsNullOrWhiteSpace([string]$target["MobileAccount"])) { $args += @("--mobile-account", [string]$target["MobileAccount"]) }
        if ($target.Contains("MobilePassword") -and -not [string]::IsNullOrWhiteSpace([string]$target["MobilePassword"])) { $args += @("--mobile-password", [string]$target["MobilePassword"]) }
            $result = Invoke-CliJson $args
            Assert-ConfirmedSuccess $result
        }
        finally {
            Restore-BindingState -Profile $profiles.From -State $fromState
            Restore-BindingState -Profile $profiles.To -State $toState
        }
        return "migrate confirmed and bindings restored"
    }
}
else {
    Add-Result -Name "service migrate" -Status "PASS-GUARDED" -Detail "skipped by ReadOnly"
}

Write-Section "Setting"
Invoke-TestCase "setting person get" {
    $result = Invoke-CliJson ((New-RootArgs) + @("setting", "person", "get", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    $rawHtml = Get-NestedValue -Object $result.Json -Path @("data", "rawHtml")
    if ($null -ne $rawHtml -and -not [string]::IsNullOrWhiteSpace([string]$rawHtml)) {
        throw "setting person get leaked rawHtml in standard JSON output"
    }
    return "person get sanitized"
}
Invoke-SemanticPassCase "setting person update" {
    $result = Invoke-CliJson ((New-RootArgs) + @("--yes", "setting", "person", "update", "--profile", $script:DayProfile, "--dry-run", "--field", "phone=13900000000"))
    Assert-SemanticResult -Result $result -ExpectedLevel "blocked"
    return "dry-run blocked semantics confirmed"
}

Write-Section "Bill"
Invoke-TestCase "bill online-log" {
    $result = Invoke-CliJson ((New-RootArgs) + @("bill", "online-log", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "online-log confirmed"
}
Invoke-TestCase "bill month-pay" {
    $result = Invoke-CliJson ((New-RootArgs) + @("bill", "month-pay", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "month-pay confirmed"
}
Invoke-TestCase "bill operator-log" {
    $result = Invoke-CliJson ((New-RootArgs) + @("bill", "operator-log", "--profile", $script:DayProfile))
    Assert-ConfirmedSuccess $result
    return "operator-log confirmed"
}

Write-Section "Portal"
if (-not $SkipPortal) {
    Invoke-TestCase "portal login" {
        $result = Invoke-CliJson ((New-RootArgs) + @("portal", "login", "--profile", $script:DayProfile, "--ip", $IP, "--isp", "mobile"))
        $json = Require-JsonPayload $result
        if ($Result.ExitCode -ne 0) {
            throw "portal login exited $($Result.ExitCode)`npayload:`n$($Result.Stdout)"
        }
        if (-not (Get-PropertyValue -Object $json -Name "success")) {
            throw "portal login expected success=true`npayload:`n$($Result.Stdout)"
        }
        $level = [string](Get-PropertyValue -Object $json -Name "level")
        if ($level -notin @("confirmed", "guarded")) {
            throw "portal login expected confirmed or guarded success, got $level"
        }
        return "portal login returned $level success"
    }
    if (-not $ReadOnly) {
        Invoke-TestCase "portal logout" {
            $result = Invoke-CliJson ((New-RootArgs) + @("--yes", "portal", "logout", "--ip", $IP))
            Assert-ConfirmedSuccess $result
            Wait-ForInternetHealthy
            return "portal logout confirmed and internet recovered"
        }
    }
    else {
        Add-Result -Name "portal logout" -Status "PASS-GUARDED" -Detail "skipped by ReadOnly"
    }
    Invoke-SemanticPassCase "portal login-801" {
        $result = Invoke-CliJson ((New-RootArgs) + @("portal", "login-801", "--profile", $script:DayProfile))
        Assert-SemanticResult -Result $result -ExpectedLevel "blocked" -ExpectedProblemCode "blocked_capability"
        return "801 login blocked semantics confirmed"
    }
    if (-not $ReadOnly) {
        Invoke-TestCase "portal logout-801" {
            $result = Invoke-CliJson ((New-RootArgs) + @("--yes", "portal", "logout-801", "--ip", $IP))
            Assert-ConfirmedSuccess $result
            return "801 logout confirmed"
        }
    }
    else {
        Add-Result -Name "portal logout-801" -Status "PASS-GUARDED" -Detail "skipped by ReadOnly"
    }
}
else {
    foreach ($command in @("portal login", "portal logout", "portal login-801", "portal logout-801")) {
        Add-Result -Name $command -Status "PASS-GUARDED" -Detail "skipped by SkipPortal"
    }
}

Write-Section "Raw"
Invoke-TestCase "raw get" {
    $result = Invoke-CliJson ((New-RootArgs) + @("raw", "get", "/Self/login/?302=LI"))
    Assert-ConfirmedSuccess $result
    return "raw get confirmed"
}
Invoke-TestCase "raw post" {
    $result = Invoke-CliJson ((New-RootArgs) + @("raw", "post", "/Self/login/verify", "--form", "account=placeholder", "--form", "password=placeholder", "--form", "code="))
    Assert-ConfirmedSuccess $result
    return "raw post confirmed"
}

Write-Section "Guard"
Wait-ForInternetHealthy
$guardOnceStateDir = Join-Path $env:TEMP ("njupt-net-guard-once-" + [guid]::NewGuid().ToString("N"))
$guardDaemonStateDir = Join-Path $env:TEMP ("njupt-net-guard-daemon-" + [guid]::NewGuid().ToString("N"))
$guardRunStateDir = Join-Path $env:TEMP ("njupt-net-guard-run-" + [guid]::NewGuid().ToString("N"))
Invoke-TestCase "guard once" {
    $result = Invoke-CliJson ((New-RootArgs) + @("--yes", "guard", "once", "--state-dir", $guardOnceStateDir))
    Assert-ConfirmedSuccess $result
    return "guard once confirmed"
}
Invoke-TestCase "guard start" {
    $result = Invoke-CliJson ((New-RootArgs) + @("--yes", "guard", "start", "--replace", "--state-dir", $guardDaemonStateDir))
    Assert-ConfirmedSuccess $result
    Start-Sleep -Seconds 3
    return "guard start confirmed"
}
Invoke-TestCase "guard status" {
    $result = Invoke-CliJson @("--output", "json", "guard", "status", "--state-dir", $guardDaemonStateDir)
    Assert-ConfirmedSuccess $result
    $running = [bool](Get-NestedValue -Object $result.Json -Path @("data", "running"))
    if (-not $running) {
        throw "guard status expected running=true"
    }
    return "guard status confirmed without config"
}
Invoke-TestCase "guard stop" {
    $result = Invoke-CliJson @("--output", "json", "--yes", "guard", "stop", "--state-dir", $guardDaemonStateDir)
    Assert-ConfirmedSuccess $result
    return "guard stop confirmed without config"
}
Invoke-TestCase "guard run" {
    $stdoutFile = [System.IO.Path]::GetTempFileName()
    $stderrFile = [System.IO.Path]::GetTempFileName()
    try {
        $process = Start-Process -FilePath $script:Exe -ArgumentList @("--config", $script:ConfigPath, "--output", "json", "--yes", "guard", "run", "--state-dir", $guardRunStateDir) -RedirectStandardOutput $stdoutFile -RedirectStandardError $stderrFile -PassThru
        $deadline = (Get-Date).AddSeconds(20)
        $sawRunning = $false
        while ((Get-Date) -lt $deadline) {
            Start-Sleep -Seconds 2
            $status = Invoke-CliJson @("--output", "json", "guard", "status", "--state-dir", $guardRunStateDir)
            if ($status.ExitCode -eq 0 -and [bool](Get-NestedValue -Object $status.Json -Path @("data", "running"))) {
                $sawRunning = $true
                break
            }
        }
        if (-not $sawRunning) {
            throw "guard run never reached running=true"
        }
        $stop = Invoke-CliJson @("--output", "json", "--yes", "guard", "stop", "--state-dir", $guardRunStateDir)
        Assert-ConfirmedSuccess $stop
        if (-not $process.WaitForExit(20000)) {
            try { $process.Kill() } catch {}
            throw "guard run process did not exit after guard stop"
        }
        if ($process.ExitCode -ne 0) {
            $stderr = Get-Content $stderrFile -Raw
            throw "guard run exited with code $($process.ExitCode)`nstderr:`n$stderr"
        }
        return "guard run foreground lifecycle confirmed"
    }
    finally {
        Remove-Item $stdoutFile, $stderrFile -ErrorAction SilentlyContinue
    }
}

Write-Section "Summary"
foreach ($entry in $Results) {
    $color = if ($entry.Status -eq "FAIL") { "Red" } else { "Green" }
    Write-Host ("[{0}] {1} :: {2}" -f $entry.Status, $entry.Name, $entry.Detail) -ForegroundColor $color
}

Write-Host ""
Write-Host "Passed: $Passed" -ForegroundColor Green
Write-Host "Semantic passes: $SemanticPassed" -ForegroundColor Cyan
Write-Host "Failed: $Failed" -ForegroundColor Red

if ($Failed -gt 0) {
    exit 1
}

exit 0
