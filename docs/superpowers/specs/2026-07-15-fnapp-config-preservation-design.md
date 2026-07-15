# fnapp Configuration Preservation Design

## Incident

The current fnOS `install_callback` runs `reset_runtime_defaults_for_install` after preparing the runtime layout. That reset copies package defaults over `config.yml`, `apps.yml`, `bookmarks.yml`, and `ports.yaml`, deletes `.env`, and then recreates login and Cookie settings. A repeated install or recovery callback therefore reports success while replacing all persisted configuration in `TRIM_PKGETC`.

This violates the fnOS lifecycle contract. fnOS defines `etc` as application configuration storage and documents that install, upgrade, and configuration flows may be re-executed during development and recovery, so lifecycle scripts must be idempotent.

## Safety Invariant

The persisted files below are user-owned data:

- `TRIM_PKGETC/config.yml`
- `TRIM_PKGETC/apps.yml`
- `TRIM_PKGETC/bookmarks.yml`
- `TRIM_PKGETC/ports.yaml`
- `TRIM_PKGETC/.env`

If any of these files exists when an install callback begins, the callback must preserve every existing file and must not apply installation-wizard defaults or credentials. Missing files may be initialized from package defaults so a partial installation remains repairable.

An upgrade callback must also preserve existing values. In particular, runtime layout preparation may add a missing `FLARE_PORT`, but it must not replace an existing `FLARE_PORT` value.

Fresh installation remains unchanged: when no persisted configuration exists, package defaults are copied and the installation wizard's username and password are applied.

## Selected Design

Add `has_existing_runtime_config` to the shared lifecycle helpers. `install_callback` records this state before `ensure_runtime_layout` creates missing files. If configuration already existed, it exits successfully after layout repair without resetting files or syncing wizard credentials.

Delete the overwrite-only helpers and install reset path. Change `.env` port initialization from unconditional upsert to missing-key initialization.

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
- a fresh install still copies every default and applies wizard credentials;
- no test touches repository or NAS configuration.

Run this test from both the repository script suite and `build-superflare-fpk.ps1`, so a package cannot be produced when configuration preservation regresses.

## Compatibility

No configuration schema, manifest field, route, application data format, or fnOS directory changes. Existing `etc` and `var` locations remain authoritative. The user's in-progress manifest version change is preserved and is not part of this fix.
