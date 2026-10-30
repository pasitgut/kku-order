param(
    [Parameter(Mandatory = $true)]
    [string]$AccountEmail,
    [string]$CredentialsPath = 'secrets/google-drive-oauth.json'
)

$ErrorActionPreference = 'Stop'
if (-not (Get-Command gcloud -ErrorAction SilentlyContinue)) {
    throw 'Install Google Cloud CLI first: https://cloud.google.com/sdk/docs/install.'
}
if ([string]::IsNullOrWhiteSpace($env:APPDATA)) {
    throw 'Run this setup script on Windows so it can preserve your existing ADC file.'
}
if ($AccountEmail -notmatch '^[^\s@]+@[^\s@]+\.[^\s@]+$') {
    throw 'AccountEmail must be a valid email address.'
}

$workspacePath = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$credentialTarget = if ([IO.Path]::IsPathRooted($CredentialsPath)) {
    [IO.Path]::GetFullPath($CredentialsPath)
} else {
    [IO.Path]::GetFullPath((Join-Path $workspacePath $CredentialsPath))
}
if (-not $credentialTarget.StartsWith($workspacePath + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'CredentialsPath must stay inside this project.'
}
if (Test-Path -LiteralPath $credentialTarget) {
    throw 'Credentials file already exists. Move or remove it before creating a replacement; credentials are never overwritten.'
}

$adcDirectory = Join-Path $env:APPDATA 'gcloud'
$adcPath = Join-Path $adcDirectory 'application_default_credentials.json'
$hadExistingAdc = Test-Path -LiteralPath $adcPath -PathType Leaf
$adcBackupPath = $null
if ($hadExistingAdc) {
    $adcBackupPath = Join-Path ([IO.Path]::GetTempPath()) "docflow-adc-backup-$PID-$([guid]::NewGuid().ToString('N')).json"
    Copy-Item -LiteralPath $adcPath -Destination $adcBackupPath
}

$credentialCreated = $false
try {
    Write-Host "A browser window will open. Sign in as $AccountEmail and grant Google Cloud CLI access to Drive."
    & gcloud auth login $AccountEmail --enable-gdrive-access --update-adc --no-activate --force
    if ($LASTEXITCODE -ne 0) { throw 'Google account authorization did not complete.' }

    $adc = Get-Content -LiteralPath $adcPath -Raw | ConvertFrom-Json
    if ($adc.type -ne 'authorized_user' -or -not $adc.client_id -or -not $adc.client_secret -or -not $adc.refresh_token) {
        throw 'Google Cloud CLI did not create a complete authorized_user credential.'
    }

    $credentialDirectory = Split-Path -Parent $credentialTarget
    New-Item -ItemType Directory -Path $credentialDirectory -Force | Out-Null
    Copy-Item -LiteralPath $adcPath -Destination $credentialTarget

    $currentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    & icacls $credentialTarget /inheritance:r /grant:r "${currentIdentity}:(R,W)" '*S-1-5-18:(F)' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'OAuth credentials were created, but file permissions could not be restricted. Review the credential file permissions.' }
    $credentialCreated = $true
} finally {
    if ($hadExistingAdc) {
        Copy-Item -LiteralPath $adcBackupPath -Destination $adcPath -Force
        Remove-Item -LiteralPath $adcBackupPath -Force
    } elseif (Test-Path -LiteralPath $adcPath) {
        Remove-Item -LiteralPath $adcPath -Force
    }
    if (-not $credentialCreated -and (Test-Path -LiteralPath $credentialTarget)) {
        Remove-Item -LiteralPath $credentialTarget -Force
    }
}

Write-Host "Google Drive credentials saved to: $credentialTarget"
Write-Host "Set GOOGLE_DRIVE_ACCOUNT_EMAIL=$AccountEmail in the backend environment."
Write-Host "Share the target Google Drive folder with $AccountEmail as Editor so the importer can rename files."
Write-Host 'Mount this file as a backend secret, restart the backend, then open Settings > Google Drive.'
Write-Host 'Test access, select Google Drive, save the folder link and daily time, then start the first import.'
