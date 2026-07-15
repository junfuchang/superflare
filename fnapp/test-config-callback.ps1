Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$scriptPath = Join-Path $PSScriptRoot "superflare\cmd\config_callback"
$tmpRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("superflare-config-callback-" + [System.Guid]::NewGuid().ToString("N"))
$appRoot = Join-Path $tmpRoot "app"
$etcRoot = Join-Path $tmpRoot "etc"
$varRoot = Join-Path $tmpRoot "var"
$defaultsRoot = Join-Path $appRoot "server\defaults"
$serverRoot = Join-Path $appRoot "server"

function Resolve-GitBash {
    $bashCandidates = @(
        "C:\Program Files\Git\bin\bash.exe",
        "C:\Program Files\Git\usr\bin\bash.exe"
    )
    foreach ($candidate in $bashCandidates) {
        if (Test-Path $candidate -PathType Leaf) {
            return $candidate
        }
    }
    $bashCmd = Get-Command bash -ErrorAction SilentlyContinue
    if ($bashCmd -and $bashCmd.Source -and $bashCmd.Source -notlike "*\system32\bash.exe") {
        return $bashCmd.Source
    }
    throw "Git Bash not found. Install Git for Windows or provide a bash-compatible shell to run fnapp script tests."
}

function To-ShPath([string]$path) {
    return $path.Replace('\', '/')
}

$stubProcessCommandMarker = (To-ShPath (Join-Path $serverRoot "superflare"))

try {
    $null = New-Item -ItemType Directory -Path $defaultsRoot -Force
    $null = New-Item -ItemType Directory -Path $etcRoot -Force
    $null = New-Item -ItemType Directory -Path $varRoot -Force

    @"
Title: 'SuperFlare'
Locale: 'zh'
Theme: 'blackboard'
LoginUser: 'old-user'
LoginPass: 'old-pass'
"@ | Set-Content -Path (Join-Path $etcRoot "config.yml") -Encoding UTF8

    @"
FLARE_DISABLE_LOGIN=false
FLARE_USER=old-user
FLARE_PASS=old-pass
FLARE_COOKIE_SECRET=test-secret
"@ | Set-Content -Path (Join-Path $etcRoot ".env") -Encoding UTF8

    @"
#!/bin/sh
while true; do sleep 60; done
"@ | Set-Content -Path (Join-Path $serverRoot "superflare") -Encoding ASCII

    $bashPath = Resolve-GitBash
    $scriptSh = To-ShPath $scriptPath
    $appSh = To-ShPath $appRoot
    $etcSh = To-ShPath $etcRoot
    $varSh = To-ShPath $varRoot
    & $bashPath -c "chmod +x '$appSh/server/superflare'"

    $callbackHarnessRoot = Join-Path $tmpRoot "write-failure-harness"
    $callbackHarnessScript = Join-Path $callbackHarnessRoot "config_callback"
    $callbackHarnessCommon = Join-Path $callbackHarnessRoot "common.sh"
    $callbackHarnessLog = Join-Path $callbackHarnessRoot "lifecycle.log"
    $null = [System.IO.Directory]::CreateDirectory($callbackHarnessRoot)
    Copy-Item -LiteralPath $scriptPath -Destination $callbackHarnessScript
    [System.IO.File]::WriteAllText($callbackHarnessCommon, @'
CONFIG_FILE="${HARNESS_CONFIG_FILE:-}"
ensure_runtime_layout() { return 0; }
read_yaml_value() {
    if [ "${HARNESS_READ_FAIL:-false}" = "true" ]; then
        return 77
    fi
    case "$2" in
        LoginUser) printf '%s\n' old-user ;;
        LoginPass) printf '%s\n' old-pass ;;
    esac
}
sync_login_config() {
    [ "${HARNESS_SYNC_FAIL:-true}" != "true" ]
}
sync_login_enabled_config() { return 0; }
stop_app() { return 0; }
start_app() { return 0; }
log_info() { :; }
log_lifecycle() { printf '%s\n' "$*" >> "${HARNESS_LOG}"; }
'@, [System.Text.UTF8Encoding]::new($false))
    $callbackHarnessScriptSh = To-ShPath $callbackHarnessScript
    $callbackHarnessLogSh = To-ShPath $callbackHarnessLog
    & $bashPath -c "chmod +x '$callbackHarnessScriptSh'"
    $callbackHarnessCommand = "HARNESS_LOG='$callbackHarnessLogSh' HARNESS_SYNC_FAIL='true' wizard_login_enabled='true' wizard_login_user='write-failure-user' wizard_login_pass='write-failure-pass' wizard_login_pass_confirm='write-failure-pass' '$callbackHarnessScriptSh'"
    $callbackHarnessOutput = @(& $bashPath -c $callbackHarnessCommand 2>&1)
    if ($LASTEXITCODE -eq 0) {
        throw "config_callback succeeded when credential synchronization failed.`n$($callbackHarnessOutput -join "`n")"
    }
    if (-not (Test-Path -LiteralPath $callbackHarnessLog -PathType Leaf) -or
        (Get-Content -Raw -LiteralPath $callbackHarnessLog) -notmatch [regex]::Escape("Failed to apply login settings.")) {
        throw "config_callback did not log the credential synchronization failure."
    }

    Remove-Item -LiteralPath $callbackHarnessLog -Force
    $callbackHarnessCommand = "HARNESS_LOG='$callbackHarnessLogSh' HARNESS_READ_FAIL='true' HARNESS_SYNC_FAIL='false' wizard_login_enabled='true' wizard_login_user='read-failure-user' wizard_login_pass='read-failure-pass' wizard_login_pass_confirm='read-failure-pass' '$callbackHarnessScriptSh'"
    $callbackHarnessOutput = @(& $bashPath -c $callbackHarnessCommand 2>&1)
    if ($LASTEXITCODE -eq 0) {
        throw "config_callback succeeded when current login settings could not be read.`n$($callbackHarnessOutput -join "`n")"
    }
    if (-not (Test-Path -LiteralPath $callbackHarnessLog -PathType Leaf) -or
        (Get-Content -Raw -LiteralPath $callbackHarnessLog) -notmatch [regex]::Escape("Failed to read current login settings.")) {
        throw "config_callback did not log the current-login read failure."
    }

    $command = "TRIM_APPDEST='$appSh' TRIM_PKGETC='$etcSh' TRIM_PKGVAR='$varSh' wizard_login_enabled='false' wizard_login_user='new-user' wizard_login_pass='new-pass' wizard_login_pass_confirm='new-pass' '$scriptSh'"
    $output = & $bashPath -c $command 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "config_callback disable-login run failed with code $LASTEXITCODE`n$output"
    }

    $envText = Get-Content -Raw -LiteralPath (Join-Path $etcRoot ".env")
    $configText = Get-Content -Raw -LiteralPath (Join-Path $etcRoot "config.yml")
    if ($envText -notmatch "(?m)^FLARE_DISABLE_LOGIN=true$") {
        throw "Expected disabled login to write FLARE_DISABLE_LOGIN=true. Actual .env:`n$envText"
    }
    if ($envText -notmatch "(?m)^FLARE_USER=new-user$" -or $envText -notmatch "(?m)^FLARE_PASS=new-pass$") {
        throw "Expected login credentials to be synced to .env. Actual .env:`n$envText"
    }
    if ($configText -notmatch "LoginUser: 'new-user'" -or $configText -notmatch "LoginPass: 'new-pass'") {
        throw "Expected login credentials to be synced to config.yml. Actual config.yml:`n$configText"
    }

    $command = "TRIM_APPDEST='$appSh' TRIM_PKGETC='$etcSh' TRIM_PKGVAR='$varSh' wizard_login_enabled='true' wizard_login_user='' wizard_login_pass='' wizard_login_pass_confirm='' '$scriptSh'"
    $output = & $bashPath -c $command 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "config_callback enable-login run failed with code $LASTEXITCODE`n$output"
    }

    $envText = Get-Content -Raw -LiteralPath (Join-Path $etcRoot ".env")
    if ($envText -notmatch "(?m)^FLARE_DISABLE_LOGIN=false$") {
        throw "Expected enabled login to write FLARE_DISABLE_LOGIN=false. Actual .env:`n$envText"
    }
    if ($envText -notmatch "(?m)^FLARE_USER=new-user$" -or $envText -notmatch "(?m)^FLARE_PASS=new-pass$") {
        throw "Expected blank credentials to preserve current values. Actual .env:`n$envText"
    }

    Write-Host "config_callback login toggle verified."
}
finally {
    Get-CimInstance Win32_Process -ErrorAction SilentlyContinue |
        Where-Object { $_.CommandLine -like "*$stubProcessCommandMarker*" } |
        ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
    if (Test-Path $tmpRoot) {
        Remove-Item -Path $tmpRoot -Recurse -Force
    }
}
