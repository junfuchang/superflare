# Primary Favicon Fidelity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace an opaque secondary hosted icon with a strictly verified transparent primary candidate while never accepting generated first-letter fallbacks.

**Architecture:** Keep the current direct, HTML, and verified `favicon.im` retrieval order. After a successful trusted hosted result, perform one sequential Icon Horse refinement only when the baseline is definitely opaque; accept the candidate only with strict long-cache source metadata, a non-empty ETag, full existing image validation, and actual transparent pixels.

**Tech Stack:** Go 1.24, `net/http`, `context`, Go image decoders, Echo route tests, embedded vanilla JavaScript, and the in-app Browser runtime.

## Global Constraints

- Keep `siteIconOverallTimeout` at exactly 10 seconds, origin timeout at 4 seconds, and hosted timeout at 6 seconds.
- Keep network work sequential inside the existing chain-level fetch slot; do not increase effective outbound concurrency.
- Keep the public route exactly `/assets/site-icons?src=...`; never add a `v` parameter.
- Never send local, private, reserved, IP-address, or definitive NXDOMAIN hosts to either provider.
- Never accept an Icon Horse result based only on HTTP success or valid image bytes.
- Preserve the 4 MiB body limit, 4-megapixel decoded limit, 512 KiB HTML limit, five-minute failure cooldown, in-flight coalescing, and atomic cache write.
- Make no bookmark, application, YAML, JSON, CSV, or settings schema changes.
- Work directly on local `main`, commit locally, and do not push.
- Preserve user-owned changes in `fnapp/superflare/manifest` and `tools/superflare-icon.zip`.

---

## File Map

- `internal/fn/favicon.go`: hosted refinement URL/security policy, alpha check, retrieval integration, and disk cache generation.
- `internal/fn/favicon_test.go`: hosted metadata, redirect, opacity, fallback, ordering, timeout, and cache-generation tests.
- `internal/pages/home/home.go`: once-per-source browser repair generation.
- `internal/pages/home/home_test.go`: browser generation and stable public request behavior.
- `docs/superpowers/specs/2026-07-18-primary-favicon-fidelity-design.md`: behavioral and compatibility contract.

### Task 1: Replace the Failed Concurrency Hypothesis with Provider-Refinement RED Tests

**Files:**
- Modify: `internal/fn/favicon_test.go`
- Modify: `internal/fn/favicon.go`

**Interfaces:**
- Consumes: current `downloadSiteFavicon`, `downloadHostedSiteFavicon`, `downloadSiteFaviconWithClient`, and `encodeTestFavicon` helpers.
- Produces: `encodeOpaqueTestPNG`, provider-refinement RED tests, and the original sequential retrieval behavior restored before implementation.

- [ ] **Step 1: Remove the uncommitted concurrent coordinator**

Restore `downloadSiteFavicon` to the committed ordering: network-class errors
try `downloadHostedSiteFavicon` first; other failures try HTML first; the
provider is attempted at most once. Remove `siteFaviconDownloadResult`,
`downloadDiscoveredSiteFavicon`, and
`downloadConcurrentSiteFaviconRecovery`.

Restore the existing provider and timeout tests to their committed sequential
request expectations so the production rollback is green before introducing
the new behavior.

- [ ] **Step 2: Add the primary-candidate RED test**

Use an opaque PNG for the trusted `favicon.im` asset and a transparent PNG for
`https://icon.horse/icon/primary-icon.test-domain.com`. Give the Icon Horse
response these exact headers:

```go
http.Header{
	"Content-Type":      []string{"image/png"},
	"Cdn-Cache-Control": []string{"max-age=2592000"},
	"Etag":              []string{`"primary-icon"`},
}
```

Call `FetchPublicSiteFavicon` after a direct network error and assert the
transparent bytes win, the existing `favicon.im` redirect/asset flow and one
Icon Horse request were made, the destination HTML was not contacted,
and the transparent bytes were written to the destination cache.

- [ ] **Step 3: Add fail-closed fallback RED tests**

Table-test these Icon Horse responses after an opaque trusted baseline:

```go
[]struct {
	name       string
	headers    http.Header
	body       []byte
	statusCode int
}{
	{name: "short-cache-letter", statusCode: 200, headers: http.Header{
		"Content-Type": []string{"image/png"},
		"Cache-Control": []string{"public, max-age=604800, s-maxage=300, stale-while-revalidate=3600"},
	}, body: opaqueLetterPNG},
	{name: "missing-cdn-source", statusCode: 200, headers: http.Header{"Content-Type": []string{"image/png"}, "Etag": []string{`"fallback"`}}, body: transparentPNG},
	{name: "blank-etag", statusCode: 200, headers: http.Header{"Content-Type": []string{"image/png"}, "Cdn-Cache-Control": []string{"max-age=2592000"}, "Etag": []string{" "}}, body: transparentPNG},
	{name: "malformed-etag", statusCode: 200, headers: http.Header{"Content-Type": []string{"image/png"}, "Cdn-Cache-Control": []string{"max-age=2592000"}, "Etag": []string{"garbage"}}, body: transparentPNG},
	{name: "opaque-candidate", statusCode: 200, headers: validPrimaryHeaders, body: opaqueLetterPNG},
}
```

Each case must return and cache the original trusted baseline, never the Icon
Horse body. Add a separate test proving an already transparent trusted
baseline performs zero Icon Horse requests.

- [ ] **Step 4: Run RED tests**

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'TestFetchPublicSiteFavicon(RefinesOpaqueHostedIconWithTransparentPrimary|RejectsUnverifiedPrimaryRefinement|SkipsPrimaryRefinementForTransparentHostedIcon)' -count=1
```

Expected: FAIL because the provider-refinement helpers and request path do not
exist.

### Task 2: Implement Fail-Closed Transparent Primary Refinement

**Files:**
- Modify: `internal/fn/favicon.go`
- Test: `internal/fn/favicon_test.go`

**Interfaces:**
- Consumes: `hostedSiteFaviconURL`, `downloadHostedSiteFavicon`, `downloadSiteFaviconWithClient`, `isAllowedHostedSiteFaviconURL`, and existing public-host filtering.
- Produces: `primaryHostedSiteFaviconURL`, `isAllowedPrimaryHostedSiteFaviconURL`, `validatePrimaryHostedSiteFaviconResponse`, `siteFaviconTransparency`, `siteFaviconTransparencyContext`, and `refineOpaqueHostedSiteFavicon`.

- [ ] **Step 1: Add the provider URL and strict response policy**

Add constants:

```go
siteIconPrimaryHostedHost  = "icon.horse"
siteIconPrimaryHostedCache = "max-age=2592000"
```

`primaryHostedSiteFaviconURL` must reuse the same public-host privacy policy as
`hostedSiteFaviconURL` and produce exactly:

```text
https://icon.horse/icon/<normalized-hostname>
```

The primary client clones `siteIconHTTPClient`, keeps its four-second timeout,
and rejects every redirect. The response validator requires the exact final
URL, exactly one trimmed `CDN-Cache-Control` equal to
`max-age=2592000`, and exactly one non-empty ETag.

- [ ] **Step 2: Add bounded transparency detection**

Use the existing image decoders and already enforced decoded-pixel ceiling:

```go
func siteFaviconTransparency(data []byte) (bool, bool) {
	decoded, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return false, false
	}
	bounds := decoded.Bounds()
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			_, _, _, alpha := decoded.At(x, y).RGBA()
			if alpha < 0xffff {
				return true, true
			}
		}
	}
	return false, true
}
```

Wrap both production scans with `siteFaviconTransparencyContext`, which acquires
and releases `siteIconDecodeLimiter`. Unsupported ICO/SVG transparency is
treated as unknown, so an existing verified icon is retained rather than
replaced and the two-decoder concurrency ceiling remains enforced.

- [ ] **Step 3: Implement optional refinement**

```go
func refineOpaqueHostedSiteFavicon(ctx context.Context, rootIconURL string, data []byte, contentType string) ([]byte, string) {
	hasTransparency, known, err := siteFaviconTransparencyContext(ctx, data)
	if err != nil || !known || hasTransparency {
		return data, contentType
	}
	primaryURL := primaryHostedSiteFaviconURL(rootIconURL)
	if primaryURL == "" {
		return data, contentType
	}
	primaryData, primaryType, err := downloadSiteFaviconWithClient(
		ctx,
		primaryURL,
		primaryHostedSiteFaviconHTTPClient(),
		validatePrimaryHostedSiteFaviconResponse,
	)
	if err != nil {
		return data, contentType
	}
	primaryHasTransparency, primaryKnown, err := siteFaviconTransparencyContext(ctx, primaryData)
	if err != nil || !primaryKnown || !primaryHasTransparency {
		return data, contentType
	}
	return primaryData, primaryType
}
```

Call this helper after every successful `downloadHostedSiteFavicon` result in
`downloadSiteFavicon`. Refinement failure is intentionally not appended to the
aggregate error because the trusted baseline remains a successful icon.

- [ ] **Step 4: Add URL, redirect, metadata, and privacy unit tests**

Require public URL construction for `cupfox.love`; empty URLs for local,
private, IP, reserved, and test hosts; rejection of any redirect; rejection of
duplicate source or ETag headers; and rejection of final URL path/query/host
changes.

- [ ] **Step 5: Run focused and complete package tests**

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'Test(PrimaryHosted|ValidatePrimaryHosted|SiteFaviconTransparency|FetchPublicSiteFavicon)' -count=1
.\.tools\go\bin\go.exe test ./internal/fn -count=1
```

Expected: PASS, including existing provider, HTML ordering, privacy, cooldown,
and ten-second deadline coverage.

### Task 3: Invalidate Opaque Cache Generations Without Public URL Changes

**Files:**
- Modify: `internal/fn/favicon.go`
- Modify: `internal/fn/favicon_test.go`
- Modify: `internal/pages/home/home.go`
- Modify: `internal/pages/home/home_test.go`

**Interfaces:**
- Produces: disk generation `2026-07-primary-favicon` and browser repair marker `superflare.site-icon.primary-favicon:`.

- [ ] **Step 1: Keep the existing RED/GREEN generation coverage**

`TestReadCachedSiteFaviconIgnoresVerifiedHostedGeneration` must write a valid
PNG under the old `2026-07-verified-hosted\x00<URL>` SHA-256 key and prove the
reader ignores it.

`TestInlineSiteIconRefreshScriptRepairsLegacyDirectBrowserCacheOncePerSource`
must require `superflare.site-icon.primary-favicon:`, reject the old marker,
and retain exactly one grouped `fetch(src,{cache:"reload"})`.

- [ ] **Step 2: Keep only internal generation changes**

```go
siteIconCacheGeneration = "2026-07-primary-favicon"
```

```js
superflare.site-icon.primary-favicon:
```

Do not change `siteIconProxyPath`, route query construction, or browser fetch
mode.

- [ ] **Step 3: Verify generation and public URL behavior**

```powershell
.\.tools\go\bin\go.exe test ./internal/fn ./internal/pages/home ./internal/resources/assets -count=1
```

Expected: PASS with no `v` parameter in generated site-icon URLs.

### Task 4: Verify Live Sources, Rendering, and Repository Health

**Files:**
- Verify all files listed above.

**Interfaces:**
- Produces: passing automated validation, live alpha evidence, Browser screenshot evidence, and one local implementation commit.

- [ ] **Step 1: Format and verify affected packages**

```powershell
.\.tools\go\bin\gofmt.exe -w internal/fn/favicon.go internal/fn/favicon_test.go internal/pages/home/home.go internal/pages/home/home_test.go
.\.tools\go\bin\go.exe test ./internal/fn ./internal/pages/home ./internal/resources/assets -count=1
.\.tools\go\bin\go.exe vet ./internal/fn ./internal/pages/home ./internal/resources/assets
git diff --check
```

- [ ] **Step 2: Run full tests and build**

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
.\.tools\go\bin\go.exe build ./...
```

Full `go vet ./...` is not a gate because of known unrelated failures at
`internal/server/server_test.go:84,87`; touched-package vet must pass.

- [ ] **Step 3: Verify a fresh server**

Build a unique executable under `$env:TEMP`, start it on an unused localhost
port with `--disable_login`, and preserve existing processes. Require:

```text
cupfox.love -> cached image/png, 8,365 bytes, alpha below 255
wallhaven.cc -> cached valid image
wallroom.io -> cached valid image
definitely-missing-superflare.invalid -> fallback image/svg+xml, no-store
```

- [ ] **Step 4: Verify the homepage in Browser**

The flow under test is: fresh localhost homepage -> `cupfox.love` bookmark ->
transparent icon pixels reveal the homepage background. Verify URL/title,
meaningful DOM, no framework overlay, console health, unique link/image state,
viewport screenshot, and reload stability.

- [ ] **Step 5: Review and commit only task files**

```powershell
git status --short
git diff --check
git add -- docs/superpowers/specs/2026-07-18-primary-favicon-fidelity-design.md docs/superpowers/plans/2026-07-18-primary-favicon-fidelity.md internal/fn/favicon.go internal/fn/favicon_test.go internal/pages/home/home.go internal/pages/home/home_test.go
git commit -m "fix: preserve primary favicon transparency"
```

The user-owned manifest and ZIP remain untouched and uncommitted. Do not push.
