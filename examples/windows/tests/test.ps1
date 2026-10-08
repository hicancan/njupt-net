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
    $arguments += @('-Executable', ($Case.Executable ?? $executable), '-TimeoutSeconds', '1', '-Account', ($Case.Account ?? 'new'), '-BindingWaitSeconds', [string]($Case.BindingWaitSeconds ?? 0))
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

function Assert-BindingReadbackBeforeLogin([hashtable] $Observed, [hashtable] $Case) {
    $steps = @($Observed.Result.data.steps)
    $bindIndex = [Array]::IndexOf([string[]] @($steps | ForEach-Object stage), 'bind')
    if ($bindIndex -lt 0) { return }
    $targetIndex = [Array]::IndexOf([string[]] @($steps | ForEach-Object stage), 'bound-target-binding')
    $loginIndex = [Array]::IndexOf([string[]] @($steps | ForEach-Object stage), 'login')
    Assert-True ($targetIndex -gt $bindIndex -and $loginIndex -gt $targetIndex) 'Login did not independently read the target binding after submission and before authentication.'
    $target = $Observed.State.calls[$targetIndex]
    Assert-True ($target.command -eq 'zfw operator' -and $target.account -eq 'new' -and -not $target.write) 'Target binding confirmation did not use a separate readonly CLI invocation.'
    if ($Case.Holder) {
        $holderIndex = [Array]::IndexOf([string[]] @($steps | ForEach-Object stage), 'unbound-holder-binding')
        Assert-True ($holderIndex -gt $targetIndex -and $loginIndex -gt $holderIndex) 'Login did not independently confirm the previous holder after binding and before authentication.'
        $holder = $Observed.State.calls[$holderIndex]
        Assert-True ($holder.command -eq 'zfw operator' -and $holder.account -eq $Case.Holder -and -not $holder.write) 'Previous holder confirmation did not use a separate readonly CLI invocation.'
    }
}

function Assert-Success([hashtable] $Observed, [hashtable] $Case, [string] $Name, $ExpectedInternet = $true) {
    Assert-True ($Observed.Process.ExitCode -eq 0) "$Name failed: $($Observed.Process.Stderr)"
    $data = $Observed.Result.data
    Assert-True ($data.stage -eq 'complete' -and $data.account_alias -eq 'new' -and $data.operator -eq $Case.State.provider -and $data.online -eq $true -and $data.internet -eq $ExpectedInternet) "$Name returned incomplete success metadata."
    Assert-True ($data.ContainsKey('internet_error')) "$Name omitted the Internet observation error field."
    if ($ExpectedInternet -eq $true) { Assert-True ($null -eq $data.internet_error) "$Name reported an Internet error alongside confirmed connectivity." }
    else { Assert-True ($data.internet_error -is [string] -and -not [string]::IsNullOrWhiteSpace($data.internet_error)) "$Name lost the Internet observation failure." }
    Assert-True ($data.previous_account_alias -eq $Case.Previous -and $data.binding_from -eq $Case.Holder -and $data.binding_moved -eq [bool]$Case.Holder) "$Name returned an incorrect previous account or binding holder."
    $bindingSubmitted = @($Observed.State.calls | Where-Object write -eq 'new:bind').Count -gt 0
    $expectedBindingWait = $(if ($bindingSubmitted) { $Case.BindingWaitSeconds ?? 0 } else { 0 })
    Assert-True ($data.ContainsKey('binding_wait_seconds') -and $data.binding_wait_seconds -eq $expectedBindingWait) "$Name returned an incorrect binding activation wait."
    $other = $(if ($Case.State.provider -eq 'cmcc') { 'njxy' } else { 'cmcc' })
    foreach ($alias in $Case.State.bindings.Keys) {
        Assert-True ($Observed.State.bindings[$alias][$other].account -eq $Case.State.bindings[$alias][$other].account -and $Observed.State.bindings[$alias][$other].password_set -eq $Case.State.bindings[$alias][$other].password_set) 'Login changed an unselected operator binding.'
    }
    Assert-BindingReadbackBeforeLogin $Observed $Case
    $script:completed.Add($Name)
}

$workflowTokens = $null
$workflowErrors = $null
$workflowAst = [Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot '../login.ps1'), [ref]$workflowTokens, [ref]$workflowErrors)
Assert-True ($workflowErrors.Count -eq 0) 'Login script could not be parsed.'
$waitParameter = @($workflowAst.ParamBlock.Parameters | Where-Object { $_.Name.VariablePath.UserPath -eq 'BindingWaitSeconds' })
Assert-True ($waitParameter.Count -eq 1 -and $waitParameter[0].DefaultValue.SafeGetValue() -eq 30) 'Binding activation wait must default to 30 seconds.'
$completed.Add('binding activation wait defaults to 30 seconds')

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

$case = New-Case 'unowned_offline'
$case.BindingWaitSeconds = 1
$observed = Invoke-Login $case
Assert-Success $observed $case 'binding activation wait precedes independent readback and single authentication'
Assert-Writes $observed @('new:bind', 'new:login')
$steps = @($observed.Result.data.steps)
$stages = [string[]] @($steps | ForEach-Object stage)
$bindCall = $observed.State.calls[[Array]::IndexOf($stages, 'bind')]
$readbackCall = $observed.State.calls[[Array]::IndexOf($stages, 'bound-target-binding')]
Assert-True (($readbackCall.at - $bindCall.at) -ge 1000) 'Login read the new binding before its configured activation wait elapsed.'

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

foreach ($provider in @('cmcc', 'njxy')) {
    foreach ($scenario in @('bind_success_target_empty', 'bind_success_holder_retained')) {
        $case = New-Case 'migrate_online' $provider $scenario
        $observed = Invoke-Login $case
        Assert-True ($observed.Process.ExitCode -ne 0) "Login accepted $scenario for $provider."
        Assert-Writes $observed @('old:offline', 'old:unbind', 'new:bind')
        $stage = $(if ($scenario -eq 'bind_success_target_empty') { 'bound-target-binding' } else { 'unbound-holder-binding' })
        Assert-True ($observed.Result.data.stage -eq $stage) 'Login did not stop at the independent binding observation that contradicted the bind result.'
        $bindStep = @($observed.Result.data.steps | Where-Object stage -eq 'bind')
        Assert-True ($bindStep.Count -eq 1 -and $bindStep[0].outcome -eq 'accepted' -and $bindStep[0].verified -eq $true) 'Independent binding failure lost the accepted bind result.'
        Assert-True (@($observed.State.calls | Where-Object command -eq 'p login').Count -eq 0) 'Login authenticated before independent binding observations agreed.'
        $completed.Add("$provider independent binding verification rejects $scenario")
    }

    $case = New-Case 'migrate_online' $provider 'portal_binding_unsynchronized'
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.Result.data.stage -eq 'login') 'Login ignored a portal rejection after successful independent binding observations.'
    Assert-Writes $observed @('old:offline', 'old:unbind', 'new:bind', 'new:login')
    Assert-BindingReadbackBeforeLogin $observed $case
    Assert-True ($observed.Result.error.message -ceq 'portal rejected login: 未绑定运营商账号,请正确绑定运营商账号再试！') "Login replaced the authentication server rejection: $($observed.Result.error.message)"
    $last = @($observed.Result.data.steps)[-1]
    Assert-True ($last.command -eq 'p login' -and $last.exit_code -eq 1 -and $last.outcome -eq 'rejected' -and $last.verified -eq $false) 'Login lost the rejected authentication result or advanced after it.'
    Assert-True (-not $observed.State.online -and $observed.State.writes['new:login'] -eq 1) 'Login retried a rejected authentication submission.'
    $completed.Add("$provider portal binding rejection is preserved without retry")
}

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
        if ($baseline[$index - 1].command -eq 'probe') {
            Assert-Success $observed $case "$plan retains login success after a probe failure" $null
        }
        else { Assert-True ($observed.Process.ExitCode -ne 0) "$plan accepted a business CLI failure at call $index." }
        Assert-True ($observed.State.calls.Count -eq $index) "$plan advanced or retried after CLI failure at call $index."
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

foreach ($scenario in @('login_wrong_identity', 'login_mac_changes', 'final_target_mismatch', 'final_holder_not_empty')) {
    $case = New-Case 'migrate_online' 'cmcc' $scenario
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -ne 0) "Login ignored $scenario."
    Assert-Writes $observed @('old:offline', 'old:unbind', 'new:bind', 'new:login')
    Assert-True (@($observed.State.calls | Where-Object command -eq 'probe').Count -eq 0) 'Login probed before its identity and final binding checks succeeded.'
    $completed.Add("final verification rejects $scenario")
}

foreach ($plan in @('ready_online', 'ready_offline', 'migrate_online')) {
    foreach ($scenario in @('probe_dns_error', 'probe_http_error', 'probe_secret_error', 'probe_offline', 'probe_error_data_offline')) {
        $case = New-Case $plan 'cmcc' $scenario
        $observed = Invoke-Login $case
        $expectedInternet = $(if ($scenario -in @('probe_offline', 'probe_error_data_offline')) { $false } else { $null })
        Assert-Success $observed $case "$plan completes with $scenario" $expectedInternet
        $expectedWrites = @($baselines[$plan].State.calls | Where-Object write | ForEach-Object write)
        Assert-Writes $observed $expectedWrites
        $calls = @($observed.State.calls)
        Assert-True (@($calls | Where-Object command -eq 'probe').Count -eq 1 -and $calls[-1].command -eq 'probe') 'Login repeated its probe or advanced to a business call after probing.'
        if ($plan -ne 'ready_online') {
            Assert-True ($calls[-2].command -eq 'zfw operator' -and -not $calls[-2].write) 'Login probed before checking final operator bindings.'
        }
        $probeStep = @($observed.Result.data.steps)[-1]
        $expectedCode = $(if ($scenario -eq 'probe_offline') { 0 } else { 1 })
        Assert-True ($probeStep.command -eq 'probe' -and $probeStep.exit_code -eq $expectedCode) 'Login lost the probe exit status.'
        if ($scenario -eq 'probe_secret_error') {
            Assert-True ($observed.Result.data.internet_error.Contains('[password]')) 'Login discarded the sanitized Internet failure detail.'
        }
    }
}

foreach ($scenario in @('probe_wrong_source_success', 'probe_wrong_source_error', 'probe_invalid_data', 'probe_missing_error', 'probe_empty_error', 'probe_usage_error', 'probe_mixed_streams', 'probe_conflicting_result')) {
    $case = New-Case 'ready_online' 'cmcc' $scenario
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.Result.data.online -eq $true) "Login accepted a corrupted probe result: $scenario."
    Assert-Writes $observed
    Assert-True (@($observed.State.calls | Where-Object command -eq 'probe').Count -eq 1) 'Login retried an invalid probe result.'
    $completed.Add("probe protocol validation rejects $scenario")
}
foreach ($mode in @('malformed_json', 'wrong_envelope')) {
    $case = New-Case 'ready_online'
    $case.State.fail_at = $baselines.ready_online.State.calls.Count
    $case.State.failure_mode = $mode
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -ne 0) "Login accepted a probe with $mode."
    Assert-Writes $observed
    $completed.Add("probe protocol validation rejects $mode")
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
