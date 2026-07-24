[CmdletBinding()]
param(
    [ValidatePattern('^(latest|v?\d+\.\d+\.\d+)$')]
    [string]$Version = 'latest',

    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\umlgen'),

    [switch]$InstallPlantUML,

    [switch]$SkipPathUpdate
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$headers = @{
    Accept = 'application/vnd.github+json'
    'User-Agent' = 'umlgen-windows-installer'
}

function Get-GitHubRelease {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Repository,

        [Parameter(Mandatory = $true)]
        [string]$RequestedVersion
    )

    if ($RequestedVersion -eq 'latest') {
        $releaseUrl = "https://api.github.com/repos/$Repository/releases/latest"
    }
    else {
        $tag = $RequestedVersion
        if (-not $tag.StartsWith('v')) {
            $tag = "v$tag"
        }
        $releaseUrl = "https://api.github.com/repos/$Repository/releases/tags/$tag"
    }

    return (Invoke-RestMethod -Uri $releaseUrl -Headers $headers)
}

function Get-ReleaseAsset {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Release,

        [Parameter(Mandatory = $true)]
        [string]$NamePattern
    )

    $matchingAssets = @($Release.assets | Where-Object { $_.name -like $NamePattern })
    if ($matchingAssets.Count -ne 1) {
        throw "Expected one release asset matching '$NamePattern', found $($matchingAssets.Count)."
    }

    return $matchingAssets[0]
}

function Save-VerifiedAsset {
    param(
        [Parameter(Mandatory = $true)]
        [object]$Asset,

        [Parameter(Mandatory = $true)]
        [string]$Destination
    )

    if (-not ($Asset.PSObject.Properties.Name -contains 'digest')) {
        throw "The release asset '$($Asset.name)' does not provide a digest."
    }

    $digest = [string]$Asset.digest
    if ($digest -notmatch '^sha256:([0-9a-fA-F]{64})$') {
        throw "The release asset '$($Asset.name)' does not provide a valid SHA-256 digest."
    }
    $expectedHash = $Matches[1].ToLowerInvariant()

    Invoke-WebRequest `
        -Uri $Asset.browser_download_url `
        -Headers $headers `
        -OutFile $Destination `
        -UseBasicParsing

    $actualHash = (Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "SHA-256 verification failed for '$($Asset.name)'."
    }
}

function Add-UserPathEntry {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Directory
    )

    $currentUserPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = @()
    if (-not [string]::IsNullOrWhiteSpace($currentUserPath)) {
        $entries = @($currentUserPath.Split(';') | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    }

    $alreadyPresent = $false
    foreach ($entry in $entries) {
        if ($entry.TrimEnd('\') -ieq $Directory.TrimEnd('\')) {
            $alreadyPresent = $true
            break
        }
    }

    if (-not $alreadyPresent) {
        $newUserPath = (@($entries) + $Directory) -join ';'
        [Environment]::SetEnvironmentVariable('Path', $newUserPath, 'User')
    }
}

$tempRoot = [IO.Path]::GetTempPath()
$tempDir = Join-Path $tempRoot ("umlgen-install-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null

try {
    Write-Host 'Downloading umlgen...'
    $umlgenRelease = Get-GitHubRelease -Repository 'Mino829/umlgen' -RequestedVersion $Version
    $umlgenAsset = Get-ReleaseAsset -Release $umlgenRelease -NamePattern 'umlgen-windows-amd64.zip'
    $umlgenArchive = Join-Path $tempDir $umlgenAsset.name
    Save-VerifiedAsset -Asset $umlgenAsset -Destination $umlgenArchive

    $umlgenExtractDir = Join-Path $tempDir 'umlgen'
    Expand-Archive -LiteralPath $umlgenArchive -DestinationPath $umlgenExtractDir -Force
    $umlgenSource = Join-Path $umlgenExtractDir 'umlgen-windows-amd64.exe'
    if (-not (Test-Path -LiteralPath $umlgenSource -PathType Leaf)) {
        throw 'The umlgen executable was not found in the downloaded archive.'
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -LiteralPath $umlgenSource -Destination (Join-Path $InstallDir 'umlgen.exe') -Force

    if ($InstallPlantUML) {
        Write-Host 'Downloading PlantUML native renderer...'
        $plantUMLRelease = Get-GitHubRelease -Repository 'plantuml/plantuml' -RequestedVersion 'latest'
        $plantUMLAsset = Get-ReleaseAsset `
            -Release $plantUMLRelease `
            -NamePattern 'native-plantuml-windows-amd64-*.zip'
        $plantUMLArchive = Join-Path $tempDir $plantUMLAsset.name
        Save-VerifiedAsset -Asset $plantUMLAsset -Destination $plantUMLArchive

        $plantUMLExtractDir = Join-Path $tempDir 'plantuml'
        Expand-Archive -LiteralPath $plantUMLArchive -DestinationPath $plantUMLExtractDir -Force
        $plantUMLExecutable = Join-Path $plantUMLExtractDir 'plantuml.exe'
        if (-not (Test-Path -LiteralPath $plantUMLExecutable -PathType Leaf)) {
            throw 'The PlantUML executable was not found in the downloaded archive.'
        }

        Get-ChildItem -LiteralPath $plantUMLExtractDir -File |
            Copy-Item -Destination $InstallDir -Force
    }

    if (-not $SkipPathUpdate) {
        Add-UserPathEntry -Directory $InstallDir
    }

    $env:Path = "$InstallDir;$env:Path"
    Write-Host ''
    Write-Host "Installed umlgen $($umlgenRelease.tag_name) in:"
    Write-Host "  $InstallDir"
    if ($InstallPlantUML) {
        Write-Host 'PlantUML SVG rendering is also installed.'
    }
    if (-not $SkipPathUpdate) {
        Write-Host 'Open a new PowerShell window, then run: umlgen version'
    }
}
finally {
    $fullTempDir = [IO.Path]::GetFullPath($tempDir)
    $fullTempRoot = [IO.Path]::GetFullPath($tempRoot)
    if ($fullTempDir.StartsWith($fullTempRoot, [StringComparison]::OrdinalIgnoreCase) -and
        (Split-Path $fullTempDir -Leaf) -like 'umlgen-install-*') {
        Remove-Item -LiteralPath $fullTempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
