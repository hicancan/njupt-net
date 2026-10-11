#Requires -Version 7.0
<#
.SYNOPSIS
Use a selected campus account, moving the configured broadband binding when needed.
.EXAMPLE
./login.ps1 -Interface Ethernet -Account default -Config ./config.json
#>
[CmdletBinding(DefaultParameterSetName = 'Interface')]
param(
    [Parameter(Mandatory, ParameterSetName = 'Interface')][ValidateNotNullOrEmpty()][string]$Interface,
    [Parameter(Mandatory, ParameterSetName = 'Source')][ValidateNotNullOrEmpty()][string]$Source,
    [Parameter(Mandatory)][ValidateNotNullOrEmpty()][string]$Account,
    [ValidateNotNullOrEmpty()][string]$Config = 'config.json',
    [ValidateNotNullOrEmpty()][string]$Executable = 'njupt-net',
    [ValidateSet(801, 802, 803, 804)][int]$PortalPort = 801,
    [ValidateRange(1, 300)][int]$TimeoutSeconds = 15,
    [ValidateRange(0, 120)][int]$LoginTimeoutSeconds = 45,
    [switch]$Probe,
    [Parameter(DontShow)][object]$Diagnostics = $null
)
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
function New-CampusContext {
    param([string]$Command, [string]$Executable, [string]$Config, [string]$Interface, [string]$Source, [int]$TimeoutSeconds, [int]$PortalPort)
    $context = @{
        Command = $Command; Executable = $Executable; Config = $Config
        Interface = $Interface; Source = $Source; TimeoutSeconds = $TimeoutSeconds
        Stage = 'configuration'; Steps = [System.Collections.Generic.List[object]]::new()
        Settings = $null; ConfigFile = $null
        Mutex = $null; MutexOwned = $false; Online = $null; Internet = $null; InternetError = $null
        Session = $null; SessionErrors = $null; PortalPort = $PortalPort
        Diagnostics = $Diagnostics
    }
    if ($Diagnostics) { $context.CommandID = 0 }
    return $context
}

function Write-CampusDiagnostic {
    param($Context, [string]$Kind, $Data)
    if (-not $Context.Diagnostics) { return }
    try { $null = $Context.Diagnostics.Emit.Invoke($Kind, $Data) }
    catch {
        $message = Protect-CampusMessage $Context $_.Exception.Message
        $Context.Diagnostics.Errors.Add($message)
        try { $null = $Context.Diagnostics.Note.Invoke("诊断日志写入失败：$message") }
        catch { [Console]::Error.WriteLine("诊断日志写入失败：$message") }
    }
}

function Initialize-CampusContext {
    param($Context, [string[]]$Aliases)
    try {
        $file = Get-Item -LiteralPath $Context.Config -ErrorAction Stop
        if ($file -isnot [System.IO.FileInfo]) { throw 'Configuration must be a filesystem file.' }
        $Context.Config = $file.FullName
        $Context.ConfigFile = [System.IO.File]::Open($Context.Config, [System.IO.FileMode]::Open, [System.IO.FileAccess]::Read, [System.IO.FileShare]::Read)
        $reader = [System.IO.StreamReader]::new($Context.ConfigFile, [System.Text.Encoding]::UTF8, $true, 1024, $true)
        try { $Context.Settings = $reader.ReadToEnd() | ConvertFrom-Json -AsHashtable -ErrorAction Stop }
        finally { $reader.Dispose() }
    } catch { throw 'Cannot read the campus account configuration as JSON.' }
    $settings = $Context.Settings
    if ($Context.Diagnostics) { $null = $Context.Diagnostics.RegisterSecrets.Invoke($settings) }
    if ($settings -isnot [System.Collections.IDictionary] -or -not $settings.Contains('accounts') -or $settings.accounts -isnot [System.Collections.IDictionary]) {
        throw 'Configuration requires an accounts object.'
    }
    foreach ($alias in $Aliases) {
        if (-not $settings.accounts.Contains($alias)) { throw "Account alias '$alias' is not configured." }
        $credential = $settings.accounts[$alias]
        if ($credential -isnot [System.Collections.IDictionary] -or -not $credential.Contains('account') -or -not $credential.Contains('password') -or
            $credential.account -isnot [string] -or $credential.password -isnot [string] -or
            [string]::IsNullOrEmpty($credential.account) -or [string]::IsNullOrEmpty($credential.password) -or $credential.account -match '[,@]') {
            throw "Account alias '$alias' requires a base campus account and password."
        }
    }
    $application = Get-Command -Name $Context.Executable -CommandType Application -ErrorAction Stop | Select-Object -First 1
    $Context.Executable = $application.Source
    $interfaces = Invoke-CampusCommand $Context 'interfaces' '' @('interfaces')
    if ($interfaces -isnot [array]) { throw 'CLI interfaces did not return an array.' }
    if ($Context.Interface) {
        $matches = @($interfaces | Where-Object { $_.name -ceq $Context.Interface })
        if ($matches.Count -ne 1 -or $matches[0].up -ne $true -or @($matches[0].ipv4).Count -ne 1) {
            throw 'Select an active interface with exactly one IPv4, or specify -Source.'
        }
        $Context.Source = [string]$matches[0].ipv4[0]
    } else {
        $matches = @($interfaces | Where-Object { $_.up -eq $true -and $Context.Source -cin @($_.ipv4) })
        if ($matches.Count -ne 1) { throw 'Source IPv4 must belong to exactly one active interface.' }
    }
    $address = $null
    if (-not [System.Net.IPAddress]::TryParse($Context.Source, [ref]$address) -or
        $address.AddressFamily -ne [System.Net.Sockets.AddressFamily]::InterNetwork -or
        [System.Net.IPAddress]::IsLoopback($address) -or $address.Equals([System.Net.IPAddress]::Any)) {
        throw 'Select an assigned, non-loopback IPv4 source.'
    }
    $Context.Source = $address.ToString()
    $Context.Stage = 'lock'
    $Context.Mutex = [System.Threading.Mutex]::new($false, "njupt-net.source.$($Context.Source)")
    try { $Context.MutexOwned = $Context.Mutex.WaitOne(0) }
    catch [System.Threading.AbandonedMutexException] { $Context.MutexOwned = $true }
    if (-not $Context.MutexOwned) { throw 'Another campus workflow is using this source IPv4.' }
    if ($Context.Diagnostics) {
        try { $null = Invoke-CampusCommand $Context 'diagnostic-version' '' @('version') -Result -Observation }
        catch { Write-CampusDiagnostic $Context 'observation-error' @{ command = 'version'; message = $_.Exception.Message } }
    }
}

function Close-CampusContext {
    param($Context)
    if ($Context.Session) {
        try {
            if (-not $Context.Session.HasExited) { $Context.Session.Kill($true); $Context.Session.WaitForExit() }
        } finally { $Context.Session.Dispose(); $Context.Session = $null }
    }
    if ($Context.Mutex) {
        try { if ($Context.MutexOwned) { $Context.Mutex.ReleaseMutex() } }
        finally { $Context.Mutex.Dispose() }
    }
    if ($Context.ConfigFile) { $Context.ConfigFile.Dispose() }
}

function Start-CampusSession {
    param($Context)
    $start = [System.Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $Context.Executable
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardInput = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $start.StandardInputEncoding = [System.Text.UTF8Encoding]::new($false)
    $start.StandardOutputEncoding = [System.Text.UTF8Encoding]::new($false)
    $start.StandardErrorEncoding = [System.Text.UTF8Encoding]::new($false)
    $selection = if ($Context.Interface) { @('--interface', $Context.Interface) } else { @('--source', $Context.Source) }
    $arguments = $selection + @('--config', $Context.Config, '--timeout', "$($Context.TimeoutSeconds)s")
    if ($Context.Diagnostics) { $arguments += @('--trace', $Context.Diagnostics.TraceFile) }
    foreach ($argument in ($arguments + @('session'))) {
        $start.ArgumentList.Add($argument)
    }
    $process = [System.Diagnostics.Process]::new()
    $process.StartInfo = $start
    try {
        if (-not $process.Start()) { throw 'Could not start the CLI session.' }
        $Context.Session = $process
        $Context.SessionErrors = $process.StandardError.ReadToEndAsync()
        if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'session-start' @{ process_id = $process.Id; executable = $Context.Executable; args = @($start.ArgumentList) } }
    } catch { $process.Dispose(); throw }
}

function Close-CampusSession {
    param($Context)
    if (-not $Context.Session) { return }
    $process = $Context.Session
    if ($Context.Diagnostics) {
        $Context.CommandID++
        $stepID = [string]$Context.CommandID
        $startedAt = [System.Diagnostics.Stopwatch]::GetTimestamp()
        Write-CampusDiagnostic $Context 'session-close-start' @{ step_id = $stepID; process_id = $process.Id }
    }
    try {
        if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'cli-request' @{ step_id = $stepID; process_id = $process.Id; request = @{ args = @('close') } } }
        $process.StandardInput.WriteLine('{"args":["close"]}')
        $process.StandardInput.Flush()
        $line = $process.StandardOutput.ReadLine()
        $response = ConvertFrom-Json -InputObject $line -AsHashtable -ErrorAction Stop
        $process.StandardInput.Close()
        $extraTask = $process.StandardOutput.ReadToEndAsync()
        $process.WaitForExit()
        $extra = $extraTask.GetAwaiter().GetResult()
        $errors = $Context.SessionErrors.GetAwaiter().GetResult()
        if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'cli-response' @{ step_id = $stepID; process_id = $process.Id; response = $response; elapsed_ms = [System.Diagnostics.Stopwatch]::GetElapsedTime($startedAt).TotalMilliseconds } }
        if ($response.command -cne 'close' -or $response.exit_code -ne 0 -or $process.ExitCode -ne 0 -or
            $response.Contains('error') -or $response.data.closed -ne $true -or
            -not [string]::IsNullOrWhiteSpace($extra) -or -not [string]::IsNullOrWhiteSpace($errors)) {
            $message = 'CLI management session cleanup failed.'
            if ($response.command -ceq 'close' -and $response.Contains('error') -and
                $response.error -is [System.Collections.IDictionary] -and $response.error.message -is [string]) {
                $message += ' ' + (Protect-CampusMessage $Context $response.error.message)
            }
            throw $message
        }
    } finally {
        if (-not $process.HasExited) { $process.Kill($true); $process.WaitForExit() }
        if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'session-close' @{ step_id = $stepID; process_id = $process.Id; exit_code = $process.ExitCode; elapsed_ms = [System.Diagnostics.Stopwatch]::GetElapsedTime($startedAt).TotalMilliseconds } }
        $process.Dispose()
        $Context.Session = $null
    }
}

function Protect-CampusMessage {
    param($Context, [string]$Message)
    if ($Context -and $Context.Settings -is [System.Collections.IDictionary]) {
        $settings = $Context.Settings
        $credentials = @()
        if ($settings.Contains('accounts') -and $settings.accounts -is [System.Collections.IDictionary]) { $credentials += @($settings.accounts.Values) }
        if ($settings.Contains('broadband_account')) { $credentials += $settings.broadband_account }
        foreach ($credential in $credentials) {
            if ($credential -is [System.Collections.IDictionary] -and $credential.Contains('password') -and $credential.password -is [string] -and $credential.password.Length -gt 0) {
                $Message = $Message.Replace($credential.password, '[password]')
            }
        }
    }
    return $Message
}

function Invoke-CampusCommand {
    param($Context, [string]$Stage, [string]$AccountAlias, [string[]]$Arguments, [switch]$ObserveProbeFailure, [switch]$Result, [switch]$Observation)
    if (-not $Observation) { $Context.Stage = $Stage }
    if ($Arguments[0] -ceq 'p') { $Arguments += @('--port', [string]$Context.PortalPort) }
    $command = $Arguments[0]
    if ($command -in @('p', 'zfw')) { $command += ' ' + $Arguments[1] }
    if ($ObserveProbeFailure -and $command -cne 'probe') { throw 'Only the Internet probe can report an observation failure.' }
    if ($command -in @('p login', 'zfw offline')) { $Context.Online = $null; $Context.Internet = $null; $Context.InternetError = $null }
    $event = [ordered]@{ stage = $Stage; command = $command; account_alias = $AccountAlias; exit_code = $null; outcome = $null; verified = $null; elapsed_ms = $null }
    if (-not $Observation) { $Context.Steps.Add($event) }
    $startedAt = [System.Diagnostics.Stopwatch]::GetTimestamp()
    if ($Context.Diagnostics) {
        $Context.CommandID++
        $stepID = [string]$Context.CommandID
        Write-CampusDiagnostic $Context 'step-start' @{ step_id = $stepID; stage = $Stage; command = $command; account_alias = $AccountAlias; observation = [bool]$Observation }
    }
    if (-not $Context.Session) { Start-CampusSession $Context }
    $request = @{ account = $AccountAlias; args = @($Arguments) } | ConvertTo-Json -Compress
    if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'cli-request' @{ step_id = $stepID; process_id = $Context.Session.Id; request = @{ account = $AccountAlias; args = @($Arguments) }; observation = [bool]$Observation } }
    try {
        $Context.Session.StandardInput.WriteLine($request)
        $Context.Session.StandardInput.Flush()
        $json = $Context.Session.StandardOutput.ReadLine()
    } catch [System.IO.IOException] { $json = $null }
    if ($null -eq $json) {
        $process = $Context.Session
        $process.WaitForExit()
        $errors = $Context.SessionErrors.GetAwaiter().GetResult()
        $event.exit_code = $process.ExitCode
        $event.elapsed_ms = [Math]::Round([System.Diagnostics.Stopwatch]::GetElapsedTime($startedAt).TotalMilliseconds, 3)
        $process.Dispose()
        $Context.Session = $null
        $failure = $null
        try { $failure = ConvertFrom-Json -InputObject $errors -AsHashtable -ErrorAction Stop } catch { }
        if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'cli-ended' @{ step_id = $stepID; exit_code = $event.exit_code; error_response = $failure; elapsed_ms = $event.elapsed_ms } }
        if ($failure -is [System.Collections.IDictionary] -and $failure.Contains('command') -and $failure.command -ceq 'session' -and
            $failure.Contains('error') -and $failure.error -is [System.Collections.IDictionary] -and
            $failure.error.Contains('message') -and $failure.error.message -is [string] -and -not [string]::IsNullOrWhiteSpace($failure.error.message)) {
            throw (Protect-CampusMessage $Context $failure.error.message)
        }
        throw "CLI session ended before '$command' returned a result."
    }
    $event.elapsed_ms = [Math]::Round([System.Diagnostics.Stopwatch]::GetElapsedTime($startedAt).TotalMilliseconds, 3)
    try { $envelope = ConvertFrom-Json -InputObject $json -AsHashtable -ErrorAction Stop }
    catch {
        if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'cli-invalid-response' @{ step_id = $stepID; command = $command; response_characters = $json.Length; elapsed_ms = $event.elapsed_ms } }
        throw "CLI '$command' did not return one JSON result."
    }
    if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'cli-response' @{ step_id = $stepID; command = $command; response = $envelope; observation = [bool]$Observation; elapsed_ms = $event.elapsed_ms } }
    if ($envelope -isnot [System.Collections.IDictionary] -or -not $envelope.Contains('command') -or $envelope.command -cne $command -or -not $envelope.Contains('data')) {
        throw "CLI '$command' returned an unexpected result envelope."
    }
    if (-not $envelope.Contains('exit_code') -or $envelope.exit_code -isnot [long] -and $envelope.exit_code -isnot [int] -or
        $envelope.exit_code -notin @(0, 1, 2)) { throw "CLI '$command' returned an invalid exit code." }
    $event.exit_code = $envelope.exit_code
    $data = $envelope.data
    if ($data -is [System.Collections.IDictionary]) {
        if ($data.Contains('outcome')) { $event.outcome = $data.outcome }
        if ($data.Contains('verified')) { $event.verified = $data.verified }
    }
    if ($event.exit_code -ne 0) {
        if ($envelope.Contains('error') -and $envelope.error -is [System.Collections.IDictionary] -and $envelope.error.Contains('message') -and $envelope.error.message -is [string]) {
            $message = Protect-CampusMessage $Context $envelope.error.message
            if ($ObserveProbeFailure -and $event.exit_code -eq 1 -and -not [string]::IsNullOrWhiteSpace($message)) {
                $Context.InternetError = $message
                return ,$data
            }
            if ($Result -and -not [string]::IsNullOrWhiteSpace($message)) {
                $envelope.error.message = $message
                return ,$envelope
            }
            throw $message
        }
        throw "CLI '$command' failed with exit code $($event.exit_code)."
    }
    if ($envelope.Contains('error')) { throw "CLI '$command' returned an error alongside success." }
    if ($Result) { return ,$envelope }
    return ,$data
}

function Get-CampusStatus {
    param($Context, [string]$Stage)
    $state = Invoke-CampusCommand $Context $Stage '' @('p', 'status')
    return Confirm-CampusStatus $Context $state
}

function Confirm-CampusStatus {
    param($Context, $State)
    $state = $State
    if ($state -isnot [System.Collections.IDictionary] -or -not $state.Contains('online') -or $state.online -isnot [bool] -or
        -not $state.Contains('source') -or $state.source -cne $Context.Source -or -not $state.Contains('terminal')) { throw 'Portal status does not describe the selected source.' }
    if ($state.online) {
        if ($state.terminal -isnot [System.Collections.IDictionary] -or $state.terminal.ip -cne $Context.Source -or [string]::IsNullOrEmpty($state.terminal.account)) {
            throw 'Portal online identity is incomplete.'
        }
        $null = ConvertTo-CampusMac $state.terminal.mac
    } elseif ($null -ne $state.terminal) { throw 'Offline portal status unexpectedly contains a terminal.' }
    $Context.Online = $state.online
    return $state
}

function Assert-CampusIdentity {
    param($Context, $State, [string]$AccountAlias, [string]$Operator = '')
    if (-not $State.online) { throw 'The selected terminal is offline.' }
    $identity = [regex]::Match($State.terminal.account, '^(?:,[01],)?([^,@]+)(?:@(njxy|cmcc))?$')
    if (-not $identity.Success -or $identity.Groups[1].Value -cne $Context.Settings.accounts[$AccountAlias].account) { throw "Portal identity does not match account alias '$AccountAlias'." }
    $actualOperator = if ($identity.Groups[2].Success) { $identity.Groups[2].Value } else { 'campus' }
    if ($Operator -and $actualOperator -cne $Operator) { throw 'Portal operator does not match the requested operator.' }
}

function ConvertTo-CampusMac {
    param([string]$Mac)
    $normalized = $Mac.Replace(':', '').Replace('-', '').ToLowerInvariant()
    if ($normalized -cnotmatch '^[0-9a-f]{12}$') { throw 'Terminal MAC address is invalid.' }
    return $normalized
}

function Get-CampusConnection {
    param($Context, [string]$AccountAlias, $State)
    $rows = Invoke-CampusCommand $Context 'connection' $AccountAlias @('zfw', 'online')
    if ($rows -isnot [array]) { throw 'Self online connections did not return an array.' }
    $mac = ConvertTo-CampusMac $State.terminal.mac
    $matches = @($rows | Where-Object { $_.ip -ceq $Context.Source -and (ConvertTo-CampusMac $_.mac) -ceq $mac })
    if ($matches.Count -ne 1 -or $matches[0].session_id -isnot [string] -or [string]::IsNullOrEmpty($matches[0].session_id)) { throw 'Expected exactly one Self session matching this terminal IP and MAC.' }
    return $matches[0]
}

function Get-CampusBindings {
    param($Context, [string]$Stage, [string]$AccountAlias)
    $bindings = Invoke-CampusCommand $Context $Stage $AccountAlias @('zfw', 'operator')
    foreach ($operator in @('njxy', 'cmcc')) {
        if ($bindings -isnot [System.Collections.IDictionary] -or -not $bindings.Contains($operator) -or $bindings[$operator] -isnot [System.Collections.IDictionary] -or
            -not $bindings[$operator].Contains('account') -or $bindings[$operator].account -isnot [string] -or
            -not $bindings[$operator].Contains('password_set') -or $bindings[$operator].password_set -isnot [bool]) { throw 'CLI returned incomplete operator bindings.' }
    }
    return $bindings
}

function Assert-CampusOperation {
    param($Result)
    if ($Result -isnot [System.Collections.IDictionary] -or -not $Result.Contains('outcome') -or $Result.outcome -cne 'accepted' -or
        -not $Result.Contains('verified') -or $Result.verified -isnot [bool] -or -not $Result.verified) { throw 'The operation was not accepted and verified.' }
}

function Connect-CampusTerminal {
    param($Context, [string]$AccountAlias, [string]$Operator, [int]$LoginTimeoutSeconds, $Details)
    $timer = [System.Diagnostics.Stopwatch]::StartNew()
    $lastError = ''
    try {
        while ($true) {
            if ($Details.login_attempts -gt 0 -and $timer.Elapsed.TotalSeconds -ge $LoginTimeoutSeconds) {
                $Context.Stage = 'login'
                throw $lastError
            }
            $attemptStarted = [System.Diagnostics.Stopwatch]::GetTimestamp()
            $Details.login_attempts++
            $response = Invoke-CampusCommand $Context 'login' $AccountAlias @('p', 'login', '--operator', $Operator) -Result
            if ($response.exit_code -eq 0) {
                Assert-CampusOperation $response.data
                if (-not $response.data.Contains('status')) { throw 'Verified login did not return the terminal status.' }
                return Confirm-CampusStatus $Context $response.data.status
            }
            $lastError = $response.error.message
            $result = $response.data
            if ($response.exit_code -ne 1 -or $result -isnot [System.Collections.IDictionary] -or
                -not $result.Contains('outcome') -or $result.outcome -cne 'rejected' -or
                -not $result.Contains('verified') -or $result.verified -isnot [bool] -or $result.verified) { throw $lastError }
            $code = ''
            if ($result.Contains('ret_code') -and ($result.ret_code -is [int] -or $result.ret_code -is [long] -or $result.ret_code -is [string])) { $code = [string]$result.ret_code }
            $message = if ($result.Contains('message') -and $result.message -is [string]) { $result.message } else { '' }
            $reason = switch ($code) {
                '2' { 'already-online' }
                '3' { 'server-busy' }
                '8' { 'authentication-timeout' }
                '1' {
                    if ($message -ceq '未绑定运营商账号,请正确绑定运营商账号再试！') { 'binding-pending' }
                    elseif ($message -ceq '从Radius获取错误代码失败！') { 'radius-error-unavailable' }
                }
            }
            if (-not $reason -or ($reason -cne 'already-online' -and $timer.Elapsed.TotalSeconds -ge $LoginTimeoutSeconds)) { throw $lastError }
            $state = Get-CampusStatus $Context 'login-recovery-status'
            if ($state.online) {
                Assert-CampusIdentity $Context $state $AccountAlias $Operator
                if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'login-recovery' @{ attempt = $Details.login_attempts; reason = $reason; decision = 'online-confirmed'; ret_code = $result.ret_code; message = $message; elapsed_ms = $timer.Elapsed.TotalMilliseconds } }
                return $state
            }
            if ($reason -ceq 'already-online') { $Context.Stage = 'login'; throw $lastError }
            $interval = if ($Details.login_attempts -eq 1) { 0 } elseif ($reason -ceq 'binding-pending') { 50 } else { [Math]::Min(1000, 100 * [Math]::Pow(2, [Math]::Min(4, $Details.login_attempts - 2))) }
            $remaining = $LoginTimeoutSeconds * 1000 - $timer.Elapsed.TotalMilliseconds
            $delay = [Math]::Max(0.0, $interval - [System.Diagnostics.Stopwatch]::GetElapsedTime($attemptStarted).TotalMilliseconds)
            if ($remaining -le 0 -or $delay -ge $remaining) { $Context.Stage = 'login'; throw $lastError }
            if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'login-recovery' @{ attempt = $Details.login_attempts; reason = $reason; decision = 'retry'; ret_code = $result.ret_code; message = $message; elapsed_ms = $timer.Elapsed.TotalMilliseconds; remaining_ms = $remaining; delay_ms = $delay } }
            if ($delay -gt 0) { Start-Sleep -Milliseconds ([int][Math]::Ceiling($delay)) }
        }
    } finally {
        $Details.login_seconds = [Math]::Round($timer.Elapsed.TotalSeconds, 3)
    }
}

function Disconnect-CampusTerminal {
    param($Context, [string]$AccountAlias, $State, $Connection)
    $result = Invoke-CampusCommand $Context 'offline' $AccountAlias @('zfw', 'offline', '--session', $Connection.session_id)
    Assert-CampusOperation $result
    if ($result.session_id -cne $Connection.session_id) { throw 'Offline result refers to a different Self session.' }
    $deadline = [System.Diagnostics.Stopwatch]::StartNew()
    do {
        $observedAt = [System.Diagnostics.Stopwatch]::GetTimestamp()
        $observed = Get-CampusStatus $Context 'offline-status'
        if (-not $observed.online) { return }
        Assert-CampusIdentity $Context $observed $AccountAlias
        if ((ConvertTo-CampusMac $observed.terminal.mac) -cne (ConvertTo-CampusMac $State.terminal.mac)) { throw 'Terminal identity changed during disconnection.' }
        if ($deadline.Elapsed.TotalSeconds -ge 10) { throw 'Self session was removed, but the portal still lists the terminal online.' }
        $delay = [Math]::Max(0, 50 - [System.Diagnostics.Stopwatch]::GetElapsedTime($observedAt).TotalMilliseconds)
        if ($delay -gt 0) { Start-Sleep -Milliseconds ([int][Math]::Ceiling($delay)) }
    } while ($true)
}

function Confirm-CampusInternet {
    param($Context)
    $probe = Invoke-CampusCommand $Context 'internet' '' @('probe') -ObserveProbeFailure
    if ($null -eq $probe -and $Context.InternetError) { return }
    if ($probe -isnot [System.Collections.IDictionary] -or -not $probe.Contains('source') -or $probe.source -cne $Context.Source -or
        -not $probe.Contains('internet') -or $probe.internet -isnot [bool]) { throw 'Internet probe did not describe the selected source.' }
    if ($probe.internet -and $Context.InternetError) { throw 'Internet probe returned conflicting success and error results.' }
    $Context.Internet = $probe.internet
    if (-not $probe.internet -and -not $Context.InternetError) { $Context.InternetError = 'The external connectivity probe returned offline.' }
}

function Write-CampusResult {
    param($Context, [System.Collections.IDictionary]$Details, [string]$Message = '')
    $data = [ordered]@{ stage = $Context.Stage; source = $Context.Source; online = $Context.Online; internet = $Context.Internet; internet_error = $Context.InternetError }
    foreach ($key in $Details.Keys) { $data[$key] = $Details[$key] }
    $data.steps = @($Context.Steps.ToArray())
    $result = [ordered]@{ command = $Context.Command; data = $data }
    if ($Message) {
        $result.error = @{ message = Protect-CampusMessage $Context $Message }
        $text = $result | ConvertTo-Json -Depth 12
        if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'result' @{ result = $result; stream = 'stderr'; console = $text; exit_code = 1 } }
        [Console]::Error.WriteLine($text)
    } else {
        $text = $result | ConvertTo-Json -Depth 12
        if ($Context.Diagnostics) { Write-CampusDiagnostic $Context 'result' @{ result = $result; stream = 'stdout'; console = $text; exit_code = 0 } }
        [Console]::Out.WriteLine($text)
    }
}

function Invoke-CampusFailureDiagnostics {
    param($Context)
    if (-not $Context.Diagnostics) { return }
    if (-not $Context.MutexOwned -or -not $Context.Session -or $Context.Session.HasExited) {
        Write-CampusDiagnostic $Context 'observation-skipped' @{ reason = 'No active locked CLI session.' }
        return
    }
    foreach ($command in @('status', 'error')) {
        if (-not $Context.Session -or $Context.Session.HasExited) { break }
        try { $null = Invoke-CampusCommand $Context ('diagnostic-' + $command) '' @('p', $command) -Result -Observation }
        catch { Write-CampusDiagnostic $Context 'observation-error' @{ command = 'p ' + $command; message = $_.Exception.Message } }
    }
}

function Get-CampusPortalAlias {
    param($Context, $State)
    $identity = [regex]::Match($State.terminal.account, '^(?:,[01],)?([^,@]+)(?:@(njxy|cmcc))?$')
    if (-not $identity.Success) { throw 'Portal account identity is invalid.' }
    $matches = @($Context.Settings.accounts.Keys | Where-Object {
        $credential = $Context.Settings.accounts[$_]
        $credential -is [System.Collections.IDictionary] -and $credential.Contains('account') -and $credential.account -ceq $identity.Groups[1].Value
    })
    if ($matches.Count -ne 1) { throw 'Current portal account must match exactly one configured campus account.' }
    return [string]$matches[0]
}

function Test-CampusBroadband {
    param($Bindings, $Broadband)
    return $Bindings[$Broadband.operator].account -ceq $Broadband.account -and $Bindings[$Broadband.operator].password_set -eq $true
}

function Assert-CampusEmptyBinding {
    param($Bindings, [string]$Operator)
    if ($Bindings[$Operator].account -cne '' -or $Bindings[$Operator].password_set -ne $false) {
        throw 'Target operator binding must be empty before assigning the configured broadband.'
    }
}

$context = New-CampusContext 'login' $Executable $Config $Interface $Source $TimeoutSeconds $PortalPort
$details = @{ account_alias = $Account; portal_port = $PortalPort; operator = $null; previous_account_alias = $null; binding_from = $null; binding_moved = $false; login_seconds = 0; login_attempts = 0 }
try {
    Initialize-CampusContext $context @($Account)
    $context.Stage = 'configuration'
    $settings = $context.Settings
    if (-not $settings.Contains('broadband_account') -or $settings.broadband_account -isnot [System.Collections.IDictionary]) {
        throw 'Login requires a configured broadband account.'
    }
    $broadband = $settings.broadband_account
    if (-not $broadband.Contains('operator') -or $broadband.operator -cnotin @('njxy', 'cmcc') -or
        -not $broadband.Contains('account') -or $broadband.account -isnot [string] -or [string]::IsNullOrEmpty($broadband.account) -or
        -not $broadband.Contains('password') -or $broadband.password -isnot [string] -or [string]::IsNullOrEmpty($broadband.password)) {
        throw 'Login requires a broadband operator, account and password.'
    }
    $operator = $broadband.operator
    $details.operator = $operator
    $state = Get-CampusStatus $context 'status'
    $currentAlias = if ($state.online) { Get-CampusPortalAlias $context $state } else { $null }
    $details.previous_account_alias = $currentAlias
    $targetBindings = Get-CampusBindings $context 'target-binding' $Account
    $targetReady = Test-CampusBroadband $targetBindings $broadband
    $holder = $null
    if (-not $targetReady) {
        Assert-CampusEmptyBinding $targetBindings $operator
        $holders = @()
        foreach ($alias in @($settings.accounts.Keys | Sort-Object -CaseSensitive)) {
            if ($alias -ceq $Account) { continue }
            $bindings = Get-CampusBindings $context 'find-binding' $alias
            if ($bindings[$operator].account -ceq $broadband.account) {
                if (-not (Test-CampusBroadband $bindings $broadband)) { throw 'Configured broadband has an incomplete binding.' }
                $holders += $alias
            }
        }
        if ($holders.Count -gt 1) { throw 'Configured broadband has multiple campus account holders.' }
        if ($holders.Count -eq 1) { $holder = [string]$holders[0]; $details.binding_from = $holder }
    }
    $portalReady = $false
    if ($state.online -and $currentAlias -ceq $Account) {
        $identity = [regex]::Match($state.terminal.account, '^(?:,[01],)?([^,@]+)(?:@(njxy|cmcc))?$')
        $portalReady = $identity.Groups[2].Value -ceq $operator
    }
    if (-not $portalReady -or -not $targetReady) {
        if ($state.online -and -not $portalReady) {
            $connection = Get-CampusConnection $context $currentAlias $state
            $targetBefore = Get-CampusBindings $context 'before-offline-target-binding' $Account
            if ($targetReady) {
                if (-not (Test-CampusBroadband $targetBefore $broadband)) { throw 'Target broadband binding changed before disconnection.' }
            } else { Assert-CampusEmptyBinding $targetBefore $operator }
            if ($holder) {
                $holderBefore = Get-CampusBindings $context 'before-offline-holder-binding' $holder
                if (-not (Test-CampusBroadband $holderBefore $broadband)) { throw 'Broadband holder changed before disconnection.' }
            }
            $before = Get-CampusStatus $context 'before-offline'
            Assert-CampusIdentity $context $before $currentAlias
            if ($before.terminal.account -cne $state.terminal.account -or
                (ConvertTo-CampusMac $before.terminal.mac) -cne (ConvertTo-CampusMac $state.terminal.mac)) { throw 'Terminal identity changed before disconnection.' }
            Disconnect-CampusTerminal $context $currentAlias $state $connection
        }
        if (-not $targetReady) {
            if ($holder) {
                $oldCurrent = Get-CampusBindings $context 'check-old-binding' $holder
                if (-not (Test-CampusBroadband $oldCurrent $broadband)) { throw 'Broadband holder changed before unbinding.' }
                $targetCurrent = Get-CampusBindings $context 'check-target-before-unbind' $Account
                Assert-CampusEmptyBinding $targetCurrent $operator
                $cleared = Invoke-CampusCommand $context 'unbind' $holder @('zfw', 'operator', '--unbind', $operator)
                Assert-CampusOperation $cleared
                if ($cleared.operator -cne $operator -or $cleared.account -cne '' -or
                    $cleared.bindings[$operator].account -cne '' -or $cleared.bindings[$operator].password_set -ne $false) { throw 'Old broadband binding was not cleared.' }
            }
            $targetCurrent = Get-CampusBindings $context 'check-target-binding' $Account
            Assert-CampusEmptyBinding $targetCurrent $operator
            $bound = Invoke-CampusCommand $context 'bind' $Account @('zfw', 'operator', '--bind')
            Assert-CampusOperation $bound
            if ($bound.operator -cne $operator -or $bound.account -cne $broadband.account -or
                -not (Test-CampusBroadband $bound.bindings $broadband)) { throw 'Target broadband binding does not match the configured account.' }
            $details.binding_moved = [bool]$holder
        }
        if ($portalReady) {
            $after = Get-CampusStatus $context 'binding-status'
            $portalReady = $after.online
        }
        if (-not $portalReady) {
            $after = Connect-CampusTerminal $context $Account $operator $LoginTimeoutSeconds $details
        }
        Assert-CampusIdentity $context $after $Account $operator
        if ($state.online -and (ConvertTo-CampusMac $after.terminal.mac) -cne (ConvertTo-CampusMac $state.terminal.mac)) { throw 'Terminal MAC changed during login.' }
    }
    if ($Probe) { Confirm-CampusInternet $context }
    $context.Stage = 'session-close'
    Close-CampusSession $context
    $context.Stage = 'complete'
    Write-CampusResult $context $details
    exit 0
} catch {
    $message = $_.Exception.Message
    if ($context.Diagnostics) { Write-CampusDiagnostic $context 'workflow-error' @{ stage = $context.Stage; message = $message; exception_type = $_.Exception.GetType().FullName; stack = $_.ScriptStackTrace } }
    Invoke-CampusFailureDiagnostics $context
    try { Close-CampusSession $context } catch { $message += " Management session cleanup failed: $($_.Exception.Message)" }
    Write-CampusResult $context $details $message
    exit 1
} finally { Close-CampusContext $context }
