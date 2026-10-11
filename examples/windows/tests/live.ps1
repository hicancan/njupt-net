#Requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Interface,
    [Parameter(Mandatory)][string]$FirstAccount,
    [Parameter(Mandatory)][string]$RestoreAccount,
    [Parameter(Mandatory)][string]$Config,
    [Parameter(Mandatory)][string]$Executable,
    [Parameter(Mandatory)][string]$LoginScript,
    [Parameter(Mandatory)][string]$OutputDirectory,
    [string]$IndependentInterface = '',
    [string]$ProxyConfigPath = '',
    [ValidateRange(1, 3)][int]$Rounds = 2,
    [ValidateSet(801, 802, 803, 804)][int]$PortalPort = 801,
    [ValidateRange(1, 300)][int]$TimeoutSeconds = 15,
    [ValidateRange(0, 120)][int]$LoginTimeoutSeconds = 45,
    [ValidateRange(30, 600)][int]$StageTimeoutSeconds = 240
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$encoding = [Text.UTF8Encoding]::new($false)
$pwsh = (Get-Command pwsh -CommandType Application | Select-Object -First 1).Source
$root = [IO.Path]::GetFullPath($OutputDirectory)
$null = [IO.Directory]::CreateDirectory($root)
. (Join-Path (Split-Path -Parent $LoginScript) 'diagnostics.ps1')
$safe = @{ Secrets = [Collections.Generic.List[string]]::new() }
$summary = [ordered]@{
    started_utc = [DateTimeOffset]::UtcNow.ToString('O'); process_id = $PID
    parameters = $PSBoundParameters; source = $null; rounds = @(); error = $null
    restoration = $null; finished_utc = $null; completed = $false
}
$sequence = 0
$source = $null
$settings = $null

function Save-BatchJSON([string]$Name, $Value) {
    $json = Protect-CampusDiagnosticValue $safe $Value | ConvertTo-Json -Depth 40
    $path = Join-Path $root $Name
    [IO.File]::WriteAllText($path + '.part', $json + [Environment]::NewLine, $encoding)
    [IO.File]::Move($path + '.part', $path, $true)
}

function Write-BatchEvent([string]$Kind, $Data) {
    $script:sequence++
    $event = [ordered]@{
        sequence = $script:sequence; utc = [DateTimeOffset]::UtcNow.ToString('O')
        epoch_ms = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds(); event = $Kind
        data = Protect-CampusDiagnosticValue $safe $Data
    }
    [IO.File]::AppendAllText((Join-Path $root 'events.jsonl'), ($event | ConvertTo-Json -Depth 40 -Compress) + [Environment]::NewLine, $encoding)
}

function Get-BatchError($Record) {
    $chain = @()
    $exception = $Record.Exception
    while ($exception) {
        $chain += @{ type = $exception.GetType().FullName; message = $exception.Message }
        $exception = $exception.InnerException
    }
    return @{ message = $Record.Exception.Message; chain = $chain; position = $Record.InvocationInfo.PositionMessage; stack = $Record.ScriptStackTrace }
}

function Invoke-BatchProcess([string]$Program, [string[]]$Arguments, [string]$Directory) {
    $null = [IO.Directory]::CreateDirectory($Directory)
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $Program; $start.WorkingDirectory = Split-Path -Parent $Config
    $start.UseShellExecute = $false; $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true; $start.RedirectStandardError = $true
    $start.StandardOutputEncoding = $encoding; $start.StandardErrorEncoding = $encoding
    foreach ($argument in $Arguments) { $start.ArgumentList.Add($argument) }
    $process = [Diagnostics.Process]::new(); $process.StartInfo = $start
    $started = $false
    $timer = [Diagnostics.Stopwatch]::StartNew()
    try {
        $started = $process.Start()
        if (-not $started) { throw 'The child process did not start.' }
        Write-BatchEvent 'process-start' @{ process_id = $process.Id; program = $Program; arguments = $Arguments; directory = $Directory }
        $stdoutTask = $process.StandardOutput.ReadToEndAsync()
        $stderrTask = $process.StandardError.ReadToEndAsync()
        $timedOut = -not $process.WaitForExit($StageTimeoutSeconds * 1000)
        if ($timedOut) { $process.Kill($true); $process.WaitForExit() }
        $stdout = $stdoutTask.GetAwaiter().GetResult(); $stderr = $stderrTask.GetAwaiter().GetResult()
        [IO.File]::WriteAllText((Join-Path $Directory 'process.stdout.txt'), (Protect-CampusDiagnosticValue $safe $stdout), $encoding)
        [IO.File]::WriteAllText((Join-Path $Directory 'process.stderr.txt'), (Protect-CampusDiagnosticValue $safe $stderr), $encoding)
        $result = @{ exit_code = $process.ExitCode; elapsed_ms = $timer.Elapsed.TotalMilliseconds; timed_out = $timedOut; stdout = $stdout; stderr = $stderr }
        Write-BatchEvent 'process-exit' @{ process_id = $process.Id; exit_code = $result.exit_code; elapsed_ms = $result.elapsed_ms; timed_out = $timedOut; directory = $Directory }
        return $result
    } finally {
        if ($started -and -not $process.HasExited) { $process.Kill($true); $process.WaitForExit() }
        $process.Dispose()
    }
}

function Invoke-BatchCLI([string]$Stage, [string]$Alias, [string[]]$Arguments, [string]$QuerySource = '') {
    $directory = Join-Path $root ('observations/{0:D3}-{1}' -f ($script:sequence + 1), $Stage)
    $trace = Join-Path $directory 'http.jsonl'
    $globalFlags = @('--config', $Config, '--timeout', "$($TimeoutSeconds)s", '--trace', $trace)
    $requestSource = if ($QuerySource) { $QuerySource } else { $script:source }
    if ($requestSource) { $globalFlags += @('--source', $requestSource) }
    if ($Alias) { $globalFlags += @('--account', $Alias) }
    $process = Invoke-BatchProcess $Executable ($globalFlags + $Arguments) $directory
    $response = $null
    $stream = if ($process.exit_code -eq 0) { $process.stdout } else { $process.stderr }
    try { $response = $stream | ConvertFrom-Json -AsHashtable -ErrorAction Stop } catch { }
    $result = @{ stage = $Stage; account_alias = $Alias; source = $requestSource; exit_code = $process.exit_code; response = $response; timed_out = $process.timed_out; log_directory = $directory }
    Write-BatchEvent 'cli-observation' $result
    return $result
}

function Read-BatchState([string]$Stage) {
    $state = [ordered]@{ stage = $Stage; portal = $null; bindings = [ordered]@{}; connections = [ordered]@{}; probe = $null }
    $state.portal = Invoke-BatchCLI ($Stage + '-portal') '' @('p', 'status', '--port', [string]$PortalPort)
    foreach ($alias in @($FirstAccount, $RestoreAccount)) {
        $state.bindings[$alias] = Invoke-BatchCLI ($Stage + '-binding-' + $alias) $alias @('zfw', 'operator')
        $state.connections[$alias] = Invoke-BatchCLI ($Stage + '-online-' + $alias) $alias @('zfw', 'online')
    }
    $state.probe = Invoke-BatchCLI ($Stage + '-internet') '' @('probe')
    Save-BatchJSON ($Stage + '.json') $state
    return $state
}

function Assert-BatchState($State, [string]$Alias) {
    $portal = $State.portal.response
    if ($State.portal.exit_code -ne 0 -or $portal.data.online -ne $true -or $portal.data.source -cne $script:source) { throw "Portal state was not confirmed for $Alias." }
    $identity = [regex]::Match($portal.data.terminal.account, '^(?:,[01],)?([^,@]+)(?:@(njxy|cmcc))?$')
    if (-not $identity.Success -or $identity.Groups[1].Value -cne $settings.accounts[$Alias].account -or $identity.Groups[2].Value -cne $settings.broadband_account.operator) { throw "Portal identity did not match $Alias." }
    $operator = $settings.broadband_account.operator
    foreach ($key in @($FirstAccount, $RestoreAccount)) {
        $response = $State.bindings[$key].response
        if ($State.bindings[$key].exit_code -ne 0) { throw "Cannot verify the broadband binding of $key." }
        $binding = $response.data[$operator]
        if ($key -ceq $Alias) {
            if ($binding.account -cne $settings.broadband_account.account -or $binding.password_set -ne $true) { throw "Broadband binding did not match $Alias." }
        } elseif ($binding.account -ceq $settings.broadband_account.account) { throw 'The tested broadband has multiple holders.' }
    }
    if ($State.probe.exit_code -ne 0 -or $State.probe.response.data.internet -ne $true -or $State.probe.response.data.source -cne $script:source) { throw "Physical campus Internet access was not confirmed for $Alias." }
}

function Invoke-BatchLogin([string]$Stage, [string]$Alias) {
    $directory = Join-Path $root $Stage
    $arguments = @('-NoLogo', '-NoProfile', '-NonInteractive', '-File', $LoginScript, '-Source', $script:source, '-Account', $Alias, '-Config', $Config, '-Executable', $Executable, '-PortalPort', [string]$PortalPort, '-TimeoutSeconds', [string]$TimeoutSeconds, '-LoginTimeoutSeconds', [string]$LoginTimeoutSeconds, '-LogDirectory', (Join-Path $directory 'runs'))
    if ($ProxyConfigPath) { $arguments += @('-ProxyConfigPath', $ProxyConfigPath) }
    Write-BatchEvent 'login-start' @{ stage = $Stage; account_alias = $Alias }
    $process = Invoke-BatchProcess $pwsh $arguments $directory
    $runDirectory = @(Get-ChildItem -LiteralPath (Join-Path $directory 'runs') -Directory -ErrorAction SilentlyContinue)
    $result = $null
    if ($runDirectory.Count -eq 1 -and (Test-Path -LiteralPath (Join-Path $runDirectory[0].FullName 'result.json'))) { $result = Get-Content -LiteralPath (Join-Path $runDirectory[0].FullName 'result.json') -Raw | ConvertFrom-Json -AsHashtable }
    $record = @{ stage = $Stage; account_alias = $Alias; exit_code = $process.exit_code; timed_out = $process.timed_out; elapsed_ms = $process.elapsed_ms; log_directory = $directory; result = $result }
    Write-BatchEvent 'login-exit' $record
    return $record
}

try {
    Write-BatchEvent 'batch-start' @{ process_id = $PID; script = $PSCommandPath; parameters = $PSBoundParameters }
    $settings = Get-Content -LiteralPath $Config -Raw | ConvertFrom-Json -AsHashtable
    Register-CampusDiagnosticSecrets $safe $settings
    if ($FirstAccount -ceq $RestoreAccount) { throw 'The two test accounts must be distinct.' }
    foreach ($alias in @($FirstAccount, $RestoreAccount)) {
        if (-not $settings.accounts.Contains($alias) -or -not $settings.accounts[$alias].account -or -not $settings.accounts[$alias].password) { throw "Configured credentials are missing for $alias." }
    }
    $summary.identities = @{ $FirstAccount = $settings.accounts[$FirstAccount].account; $RestoreAccount = $settings.accounts[$RestoreAccount].account; operator = $settings.broadband_account.operator }
    $summary.files = @(foreach ($path in @($PSCommandPath, $Executable, $LoginScript, (Join-Path (Split-Path -Parent $LoginScript) 'login.ps1'), (Join-Path (Split-Path -Parent $LoginScript) 'diagnostics.ps1'))) { Get-CampusDiagnosticFileIdentity $path })
    $interfaces = Invoke-BatchCLI 'interfaces' '' @('interfaces')
    if ($interfaces.exit_code -ne 0) { throw 'Cannot enumerate campus interfaces.' }
    $selected = @($interfaces.response.data | Where-Object { $_.name -ceq $Interface -and $_.up -eq $true })
    if ($selected.Count -ne 1 -or @($selected[0].ipv4).Count -ne 1) { throw 'The campus interface must have one active IPv4 address.' }
    $script:source = $selected[0].ipv4[0]; $summary.source = $script:source
    $snapshotState = @{ Parameters = @{ interface = $Interface; source = $script:source; proxy_config_path = $ProxyConfigPath } }
    Save-BatchJSON 'network-before.json' (Get-CampusDiagnosticSnapshot $snapshotState)
    if ($IndependentInterface) {
        $independent = @($interfaces.response.data | Where-Object { $_.name -ceq $IndependentInterface -and $_.up -eq $true })
        if ($independent.Count -eq 1 -and @($independent[0].ipv4).Count -eq 1) {
            $summary.independent_network = Invoke-BatchCLI 'independent-internet' '' @('probe') $independent[0].ipv4[0]
        } else { $summary.independent_network = @{ available = $false; interface = $IndependentInterface } }
    }
    Save-BatchJSON 'result.json' $summary
    $initial = Read-BatchState 'initial'
    $summary.initial = $initial
    if ($initial.portal.exit_code -ne 0) { throw 'Campus portal precheck failed.' }
    foreach ($round in 1..$Rounds) {
        foreach ($alias in @($FirstAccount, $RestoreAccount)) {
            $stage = 'round-{0:D2}-{1}' -f $round, $alias
            $before = Read-BatchState ($stage + '-before')
            $login = Invoke-BatchLogin $stage $alias
            $after = Read-BatchState ($stage + '-after')
            $entry = @{ round = $round; account_alias = $alias; login = $login; before = $before; after = $after; verified = $false }
            $summary.rounds += $entry
            Save-BatchJSON 'result.json' $summary
            if ($login.exit_code -ne 0) {
                $message = if ($login.result -and $login.result.Contains('error')) { $login.result.error.message } else { 'No successful login result was produced.' }
                throw "Login $alias failed: $message"
            }
            if (-not $login.result -or $login.result.data.online -ne $true -or $login.result.data.account_alias -cne $alias -or $login.result.data.source -cne $script:source) { throw "Login result did not confirm $alias." }
            Assert-BatchState $after $alias
            $entry.verified = $true
            Write-BatchEvent 'round-verified' @{ round = $round; account_alias = $alias; source = $script:source }
            Save-BatchJSON 'result.json' $summary
        }
    }
    $summary.completed = $true
} catch {
    $summary.error = Get-BatchError $_
    Write-BatchEvent 'batch-error' $summary.error
} finally {
    if ($script:source -and $settings) {
        try {
            $restore = Invoke-BatchLogin 'restore' $RestoreAccount
            $summary.restoration = @{ login = $restore; state = $null; verified = $false; error = $null }
            Save-BatchJSON 'result.json' $summary
            $state = Read-BatchState 'final'
            $summary.restoration.state = $state
            if ($restore.exit_code -ne 0) { throw 'The final recovery login failed.' }
            Assert-BatchState $state $RestoreAccount
            $summary.restoration.verified = $true
            Write-BatchEvent 'restore-verified' @{ account_alias = $RestoreAccount; source = $script:source }
        } catch {
            if (-not $summary.restoration) { $summary.restoration = @{ verified = $false; error = $null } }
            $summary.restoration.error = Get-BatchError $_
            Write-BatchEvent 'restore-error' $summary.restoration.error
        }
    } else { $summary.restoration = @{ verified = $false; skipped = 'Campus source was not selected; no login was executed.' } }
    try {
        $snapshotState = @{ Parameters = @{ interface = $Interface; source = $script:source; proxy_config_path = $ProxyConfigPath } }
        Save-BatchJSON 'network-after.json' (Get-CampusDiagnosticSnapshot $snapshotState)
    } catch { Write-BatchEvent 'final-snapshot-error' (Get-BatchError $_) }
    $summary.finished_utc = [DateTimeOffset]::UtcNow.ToString('O')
    Save-BatchJSON 'result.json' $summary
    Write-BatchEvent 'batch-exit' @{ completed = $summary.completed; restoration_verified = $summary.restoration.verified }
}
if (-not $summary.completed -or -not $summary.restoration.verified) { exit 1 }
exit 0
