Set-StrictMode -Version Latest
$ErrorActionPreference = "Stop"

$commonPath = Join-Path $PSScriptRoot "superflare\cmd\common.sh"
$tmpRoot = Join-Path ([System.IO.Path]::GetTempPath()) ("superflare-common-" + [System.Guid]::NewGuid().ToString("N"))

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

try {
    $null = New-Item -ItemType Directory -Path $tmpRoot -Force
    $bashPath = Resolve-GitBash
    $commonSh = To-ShPath $commonPath
    $tmpSh = To-ShPath $tmpRoot
    $testScriptPath = Join-Path $tmpRoot "common-test.sh"
    $testScriptSh = To-ShPath $testScriptPath

    $script = @"
set -e
TRIM_APPDEST='$tmpSh/app'
TRIM_PKGETC='$tmpSh/etc'
TRIM_PKGVAR='$tmpSh/var'
. '$commonSh'

mkdir -p "`$TRIM_PKGETC" "`$TRIM_PKGVAR/run" "`$TRIM_APPDEST/server"
env_file="`$TRIM_PKGETC/.env"
yaml_file="`$TRIM_PKGETC/config.yml"
lock_file="`$TRIM_PKGETC/.superflare-config.lock"

cat >"`$env_file" <<'EOF'
export FLARE_USER = "admin user"
FLARE_PASS='abc#123:xyz'
FLARE_COOKIE_SECRET = old-secret # comment
EOF

[ "`$(read_env_value "`$env_file" FLARE_USER)" = "admin user" ] || { echo "FLARE_USER parse failed: `$(read_env_value "`$env_file" FLARE_USER)"; exit 1; }
[ "`$(read_env_value "`$env_file" FLARE_PASS)" = "abc#123:xyz" ] || { echo "FLARE_PASS parse failed: `$(read_env_value "`$env_file" FLARE_PASS)"; exit 1; }

upsert_env_value "`$env_file" FLARE_PASS 'new pass#1'
grep -Eq '^FLARE_PASS="new pass#1"`$' "`$env_file" || { echo "quoted env upsert failed"; cat "`$env_file"; exit 1; }

cat >"`$yaml_file" <<'EOF'
LoginUser: "old user"
LoginPass: 'abc#123:xyz'
Title: "SuperFlare"
EOF

[ "`$(read_yaml_value "`$yaml_file" LoginUser)" = "old user" ] || { echo "LoginUser yaml parse failed: `$(read_yaml_value "`$yaml_file" LoginUser)"; exit 1; }
[ "`$(read_yaml_value "`$yaml_file" LoginPass)" = "abc#123:xyz" ] || { echo "LoginPass yaml parse failed: `$(read_yaml_value "`$yaml_file" LoginPass)"; exit 1; }

upsert_yaml_value "`$yaml_file" LoginPass 'new: pass #1'
grep -Eq "^LoginPass: 'new: pass #1'`$" "`$yaml_file" || { echo "yaml upsert failed"; cat "`$yaml_file"; exit 1; }

cat >"`$env_file" <<'EOF'
FLARE_USER=custom-user
FLARE_PASS=
EOF
ensure_login_env_defaults
grep -Eq '^FLARE_USER=admin`$' "`$env_file" || { echo "env blank password should reset user to admin"; cat "`$env_file"; exit 1; }
grep -Eq '^FLARE_PASS=admin`$' "`$env_file" || { echo "env blank password should reset pass to admin"; cat "`$env_file"; exit 1; }

cat >"`$yaml_file" <<'EOF'
LoginUser: 'custom-user'
LoginPass: ''
Title: "SuperFlare"
EOF
ensure_login_config_defaults
grep -Eq "^LoginUser: 'admin'`$" "`$yaml_file" || { echo "yaml blank password should reset user to admin"; cat "`$yaml_file"; exit 1; }
grep -Eq "^LoginPass: 'admin'`$" "`$yaml_file" || { echo "yaml blank password should reset pass to admin"; cat "`$yaml_file"; exit 1; }

lock_result_file="`$TRIM_PKGETC/lock-result"
write_lock_result() {
    printf 'locked\n' > "`$lock_result_file"
}
with_config_lock write_lock_result
[ "`$(cat "`$lock_result_file" 2>/dev/null)" = "locked" ] || { echo "with_config_lock did not run command"; exit 1; }
[ -f "`$lock_file" ] || { echo "with_config_lock did not create shared lock file"; exit 1; }

APP_BIN="`$TRIM_APPDEST/server/superflare"
printf '%s\n' '#!/bin/sh' 'while true; do sleep 60; done' > "`$APP_BIN"
chmod +x "`$APP_BIN"
bad_pid=""
"`$APP_BIN" >/dev/null 2>&1 &
good_pid="`$!"
trap 'kill "`$good_pid" "`$bad_pid" 2>/dev/null || true' EXIT
sleep 0.3
process_matches_pid "`$good_pid" || { echo "expected app pid to match"; exit 1; }

(sh -c 'while true; do sleep 60; done') >/dev/null 2>&1 &
bad_pid="`$!"
sleep 0.3
if process_matches_pid "`$bad_pid"; then
    echo "unrelated process matched unexpectedly"
    exit 1
fi

echo "common.sh helpers verified."
"@
    [System.IO.File]::WriteAllText($testScriptPath, $script, [System.Text.UTF8Encoding]::new($false))

    $output = & $bashPath $testScriptSh 2>&1
    if ($LASTEXITCODE -ne 0) {
        throw "common.sh helper tests failed with code $LASTEXITCODE`n$output"
    }
    Write-Host ($output -join "`n")
}
finally {
    if (Test-Path $tmpRoot) {
        Remove-Item -Path $tmpRoot -Recurse -Force
    }
}
