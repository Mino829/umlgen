# Run on a disposable Windows runner with Scoop installed; never replace a user's installation.
param([string] $RenderOutputDirectory)

$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
if (-not (Get-Command scoop -ErrorAction SilentlyContinue)) {
    throw 'Install Scoop before running this smoke test (https://scoop.sh).'
}
if (Get-Command umlgen -ErrorAction SilentlyContinue) {
    throw 'Refusing to replace an existing umlgen command.'
}
$scoopRoot = if ($env:SCOOP) { $env:SCOOP } else { Join-Path $HOME 'scoop' }
if (Test-Path (Join-Path $scoopRoot 'buckets/umlgen')) {
    throw 'Refusing to replace an existing umlgen bucket.'
}
function Invoke-Scoop {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]] $Arguments)
    $global:LASTEXITCODE = 0
    & scoop @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Scoop failed: $Arguments" }
}
function Invoke-Git {
    param([Parameter(ValueFromRemainingArguments = $true)][string[]] $Arguments)
    & git @Arguments
    if ($LASTEXITCODE -ne 0) { throw "Git failed: $Arguments" }
}
function Test-Package([string] $Version, [string] $Name) {
    $binary = (Get-Command umlgen -ErrorAction Stop).Source
    python "$root/scripts/smoke-package-manager.py" --binary $binary --version $Version --work-dir (Join-Path $work $Name)
    if ($LASTEXITCODE -ne 0) { throw "Installed package smoke failed: $Version" }
}
$tempRoot = if ($env:RUNNER_TEMP) { $env:RUNNER_TEMP } else { [IO.Path]::GetTempPath() }
$work = Join-Path $tempRoot "umlgen-scoop-$([Guid]::NewGuid().ToString('N'))"
$tapRepo = Join-Path $work 'bucket-repo'
$previousPlantUMLJar = $env:PLANTUML_JAR
try {
    python "$root/scripts/update-package-managers.py" --version v0.6.0 --output-dir $tapRepo
    if ($LASTEXITCODE -ne 0) { throw 'Could not generate the published baseline.' }
    Invoke-Git -C $tapRepo init --quiet
    Invoke-Git -C $tapRepo add bucket/umlgen.json
    Invoke-Git -C $tapRepo -c user.name=PackageSmoke -c user.email=smoke@example.invalid -c commit.gpgsign=false commit --quiet -m 'Published v0.6.0 baseline'
    # Scoop validates Git URLs; a Windows drive path is not accepted as a bucket URL.
    Invoke-Scoop bucket add umlgen ([Uri]$tapRepo).AbsoluteUri
    Invoke-Scoop install umlgen/umlgen
    Test-Package '0.6.0' 'baseline'

    Copy-Item "$root/bucket/umlgen.json" "$tapRepo/bucket/umlgen.json" -Force
    Invoke-Git -C $tapRepo add bucket/umlgen.json
    & git -C $tapRepo diff --cached --quiet
    if ($LASTEXITCODE -eq 1) {
        Invoke-Git -C $tapRepo -c user.name=PackageSmoke -c user.email=smoke@example.invalid -c commit.gpgsign=false commit --quiet -m 'Package definition under test'
    } elseif ($LASTEXITCODE -ne 0) {
        throw 'Could not inspect the package update.'
    }
    Invoke-Scoop update
    Invoke-Scoop update umlgen
    $version = (Get-Content "$root/bucket/umlgen.json" -Raw | ConvertFrom-Json).version
    if ($version -eq '0.6.0') {
        # No newer release on initial adoption; exercise same-version replacement too.
        Invoke-Scoop update umlgen --force
    }
    Test-Package $version 'current'
    if ($RenderOutputDirectory) {
        # Scoop's batch shim cannot be launched directly by the CLI; use its JAR.
        $env:PLANTUML_JAR = Join-Path (Invoke-Scoop prefix plantuml) 'plantuml.jar'
        if (-not (Test-Path -LiteralPath $env:PLANTUML_JAR -PathType Leaf)) {
            throw 'Scoop PlantUML JAR was not found.'
        }
        $binary = (Get-Command umlgen -ErrorAction Stop).Source
        python "$root/scripts/smoke-package-manager.py" --binary $binary --version $version --work-dir $RenderOutputDirectory --render
        if ($LASTEXITCODE -ne 0) { throw 'Scoop SVG/PNG rendering smoke failed.' }
    }
    Invoke-Scoop uninstall umlgen
    if (Get-Command umlgen -ErrorAction SilentlyContinue) {
        throw 'Scoop uninstall left the umlgen command on PATH.'
    }
    if (Test-Path (Join-Path $scoopRoot 'apps/umlgen')) {
        throw 'Scoop uninstall left the application directory.'
    }
    Invoke-Scoop bucket rm umlgen
} finally {
    if ($RenderOutputDirectory) { $env:PLANTUML_JAR = $previousPlantUMLJar }
    if (Test-Path (Join-Path $scoopRoot 'apps/umlgen')) { & scoop uninstall umlgen }
    if (Test-Path (Join-Path $scoopRoot 'buckets/umlgen')) { & scoop bucket rm umlgen }
    if (Test-Path $work) { Remove-Item $work -Recurse -Force }
}
