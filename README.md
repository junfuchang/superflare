# SuperFlare

SuperFlare is a lean fork of `soulteary/docker-flare` focused on native Go runtime and long-term maintainability.

This repository keeps only the code, assets, and minimal instructions needed for development and operation. Docker packaging, screenshots, historical branch notes, and loosely related documents are intentionally removed from the mainline.

## Positioning

- Default runtime: native binary
- Primary development mode: local native debugging
- Deployment direction: adapt for fnOS / Feiniu NAS later, but not as a Docker-first project
- Source of truth: current code and current configuration model

## Quick Start

Requirements:

- Go `1.26.x`

Commands:

```bash
go run build/build.go
go run .
```

Validation:

```bash
go test ./... -count=1
go build ./...
```

## Runtime Files

SuperFlare reads these local runtime files from the repository root:

- `config.yml`
- `apps.yml`
- `bookmarks.yml`
- `ports.yaml`

Local runtime state is stored under `var/`.

## Repository Layout

```text
build/      asset generation helpers
cmd/        startup flags and runtime entry helpers
config/     config models and defaults
embed/      source templates and static assets
internal/   application code
ai/         AI-facing repository rules
```

## Notes

- Re-run `go run build/build.go` after changing embedded assets or templates.
- Site/app icons can use a custom image URL, a built-in MDI icon name, or automatic favicon fallback.
- Login credentials can be supplied from configuration.

## AI Rules

AI-facing maintenance rules live in [ai/README.md](./ai/README.md).
