#Requires -Version 7.0
<#
.SYNOPSIS
Move the configured broadband binding to another campus account and connect it.
.EXAMPLE
./switch.ps1 -Interface Ethernet -From old -To new -Config ./config.json
#>
[CmdletBinding(DefaultParameterSetName = 'Interface')]
param(
    [Parameter(Mandatory, ParameterSetName = 'Interface')][ValidateNotNullOrEmpty()][string]$Interface,
    [Parameter(Mandatory, ParameterSetName = 'Source')][ValidateNotNullOrEmpty()][string]$Source,
    [Parameter(Mandatory)][ValidateNotNullOrEmpty()][string]$From,
    [Parameter(Mandatory)][ValidateNotNullOrEmpty()][string]$To,
    [ValidateNotNullOrEmpty()][string]$Config = 'config.json',
    [ValidateNotNullOrEmpty()][string]$Executable = 'njupt-net',
    [ValidateRange(1, 300)][int]$TimeoutSeconds = 15
)
Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
Import-Module (Join-Path $PSScriptRoot 'cli.psm1') -Force
$context = New-CampusContext 'switch' $Executable $Config $Interface $Source $TimeoutSeconds
$details = @{ from = $From; to = $To; operator = $null }
try {
    if ($From -ceq $To) { throw 'Switch requires distinct source and target account aliases.' }
    Initialize-CampusContext $context @($From, $To)
    $context.Stage = 'preconditions'
    $settings = $context.Settings
    if ($settings.accounts[$From].account -ceq $settings.accounts[$To].account) { throw 'Switch requires two different campus accounts.' }
    if (-not $settings.Contains('broadband_account') -or $settings.broadband_account -isnot [System.Collections.IDictionary]) { throw 'Switch requires a configured broadband account.' }
    $broadband = $settings.broadband_account
    if (-not $broadband.Contains('operator') -or $broadband.operator -cnotin @('njxy', 'cmcc') -or
        -not $broadband.Contains('account') -or $broadband.account -isnot [string] -or [string]::IsNullOrEmpty($broadband.account) -or
        -not $broadband.Contains('password') -or $broadband.password -isnot [string] -or [string]::IsNullOrEmpty($broadband.password)) { throw 'Switch requires a broadband operator, account and password.' }
    $operator = $broadband.operator
    $details.operator = $operator
    $state = Get-CampusStatus $context 'status'
    Assert-CampusIdentity $context $state $From $operator
    foreach ($alias in @($From, $To)) {
        $verified = Invoke-CampusCommand $context 'verify-account' $alias @('zfw', 'verify')
        if ($verified.identity_verified -isnot [bool] -or $verified.identity_verified -ne $true -or
            $verified.credentials_valid -isnot [bool] -or $verified.credentials_valid -ne $true -or $verified.account_alias -cne $alias) { throw 'Campus account credentials were not verified.' }
    }
    $oldBindings = Get-CampusBindings $context 'old-binding' $From
    $newBindings = Get-CampusBindings $context 'new-binding' $To
    if ($oldBindings[$operator].account -cne $broadband.account -or $oldBindings[$operator].password_set -ne $true) { throw 'Old account does not hold the configured broadband binding.' }
    if ($newBindings[$operator].account -cne '' -or $newBindings[$operator].password_set -ne $false) { throw 'Target account already has a binding for this operator.' }
    $connection = Get-CampusConnection $context $From $state
    Disconnect-CampusTerminal $context $From $state $connection
    $oldCurrent = Get-CampusBindings $context 'check-old-binding' $From
    if ($oldCurrent[$operator].account -cne $broadband.account -or $oldCurrent[$operator].password_set -ne $true) { throw 'Old broadband binding changed before unbinding.' }
    $targetCurrent = Get-CampusBindings $context 'check-target-before-unbind' $To
    if ($targetCurrent[$operator].account -cne '' -or $targetCurrent[$operator].password_set -ne $false) { throw 'Target broadband binding changed before unbinding.' }
    $cleared = Invoke-CampusCommand $context 'unbind' $From @('zfw', 'operator', '--unbind', $operator)
    Assert-CampusOperation $cleared
    if ($cleared.operator -cne $operator -or $cleared.account -cne '' -or $cleared.bindings[$operator].account -cne '' -or $cleared.bindings[$operator].password_set -ne $false) { throw 'Old broadband binding was not cleared.' }
    $newCurrent = Get-CampusBindings $context 'check-new-binding' $To
    if ($newCurrent[$operator].account -cne '' -or $newCurrent[$operator].password_set -ne $false) { throw 'Target broadband binding changed before binding.' }
    $bound = Invoke-CampusCommand $context 'bind' $To @('zfw', 'operator', '--bind')
    Assert-CampusOperation $bound
    if ($bound.operator -cne $operator -or $bound.account -cne $broadband.account -or $bound.bindings[$operator].account -cne $broadband.account -or $bound.bindings[$operator].password_set -ne $true) { throw 'Target broadband binding does not match the configured account.' }
    $loggedIn = Invoke-CampusCommand $context 'login' $To @('p', 'login', '--operator', $operator)
    Assert-CampusOperation $loggedIn
    $after = Get-CampusStatus $context 'login-status'
    Assert-CampusIdentity $context $after $To $operator
    if ((ConvertTo-CampusMac $after.terminal.mac) -cne (ConvertTo-CampusMac $state.terminal.mac)) { throw 'Terminal MAC changed during account switching.' }
    Confirm-CampusInternet $context
    $oldFinal = Get-CampusBindings $context 'final-old-binding' $From
    $newFinal = Get-CampusBindings $context 'final-new-binding' $To
    if ($oldFinal[$operator].account -cne '' -or $oldFinal[$operator].password_set -ne $false -or
        $newFinal[$operator].account -cne $broadband.account -or $newFinal[$operator].password_set -ne $true) { throw 'Final broadband bindings do not match the completed transfer.' }
    $context.Stage = 'complete'
    Write-CampusResult $context $details
    exit 0
} catch {
    Write-CampusResult $context $details $_.Exception.Message
    exit 1
} finally { Close-CampusContext $context }
