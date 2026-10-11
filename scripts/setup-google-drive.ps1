param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^[a-z][a-z0-9-]{4,61}[a-z0-9]$')]
    [string]$ProjectId,
    [ValidatePattern('^[a-z][a-z0-9-]{4,28}[a-z0-9]$')]
    [string]$AccountId = 'docflow-drive-reader',
    [string]$CredentialsPath = 'secrets/google-drive-service-account.json'
)

$ErrorActionPreference = 'Stop'
if (-not (Get-Command gcloud -ErrorAction SilentlyContinue)) {
    throw 'Install Google Cloud CLI first: https://cloud.google.com/sdk/docs/install and run gcloud auth login.'
}

$workspacePath = [IO.Path]::GetFullPath((Join-Path $PSScriptRoot '..'))
$credentialTarget = [IO.Path]::GetFullPath((Join-Path $workspacePath $CredentialsPath))
if (-not $credentialTarget.StartsWith($workspacePath + [IO.Path]::DirectorySeparatorChar, [StringComparison]::OrdinalIgnoreCase)) {
    throw 'CredentialsPath must stay inside this project.'
}
if (Test-Path -LiteralPath $credentialTarget) {
    throw 'Credentials file already exists. Reuse it or choose a different CredentialsPath; keys are never overwritten.'
}

# Google issues the account email. The deployment uses only Drive readonly scope;
# the account needs no IAM project role to read a folder shared directly to it.
& gcloud services enable drive.googleapis.com iam.googleapis.com --project=$ProjectId --quiet
if ($LASTEXITCODE -ne 0) { throw 'Could not enable APIs. Check the project and your Google Cloud permissions.' }
$accountEmail = "$AccountId@$ProjectId.iam.gserviceaccount.com"
$existingAccount = & gcloud iam service-accounts list --project=$ProjectId --filter="email=$accountEmail" --format='value(email)'
if ($LASTEXITCODE -ne 0) { throw 'Could not look up service accounts in this project.' }
if (-not $existingAccount) {
    & gcloud iam service-accounts create $AccountId --project=$ProjectId --display-name='DocFlow Google Drive reader' --quiet
    if ($LASTEXITCODE -ne 0) { throw 'Could not create the service account.' }
}

$credentialDirectory = Split-Path -Parent $credentialTarget
New-Item -ItemType Directory -Path $credentialDirectory -Force | Out-Null
& gcloud iam service-accounts keys create $credentialTarget --iam-account=$accountEmail --project=$ProjectId --quiet
if ($LASTEXITCODE -ne 0) { throw 'Could not create the key. Check IAM permissions and the organization policy for service account keys.' }

if ($env:OS -eq 'Windows_NT') {
    $currentIdentity = [Security.Principal.WindowsIdentity]::GetCurrent().Name
    & icacls $credentialTarget /inheritance:r /grant:r "${currentIdentity}:(R,W)" '*S-1-5-18:(F)' | Out-Null
    if ($LASTEXITCODE -ne 0) { throw 'Key was created, but file access permissions could not be restricted. Review the key file permissions.' }
}

Write-Host "Service account email: $accountEmail"
Write-Host "Credentials saved to: $credentialTarget"
Write-Host 'Share the target Google Drive folder with this email as Viewer (allow access to subfolders).'
Write-Host 'Mount this file as a backend secret, restart the backend, then open Settings > Google Drive.'
Write-Host 'Test access, select Google Drive, save the folder link and daily time, then start the first import.'
