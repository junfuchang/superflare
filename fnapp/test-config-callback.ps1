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
