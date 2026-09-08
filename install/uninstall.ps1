[CmdletBinding()]
param(
    [switch]$PurgeLocalState
)

$ErrorActionPreference = "Stop"
$InstallRoot = Join-Path $env:LOCALAPPDATA "Programs\Overgent"
$Binary = Join-Path $InstallRoot "overgent.exe"
$State = Join-Path $env:APPDATA "Overgent"
$BindingsRemoved = $true
$CredentialTargets = @("com.overgent.comice:overgent.local-backend.secrets-key")

$BackendState = Join-Path $State "backend\backend.json"
if ($PurgeLocalState -and (Test-Path -LiteralPath $BackendState -PathType Leaf)) {
    try {
        $InstanceName = [string](Get-Content -LiteralPath $BackendState -Raw | ConvertFrom-Json).instanceName
        if ($InstanceName -cmatch '^overgent-local-[a-f0-9]{12}$') {
            $CredentialTargets += "com.overgent.comice:overgent.local-backend.$InstanceName"
        }
    } catch {
        Write-Warning "Could not read the local backend instance name; its instance credential may need manual removal."
    }
}

if (Test-Path -LiteralPath $Binary -PathType Leaf) {
    & $Binary setup remove-all
    if ($LASTEXITCODE -ne 0) { $BindingsRemoved = $false }
    & $Binary service remove
    if ($LASTEXITCODE -ne 0) { throw "overgent uninstaller: scheduled-task removal failed with exit code $LASTEXITCODE; the executable was preserved" }
    if ($PurgeLocalState -and (Test-Path -LiteralPath (Join-Path $State "backend") -PathType Container)) {
        & $Binary backend stop *> $null
    }
} else {
    & schtasks.exe /End /TN Overgent *> $null
    & schtasks.exe /Delete /TN Overgent /F *> $null
}

Remove-Item -LiteralPath $Binary -Force -ErrorAction SilentlyContinue
Remove-Item -LiteralPath "$Binary.previous" -Force -ErrorAction SilentlyContinue

if ($PurgeLocalState) {
    if (Test-Path -LiteralPath $State -PathType Container) {
        Add-Type -AssemblyName Microsoft.VisualBasic
        [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory(
            $State,
            [Microsoft.VisualBasic.FileIO.UIOption]::OnlyErrorDialogs,
            [Microsoft.VisualBasic.FileIO.RecycleOption]::SendToRecycleBin
        )
        Write-Host "Local Overgent state moved to the Recycle Bin."
    }
    $RemovedCredentials = 0
    foreach ($Target in $CredentialTargets) {
        & cmdkey.exe "/delete:$Target" *> $null
        if ($LASTEXITCODE -eq 0) { $RemovedCredentials++ }
    }
    if ($RemovedCredentials -gt 0) { Write-Host "Removed $RemovedCredentials local-backend Credential Manager item(s)." }
} else {
    Write-Host "Overgent removed. Local state and Credential Manager credentials were preserved."
    Write-Host "Run uninstall.ps1 -PurgeLocalState only if you also want recoverable local-state removal."
}

$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath) {
    $Kept = @($UserPath -split ';' | Where-Object { $_ -and $_ -cne $InstallRoot })
    [Environment]::SetEnvironmentVariable("Path", ($Kept -join ';'), "User")
}
if (Test-Path -LiteralPath $InstallRoot -PathType Container) {
    Remove-Item -LiteralPath $InstallRoot -Force -ErrorAction SilentlyContinue
}
if (-not $BindingsRemoved) {
    Write-Warning "One or more managed agent bindings had drifted and were left untouched for safety. Review them before deleting preserved state."
}
