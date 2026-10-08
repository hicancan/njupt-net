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
        scenario = $Scenario; fail_at = 0; failure_mode = ''; calls = @(); writes = @{}; reads = @{}; offline_ids = @(); residual = @{}; session_starts = 0; session_closes = 0
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
    $arguments += @('-Executable', ($Case.Executable ?? $executable), '-TimeoutSeconds', '1', '-Account', ($Case.Account ?? 'new'), '-BindingTimeoutSeconds', [string]($Case.BindingTimeoutSeconds ?? 0))
    if ($Case.ContainsKey('PortalPort')) { $arguments += @('-PortalPort', [string]$Case.PortalPort) }
    if ($Case.Probe ?? $true) { $arguments += '-Probe' }
    $environment = @{ NJUPT_NET_FAKE_STATE = $Case.StatePath }
    if ($Case.Environment) { foreach ($name in $Case.Environment.Keys) { $environment[$name] = $Case.Environment[$name] } }
    $workingDirectory = $Case.Directory
    if ($Case.SetLocationOverride) {
        $wrapperPath = Join-Path $Case.Directory 'set location.ps1'
        $argumentPath = Join-Path $Case.Directory 'workflow arguments.json'
        $workflowParameters = @{}
        for ($index = 5; $index -lt $arguments.Count; $index++) {
            $parameter = $arguments[$index].TrimStart('-')
            if ($parameter -eq 'Probe') { $workflowParameters[$parameter] = $true }
            else { $index++; $workflowParameters[$parameter] = $arguments[$index] }
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
        $portIndex = [Array]::IndexOf([string[]] $call.args, '--port')
        if ($call.command -like 'p *') {
            Assert-True (@($call.args | Where-Object { $_ -ceq '--port' }).Count -eq 1 -and $portIndex -ge 0 -and $call.args[$portIndex + 1] -eq [string]($Case.PortalPort ?? $defaultPortalPort)) 'Login did not pass the selected portal port exactly once to each p command.'
        } else {
            Assert-True ($portIndex -lt 0) 'Login passed the portal port to a non-portal command.'
        }
        if ($call.command -eq 'interfaces') { continue }
        $sourceIndex = [Array]::IndexOf([string[]] $call.args, '--source')
        Assert-True ($sourceIndex -ge 0 -and $call.args[$sourceIndex + 1] -eq '10.20.30.40') 'Login did not pin all network calls to the selected source.'
        Assert-True (-not ($call.args -contains '--interface')) 'Login selected an interface again after source resolution.'
    }
    foreach ($group in @($observed.calls | Where-Object write | Group-Object write)) {
        $activationRetry = $group.Name -eq 'new:login' -and $observed.writes['new:bind'] -gt 0 -and
            $Case.State.scenario -in @('binding_activation_delayed', 'portal_binding_unsynchronized') -and ($Case.BindingTimeoutSeconds ?? 0) -gt 0
        Assert-True ($group.Count -eq 1 -or $activationRetry) 'Login repeated a mutation outside a newly assigned binding activation window.'
    }
    $startupFailure = $Case.State.scenario -like 'session_start_*'
    $expectedSessions = $(if ($observed.calls.Count -gt 0 -or $startupFailure) { 1 } else { 0 })
    $expectedCloses = $(if ($startupFailure) { 0 } else { $expectedSessions })
    Assert-True ($observed.session_starts -eq $expectedSessions -and $observed.session_closes -eq $expectedCloses) "Login did not use one CLI process from interface selection through closure: starts=$($observed.session_starts), closes=$($observed.session_closes), expected=$expectedSessions/$expectedCloses."
    if ($expectedSessions -eq 1) {
        $selectionFlag = $(if ($Selection[0] -eq '-Interface') { '--interface' } else { '--source' })
        $selectionIndex = [Array]::IndexOf([string[]] $observed.session_args, $selectionFlag)
        Assert-True ($selectionIndex -ge 0 -and $observed.session_args[$selectionIndex + 1] -ceq $Selection[1]) 'CLI session did not retain the initial interface or source selection.'
    }
    return @{ Process = $process; Result = $envelope; State = $observed }
}

function Assert-Writes([hashtable] $Observed, [string[]] $Expected = @()) {
    $actual = @($Observed.State.calls | Where-Object write | ForEach-Object write)
    Assert-True ([string]::Join(',', $actual) -ceq [string]::Join(',', $Expected)) "Mutation sequence differs: expected $([string]::Join(',', $Expected)), got $([string]::Join(',', $actual))."
}

function Assert-Success([hashtable] $Observed, [hashtable] $Case, [string] $Name, $ExpectedInternet = $true) {
    Assert-True ($Observed.Process.ExitCode -eq 0) "$Name failed: $($Observed.Process.Stderr)"
    $data = $Observed.Result.data
    Assert-True ($data.stage -eq 'complete' -and $data.account_alias -eq 'new' -and $data.operator -eq $Case.State.provider -and $data.online -eq $true -and $data.internet -eq $ExpectedInternet) "$Name returned incomplete success metadata."
    Assert-True ($data.ContainsKey('internet_error')) "$Name omitted the Internet observation error field."
    if (-not ($Case.Probe ?? $true)) { Assert-True ($null -eq $data.internet -and $null -eq $data.internet_error) "$Name reported an external connectivity observation without a probe." }
    elseif ($ExpectedInternet -eq $true) { Assert-True ($null -eq $data.internet_error) "$Name reported an Internet error alongside confirmed connectivity." }
    else { Assert-True ($data.internet_error -is [string] -and -not [string]::IsNullOrWhiteSpace($data.internet_error)) "$Name lost the Internet observation failure." }
    Assert-True ($data.previous_account_alias -eq $Case.Previous -and $data.binding_from -eq $Case.Holder -and $data.binding_moved -eq [bool]$Case.Holder) "$Name returned an incorrect previous account or binding holder."
    $loginCalls = @($Observed.State.calls | Where-Object command -eq 'p login')
    Assert-True ($data.ContainsKey('login_attempts') -and $data.login_attempts -eq $loginCalls.Count) "$Name returned an incorrect authentication attempt count."
    Assert-True ($data.ContainsKey('binding_activation_seconds') -and $data.binding_activation_seconds -ge 0) "$Name omitted binding activation duration."
    if (@($Observed.State.calls | Where-Object write -eq 'new:bind').Count -eq 0) {
        Assert-True ($data.binding_activation_seconds -eq 0) "$Name activated an existing binding."
    }
    Assert-True (@($Observed.Result.data.steps | Where-Object stage -in @('login-status', 'final-target-binding', 'final-old-binding', 'bound-target-binding', 'unbound-holder-binding')).Count -eq 0) 'Login repeated verification already provided by the core operation result.'
    $other = $(if ($Case.State.provider -eq 'cmcc') { 'njxy' } else { 'cmcc' })
    foreach ($alias in $Case.State.bindings.Keys) {
        Assert-True ($Observed.State.bindings[$alias][$other].account -eq $Case.State.bindings[$alias][$other].account -and $Observed.State.bindings[$alias][$other].password_set -eq $Case.State.bindings[$alias][$other].password_set) 'Login changed an unselected operator binding.'
    }
    $script:completed.Add($Name)
}

$workflowTokens = $null
$workflowErrors = $null
$workflowAst = [Management.Automation.Language.Parser]::ParseFile((Join-Path $PSScriptRoot '../login.ps1'), [ref]$workflowTokens, [ref]$workflowErrors)
Assert-True ($workflowErrors.Count -eq 0) 'Login script could not be parsed.'
$timeoutParameter = @($workflowAst.ParamBlock.Parameters | Where-Object { $_.Name.VariablePath.UserPath -eq 'BindingTimeoutSeconds' })
Assert-True ($timeoutParameter.Count -eq 1 -and $timeoutParameter[0].DefaultValue.SafeGetValue() -eq 45) 'Binding activation timeout must default to 45 seconds.'
Assert-True (@($workflowAst.ParamBlock.Parameters | Where-Object { $_.Name.VariablePath.UserPath -eq 'BindingWaitSeconds' }).Count -eq 0) 'Login retained its fixed binding wait parameter.'
$completed.Add('binding activation timeout defaults to 45 seconds without a fixed wait')
$portParameter = @($workflowAst.ParamBlock.Parameters | Where-Object { $_.Name.VariablePath.UserPath -eq 'PortalPort' })
Assert-True ($portParameter.Count -eq 1) 'Login omitted its portal port parameter.'
$defaultPortalPort = $portParameter[0].DefaultValue.SafeGetValue()
Assert-True ($defaultPortalPort -in @(801, 802, 803, 804)) 'Login defaults to an unsupported portal port.'
$completed.Add('portal default selects a supported endpoint')

foreach ($scenario in @('session_start_error', 'session_start_secret_error', 'session_start_malformed', 'session_start_eof')) {
    $case = New-Case 'ready_offline' 'cmcc' $scenario
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -eq 1 -and $observed.Result.data.stage -eq 'interfaces') "Login ignored $scenario."
    Assert-True ($observed.State.calls.Count -eq 0) 'Login sent a command after its CLI session failed to start.'
    Assert-Writes $observed
    if ($scenario -in @('session_start_error', 'session_start_secret_error')) {
        Assert-True ($observed.Result.error.message.StartsWith('cannot select the initial campus interface')) 'Login replaced the native session startup error.'
        Assert-True (-not $observed.Result.error.message.Contains('cleanup')) 'Login added a cleanup error for an already exited session.'
    }
    $completed.Add("session initialization handles $scenario")
}

$case = New-Case 'ready_offline' 'cmcc' 'source_changes_after_session_start'
$observed = Invoke-Login $case
Assert-True ($observed.Process.ExitCode -eq 1 -and $observed.Result.data.stage -eq 'status') 'Login accepted a changed source after opening its fixed CLI session.'
Assert-Writes $observed
$completed.Add('fixed session source is checked against the interface snapshot before modification')

$baselines = @{}
foreach ($plan in @('ready_online', 'ready_offline', 'ready_other_online', 'target_wrong_operator', 'unowned_offline', 'unowned_online', 'migrate_online', 'migrate_offline', 'migrate_third_online', 'new_empty_online')) {
    $case = New-Case $plan
    $observed = Invoke-Login $case
    Assert-Success $observed $case "$plan success"
    $baselines[$plan] = $observed
    $expected = @()
    if ($plan -ne 'ready_online') {
        if ($case.State.online -and $plan -ne 'new_empty_online') { $expected += "$($case.Previous):offline" }
        if ($case.Holder) { $expected += "$($case.Holder):unbind" }
        if (-not ($plan -like 'ready_*' -or $plan -eq 'target_wrong_operator')) { $expected += 'new:bind' }
        if ($plan -ne 'new_empty_online') { $expected += 'new:login' }
    }
    Assert-Writes $observed $expected
    if ($case.State.online -and $plan -notin @('ready_online', 'new_empty_online')) {
        Assert-True ($observed.State.offline_ids.Count -eq 1 -and $observed.State.offline_ids[0] -eq "self-session-$($case.Previous)") 'Login disconnected the broadband holder or portal ID instead of the actual online Self session.'
    }
    if ($plan -like 'ready_*' -or $plan -eq 'target_wrong_operator') {
        Assert-True (@($observed.State.calls | Where-Object { $_.command -eq 'zfw operator' -and $_.account -ne 'new' }).Count -eq 0) 'Ready target caused unrelated account binding queries.'
    }
}

foreach ($provider in @('cmcc', 'njxy')) {
    $case = New-Case 'new_empty_online' $provider
    $observed = Invoke-Login $case
    Assert-Success $observed $case "$provider target portal session survives a binding transfer"
    Assert-Writes $observed @('old:unbind', 'new:bind')
    Assert-True (@($observed.State.calls | Where-Object command -eq 'zfw online').Count -eq 0) 'Binding transfer queried sessions for an already selected target portal identity.'
    Assert-True ($observed.Result.data.login_attempts -eq 0 -and $observed.Result.data.binding_activation_seconds -eq 0) 'Binding transfer reauthenticated the existing target portal session.'
    Assert-True (@($observed.Result.data.steps | Where-Object stage -eq 'binding-status').Count -eq 1) 'Binding transfer did not confirm the preserved portal session.'
}

$case = New-Case 'new_empty_online' 'cmcc' 'binding_disconnects_terminal'
$observed = Invoke-Login $case
Assert-Success $observed $case 'target portal is authenticated when binding transfer actually disconnects it'
Assert-Writes $observed @('old:unbind', 'new:bind', 'new:login')

foreach ($scenario in @('binding_changes_portal_identity', 'binding_changes_portal_mac')) {
    $case = New-Case 'new_empty_online' 'cmcc' $scenario
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -eq 1) "Binding transfer ignored $scenario."
    Assert-Writes $observed @('old:unbind', 'new:bind')
    Assert-True (@($observed.State.calls | Where-Object command -in @('zfw online', 'p login', 'probe')).Count -eq 0) 'Binding transfer modified or probed an unexpected terminal identity.'
    $completed.Add("binding transfer stops after $scenario")
}

foreach ($plan in @('ready_online', 'ready_offline', 'migrate_online')) {
    $case = New-Case $plan
    $case.Probe = $false
    $observed = Invoke-Login $case
    Assert-Success $observed $case "$plan completes without an optional Internet probe" $null
    Assert-True (@($observed.State.calls | Where-Object command -eq 'probe').Count -eq 0 -and @($observed.Result.data.steps | Where-Object stage -eq 'internet').Count -eq 0) 'Login performed an external connectivity probe without -Probe.'
    Assert-Writes $observed @($baselines[$plan].State.calls | Where-Object write | ForEach-Object write)
}

$case = New-Case 'migrate_online' 'njxy'
$observed = Invoke-Login $case @('-Source', '10.20.30.40')
Assert-Success $observed $case 'configured njxy migration with explicit source'
Assert-Writes $observed @('old:offline', 'old:unbind', 'new:bind', 'new:login')

foreach ($port in @(801, 802, 803, 804)) {
    $case = New-Case 'migrate_online'
    $case.PortalPort = $port
    $observed = Invoke-Login $case
    Assert-Success $observed $case "explicit portal port $port migration"
    Assert-Writes $observed @('old:offline', 'old:unbind', 'new:bind', 'new:login')
    Assert-True (@($observed.State.calls | Where-Object command -eq 'p status').Count -ge 2 -and @($observed.State.calls | Where-Object command -eq 'p login').Count -eq 1) 'Explicit portal selection did not exercise status and authentication.'
}

$case = New-Case 'unowned_offline' 'cmcc' 'binding_activation_delayed'
$case.BindingTimeoutSeconds = 45
$case.State.login_delay_ms = 350
$observed = Invoke-Login $case
Assert-Success $observed $case 'binding activation authenticates as soon as the assignment is available'
$loginCalls = @($observed.State.calls | Where-Object command -eq 'p login')
Assert-True ($loginCalls.Count -ge 2) 'Login did not observe the pending binding activation.'
Assert-Writes $observed (@('new:bind') + @('new:login') * $loginCalls.Count)
$steps = @($observed.Result.data.steps)
$stages = [string[]] @($steps | ForEach-Object stage)
$bindCall = $observed.State.calls[[Array]::IndexOf($stages, 'bind')]
Assert-True (($loginCalls[0].at - $bindCall.at) -lt 1000) 'Login waited before its first authentication attempt.'
for ($index = 1; $index -lt $loginCalls.Count; $index++) {
    $previous = $loginCalls[$index - 1]
    Assert-True ($loginCalls[$index].received_ns -ge $previous.returned_ns) 'Login submitted another authentication before the previous response.'
    Assert-True (($loginCalls[$index].received_ns - $previous.returned_ns) / 1000000 -lt 150) 'Login delayed another authentication after the previous rejection.'
}
$firstLoginStep = @($steps | Where-Object command -eq 'p login')[0]
Assert-True ($firstLoginStep.elapsed_ms -ge 350) 'Authentication timing omitted the server request duration.'
Assert-True ($observed.Result.data.binding_activation_seconds -lt 10) 'Login waited for the entire activation timeout after authentication was available.'

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
    $case = New-Case 'migrate_online' $provider 'portal_binding_unsynchronized'
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -ne 0 -and $observed.Result.data.stage -eq 'login') 'Login ignored a portal rejection after a verified bind result.'
    Assert-Writes $observed @('old:offline', 'old:unbind', 'new:bind', 'new:login')
    Assert-True ($observed.Result.error.message -ceq 'portal rejected login: 未绑定运营商账号,请正确绑定运营商账号再试！') "Login replaced the authentication server rejection: $($observed.Result.error.message)"
    $last = @($observed.Result.data.steps)[-1]
    Assert-True ($last.command -eq 'p login' -and $last.exit_code -eq 1 -and $last.outcome -eq 'rejected' -and $last.verified -eq $false) 'Login lost the rejected authentication result or advanced after it.'
    Assert-True (-not $observed.State.online -and $observed.State.writes['new:login'] -eq 1) 'Login retried a rejected authentication submission.'
    $completed.Add("$provider portal binding rejection is preserved without retry")
}

foreach ($deadline in @(0, 1, 2)) {
    $case = New-Case 'unowned_offline' 'cmcc' 'portal_binding_unsynchronized'
    $case.BindingTimeoutSeconds = $deadline
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -eq 1) 'Login accepted an assignment that remained unavailable for authentication.'
    $loginCalls = @($observed.State.calls | Where-Object command -eq 'p login')
    Assert-True ($loginCalls.Count -ge 1) 'Login omitted the first authentication attempt.'
    if ($deadline -eq 0) { Assert-True ($loginCalls.Count -eq 1) 'Login retried authentication with a zero activation window.' }
    else {
        Assert-True (($loginCalls[-1].received_ns - $loginCalls[0].received_ns) / 1000000 -lt ($deadline * 1000 + 100)) 'Login started another authentication outside its binding activation deadline.'
    }
    Assert-Writes $observed (@('new:bind') + @('new:login') * $loginCalls.Count)
    Assert-True ($observed.Result.data.login_attempts -eq $loginCalls.Count) 'Login lost the authentication attempt count at its activation deadline.'
    for ($index = 1; $index -lt $loginCalls.Count; $index++) {
        Assert-True ($loginCalls[$index].received_ns -ge $loginCalls[$index - 1].returned_ns) 'Login started another authentication before the previous request completed.'
    }
    Assert-True ($observed.Result.error.message -ceq 'portal rejected login: 未绑定运营商账号,请正确绑定运营商账号再试！') 'Activation timeout replaced the server rejection.'
    Assert-True ($observed.Result.data.binding_activation_seconds -ge $deadline -and $observed.Result.data.binding_activation_seconds -lt ($deadline + 5)) 'Binding activation duration did not describe the elapsed deadline.'
    Assert-True (@($observed.State.calls | Where-Object command -eq 'probe').Count -eq 0) 'Login probed after its authentication deadline expired.'
    $completed.Add("new binding authentication respects the $deadline second activation deadline")
}

foreach ($scenario in @('login_password_rejected', 'login_binding_message_partial', 'login_binding_unknown', 'login_binding_unverified', 'login_binding_exit2', 'login_network_error')) {
    $case = New-Case 'unowned_offline' 'cmcc' $scenario
    $case.BindingTimeoutSeconds = 2
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -eq 1 -and $observed.Result.data.login_attempts -eq 1) "Login retried $scenario."
    Assert-Writes $observed @('new:bind', 'new:login')
    Assert-True ($observed.Result.data.binding_activation_seconds -lt 1) 'Login delayed a rejection that did not exactly describe pending binding activation.'
    $completed.Add("binding activation does not retry $scenario")
}

$case = New-Case 'ready_offline' 'cmcc' 'portal_binding_unsynchronized'
$case.BindingTimeoutSeconds = 2
$observed = Invoke-Login $case
Assert-True ($observed.Process.ExitCode -eq 1 -and $observed.Result.data.login_attempts -eq 1 -and $observed.Result.data.binding_activation_seconds -eq 0) 'Login retried authentication for an existing binding.'
Assert-Writes $observed @('new:login')
$completed.Add('existing bindings do not enter the activation retry window')

foreach ($scenario in @('close_error', 'close_wrong_envelope', 'close_eof', 'close_extra_output')) {
    $case = New-Case 'ready_online' 'cmcc' $scenario
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -eq 1 -and $observed.Result.data.online -eq $true -and $observed.Result.data.internet -eq $true) "Login ignored $scenario or lost the completed observations."
    Assert-Writes $observed
    Assert-True (@($observed.Result.data.steps | Where-Object command -eq 'close').Count -eq 0) 'Session closure became a business operation.'
    $completed.Add("CLI session closure reports $scenario")
}

foreach ($alias in @('new', 'old')) {
    $case = New-Case 'migrate_offline'
    $case.State.residual[$alias] = $true
    $observed = Invoke-Login $case
    Assert-Success $observed $case "offline portal proceeds despite residual $alias Self accounting"
    Assert-Writes $observed @('old:unbind', 'new:bind', 'new:login')
    Assert-True (@($observed.State.calls | Where-Object command -eq 'zfw online').Count -eq 0) 'Login used account-level accounting as an offline terminal prerequisite.'
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
$resume.State.session_starts = 0; $resume.State.session_closes = 0
$resume.State.fail_at = 0; $resume.State.failure_mode = ''; $resume.State.portal_reads = 0
$observed = Invoke-Login $resume
Assert-Success $observed $resume 'manual invocation recomputes a previously interrupted transfer'
Assert-Writes $observed @('new:bind', 'new:login')

foreach ($scenario in @('login_wrong_identity', 'login_mac_changes', 'login_missing_status')) {
    $case = New-Case 'migrate_online' 'cmcc' $scenario
    $observed = Invoke-Login $case
    Assert-True ($observed.Process.ExitCode -ne 0) "Login ignored $scenario."
    Assert-Writes $observed @('old:offline', 'old:unbind', 'new:bind', 'new:login')
    Assert-True (@($observed.State.calls | Where-Object command -eq 'probe').Count -eq 0) 'Login probed before the returned authenticated identity was verified.'
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
            Assert-True ($calls[-2].command -eq 'p login') 'Login repeated business calls after its verified authentication result.'
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
        $replacementPath = Join-Path $case.Directory 'replacement config.json'
        [IO.File]::WriteAllText($replacementPath, '{}', [Text.UTF8Encoding]::new($false))
        $replacementBlocked = $false
        try { [IO.File]::Move($replacementPath, $case.ConfigPath, $true) }
        catch [IO.IOException], [UnauthorizedAccessException] { $replacementBlocked = $true }
        Assert-True $replacementBlocked 'Login did not prevent configuration replacement while active.'
        $completed.Add('active login protects configuration from replacement')
        $renameBlocked = $false
        try { [IO.File]::Move($case.ConfigPath, $case.ConfigPath + '.renamed') }
        catch [IO.IOException], [UnauthorizedAccessException] { $renameBlocked = $true }
        Assert-True $renameBlocked 'Login did not prevent configuration rename while active.'
        $completed.Add('active login protects configuration from rename')
        $deleteBlocked = $false
        try { [IO.File]::Delete($case.ConfigPath) }
        catch [IO.IOException], [UnauthorizedAccessException] { $deleteBlocked = $true }
        Assert-True $deleteBlocked 'Login did not prevent configuration deletion while active.'
        $completed.Add('active login protects configuration from deletion')
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
if ($IsWindows) {
    $writer = [IO.File]::Open($case.ConfigPath, [IO.FileMode]::Open, [IO.FileAccess]::Write, [IO.FileShare]::ReadWrite)
    $writer.Dispose()
    [IO.File]::Move($case.ConfigPath, $case.ConfigPath + '.released')
    [IO.File]::Move($case.ConfigPath + '.released', $case.ConfigPath)
    $completed.Add('completed login releases its configuration file lock')
}
$case = New-Case 'ready_offline'
$observed = Invoke-Login $case
Assert-Success $observed $case 'completed login releases its source lock'

@{ result = 'passed'; cases = $caseNumber; checks = $completed.ToArray() } | ConvertTo-Json -Depth 8
