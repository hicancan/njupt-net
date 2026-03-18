param(
    [string]$ExePath = "./dist/njupt-net-windows-amd64.exe",
    [string]$CredentialsPath = "./credentials.json"
)

$ErrorActionPreference = "Stop"
Set-Location (Join-Path $PSScriptRoot "..\..")

if (-not (Test-Path $ExePath)) {
    throw "Executable not found: $ExePath"
}
if (-not (Test-Path $CredentialsPath)) {
    throw "Credentials file not found: $CredentialsPath"
}

$exe = (Resolve-Path $ExePath).Path
$cred = Get-Content $CredentialsPath -Raw | ConvertFrom-Json

$accounts = $cred.accounts.PSObject.Properties
if (-not $accounts -or $accounts.Count -eq 0) {
    throw "No accounts found in credentials.json"
}

$results = @()

foreach ($entry in $accounts) {
    $alias = $entry.Name
    $username = $entry.Value.username
    $password = $entry.Value.password

    Write-Host "[RUN] checking binding for account alias=$alias user=$username" -ForegroundColor Yellow

    $output = & $exe --output json service binding get --username $username --password $password 2>&1
    if ($LASTEXITCODE -ne 0) {
        $results += [PSCustomObject]@{
            alias = $alias
            username = $username
            ok = $false
            error = ($output | Out-String).Trim()
        }
        continue
    }

    $raw = ($output | Out-String).Trim()
    try {
        $binding = $raw | ConvertFrom-Json
        $results += [PSCustomObject]@{
            alias = $alias
            username = $username
            ok = $true
            telecomAccount = $binding.data.telecomAccount
            telecomPassword = $binding.data.telecomPassword
            mobileAccount = $binding.data.mobileAccount
            mobilePassword = $binding.data.mobilePassword
        }
    }
    catch {
        $results += [PSCustomObject]@{
            alias = $alias
            username = $username
            ok = $false
            error = "invalid json output: $raw"
        }
    }
}

Write-Host ""
Write-Host "=== Binding Status Summary ===" -ForegroundColor Cyan
$results | ConvertTo-Json -Depth 5

if ($results.Where({ -not $_.ok }).Count -gt 0) {
    exit 1
}

exit 0
