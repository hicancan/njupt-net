#Requires -Version 7.0
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
    [switch]$Probe = $true,
    [switch]$NoProbe,
    [string]$ProxyConfigPath = '',
    [ValidateNotNullOrEmpty()][string]$LogDirectory = (Join-Path ([Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)) 'njupt-net/logs')
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
[Console]::OutputEncoding = [System.Text.UTF8Encoding]::new($false)
. (Join-Path $PSScriptRoot 'diagnostics.ps1')
$parameters = [ordered]@{
    interface = $Interface; source = $Source; account_alias = $Account; config = $Config; executable = $Executable
    portal_port = $PortalPort; timeout_seconds = $TimeoutSeconds; login_timeout_seconds = $LoginTimeoutSeconds
    probe = [bool]($Probe -and -not $NoProbe); proxy_config_path = $ProxyConfigPath
}
$diagnostics = $null
$exitCode = 1
try {
    $diagnostics = New-CampusDiagnostics $LogDirectory $parameters
    $null = $diagnostics.Note.Invoke("诊断日志目录：$($diagnostics.Directory)")
    if ($diagnostics.InitializationError) { throw $diagnostics.InitializationError }
    $workflow = @{
        Account = $Account; Config = $Config; Executable = $Executable; PortalPort = $PortalPort
        TimeoutSeconds = $TimeoutSeconds; LoginTimeoutSeconds = $LoginTimeoutSeconds
        Probe = $parameters.probe; Diagnostics = $diagnostics
    }
    if ($PSCmdlet.ParameterSetName -ceq 'Interface') { $workflow.Interface = $Interface } else { $workflow.Source = $Source }
    & (Join-Path $PSScriptRoot 'login.ps1') @workflow
    $exitCode = $LASTEXITCODE
} catch {
    $message = $_.Exception.Message
    $exceptionType = $_.Exception.GetType().FullName
    if ($diagnostics) {
        $null = $diagnostics.Note.Invoke("诊断运行失败：$message")
        $stage = if ($diagnostics.InitializationError) { 'diagnostic-initialization' } else { 'diagnostic-wrapper' }
        try { $null = $diagnostics.Fail.Invoke($stage, $message) }
        catch { $null = $diagnostics.Note.Invoke("诊断结果写入失败：$($_.Exception.Message)") }
        try { $null = $diagnostics.Emit.Invoke('wrapper-error', @{ message = $message; exception_type = $exceptionType }) }
        catch { $null = $diagnostics.Note.Invoke("诊断日志写入失败：$($_.Exception.Message)") }
    } else { [Console]::Error.WriteLine("诊断运行失败：$message") }
} finally {
    if ($diagnostics) {
        try { $null = $diagnostics.Complete.Invoke($exitCode) }
        catch { $null = $diagnostics.Note.Invoke("诊断日志收尾失败：$($_.Exception.Message)") }
    }
}
exit $exitCode
