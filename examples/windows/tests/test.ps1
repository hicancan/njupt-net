#requires -Version 7.0
[CmdletBinding()]
param(
    [Parameter(Mandatory)]
    [string] $OutputDirectory
)

$ErrorActionPreference = 'Stop'
$repository = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
$outputRoot = [IO.Path]::GetFullPath($OutputDirectory)
if ($outputRoot -eq $repository -or $outputRoot.StartsWith($repository + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'OutputDirectory must be outside the repository.'
}
[IO.Directory]::CreateDirectory($outputRoot) | Out-Null
$temporaryDirectory = Join-Path $outputRoot 'tmp'
[IO.Directory]::CreateDirectory($temporaryDirectory) | Out-Null
$fixtureDirectory = Join-Path $outputRoot '含空格 测试程序'
[IO.Directory]::CreateDirectory($fixtureDirectory) | Out-Null
$executable = Join-Path $fixtureDirectory $(if ($IsWindows) { 'fake cli.exe' } else { 'fake cli' })
$pwsh = (Get-Command pwsh -CommandType Application -ErrorAction Stop).Source
$go = (Get-Command go -CommandType Application -ErrorAction Stop).Source
$secrets = @('CAMPUS_OLD_PASSWORD_73e6', 'CAMPUS_NEW_PASSWORD_82a1', 'BROADBAND_PASSWORD_91f4')
$completed = [Collections.Generic.List[string]]::new()
$caseNumber = 0

function Assert-True([bool] $Condition, [string] $Message) {
    if (-not $Condition) { throw $Message }
}

function Invoke-Process([string] $File, [string[]] $Arguments, [hashtable] $Environment = @{}, [string] $WorkingDirectory = $outputRoot) {
    $start = [Diagnostics.ProcessStartInfo]::new()
    $start.FileName = $File
    $start.WorkingDirectory = $WorkingDirectory
    $start.UseShellExecute = $false
    $start.CreateNoWindow = $true
    $start.RedirectStandardOutput = $true
    $start.RedirectStandardError = $true
    $start.StandardOutputEncoding = [Text.UTF8Encoding]::new($false)
    $start.StandardErrorEncoding = [Text.UTF8Encoding]::new($false)
    foreach ($argument in $Arguments) { $start.ArgumentList.Add($argument) }
    $start.Environment['TEMP'] = $temporaryDirectory
    $start.Environment['TMP'] = $temporaryDirectory
    $start.Environment['GOTMPDIR'] = $temporaryDirectory
    foreach ($name in $Environment.Keys) { $start.Environment[$name] = [string] $Environment[$name] }
    $process = [Diagnostics.Process]::new()
    $process.StartInfo = $start
    try {
        $null = $process.Start()
        $stdout = $process.StandardOutput.ReadToEndAsync()
        $stderr = $process.StandardError.ReadToEndAsync()
        if (-not $process.WaitForExit(45000)) {
            $process.Kill($true)
            throw 'Offline workflow test exceeded 45 seconds.'
        }
        return @{ ExitCode = $process.ExitCode; Stdout = $stdout.GetAwaiter().GetResult(); Stderr = $stderr.GetAwaiter().GetResult() }
    }
    finally { $process.Dispose() }
}

$build = Invoke-Process $go @('build', '-trimpath', '-o', $executable, (Join-Path $PSScriptRoot 'cli.go'))
if ($build.ExitCode -ne 0) { throw "Could not build offline CLI fixture: $($build.Stderr)" }

function New-Case([string] $Scenario = '', [string] $Provider = 'cmcc') {
    $script:caseNumber++
    $directory = Join-Path $outputRoot ('cases/用例 {0:D3}' -f $script:caseNumber)
    [IO.Directory]::CreateDirectory($directory) | Out-Null
    $configuration = @{
        accounts = @{
            old = @{ account = 'old-campus'; password = $secrets[0] }
            new = @{ account = 'new-campus'; password = $secrets[1] }
        }
        broadband_account = @{ operator = $Provider; account = 'broadband-fixture'; password = $secrets[2] }
    }
    $other = $(if ($Provider -eq 'cmcc') { 'njxy' } else { 'cmcc' })
    $state = @{
        scenario = $Scenario; fail_at = 0; failure_mode = ''; calls = @(); writes = @(); offline_ids = @()
        online = $true; online_account = "old-campus@$Provider"; provider = $Provider
        bindings = @{
            old = @{ $Provider = @{ account = 'broadband-fixture'; password_set = $true }; $other = @{ account = 'old-other-provider'; password_set = $true } }
            new = @{ $Provider = @{ account = ''; password_set = $false }; $other = @{ account = 'new-other-provider'; password_set = $true } }
        }
    }
    # JSON objects, including an empty write counter, remain objects on every run.
    $state.writes = @{}
    $stateName = 'state-{0}.json' -f [guid]::NewGuid().ToString('N')
    return @{ Directory = $directory; ConfigPath = (Join-Path $directory '校园 配置.json'); StatePath = (Join-Path $directory $stateName); Config = $configuration; State = $state; From = 'old'; To = 'new' }
}

function Invoke-Workflow([string] $Command, [hashtable] $Case, [string[]] $Selection = @('-Interface', '校园 有线 网络'), [string[]] $Extra = @(), [bool] $UseDefaultConfig = $false) {
    [IO.File]::WriteAllText($Case.ConfigPath, ($Case.Config | ConvertTo-Json -Depth 20), [Text.UTF8Encoding]::new($false))
    [IO.File]::WriteAllText($Case.StatePath, ($Case.State | ConvertTo-Json -Depth 20), [Text.UTF8Encoding]::new($false))
    $arguments = @('-NoLogo', '-NoProfile', '-NonInteractive', '-File', (Join-Path $PSScriptRoot "../$Command.ps1"))
    $arguments += $Selection
    if (-not $UseDefaultConfig) { $arguments += @('-Config', ($Case.ConfigArgument ?? $Case.ConfigPath)) }
    $arguments += @('-Executable', $executable, '-TimeoutSeconds', '1')
    switch ($Command) {
        'connect' { $arguments += @('-Account', 'new', '-Operator', ($Case.Operator ?? $Case.State.provider)) }
        'disconnect' { $arguments += @('-Account', 'old') }
        'switch' { $arguments += @('-From', $Case.From, '-To', $Case.To) }
    }
    $arguments += $Extra
    $environment = @{ NJUPT_NET_FAKE_STATE = $Case.StatePath }
    $workingDirectory = $Case.Directory
    if ($Case.SetLocationOverride) {
        $wrapperPath = Join-Path $Case.Directory 'set location.ps1'
        $argumentPath = Join-Path $Case.Directory 'workflow arguments.json'
        $workflowParameters = @{}
        for ($index = 5; $index -lt $arguments.Count; $index += 2) {
            $workflowParameters[$arguments[$index].TrimStart('-')] = $arguments[$index + 1]
        }
        [IO.File]::WriteAllText($argumentPath, (ConvertTo-Json -InputObject $workflowParameters -Depth 5), [Text.UTF8Encoding]::new($false))
        [IO.File]::WriteAllText($wrapperPath, @'
#requires -Version 7.0
$ErrorActionPreference = 'Stop'
Set-Location -LiteralPath $env:NJUPT_NET_TEST_LOCATION
$workflowParameters = Get-Content -LiteralPath $env:NJUPT_NET_TEST_ARGUMENTS -Raw | ConvertFrom-Json -AsHashtable
& $env:NJUPT_NET_TEST_SCRIPT @workflowParameters
exit $LASTEXITCODE
'@, [Text.UTF8Encoding]::new($false))
        $environment['NJUPT_NET_TEST_LOCATION'] = $Case.Directory
        $environment['NJUPT_NET_TEST_ARGUMENTS'] = $argumentPath
        $environment['NJUPT_NET_TEST_SCRIPT'] = Join-Path $PSScriptRoot "../$Command.ps1"
        $arguments = @('-NoLogo', '-NoProfile', '-NonInteractive', '-File', $wrapperPath)
        $workingDirectory = $outputRoot
    }
    $process = Invoke-Process $pwsh $arguments $environment $workingDirectory
    $stateText = [IO.File]::ReadAllText($Case.StatePath)
    $observed = $stateText | ConvertFrom-Json -AsHashtable
    foreach ($secret in $secrets) {
        Assert-True (-not ($process.Stdout + $process.Stderr + $stateText).Contains($secret)) 'A password reached workflow output or CLI arguments.'
    }
    $text = $(if ($process.ExitCode -eq 0) { $process.Stdout } else { $process.Stderr })
    Assert-True (-not [string]::IsNullOrWhiteSpace($text)) "$Command did not emit a result."
    try {
        $document = [Text.Json.JsonDocument]::Parse($text)
        $document.Dispose()
        $envelope = $text | ConvertFrom-Json -AsHashtable
    }
    catch { throw "$Command did not emit exactly one JSON document: $text" }
    Assert-True ($envelope.command -eq $Command) "$Command emitted an incorrect command envelope."
    Assert-True ($envelope.data.ContainsKey('stage') -and $envelope.data.ContainsKey('steps')) "$Command omitted stage or steps."
    if ($process.ExitCode -eq 0) {
        Assert-True ([string]::IsNullOrWhiteSpace($process.Stderr) -and -not $envelope.ContainsKey('error')) "$Command mixed its success and error streams."
    }
    else {
        Assert-True ($process.ExitCode -eq 1 -and [string]::IsNullOrWhiteSpace($process.Stdout) -and $envelope.error.message) "$Command omitted its JSON error or mixed output streams."
    }
    foreach ($call in $observed.calls) {
        if ($call.command -eq 'interfaces') { continue }
        $sourceIndex = [Array]::IndexOf([string[]] $call.args, '--source')
        Assert-True ($sourceIndex -ge 0 -and $call.args[$sourceIndex + 1] -eq '10.20.30.40') "$Command did not pin all network calls to the selected source."
        Assert-True (-not ($call.args -contains '--interface')) "$Command selected an interface again after source resolution."
    }
    foreach ($count in $observed.writes.Values) { Assert-True ($count -le 1) "$Command retried a mutation." }
    return @{ Process = $process; Result = $envelope; State = $observed }
}

function Assert-NoWrites([hashtable] $Observed, [string] $Message) {
    Assert-True ($Observed.State.writes.Count -eq 0) $Message
}

function Assert-Success([hashtable] $Observed, [string] $Name) {
    Assert-True ($Observed.Process.ExitCode -eq 0) "$Name failed: $($Observed.Process.Stderr)"
    $script:completed.Add($Name)
}

$baselines = @{}
foreach ($command in @('connect', 'disconnect', 'switch')) {
    $case = New-Case
    if ($command -eq 'connect') { $case.State.online = $false }
    $observed = Invoke-Workflow $command $case
    Assert-Success $observed "$command success"
    $baselines[$command] = $observed
    if ($command -eq 'connect') {
        Assert-True ($observed.State.writes['new:login'] -eq 1 -and $observed.State.online_account -eq 'new-campus@cmcc') 'Connect did not submit one login for the selected account.'
    }
    elseif ($command -eq 'disconnect') {
        Assert-True ($observed.State.writes['old:offline'] -eq 1 -and -not $observed.State.online -and $observed.State.writes.Count -eq 1) 'Disconnect changed an operator or another setting.'
    }
    else {
        foreach ($write in @('old:offline', 'old:unbind', 'new:bind', 'new:login')) { Assert-True ($observed.State.writes[$write] -eq 1) "Switch omitted $write." }
        Assert-True ($observed.State.bindings.old.cmcc.account -eq '' -and $observed.State.bindings.new.cmcc.account -eq 'broadband-fixture') 'Switch did not migrate the configured provider.'
        Assert-True ($observed.State.bindings.old.njxy.account -eq 'old-other-provider' -and $observed.State.bindings.new.njxy.account -eq 'new-other-provider') 'Switch changed the unselected provider.'
    }
    if ($command -ne 'connect') {
        Assert-True ($observed.State.offline_ids.Count -eq 1 -and $observed.State.offline_ids[0] -eq 'self-session') 'Disconnect used a portal session instead of the matched Self session.'
    }
}

$case = New-Case
$case.State.online_account = 'new-campus@cmcc'
$observed = Invoke-Workflow 'connect' $case @('-Source', '10.20.30.40')
Assert-Success $observed 'connect already online with explicit source'
Assert-NoWrites $observed 'Connect submitted a password for an already online target.'
Assert-True (@($observed.State.calls | Where-Object command -eq 'probe').Count -eq 1) 'Connect did not verify connectivity for the online target.'

$case = New-Case
$case.State.online = $false
$case.Operator = 'campus'
$case.ConfigPath = Join-Path $case.Directory 'config.json'
$observed = Invoke-Workflow 'connect' $case -UseDefaultConfig $true
Assert-Success $observed 'connect campus with configuration in current directory'
Assert-True ($observed.State.online_account -eq 'new-campus') 'Campus login received an operator suffix.'

foreach ($configurationMode in @('default', 'relative')) {
    $case = New-Case
    $case.State.online = $false
    $case.SetLocationOverride = $true
    if ($configurationMode -eq 'default') {
        $case.ConfigPath = Join-Path $case.Directory 'config.json'
        $observed = Invoke-Workflow 'connect' $case -UseDefaultConfig $true
    }
    else {
        $case.ConfigArgument = './校园 配置.json'
        $observed = Invoke-Workflow 'connect' $case
    }
    Assert-Success $observed "connect resolves $configurationMode Config after Set-Location"
    foreach ($call in $observed.State.calls) {
        if ($call.command -eq 'interfaces') { continue }
        $configIndex = [Array]::IndexOf([string[]] $call.args, '--config')
        Assert-True ($configIndex -ge 0 -and $call.args[$configIndex + 1] -eq $case.ConfigPath) 'Config followed the process working directory instead of the PowerShell location.'
    }
}

$case = New-Case
$case.State.online = $false
$case.State.fail_at = 2
$case.State.failure_mode = 'secret_error'
$observed = Invoke-Workflow 'connect' $case
Assert-True ($observed.Process.ExitCode -ne 0) 'Connect ignored a CLI error containing credentials.'
Assert-NoWrites $observed 'Connect modified state after a credential-bearing error.'
$completed.Add('CLI error messages remove configured passwords')

$case = New-Case
$case.State.online_account = ',1,new-campus@cmcc'
$observed = Invoke-Workflow 'connect' $case
Assert-Success $observed 'connect recognizes prefixed portal identity'
Assert-NoWrites $observed 'Connect reauthenticated a matching prefixed identity.'

$case = New-Case
$case.State.online = $false
$observed = Invoke-Workflow 'disconnect' $case
Assert-Success $observed 'disconnect verifies an already offline terminal'
Assert-NoWrites $observed 'Disconnect modified an already offline terminal.'

$case = New-Case 'offline_self_online'
$case.State.online = $false
$observed = Invoke-Workflow 'disconnect' $case
Assert-True ($observed.Process.ExitCode -ne 0) 'Disconnect accepted a remaining Self session while the portal was offline.'
Assert-NoWrites $observed 'Disconnect mutated an unconfirmed offline state.'
$completed.Add('disconnect checks Self when portal is already offline')

foreach ($scenario in @('multiple_ipv4', 'interface_down')) {
    $case = New-Case $scenario
    $case.State.online = $false
    $observed = Invoke-Workflow 'connect' $case
    Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.State.calls.Count -eq 1) "Connect accepted $scenario."
    Assert-NoWrites $observed 'Connect modified state after invalid interface selection.'
    $completed.Add("interface selection rejects $scenario")
}

foreach ($command in @('connect', 'disconnect', 'switch')) {
    $case = New-Case
    $case.State.online_account = 'unrelated-campus@cmcc'
    $observed = Invoke-Workflow $command $case
    Assert-True ($observed.Process.ExitCode -ne 0) "$command accepted another online identity."
    Assert-NoWrites $observed "$command modified another account's terminal."
    $completed.Add("$command rejects another identity")
}

foreach ($command in @('disconnect', 'switch')) {
    foreach ($scenario in @('zero_session', 'multiple_sessions', 'wrong_mac')) {
        $case = New-Case $scenario
        $observed = Invoke-Workflow $command $case
        Assert-True ($observed.Process.ExitCode -ne 0) "$command accepted $scenario."
        Assert-NoWrites $observed "$command modified state without a unique IP and MAC match."
        $completed.Add("$command rejects $scenario")
    }
}

$case = New-Case '' 'njxy'
$observed = Invoke-Workflow 'switch' $case
Assert-Success $observed 'switch migrates configured njxy provider'
Assert-True ($observed.State.bindings.old.njxy.account -eq '' -and $observed.State.bindings.new.njxy.account -eq 'broadband-fixture') 'Switch ignored the configured njxy provider.'
Assert-True ($observed.State.bindings.old.cmcc.account -eq 'old-other-provider' -and $observed.State.bindings.new.cmcc.account -eq 'new-other-provider') 'NJXY migration changed CMCC bindings.'

$case = New-Case 'target_changes_after_offline'
$observed = Invoke-Workflow 'switch' $case
Assert-True ($observed.Process.ExitCode -ne 0) 'Switch ignored a target binding changed after disconnection.'
Assert-True ($observed.State.writes.Count -eq 1 -and $observed.State.writes['old:offline'] -eq 1) 'Switch cleared the old binding before detecting a changed target.'
Assert-True ($observed.State.bindings.old.cmcc.account -eq 'broadband-fixture' -and $observed.State.bindings.new.cmcc.account -eq 'concurrent-target-binding') 'Switch changed bindings after the target became occupied.'
$completed.Add('switch rechecks the target before clearing the old binding')

foreach ($variant in @('same_alias', 'same_real_account', 'missing_old_password', 'missing_new_password', 'invalid_provider', 'missing_broadband_password', 'old_binding_mismatch', 'old_binding_without_password', 'target_binding_occupied', 'target_password_without_account')) {
    $case = New-Case
    switch ($variant) {
        'same_alias' { $case.To = 'old' }
        'same_real_account' { $case.Config.accounts.new.account = $case.Config.accounts.old.account }
        'missing_old_password' { $case.Config.accounts.old.password = '' }
        'missing_new_password' { $case.Config.accounts.new.password = '' }
        'invalid_provider' { $case.Config.broadband_account.operator = 'campus' }
        'missing_broadband_password' { $case.Config.broadband_account.password = '' }
        'old_binding_mismatch' { $case.State.bindings.old.cmcc.account = 'another-broadband' }
        'old_binding_without_password' { $case.State.bindings.old.cmcc.password_set = $false }
        'target_binding_occupied' { $case.State.bindings.new.cmcc = @{ account = 'another-broadband'; password_set = $true } }
        'target_password_without_account' { $case.State.bindings.new.cmcc.password_set = $true }
    }
    $observed = Invoke-Workflow 'switch' $case
    Assert-True ($observed.Process.ExitCode -ne 0) "Switch accepted $variant."
    Assert-NoWrites $observed "Switch modified state after $variant."
    $completed.Add("switch rejects $variant")
}

foreach ($command in @('connect', 'disconnect', 'switch')) {
    $baseline = $baselines[$command].State.calls
    for ($index = 1; $index -le $baseline.Count; $index++) {
        $case = New-Case
        if ($command -eq 'connect') { $case.State.online = $false }
        $case.State.fail_at = $index
        $case.State.failure_mode = 'stderr_error'
        $observed = Invoke-Workflow $command $case
        Assert-True ($observed.Process.ExitCode -ne 0) "$command ignored CLI failure at call $index."
        Assert-True ($observed.State.calls.Count -eq $index) "$command advanced or retried after failure at call $index."
        for ($prefix = 0; $prefix -lt $index; $prefix++) {
            $actual = $observed.State.calls[$prefix]
            $expected = $baseline[$prefix]
            Assert-True ($actual.command -eq $expected.command -and $actual.account -eq $expected.account -and $actual.write -eq $expected.write) "$command changed its call sequence before injected failure."
        }
    }
    $completed.Add("$command stops at every CLI failure")
    foreach ($mode in @('accepted_unverified', 'accepted_unverified_success')) {
        for ($index = 1; $index -le $baseline.Count; $index++) {
            if (-not $baseline[$index - 1].write) { continue }
            $case = New-Case
            if ($command -eq 'connect') { $case.State.online = $false }
            $case.State.fail_at = $index
            $case.State.failure_mode = $mode
            $observed = Invoke-Workflow $command $case
            Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.State.calls.Count -eq $index) "$command advanced after an accepted but unverified operation."
            $lastStep = @($observed.Result.data.steps)[-1]
            Assert-True ($lastStep.outcome -eq 'accepted' -and $lastStep.verified -eq $false) "$command lost the accepted but unverified state."
        }
    }
    $completed.Add("$command preserves accepted unverified results")
    foreach ($mode in @('malformed_json', 'wrong_envelope')) {
        $case = New-Case
        if ($command -eq 'connect') { $case.State.online = $false }
        $case.State.fail_at = 2
        $case.State.failure_mode = $mode
        $observed = Invoke-Workflow $command $case
        Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.State.calls.Count -eq 2) "$command ignored $mode."
        Assert-NoWrites $observed "$command modified state after $mode."
        $completed.Add("$command rejects $mode")
    }
}

foreach ($command in @('disconnect', 'switch')) {
    $case = New-Case 'offline_delayed'
    $observed = Invoke-Workflow $command $case
    Assert-Success $observed "$command observes delayed portal disconnection"
    Assert-True ($observed.State.portal_reads -ge 3 -and $observed.State.writes['old:offline'] -eq 1) "$command did not observe delayed state without repeating offline."
    $case = New-Case 'offline_pending'
    $observed = Invoke-Workflow $command $case
    Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.State.portal_reads -ge 2) "$command did not stop after bounded portal observations."
    Assert-True ($observed.State.writes.Count -eq 1 -and $observed.State.writes['old:offline'] -eq 1) "$command advanced beyond an unconfirmed portal disconnection."
    $completed.Add("$command stops when portal disconnection does not converge")
}

foreach ($command in @('connect', 'switch')) {
    $case = New-Case 'probe_offline'
    if ($command -eq 'connect') { $case.State.online = $false }
    $observed = Invoke-Workflow $command $case
    Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.State.calls[-1].command -eq 'probe') "$command accepted a failed connectivity probe or continued afterwards."
    $completed.Add("$command rejects an offline connectivity result")
}

# Hold one source workflow at a read-only fixture response, then contend from
# another process. The same pause also exposes the configuration sharing mode.
$case = New-Case 'pause_status'
$case.State.online = $false
[IO.File]::WriteAllText($case.ConfigPath, ($case.Config | ConvertTo-Json -Depth 20), [Text.UTF8Encoding]::new($false))
[IO.File]::WriteAllText($case.StatePath, ($case.State | ConvertTo-Json -Depth 20), [Text.UTF8Encoding]::new($false))
$start = [Diagnostics.ProcessStartInfo]::new()
$start.FileName = $pwsh
$start.WorkingDirectory = $case.Directory
$start.UseShellExecute = $false
$start.CreateNoWindow = $true
$start.RedirectStandardOutput = $true
$start.RedirectStandardError = $true
$start.StandardOutputEncoding = [Text.UTF8Encoding]::new($false)
$start.StandardErrorEncoding = [Text.UTF8Encoding]::new($false)
$start.Environment['NJUPT_NET_FAKE_STATE'] = $case.StatePath
$start.Environment['TEMP'] = $temporaryDirectory
$start.Environment['TMP'] = $temporaryDirectory
foreach ($argument in @('-NoLogo', '-NoProfile', '-NonInteractive', '-File', (Join-Path $PSScriptRoot '../connect.ps1'), '-Interface', '校园 有线 网络', '-Account', 'new', '-Operator', 'cmcc', '-Config', $case.ConfigPath, '-Executable', $executable, '-TimeoutSeconds', '1')) {
    $start.ArgumentList.Add($argument)
}
$active = [Diagnostics.Process]::new()
$active.StartInfo = $start
try {
    $null = $active.Start()
    $activeStdout = $active.StandardOutput.ReadToEndAsync()
    $activeStderr = $active.StandardError.ReadToEndAsync()
    $deadline = [Diagnostics.Stopwatch]::StartNew()
    while (-not [IO.File]::Exists($case.StatePath + '.ready')) {
        if ($active.HasExited -or $deadline.Elapsed.TotalSeconds -ge 10) { throw 'First workflow did not reach its offline pause.' }
        Start-Sleep -Milliseconds 25
    }
    if ($IsWindows) {
        $writeBlocked = $false
        try {
            $writer = [IO.File]::Open($case.ConfigPath, [IO.FileMode]::Open, [IO.FileAccess]::Write, [IO.FileShare]::ReadWrite)
            $writer.Dispose()
        }
        catch [IO.IOException] { $writeBlocked = $true }
        Assert-True $writeBlocked 'Workflow did not prevent configuration writes while active.'
        $completed.Add('active workflow protects its configuration from writes')
    }
    $contender = New-Case
    $contender.State.online = $false
    $observed = Invoke-Workflow 'connect' $contender
    Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.Result.data.stage -eq 'lock') 'Concurrent workflow did not reject an occupied source.'
    Assert-NoWrites $observed 'Concurrent workflow modified the occupied source.'
    Assert-True (@($observed.State.calls | Where-Object command -ne 'interfaces').Count -eq 0) 'Concurrent workflow queried the campus network before obtaining the source lock.'
    $completed.Add('concurrent workflow rejects an occupied source')
}
finally {
    [IO.File]::WriteAllText($case.StatePath + '.release', 'release')
    if (-not $active.WaitForExit(10000)) { $active.Kill($true) }
    $activeOutput = $activeStdout.GetAwaiter().GetResult()
    $activeError = $activeStderr.GetAwaiter().GetResult()
    $activeCode = $active.ExitCode
    $active.Dispose()
}
Assert-True ($activeCode -eq 0 -and [string]::IsNullOrWhiteSpace($activeError)) "First source workflow failed after release: $activeError"
foreach ($secret in $secrets) { Assert-True (-not ($activeOutput + $activeError).Contains($secret)) 'Concurrent test output exposed a password.' }
$case = New-Case
$case.State.online = $false
$observed = Invoke-Workflow 'connect' $case
Assert-Success $observed 'completed workflow releases its source lock'

@{ result = 'passed'; cases = $caseNumber; checks = $completed.ToArray() } | ConvertTo-Json -Depth 8
