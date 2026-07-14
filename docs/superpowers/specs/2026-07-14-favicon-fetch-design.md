# Favicon Fetch and Request Scheduling Design

Date: 2026-07-14
Status: Approved for implementation

## Problem

SuperFlare derives a default favicon URL from each bookmark and serves the
result through `/assets/site-icons`. Two independent behaviors currently cause
bad results:

- Some sites return HTML from `/favicon.ico` and declare the real icon in the
  document head. The current discovery path rejects the entire response when
  the page is larger than 512 KiB, even when the icon declaration appears near
  the beginning of the document.
- On a cache miss, the browser polls the same local icon URL up to eight times.
  Each DOM node polls independently, so a permanently unreachable site can
  produce frequent duplicate requests without improving the rendered result.

Network failure itself is not a bug. When the current runtime environment
cannot reach an icon, SuperFlare should keep the built-in bookmark placeholder.

## Goals

- Discover icons declared near the start of large HTML pages without reading
  an unbounded response.
- Issue at most one browser request per unique favicon source during a page
  load.
- Preserve the existing placeholder for network, timeout, HTTP, parsing, and
  unsupported-content failures.
- Keep behavior identical across native Windows, Linux, Docker, and fnapp
  deployments.
- Preserve existing cache, redirect validation, response-size limits, and
  concurrent-fetch coalescing.

## Non-Goals

- Reading Windows system proxy settings or adding any platform-specific proxy
  discovery.
- Retrying indefinitely or guaranteeing access to every third-party site.
- Replacing the existing local favicon proxy or persistent cache.
- Expanding the accepted icon formats or maximum icon size.

## Server Flow

The `/assets/site-icons` handler will become a single-result request:

1. Validate the `src` query parameter.
2. Return a valid cached icon immediately when one exists.
3. On a cache miss, call the existing favicon fetch-and-cache path and wait for
   it to finish. Existing in-flight coalescing ensures concurrent requests for
   the same source share one upstream fetch.
4. On success, return the fetched icon with the existing `cached` state header
   and cache policy.
5. On failure, return the built-in bookmark SVG with the existing `fallback`
   state header and `no-store` policy.

Page rendering remains non-blocking. The existing warm-up can start an upstream
fetch before the browser calls the route; the route then joins that in-flight
operation instead of polling its status.

## HTML Icon Discovery

Direct `/favicon.ico` fetching remains the first attempt. When that attempt
fails and the source is a root HTTP(S) favicon URL, SuperFlare requests the site
root and scans a bounded prefix of the HTML response.

The discovery parser will tokenize at most 512 KiB and inspect `<link>` elements
for favicon-compatible `rel` values. It may return as soon as a valid icon is
found and does not require the entire HTML document to fit inside the limit.
If no valid declaration appears within the bounded prefix, discovery fails and
the placeholder is retained.

Relative icon URLs continue to resolve against the site root and pass through
the existing source validation and redirect rules before download.

## Browser Flow

The inline refresh script will group all `img[data-site-icon-src]` nodes by
their source URL. It performs one `fetch` for each unique source and applies one
returned object URL to every node in that group when the response is successful
and carries `X-SuperFlare-Site-Icon: cached`.

Fallback responses and rejected requests leave the existing bookmark SVG in
place. The script will not schedule timers or retries. Created object URLs are
revoked once during `pagehide`.

## Error Handling

The following conditions are expected fallback cases:

- DNS, connection, TLS, proxy, or timeout failure.
- Non-success HTTP status.
- Empty, oversized, or unsupported icon content.
- HTML without a valid icon declaration in the bounded scan.
- A discovered icon URL that fails existing source validation.

These cases do not produce client retry loops and do not require
platform-specific behavior.

## Tests

Implementation will follow a red-green cycle with focused regression coverage:

- A root favicon response that falls back to an HTML page larger than 512 KiB
  still discovers an icon declared near the start of the document.
- A cache-miss `/assets/site-icons` request waits for a successful local test
  server response and returns the fetched icon.
- A failed upstream request returns the built-in placeholder and fallback state.
- The refresh script contains no polling timer and groups duplicate source URLs
  into one request.
- Browser network-event verification confirms that an unreachable bookmark
  produces exactly one `/assets/site-icons` request during the observation
  window while the placeholder remains visible.

The full Go test suite, build, formatting checks, and script checks will run
after the focused tests pass.
