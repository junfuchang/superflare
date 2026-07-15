Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$protectedConfigFiles = @(".env", "config.yml", "apps.yml", "bookmarks.yml", "ports.yaml")
$tempBase = [System.IO.Path]::GetFullPath([System.IO.Path]::GetTempPath())
$tempRoot = [System.IO.Path]::GetFullPath((Join-Path $tempBase ("superflare-lifecycle-config-" + [System.Guid]::NewGuid().ToString("N"))))
$installCallback = Join-Path $PSScriptRoot "superflare\cmd\install_callback"
$upgradeCallback = Join-Path $PSScriptRoot "superflare\cmd\upgrade_callback"
$uninstallCallback = Join-Path $PSScriptRoot "superflare\cmd\uninstall_callback"
$bashPath = $null

function Resolve-GitBash {
    $bashCandidates = @(
        "C:\Program Files\Git\bin\bash.exe",
        "C:\Program Files\Git\usr\bin\bash.exe"
    )
    foreach ($candidate in $bashCandidates) {
        if (Test-Path -LiteralPath $candidate -PathType Leaf) {
            return $candidate
        }
    }

    $bashCommand = Get-Command bash -ErrorAction SilentlyContinue
    if ($bashCommand -and $bashCommand.Source -and $bashCommand.Source -notlike "*\system32\bash.exe") {
        return $bashCommand.Source
    }

    throw "Git Bash not found. Install Git for Windows or provide a bash-compatible shell to run fnapp script tests."
}

function To-ShPath([string]$Path) {
    return $Path.Replace('\', '/')
}

function Write-Utf8NoBom([string]$Path, [string]$Content) {
    $parent = Split-Path -Parent $Path
    if ($parent) {
        $null = [System.IO.Directory]::CreateDirectory($parent)
    }
    $encoding = New-Object System.Text.UTF8Encoding($false)
    [System.IO.File]::WriteAllText($Path, $Content, $encoding)
}

function New-LifecycleScenario([string]$Name, [bool]$WithExistingConfig) {
    $root = Join-Path $tempRoot $Name
    $appRoot = Join-Path $root "app"
    $etcRoot = Join-Path $root "etc"
    $varRoot = Join-Path $root "var"
    $defaultsRoot = Join-Path $appRoot "server\defaults"
    $defaultApplicationSentinel = "default-application-$Name"
    $defaultBookmarkSentinel = "default-bookmark-$Name"
    $defaultPortSentinel = "default-port-$Name"

    $null = [System.IO.Directory]::CreateDirectory($defaultsRoot)
    $null = [System.IO.Directory]::CreateDirectory($etcRoot)
    $null = [System.IO.Directory]::CreateDirectory($varRoot)

    Write-Utf8NoBom (Join-Path $defaultsRoot "config.yml") @"
Title: 'default-title-$Name'
LoginUser: 'default-user'
LoginPass: 'default-pass'
"@
    Write-Utf8NoBom (Join-Path $defaultsRoot "apps.yml") @"
- name: '$defaultApplicationSentinel'
  url: 'https://default.invalid/$Name'
"@
    Write-Utf8NoBom (Join-Path $defaultsRoot "bookmarks.yml") @"
- name: '$defaultBookmarkSentinel'
  url: 'https://bookmark.invalid/$Name'
"@
    Write-Utf8NoBom (Join-Path $defaultsRoot "ports.yaml") @"
${defaultPortSentinel}: 7000
"@

    if ($WithExistingConfig) {
        Write-Utf8NoBom (Join-Path $etcRoot ".env") @"
# existing-env-$Name
FLARE_PORT=9443
FLARE_DISABLE_LOGIN=false
FLARE_EDITOR=true
FLARE_GUIDE=true
FLARE_COOKIE_NAME=superflare
export FLARE_COOKIE_SECRET = "custom-secret"
FLARE_USER=existing-user-$Name
FLARE_PASS=existing-pass-$Name
"@
        Write-Utf8NoBom (Join-Path $etcRoot "config.yml") @"
# existing-config-$Name
Title: 'existing-title-$Name'
LoginUser: 'existing-user-$Name'
LoginPass: 'existing-pass-$Name'
"@
        Write-Utf8NoBom (Join-Path $etcRoot "apps.yml") @"
- name: 'existing-application-$Name'
  url: 'https://existing-app.invalid/$Name'
"@
        Write-Utf8NoBom (Join-Path $etcRoot "bookmarks.yml") @"
- name: 'existing-bookmark-$Name'
  url: 'https://existing-bookmark.invalid/$Name'
"@
        Write-Utf8NoBom (Join-Path $etcRoot "ports.yaml") @"
existing-port-${Name}: 9443
"@
    }

    return [pscustomobject]@{
        Root = $root
        AppRoot = $appRoot
        EtcRoot = $etcRoot
        VarRoot = $varRoot
        DefaultApplicationSentinel = $defaultApplicationSentinel
        DefaultBookmarkSentinel = $defaultBookmarkSentinel
        DefaultPortSentinel = $defaultPortSentinel
    }
}

function Invoke-LifecycleCallback([string]$ScriptPath, [string]$ScenarioRoot, [hashtable]$WizardValues) {
    $appRoot = Join-Path $ScenarioRoot "app"
    $etcRoot = Join-Path $ScenarioRoot "etc"
    $varRoot = Join-Path $ScenarioRoot "var"
    $logPath = Join-Path $ScenarioRoot "lifecycle.log"
    $managedNames = @(
        "TRIM_APPDEST",
        "TRIM_PKGETC",
        "TRIM_PKGVAR",
        "TRIM_SERVICE_PORT",
        "TRIM_TEMP_LOGFILE",
        "wizard_install_login_user",
        "wizard_install_login_pass",
        "wizard_install_login_pass_confirm"
    )
    $previousValues = @{}

    foreach ($name in $managedNames) {
        $previousValues[$name] = [System.Environment]::GetEnvironmentVariable($name, "Process")
        [System.Environment]::SetEnvironmentVariable($name, $null, "Process")
    }

    try {
        [System.Environment]::SetEnvironmentVariable("TRIM_APPDEST", (To-ShPath $appRoot), "Process")
        [System.Environment]::SetEnvironmentVariable("TRIM_PKGETC", (To-ShPath $etcRoot), "Process")
        [System.Environment]::SetEnvironmentVariable("TRIM_PKGVAR", (To-ShPath $varRoot), "Process")
        [System.Environment]::SetEnvironmentVariable("TRIM_SERVICE_PORT", "3636", "Process")
        [System.Environment]::SetEnvironmentVariable("TRIM_TEMP_LOGFILE", (To-ShPath $logPath), "Process")

        foreach ($entry in $WizardValues.GetEnumerator()) {
            if ($managedNames -notcontains [string]$entry.Key) {
                throw "Unsupported lifecycle wizard variable: $($entry.Key)"
            }
            [System.Environment]::SetEnvironmentVariable([string]$entry.Key, [string]$entry.Value, "Process")
        }

        $scriptSh = To-ShPath ([System.IO.Path]::GetFullPath($ScriptPath))
        $output = @(& $bashPath $scriptSh 2>&1)
        $exitCode = $LASTEXITCODE
        if ($exitCode -ne 0) {
            throw "Lifecycle callback '$ScriptPath' failed with code $exitCode`n$($output -join "`n")"
        }
    }
    finally {
        foreach ($name in $managedNames) {
            [System.Environment]::SetEnvironmentVariable($name, $previousValues[$name], "Process")
        }
    }
}

function Get-ConfigHashes([string]$EtcRoot) {
    $hashes = @{}
    foreach ($name in $protectedConfigFiles) {
        $path = Join-Path $EtcRoot $name
        if (-not (Test-Path -LiteralPath $path -PathType Leaf)) {
            throw "Expected configuration file is missing: $path"
        }
        $hashes[$name] = (Get-FileHash -LiteralPath $path -Algorithm SHA256).Hash
    }
    return $hashes
}

function Assert-HashesEqual([hashtable]$Expected, [hashtable]$Actual, [string]$Context) {
    $differences = @()
    foreach ($name in $protectedConfigFiles) {
        if (-not $Expected.ContainsKey($name)) {
            $differences += "expected hash missing for '$name'"
            continue
        }
        if (-not $Actual.ContainsKey($name)) {
            $differences += "actual hash missing for '$name'"
            continue
        }
        if ($Expected[$name] -ne $Actual[$name]) {
            $differences += "'$name' (expected $($Expected[$name]), actual $($Actual[$name]))"
        }
    }
    if ($differences.Count -gt 0) {
        throw "$Context changed persisted configuration: $($differences -join '; ')."
    }
}

function Find-PreservedLinkPath([string]$EtcRoot) {
    $basePath = Join-Path $EtcRoot "var.pre-superflare-link"
    for ($index = 0; $index -le 1000; $index++) {
        $candidate = $basePath
        if ($index -gt 0) {
            $candidate = "$basePath.$index"
        }
        if (Test-Path -LiteralPath (Join-Path $candidate "operator-var-marker.txt") -PathType Leaf) {
            return $candidate
        }
    }
    return $null
}

function Assert-Contains([string]$Path, [string]$Expected, [string]$Context) {
    $content = Get-Content -Raw -LiteralPath $Path
    if ($content.IndexOf($Expected, [System.StringComparison]::Ordinal) -lt 0) {
        throw "$Context did not contain '$Expected'. Actual content:`n$content"
    }
}

function Assert-FileHashEqual([string]$Path, [string]$ExpectedHash, [string]$Context) {
    if (-not (Test-Path -LiteralPath $Path -PathType Leaf)) {
        throw "$Context is missing: $Path"
    }
    $actualHash = (Get-FileHash -LiteralPath $Path -Algorithm SHA256).Hash
    if ($actualHash -ne $ExpectedHash) {
        throw "$Context changed: expected $ExpectedHash, actual $actualHash."
    }
}

$tempPrefix = $tempBase.TrimEnd([System.IO.Path]::DirectorySeparatorChar, [System.IO.Path]::AltDirectorySeparatorChar) + [System.IO.Path]::DirectorySeparatorChar
$tempLeaf = [System.IO.Path]::GetFileName($tempRoot)
$cleanupRootVerified = $tempRoot.StartsWith($tempPrefix, [System.StringComparison]::OrdinalIgnoreCase) -and $tempLeaf.StartsWith("superflare-lifecycle-config-", [System.StringComparison]::Ordinal)
if (-not $cleanupRootVerified) {
    throw "Refusing to use unverified lifecycle test root: $tempRoot"
}

try {
    $null = [System.IO.Directory]::CreateDirectory($tempRoot)
    $bashPath = Resolve-GitBash

    $existingInstall = New-LifecycleScenario "existing-install" $true
    $existingUpgrade = New-LifecycleScenario "existing-upgrade" $true
    $freshInstall = New-LifecycleScenario "fresh-install" $false

    $before = Get-ConfigHashes $existingInstall.EtcRoot
    Invoke-LifecycleCallback $installCallback $existingInstall.Root @{
        wizard_install_login_user = "replacement-user"
        wizard_install_login_pass = "replacement-pass"
        wizard_install_login_pass_confirm = "replacement-pass"
    }
    Assert-HashesEqual $before (Get-ConfigHashes $existingInstall.EtcRoot) "repeated install"

    $before = Get-ConfigHashes $existingUpgrade.EtcRoot
    Invoke-LifecycleCallback $upgradeCallback $existingUpgrade.Root @{}
    Assert-HashesEqual $before (Get-ConfigHashes $existingUpgrade.EtcRoot) "upgrade"

    Invoke-LifecycleCallback $installCallback $freshInstall.Root @{
        wizard_install_login_user = "fresh-user"
        wizard_install_login_pass = "fresh-pass"
        wizard_install_login_pass_confirm = "fresh-pass"
    }
    $null = Get-ConfigHashes $freshInstall.EtcRoot
    Assert-Contains (Join-Path $freshInstall.EtcRoot ".env") "FLARE_USER=fresh-user" "fresh install .env username"
    Assert-Contains (Join-Path $freshInstall.EtcRoot ".env") "FLARE_PASS=fresh-pass" "fresh install .env password"
    Assert-Contains (Join-Path $freshInstall.EtcRoot "config.yml") "LoginUser: 'fresh-user'" "fresh install config username"
    Assert-Contains (Join-Path $freshInstall.EtcRoot "config.yml") "LoginPass: 'fresh-pass'" "fresh install config password"
    Assert-Contains (Join-Path $freshInstall.EtcRoot "apps.yml") $freshInstall.DefaultApplicationSentinel "fresh install application defaults"
    Assert-Contains (Join-Path $freshInstall.EtcRoot "bookmarks.yml") $freshInstall.DefaultBookmarkSentinel "fresh install bookmark defaults"
    Assert-Contains (Join-Path $freshInstall.EtcRoot "ports.yaml") $freshInstall.DefaultPortSentinel "fresh install port defaults"

    $partialUpgrade = New-LifecycleScenario "partial-upgrade" $false
    Write-Utf8NoBom (Join-Path $partialUpgrade.EtcRoot ".env") @"
# partial-env-sentinel
FLARE_PORT=9443
FLARE_COOKIE_SECRET=partial-secret
FLARE_USER=partial-custom-user
"@
    Write-Utf8NoBom (Join-Path $partialUpgrade.EtcRoot "config.yml") @"
# partial-config-sentinel
Title: 'partial-title'
LoginUser: 'partial-custom-user'
"@
    Invoke-LifecycleCallback $upgradeCallback $partialUpgrade.Root @{}
    Assert-Contains (Join-Path $partialUpgrade.EtcRoot ".env") "FLARE_USER=partial-custom-user" "partial upgrade .env username"
    Assert-Contains (Join-Path $partialUpgrade.EtcRoot "config.yml") "LoginUser: 'partial-custom-user'" "partial upgrade config username"

    $legacyOnly = New-LifecycleScenario "legacy-only" $false
    $legacyRoot = Join-Path $legacyOnly.VarRoot "runtime"
    $null = [System.IO.Directory]::CreateDirectory($legacyRoot)
    Write-Utf8NoBom (Join-Path $legacyRoot ".env") @"
# legacy-env-sentinel
FLARE_PORT=9443
FLARE_DISABLE_LOGIN=false
FLARE_EDITOR=true
FLARE_GUIDE=true
FLARE_COOKIE_NAME=superflare
FLARE_COOKIE_SECRET=legacy-secret
FLARE_USER=legacy-user
FLARE_PASS=legacy-pass
"@
    Write-Utf8NoBom (Join-Path $legacyRoot "config.yml") @"
# legacy-config-sentinel
LoginUser: 'legacy-user'
LoginPass: 'legacy-pass'
"@
    Write-Utf8NoBom (Join-Path $legacyRoot "apps.yml") "# legacy-apps-sentinel`n"
    Write-Utf8NoBom (Join-Path $legacyRoot "bookmarks.yml") "# legacy-bookmarks-sentinel`n"
    Write-Utf8NoBom (Join-Path $legacyRoot "ports.yaml") "# legacy-ports-sentinel`n"
    $legacyHashes = Get-ConfigHashes $legacyRoot
    Invoke-LifecycleCallback $installCallback $legacyOnly.Root @{
        wizard_install_login_user = "replacement-user"
        wizard_install_login_pass = "replacement-pass"
        wizard_install_login_pass_confirm = "replacement-pass"
    }
    Assert-HashesEqual $legacyHashes (Get-ConfigHashes $legacyOnly.EtcRoot) "legacy-only install migration"
    foreach ($name in $protectedConfigFiles) {
        if (Test-Path -LiteralPath (Join-Path $legacyRoot $name)) {
            throw "legacy-only install did not move '$name' into etc."
        }
    }
    Assert-Contains (Join-Path $legacyOnly.EtcRoot ".env") "FLARE_USER=legacy-user" "legacy-only install .env username"
    Assert-Contains (Join-Path $legacyOnly.EtcRoot "config.yml") "LoginUser: 'legacy-user'" "legacy-only install config username"

    $legacyConflict = New-LifecycleScenario "legacy-conflict" $true
    $legacyConflictRoot = Join-Path $legacyConflict.VarRoot "runtime"
    $legacyConflictBookmark = Join-Path $legacyConflictRoot "bookmarks.yml"
    $currentConflictBookmark = Join-Path $legacyConflict.EtcRoot "bookmarks.yml"
    $null = [System.IO.Directory]::CreateDirectory($legacyConflictRoot)
    Write-Utf8NoBom $legacyConflictBookmark "# legacy-conflict-bookmark-sentinel`n"
    $legacyConflictHash = (Get-FileHash -LiteralPath $legacyConflictBookmark -Algorithm SHA256).Hash
    $currentConflictHash = (Get-FileHash -LiteralPath $currentConflictBookmark -Algorithm SHA256).Hash
    Invoke-LifecycleCallback $upgradeCallback $legacyConflict.Root @{}
    Assert-FileHashEqual $currentConflictBookmark $currentConflictHash "upgrade current bookmark conflict copy"
    Assert-FileHashEqual $legacyConflictBookmark $legacyConflictHash "upgrade legacy bookmark conflict copy"

    $realVar = New-LifecycleScenario "real-etc-var" $true
    $realVarPath = Join-Path $realVar.EtcRoot "var"
    $collisionPath = Join-Path $realVar.EtcRoot "var.pre-superflare-link"
    $realVarMarker = Join-Path $realVarPath "operator-var-marker.txt"
    $collisionMarker = Join-Path $collisionPath "collision-marker.txt"
    $null = [System.IO.Directory]::CreateDirectory($realVarPath)
    $null = [System.IO.Directory]::CreateDirectory($collisionPath)
    Write-Utf8NoBom $realVarMarker "operator-var-data-sentinel`n"
    Write-Utf8NoBom $collisionMarker "preexisting-backup-collision-sentinel`n"
    $realVarMarkerHash = (Get-FileHash -LiteralPath $realVarMarker -Algorithm SHA256).Hash
    $collisionMarkerHash = (Get-FileHash -LiteralPath $collisionMarker -Algorithm SHA256).Hash
    Invoke-LifecycleCallback $upgradeCallback $realVar.Root @{}
    Assert-FileHashEqual $collisionMarker $collisionMarkerHash "upgrade existing var preservation collision"
    $preservedVarPath = Find-PreservedLinkPath $realVar.EtcRoot
    if (-not $preservedVarPath) {
        throw "upgrade did not preserve the displaced real etc/var under a unique var.pre-superflare-link.N path."
    }
    $expectedPreservedVarPath = "$collisionPath.1"
    if ($preservedVarPath -ne $expectedPreservedVarPath) {
        throw "upgrade preserved real etc/var at '$preservedVarPath'; expected first free collision suffix '$expectedPreservedVarPath'."
    }
    Assert-FileHashEqual (Join-Path $preservedVarPath "operator-var-marker.txt") $realVarMarkerHash "upgrade displaced real etc/var marker"

    $uninstall = New-LifecycleScenario "uninstall" $false
    $uninstallVarPath = Join-Path $uninstall.EtcRoot "var"
    $uninstallLegacyRoot = Join-Path $uninstall.VarRoot "runtime"
    $uninstallVarMarker = Join-Path $uninstallVarPath "operator-uninstall-marker.txt"
    $uninstallLegacyMarker = Join-Path $uninstallLegacyRoot "config.yml"
    $null = [System.IO.Directory]::CreateDirectory($uninstallVarPath)
    $null = [System.IO.Directory]::CreateDirectory($uninstallLegacyRoot)
    Write-Utf8NoBom $uninstallVarMarker "operator-uninstall-data-sentinel`n"
    Write-Utf8NoBom $uninstallLegacyMarker "# legacy-uninstall-config-sentinel`n"
    $uninstallVarHash = (Get-FileHash -LiteralPath $uninstallVarMarker -Algorithm SHA256).Hash
    $uninstallLegacyHash = (Get-FileHash -LiteralPath $uninstallLegacyMarker -Algorithm SHA256).Hash
    Invoke-LifecycleCallback $uninstallCallback $uninstall.Root @{}
    Assert-FileHashEqual $uninstallVarMarker $uninstallVarHash "uninstall real etc/var marker"
    Assert-FileHashEqual $uninstallLegacyMarker $uninstallLegacyHash "uninstall legacy runtime configuration"

    Write-Host "fnapp lifecycle configuration preservation verified."
}
finally {
    $cleanupCandidate = [System.IO.Path]::GetFullPath($tempRoot)
    if ($cleanupRootVerified -and $cleanupCandidate -eq $tempRoot -and (Test-Path -LiteralPath $cleanupCandidate)) {
        Remove-Item -LiteralPath $cleanupCandidate -Recurse -Force
    }
}
