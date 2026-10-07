#requires -Version 7.0
[CmdletBinding()]
param([Parameter(Mandatory)][string] $OutputDirectory)

$ErrorActionPreference = 'Stop'
$repository = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '../../..'))
$outputRoot = [IO.Path]::GetFullPath($OutputDirectory)
if ($outputRoot -eq $repository -or $outputRoot.StartsWith($repository + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'OutputDirectory must be outside the repository.'
}
[IO.Directory]::CreateDirectory($outputRoot) | Out-Null
$temporaryDirectory = Join-Path $outputRoot 'tmp'
$fixtureDirectory = Join-Path $outputRoot '含空格 测试程序'
[IO.Directory]::CreateDirectory($temporaryDirectory) | Out-Null
[IO.Directory]::CreateDirectory($fixtureDirectory) | Out-Null
$executable = Join-Path $fixtureDirectory $(if ($IsWindows) { 'fake cli.exe' } else { 'fake cli' })
$pwsh = (Get-Command pwsh -CommandType Application -ErrorAction Stop | Select-Object -First 1).Source
$go = (Get-Command go -CommandType Application -ErrorAction Stop | Select-Object -First 1).Source
$secrets = @('CAMPUS_OLD_PASSWORD_73e6', 'CAMPUS_NEW_PASSWORD_82a1', 'BROADBAND_PASSWORD_91f4', 'CAMPUS_THIRD_PASSWORD_a62c')
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

function New-Case([string] $Plan = 'migrate_online', [string] $Provider = 'cmcc', [string] $Scenario = '') {
    $script:caseNumber++
    $directory = Join-Path $outputRoot ('cases/用例 {0:D3}' -f $script:caseNumber)
    [IO.Directory]::CreateDirectory($directory) | Out-Null
    $other = $(if ($Provider -eq 'cmcc') { 'njxy' } else { 'cmcc' })
    $configuration = @{
        accounts = @{
            old = @{ account = 'old-campus'; password = $secrets[0] }
            new = @{ account = 'new-campus'; password = $secrets[1] }
        }
        broadband_account = @{ operator = $Provider; account = 'broadband-fixture'; password = $secrets[2] }
    }
    $state = @{
        scenario = $Scenario; fail_at = 0; failure_mode = ''; calls = @(); writes = @{}; reads = @{}; offline_ids = @(); residual = @{}
        online = $true; online_account = "old-campus@$Provider"; provider = $Provider
        bindings = @{
            old = @{ $Provider = @{ account = 'broadband-fixture'; password_set = $true }; $other = @{ account = 'old-other-provider'; password_set = $true } }
            new = @{ $Provider = @{ account = ''; password_set = $false }; $other = @{ account = 'new-other-provider'; password_set = $true } }
        }
    }
    $previous = 'old'
    $holder = 'old'
    switch ($Plan) {
        'ready_online' { $state.online_account = "new-campus@$Provider"; $previous = 'new' }
        'ready_offline' { $state.online = $false; $previous = $null }
        'ready_other_online' { }
        'target_wrong_operator' { $state.online_account = "new-campus@$other"; $previous = 'new' }
        'new_empty_online' { $state.online_account = "new-campus@$Provider"; $previous = 'new' }
        'unowned_offline' { $state.online = $false; $previous = $null }
        'unowned_online' { }
        'migrate_offline' { $state.online = $false; $previous = $null }
        'migrate_third_online' {
            $configuration.accounts.third = @{ account = 'third-campus'; password = $secrets[3] }
            $state.bindings.third = @{ $Provider = @{ account = ''; password_set = $false }; $other = @{ account = 'third-other-provider'; password_set = $true } }
            $state.online_account = "third-campus@$Provider"; $previous = 'third'
        }
        'migrate_online' { }
        default { throw "Unknown fixture plan: $Plan" }
    }
    if ($Plan -like 'ready_*' -or $Plan -eq 'target_wrong_operator') {
        $state.bindings.new[$Provider] = @{ account = 'broadband-fixture'; password_set = $true }
        $state.bindings.old[$Provider] = @{ account = ''; password_set = $false }
        $holder = $null
    }
    if ($Plan -like 'unowned_*') {
        $state.bindings.old[$Provider] = @{ account = ''; password_set = $false }
        $holder = $null
    }
    $stateName = 'state-{0}.json' -f [guid]::NewGuid().ToString('N')
    return @{ Directory = $directory; ConfigPath = (Join-Path $directory '校园 配置.json'); StatePath = (Join-Path $directory $stateName); Config = $configuration; State = $state; Plan = $Plan; Previous = $previous; Holder = $holder }
}

function Invoke-Login([hashtable] $Case, [string[]] $Selection = @('-Interface', '校园 有线 网络'), [bool] $UseDefaultConfig = $false) {
    $configurationText = $Case.RawConfig ?? ($Case.Config | ConvertTo-Json -Depth 20)
    [IO.File]::WriteAllText($Case.ConfigPath, $configurationText, [Text.UTF8Encoding]::new($false))
    [IO.File]::WriteAllText($Case.StatePath, ($Case.State | ConvertTo-Json -Depth 20), [Text.UTF8Encoding]::new($false))
    $arguments = @('-NoLogo', '-NoProfile', '-NonInteractive', '-File', (Join-Path $PSScriptRoot '../login.ps1'))
    $arguments += $Selection
    if (-not $UseDefaultConfig) { $arguments += @('-Config', ($Case.ConfigArgument ?? $Case.ConfigPath)) }
    $arguments += @('-Executable', ($Case.Executable ?? $executable), '-TimeoutSeconds', '1', '-Account', ($Case.Account ?? 'new'))
    $environment = @{ NJUPT_NET_FAKE_STATE = $Case.StatePath }
    if ($Case.Environment) { foreach ($name in $Case.Environment.Keys) { $environment[$name] = $Case.Environment[$name] } }
    $workingDirectory = $Case.Directory
    if ($Case.SetLocationOverride) {
        $wrapperPath = Join-Path $Case.Directory 'set location.ps1'
        $argumentPath = Join-Path $Case.Directory 'workflow arguments.json'
        $workflowParameters = @{}
        for ($index = 5; $index -lt $arguments.Count; $index += 2) { $workflowParameters[$arguments[$index].TrimStart('-')] = $arguments[$index + 1] }
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
        $environment['NJUPT_NET_TEST_SCRIPT'] = Join-Path $PSScriptRoot '../login.ps1'
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
    Assert-True (-not [string]::IsNullOrWhiteSpace($text)) 'Login did not emit a result.'
    try {
        $document = [Text.Json.JsonDocument]::Parse($text)
        $document.Dispose()
        $envelope = $text | ConvertFrom-Json -AsHashtable
    }
    catch { throw "Login did not emit exactly one JSON document: $text" }
    Assert-True ($envelope.command -eq 'login') 'Login emitted an incorrect command envelope.'
    Assert-True ($envelope.data.ContainsKey('stage') -and $envelope.data.ContainsKey('steps')) 'Login omitted stage or steps.'
    if ($process.ExitCode -eq 0) {
        Assert-True ([string]::IsNullOrWhiteSpace($process.Stderr) -and -not $envelope.ContainsKey('error')) 'Login mixed its success and error streams.'
    }
    else {
        Assert-True ($process.ExitCode -eq 1 -and [string]::IsNullOrWhiteSpace($process.Stdout) -and $envelope.error.message) 'Login omitted its JSON error or mixed output streams.'
    }
    foreach ($call in $observed.calls) {
        if ($call.command -eq 'interfaces') { continue }
        $sourceIndex = [Array]::IndexOf([string[]] $call.args, '--source')
        Assert-True ($sourceIndex -ge 0 -and $call.args[$sourceIndex + 1] -eq '10.20.30.40') 'Login did not pin all network calls to the selected source.'
        Assert-True (-not ($call.args -contains '--interface')) 'Login selected an interface again after source resolution.'
    }
    foreach ($group in @($observed.calls | Where-Object write | Group-Object write)) { Assert-True ($group.Count -eq 1) 'Login repeated a mutation submission.' }
    return @{ Process = $process; Result = $envelope; State = $observed }
}

function Assert-Writes([hashtable] $Observed, [string[]] $Expected = @()) {
    $actual = @($Observed.State.calls | Where-Object write | ForEach-Object write)
    Assert-True ([string]::Join(',', $actual) -ceq [string]::Join(',', $Expected)) "Mutation sequence differs: expected $([string]::Join(',', $Expected)), got $([string]::Join(',', $actual))."
}
function Assert-Success([hashtable] $Observed, [hashtable] $Case, [string] $Name) {
    Assert-True ($Observed.Process.ExitCode -eq 0) "$Name failed: $($Observed.Process.Stderr)"
    $data = $Observed.Result.data
    Assert-True ($data.stage -eq 'complete' -and $data.account_alias -eq 'new' -and $data.operator -eq $Case.State.provider -and $data.online -eq $true -and $data.internet -eq $true) "$Name returned incomplete success metadata."
    Assert-True ($data.previous_account_alias -eq $Case.Previous -and $data.binding_from -eq $Case.Holder -and $data.binding_moved -eq [bool]$Case.Holder) "$Name returned an incorrect previous account or binding holder."
    $other = $(if ($Case.State.provider -eq 'cmcc') { 'njxy' } else { 'cmcc' })
    foreach ($alias in $Case.State.bindings.Keys) {
        Assert-True ($Observed.State.bindings[$alias][$other].account -eq $Case.State.bindings[$alias][$other].account -and $Observed.State.bindings[$alias][$other].password_set -eq $Case.State.bindings[$alias][$other].password_set) 'Login changed an unselected operator binding.'
    }
    $script:completed.Add($Name)
}

$baselines = @{}
foreach ($plan in @('ready_online', 'ready_offline', 'ready_other_online', 'target_wrong_operator', 'unowned_offline', 'unowned_online', 'migrate_online', 'migrate_offline', 'migrate_third_online', 'new_empty_online')) {
    $case = New-Case $plan
    $observed = Invoke-Login $case
    Assert-Success $observed $case "$plan success"
    $baselines[$plan] = $observed
    $expected = @()
    if ($plan -ne 'ready_online') {
        if ($case.State.online) { $expected += "$($case.Previous):offline" }
        if ($case.Holder) { $expected += "$($case.Holder):unbind" }
        if (-not ($plan -like 'ready_*' -or $plan -eq 'target_wrong_operator')) { $expected += 'new:bind' }
        $expected += 'new:login'
    }
    Assert-Writes $observed $expected
    if ($case.State.online -and $plan -ne 'ready_online') {
        Assert-True ($observed.State.offline_ids.Count -eq 1 -and $observed.State.offline_ids[0] -eq "self-session-$($case.Previous)") 'Login disconnected the broadband holder or portal ID instead of the actual online Self session.'
    }
    if ($plan -like 'ready_*' -or $plan -eq 'target_wrong_operator') {
        Assert-True (@($observed.State.calls | Where-Object { $_.command -eq 'zfw operator' -and $_.account -ne 'new' }).Count -eq 0) 'Ready target caused unrelated account binding queries.'
    }
}

$case = New-Case 'migrate_online' 'njxy'
$observed = Invoke-Login $case @('-Source', '10.20.30.40')
Assert-Success $observed $case 'configured njxy migration with explicit source'
Assert-Writes $observed @('old:offline', 'old:unbind', 'new:bind', 'new:login')

$case = New-Case 'ready_online'
$case.State.online_account = ',1,new-campus@cmcc'
$case.Config.accounts.old.password = ''
$case.State.bindings.old.cmcc = @{ account = 'broadband-fixture'; password_set = $true }
$observed = Invoke-Login $case
Assert-Success $observed $case 'ready target recognizes prefix and ignores unrelated account credentials and bindings'
Assert-Writes $observed
Assert-True ($observed.State.calls.Count -eq 4) 'Already online target executed operations beyond interfaces, status, target binding and probe.'

foreach ($mode in @('default', 'relative')) {
    $case = New-Case 'ready_offline'
    $case.SetLocationOverride = $true
    if ($mode -eq 'default') { $case.ConfigPath = Join-Path $case.Directory 'config.json'; $observed = Invoke-Login $case -UseDefaultConfig $true }
    else { $case.ConfigArgument = './校园 配置.json'; $observed = Invoke-Login $case }
    Assert-Success $observed $case "Config $mode follows Set-Location"
    foreach ($call in $observed.State.calls) {
        if ($call.command -eq 'interfaces') { continue }
        $configIndex = [Array]::IndexOf([string[]] $call.args, '--config')
        Assert-True ($configIndex -ge 0 -and $call.args[$configIndex + 1] -eq $case.ConfigPath) 'Config followed the process working directory instead of the PowerShell location.'
    }
}

$case = New-Case 'ready_offline'
$firstPath = Join-Path $case.Directory 'first PATH directory'
$secondPath = Join-Path $case.Directory 'second PATH directory'
[IO.Directory]::CreateDirectory($firstPath) | Out-Null
[IO.Directory]::CreateDirectory($secondPath) | Out-Null
$commandName = $(if ($IsWindows) { 'njupt-net-path-fixture.exe' } else { 'njupt-net-path-fixture' })
$firstExecutable = Join-Path $firstPath $commandName
Copy-Item -LiteralPath $executable -Destination $firstExecutable
Copy-Item -LiteralPath $executable -Destination (Join-Path $secondPath $commandName)
$case.Executable = $commandName
$case.Environment = @{ PATH = $firstPath + [IO.Path]::PathSeparator + $secondPath + [IO.Path]::PathSeparator + $env:PATH }
$observed = Invoke-Login $case
Assert-Success $observed $case 'PATH chooses the first matching native executable'
foreach ($call in $observed.State.calls) { Assert-True ($call.executable -eq $firstExecutable) 'Login chose a later PATH executable.' }

foreach ($variant in @('malformed_config', 'missing_target_password', 'missing_holder_password', 'missing_current_password', 'missing_broadband', 'invalid_provider', 'empty_broadband_account', 'empty_broadband_password', 'target_other_binding', 'target_incomplete_binding', 'target_password_without_account', 'holder_incomplete_binding', 'multiple_holders', 'unknown_portal_account', 'ambiguous_portal_account', 'invalid_portal_prefix')) {
    $case = if ($variant -eq 'missing_current_password') { New-Case 'ready_other_online' } else { New-Case }
    switch ($variant) {
        'malformed_config' { $case.RawConfig = '{ malformed' }
        'missing_target_password' { $case.Config.accounts.new.password = '' }
        'missing_holder_password' { $case.Config.accounts.old.password = '' }
        'missing_current_password' { $case.Config.accounts.old.password = '' }
        'missing_broadband' { $case.Config.Remove('broadband_account') }
        'invalid_provider' { $case.Config.broadband_account.operator = 'campus' }
        'empty_broadband_account' { $case.Config.broadband_account.account = '' }
        'empty_broadband_password' { $case.Config.broadband_account.password = '' }
        'target_other_binding' { $case.State.bindings.new.cmcc = @{ account = 'other-broadband'; password_set = $true } }
        'target_incomplete_binding' { $case.State.bindings.new.cmcc = @{ account = 'broadband-fixture'; password_set = $false } }
        'target_password_without_account' { $case.State.bindings.new.cmcc.password_set = $true }
        'holder_incomplete_binding' { $case.State.bindings.old.cmcc.password_set = $false }
        'multiple_holders' {
            $case.Config.accounts.third = @{ account = 'third-campus'; password = $secrets[3] }
            $case.State.bindings.third = @{ cmcc = @{ account = 'broadband-fixture'; password_set = $true }; njxy = @{ account = ''; password_set = $false } }
        }
        'unknown_portal_account' { $case.State.online_account = 'unrelated-campus@cmcc' }
        'ambiguous_portal_account' { $case.Config.accounts.duplicate = @{ account = 'old-campus'; password = $secrets[3] } }
        'invalid_portal_prefix' { $case.State.online_account = ',2,old-campus@cmcc' }
    }
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -ne 0) "Login accepted $variant."
    Assert-Writes $observed
    $completed.Add("preflight rejects $variant")
}

foreach ($scenario in @('multiple_ipv4', 'interface_down', 'duplicate_source', 'zero_session', 'multiple_sessions', 'wrong_mac', 'wrong_status_ip', 'before_offline_identity_changes', 'before_offline_mac_changes', 'target_changes_before_offline_empty', 'holder_changes_before_offline')) {
    $case = New-Case 'migrate_online' 'cmcc' $scenario
    $selection = $(if ($scenario -eq 'duplicate_source') { @('-Source', '10.20.30.40') } else { @('-Interface', '校园 有线 网络') })
    $observed = Invoke-Login $case $selection
    Assert-True ($observed.Process.ExitCode -ne 0) "Login accepted $scenario."
    Assert-Writes $observed
    $completed.Add("no modifications after $scenario")
}

$case = New-Case 'ready_other_online' 'cmcc' 'target_changes_before_offline_ready'
$observed = Invoke-Login $case
Assert-True ($observed.Process.ExitCode -ne 0) 'Login disconnected despite a changed ready target.'
Assert-Writes $observed
$completed.Add('ready target binding is rechecked before disconnection')

foreach ($scenario in @('target_changes_after_offline', 'holder_changes_after_offline')) {
    $case = New-Case 'migrate_online' 'cmcc' $scenario
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -ne 0) "Login ignored $scenario."
    Assert-Writes $observed @('old:offline')
    $completed.Add("no unbind after $scenario")
}
$case = New-Case 'unowned_offline' 'cmcc' 'target_changes_before_bind'
$observed = Invoke-Login $case
Assert-True ($observed.Process.ExitCode -ne 0) 'Login overwrote a target changed before binding.'
Assert-Writes $observed
$completed.Add('unowned binding is rechecked before submission')

foreach ($alias in @('new', 'old')) {
    $case = New-Case 'migrate_offline'
    $case.State.residual[$alias] = $true
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -ne 0) "Login ignored the initial offline $alias Self session."
    Assert-Writes $observed
    $completed.Add("initial offline portal verifies $alias Self sessions")
}

foreach ($plan in @('ready_online', 'ready_offline', 'unowned_offline', 'migrate_online', 'migrate_offline')) {
    $baseline = $baselines[$plan].State.calls
    for ($index = 1; $index -le $baseline.Count; $index++) {
        $case = New-Case $plan
        $case.State.fail_at = $index
        $case.State.failure_mode = 'stderr_error'
        $observed = Invoke-Login $case
        Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.State.calls.Count -eq $index) "$plan advanced or retried after CLI failure at call $index."
        for ($prefix = 0; $prefix -lt $index; $prefix++) {
            $actual, $expected = $observed.State.calls[$prefix], $baseline[$prefix]
            Assert-True ($actual.command -eq $expected.command -and $actual.account -eq $expected.account -and $actual.write -eq $expected.write) "$plan changed its call sequence before injected failure."
        }
    }
    $completed.Add("$plan stops at every CLI failure")
}
foreach ($plan in @('ready_offline', 'unowned_offline', 'migrate_online')) {
    $baseline = $baselines[$plan].State.calls
    foreach ($mode in @('accepted_unverified', 'accepted_unverified_success', 'unknown_mutation')) {
        for ($index = 1; $index -le $baseline.Count; $index++) {
            if (-not $baseline[$index - 1].write) { continue }
            $case = New-Case $plan
            $case.State.fail_at = $index
            $case.State.failure_mode = $mode
            $observed = Invoke-Login $case
            Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.State.calls.Count -eq $index) "$plan advanced after $mode."
            $last = @($observed.Result.data.steps)[-1]
            $expectedOutcome = $(if ($mode -eq 'unknown_mutation') { 'unknown' } else { 'accepted' })
            Assert-True ($last.outcome -eq $expectedOutcome -and $last.verified -eq $false) "$plan lost the submitted operation state."
        }
    }
    $completed.Add("$plan preserves uncertain mutations without retry")
}
foreach ($mode in @('malformed_json', 'wrong_envelope', 'secret_error')) {
    $case = New-Case
    $case.State.fail_at = 2
    $case.State.failure_mode = $mode
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.State.calls.Count -eq 2) "Login ignored $mode."
    Assert-Writes $observed
    $completed.Add("CLI failure handling rejects $mode")
}

$case = New-Case 'migrate_online'
$migrationCalls = $baselines.migrate_online.State.calls
$unbindIndex = -1
for ($index = 0; $index -lt $migrationCalls.Count; $index++) {
    if ($migrationCalls[$index].write -eq 'old:unbind') { $unbindIndex = $index; break }
}
Assert-True ($unbindIndex -ge 0 -and $migrationCalls[$unbindIndex + 1].command -eq 'zfw operator' -and -not $migrationCalls[$unbindIndex + 1].write) 'Migration baseline does not recheck the target after unbinding.'
$case.State.fail_at = $unbindIndex + 2
$case.State.failure_mode = 'stderr_error'
$interrupted = Invoke-Login $case
Assert-True ($interrupted.Process.ExitCode -ne 0 -and -not $interrupted.State.online) 'Interrupted migration did not preserve its observed offline state.'
Assert-Writes $interrupted @('old:offline', 'old:unbind')
Assert-True ($interrupted.State.bindings.old.cmcc.account -eq '' -and $interrupted.State.bindings.new.cmcc.account -eq '') 'Interrupted migration unexpectedly retained or assigned the broadband binding.'
$resume = New-Case 'unowned_offline'
$resume.State = $interrupted.State
$resume.State.calls = @(); $resume.State.writes = @{}; $resume.State.reads = @{}; $resume.State.offline_ids = @()
$resume.State.fail_at = 0; $resume.State.failure_mode = ''; $resume.State.portal_reads = 0
$observed = Invoke-Login $resume
Assert-Success $observed $resume 'manual invocation recomputes a previously interrupted transfer'
Assert-Writes $observed @('new:bind', 'new:login')

foreach ($scenario in @('login_wrong_identity', 'login_mac_changes', 'probe_offline', 'final_target_mismatch', 'final_holder_not_empty')) {
    $case = New-Case 'migrate_online' 'cmcc' $scenario
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -ne 0) "Login ignored $scenario."
    Assert-Writes $observed @('old:offline', 'old:unbind', 'new:bind', 'new:login')
    $completed.Add("final verification rejects $scenario")
}
$case = New-Case 'migrate_online' 'cmcc' 'offline_delayed'
$observed = Invoke-Login $case
Assert-Success $observed $case 'portal disconnection is observed with bounded readonly polling'
Assert-Writes $observed @('old:offline', 'old:unbind', 'new:bind', 'new:login')
Assert-True ($observed.State.portal_reads -ge 3) 'Delayed portal state was not observed.'
$case = New-Case 'migrate_online' 'cmcc' 'offline_pending'
$observed = Invoke-Login $case
Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.State.portal_reads -ge 2) 'Login did not stop after bounded offline observations.'
Assert-Writes $observed @('old:offline')
$completed.Add('unconfirmed portal disconnection stops before unbinding')

$case = New-Case 'ready_offline' 'cmcc' 'pause_status'
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
foreach ($argument in @('-NoLogo', '-NoProfile', '-NonInteractive', '-File', (Join-Path $PSScriptRoot '../login.ps1'), '-Interface', '校园 有线 网络', '-Account', 'new', '-Config', $case.ConfigPath, '-Executable', $executable, '-TimeoutSeconds', '1')) { $start.ArgumentList.Add($argument) }
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
        try { $writer = [IO.File]::Open($case.ConfigPath, [IO.FileMode]::Open, [IO.FileAccess]::Write, [IO.FileShare]::ReadWrite); $writer.Dispose() }
        catch [IO.IOException] { $writeBlocked = $true }
        Assert-True $writeBlocked 'Login did not prevent configuration writes while active.'
        $completed.Add('active login protects configuration from writes')
    }
    $contender = New-Case 'ready_offline'
    $observed = Invoke-Login $contender
    Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.Result.data.stage -eq 'lock') 'Concurrent login did not reject an occupied source.'
    Assert-Writes $observed
    Assert-True (@($observed.State.calls | Where-Object command -ne 'interfaces').Count -eq 0) 'Concurrent login queried the campus network before obtaining the source lock.'
    $completed.Add('concurrent login rejects an occupied source')
}
finally {
    [IO.File]::WriteAllText($case.StatePath + '.release', 'release')
    if (-not $active.WaitForExit(10000)) { $active.Kill($true) }
    $activeOutput = $activeStdout.GetAwaiter().GetResult()
    $activeError = $activeStderr.GetAwaiter().GetResult()
    $activeCode = $active.ExitCode
    $active.Dispose()
}
Assert-True ($activeCode -eq 0 -and [string]::IsNullOrWhiteSpace($activeError)) "First login failed after release: $activeError"
foreach ($secret in $secrets) { Assert-True (-not ($activeOutput + $activeError).Contains($secret)) 'Concurrent login output exposed a password.' }
$case = New-Case 'ready_offline'
$observed = Invoke-Login $case
Assert-Success $observed $case 'completed login releases its source lock'

@{ result = 'passed'; cases = $caseNumber; checks = $completed.ToArray() } | ConvertTo-Json -Depth 8
