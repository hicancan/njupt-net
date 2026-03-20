<#
.SYNOPSIS
Deploy njupt-net to an ImmortalWrt/OpenWrt arm64 router and install it as a procd service.

.DESCRIPTION
This script runs on the local machine. It uploads the linux/arm64 binary and optionally
config.json to the remote router over scp/ssh, installs the binary under /usr/bin, creates
/etc/init.d/njupt-net, enables it, restarts it, and verifies guard status.

The supported runtime model on the router is:
- procd service
- foreground `guard run`
- state/log files under /tmp to avoid flash churn

.EXAMPLE
.\scripts\install-immortalwrt.ps1

.EXAMPLE
.\scripts\install-immortalwrt.ps1 -Build -HostName immortalwrt

.EXAMPLE
.\scripts\install-immortalwrt.ps1 -SkipConfigUpload
#>
param(
	[string]$HostName = "immortalwrt",
	[string]$User = "root",
	[string]$BinaryPath = ".\dist\njupt-net-linux-arm64",
	[string]$ConfigPath = "",
    [string]$RemoteBinaryPath = "/usr/bin/njupt-net",
    [string]$RemoteConfigPath = "/etc/njupt-net/config.json",
    [string]$StateDir = "/tmp/njupt-net",
    [string]$ServiceName = "njupt-net",
    [string]$RemoteTempDir = "/tmp/njupt-net-deploy",
    [bool]$UseInsecureTLS = $true,
    [switch]$Build,
    [switch]$SkipConfigUpload,
    [switch]$SkipStart,
    [switch]$SkipEnable
)

Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$RepoRoot = Resolve-Path (Join-Path $PSScriptRoot "..")
Set-Location $RepoRoot

function Require-Command {
	param([Parameter(Mandatory = $true)][string]$Name)
	$command = Get-Command $Name -ErrorAction SilentlyContinue
	if (-not $command) {
		throw "Required command not found: $Name"
	}
	return $command.Source
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

function Invoke-SSH {
	param(
		[Parameter(Mandatory = $true)][string]$Target,
		[Parameter(Mandatory = $true)][string]$Command
	)
	& $script:SshExe $Target $Command
	if ($LASTEXITCODE -ne 0) {
		throw "ssh command failed: $Command"
	}
}

function Copy-SCP {
	param(
		[Parameter(Mandatory = $true)][string]$Source,
		[Parameter(Mandatory = $true)][string]$Destination
	)
	& $script:ScpExe $Source $Destination
	if ($LASTEXITCODE -ne 0) {
		throw "scp failed: $Source -> $Destination"
	}
}

function Invoke-RemoteScript {
	param(
		[Parameter(Mandatory = $true)][string]$Target,
		[Parameter(Mandatory = $true)][string]$Script
	)
	$tempScriptPath = Join-Path ([System.IO.Path]::GetTempPath()) "njupt-net-remote-install.sh"
	$remoteTempScript = "/tmp/njupt-net-remote-install.sh"
	try {
		Write-AsciiFile -Path $tempScriptPath -Content $Script
		Copy-SCP -Source $tempScriptPath -Destination "${Target}:$remoteTempScript"
		& $script:SshExe $Target "tr -d '\r' < $remoteTempScript > ${remoteTempScript}.run && sh ${remoteTempScript}.run; status=`$?; rm -f $remoteTempScript ${remoteTempScript}.run; exit `$status"
		if ($LASTEXITCODE -ne 0) {
			throw "remote install script failed"
		}
	}
	finally {
		Remove-Item $tempScriptPath -ErrorAction SilentlyContinue
	}
}

function Write-AsciiFile {
	param(
		[Parameter(Mandatory = $true)][string]$Path,
		[Parameter(Mandatory = $true)][string]$Content
	)
	$encoding = [System.Text.ASCIIEncoding]::new()
	[System.IO.File]::WriteAllText($Path, $Content, $encoding)
}

Write-Host "==> Preparing local environment" -ForegroundColor Cyan
$script:SshExe = Require-Command "ssh"
$script:ScpExe = Require-Command "scp"

if ($Build) {
	Write-Host "==> Building windows/local + linux/arm64 artifacts" -ForegroundColor Cyan
	& (Join-Path $RepoRoot "scripts\build.ps1") -Mode local
	if ($LASTEXITCODE -ne 0) {
		throw "build.ps1 failed"
	}
}

$BinaryFullPath = Resolve-InputPath -Path $BinaryPath -Label "Binary"
$ConfigFullPath = $null
if (-not $SkipConfigUpload) {
	$ConfigFullPath = Resolve-ConfigPath -Path $ConfigPath
}

$Target = "$User@$HostName"
$RemoteBinaryDir = [System.IO.Path]::GetDirectoryName($RemoteBinaryPath).Replace("\", "/")
$RemoteConfigDir = [System.IO.Path]::GetDirectoryName($RemoteConfigPath).Replace("\", "/")
$RemoteInitPath = "/etc/init.d/$ServiceName"
$RemoteTempBinary = "$RemoteTempDir/njupt-net"
$RemoteTempConfig = "$RemoteTempDir/config.json"
$RemoteTempInit = "$RemoteTempDir/$ServiceName.init"
$ServiceCommandLine = '    procd_set_param command "$PROG" --config "$CONFIG" --yes guard run --state-dir "$STATE_DIR"'
if ($UseInsecureTLS) {
	$ServiceCommandLine = '    procd_set_param command "$PROG" --config "$CONFIG" --insecure-tls --yes guard run --state-dir "$STATE_DIR"'
}

Write-Host "==> Probing router architecture" -ForegroundColor Cyan
$arch = & $script:SshExe $Target "uname -m"
if ($LASTEXITCODE -ne 0) {
	throw "Unable to connect to $Target via ssh"
}
$arch = ($arch | Out-String).Trim()
Write-Host "Remote architecture: $arch"
if ($arch -notin @("aarch64", "arm64")) {
	throw "Remote architecture $arch is not arm64/aarch64; the linux-arm64 binary is not suitable"
}

Write-Host "==> Creating remote directories" -ForegroundColor Cyan
Invoke-SSH -Target $Target -Command "mkdir -p $RemoteTempDir $RemoteBinaryDir $RemoteConfigDir $StateDir"

Write-Host "==> Uploading binary" -ForegroundColor Cyan
Copy-SCP -Source $BinaryFullPath -Destination "${Target}:$RemoteTempBinary"

if (-not $SkipConfigUpload) {
	Write-Host "==> Uploading config file" -ForegroundColor Cyan
	Copy-SCP -Source $ConfigFullPath -Destination "${Target}:$RemoteTempConfig"
}

$ServiceScript = @"
#!/bin/sh /etc/rc.common

USE_PROCD=1
START=95
STOP=10

PROG="$RemoteBinaryPath"
CONFIG="$RemoteConfigPath"
STATE_DIR="$StateDir"

start_service() {
    mkdir -p "`$STATE_DIR"
    procd_open_instance
$ServiceCommandLine
    procd_set_param respawn 3600 5 5
    procd_set_param stdout 1
    procd_set_param stderr 1
    procd_close_instance
}
"@

$LocalInitPath = Join-Path ([System.IO.Path]::GetTempPath()) "$ServiceName.init"
Write-AsciiFile -Path $LocalInitPath -Content $ServiceScript

Write-Host "==> Uploading init script" -ForegroundColor Cyan
Copy-SCP -Source $LocalInitPath -Destination "${Target}:$RemoteTempInit"

$RemoteScript = @"
set -eu

mkdir -p $RemoteBinaryDir $StateDir
cp $RemoteTempBinary $RemoteBinaryPath
chmod 0755 $RemoteBinaryPath

if [ -f $RemoteTempConfig ]; then
    mkdir -p $RemoteConfigDir
    cp $RemoteTempConfig $RemoteConfigPath
    chmod 0600 $RemoteConfigPath
fi

tr -d '\r' < $RemoteTempInit > $RemoteInitPath
chmod 0755 $RemoteInitPath

if [ ! -f $RemoteConfigPath ]; then
    echo "Remote config missing: $RemoteConfigPath" >&2
    exit 1
fi

$RemoteBinaryPath --help >/dev/null

if [ "$($SkipEnable.IsPresent.ToString().ToLower())" = "false" ]; then
    $RemoteInitPath enable
fi

if [ "$($SkipStart.IsPresent.ToString().ToLower())" = "false" ]; then
    $RemoteInitPath restart || {
        $RemoteInitPath stop || true
        $RemoteInitPath start
    }
    sleep 4
fi

echo "=== remote status ==="
if [ "$($SkipStart.IsPresent.ToString().ToLower())" = "false" ]; then
    $RemoteInitPath status || true
    $RemoteBinaryPath --config $RemoteConfigPath --output json guard status --state-dir $StateDir || true
fi

rm -f $RemoteTempBinary $RemoteTempInit
if [ -f $RemoteTempConfig ] && [ "$($SkipConfigUpload.IsPresent.ToString().ToLower())" = "false" ]; then
    rm -f $RemoteTempConfig
fi
rmdir $RemoteTempDir 2>/dev/null || true

echo "=== install complete ==="
echo "binary: $RemoteBinaryPath"
echo "config: $RemoteConfigPath"
echo "stateDir: $StateDir"
echo "service: $RemoteInitPath"
"@

Write-Host "==> Installing service on router" -ForegroundColor Cyan
try {
	Invoke-RemoteScript -Target $Target -Script $RemoteScript
}
finally {
	Remove-Item $LocalInitPath -ErrorAction SilentlyContinue
}

Write-Host ""
Write-Host "Deployment complete." -ForegroundColor Green
Write-Host "Useful commands on the router:" -ForegroundColor Cyan
Write-Host "  /etc/init.d/$ServiceName status"
Write-Host "  /etc/init.d/$ServiceName restart"
Write-Host "  /etc/init.d/$ServiceName stop"
Write-Host "  $RemoteBinaryPath --config $RemoteConfigPath --output json guard status --state-dir $StateDir"
Write-Host "  logread -e $ServiceName"
Write-Host "  cat $StateDir/status.json"
