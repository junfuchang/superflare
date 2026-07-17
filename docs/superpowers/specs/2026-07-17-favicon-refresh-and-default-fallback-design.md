# Favicon Refresh and Default Fallback Design

Date: 2026-07-17
Status: Approved for implementation

## Problem

Two favicon regressions share the same stable browser-visible site-icon URL:

- On a fresh SuperFlare runtime, an ordinary page refresh can keep showing the
  built-in bookmark icon first and replacing it later. The flashing stops only
  after a `Shift+F5` refresh.
- A bookmark whose destination has no usable favicon can be replaced with a
  square containing the site's first letter instead of retaining SuperFlare's
  built-in bookmark icon.

The public `/assets/site-icons?src=...` URL intentionally has no version query
parameter. That requirement remains unchanged.

## Root Causes

### Browser Cache Can Bypass the Server Cache

The asynchronous homepage script currently calls `fetch(src)` with the default
browser cache mode. Site-icon success responses are browser-cacheable for seven
days and marked `immutable`.

If the browser already has a response while the current SuperFlare process or
disk cache does not, the fetch can be satisfied entirely from the browser
cache. SuperFlare never receives the request and therefore never writes its
persistent site-icon cache. Every following homepage render still emits the
built-in placeholder plus an asynchronous source marker, so the icon is
replaced after page paint on every ordinary refresh. `Shift+F5` bypasses the
browser cache, reaches SuperFlare, and happens to populate the server cache.

### Hosted Provider Generates a Synthetic Icon

After direct `/favicon.ico` and HTML discovery fail, SuperFlare currently asks
Icon Horse for an icon. Icon Horse may return a valid PNG containing a generated
first-letter tile when the destination has no favicon. The bytes are a valid
image and provide no reliable metadata that distinguishes the generated tile
from a real site icon, so SuperFlare accepts and caches it.

Old generated provider images can also remain in either the persistent server
cache or the browser's immutable cache under the stable public URL.

## Goals

- Ensure the first asynchronous request reaches SuperFlare even when the
  browser has an older response for the stable site-icon URL.
- Render cached valid icons directly on subsequent page loads without a
  placeholder-to-icon transition.
- Use only icons returned by the destination origin or declared in its HTML.
- Keep the built-in bookmark SVG when no valid origin icon is available.
- Avoid repeated upstream work for a temporarily unavailable or icon-less
  destination while allowing later retries.
- Keep behavior identical on native Windows, Linux, Docker, and fnapp.
- Preserve public site-icon URLs without `v` or another version parameter.

## Non-Goals

- Detecting generated provider images by pixel or OCR heuristics.
- Guaranteeing favicon retrieval when the SuperFlare runtime cannot reach the
  destination.
- Permanently caching failures.
- Adding configuration fields, migration steps, or environment-specific
  networking behavior.
- Disabling normal browser caching for successfully retrieved icons.

## Design

### Browser Request Mode

The homepage's grouped asynchronous request uses:

```js
fetch(src, {cache: "reload"})
```

`reload` bypasses a stored browser response for this request and updates the
browser cache with the response received from SuperFlare. It does not add a
public version marker and does not disable later reuse. The script retains its
existing source deduplication, single request per source, response-state check,
blob decode check, and object URL cleanup.

On the first successful request, SuperFlare downloads and writes the icon to
its persistent cache. The next homepage render recognizes the validated cache
entry and emits the direct stable site-icon URL, which the browser can render
from its cache during initial layout.

### Origin-Only Discovery

The favicon download chain becomes:

1. Request the destination's root `/favicon.ico`.
2. If that fails and the source is a root HTTP(S) favicon URL, request the
   destination page and inspect its bounded HTML prefix for favicon links.
3. Download and validate the discovered origin-declared icon, if present.
4. Return an error when no valid icon is available.

The Icon Horse provider and all provider-selection helpers are removed. DNS,
connection, TLS, timeout, HTTP, parsing, unsupported-content, and no-icon
failures flow to the existing route fallback, which returns the built-in
bookmark SVG with `X-SuperFlare-Site-Icon: fallback` and `Cache-Control:
no-store`.

The existing NXDOMAIN short circuit remains: a definitive name-not-found error
does not trigger a redundant HTML request to the same nonexistent host.

### Finite Failure Cooldown

Actual upstream download or HTML-discovery failures are stored in a
process-local concurrent map for five minutes. Entries are keyed with the same
generation-aware site-icon cache key as persistent files.

The fetch path always checks for a valid persistent cache file before checking
the failure map. During an active failure cooldown it returns immediately
without another upstream request. Successful download and cache write clears
the corresponding failure entry. Cache-write failures are not recorded as
upstream failures.

When homepage rendering finds an active failure and no validated icon, the
asset URL helper returns an empty string. The bookmark therefore remains the
plain built-in SVG and receives no asynchronous fetch marker on subsequent
renders during the cooldown. Expired entries are removed lazily and the next
request may retry the origin.

The cooldown is intentionally in memory. Restarting SuperFlare permits a fresh
attempt, while persistent valid icons continue to survive restarts.

### Cache Generation

The internal persistent cache generation changes from
`2026-07-nxdomain` to `2026-07-origin-only`. This changes only the hashed
filename under `var/cache/site-icons`; it does not change the browser-visible
route or configuration.

Provider-generated images stored under earlier generations are ignored. The
`cache: "reload"` asynchronous request also prevents a browser-cached old
provider response at the stable URL from being applied without consulting the
new server behavior.

## Compatibility

- No bookmark, application, YAML, JSON, CSV, or settings schema changes.
- No deletion or migration of existing cache files is required.
- Explicit configured icons still take precedence over automatic favicons.
- Local-network bookmark favicon URLs retain the same local proxy route and
  origin-only behavior.
- Valid direct icons and HTML-declared relative or absolute icons keep the
  existing validation, payload limits, redirect handling, coalescing, and
  persistent caching.
- The public route contains only the encoded `src` parameter.

## Verification

Automated coverage will prove that:

- the grouped homepage request uses `cache: "reload"`, never `no-store`, and
  still contains no polling;
- an unavailable origin never contacts Icon Horse and produces no cache file;
- the first origin failure starts a cooldown and an immediate second request
  performs no network work;
- an active cooldown suppresses the homepage asynchronous marker;
- an expired cooldown permits a successful retry and is cleared on success;
- the previous internal cache generation is ignored;
- direct and HTML-discovered icons still download, validate, cache, and render;
- public site-icon URLs still contain no `v` parameter;
- the focused packages, full Go test suite, build, vet, formatting, and diff
  checks pass.
