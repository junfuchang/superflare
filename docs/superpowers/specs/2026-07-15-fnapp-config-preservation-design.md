# fnapp Configuration Preservation Design

## Incident

The current fnOS `install_callback` runs `reset_runtime_defaults_for_install` after preparing the runtime layout. That reset copies package defaults over `config.yml`, `apps.yml`, `bookmarks.yml`, and `ports.yaml`, deletes `.env`, and then recreates login and Cookie settings. A repeated install or recovery callback therefore reports success while replacing all persisted configuration in `TRIM_PKGETC`.

This violates the fnOS lifecycle contract. fnOS defines `etc` as application configuration storage and documents that install, upgrade, and configuration flows may be re-executed during development and recovery, so lifecycle scripts must be idempotent.

The same audit found three adjacent destructive paths. `upgrade_callback` replaces an existing `FLARE_PORT`; legacy `var/runtime` configuration is deleted when the corresponding `etc` file already exists; and `relink_path` recursively deletes a real `etc/var` path before creating the runtime link. `uninstall_callback` repeats the last two deletion patterns. Each callback can exit successfully after removing operator-owned data.

## Safety Invariant

The persisted files below are user-owned data:

- `TRIM_PKGETC/config.yml`
- `TRIM_PKGETC/apps.yml`
- `TRIM_PKGETC/bookmarks.yml`
- `TRIM_PKGETC/ports.yaml`
- `TRIM_PKGETC/.env`

Configuration still stored in a real legacy `TRIM_PKGVAR/runtime` directory is also user-owned. When the same filename exists in both layouts, neither copy may be deleted automatically. A symlinked legacy directory is never traversed as configuration storage. A non-link `TRIM_PKGETC/var` path is likewise operator-owned and must be moved to a uniquely named sibling such as `var.pre-superflare-link` before the managed runtime link is created.

If any of these files exists when an install callback begins, the callback must preserve every existing file and must not apply installation-wizard defaults or credentials. Missing files may be initialized from package defaults so a partial installation remains repairable.

An upgrade callback must also preserve existing values. Runtime layout preparation may add a missing key, but it must never replace the existing half of a partial credential pair. In particular, it may add a missing `FLARE_PORT`, but it must not replace an existing `FLARE_PORT`; and it must recognize supported `export KEY = value` syntax when deciding whether a Cookie secret already exists.

Fresh installation remains unchanged: when no persisted configuration exists in either current or legacy storage, package defaults are copied and the installation wizard's username and password are applied.

## Selected Design

Add `has_existing_runtime_config` to the shared lifecycle helpers and make it inspect both current `etc` files and real legacy `var/runtime` files. `install_callback` records this state before `ensure_runtime_layout` creates or migrates files. If configuration already existed, it exits successfully after layout repair without resetting files or syncing wizard credentials.

Delete the overwrite-only helpers and install reset path. Change `.env` port initialization from unconditional upsert to missing-key initialization. Initialize missing login username and password keys independently, and use the shared environment parser when checking for an existing Cookie secret.

Make legacy cleanup conservative: migrate a real legacy file only when the current destination is absent, retain and log both copies when they conflict, remove only obsolete symbolic links, and remove the legacy directory only when it is genuinely empty. Replace recursive link-path deletion with a rename-to-preserve operation for any real file or directory; only an obsolete symbolic link may be unlinked in place. Uninstall verifies the symbolic-link target and removes only the managed `etc/var -> TRIM_PKGVAR` projection, never an operator-owned link or legacy/preserved configuration.

Every required configuration write must propagate failure to its callback. Temporary-file cleanup must not mask a failed replacement, and fresh installation must fail if wizard credentials cannot be synchronized. A dangling protected symlink is an error, not permission to create or replace its external target.

This makes install and upgrade callbacks idempotent at their configuration boundary while retaining the current default-file projection and legacy-layout migration.

## Alternatives

### Protect only `upgrade_callback`

Rejected because the destructive code is in `install_callback`, and fnOS explicitly allows installation flows to be replayed during recovery.

### Snapshot and restore every upgrade

Not selected for this fix. A second copy and restore protocol adds stale-backup, permissions, retention, and future migration concerns. fnOS already provides persistent `etc`; the direct overwrite must be removed instead. Existing SuperFlare backup and restore remains available as an independent recovery mechanism.

## Regression Protection

Add an isolated lifecycle test that invokes the real callbacks with temporary `TRIM_APPDEST`, `TRIM_PKGETC`, and `TRIM_PKGVAR` paths.

The test must prove:

- repeated `install_callback` preserves all five files byte-for-byte even when wizard credentials contain replacement values;
- `upgrade_callback` preserves all five files byte-for-byte even when `TRIM_SERVICE_PORT` differs from the saved `.env` value;
- partial credentials retain each existing username while only the missing password keys are initialized, and formatted existing Cookie secrets are not regenerated;
- a fresh install still copies every default and applies wizard credentials;
- legacy-only configuration is treated as an existing installation and is migrated without applying replacement wizard credentials;
- conflicting current and legacy configuration files both remain byte-for-byte intact;
- a real `etc/var` directory is retained under a unique `var.pre-superflare-link[.N]` name before link creation;
- uninstall never recursively deletes a real `etc/var` directory or legacy runtime configuration;
- a symlinked legacy root is not traversed for persisted-install detection, and an unmanaged `etc/var` link survives uninstall;
- failed configuration writes make the lifecycle callback fail instead of reporting false success;
- no test touches repository or NAS configuration.

Run this test from both the repository script suite and `build-superflare-fpk.ps1`, so a package cannot be produced when configuration preservation regresses.

## Compatibility

No configuration schema, manifest field, route, application data format, or fnOS directory changes. Existing `etc` and `var` locations remain authoritative. Preservation siblings are created only when a conflicting real `etc/var` path would otherwise be destroyed. The user's in-progress manifest version change is preserved and is not part of this fix.
