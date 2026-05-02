<#
.SYNOPSIS
Uninstall the local njupt-net Windows service created by install-windows-service.ps1.
#>
param(
    [string]$ServiceID = "njuptnetguard"
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $RepoRoot

function Test-IsAdmin {
    $identity = [Security.Principal.WindowsIdentity]::GetCurrent()
    $principal = New-Object Security.Principal.WindowsPrincipal($identity)
    return $principal.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)
}

function Invoke-WinSW {
    param(
        [Parameter(Mandatory = $true)][string]$Executable,
        [Parameter(Mandatory = $true)][string[]]$Arguments
    )

    & $Executable @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "WinSW command failed: $Executable $($Arguments -join ' ') (exit code $LASTEXITCODE)"
    }
}

if (-not (Test-IsAdmin)) {
    $hostExe = (Get-Process -Id $PID).Path
    $args = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", ('"' + $PSCommandPath + '"')
    )
    foreach ($entry in $MyInvocation.BoundParameters.GetEnumerator()) {
        $args += "-$($entry.Key)"
        $args += ('"' + [string]$entry.Value + '"')
    }
    $process = Start-Process -FilePath $hostExe -Verb RunAs -ArgumentList $args -PassThru -Wait
    exit $process.ExitCode
}

$wrapperPath = Join-Path $RepoRoot "dist\winsw\njupt-net-guard-service.exe"
if (-not (Test-Path $wrapperPath)) {
    throw "WinSW wrapper not found: $wrapperPath"
}

if (-not (Get-Service -Name $ServiceID -ErrorAction SilentlyContinue)) {
    Write-Host "Service $ServiceID is not installed."
    exit 0
}

try {
    Invoke-WinSW -Executable $wrapperPath -Arguments @("stop")
}
catch {
    Write-Warning $_.Exception.Message
}

Invoke-WinSW -Executable $wrapperPath -Arguments @("uninstall")
Write-Host "Service $ServiceID removed."
