Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

function Resolve-RepoRoot {
    return (Resolve-Path (Join-Path $PSScriptRoot "..\..")).Path
}

function Resolve-GoExe {
    $candidates = @()
    $repoRoot = Resolve-RepoRoot

    if ($env:SUPERFLARE_GO_BIN) {
        $candidates += $env:SUPERFLARE_GO_BIN
    }

    $candidates += (Join-Path $repoRoot ".tools\go\bin\go.exe")
    $candidates += (Join-Path $repoRoot ".tools\go\bin")

    $workspaceRoot = Split-Path -Parent $repoRoot
    if (-not [string]::IsNullOrWhiteSpace($workspaceRoot)) {
        $candidates += (Join-Path $workspaceRoot ".tools\go\bin\go.exe")
        $candidates += (Join-Path $workspaceRoot ".tools\go\bin")
    }

    foreach ($candidate in $candidates) {
        if ([string]::IsNullOrWhiteSpace($candidate)) {
            continue
        }

        if (Test-Path $candidate -PathType Leaf) {
            return (Resolve-Path $candidate).Path
        }

        $goExe = Join-Path $candidate "go.exe"
        if (Test-Path $goExe -PathType Leaf) {
            return (Resolve-Path $goExe).Path
        }
    }

    $cmd = Get-Command go -ErrorAction SilentlyContinue
    if ($cmd -and $cmd.Source) {
        return $cmd.Source
    }

    throw "go.exe not found. Set SUPERFLARE_GO_BIN, place Go under .tools\\go\\bin beside the repo or workspace, or add go.exe to PATH."
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

function Get-EnableLogin {
    $raw = $env:SUPERFLARE_ENABLE_LOGIN
    if ([string]::IsNullOrWhiteSpace($raw)) {
        while ($true) {
            $choice = Read-Host "Enable login for this run? [Y/n]"
            if ([string]::IsNullOrWhiteSpace($choice)) {
                return $true
            }

            switch ($choice.Trim().ToLowerInvariant()) {
                "y" { return $true }
                "yes" { return $true }
                "n" { return $false }
                "no" { return $false }
                default { Write-Host "Please enter Y or N." }
            }
        }
    }

    switch ($raw.Trim().ToLowerInvariant()) {
        "1" { return $true }
        "true" { return $true }
        "yes" { return $true }
        "on" { return $true }
        "0" { return $false }
        "false" { return $false }
        "no" { return $false }
        "off" { return $false }
        default { throw "Invalid SUPERFLARE_ENABLE_LOGIN value: $raw" }
    }
}

function New-CookieSecret {
    $bytes = [byte[]]::new(32)
    [System.Security.Cryptography.RandomNumberGenerator]::Fill($bytes)
    return ([System.BitConverter]::ToString($bytes) -replace "-", "").ToLowerInvariant()
}

function Invoke-CheckedNative {
    param(
        [string]$FilePath,
        [string[]]$Arguments,
        [string]$StepName
    )

    & $FilePath @Arguments
    if ($LASTEXITCODE -ne 0) {
        throw "$StepName failed with exit code $LASTEXITCODE."
    }
}

function Stop-TrackedProcess {
    param(
        [int]$ProcessId,
        [System.Collections.Generic.HashSet[int]]$Seen,
        [string]$ExpectedPath
    )

    if ($ProcessId -le 0 -or $Seen.Contains($ProcessId)) {
        return
    }

    if (-not (Test-SuperflareProcess -ProcessId $ProcessId -ExpectedPath $ExpectedPath)) {
        Write-Host "Skipped PID $ProcessId because it is not this checkout's superflare.exe."
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

function Test-SuperflareProcess {
    param(
        [int]$ProcessId,
        [string]$ExpectedPath
    )

    if ([string]::IsNullOrWhiteSpace($ExpectedPath)) {
        return $false
    }

    $expected = [System.IO.Path]::GetFullPath($ExpectedPath).ToLowerInvariant()
    try {
        $proc = Get-CimInstance Win32_Process -Filter "ProcessId = $ProcessId" -ErrorAction Stop
        if ($null -eq $proc -or $null -eq $proc.ExecutablePath) {
            return $false
        }
        return ([System.IO.Path]::GetFullPath($proc.ExecutablePath).ToLowerInvariant() -eq $expected)
    } catch {
        return $false
    }
}

function Stop-Superflare {
    param(
        [string]$RepoRoot,
        [int]$Port,
        [string]$PidFile
    )

    $seen = [System.Collections.Generic.HashSet[int]]::new()
    $exePath = Join-Path $RepoRoot "superflare.exe"

    if (Test-Path $PidFile) {
        $rawPid = (Get-Content $PidFile -Raw).Trim()
        $pidValue = 0
        if ([int]::TryParse($rawPid, [ref]$pidValue)) {
            Stop-TrackedProcess -ProcessId $pidValue -Seen $seen -ExpectedPath $exePath
        }
        Remove-Item $PidFile -Force -ErrorAction SilentlyContinue
    }

    $repoProcesses = Get-CimInstance Win32_Process -Filter "Name = 'superflare.exe'" -ErrorAction SilentlyContinue
    foreach ($repoProc in $repoProcesses) {
        if ($null -ne $repoProc.ExecutablePath -and $repoProc.ExecutablePath.ToLowerInvariant() -eq $exePath.ToLowerInvariant()) {
            Stop-TrackedProcess -ProcessId ([int]$repoProc.ProcessId) -Seen $seen -ExpectedPath $exePath
        }
    }

    for ($i = 0; $i -lt 20; $i++) {
        if (-not (Test-TcpPortOpen -Port $Port)) {
            break
        }
        Start-Sleep -Milliseconds 250
    }
}

function Test-TcpPortOpen {
    param([int]$Port)

    $client = [System.Net.Sockets.TcpClient]::new()
    try {
        $async = $client.BeginConnect("127.0.0.1", $Port, $null, $null)
        if (-not $async.AsyncWaitHandle.WaitOne(200)) {
            return $false
        }
        $client.EndConnect($async)
        return $true
    } catch {
        return $false
    } finally {
        $client.Close()
    }
}

function Wait-ForHttpReady {
    param(
        [int]$Port,
        [int]$TimeoutSeconds
    )

    $deadline = (Get-Date).AddSeconds($TimeoutSeconds)
    while ((Get-Date) -lt $deadline) {
        try {
            $resp = Invoke-WebRequest -Uri ("http://127.0.0.1:{0}/help" -f $Port) -UseBasicParsing -TimeoutSec 2
            if ($resp.StatusCode -eq 200) {
                return $true
            }
        } catch {
        }
        Start-Sleep -Milliseconds 500
    }

    return $false
}

$repoRoot = Resolve-RepoRoot
$goExe = Resolve-GoExe
$goBinDir = Split-Path -Parent $goExe
$port = Get-TargetPort
$enableLogin = Get-EnableLogin
$cookieSecret = if ([string]::IsNullOrWhiteSpace($env:SUPERFLARE_COOKIE_SECRET)) { New-CookieSecret } else { $env:SUPERFLARE_COOKIE_SECRET }

if ([string]::IsNullOrWhiteSpace($env:SUPERFLARE_COOKIE_SECRET)) {
    Write-Host "SUPERFLARE_COOKIE_SECRET is not set. A temporary random cookie secret will be used for this run."
}

$env:PATH = "$goBinDir;$env:PATH"

$runDir = Join-Path $repoRoot "var\run"
$pidFile = Join-Path $runDir "superflare.pid"
$stdoutLog = Join-Path $runDir "superflare.stdout.log"
$stderrLog = Join-Path $runDir "superflare.stderr.log"
$exePath = Join-Path $repoRoot "superflare.exe"

if (-not (Test-Path $runDir)) {
    New-Item -ItemType Directory -Path $runDir -Force | Out-Null
}

Write-Host "Repo root: $repoRoot"
Write-Host "Go: $goExe"
Write-Host "Port: $port"
Write-Host ("Login mode: " + ($(if ($enableLogin) { "enabled" } else { "disabled" })))
Write-Host

Stop-Superflare -RepoRoot $repoRoot -Port $port -PidFile $pidFile

Push-Location $repoRoot
try {
    Write-Host "Generating embedded assets..."
    Invoke-CheckedNative -FilePath $goExe -Arguments @("run", ".\build\build.go") -StepName "Embedded asset generation"

    Write-Host "Running tests..."
    Invoke-CheckedNative -FilePath $goExe -Arguments @("test", "./...", "-count=1") -StepName "Test execution"

    Write-Host "Building superflare.exe..."
    Invoke-CheckedNative -FilePath $goExe -Arguments @("build", "-o", ".\superflare.exe", ".") -StepName "Binary build"
} finally {
    Pop-Location
}

foreach ($path in @($stdoutLog, $stderrLog)) {
    if (Test-Path $path) {
        Remove-Item $path -Force -ErrorAction SilentlyContinue
    }
}

$args = @(
    "--port", "$port",
    "--enable_editor",
    "--cookie_secret", $cookieSecret
)

if (-not $enableLogin) {
    $args += "--disable_login"
}

Write-Host "Starting superflare.exe in background..."
$proc = Start-Process -FilePath $exePath `
    -ArgumentList $args `
    -WorkingDirectory $repoRoot `
    -WindowStyle Hidden `
    -RedirectStandardOutput $stdoutLog `
    -RedirectStandardError $stderrLog `
    -PassThru

Set-Content -Path $pidFile -Value $proc.Id -NoNewline

if (-not (Wait-ForHttpReady -Port $port -TimeoutSeconds 30)) {
    Write-Host
    Write-Host "SuperFlare did not become ready within 30 seconds."
    Write-Host "Stdout log: $stdoutLog"
    Write-Host "Stderr log: $stderrLog"
    if (Test-Path $stdoutLog) {
        Write-Host
        Write-Host "--- stdout (tail) ---"
        Get-Content $stdoutLog -Tail 40
    }
    if (Test-Path $stderrLog) {
        Write-Host
        Write-Host "--- stderr (tail) ---"
        Get-Content $stderrLog -Tail 40
    }
    exit 1
}

Write-Host
Write-Host "SuperFlare is ready."
Write-Host "PID: $($proc.Id)"
Write-Host "Home:   http://127.0.0.1:$port/"
Write-Host "Help:   http://127.0.0.1:$port/help"
Write-Host "Editor: http://127.0.0.1:$port/editor"
Write-Host "PID file: $pidFile"
Write-Host "Stdout log: $stdoutLog"
Write-Host "Stderr log: $stderrLog"
if ($enableLogin) {
    Write-Host "Login is enabled. Use credentials from config.yml or .env."
} else {
    Write-Host "Login is disabled for this run."
}
