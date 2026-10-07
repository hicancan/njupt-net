#Requires -Version 7.0
<#
.SYNOPSIS
Disconnect this terminal through Self while keeping its broadband binding.
.EXAMPLE
./disconnect.ps1 -Interface Ethernet -Account default -Config ./config.json
#>
[CmdletBinding(DefaultParameterSetName = 'Interface')]
param(
    [Parameter(Mandatory, ParameterSetName = 'Interface')][ValidateNotNullOrEmpty()][string]$Interface,
    [Parameter(Mandatory, ParameterSetName = 'Source')][ValidateNotNullOrEmpty()][string]$Source,
    [Parameter(Mandatory)][ValidateNotNullOrEmpty()][string]$Account,
    [ValidateNotNullOrEmpty()][string]$Config = 'config.json',
    [ValidateNotNullOrEmpty()][string]$Executable = 'njupt-net',
    [ValidateRange(1, 300)][int]$TimeoutSeconds = 15
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'cli.psm1') -Force
$context = New-CampusContext 'disconnect' $Executable $Config $Interface $Source $TimeoutSeconds
$details = @{ account_alias = $Account }
try {
    Initialize-CampusContext $context @($Account)
    $state = Get-CampusStatus $context 'status'
    if ($state.online) {
        Assert-CampusIdentity $context $state $Account
        $connection = Get-CampusConnection $context $Account $state
        Disconnect-CampusTerminal $context $Account $state $connection
    } else {
        $connections = Invoke-CampusCommand $context 'offline-connections' $Account @('zfw', 'online')
        if ($connections -isnot [array] -or @($connections | Where-Object { $_.ip -ceq $context.Source }).Count -ne 0) {
            throw 'Portal is offline, but Self has not confirmed removal of this terminal.'
        }
    }
    $context.Stage = 'complete'
    Write-CampusResult $context $details
    exit 0
} catch {
    Write-CampusResult $context $details $_.Exception.Message
    exit 1
} finally { Close-CampusContext $context }
