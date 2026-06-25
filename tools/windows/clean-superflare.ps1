Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Resolve-RepoRoot {
    return (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
}

function Get-TargetPort {
    $raw = $env:SUPERFLARE_PORT
    if ([string]::IsNullOrWhiteSpace($raw)) {
        return 3636
    }

    $parsed = 0
    if (-not [int]::TryParse($raw, [ref]$parsed) -or $parsed -lt 1 -or $parsed -gt 65535) {
        throw "Invalid SUPERFLARE_PORT value: $raw"
    }

    return $parsed
}

function Stop-TrackedProcess {
    param(
        [int]$ProcessId,
        [System.Collections.Generic.HashSet[int]]$Seen
    )

    if ($ProcessId -le 0 -or $Seen.Contains($ProcessId)) {
        return
    }

    try {
        Stop-Process -Id $ProcessId -Force -ErrorAction Stop
        [void]$Seen.Add($ProcessId)
        Write-Host "Stopped PID $ProcessId"
    } catch {
        Write-Host "PID $ProcessId was already gone or could not be stopped."
    }
}

function Remove-Children {
    param([string]$Path)

    if (-not (Test-Path $Path)) {
        return
    }

    Get-ChildItem -Path $Path -Force -ErrorAction SilentlyContinue |
        Remove-Item -Recurse -Force -ErrorAction SilentlyContinue
}

$repoRoot = Resolve-RepoRoot
$port = Get-TargetPort
$runDir = Join-Path $repoRoot "var\run"
$cacheDir = Join-Path $repoRoot "var\cache"
$pidFile = Join-Path $runDir "superflare.pid"
$exePath = Join-Path $repoRoot "superflare.exe"
$tmpFnappUnpackDir = Join-Path $repoRoot "tmp-fnapp-unpack"
$fnappBinary = Join-Path $repoRoot "fnapp\superflare\app\server\superflare"
$fnappPackage = Join-Path $repoRoot "fnapp\superflare\superflare.fpk"

Write-Host "Repo root: $repoRoot"
Write-Host "Port: $port"
Write-Host

$seen = [System.Collections.Generic.HashSet[int]]::new()

if (Test-Path $pidFile) {
    $rawPid = (Get-Content $pidFile -Raw).Trim()
    $pidValue = 0
    if ([int]::TryParse($rawPid, [ref]$pidValue)) {
        Stop-TrackedProcess -ProcessId $pidValue -Seen $seen
    }
    Remove-Item $pidFile -Force -ErrorAction SilentlyContinue
}

try {
    $listenerPids = Get-NetTCPConnection -LocalPort $port -State Listen -ErrorAction Stop |
        Select-Object -ExpandProperty OwningProcess -Unique
    foreach ($listenerPid in $listenerPids) {
        Stop-TrackedProcess -ProcessId ([int]$listenerPid) -Seen $seen
    }
} catch {
}

$repoProcesses = Get-CimInstance Win32_Process -Filter "Name = 'superflare.exe'" -ErrorAction SilentlyContinue
foreach ($repoProc in $repoProcesses) {
    if ($null -ne $repoProc.ExecutablePath -and $repoProc.ExecutablePath.ToLowerInvariant() -eq $exePath.ToLowerInvariant()) {
        Stop-TrackedProcess -ProcessId ([int]$repoProc.ProcessId) -Seen $seen
    }
}

if (Test-Path $exePath) {
    Remove-Item $exePath -Force -ErrorAction SilentlyContinue
    Write-Host "Removed $exePath"
}

foreach ($artifact in @($fnappBinary, $fnappPackage)) {
    if (Test-Path $artifact) {
        Remove-Item $artifact -Force -ErrorAction SilentlyContinue
        Write-Host "Removed $artifact"
    }
}

if (Test-Path $tmpFnappUnpackDir) {
    Remove-Item -LiteralPath $tmpFnappUnpackDir -Recurse -Force -ErrorAction SilentlyContinue
    Write-Host "Removed $tmpFnappUnpackDir"
}

Get-ChildItem -Path $repoRoot -Filter "tmp-superflare*" -Force -ErrorAction SilentlyContinue |
    Remove-Item -Force -Recurse -ErrorAction SilentlyContinue

Remove-Children -Path $runDir
Remove-Children -Path $cacheDir

Write-Host
Write-Host "Cleanup complete."
Write-Host "Preserved runtime data files:"
Write-Host "  config.yml"
Write-Host "  apps.yml"
Write-Host "  bookmarks.yml"
Write-Host "  ports.yaml"
