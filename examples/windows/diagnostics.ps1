#Requires -Version 7.0
Set-StrictMode -Version Latest

function Protect-CampusDiagnosticValue {
    param($State, $Value, [string]$Name = '')
    if ($Name -match '(?i)(?:password|passwd|secret|private.?key|preshared.?key|csrf|cookie|authorization|signature|(?:^|_)token(?:$|_)|^sign$|^checkcode$|^foo$|^bar$|^(?:self|operator|bridge)_url$|^self_auth_url2?$)' -and $Name -cne 'password_set') {
        return '[redacted]'
    }
    if ($null -eq $Value) { return $null }
    if ($Value -is [string]) {
        $text = $Value
        foreach ($secret in $State.Secrets) {
            if ($secret.Length -gt 0) { $text = $text.Replace($secret, '[redacted]') }
        }
        $text = [regex]::Replace($text, '(?i)https?://[^\s"<>]+', {
            param($Match)
            try {
                $uri = [uri]$Match.Value
                $path = [regex]::Replace($uri.AbsolutePath, '(?i)(?:;|%3b)jsessionid(?:=|%3d)[^/?#;]+', ';jsessionid=[redacted]')
                return $uri.GetComponents([System.UriComponents]::SchemeAndServer, [System.UriFormat]::UriEscaped) + $path
            } catch { return '[redacted URL]' }
        })
        $text = [regex]::Replace($text, '(?i)\b(password|passwd|csrftoken|csrf|cookie|authorization|signature|token|secret|checkcode|sign)\s*[:=]\s*[^\s,;]+', '$1=[redacted]')
        return $text
    }
    if ($Value -is [System.Collections.IDictionary]) {
        $copy = [ordered]@{}
        foreach ($key in $Value.Keys) { $copy[[string]$key] = Protect-CampusDiagnosticValue $State $Value[$key] ([string]$key) }
        return $copy
    }
    if ($Value -is [System.Collections.IEnumerable]) {
        $copy = [System.Collections.Generic.List[object]]::new()
        foreach ($item in $Value) { $copy.Add((Protect-CampusDiagnosticValue $State $item)) }
        return ,$copy.ToArray()
    }
    if ($Value -is [System.Management.Automation.PSCustomObject]) {
        $copy = [ordered]@{}
        foreach ($property in $Value.PSObject.Properties) { $copy[$property.Name] = Protect-CampusDiagnosticValue $State $property.Value $property.Name }
        return $copy
    }
    return $Value
}

function Register-CampusDiagnosticSecrets {
    param($State, $Value, [string]$Name = '')
    if ($Value -is [string]) {
        if ($Name -match '(?i)password|passwd|secret|private.?key|preshared.?key|csrf|cookie|signature|token' -and $Value.Length -gt 0 -and -not $State.Secrets.Contains($Value)) {
            $State.Secrets.Add($Value)
        }
    } elseif ($Value -is [System.Collections.IDictionary]) {
        foreach ($key in $Value.Keys) { Register-CampusDiagnosticSecrets $State $Value[$key] ([string]$key) }
    } elseif ($Value -is [System.Collections.IEnumerable]) {
        foreach ($item in $Value) { Register-CampusDiagnosticSecrets $State $item $Name }
    }
}

function Write-CampusDiagnosticFile {
    param($State, [string]$Name, $Value)
    $safe = Protect-CampusDiagnosticValue $State $Value
    $json = ConvertTo-Json -InputObject $safe -Depth 40
    [System.IO.File]::WriteAllText((Join-Path $State.Directory $Name), $json + [Environment]::NewLine, [System.Text.UTF8Encoding]::new($false))
}

function Add-CampusDiagnosticConsole {
    param($State, [string]$Stream, [string]$Text)
    $safe = Protect-CampusDiagnosticValue $State $Text
    [System.IO.File]::AppendAllText((Join-Path $State.Directory ('console.' + $Stream + '.txt')), $safe + [Environment]::NewLine, [System.Text.UTF8Encoding]::new($false))
}

function Write-CampusDiagnosticNote {
    param($State, [string]$Message)
    $safe = Protect-CampusDiagnosticValue $State $Message
    [Console]::Error.WriteLine($safe)
    try { Add-CampusDiagnosticConsole $State 'stderr' $safe }
    catch {
        $failure = Protect-CampusDiagnosticValue $State $_.Exception.Message
        $State.Errors.Add($failure)
        [Console]::Error.WriteLine("诊断控制台日志写入失败：$failure")
    }
}

function Save-CampusDiagnosticWorkflowResult {
    param($State)
    if (-not $State.WorkflowResult) { return }
    if (-not $State.ResultRecorded) {
        Write-CampusDiagnosticFile $State 'result.json' $State.WorkflowResult.result
        $State.ResultRecorded = $true
    }
    if (-not $State.ResultConsoleRecorded) {
        Add-CampusDiagnosticConsole $State $State.WorkflowResult.stream $State.WorkflowResult.console
        $State.ResultConsoleRecorded = $true
    }
}

function Save-CampusDiagnosticFailureResult {
    param($State, [string]$Stage, [string]$Message)
    if ($State.WorkflowResult) { Save-CampusDiagnosticWorkflowResult $State; return }
    if ($State.ResultRecorded) { return }
    $result = [ordered]@{
        command = 'diagnostics'
        data = [ordered]@{ stage = $Stage; account_alias = $State.Parameters.account_alias; source = $State.Parameters.source }
        error = [ordered]@{ message = $Message }
    }
    Write-CampusDiagnosticFile $State 'result.json' $result
    $State.ResultRecorded = $true
}

function Write-CampusDiagnosticEvent {
    param($State, [string]$Kind, $Data)
    if ($Kind -ceq 'result') { $State.WorkflowResult = $Data }
    $State.Sequence++
    $event = [ordered]@{
        sequence = $State.Sequence
        epoch_ms = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
        utc = [DateTimeOffset]::UtcNow.ToString('O')
        elapsed_ms = [Math]::Round([System.Diagnostics.Stopwatch]::GetElapsedTime($State.StartedAt).TotalMilliseconds, 3)
        event = $Kind
        data = Protect-CampusDiagnosticValue $State $Data
    }
    $State.Writer.WriteLine((ConvertTo-Json -InputObject $event -Depth 40 -Compress))
    $State.Writer.Flush()
    if ($Kind -ceq 'cli-response' -and $Data.response -is [System.Collections.IDictionary] -and $Data.response.Contains('command') -and $Data.response.command -ceq 'version') {
        $State.Run['cli_version'] = Protect-CampusDiagnosticValue $State $Data.response.data
        Write-CampusDiagnosticFile $State 'run.json' $State.Run
    }
    if ($Kind -ceq 'cli-response' -and $Data.response -is [System.Collections.IDictionary] -and $Data.response.Contains('trace_error')) {
        $message = Protect-CampusDiagnosticValue $State ([string]$Data.response.trace_error)
        $State.Errors.Add($message)
        Write-CampusDiagnosticNote $State "HTTP 诊断日志写入失败：$message"
    }
    if ($Kind -ceq 'session-start') { $State.CLIProcessID = $Data.process_id }
    if ($Kind -ceq 'workflow-error' -and $State.CLIProcessID) {
        try {
            $sockets = @(Get-NetTCPConnection -OwningProcess $State.CLIProcessID -ErrorAction Stop | Select-Object OwningProcess, LocalAddress, LocalPort, RemoteAddress, RemotePort, State)
            Write-CampusDiagnosticEvent $State 'socket-snapshot' @{ process_id = $State.CLIProcessID; observation = 'TCP sockets present at this local snapshot.'; sockets = $sockets }
        } catch { Write-CampusDiagnosticEvent $State 'socket-snapshot' @{ process_id = $State.CLIProcessID; error = $_.Exception.Message } }
    }
    if ($Kind -ceq 'result') {
        Save-CampusDiagnosticWorkflowResult $State
    }
}

function Get-CampusDiagnosticFileIdentity {
    param([string]$Path)
    try {
        $file = Get-Item -LiteralPath $Path -ErrorAction Stop
        return [ordered]@{ path = $file.FullName; sha256 = (Get-FileHash -LiteralPath $file.FullName -Algorithm SHA256).Hash; version = $file.VersionInfo.FileVersion; product_version = $file.VersionInfo.ProductVersion }
    } catch { return [ordered]@{ path = $Path; error = $_.Exception.Message } }
}

function Get-CampusDiagnosticRouteChoice {
    param([string]$Remote, $InterfaceIndex, [string]$LocalAddress = '')
    $parameters = @{ RemoteIPAddress = $Remote; ErrorAction = 'Stop' }
    if ($InterfaceIndex -and $LocalAddress) { $parameters.InterfaceIndex = $InterfaceIndex; $parameters.LocalIPAddress = $LocalAddress }
    $result = [ordered]@{ address = $null; route = $null }
    foreach ($choice in @(Find-NetRoute @parameters)) {
        if ($choice.PSObject.Properties['IPAddress']) {
            $result.address = $choice | Select-Object IPAddress, InterfaceIndex, InterfaceAlias, PrefixLength, AddressState
        } elseif ($choice.PSObject.Properties['DestinationPrefix']) {
            $result.route = $choice | Select-Object DestinationPrefix, InterfaceIndex, InterfaceAlias, NextHop, RouteMetric, InterfaceMetric
        }
    }
    return $result
}

function Get-CampusDiagnosticProxyConfiguration {
    param([string]$Path)
    try {
        $configuration = Get-Content -LiteralPath $Path -Raw -ErrorAction Stop | ConvertFrom-Json -AsHashtable -ErrorAction Stop
        $result = [ordered]@{ configuration = $Path; available = $true; inbounds = @(); route = [ordered]@{} }
        $inbounds = [System.Collections.Generic.List[object]]::new()
        if ($configuration -is [System.Collections.IDictionary] -and $configuration.Contains('inbounds')) {
            foreach ($inbound in $configuration.inbounds) {
                if ($inbound -isnot [System.Collections.IDictionary] -or -not $inbound.Contains('type') -or $inbound.type -cne 'tun') { continue }
                $fields = [ordered]@{}
                foreach ($field in @('type', 'tag', 'interface_name', 'address', 'mtu', 'auto_route', 'strict_route', 'stack', 'route_exclude_address')) {
                    if ($inbound.Contains($field)) { $fields[$field] = $inbound[$field] }
                }
                $inbounds.Add($fields)
            }
        }
        $result.inbounds = $inbounds.ToArray()
        if ($configuration -is [System.Collections.IDictionary] -and $configuration.Contains('route') -and $configuration.route -is [System.Collections.IDictionary]) {
            foreach ($field in @('auto_detect_interface', 'default_interface')) {
                if ($configuration.route.Contains($field)) { $result.route[$field] = $configuration.route[$field] }
            }
        }
        return $result
    } catch { return [ordered]@{ configuration = $Path; available = $false; error = $_.Exception.Message } }
}

function Get-CampusDiagnosticSnapshot {
    param($State)
    $snapshot = [ordered]@{
        epoch_ms = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
        utc = [DateTimeOffset]::UtcNow.ToString('O')
        observation = 'Local network configuration and process state.'
    }
    $queries = [ordered]@{
        adapters = { Get-NetAdapter -ErrorAction Stop | Select-Object Name, InterfaceDescription, ifIndex, Status, MacAddress, LinkSpeed }
        interfaces = { Get-NetIPInterface -ErrorAction Stop | Select-Object InterfaceAlias, InterfaceIndex, AddressFamily, ConnectionState, AutomaticMetric, InterfaceMetric, NlMtu, Dhcp }
        addresses = { Get-NetIPAddress -ErrorAction Stop | Select-Object InterfaceAlias, InterfaceIndex, AddressFamily, IPAddress, PrefixLength, AddressState, SkipAsSource }
        routes = { Get-NetRoute -ErrorAction Stop | Select-Object InterfaceAlias, InterfaceIndex, AddressFamily, DestinationPrefix, NextHop, RouteMetric, State, PolicyStore }
        dns = { Get-DnsClientServerAddress -ErrorAction Stop | Select-Object InterfaceAlias, InterfaceIndex, AddressFamily, ServerAddresses }
        proxy = {
            $proxy = Get-ItemProperty -LiteralPath 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Internet Settings' -ErrorAction Stop
            $values = [ordered]@{}
            foreach ($name in @('ProxyEnable', 'ProxyServer', 'ProxyOverride', 'AutoConfigURL')) {
                $property = $proxy.PSObject.Properties[$name]
                if ($property) { $values[$name] = $property.Value }
            }
            $values
        }
    }
    foreach ($name in $queries.Keys) {
        try { $snapshot[$name] = @(& $queries[$name]) }
        catch { $snapshot[$name] = @{ error = $_.Exception.Message } }
    }
    $selectedSource = $State.Parameters.source
    $selectedIndex = $null
    if ($snapshot.addresses -is [array]) {
        $selected = @($snapshot.addresses | Where-Object {
            if ($selectedSource) { $_.IPAddress -ceq $selectedSource }
            else { $_.InterfaceAlias -ceq $State.Parameters.interface -and $_.AddressFamily -eq 'IPv4' }
        })
        if ($selected.Count -eq 1) { $selectedSource = $selected[0].IPAddress; $selectedIndex = $selected[0].InterfaceIndex }
    }
    $choices = [System.Collections.Generic.List[object]]::new()
    foreach ($remote in @('10.10.244.11', '10.10.244.240', '1.1.1.1')) {
        try {
            $route = Get-CampusDiagnosticRouteChoice $remote
            $choices.Add(@{ remote = $remote; selection = 'system'; basis = 'Local Windows route selection.'; result = $route })
        } catch { $choices.Add(@{ remote = $remote; selection = 'system'; error = $_.Exception.Message }) }
        if ($selectedIndex -and $selectedSource) {
            try {
                $route = Get-CampusDiagnosticRouteChoice $remote $selectedIndex $selectedSource
                $choices.Add(@{ remote = $remote; selection = 'selected-interface'; basis = 'Local Windows route selection.'; result = $route })
            } catch { $choices.Add(@{ remote = $remote; selection = 'selected-interface'; error = $_.Exception.Message }) }
        }
    }
    $snapshot.route_choices = $choices.ToArray()
    $processes = [System.Collections.Generic.List[object]]::new()
    $tun = [System.Collections.Generic.List[object]]::new()
    $configurationPaths = [System.Collections.Generic.HashSet[string]]::new([System.StringComparer]::OrdinalIgnoreCase)
    if ($State.Parameters.Contains('proxy_config_path') -and $State.Parameters.proxy_config_path) {
        $null = $configurationPaths.Add($State.Parameters.proxy_config_path)
    }
    try {
        foreach ($process in @(Get-Process -ErrorAction Stop | Where-Object { $_.ProcessName -in @('v2rayN', 'sing-box', 'xray', 'wireguard', 'njupt-net', 'pwsh') })) {
            $entry = [ordered]@{ name = $process.ProcessName; id = $process.Id }
            try { $entry.started_utc = $process.StartTime.ToUniversalTime().ToString('O'); $entry.executable = Get-CampusDiagnosticFileIdentity $process.Path }
            catch { $entry.error = $_.Exception.Message }
            $processes.Add($entry)
            if ($process.ProcessName -eq 'v2rayN' -and $process.Path) {
                foreach ($relative in @('binConfigs/config.json', 'guiConfigs/config.json')) {
                    $path = Join-Path (Split-Path -Parent $process.Path) $relative
                    if (-not (Test-Path -LiteralPath $path -PathType Leaf)) { continue }
                    $null = $configurationPaths.Add($path)
                }
            }
        }
    } catch { $snapshot.process_error = $_.Exception.Message }
    foreach ($path in $configurationPaths) { $tun.Add((Get-CampusDiagnosticProxyConfiguration $path)) }
    if ($tun.Count -eq 0) { $tun.Add(@{ available = $false; reason = 'No proxy-core configuration path was found. Specify -ProxyConfigPath to record TUN settings.' }) }
    $snapshot.processes = $processes.ToArray()
    $snapshot.tun = $tun.ToArray()
    return $snapshot
}

function New-CampusDiagnostics {
    param([string]$LogDirectory, [System.Collections.IDictionary]$Parameters)
    $root = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($LogDirectory)
    $runID = [DateTimeOffset]::UtcNow.ToString('yyyyMMddTHHmmssfffZ') + '-' + [guid]::NewGuid().ToString('N')
    $directory = Join-Path $root $runID
    $null = [System.IO.Directory]::CreateDirectory($directory)
    $state = @{
        Directory = $directory; Writer = $null; StartedAt = [System.Diagnostics.Stopwatch]::GetTimestamp(); Sequence = 0
        Secrets = [System.Collections.Generic.List[string]]::new(); Errors = [System.Collections.Generic.List[string]]::new()
        Parameters = $Parameters; ResultRecorded = $false; ResultConsoleRecorded = $false; WorkflowResult = $null; CLIProcessID = $null; InitializationError = $null
        Run = [ordered]@{ run_id = $runID; epoch_ms = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds(); started_utc = [DateTimeOffset]::UtcNow.ToString('O'); parameters = $Parameters; process_id = $PID; powershell_version = $PSVersionTable.PSVersion.ToString() }
    }
    try {
        foreach ($name in @('console.stdout.txt', 'console.stderr.txt')) {
            [System.IO.File]::WriteAllText((Join-Path $directory $name), '', [System.Text.UTF8Encoding]::new($false))
        }
        $state.Writer = [System.IO.StreamWriter]::new([System.IO.FileStream]::new((Join-Path $directory 'events.jsonl'), [System.IO.FileMode]::CreateNew, [System.IO.FileAccess]::Write, [System.IO.FileShare]::Read), [System.Text.UTF8Encoding]::new($false))
        $state.Run['scripts'] = @(Get-CampusDiagnosticFileIdentity (Join-Path $PSScriptRoot 'login-test.ps1'); Get-CampusDiagnosticFileIdentity (Join-Path $PSScriptRoot 'login.ps1'); Get-CampusDiagnosticFileIdentity (Join-Path $PSScriptRoot 'diagnostics.ps1'))
        try {
            $application = Get-Command -Name $Parameters.executable -CommandType Application -ErrorAction Stop | Select-Object -First 1
            $state.Run['executable'] = Get-CampusDiagnosticFileIdentity $application.Source
        } catch { $state.Run['executable'] = @{ command = $Parameters.executable; error = $_.Exception.Message } }
        Write-CampusDiagnosticFile $state 'run.json' $state.Run
        Write-CampusDiagnosticEvent $state 'run-start' @{ run_id = $runID; parameters = $Parameters }
        $startedAt = [System.Diagnostics.Stopwatch]::GetTimestamp()
        Write-CampusDiagnosticFile $state 'before.json' (Get-CampusDiagnosticSnapshot $state)
        Write-CampusDiagnosticEvent $state 'snapshot-before' @{ elapsed_ms = [System.Diagnostics.Stopwatch]::GetElapsedTime($startedAt).TotalMilliseconds }
    } catch {
        $state.InitializationError = Protect-CampusDiagnosticValue $state $_.Exception.Message
        $state.Errors.Add($state.InitializationError)
        try { Write-CampusDiagnosticFile $state 'before.json' @{ error = $state.InitializationError; observation = 'Local initialization snapshot failed.' } }
        catch { $state.Errors.Add((Protect-CampusDiagnosticValue $state $_.Exception.Message)) }
        try { Save-CampusDiagnosticFailureResult $state 'diagnostic-initialization' $state.InitializationError }
        catch { $state.Errors.Add((Protect-CampusDiagnosticValue $state $_.Exception.Message)) }
    }
    return [pscustomobject]@{
        Directory = $directory; TraceFile = (Join-Path $directory 'http.jsonl'); Errors = $state.Errors; InitializationError = $state.InitializationError
        Emit = { param([string]$Kind, $Data) Write-CampusDiagnosticEvent $state $Kind $Data }.GetNewClosure()
        Note = { param([string]$Message) Write-CampusDiagnosticNote $state $Message }.GetNewClosure()
        Fail = { param([string]$Stage, [string]$Message) Save-CampusDiagnosticFailureResult $state $Stage $Message }.GetNewClosure()
        RegisterSecrets = { param($Settings) Register-CampusDiagnosticSecrets $state $Settings }.GetNewClosure()
        Complete = {
            param([int]$ExitCode)
            try {
                if ($state.WorkflowResult) { Save-CampusDiagnosticWorkflowResult $state }
                elseif (-not $state.ResultRecorded) { Save-CampusDiagnosticFailureResult $state 'diagnostic-wrapper' 'The diagnostic run ended before the login workflow produced a result.' }
                $trace = Join-Path $state.Directory 'http.jsonl'
                if (-not [System.IO.File]::Exists($trace)) { [System.IO.File]::WriteAllText($trace, '', [System.Text.UTF8Encoding]::new($false)) }
                $startedAt = [System.Diagnostics.Stopwatch]::GetTimestamp()
                try { $snapshot = Get-CampusDiagnosticSnapshot $state }
                catch {
                    $message = Protect-CampusDiagnosticValue $state $_.Exception.Message
                    $state.Errors.Add($message)
                    Write-CampusDiagnosticNote $state "诊断结束快照采集失败：$message"
                    $snapshot = @{ error = $message; observation = 'Local final snapshot failed.' }
                }
                Write-CampusDiagnosticFile $state 'after.json' $snapshot
                Write-CampusDiagnosticEvent $state 'snapshot-after' @{ elapsed_ms = [System.Diagnostics.Stopwatch]::GetElapsedTime($startedAt).TotalMilliseconds }
                $state.Run['finished_epoch_ms'] = [DateTimeOffset]::UtcNow.ToUnixTimeMilliseconds()
                $state.Run['finished_utc'] = [DateTimeOffset]::UtcNow.ToString('O')
                $state.Run['elapsed_ms'] = [Math]::Round([System.Diagnostics.Stopwatch]::GetElapsedTime($state.StartedAt).TotalMilliseconds, 3)
                $state.Run['exit_code'] = $ExitCode
                $state.Run['diagnostic_errors'] = $state.Errors.ToArray()
                Write-CampusDiagnosticFile $state 'run.json' $state.Run
                Write-CampusDiagnosticEvent $state 'run-exit' @{ exit_code = $ExitCode; elapsed_ms = $state.Run.elapsed_ms; diagnostic_errors = $state.Errors.ToArray() }
            } finally { if ($state.Writer) { $state.Writer.Dispose() } }
        }.GetNewClosure()
    }
}
