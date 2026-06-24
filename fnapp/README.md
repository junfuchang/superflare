# SuperFlare fnOS Native Package

This directory contains the native fnOS packaging assets for SuperFlare.

## Packaging Model

- Package root: `fnapp/superflare`
- Package type: fnOS native app
- Runtime model: native Go binary, not Docker
- Binary working directory: `var/runtime`

SuperFlare currently reads `config.yml`, `apps.yml`, `bookmarks.yml`, `ports.yaml`, `.env`, and `var/` from the current working directory. `.env` is only used for a small set of startup items such as login and cookie settings. fnOS separates immutable binaries from writable `etc/` and `var/`, so the lifecycle script bridges that gap by creating this runtime layout:

- `etc/` stores `.env`, `config.yml`, `apps.yml`, `bookmarks.yml`, `ports.yaml`
- `var/` stores uploads, cache, pid, log, and other mutable runtime data
- `var/runtime/` becomes the working directory
- `var/runtime/var -> var/`
- `var/runtime/*.yml` and `var/runtime/.env` are symlinked to `etc/`

That keeps the current codebase usable on fnOS without a broad path refactor.

## Build

From the repository root:

```powershell
powershell -ExecutionPolicy Bypass -File .\fnapp\build-superflare-fpk.ps1
```

The build script will:

1. run `go run build/build.go`
2. run `go test ./... -count=1`
3. cross-compile `superflare` for Linux `amd64`
4. copy the current root `config.yml`, `apps.yml`, `bookmarks.yml`, `ports.yaml` into package defaults
5. run `fnpack build`

Result:

- `fnapp/superflare/superflare.fpk`

## Runtime Defaults

- Port: `3636`
- Login: enabled
- Default credentials come from the packaged `config.yml` defaults, currently `admin / admin`
- Editor: enabled
- fnOS privilege: `run-as: root` so the ports page can read full system port owner names on NAS
- fnOS app settings: `系统设置 -> 应用设置` now supports editing `FLARE_USER` / `FLARE_PASS`, persisted into `etc/.env` and applied by automatic restart

## Notes

- This package intentionally targets native fnOS execution and does not depend on Docker.
- Existing `etc/` and `var/` data are preserved across upgrades because defaults are copied only when missing.
