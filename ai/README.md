# SuperFlare AI Rules

These notes are for AI agents and automation working in this repository.

## Priorities

1. Treat current code behavior as the source of truth.
2. Prefer native runtime and native debugging.
3. Keep changes small, direct, and easy to verify.
4. Preserve performance-sensitive paths unless there is a clear reason to change them.

## Working Style

- Reuse existing structure before adding new abstractions.
- Prefer plain functions and simple data flow over unnecessary indirection.
- Avoid broad refactors unless they are required to finish the task safely.
- Keep comments short and only where intent is not obvious.

## Generated Assets

- If templates, embedded assets, icons, or styles change, run:

```bash
go run build/build.go
```

## Verification

Run these checks from the repository root when code changes are complete:

```bash
go test ./... -count=1
go build ./...
```

## Local Runtime Data

These files are local runtime data and should not be treated as stable source artifacts:

- `config.yml`
- `apps.yml`
- `bookmarks.yml`
- `ports.yaml`
- `var/`
