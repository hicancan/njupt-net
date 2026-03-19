param(
	[ValidateSet("local", "all")]
	[string]$Mode = "local"
)

$ErrorActionPreference = "Stop"

Set-Location (Join-Path $PSScriptRoot "..")

New-Item -ItemType Directory -Force -Path "dist" | Out-Null

function Resolve-GoExe {
	$command = Get-Command go -ErrorAction SilentlyContinue
	if ($command) {
		return $command.Source
	}

	$candidates = @(
		"$(Join-Path $env:USERPROFILE '.version-fox\sdks\golang\bin\go.exe')",
		"$(Join-Path $env:ProgramFiles 'Go\bin\go.exe')",
		"$(Join-Path $env:LOCALAPPDATA 'Programs\Go\bin\go.exe')"
	)
	foreach ($candidate in $candidates) {
		if ($candidate -and (Test-Path $candidate)) {
			return $candidate
		}
	}

	throw "go executable not found in PATH or known install locations"
}

$GoExe = Resolve-GoExe

function Build-Target {
	param(
		[Parameter(Mandatory = $true)][string]$GoOS,
		[Parameter(Mandatory = $true)][string]$GoArch,
		[string]$Extension = ""
	)

	$out = "dist/njupt-net-$GoOS-$GoArch$Extension"
	Write-Host "Building $GoOS/$GoArch -> $out"

	$env:CGO_ENABLED = "0"
	$env:GOOS = $GoOS
	$env:GOARCH = $GoArch
	& $GoExe build -trimpath -ldflags "-s -w" -o $out ./cmd/njupt-net
}

try {
	if ($Mode -eq "local") {
		Build-Target -GoOS "windows" -GoArch "amd64" -Extension ".exe"
		Build-Target -GoOS "linux" -GoArch "arm64"
	}
	else {
		Build-Target -GoOS "windows" -GoArch "amd64" -Extension ".exe"
		Build-Target -GoOS "linux" -GoArch "amd64"
		Build-Target -GoOS "linux" -GoArch "arm64"
		Build-Target -GoOS "darwin" -GoArch "arm64"
	}

	Write-Host "Done. Artifacts are in ./dist"
}
finally {
	Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
	Remove-Item Env:GOOS -ErrorAction SilentlyContinue
	Remove-Item Env:GOARCH -ErrorAction SilentlyContinue
}
