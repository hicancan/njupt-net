<#
.SYNOPSIS
Install njupt-net guard as a Windows service via the official WinSW wrapper.

.DESCRIPTION
This script runs on the local Windows machine. It downloads the official WinSW
release into dist/winsw/, generates the service XML beside the wrapper, installs
the service, and starts it.

The repository keeps only this installer script under version control. Generated
wrapper binaries, XML, logs, and runtime state stay under dist/ and remain ignored.

.EXAMPLE
.\scripts\install-windows-service.ps1

.EXAMPLE
.\scripts\install-windows-service.ps1 -ConfigPath .\config.json -StateDir .\dist\guard
#>
param(
    [string]$ServiceID = "njuptnetguard",
    [string]$DisplayName = "NJUPT Net Guard",
    [string]$Description = "Runs njupt-net guard as a Windows service via WinSW.",
    [string]$BinaryPath = ".\dist\njupt-net.exe",
    [string]$ConfigPath = "",
    [string]$StateDir = ".\dist\guard",
    [string]$WinSWVersion = "v2.12.0",
    [switch]$RefreshWinSW,
    [switch]$ReplaceLegacy
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

function Resolve-InputPath {
    param(
        [Parameter(Mandatory = $true)][string]$Path,
        [Parameter(Mandatory = $true)][string]$Label
    )

    if (-not (Test-Path $Path)) {
        throw "$Label not found: $Path"
    }
    return (Resolve-Path $Path).Path
}

function Resolve-ConfigPath {
    param([string]$Path)
    if (-not [string]::IsNullOrWhiteSpace($Path)) {
        return Resolve-InputPath -Path $Path -Label "Config"
    }
    if (Test-Path ".\config.json") {
        return (Resolve-Path ".\config.json").Path
    }
    throw "Config not found. Pass -ConfigPath explicitly or create .\config.json from .\config.example.json."
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

function Escape-Xml {
    param([Parameter(Mandatory = $true)][string]$Value)
    return [System.Security.SecurityElement]::Escape($Value)
}

function Ensure-WinSW {
    param(
        [Parameter(Mandatory = $true)][string]$Directory,
        [Parameter(Mandatory = $true)][string]$ExecutablePath,
        [Parameter(Mandatory = $true)][string]$Version,
        [Parameter(Mandatory = $true)][bool]$ForceRefresh
    )

    New-Item -ItemType Directory -Force -Path $Directory | Out-Null
    if ((-not $ForceRefresh) -and (Test-Path $ExecutablePath)) {
        return
    }

    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { $asset = "WinSW-x64.exe" }
        default { throw "Unsupported Windows architecture for bundled installer: $env:PROCESSOR_ARCHITECTURE" }
    }

    $uri = "https://github.com/winsw/winsw/releases/download/$Version/$asset"
    Invoke-WebRequest -Uri $uri -OutFile $ExecutablePath
}

if (-not (Test-IsAdmin)) {
    $hostExe = (Get-Process -Id $PID).Path
    $args = @(
        "-NoProfile",
        "-ExecutionPolicy", "Bypass",
        "-File", ('"' + $PSCommandPath + '"')
    )
    foreach ($entry in $MyInvocation.BoundParameters.GetEnumerator()) {
        if ($entry.Value -is [switch] -or $entry.Value -is [System.Management.Automation.SwitchParameter]) {
            if ($entry.Value.IsPresent) {
                $args += "-$($entry.Key)"
            }
            continue
        }
        $args += "-$($entry.Key)"
        $args += ('"' + [string]$entry.Value + '"')
    }
    $process = Start-Process -FilePath $hostExe -Verb RunAs -ArgumentList $args -PassThru -Wait
    exit $process.ExitCode
}

$binary = Resolve-InputPath -Path $BinaryPath -Label "Binary"
$config = Resolve-ConfigPath -Path $ConfigPath
$state = if (Test-Path $StateDir) { (Resolve-Path $StateDir).Path } else { [System.IO.Path]::GetFullPath($StateDir) }
$serviceDir = Join-Path $RepoRoot "dist\winsw"
$wrapperPath = Join-Path $serviceDir "njupt-net-guard-service.exe"
$xmlPath = Join-Path $serviceDir "njupt-net-guard-service.xml"

Ensure-WinSW -Directory $serviceDir -ExecutablePath $wrapperPath -Version $WinSWVersion -ForceRefresh:$RefreshWinSW.IsPresent
New-Item -ItemType Directory -Force -Path $state | Out-Null

$serviceIDXml = Escape-Xml $ServiceID
$displayNameXml = Escape-Xml $DisplayName
$descriptionXml = Escape-Xml $Description
$binaryXml = Escape-Xml $binary
$repoRootXml = Escape-Xml ([string]$RepoRoot)
$arguments = @(
    "--config",
    ('"' + $config + '"'),
    "guard",
    "run",
    "--state-dir",
    ('"' + $state + '"'),
    "--yes"
)
if ($ReplaceLegacy.IsPresent) {
    $arguments += "--replace"
}
$argumentsXml = Escape-Xml ($arguments -join " ")

$xml = @"
<service>
  <id>$serviceIDXml</id>
  <name>$displayNameXml</name>
  <description>$descriptionXml</description>
  <executable>$binaryXml</executable>
  <arguments>$argumentsXml</arguments>
  <workingdirectory>$repoRootXml</workingdirectory>
  <startmode>Automatic</startmode>
  <delayedAutoStart>true</delayedAutoStart>
  <serviceaccount>
    <user>LocalSystem</user>
  </serviceaccount>
  <logpath>%BASE%\logs</logpath>
  <log mode="append" />
  <onfailure action="restart" delay="10 sec" />
  <resetfailure>1 hour</resetfailure>
  <stoptimeout>30 sec</stoptimeout>
  <stopparentprocessfirst>true</stopparentprocessfirst>
</service>
"@

Set-Content -LiteralPath $xmlPath -Value $xml -Encoding ASCII

$existing = Get-Service -Name $ServiceID -ErrorAction SilentlyContinue
if ($existing) {
    try {
        Invoke-WinSW -Executable $wrapperPath -Arguments @("stop")
    }
    catch {
        Write-Warning $_.Exception.Message
    }
    Invoke-WinSW -Executable $wrapperPath -Arguments @("uninstall")
}

Invoke-WinSW -Executable $wrapperPath -Arguments @("install")
Invoke-WinSW -Executable $wrapperPath -Arguments @("start")

Get-Service -Name $ServiceID | Format-List Name, Status, StartType
