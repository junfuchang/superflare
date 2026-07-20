# README Refresh Design

## Goal

Refresh the repository root `README.md` so that it is a practical entry point for SuperFlare users and deployers. The document must describe the current product, direct readers to the supported deployment paths, explain which data must be persisted, and reflect the features added since the last README update.

## Audience

The primary audience is a person evaluating, installing, upgrading, or operating SuperFlare. Contributor details remain available, but they follow the user-facing material instead of driving the document structure.

## Content Structure

The README will use this order:

1. Project summary and current capabilities.
2. Feature groups covering navigation, access control, online editing, appearance, favicon recovery, port discovery, and multilingual support.
3. A deployment matrix for source/native startup, Docker, Linux, Windows, and fnOS, with concise commands or links to the platform-specific guides.
4. First-run and access guidance, including the default port, login behavior, editor and settings routes, and production Cookie secret advice.
5. Runtime data and upgrade safety, identifying the four YAML files, optional `.env`, and `var/` directory as persistent data.
6. Configuration examples for applications, bookmarks, private visibility, favorites, descriptions, categories, and subdirectories.
7. Development and verification commands, repository layout, and upstream acknowledgement.

## Source Of Truth

Every statement will be grounded in the current repository:

- `config/defaults/` and `config/model/` define runtime configuration and supported fields.
- `cmd/`, `internal/server/`, and page handlers define startup flags and routes.
- `tools/docker/`, `tools/linux/`, and `tools/windows/` define supported operating workflows.
- `fnapp/README.md` and fnOS lifecycle scripts define package behavior and configuration preservation guarantees.
- Recent implementation commits define favorites, private items, application folders, link checks, editor behavior, and favicon recovery.

The root README will link to detailed platform guides rather than copying all of their procedures. This keeps platform-specific instructions maintainable in one place.

## Compatibility And Safety

- Existing runtime configuration remains compatible; the README only documents fields already supported by the current loaders.
- Docker users will be told to persist `/data`.
- Native users will be told that configuration is resolved from the working directory.
- fnOS users will be told that upgrades preserve existing configuration and that package defaults only fill missing data.
- Default credentials will be documented together with an explicit instruction to change them and configure a stable random `FLARE_COOKIE_SECRET` when login is enabled.

## Validation

The completed change will be checked by:

- reviewing every README command and relative link against the repository;
- scanning the Markdown for stale paths and placeholder text;
- running `git diff --check`;
- running the full Go test suite because the README is embedded into the Docker build context and the final commit is intended for `main`;
- verifying the pushed GitHub `main` reference matches the local commit.

## Out Of Scope

- No application behavior, configuration defaults, packaging scripts, or runtime files will change.
- No new screenshots, badges, release artifacts, or external services will be introduced.
- Existing user changes in `fnapp/superflare/manifest` and `tools/superflare-icon.zip` will remain untouched and uncommitted.
