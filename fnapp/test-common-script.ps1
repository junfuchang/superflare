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
    $danglingReadTarget = Join-Path $tmpRoot "dangling-read-target"
    $danglingReadPath = Join-Path $tmpRoot "dangling-read-link"
    $null = [System.IO.Directory]::CreateDirectory($danglingReadTarget)
    $danglingReadLink = New-Item -ItemType Junction -Path $danglingReadPath -Target $danglingReadTarget
    if ($danglingReadLink.LinkType -ne "Junction") {
        throw "Expected dangling reader test path to be a Junction, found '$($danglingReadLink.LinkType)'."
    }
    Remove-Item -LiteralPath $danglingReadTarget -Recurse -Force
    $danglingReadSh = To-ShPath $danglingReadPath
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

missing_read_path="`$TRIM_PKGETC/missing-read-path"
missing_value=""
missing_value="`$(read_env_value "`$missing_read_path" FLARE_USER)" || { echo "read_env_value rejected a missing path"; exit 1; }
[ -z "`$missing_value" ] || { echo "read_env_value returned data for a missing path"; exit 1; }
missing_value="`$(read_yaml_value "`$missing_read_path" LoginUser)" || { echo "read_yaml_value rejected a missing path"; exit 1; }
[ -z "`$missing_value" ] || { echo "read_yaml_value returned data for a missing path"; exit 1; }

env_read_failure_path="`$TRIM_PKGETC/env-read-failure"
yaml_read_failure_path="`$TRIM_PKGETC/yaml-read-failure"
mkdir -p "`$env_read_failure_path" "`$yaml_read_failure_path"
if read_env_value "`$env_read_failure_path" FLARE_USER >/dev/null 2>&1; then
    echo "read_env_value accepted a directory path"
    exit 1
fi
if read_yaml_value "`$yaml_read_failure_path" LoginUser >/dev/null 2>&1; then
    echo "read_yaml_value accepted a directory path"
    exit 1
fi

dangling_read_path='$danglingReadSh'
if read_env_value "`$dangling_read_path" FLARE_USER >/dev/null 2>&1; then
    echo "read_env_value accepted a dangling symbolic link"
    exit 1
fi
if read_yaml_value "`$dangling_read_path" LoginUser >/dev/null 2>&1; then
    echo "read_yaml_value accepted a dangling symbolic link"
    exit 1
fi

env_parse_status=0
(
    parse_shell_value() { return 73; }
    read_env_value "`$env_file" FLARE_USER >/dev/null
) || env_parse_status=`$?
[ "`$env_parse_status" -eq 73 ] || { echo "read_env_value masked parser status 73 as `$env_parse_status"; exit 1; }

yaml_parse_status=0
(
    parse_shell_value() { return 74; }
    read_yaml_value "`$yaml_file" LoginUser >/dev/null
) || yaml_parse_status=`$?
[ "`$yaml_parse_status" -eq 74 ] || { echo "read_yaml_value masked parser status 74 as `$yaml_parse_status"; exit 1; }

upsert_yaml_value "`$yaml_file" LoginPass 'new: pass #1'
grep -Eq "^LoginPass: 'new: pass #1'`$" "`$yaml_file" || { echo "yaml upsert failed"; cat "`$yaml_file"; exit 1; }

env_failure_path="`$TRIM_PKGETC/env-write-failure"
yaml_failure_path="`$TRIM_PKGETC/yaml-write-failure"
mkdir -p "`$env_failure_path" "`$yaml_failure_path"
if upsert_env_value "`$env_failure_path" FLARE_USER should-fail 2>/dev/null; then
    echo "upsert_env_value masked a directory write failure"
    exit 1
fi
if upsert_yaml_value "`$yaml_failure_path" LoginUser should-fail 2>/dev/null; then
    echo "upsert_yaml_value masked a directory write failure"
    exit 1
fi

original_etc_dir="`$ETC_DIR"
original_config_file="`$CONFIG_FILE"
original_config_lock_file="`$CONFIG_LOCK_FILE"
ETC_DIR="`$TRIM_PKGETC/sync-write-failure"
CONFIG_FILE="`$ETC_DIR/config.yml"
CONFIG_LOCK_FILE="`$ETC_DIR/.superflare-config.lock"
mkdir -p "`$ETC_DIR/.env"
cat >"`$CONFIG_FILE" <<'EOF'
LoginUser: 'old-user'
LoginPass: 'old-pass'
EOF
if sync_login_config should-fail should-fail 2>/dev/null; then
    echo "sync_login_config masked a required credential write failure"
    exit 1
fi
ETC_DIR="`$original_etc_dir"
CONFIG_FILE="`$original_config_file"
CONFIG_LOCK_FILE="`$original_config_lock_file"

ensure_key_marker="`$TRIM_PKGETC/ensure-key-write-marker"
(
    read_env_value() { return 71; }
    upsert_env_value() { : > "`$ensure_key_marker"; return 0; }
    if ensure_env_key "`$env_file" FLARE_USER should-not-write; then
        echo "ensure_env_key masked a required read failure"
        exit 1
    fi
    [ ! -e "`$ensure_key_marker" ] || { echo "ensure_env_key wrote a default after a read failure"; exit 1; }
)

cookie_read_dir="`$TRIM_PKGETC/cookie-read-failure"
cookie_read_marker="`$TRIM_PKGETC/cookie-read-write-marker"
mkdir -p "`$cookie_read_dir"
printf '%s\n' 'FLARE_COOKIE_SECRET=existing-secret' > "`$cookie_read_dir/.env"
(
    ETC_DIR="`$cookie_read_dir"
    ensure_env_key() { return 0; }
    read_env_value() { return 72; }
    upsert_env_value() { : > "`$cookie_read_marker"; return 0; }
    if ensure_env_file; then
        echo "ensure_env_file masked the Cookie-secret read failure"
        exit 1
    fi
    [ ! -e "`$cookie_read_marker" ] || { echo "ensure_env_file wrote a Cookie secret after a read failure"; exit 1; }
)

login_env_marker="`$TRIM_PKGETC/login-env-write-marker"
(
    read_env_value() { return 75; }
    upsert_env_value() { : > "`$login_env_marker"; return 0; }
    if ensure_login_env_defaults; then
        echo "ensure_login_env_defaults masked a required read failure"
        exit 1
    fi
    [ ! -e "`$login_env_marker" ] || { echo "ensure_login_env_defaults wrote defaults after a read failure"; exit 1; }
)

login_config_marker="`$TRIM_PKGETC/login-config-write-marker"
(
    read_yaml_value() { return 76; }
    upsert_yaml_value() { : > "`$login_config_marker"; return 0; }
    if ensure_login_config_defaults; then
        echo "ensure_login_config_defaults masked a required read failure"
        exit 1
    fi
    [ ! -e "`$login_config_marker" ] || { echo "ensure_login_config_defaults wrote defaults after a read failure"; exit 1; }
)

cat >"`$env_file" <<'EOF'
FLARE_USER=custom-user
FLARE_PASS=
EOF
ensure_login_env_defaults
grep -Eq '^FLARE_USER=custom-user`$' "`$env_file" || { echo "env blank password should preserve the existing user"; cat "`$env_file"; exit 1; }
grep -Eq '^FLARE_PASS=admin`$' "`$env_file" || { echo "env blank password should initialize pass to admin"; cat "`$env_file"; exit 1; }

cat >"`$yaml_file" <<'EOF'
LoginUser: 'custom-user'
LoginPass: ''
Title: "SuperFlare"
EOF
ensure_login_config_defaults
grep -Eq "^LoginUser: 'custom-user'`$" "`$yaml_file" || { echo "yaml blank password should preserve the existing user"; cat "`$yaml_file"; exit 1; }
grep -Eq "^LoginPass: 'admin'`$" "`$yaml_file" || { echo "yaml blank password should initialize pass to admin"; cat "`$yaml_file"; exit 1; }

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
