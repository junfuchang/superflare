# Verified Hosted Favicon Recovery Design

Date: 2026-07-18
Status: Approved for implementation

## Problem

The origin-only favicon policy introduced in commit `c574cb8` correctly
removed synthetic first-letter icons, but it also removed the only recovery
path for valid sites that the SuperFlare runtime cannot reach directly.

The regression is reproducible for `wallroom.io` and `wallhaven.cc`:

- `/assets/site-icons` spends four seconds on the direct icon request and up
  to another four seconds on HTML discovery, then returns the built-in SVG.
- The sites publish valid 1150-byte and 1354-byte ICO files respectively.
- A system HTTP client can retrieve those files, while the Go transport in the
  current runtime cannot establish the origin connection.
- The failure is therefore network-path dependent, not an ICO decoder or HTML
  discovery defect.

Network failure must still fall back safely, but a hosted recovery service may
be used when it can distinguish a real favicon from its own default image.

## Evidence and Provider Choice

Three recovery approaches were compared:

1. `favicon.im` with `throw-error-on-404=true` returns the exact origin ICO
   bytes for both affected sites. It returns HTTP 404 and
   `X-Favicon-Source: default` for a nonexistent domain. Successful responses
   identify their source as `origin`, `cache-fresh`, or `cache-stale`.
2. Icon Horse returns the same real bytes for the affected sites, but also
   returns HTTP 200 generated letter tiles. Distinguishing those tiles depends
   on undocumented cache headers and is not a durable contract.
3. Remaining origin-only preserves privacy but cannot restore icons in
   runtimes whose direct route is unavailable.

The approved design uses `favicon.im` as a fail-closed hosted recovery path.
Live testing through SuperFlare's Go runtime confirmed that it can retrieve the
two affected icons and that a nonexistent host remains on the built-in SVG.

## Goals

- Restore real favicons when the destination origin is temporarily or
  permanently unreachable from SuperFlare.
- Never accept a provider-generated default, letter tile, or unknown source.
- Keep direct `/favicon.ico` and HTML-declared icons as preferred sources.
- Preserve the built-in bookmark SVG for provider outage, rate limiting, 404,
  missing or unknown provenance, invalid content, and all other failures.
- Keep behavior platform-neutral across Windows, Linux, Docker, and fnapp.
- Preserve the stable public `/assets/site-icons?src=...` URL without a version
  parameter.
- Preserve existing size, decode, cooldown, coalescing, and persistent-cache
  limits.

## Non-Goals

- Reading Windows system proxy or PAC settings.
- Guaranteeing icon availability when neither origin nor provider is
  reachable.
- Adding a configurable provider or configuration migration.
- Accepting a provider result based only on image validity.
- Sending local, private, IP-address, reserved, or NXDOMAIN bookmark hosts to
  a third party.

## Retrieval Flow

For a root HTTP(S) favicon URL:

1. Read the validated persistent cache.
2. Fetch the destination's `/favicon.ico` directly.
3. If the direct failure is a definitive NXDOMAIN, stop and use the built-in
   fallback without contacting a provider or the same host again.
4. If the direct failure is a network error or timeout, try verified hosted
   recovery before spending the remaining overall timeout on the same origin's
   HTML page.
5. Otherwise inspect the bounded HTML prefix and download a declared icon.
6. If HTML discovery or the declared icon fails and hosted recovery has not
   yet been attempted, try it once.
7. If no path returns a valid real icon, return an error to the existing route,
   which serves the built-in bookmark SVG with `no-store`.

The existing eight-second overall context remains authoritative. The existing
four-second HTTP client timeout bounds each individual request. The early
provider attempt for network errors is important: requesting the same
unreachable origin page first can exhaust the entire budget.

## Provider Request

For a public hostname `example.com`, SuperFlare constructs:

```text
https://favicon.im/example.com?throw-error-on-404=true
```

The provider receives only a normalized hostname. Provider recovery is
disabled for:

- localhost and single-label hosts;
- private or literal IP addresses;
- `.local`, `.internal`, `.onion`, `.alt`, `.invalid`, `.test`, and `.example`
  names;
- `example.com`, `example.net`, `example.org`, and their subdomains;
- definitive NXDOMAIN errors.

No bookmark path, query, fragment, credentials, description, or user data is
sent.

## Provenance Validation

A hosted response is accepted only when all of these checks pass:

- HTTP status is in the 2xx range.
- The final URL remains HTTPS on exactly `favicon.im` or `a.favicon.im`.
- The final path and `throw-error-on-404=true` query still identify the
  originally requested hostname.
- `X-Favicon-Source`, after trimming and lowercasing, is exactly `origin`,
  `cache-fresh`, or `cache-stale`.
- The body passes the existing byte limit, decoded-pixel limit, ICO structure,
  SVG-document, and supported-image validation.

`default`, blank, malformed, and unknown source values are rejected even when
the body is a valid image. Header or API drift therefore fails closed to the
built-in bookmark icon.

## Redirect Security

Ordinary origin favicon redirects retain the existing behavior. Hosted
requests use a cloned HTTP client with a provider-specific redirect validator:

- at most five redirects;
- HTTPS only;
- exact host allowlist: `favicon.im`, `a.favicon.im`;
- no userinfo or non-default port;
- unchanged escaped path and query relative to the initial provider request.

A compromised provider response therefore cannot redirect the favicon fetcher
to a local-network address or unrelated host.

## Cache Compatibility

The internal disk cache generation changes to
`2026-07-verified-hosted`. Files from both the provider-capable
`2026-07-nxdomain` generation and the origin-only
`2026-07-origin-only` generation are ignored.

The browser repair marker changes to
`superflare.site-icon.verified-hosted:`. This remains local storage metadata,
not a public URL parameter. A grouped `cache: "reload"` request updates any
old browser response once per source, then normal cached rendering resumes.

No YAML, JSON, CSV, bookmark, or settings schema changes are required.

## Failure and Cooldown Behavior

- A rejected provider response participates in the same aggregate fetch error
  as direct and HTML failures.
- Only the final failed retrieval chain records the existing five-minute
  negative-cache entry.
- Provider failure bytes are never written to disk.
- Successful verified provider bytes are written atomically under the original
  destination favicon key, not under the provider URL.
- Concurrent requests keep using the existing per-source in-flight
  coalescing and global fetch limit.

## Verification

Automated tests will prove:

- public-domain provider URL construction and local/reserved suppression;
- accepted source values and rejection of default, blank, and unknown values;
- provider redirect acceptance only between the two approved HTTPS hosts with
  unchanged request identity;
- a direct network failure uses verified hosted recovery before HTML;
- an ordinary origin 404 still prefers a successfully discovered HTML icon;
- verified provider success is cached under the destination key;
- provider 404, default, missing provenance, unknown provenance, invalid image,
  and redirect escape all use the built-in route fallback and create no cache;
- NXDOMAIN never contacts the provider;
- the immediately previous cache generation is ignored;
- site-icon URLs remain free of `v` parameters;
- full tests, build, touched-package vet, formatting, and diff checks pass.

Live verification will request `wallroom.io`, `wallhaven.cc`, and a guaranteed
nonexistent domain through a freshly built SuperFlare process. The first two
must return `cached` with their real ICO sizes; the nonexistent domain must
return `fallback`, `image/svg+xml`, and `no-store`.
