[CmdletBinding()]
param(
    [string]$Version = 'latest',

    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA 'Programs\umlgen'),

    # Offline candidate directory containing the native ZIP and SHA256SUMS.txt.
    # Requires an explicit stable or prerelease -Version; never downloads.
    [string]$ArchiveDirectory,

    [switch]$InstallPlantUML,

    [switch]$SkipPathUpdate,

    [string]$GitHubToken = $env:GITHUB_TOKEN
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

$downloadHeaders = @{
    Accept = 'application/vnd.github+json'
    'User-Agent' = 'umlgen-windows-installer'
}
$apiHeaders = $downloadHeaders.Clone()
if (-not [string]::IsNullOrWhiteSpace($GitHubToken)) {
    $apiHeaders.Authorization = "Bearer $GitHubToken"
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

    return (Invoke-RestMethod -Uri $releaseUrl -Headers $apiHeaders)
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
        -Headers $downloadHeaders `
        -OutFile $Destination `
        -UseBasicParsing

    $actualHash = (Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        throw "SHA-256 verification failed for '$($Asset.name)'."
    }
}

function Get-NormalizedVersion {
    param([string]$RequestedVersion)

    $pattern = '\Av?(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)(?:-([0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*))?(?:\+[0-9A-Za-z-]+(?:\.[0-9A-Za-z-]+)*)?\z'
    if ($RequestedVersion -cnotmatch $pattern) {
        throw 'Invalid release version; expected a stable or prerelease SemVer version.'
    }
    $prerelease = $Matches[4]
    if ($prerelease) {
        foreach ($identifier in $prerelease.Split('.')) {
            if ($identifier -cmatch '\A0[0-9]+\z') {
                throw 'Invalid numeric prerelease identifier.'
            }
        }
    }
    return ($RequestedVersion -creplace '\Av', '')
}

function Save-LocalArchive {
    param([string]$Directory, [string]$ArchiveName, [string]$Destination)

    $archivePath = Join-Path $Directory $ArchiveName
    $checksumPath = Join-Path $Directory 'SHA256SUMS.txt'
    if (-not (Test-Path -LiteralPath $archivePath -PathType Leaf) -or
        -not (Test-Path -LiteralPath $checksumPath -PathType Leaf)) {
        throw "The archive directory must contain $ArchiveName and SHA256SUMS.txt."
    }
    # Snapshot both inputs so the checked ZIP is also the one we later read.
    Copy-Item -LiteralPath $archivePath -Destination $Destination
    $checksumSnapshot = Join-Path $tempDir 'SHA256SUMS.txt'
    Copy-Item -LiteralPath $checksumPath -Destination $checksumSnapshot
    $checksumCount = 0
    $expectedHash = ''
    foreach ($line in [IO.File]::ReadAllLines($checksumSnapshot)) {
        if ($line -cmatch '\A(\S+)[ \t]+\*?(\S+)\z') {
            $hash = $Matches[1]
            $name = $Matches[2]
            if ($name -ceq $ArchiveName) {
                $checksumCount++
                if ($hash -cnotmatch '\A[0-9a-fA-F]{64}\z') {
                    throw "Invalid SHA-256 checksum for $ArchiveName."
                }
                $expectedHash = $hash
            }
        }
        elseif ($line.Contains($ArchiveName)) {
            throw "Malformed SHA-256 checksum entry for $ArchiveName."
        }
    }
    if ($checksumCount -ne 1) {
        throw "Expected exactly one SHA-256 checksum for $ArchiveName."
    }
    $actualHash = (Get-FileHash -LiteralPath $Destination -Algorithm SHA256).Hash
    if ($actualHash -ine $expectedHash) {
        throw "SHA-256 verification failed for $ArchiveName."
    }
}

function Export-NativeZip {
    param(
        [string]$Archive,
        [string]$MemberName,
        [string]$DestinationDirectory,
        [switch]$IncludeDlls
    )

    Add-Type -AssemblyName System.IO.Compression.FileSystem
    $zip = [IO.Compression.ZipFile]::OpenRead($Archive)
    try {
        $executables = @($zip.Entries | Where-Object { $_.FullName -ieq $MemberName })
        if ($executables.Count -ne 1 -or $executables[0].FullName -cne $MemberName) {
            throw "Expected exactly one exact archive member '$MemberName'."
        }
        if (-not $IncludeDlls -and $zip.Entries.Count -ne 1) {
            throw 'The umlgen archive must contain only its native executable.'
        }
        $selected = @($executables[0])
        if ($IncludeDlls) {
            # Native PlantUML may ship sibling DLLs. Select only flat DLL
            # basenames, never archive paths or a second executable.
            $selected += @($zip.Entries | Where-Object {
                $_.FullName -cmatch '\A[A-Za-z0-9][A-Za-z0-9_.-]*\.[dD][lL][lL]\z'
            })
        }
        $names = [Collections.Generic.HashSet[string]]::new([StringComparer]::OrdinalIgnoreCase)
        foreach ($entry in $selected) {
            $unixType = ($entry.ExternalAttributes -shr 16) -band 0xF000
            if (-not $names.Add($entry.FullName) -or
                ($unixType -ne 0 -and $unixType -ne 0x8000) -or
                ($entry.ExternalAttributes -band 0x410) -ne 0 -or $entry.Length -eq 0) {
                throw 'The native archive contains a duplicate, link, directory, or empty file.'
            }
        }
        New-Item -ItemType Directory -Path $DestinationDirectory | Out-Null
        foreach ($entry in $selected) {
            $source = $entry.Open()
            try {
                $destination = [IO.File]::Open(
                    (Join-Path $DestinationDirectory $entry.FullName),
                    [IO.FileMode]::CreateNew, [IO.FileAccess]::Write, [IO.FileShare]::None)
                try { $source.CopyTo($destination) }
                finally { $destination.Dispose() }
            }
            finally { $source.Dispose() }
        }
    }
    finally { $zip.Dispose() }
}

if ($PSBoundParameters.ContainsKey('ArchiveDirectory')) {
    if ([string]::IsNullOrWhiteSpace($ArchiveDirectory) -or
        -not (Test-Path -LiteralPath $ArchiveDirectory -PathType Container)) {
        throw 'The archive directory does not exist.'
    }
    if (-not $PSBoundParameters.ContainsKey('Version') -or $Version -eq 'latest') {
        throw '-ArchiveDirectory requires an explicit release -Version.'
    }
    if ($InstallPlantUML) {
        throw '-InstallPlantUML is not available in offline archive mode.'
    }
}
if ($Version -cne 'latest') {
    $requestedNumericVersion = Get-NormalizedVersion -RequestedVersion $Version
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

# PowerShell's location can differ from the process working directory used by
# System.IO; resolve once so staging and atomic replacement use the same path.
$InstallDir = $ExecutionContext.SessionState.Path.GetUnresolvedProviderPathFromPSPath($InstallDir)
$tempRoot = [IO.Path]::GetTempPath()
$tempDir = Join-Path $tempRoot ("umlgen-install-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempDir -Force | Out-Null
$stagedInstall = $null

try {
    $archiveName = 'umlgen-windows-amd64.zip'
    $umlgenArchive = Join-Path $tempDir $archiveName
    if ($PSBoundParameters.ContainsKey('ArchiveDirectory')) {
        Write-Host 'Installing local umlgen...'
        $numericVersion = $requestedNumericVersion
        Save-LocalArchive -Directory $ArchiveDirectory -ArchiveName $archiveName -Destination $umlgenArchive
    }
    else {
        Write-Host 'Downloading umlgen...'
        $umlgenRelease = Get-GitHubRelease -Repository 'Mino829/umlgen' -RequestedVersion $Version
        $numericVersion = Get-NormalizedVersion -RequestedVersion $umlgenRelease.tag_name
        if ($Version -cne 'latest' -and $numericVersion -cne $requestedNumericVersion) {
            throw 'The release version does not match the requested version.'
        }
        $umlgenAsset = Get-ReleaseAsset -Release $umlgenRelease -NamePattern $archiveName
        if ($umlgenAsset.name -cne $archiveName) {
            throw 'The release asset name does not exactly match the native archive name.'
        }
        Save-VerifiedAsset -Asset $umlgenAsset -Destination $umlgenArchive
    }

    $umlgenExtractDir = Join-Path $tempDir 'umlgen'
    Export-NativeZip -Archive $umlgenArchive -MemberName 'umlgen-windows-amd64.exe' `
        -DestinationDirectory $umlgenExtractDir
    $umlgenSource = Join-Path $umlgenExtractDir 'umlgen-windows-amd64.exe'
    $versionOutput = @(& $umlgenSource version)
    if ($LASTEXITCODE -ne 0 -or $versionOutput.Count -ne 1 -or
        $versionOutput[0] -cne "umlgen version $numericVersion") {
        throw 'The archive executable version does not match the requested release.'
    }

    if ($InstallPlantUML) {
        Write-Host 'Downloading PlantUML native renderer...'
        $plantUMLRelease = Get-GitHubRelease -Repository 'plantuml/plantuml' -RequestedVersion 'latest'
        $plantUMLAsset = Get-ReleaseAsset `
            -Release $plantUMLRelease `
            -NamePattern 'native-plantuml-windows-amd64-*.zip'
        $plantUMLArchive = Join-Path $tempDir 'plantuml.zip'
        Save-VerifiedAsset -Asset $plantUMLAsset -Destination $plantUMLArchive

        $plantUMLExtractDir = Join-Path $tempDir 'plantuml'
        Export-NativeZip -Archive $plantUMLArchive -MemberName 'plantuml.exe' `
            -DestinationDirectory $plantUMLExtractDir -IncludeDlls
    }

    # Stage on the destination filesystem. Replace is atomic for an existing
    # executable, so validation/copy errors leave the previous install intact.
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $target = Join-Path $InstallDir 'umlgen.exe'
    if (Test-Path -LiteralPath $target) {
        $targetInfo = Get-Item -LiteralPath $target -Force
        if (($targetInfo.Attributes -band ([IO.FileAttributes]::Directory -bor [IO.FileAttributes]::ReparsePoint)) -ne 0) {
            throw 'The install target must be a regular file.'
        }
    }
    $stagedInstall = Join-Path $InstallDir ('.umlgen-install-' + [Guid]::NewGuid().ToString('N') + '.tmp')
    Copy-Item -LiteralPath $umlgenSource -Destination $stagedInstall
    if ([IO.File]::Exists($target)) {
        [IO.File]::Replace($stagedInstall, $target, $null)
    }
    else {
        [IO.File]::Move($stagedInstall, $target)
    }
    $stagedInstall = $null

    if ($InstallPlantUML) {
        Get-ChildItem -LiteralPath $plantUMLExtractDir -File |
            Copy-Item -Destination $InstallDir -Force
    }

    if (-not $SkipPathUpdate) {
        Add-UserPathEntry -Directory $InstallDir
    }

    if (-not $SkipPathUpdate) {
        $env:Path = "$InstallDir;$env:Path"
    }
    Write-Host ''
    Write-Host "Installed umlgen v$numericVersion in:"
    Write-Host "  $InstallDir"
    if ($InstallPlantUML) {
        Write-Host 'PlantUML SVG rendering is also installed.'
    }
    if (-not $SkipPathUpdate) {
        Write-Host 'Open a new PowerShell window, then run: umlgen version'
    }
}
finally {
    if ($stagedInstall -and (Test-Path -LiteralPath $stagedInstall)) {
        Remove-Item -LiteralPath $stagedInstall -Force -ErrorAction SilentlyContinue
    }
    $fullTempDir = [IO.Path]::GetFullPath($tempDir)
    $fullTempRoot = [IO.Path]::GetFullPath($tempRoot)
    if ($fullTempDir.StartsWith($fullTempRoot, [StringComparison]::OrdinalIgnoreCase) -and
        (Split-Path $fullTempDir -Leaf) -like 'umlgen-install-*') {
        Remove-Item -LiteralPath $fullTempDir -Recurse -Force -ErrorAction SilentlyContinue
    }
}
