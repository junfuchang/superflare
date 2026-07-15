Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Resolve-RepoRoot {
    return (Resolve-Path (Join-Path $PSScriptRoot "..")).Path
}

function Resolve-GoExe {
    $repoRoot = Resolve-RepoRoot
    $candidates = @()

    if ($env:SUPERFLARE_GO_BIN) {
        $candidates += $env:SUPERFLARE_GO_BIN
    }

    $candidates += (Join-Path $repoRoot ".tools\go\bin\go.exe")
    $workspaceRoot = Split-Path -Parent $repoRoot
    if (-not [string]::IsNullOrWhiteSpace($workspaceRoot)) {
        $candidates += (Join-Path $workspaceRoot ".tools\go\bin\go.exe")
    }

    foreach ($candidate in $candidates) {
        if ([string]::IsNullOrWhiteSpace($candidate)) {
            continue
        }
        if (Test-Path $candidate -PathType Leaf) {
            return (Resolve-Path $candidate).Path
        }
    }

    $cmd = Get-Command go -ErrorAction SilentlyContinue
    if ($cmd -and $cmd.Source) {
        return $cmd.Source
    }

    throw "go.exe not found. Set SUPERFLARE_GO_BIN, place Go under .tools\\go\\bin, or add go.exe to PATH."
}

function Resolve-FnpackExe {
    $repoRoot = Resolve-RepoRoot
    $candidates = @()

    if ($env:SUPERFLARE_FNPACK_BIN) {
        $candidates += $env:SUPERFLARE_FNPACK_BIN
    }

    $candidates += (Join-Path $repoRoot ".tools\fnpack\fnpack.exe")

    foreach ($candidate in $candidates) {
        if ([string]::IsNullOrWhiteSpace($candidate)) {
            continue
        }
        if (Test-Path $candidate -PathType Leaf) {
            return (Resolve-Path $candidate).Path
        }
    }

    $cmd = Get-Command fnpack -ErrorAction SilentlyContinue
    if ($cmd -and $cmd.Source) {
        return $cmd.Source
    }

    throw "fnpack not found. Set SUPERFLARE_FNPACK_BIN, place fnpack.exe under .tools\\fnpack\\, or add fnpack to PATH."
}

function Ensure-Directory {
    param([string]$Path)

    if (-not (Test-Path $Path)) {
        New-Item -ItemType Directory -Path $Path -Force | Out-Null
    }
}

function Reset-PackageRuntimeDirs {
    param(
        [string]$EtcDir,
        [string]$VarDir
    )

    foreach ($dir in @($EtcDir, $VarDir)) {
        if (Test-Path $dir) {
            Remove-Item -LiteralPath $dir -Recurse -Force
        }
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
    }
}

function Copy-DefaultConfigFiles {
    param(
        [string]$RepoRoot,
        [string]$DefaultsDir
    )

    Ensure-Directory -Path $DefaultsDir
    $sourceRoot = Join-Path $RepoRoot "config\defaults"

    foreach ($name in @("config.yml", "apps.yml", "bookmarks.yml", "ports.yaml")) {
        $source = Join-Path $sourceRoot $name
        $target = Join-Path $DefaultsDir $name
        if (-not (Test-Path $source -PathType Leaf)) {
            throw "Required default template missing: $source"
        }
        Copy-Item -Path $source -Destination $target -Force
    }
}

$repoRoot = Resolve-RepoRoot
$goExe = Resolve-GoExe
$fnpackExe = Resolve-FnpackExe
$packageRoot = Join-Path $repoRoot "fnapp\superflare"
$serverDir = Join-Path $packageRoot "app\server"
$defaultsDir = Join-Path $serverDir "defaults"
$packageEtcDir = Join-Path $packageRoot "etc"
$packageVarDir = Join-Path $packageRoot "var"
$linuxBinary = Join-Path $serverDir "superflare"
$fpkFile = Join-Path $packageRoot "superflare.fpk"

if (-not (Test-Path $packageRoot -PathType Container)) {
    throw "Package directory not found: $packageRoot"
}

Write-Host "Repo root: $repoRoot"
Write-Host "Go: $goExe"
Write-Host "fnpack: $fnpackExe"
Write-Host "Package: $packageRoot"
Write-Host

Push-Location $repoRoot
try {
    Write-Host "Generating embedded assets..."
    & $goExe run .\build\build.go
    if ($LASTEXITCODE -ne 0) {
        throw "Asset build failed with exit code $LASTEXITCODE."
    }

    Write-Host "Running tests..."
    & $goExe test ./... -count=1
    if ($LASTEXITCODE -ne 0) {
        throw "go test failed with exit code $LASTEXITCODE."
    }

    Write-Host "Testing fnapp configuration preservation..."
    & (Join-Path $repoRoot "fnapp\test-lifecycle-config-preservation.ps1")
    if (-not $?) {
        throw "fnapp configuration preservation test failed."
    }

    Write-Host "Syncing package defaults..."
    Copy-DefaultConfigFiles -RepoRoot $repoRoot -DefaultsDir $defaultsDir

    Write-Host "Resetting package runtime directories..."
    Reset-PackageRuntimeDirs -EtcDir $packageEtcDir -VarDir $packageVarDir

    Write-Host "Building Linux binary..."
    $oldCgo = $env:CGO_ENABLED
    $oldGoos = $env:GOOS
    $oldGoarch = $env:GOARCH
    try {
        $env:CGO_ENABLED = "0"
        $env:GOOS = "linux"
        $env:GOARCH = "amd64"
        & $goExe build -o $linuxBinary .
        if ($LASTEXITCODE -ne 0) {
            throw "Linux build failed with exit code $LASTEXITCODE."
        }
    } finally {
        $env:CGO_ENABLED = $oldCgo
        $env:GOOS = $oldGoos
        $env:GOARCH = $oldGoarch
    }
} finally {
    Pop-Location
}

if (-not (Test-Path $linuxBinary -PathType Leaf)) {
    throw "Linux binary was not produced: $linuxBinary"
}

if (Test-Path $fpkFile) {
    Remove-Item $fpkFile -Force
}

Push-Location $packageRoot
try {
    Write-Host "Packing fnOS native app..."
    & $fnpackExe build
    if ($LASTEXITCODE -ne 0) {
        throw "fnpack build failed with exit code $LASTEXITCODE."
    }
} finally {
    Pop-Location
}

if (-not (Test-Path $fpkFile -PathType Leaf)) {
    throw "fnpack did not produce the expected package: $fpkFile"
}

Write-Host
Write-Host "SuperFlare fnOS package is ready:"
Write-Host $fpkFile
