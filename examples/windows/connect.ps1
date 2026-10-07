#Requires -Version 7.0
<#
.SYNOPSIS
Connect a selected campus account and verify Internet access through that interface.
.EXAMPLE
./connect.ps1 -Interface Ethernet -Account default -Operator cmcc -Config ./config.json
#>
[CmdletBinding(DefaultParameterSetName = 'Interface')]
param(
    [Parameter(Mandatory, ParameterSetName = 'Interface')][ValidateNotNullOrEmpty()][string]$Interface,
    [Parameter(Mandatory, ParameterSetName = 'Source')][ValidateNotNullOrEmpty()][string]$Source,
    [Parameter(Mandatory)][ValidateNotNullOrEmpty()][string]$Account,
    [Parameter(Mandatory)][ValidateSet('campus', 'njxy', 'cmcc')][string]$Operator,
    [ValidateNotNullOrEmpty()][string]$Config = 'config.json',
    [ValidateNotNullOrEmpty()][string]$Executable = 'njupt-net',
    [ValidateRange(1, 300)][int]$TimeoutSeconds = 15
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'cli.psm1') -Force
$context = New-CampusContext 'connect' $Executable $Config $Interface $Source $TimeoutSeconds
$details = @{ account_alias = $Account; operator = $Operator }
try {
    Initialize-CampusContext $context @($Account)
    $state = Get-CampusStatus $context 'status'
    if ($state.online) { Assert-CampusIdentity $context $state $Account $Operator }
    else {
        $result = Invoke-CampusCommand $context 'login' $Account @('p', 'login', '--operator', $Operator)
        Assert-CampusOperation $result
        $state = Get-CampusStatus $context 'login-status'
        Assert-CampusIdentity $context $state $Account $Operator
    }
    Confirm-CampusInternet $context
    $context.Stage = 'complete'
    Write-CampusResult $context $details
    exit 0
} catch {
    Write-CampusResult $context $details $_.Exception.Message
    exit 1
} finally { Close-CampusContext $context }
