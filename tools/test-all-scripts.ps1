Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot "..")).Path

function Resolve-GitBash {
    $candidates = @(
        "C:\Program Files\Git\bin\bash.exe",
        "C:\Program Files\Git\usr\bin\bash.exe"
    )
    foreach ($candidate in $candidates) {
        if (Test-Path $candidate -PathType Leaf) {
            return (Resolve-Path $candidate).Path
        }
    }
    $cmd = Get-Command bash -ErrorAction SilentlyContinue
    if ($cmd -and $cmd.Source) {
        return $cmd.Source
    }
    throw "bash was not found. Install Git for Windows or provide bash in PATH."
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

function Test-PowerShellScripts {
    $scripts = Get-ChildItem -Path $repoRoot -Recurse -File -Filter "*.ps1" |
        Where-Object { $_.FullName -notmatch "\\.git\\" }
    foreach ($script in $scripts) {
        $tokens = $null
        $errors = $null
        [System.Management.Automation.Language.Parser]::ParseFile($script.FullName, [ref]$tokens, [ref]$errors) | Out-Null
        if ($errors.Count -gt 0) {
            $detail = ($errors | ForEach-Object { "$($_.Extent.StartLineNumber):$($_.Extent.StartColumnNumber) $($_.Message)" }) -join "; "
            throw "PowerShell parse failed for $($script.FullName): $detail"
        }
    }
    Write-Host "PowerShell parse checks passed: $($scripts.Count)"
}

function Test-ShellScripts {
    $bash = Resolve-GitBash
    $scripts = @(
        "tools/linux/run-superflare.sh",
        "tools/linux/restart-superflare.sh",
        "tools/linux/install-systemd.sh",
        "tools/linux/clean-superflare.sh",
        "tools/linux/build-superflare.sh",
        "tools/docker/docker-entrypoint.sh",
        "tools/docker/build-image.sh",
        "fnapp/superflare/cmd/common.sh",
        "fnapp/superflare/cmd/config_callback",
        "fnapp/superflare/cmd/config_init",
        "fnapp/superflare/cmd/install_callback",
        "fnapp/superflare/cmd/upgrade_callback",
        "fnapp/superflare/cmd/uninstall_callback",
        "fnapp/superflare/cmd/main",
        "fnapp/superflare/cmd/install_init",
        "fnapp/superflare/cmd/upgrade_init",
        "fnapp/superflare/cmd/uninstall_init"
    )
    foreach ($relative in $scripts) {
        $full = Join-Path $repoRoot $relative
        Assert-True -Condition (Test-Path $full -PathType Leaf) -Message "Missing shell script: $relative"
        $output = & $bash -n $full 2>&1
        if ($LASTEXITCODE -ne 0) {
            throw "bash -n failed for ${relative}: $($output -join "`n")"
        }
    }
    Write-Host "Shell syntax checks passed: $($scripts.Count)"
}

function Test-ShellScriptTextFormat {
    $scripts = @(
        Get-ChildItem -Path (Join-Path $repoRoot "tools/linux") -File -Filter "*.sh"
        Get-ChildItem -Path (Join-Path $repoRoot "tools/docker") -File -Filter "*.sh"
        Get-ChildItem -Path (Join-Path $repoRoot "fnapp/superflare/cmd") -File
    ) | Where-Object { $_.Name -ne ".gitkeep" }

    foreach ($script in $scripts) {
        $relative = $script.FullName.Substring($repoRoot.Length + 1).Replace("\", "/")
        $bytes = [System.IO.File]::ReadAllBytes($script.FullName)

        if ($bytes.Length -ge 2 -and -not ($bytes[0] -eq 0x23 -and $bytes[1] -eq 0x21)) {
            throw "$relative must start with a shebang."
        }

        for ($i = 0; $i -lt $bytes.Length - 1; $i++) {
            if ($bytes[$i] -eq 0x0D -and $bytes[$i + 1] -eq 0x0A) {
                throw "$relative must use LF line endings, not CRLF."
            }
        }

        if ($bytes.Length -gt 0 -and $bytes[$bytes.Length - 1] -ne 0x0A) {
            throw "$relative must end with a newline."
        }
    }

    Write-Host "Shell text format checks passed: $($scripts.Count)"
}

function Test-GitExecutableModes {
    $git = Get-Command git -ErrorAction SilentlyContinue
    if (-not $git) {
        Write-Host "Git executable mode checks skipped: git was not found."
        return
    }

    $scriptPatterns = @(
        "tools/linux/*.sh",
        "tools/docker/*.sh",
        "fnapp/superflare/cmd/*"
    )

    $entries = & $git.Source -C $repoRoot ls-files --stage -- $scriptPatterns 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "git ls-files failed while checking executable modes: $($entries -join "`n")"
    }

    $checked = 0
    foreach ($entry in $entries) {
        if ($entry -notmatch "^(?<mode>\d{6})\s+[0-9a-f]{40,}\s+\d+\s+(?<path>.+)$") {
            continue
        }
        $checked++
        if ($Matches["mode"] -ne "100755") {
            throw "$($Matches["path"]) must be tracked as executable mode 100755, got $($Matches["mode"]). Run: git update-index --chmod=+x -- $($Matches["path"])"
        }
    }

    Write-Host "Git executable mode checks passed: $checked"
}

function Test-CmdWrappers {
    $wrappers = Get-ChildItem -Path (Join-Path $repoRoot "tools/windows") -File -Filter "*.cmd"
    foreach ($wrapper in $wrappers) {
        $text = Get-Content -Path $wrapper.FullName -Raw
        if ($text -match '-File\s+"%~dp0([^"]+\.ps1)"') {
            $target = Join-Path $wrapper.DirectoryName $Matches[1]
            Assert-True -Condition (Test-Path $target -PathType Leaf) -Message "CMD wrapper $($wrapper.Name) points to missing PowerShell script $($Matches[1])"
        } else {
            throw "CMD wrapper $($wrapper.Name) does not call a sibling PowerShell script with -File."
        }
    }
    Write-Host "CMD wrapper checks passed: $($wrappers.Count)"
}

function Test-DockerComposeFiles {
    $docker = Get-Command docker -ErrorAction SilentlyContinue
    if (-not $docker) {
        Write-Host "Docker compose checks skipped: docker was not found."
        return
    }
    $compose = Join-Path $repoRoot "tools/docker/docker-compose.yml"
    $hostProcCompose = Join-Path $repoRoot "tools/docker/docker-compose.host-proc.yml"
    & $docker.Source compose -f $compose config --quiet
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose config failed for tools/docker/docker-compose.yml"
    }
    & $docker.Source compose -f $compose -f $hostProcCompose config --quiet
    if ($LASTEXITCODE -ne 0) {
        throw "docker compose config failed for host proc overlay."
    }
    Write-Host "Docker compose checks passed."
}

function Test-WindowsRuntimeCompatibility {
    $bytes = [byte[]]::new(32)
    $rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
    try {
        $rng.GetBytes($bytes)
    } finally {
        if ($rng -is [System.IDisposable]) {
            $rng.Dispose()
        }
    }
    Assert-True -Condition (($bytes | Where-Object { $_ -ne 0 }).Count -gt 0) -Message "Random cookie secret generation produced only zero bytes."

    $windowsRestart = Get-Content -Path (Join-Path $repoRoot "tools/windows/restart-superflare.ps1") -Raw
    Assert-True -Condition (-not $windowsRestart.Contains("[System.Security.Cryptography.RandomNumberGenerator]::Fill")) -Message "tools/windows/restart-superflare.ps1 must not use RandomNumberGenerator.Fill because Windows PowerShell 5.1 does not provide it."

    Write-Host "Windows runtime compatibility checks passed."
}

function Test-JsonAndTextFixtures {
    $jsonFiles = @(
        "fnapp/superflare/wizard/config",
        "fnapp/superflare/wizard/install"
    )
    foreach ($relative in $jsonFiles) {
        $full = Join-Path $repoRoot $relative
        Assert-True -Condition (Test-Path $full -PathType Leaf) -Message "Missing JSON-like file: $relative"
        $bytes = [System.IO.File]::ReadAllBytes($full)
        if ($bytes.Length -ge 3 -and $bytes[0] -eq 0xEF -and $bytes[1] -eq 0xBB -and $bytes[2] -eq 0xBF) {
            throw "$relative must be UTF-8 without BOM."
        }
        if ($bytes.Length -gt 0 -and $bytes[$bytes.Length - 1] -ne 0x0A) {
            throw "$relative must end with a newline."
        }
        $text = [System.Text.Encoding]::UTF8.GetString($bytes)
        try {
            $null = $text | ConvertFrom-Json
        } catch {
            throw "$relative is not valid JSON: $($_.Exception.Message)"
        }
    }
    Write-Host "JSON/text fixture checks passed: $($jsonFiles.Count)"
}

function Test-FnappLifecycleConfigPreservation {
    $testScript = Join-Path $repoRoot "fnapp/test-lifecycle-config-preservation.ps1"
    Assert-True -Condition (Test-Path $testScript -PathType Leaf) -Message "Missing fnapp lifecycle configuration preservation test: $testScript"

    Write-Host "Testing fnapp lifecycle configuration preservation..."
    & $testScript
    if (-not $?) {
        throw "fnapp lifecycle configuration preservation test failed."
    }
}

Test-PowerShellScripts
Test-ShellScripts
Test-ShellScriptTextFormat
Test-GitExecutableModes
Test-CmdWrappers
Test-DockerComposeFiles
Test-WindowsRuntimeCompatibility
Test-JsonAndTextFixtures
Test-FnappLifecycleConfigPreservation
Write-Host "All script checks passed."
