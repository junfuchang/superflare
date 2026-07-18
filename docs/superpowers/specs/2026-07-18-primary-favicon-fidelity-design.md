# Primary Favicon Fidelity Design

Date: 2026-07-18
Status: Approved for implementation

## Problem

SuperFlare displays an opaque white-backed icon for `cupfox.love` even though
the page's primary `rel="icon"` PNG has transparency.

The complete data path proves that CSS and SuperFlare do not add the white
pixels:

- the page's first `rel="icon"` is an 8,365-byte 256x256 PNG with 45,814
  non-opaque pixels;
- the page's later `apple-touch-icon` is an opaque 4,770-byte 180x180 PNG;
- `favicon.im`, the persistent SuperFlare cache, and the public
  `/assets/site-icons` response are byte-for-byte equal to the opaque Apple
  icon;
- bookmark CSS leaves the image and its parent background transparent.

`favicon.im` proves that its result came from the destination, but it may
select a secondary Apple icon rather than the page's primary favicon. The Go
network path in the affected runtime cannot reach the destination HTML, so
changing HTML/provider ordering alone still returns the opaque provider image.

## Goals

- Preserve transparent primary favicons when the verified hosted baseline
  selected an opaque secondary icon.
- Never restore Icon Horse's generated first-letter fallback tiles.
- Preserve `favicon.im` as the fail-closed availability baseline for
  `wallhaven.cc`, `wallroom.io`, and other origins SuperFlare cannot reach.
- Keep the ten-second overall request bound, existing global concurrency
  ceiling, and sequential network behavior.
- Invalidate the current opaque server/browser cache once without adding a
  public version parameter.
- Keep behavior platform-neutral across Windows, Linux, Docker, and fnapp.
- Preserve existing validation, privacy, cooldown, coalescing, and atomic
  cache behavior.

## Non-Goals

- Site-specific hostname rules or hard-coded icon URLs.
- Rewriting opaque pixels or guessing a background color to remove.
- Treating arbitrary provider image validity as proof that an image is real.
- Contacting a hosted provider for local, private, reserved, IP-address, or
  definitive NXDOMAIN destinations.
- Changing bookmark, application, or settings schemas.
- Adding `v` or another cache-generation parameter to public site-icon URLs.

## Approaches Considered

### Cache Invalidation Only

This fixes the current cache only when the destination HTML happens to be
reachable. A later network failure can persist the same opaque hosted icon, so
the defect recurs.

### Concurrent HTML and Hosted Recovery

The implementation and tests proved deterministic HTML preference, but live
validation still returned the opaque icon because the Go runtime cannot reach
the destination HTML at all. The concurrent version also made one chain own
two active outbound requests and required join semantics beyond the existing
global concurrency contract. It is not retained.

### Fail-Closed Transparent Primary Refinement

Keep the existing verified `favicon.im` result as a trusted baseline. Only
when that baseline is a decodable raster with no non-opaque pixels, request one
Icon Horse candidate. Replace the baseline only when the candidate passes both
provider-source and pixel-fidelity checks.

Live evidence supports this approach:

- Icon Horse returns the exact 8,365-byte transparent primary PNG for
  `cupfox.love` and the exact origin ICO bytes for Wallhaven and Wallroom.
- A nonexistent domain returns an opaque generated first-letter tile.
- Real cached icons include exactly `CDN-Cache-Control: max-age=2592000` and a
  non-empty ETag; generated fallbacks omit both and use a short shared-cache
  lifetime.
- Icon Horse is reachable through the same Go transport that cannot reach the
  destination HTML.

The metadata is treated as a strict positive allowlist, not a heuristic. If it
is absent, duplicated, malformed, or changes in the future, refinement fails
closed and SuperFlare retains the already verified `favicon.im` baseline.

## Retrieval Flow

The ordinary root favicon flow remains:

1. Read the validated persistent cache.
2. Fetch the destination's `/favicon.ico` directly.
3. Stop on definitive NXDOMAIN without contacting any provider.
4. For network, TLS, EOF, or timeout failures, try verified `favicon.im`
   recovery before HTML, preserving its remaining timeout budget.
5. For other failures, try bounded HTML discovery and the declared icon before
   verified hosted recovery.
6. If all eligible paths fail, let the route serve the built-in bookmark SVG
   with `no-store`.

Every successful `favicon.im` result then passes through an optional quality
refinement:

1. Decode the already validated baseline and scan until a non-opaque pixel is
   found.
2. If transparency is present or cannot be determined, retain the baseline and
   make no additional request.
3. If the baseline is definitely fully opaque, construct
   `https://icon.horse/icon/<public-hostname>`.
4. Reject redirects; the final URL must remain HTTPS on exactly `icon.horse`,
   with the normalized destination hostname as its only escaped path segment
   and no query, port, or userinfo.
5. Require exactly one `CDN-Cache-Control` value equal to
   `max-age=2592000` and exactly one non-empty ETag.
6. Apply the existing status, byte, decoded-pixel, SVG/ICO, and supported-image
   validation to the body.
7. Decode the candidate and require at least one non-opaque pixel.
8. Use the candidate only when every check succeeds. Otherwise retain the
   verified baseline without turning a quality-refinement failure into a
   broken icon.

The two providers are contacted sequentially inside the existing chain-level
fetch slot. No new goroutines or additional concurrency slots are introduced.
Both receive only the normalized public hostname, never bookmark paths,
queries, fragments, credentials, descriptions, or local addresses.

## Cache Compatibility

The disk cache generation changes from `2026-07-verified-hosted` to
`2026-07-primary-favicon`. Existing files remain on disk but are ignored by the
generation-aware key.

The browser repair marker changes from
`superflare.site-icon.verified-hosted:` to
`superflare.site-icon.primary-favicon:`. The existing grouped
`fetch(src, {cache: "reload"})` replaces an old immutable browser response once
after the corrected server response decodes.

The public route remains exactly `/assets/site-icons?src=...`, with no `v`
parameter and no configuration migration.

## Failure and Security Behavior

- The trusted `favicon.im` baseline retains its current strict URL, redirect,
  and `X-Favicon-Source` validation.
- Icon Horse is never contacted unless that trusted baseline already proved
  the public destination has a real icon and the baseline is fully opaque.
- A generated or unverifiable Icon Horse body is never cached or exposed.
- A quality-refinement failure does not record a cooldown because the trusted
  baseline remains a successful result.
- Only the final selected bytes are written atomically under the original
  destination key.
- Existing in-flight coalescing and the eight-chain global fetch limit remain
  unchanged.

## Verification

Automated tests will prove:

- an opaque trusted baseline is replaced by a strictly validated transparent
  primary candidate;
- transparent trusted baselines never contact Icon Horse;
- short-cache first-letter fallback responses, missing/duplicate source
  headers, blank ETags, redirects, invalid bodies, and opaque candidates are
  rejected while retaining the trusted baseline;
- private, local, reserved, IP-address, and NXDOMAIN destinations are never
  sent to either provider;
- Wallhaven/Wallroom hosted recovery and the ten-second timeout remain intact;
- the previous server cache and browser marker generations are ignored;
- public site-icon URLs remain free of `v` parameters;
- focused tests, full tests, build, touched-package vet, formatting, and diff
  checks pass.

Live verification will use a freshly built server. `cupfox.love` must return
the 8,365-byte transparent PNG, Wallhaven and Wallroom must retain their valid
ICO responses, and a nonexistent destination must retain the built-in SVG
fallback.
