Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

function Read-RepoFile {
    param([string]$RelativePath)
    return Get-Content -Path (Join-Path $repoRoot $RelativePath) -Raw
}

function Assert-True {
    param(
        [bool]$Condition,
        [string]$Message
    )

    if (-not $Condition) {
        throw $Message
    }
}

function Assert-NotContains {
    param(
        [string]$Text,
        [string]$Needle,
        [string]$Message
    )

    Assert-True -Condition (-not $Text.Contains($Needle)) -Message $Message
}

function Assert-Contains {
    param(
        [string]$Text,
        [string]$Needle,
        [string]$Message
    )

    Assert-True -Condition $Text.Contains($Needle) -Message $Message
}

$linuxRun = Read-RepoFile "tools/linux/run-superflare.sh"
$linuxRestart = Read-RepoFile "tools/linux/restart-superflare.sh"
$linuxClean = Read-RepoFile "tools/linux/clean-superflare.sh"
$linuxInstall = Read-RepoFile "tools/linux/install-systemd.sh"
$linuxReadme = Read-RepoFile "tools/linux/README.md"
$windowsRestart = Read-RepoFile "tools/windows/restart-superflare.ps1"
$windowsClean = Read-RepoFile "tools/windows/clean-superflare.ps1"

foreach ($entry in @(
    @{ Name = "tools/linux/run-superflare.sh"; Text = $linuxRun },
    @{ Name = "tools/linux/restart-superflare.sh"; Text = $linuxRestart },
    @{ Name = "tools/linux/install-systemd.sh"; Text = $linuxInstall },
    @{ Name = "tools/linux/README.md"; Text = $linuxReadme },
    @{ Name = "tools/windows/restart-superflare.ps1"; Text = $windowsRestart }
)) {
    Assert-NotContains -Text $entry.Text -Needle "superflare-local-secret" -Message "$($entry.Name) must not use the old fixed local cookie secret."
    Assert-NotContains -Text $entry.Text -Needle "superflare-production-secret" -Message "$($entry.Name) must not use the old fixed production cookie secret."
}

foreach ($entry in @(
    @{ Name = "tools/linux/run-superflare.sh"; Text = $linuxRun },
    @{ Name = "tools/linux/restart-superflare.sh"; Text = $linuxRestart },
    @{ Name = "tools/linux/install-systemd.sh"; Text = $linuxInstall }
)) {
    Assert-Contains -Text $entry.Text -Needle "generate_cookie_secret" -Message "$($entry.Name) must generate a random cookie secret when none is provided."
}

foreach ($entry in @(
    @{ Name = "tools/linux/restart-superflare.sh"; Text = $linuxRestart },
    @{ Name = "tools/linux/clean-superflare.sh"; Text = $linuxClean }
)) {
    Assert-NotContains -Text $entry.Text -Needle "ss -ltnp" -Message "$($entry.Name) must not kill processes discovered only by a listening port lookup."
    Assert-Contains -Text $entry.Text -Needle "process_matches_superflare" -Message "$($entry.Name) must verify a PID belongs to this checkout before stopping it."
}

foreach ($entry in @(
    @{ Name = "tools/windows/restart-superflare.ps1"; Text = $windowsRestart },
    @{ Name = "tools/windows/clean-superflare.ps1"; Text = $windowsClean }
)) {
    Assert-NotContains -Text $entry.Text -Needle "Get-NetTCPConnection -LocalPort" -Message "$($entry.Name) must not stop processes discovered only by a listening port lookup."
    Assert-Contains -Text $entry.Text -Needle "Test-SuperflareProcess" -Message "$($entry.Name) must verify a PID belongs to this checkout before stopping it."
}

Write-Host "Script safety checks passed."
