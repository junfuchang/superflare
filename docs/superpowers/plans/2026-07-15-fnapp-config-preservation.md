# fnapp Configuration Preservation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make fnOS install, recovery, and upgrade lifecycle callbacks preserve all existing SuperFlare configuration while retaining fresh-install initialization.

**Architecture:** Treat `TRIM_PKGETC` configuration as user-owned state. Detect existing configuration before layout initialization, remove the overwrite/reset path, initialize only missing files or keys, and guard the behavior with a real callback integration test that also runs during FPK packaging.

**Tech Stack:** Bash lifecycle scripts, Windows PowerShell 5.1 test/build wrappers, fnOS `TRIM_*` lifecycle environment, Git Bash, fnpack 1.2.0.

## Global Constraints

- Preserve existing `.env`, `config.yml`, `apps.yml`, `bookmarks.yml`, and `ports.yaml` values during repeated install and upgrade callbacks; complete existing files must remain byte-for-byte unchanged.
- Treat real protected files in either current `etc` storage or legacy `var/runtime` storage as persisted configuration when deciding whether installation-wizard credentials may be applied.
- Initialize missing files and keys so fresh and partial installations remain repairable.
- Preserve conflicting legacy files and any real `etc/var` path instead of recursively deleting them; uninstall must obey the same boundary.
- Never traverse a symlinked legacy root, unlink an unmanaged `etc/var` symlink, or report lifecycle success after a required configuration write failed.
- Do not change configuration schemas, fnOS paths, manifest fields, runtime `var` data, or the user's uncommitted manifest version change.
- Add the preservation test to both repository script checks and the FPK build gate.
- Work on local `main`, commit locally, and do not push.

---

### Task 1: Add a real lifecycle preservation regression test

**Files:**
- Create: `fnapp/test-lifecycle-config-preservation.ps1`

**Interfaces:**
- Consumes: `fnapp/superflare/cmd/install_callback` and `upgrade_callback` through temporary `TRIM_APPDEST`, `TRIM_PKGETC`, and `TRIM_PKGVAR` directories.
- Produces: a standalone PowerShell test that exits successfully only when existing files are unchanged and fresh initialization still works.

- [x] **Step 1: Create isolated callback scenarios**

Implement helpers with these signatures:

```powershell
function Resolve-GitBash
function To-ShPath([string]$Path)
function Write-Utf8NoBom([string]$Path, [string]$Content)
function New-LifecycleScenario([string]$Name, [bool]$WithExistingConfig)
function Invoke-LifecycleCallback([string]$ScriptPath, [string]$ScenarioRoot, [hashtable]$WizardValues)
function Get-ConfigHashes([string]$EtcRoot)
function Assert-HashesEqual([hashtable]$Expected, [hashtable]$Actual, [string]$Context)
function Find-PreservedLinkPath([string]$EtcRoot)
```

Each scenario creates package defaults under `app/server/defaults`. Existing scenarios create all five user files with unique sentinels and `FLARE_PORT=9443`. `Invoke-LifecycleCallback` sets `TRIM_SERVICE_PORT=3636` so the upgrade test detects port replacement.

- [x] **Step 2: Assert repeated install, upgrade, and fresh install behavior**

The test must execute these cases in order:

```powershell
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
```

For the fresh case, require all five files and assert `fresh-user` / `fresh-pass` in both `.env` and `config.yml` while package default application, bookmark, and port sentinels remain present.

Add isolated callback scenarios proving that:

- a partial current-layout install retains custom existing values, initializes missing credentials/files/default keys, and suppresses replacement wizard credentials;
- partial legacy-only configuration is recognized before migration, is moved into `etc`, initializes missing files, and suppresses replacement wizard credentials;
- an `export FLARE_COOKIE_SECRET = "custom-secret"` entry is recognized rather than replaced;
- when `etc/bookmarks.yml` and `var/runtime/bookmarks.yml` contain different sentinels, `upgrade_callback` preserves both files byte-for-byte;
- when `etc/var` is a real directory containing a marker and `var.pre-superflare-link` already contains a different marker, `upgrade_callback` leaves the existing sibling intact, retains `etc/var` under the first free `.N` suffix, and recreates the managed `etc/var` path;
- `uninstall_callback` preserves a real `etc/var` marker and a legacy runtime configuration marker.

- [x] **Step 3: Run the regression test and verify RED**

Run:

```powershell
& .\fnapp\test-lifecycle-config-preservation.ps1
```

Expected: FAIL in `repeated install` because every file is overwritten. If install were bypassed, `upgrade` must also fail because `ensure_env_file` replaces `FLARE_PORT=9443` with `3636`. The legacy conflict, real `etc/var`, and uninstall cases must fail because the current callbacks recursively delete their markers.

---

### Task 2: Make lifecycle initialization idempotent

**Files:**
- Modify: `fnapp/superflare/cmd/common.sh:102-120,300-323,389-485`
- Modify: `fnapp/superflare/cmd/install_callback:15-35`
- Modify: `fnapp/superflare/cmd/config_callback:49`
- Modify: `fnapp/superflare/cmd/uninstall_callback:9-11`
- Test: `fnapp/test-lifecycle-config-preservation.ps1`
- Test: `fnapp/test-common-script.ps1`

**Interfaces:**
- Produces: `has_existing_runtime_config() -> shell success when any protected file exists`.
- Preserves: `ensure_runtime_layout()` still creates directories, migrates legacy files, copies missing defaults, and repairs runtime links.

- [x] **Step 1: Add existing-configuration detection**

Add to `common.sh`, checking both `ETC_DIR` and real entries under `LEGACY_RUNTIME_DIR`:

```bash
has_existing_runtime_config() {
    local name
    for name in .env config.yml apps.yml bookmarks.yml ports.yaml; do
        if [ -e "${ETC_DIR}/${name}" ] || [ -L "${ETC_DIR}/${name}" ] ||
           { [ -e "${LEGACY_RUNTIME_DIR}/${name}" ] && [ ! -L "${LEGACY_RUNTIME_DIR}/${name}" ]; }; then
            return 0
        fi
    done
    return 1
}
```

- [x] **Step 2: Remove destructive reset helpers**

Delete `overwrite_default_file`, `reset_runtime_defaults_for_install_locked`, and `reset_runtime_defaults_for_install`. No lifecycle path may copy a package default over an existing protected file or remove an existing `.env`.

- [x] **Step 3: Preserve an existing environment port**

In `ensure_env_file`, replace:

```bash
upsert_env_value "${env_file}" "FLARE_PORT" "${TRIM_SERVICE_PORT:-3636}"
```

with:

```bash
ensure_env_key "${env_file}" "FLARE_PORT" "${TRIM_SERVICE_PORT:-3636}"
```

Use `read_env_value` instead of an exact-format `awk` expression when checking whether `FLARE_COOKIE_SECRET` exists. Change `ensure_login_env_defaults` and `ensure_login_config_defaults` to initialize the username and password independently, so a missing partner never resets an existing credential value.

Update `test-common-script.ps1` to assert the same independent initialization contract: a custom username survives when only its password is blank, while the blank password is initialized to `admin`.

Make `upsert_env_value`, `upsert_yaml_value`, `ensure_env_file`, both login-default helpers, and `sync_login_config_locked` return nonzero on every required read/write failure. Temporary-file cleanup must not replace the failed operation's status. Reject a dangling `.env` symlink before the initial heredoc write, and make both `install_callback` and `config_callback` fail through `log_and_fail` when `sync_login_config` fails.

- [x] **Step 4: Make `install_callback` recovery-safe**

Capture state before layout initialization and bypass wizard writes for existing data:

```bash
had_existing_config=false
if has_existing_runtime_config; then
    had_existing_config=true
fi

ensure_runtime_layout || log_and_fail "Failed to prepare runtime layout after install."

if [ "${had_existing_config}" = "true" ]; then
    log_info "Existing SuperFlare configuration preserved during install callback."
    exit 0
fi
```

Keep the current wizard validation and `sync_login_config` path for a genuinely fresh install.

- [x] **Step 5: Preserve legacy conflicts and real link paths**

Change legacy detection, migration, and cleanup so `LEGACY_RUNTIME_DIR` is checked for `-L` before any child path is probed. A real source is moved only when its destination is absent. If both exist, retain the legacy file byte-for-byte and log the conflict. Cleanup may remove obsolete symbolic links and an empty real directory, but never traverse a symlinked root or remove a real protected file.

Change `relink_path` so an obsolete symbolic link is unlinked, while a real file or directory is atomically renamed to the first free sibling in this sequence before link creation:

```text
var.pre-superflare-link
var.pre-superflare-link.1
var.pre-superflare-link.2
...
```

If preservation or link creation fails, return failure without deleting the preserved path.

- [x] **Step 6: Make uninstall non-destructive**

Remove recursive deletion from `uninstall_callback`. Unlink `ETC_DIR/var` only when it is a symbolic link whose `readlink` value exactly identifies `VAR_DIR`; preserve and log all other links. Leave a real `ETC_DIR/var`, all `var.pre-superflare-link[.N]` siblings, and `LEGACY_RUNTIME_DIR` untouched.

- [x] **Step 7: Run the focused test and verify GREEN**

Run:

```powershell
& .\fnapp\test-lifecycle-config-preservation.ps1
```

Expected: PASS for repeated install, upgrade, fresh install, legacy migration/conflict preservation, real link-path preservation, and non-destructive uninstall.

---

### Task 3: Gate packaging and document the guarantee

**Files:**
- Modify: `fnapp/build-superflare-fpk.ps1`
- Modify: `tools/test-all-scripts.ps1`
- Modify: `fnapp/README.md`
- Test: `fnapp/test-lifecycle-config-preservation.ps1`

**Interfaces:**
- Consumes: the standalone lifecycle preservation test from Task 1.
- Produces: package and repository checks that stop on any preservation failure.

- [x] **Step 1: Run the lifecycle test during FPK packaging**

Before syncing defaults in `build-superflare-fpk.ps1`, add:

```powershell
Write-Host "Testing fnapp configuration preservation..."
& (Join-Path $repoRoot "fnapp\test-lifecycle-config-preservation.ps1")
if (-not $?) {
    throw "fnapp configuration preservation test failed."
}
```

- [x] **Step 2: Run the lifecycle test in repository script checks**

Add `Test-FnappLifecycleConfigPreservation` to `tools/test-all-scripts.ps1`; it invokes the test script and throws on failure. Call it before the final success message.

- [x] **Step 3: Update fnapp documentation**

Document that install/recovery/upgrade callbacks preserve all five existing files, legacy conflicts, and real link paths; defaults only fill missing files; wizard credentials apply only to fresh installs; uninstall does not delete persisted configuration; and FPK builds run the destructive-update regression test.

- [x] **Step 4: Run full verification**

Run:

```powershell
& .\fnapp\test-lifecycle-config-preservation.ps1
& .\fnapp\test-common-script.ps1
& .\fnapp\test-config-init.ps1
& .\fnapp\test-config-callback.ps1
& .\tools\test-script-safety.ps1
& .\tools\test-all-scripts.ps1
.\.tools\go\bin\go.exe test ./... -count=1
.\.tools\go\bin\go.exe build ./...
git diff --check
```

Then run `fnapp/build-superflare-fpk.ps1`, verify its lifecycle gate passes, confirm `superflare.fpk` is produced, and inspect the package source/runtime directories to ensure no user configuration is embedded outside `app/server/defaults`.

Verification completed on 2026-07-15. The focused fnapp tests, repository script suites, `go test ./... -count=1`, `go build ./...`, and the real FPK build passed. The first packaging run exposed a pre-existing race in the repository-wide Go source scan when ignored runtime YAML files disappeared concurrently; commits `b96aa11` and `0e573db` added focused coverage and limited the scanner exception to missing known runtime configuration artifacts, while missing source directories, Go files, unrelated files, and other errors still fail. The successful package is version `0.0.2`, SHA-256 `CC8AB8CFBA46F87B146AD5863CE1502A9699612FE43AAAC6BE1DBDBEC13E87F7`, and contains configuration only under `server/defaults/` inside `app.tgz`.

- [x] **Step 5: Commit locally**

Stage only the fix, tests, generated package binary if changed, documentation, and this plan. Do not stage `fnapp/superflare/manifest`; it is a user-owned version change.

```powershell
git commit -m "fix: preserve fnapp config during updates"
```
