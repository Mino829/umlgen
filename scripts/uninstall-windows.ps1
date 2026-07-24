[CmdletBinding(SupportsShouldProcess = $true, ConfirmImpact = 'Medium')]
param(
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\umlgen'),

    [switch]$SkipPathUpdate
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

$fullInstallDir = [IO.Path]::GetFullPath($InstallDir).TrimEnd('\')
$driveRoot = [IO.Path]::GetPathRoot($fullInstallDir).TrimEnd('\')
$userProfile = [IO.Path]::GetFullPath($env:USERPROFILE).TrimEnd('\')

if ($fullInstallDir.Length -lt 8 -or
    $fullInstallDir -ieq $driveRoot -or
    $fullInstallDir -ieq $userProfile) {
    throw "Refusing to remove unsafe install directory '$fullInstallDir'."
}

if (Test-Path -LiteralPath $fullInstallDir) {
    if ($PSCmdlet.ShouldProcess($fullInstallDir, 'Remove umlgen installation')) {
        Remove-Item -LiteralPath $fullInstallDir -Recurse -Force
    }
}

if (-not $SkipPathUpdate) {
    $currentUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    if (-not [string]::IsNullOrWhiteSpace($currentUserPath)) {
        $remainingEntries = @(
            $currentUserPath.Split(';') |
                Where-Object {
                    -not [string]::IsNullOrWhiteSpace($_) -and
                    $_.TrimEnd('\') -ine $fullInstallDir
                }
        )
        [Environment]::SetEnvironmentVariable('Path', ($remainingEntries -join ';'), 'User')
    }
}

Write-Host "Removed umlgen from: $fullInstallDir"
if (-not $SkipPathUpdate) {
    Write-Host 'Open a new PowerShell window to refresh PATH.'
}
