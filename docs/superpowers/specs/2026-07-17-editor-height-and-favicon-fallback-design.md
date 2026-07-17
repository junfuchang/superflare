# Editor Runtime Height and Favicon Fallback Design

## Scope

This change addresses three related regressions without changing user data or
configuration formats:

1. Both tables on `/editor` must grow to their full rendered height and must
   not have an internal vertical scrollbar. Horizontal scrolling remains
   available when the table is wider than its panel.
2. Homepage-generated site-icon URLs must not expose the `v=2` query
   parameter.
3. A bookmark whose hostname definitively does not exist must keep the built-in
   bookmark placeholder instead of caching a third-party letter icon.

The existing dirty `fnapp/superflare/manifest` file is outside this scope and
must remain untouched. The completed changes stay on local `main` and are not
pushed.

## Root Causes

### Editor tables

The two Handsontable instances enable `autoRowSize`, so wrapped content can
make a row taller than 27 pixels. The template nevertheless calculates each
table height from a fixed 27-pixel row assumption and reapplies that value
after data or layout changes. Once a row wraps, the rendered content is taller
than the fixed container and Handsontable creates an internal vertical scroll
area.

### Browser cache marker

`siteIconProxyURL` always adds a public `v=2` query parameter. Server-side
immutable caching and ETag handling now provide stable browser reuse, so this
one-off public cache marker is no longer needed.

### Invalid bookmark icon

When the origin request fails with DNS NXDOMAIN, the error is also a network
error. The favicon fallback chain therefore calls Icon Horse. Icon Horse can
return an HTTP 200 generic first-letter image for a hostname that does not
exist, and SuperFlare validates and caches that image as if it were the site's
favicon. Existing cache files do not record whether their bytes came from the
origin or the hosted fallback, so old generic images cannot be identified
reliably after the fact.

## Design

### Handsontable sizing

Both category and bookmark tables use the same rendered-content sizing helper:

- leave `height` out of both initial Handsontable configurations;
- set `renderAllRows: true` so the complete dataset is present for measurement;
- retain `autoRowSize: true` and `wordWrap: true`;
- render the instance, then measure the master `.htCore` table with
  `getBoundingClientRect().height`;
- round the measured content height upward and add the holder's horizontal
  scrollbar thickness (`offsetHeight - clientHeight`);
- apply the resulting numeric height only when it is positive and differs from
  the current setting, preventing resize/update loops;
- trigger scheduled layout synchronization for bookmark cell edits and manual
  column resizing as well as theme changes, data changes, row operations, page
  load, and viewport resize.

Browser QA at devicePixelRatio 1.5 showed that neither native shortcut is
reliable in Handsontable 6.2.2. `height: 'auto'` rounds the holder 3-5 pixels
below the rendered table, while omitting height entirely collapses both holders
to about 29 pixels and makes their content scroll internally. Measuring the
rendered master table avoids fixed row assumptions, includes wrapped row
heights, and preserves the extra space needed for the bookmark table's
horizontal scrollbar.

The surrounding panel remains width-bounded. Handsontable continues to own
horizontal overflow; no CSS override is added to its internal holder layers.

### Site-icon URL and disk cache

`siteIconProxyURL` emits only the encoded `src` parameter. No replacement
browser-visible version parameter is introduced.

The disk cache key includes an internal cache generation before hashing. This
changes only filenames under `var/cache/site-icons`; it does not change routes,
configuration, or bookmark data. Existing files generated before this fix are
ignored, which prevents a previously cached Icon Horse placeholder from being
rendered as a valid favicon. Valid icons are downloaded once under the new key
and then reuse the existing persistent disk cache, immutable browser cache,
and ETag behavior.

### Definitive DNS failures

After the direct root favicon request fails, the error chain is inspected for
`*net.DNSError` with `IsNotFound == true`. This is the cross-platform Go
representation of a definitive DNS name-not-found result on Windows, Linux,
Docker, and fnapp environments.

For that one definitive condition, the fallback chain stops and returns an
error to the existing `/assets/site-icons` handler. The handler already sends
the built-in bookmark SVG with `X-SuperFlare-Site-Icon: fallback` and
`Cache-Control: no-store`; the homepage refresh script therefore leaves its
already-visible bookmark placeholder in place.

Timeouts, connection failures, HTTP failures, and other non-definitive errors
keep the existing HTML discovery and Icon Horse fallback behavior. This
preserves favicon support for valid domains whose origin blocks or delays the
SuperFlare runtime.

## Compatibility

- No YAML/JSON/CSV configuration schema changes.
- No migration or deletion of old cache files is required.
- No development-environment special case or platform-specific command is
  introduced.
- Local-network bookmarks retain the current proxy route behavior.
- Explicitly configured bookmark icons continue to take precedence.
- Invalid URLs that cannot be parsed continue to use the built-in fallback.

## Error Handling

- Only a definitive DNS name-not-found error suppresses the hosted provider.
- Temporary DNS failures remain retryable through the existing request path.
- A failed favicon fetch is not written to disk and the HTTP fallback response
  remains non-cacheable.
- Cache key generation remains deterministic for a normalized icon URL.

## Verification

Automated coverage will verify:

- both editor tables omit an initial height, render all rows, and use the same
  rendered-content height helper;
- the helper rounds the real table height upward, compensates for a horizontal
  scrollbar, and avoids unchanged height updates;
- bookmark cell edits and manual column resizing refit the affected rendered
  row heights;
- generated site-icon URLs contain only `src` and never `v=2`;
- legacy disk cache filenames are ignored;
- NXDOMAIN never reaches Icon Horse and returns an error;
- resolvable or otherwise non-NXDOMAIN failures retain hosted fallback;
- existing fallback-route, cache-hit, homepage rendering, and template tests
  remain green.

Rendered browser QA will verify both tables after wrapped content and row
insertion/removal, confirm horizontal scrolling still works, inspect homepage
site-icon URLs, and confirm an invalid hostname keeps the built-in bookmark
icon without repeated replacement.
