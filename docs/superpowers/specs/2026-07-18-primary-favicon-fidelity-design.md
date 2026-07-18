# Primary Favicon Fidelity Design

Date: 2026-07-18
Status: Approved for implementation

## Problem

SuperFlare displays an opaque white-backed icon for `cupfox.love` even though
the page's primary `rel="icon"` PNG has transparency.

The complete data path shows that SuperFlare does not add the white pixels:

- the page's first `rel="icon"` is a 256x256 PNG with 45,814 non-opaque
  pixels;
- the page's later `apple-touch-icon` is an opaque 180x180 PNG;
- `favicon.im`, the persistent SuperFlare cache, and the public
  `/assets/site-icons` response are byte-for-byte equal to that opaque Apple
  icon;
- bookmark CSS leaves both the image and its parent background transparent.

The cache entry was created after the direct root favicon request followed the
network-error recovery path. That path returns a verified hosted result before
attempting HTML discovery. Provider provenance proves that an image came from
the site, but it does not prove that the provider selected the page's primary
favicon or preserved its alpha channel.

## Goals

- Prefer a valid page-declared primary favicon over a verified hosted result,
  including after direct network and TLS failures.
- Preserve the existing verified hosted recovery for origins that SuperFlare
  cannot reach, including `wallhaven.cc` and `wallroom.io`.
- Keep the existing ten-second request bound imposed by the server's write
  timeout.
- Ignore opaque provider-derived cache entries written by the previous
  generation and repair the stable browser URL once without adding a public
  version parameter.
- Keep behavior platform-neutral across Windows, Linux, Docker, and fnapp.
- Preserve all existing byte, decoded-pixel, redirect, privacy, concurrency,
  cooldown, and atomic-cache limits.

## Non-Goals

- Detecting transparency loss with per-site rules, OCR, color thresholds, or
  image-comparison heuristics.
- Rewriting an opaque image to manufacture transparent pixels.
- Contacting a hosted provider for local, private, reserved, IP-address, or
  definitive NXDOMAIN destinations.
- Changing bookmark or settings schemas.
- Adding `v` or any other cache-generation parameter to public site-icon URLs.

## Approaches Considered

### Cache Invalidation Only

Bumping the cache generation would fix the currently reachable
`cupfox.love`, because its ordinary 404 path now discovers the transparent
HTML icon. A later transient network failure could select and persist the
opaque hosted image again, so this does not address the root cause.

### Fully Sequential Origin-First Retrieval

Always running direct favicon, HTML page, declared icon, and hosted recovery
in sequence gives deterministic source preference. Their existing per-request
budgets require up to 18 seconds, while the server has a ten-second write
timeout. Increasing only the favicon context would therefore produce broken
responses; increasing the global server timeout would broaden this narrowly
scoped change.

### Concurrent Recovery with Deterministic HTML Preference

After a direct network-class failure, start HTML discovery and verified hosted
recovery concurrently. Retain an early hosted success in memory while waiting
for the HTML branch. Return the HTML-declared icon whenever that branch
succeeds within the existing overall deadline. Return the retained hosted icon
only after the HTML branch fails or the deadline expires.

This approach is selected because it reserves the provider's full remaining
budget without allowing response timing to override source quality.

## Retrieval Flow

For a root HTTP(S) favicon URL:

1. Read the validated current-generation persistent cache.
2. Fetch the destination's `/favicon.ico` directly.
3. Stop immediately on definitive NXDOMAIN without contacting HTML or a
   provider.
4. For a network, TLS, EOF, or timeout failure, start two bounded branches:
   HTML discovery plus declared-icon download, and verified hosted recovery.
5. If the HTML branch returns a valid icon, cancel the hosted branch and use
   the HTML bytes regardless of which branch completed first.
6. If the HTML branch fails, use an already successful hosted result or wait
   for the hosted branch within the existing overall context.
7. If the overall context ends after hosted success but before HTML success,
   use the retained hosted result. No new work continues after return.
8. For non-network direct failures, preserve the existing sequential order:
   HTML discovery first, then one hosted attempt.
9. If all eligible paths fail, return the existing aggregate error so the
   route serves the built-in bookmark SVG with `no-store`.

Each recovery goroutine writes to a capacity-one result channel and shares a
cancelable child context. The coordinator is the only owner of result
selection, so a hosted success cannot be cached while a successful HTML result
is available. Cancellation and buffered delivery prevent blocked senders or
orphaned request work.

## Cache Compatibility

The disk cache generation changes from `2026-07-verified-hosted` to
`2026-07-primary-favicon`. Existing files remain on disk but are ignored by the
generation-aware key.

The browser repair marker changes from
`superflare.site-icon.verified-hosted:` to
`superflare.site-icon.primary-favicon:`. The existing grouped
`fetch(src, {cache: "reload"})` then replaces an old immutable browser response
once per source after the new server response decodes successfully.

The public route remains exactly `/assets/site-icons?src=...`, with no `v`
parameter or schema migration.

## Error and Security Behavior

- Both branches retain the same source validation and redirect policies.
- Provider responses still require exactly one trusted
  `X-Favicon-Source` value and an approved final provider URL.
- A provider failure is retained in the aggregate error when HTML also fails.
- A successful HTML result suppresses provider errors and is the only result
  written to the destination cache.
- Only a final failed chain records the existing five-minute cooldown.
- Existing in-flight coalescing and global fetch/decode limits remain in
  force; the two branch requests still share the caller's overall context.

## Verification

Automated tests will prove:

- after a direct network failure, a transparent HTML-declared PNG wins over an
  earlier successful opaque hosted PNG;
- the hosted and HTML branches are both started for eligible network recovery,
  while selection remains deterministic rather than completion-order based;
- hosted recovery still succeeds when the HTML branch fails;
- NXDOMAIN and non-public destinations retain their privacy behavior;
- non-network direct failures retain HTML-first sequential ordering;
- the previous cache generation is ignored;
- the browser repair prefix changes while public URLs remain free of `v`;
- focused tests, full tests, build, touched-package vet, formatting, and diff
  checks pass.

Rendered verification will use a freshly built local server and confirm that
`cupfox.love` resolves to the 8,365-byte transparent primary PNG, while
`wallhaven.cc` and `wallroom.io` still return valid cached icons and a
nonexistent destination still returns the built-in fallback.
