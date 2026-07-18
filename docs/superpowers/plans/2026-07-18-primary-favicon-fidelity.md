# Primary Favicon Fidelity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve a site's HTML-declared primary favicon, including transparency, without removing verified hosted recovery for origins that SuperFlare cannot reach.

**Architecture:** Keep direct `/favicon.ico` first. When that request fails with a network-class error, run HTML discovery and verified hosted recovery concurrently, retain an early hosted success, and deterministically prefer a successful HTML result within the existing ten-second overall context. Change only internal disk and browser repair generations so the stable public route reloads the corrected bytes once.

**Tech Stack:** Go 1.24, `net/http`, `context`, Echo route tests, embedded vanilla JavaScript, Go's image decoders, and the in-app Browser validation runtime.

## Global Constraints

- Keep `siteIconOverallTimeout` at exactly 10 seconds because the HTTP server's write timeout is 10 seconds.
- Keep origin requests at 4 seconds and hosted requests at 6 seconds.
- Keep the public route exactly `/assets/site-icons?src=...`; never add a `v` parameter.
- Never send local, private, reserved, IP-address, or definitive NXDOMAIN hosts to a provider.
- Accept hosted images only with the existing URL, redirect, provenance, size, and decode validation.
- Preserve the existing 4 MiB body limit, 4-megapixel decode limit, 512 KiB HTML limit, five-minute failure cooldown, in-flight coalescing, and atomic cache write.
- Make no bookmark, application, YAML, JSON, CSV, or settings schema changes.
- Work directly on local `main`, commit locally, and do not push.
- Preserve user-owned changes in `fnapp/superflare/manifest` and `tools/superflare-icon.zip`.

---

## File Map

- `internal/fn/favicon.go`: coordinates direct, HTML, and hosted retrieval and owns the disk cache generation.
- `internal/fn/favicon_test.go`: proves deterministic source preference, hosted fallback preservation, timeout bounds, and cache invalidation.
- `internal/pages/home/home.go`: owns the once-per-source browser repair marker while keeping the public URL stable.
- `internal/pages/home/home_test.go`: locks the repair marker and grouped `cache: "reload"` behavior.
- `docs/superpowers/specs/2026-07-18-primary-favicon-fidelity-design.md`: approved behavioral and compatibility contract.

### Task 1: Reproduce Provider Selection Losing Primary Favicon Fidelity

**Files:**
- Modify: `internal/fn/favicon_test.go`

**Interfaces:**
- Consumes: `FetchPublicSiteFavicon(string) ([]byte, string, error)`, `siteIconHTTPClient`, `roundTripperFunc`, `encodeTestFavicon`, and `siteFaviconCachePath`.
- Produces: regression test `TestFetchPublicSiteFaviconPrefersHTMLIconOverEarlierHostedSuccessAfterNetworkFailure` and concurrency-safe expectations for existing network/timeout tests.

- [ ] **Step 1: Add an opaque PNG test helper**

Add this helper beside `encodeTestFaviconSize` without adding imports:

```go
func encodeOpaqueTestPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 2, 2))
	for index := 3; index < len(img.Pix); index += 4 {
		img.Pix[index] = 0xff
	}
	var out bytes.Buffer
	if err := png.Encode(&out, img); err != nil {
		t.Fatalf("encode opaque png favicon: %v", err)
	}
	return out.Bytes()
}
```

- [ ] **Step 2: Add the deterministic source-preference regression test**

Add a test after the existing TLS classification test. The declared icon request blocks until the provider asset has returned, proving that completion order cannot select the provider:

```go
func TestFetchPublicSiteFaviconPrefersHTMLIconOverEarlierHostedSuccessAfterNetworkFailure(t *testing.T) {
	withSiteIconTempWorkingDir(t)

	oldClient := siteIconHTTPClient
	defer func() { siteIconHTTPClient = oldClient }()
	host := "primary-icon.test-domain.com"
	transparentPNG := encodeTestFavicon(t, "png")
	opaquePNG := encodeOpaqueTestPNG(t)
	providerReady := make(chan struct{})
	var htmlRequests int32
	var hostedRequests int32
	siteIconHTTPClient = &http.Client{
		Timeout: 2 * time.Second,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.URL.Host == host && req.URL.Path == "/favicon.ico":
				return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
			case req.URL.Host == host && req.URL.Path == "/":
				atomic.AddInt32(&htmlRequests, 1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader(`<html><head><link rel="icon" href="/primary.png"></head></html>`)),
					Request:    req,
				}, nil
			case req.URL.Host == host && req.URL.Path == "/primary.png":
				atomic.AddInt32(&htmlRequests, 1)
				<-providerReady
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"image/png"}},
					Body:       io.NopCloser(bytes.NewReader(transparentPNG)),
					Request:    req,
				}, nil
			case req.URL.Host == siteIconHostedHost:
				atomic.AddInt32(&hostedRequests, 1)
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{"https://a.favicon.im/" + host + "?throw-error-on-404=true"}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			case req.URL.Host == siteIconHostedAssetHost:
				atomic.AddInt32(&hostedRequests, 1)
				close(providerReady)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header: http.Header{
						"Content-Type":     []string{"image/png"},
						"X-Favicon-Source": []string{"cache-fresh"},
					},
					Body:    io.NopCloser(bytes.NewReader(opaquePNG)),
					Request: req,
				}, nil
			default:
				return nil, fmt.Errorf("unexpected favicon request URL: %s", req.URL)
			}
		}),
	}

	iconURL := "https://" + host + "/favicon.ico"
	data, contentType, err := FetchPublicSiteFavicon(iconURL)
	if err != nil {
		t.Fatalf("FetchPublicSiteFavicon: %v", err)
	}
	if contentType != "image/png" || !bytes.Equal(data, transparentPNG) {
		t.Fatalf("network recovery should prefer the primary HTML icon, type=%q data=%x", contentType, data)
	}
	if bytes.Equal(data, opaquePNG) {
		t.Fatal("network recovery selected the earlier opaque hosted icon")
	}
	if got := atomic.LoadInt32(&htmlRequests); got != 2 {
		t.Fatalf("HTML favicon requests = %d, want page and primary icon", got)
	}
	if got := atomic.LoadInt32(&hostedRequests); got != 2 {
		t.Fatalf("hosted favicon requests = %d, want redirect and asset", got)
	}

	cached, _, err := readCachedSiteFavicon(iconURL)
	if err != nil || !bytes.Equal(cached, transparentPNG) {
		t.Fatalf("primary HTML icon was not cached: err=%v data=%x", err, cached)
	}
}
```

- [ ] **Step 3: Run the regression test and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run TestFetchPublicSiteFaviconPrefersHTMLIconOverEarlierHostedSuccessAfterNetworkFailure -count=1
```

Expected: FAIL because current network recovery returns the opaque hosted PNG before requesting the HTML page.

- [ ] **Step 4: Make existing provider and timeout assertions concurrency-safe**

In `TestFetchPublicSiteFaviconUsesVerifiedHostedFallbackForNetworkFailure`, replace
the shared request slice with:

```go
var directRequests int32
var htmlRequests int32
var hostedRequests int32
```

Use these exact destination cases before the existing provider redirect and
asset cases:

```go
case req.URL.Host == "network-blocked.test-domain.com" && req.URL.Path == "/favicon.ico":
	atomic.AddInt32(&directRequests, 1)
	return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
case req.URL.Host == "network-blocked.test-domain.com" && req.URL.Path == "/":
	atomic.AddInt32(&htmlRequests, 1)
	return &http.Response{
		StatusCode: http.StatusServiceUnavailable,
		Header:     http.Header{"Content-Type": []string{"text/plain"}},
		Body:       io.NopCloser(strings.NewReader("unavailable")),
		Request:    req,
	}, nil
case req.URL.Host == siteIconHostedHost && req.URL.Path == "/network-blocked.test-domain.com":
	atomic.AddInt32(&hostedRequests, 1)
	return &http.Response{
		StatusCode: http.StatusFound,
		Header:     http.Header{"Location": []string{"https://a.favicon.im/network-blocked.test-domain.com?throw-error-on-404=true"}},
		Body:       io.NopCloser(strings.NewReader("")),
		Request:    req,
	}, nil
case req.URL.Host == siteIconHostedAssetHost && req.URL.Path == "/network-blocked.test-domain.com":
	atomic.AddInt32(&hostedRequests, 1)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header: http.Header{
			"Content-Type":     []string{"image/png"},
			"X-Favicon-Source": []string{"cache-fresh"},
		},
		Body:    io.NopCloser(bytes.NewReader(pngData)),
		Request: req,
	}, nil
```

After the existing two fetch attempts, replace the order assertion with:

```go
if got := atomic.LoadInt32(&directRequests); got != 1 {
	t.Fatalf("direct favicon requests = %d, want 1", got)
}
if got := atomic.LoadInt32(&htmlRequests); got != 1 {
	t.Fatalf("HTML favicon requests = %d, want 1", got)
}
if got := atomic.LoadInt32(&hostedRequests); got != 2 {
	t.Fatalf("hosted favicon requests = %d, want redirect and asset", got)
}
```

In `TestFetchPublicSiteFaviconBoundsWholeFallbackChain`, replace the request
slice and its append with:

```go
var originRequests int32
var hostedRequests int32

// Inside the transport:
switch req.URL.Host {
case "slow.test-domain.com":
	atomic.AddInt32(&originRequests, 1)
case siteIconHostedHost:
	atomic.AddInt32(&hostedRequests, 1)
}
<-req.Context().Done()
return nil, req.Context().Err()
```

Keep the deadline and elapsed assertions, then require:

```go
if got := atomic.LoadInt32(&originRequests); got != 2 {
	t.Fatalf("slow origin requests = %d, want direct favicon and HTML page", got)
}
if got := atomic.LoadInt32(&hostedRequests); got != 1 {
	t.Fatalf("slow hosted requests = %d, want 1", got)
}
```

- [ ] **Step 5: Run the focused package to confirm only the new behavior remains RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -count=1
```

Expected: FAIL only at the new primary-icon preference assertion; existing hosted, rejection, cooldown, timeout, and privacy tests remain valid.

### Task 2: Coordinate Concurrent Recovery and Prefer HTML Deterministically

**Files:**
- Modify: `internal/fn/favicon.go`
- Test: `internal/fn/favicon_test.go`

**Interfaces:**
- Consumes: `discoverSiteFaviconFromHTML(context.Context, string)`, `downloadSiteFaviconDirect(context.Context, string)`, `downloadHostedSiteFavicon(context.Context, string)`, and `shouldPreferHostedSiteFavicon(error)`.
- Produces: `siteFaviconDownloadResult`, `downloadDiscoveredSiteFavicon(context.Context, string)`, and `downloadConcurrentSiteFaviconRecovery(context.Context, string)`.

- [ ] **Step 1: Extract HTML discovery plus download into one result-producing unit**

Add:

```go
type siteFaviconDownloadResult struct {
	data        []byte
	contentType string
	err         error
}

func downloadDiscoveredSiteFavicon(ctx context.Context, rootIconURL string) ([]byte, string, error) {
	discoveredURL, err := discoverSiteFaviconFromHTML(ctx, rootIconURL)
	if err != nil {
		return nil, "", fmt.Errorf("html favicon discovery failed: %w", err)
	}
	if discoveredURL == "" || discoveredURL == rootIconURL {
		return nil, "", fmt.Errorf("html favicon discovery returned no alternate icon")
	}
	data, contentType, err := downloadSiteFaviconDirect(ctx, discoveredURL)
	if err != nil {
		return nil, "", fmt.Errorf("html favicon fetch failed: %w", err)
	}
	return data, contentType, nil
}
```

Replace the duplicate sequential discovery block in `downloadSiteFavicon` with this helper while preserving its wrapped aggregate error.

- [ ] **Step 2: Implement the concurrent network-recovery coordinator**

Add a helper that creates a cancelable child context and two capacity-one channels. Start one goroutine for `downloadDiscoveredSiteFavicon` and one for `downloadHostedSiteFavicon` (wrapping its error as `verified hosted favicon recovery failed`). Store branch results rather than returning on hosted success. Return immediately on HTML success; after HTML failure return hosted success; after both failures return `errors.Join`; on parent deadline non-blockingly drain a queued HTML result before falling back to a retained hosted success.

The core selection loop must follow this shape:

```go
func downloadConcurrentSiteFaviconRecovery(ctx context.Context, iconURL string) ([]byte, string, error) {
	recoveryCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	htmlResults := make(chan siteFaviconDownloadResult, 1)
	hostedResults := make(chan siteFaviconDownloadResult, 1)

	go func() {
		data, contentType, err := downloadDiscoveredSiteFavicon(recoveryCtx, iconURL)
		htmlResults <- siteFaviconDownloadResult{data: data, contentType: contentType, err: err}
	}()
	go func() {
		data, contentType, err := downloadHostedSiteFavicon(recoveryCtx, iconURL)
		if err != nil {
			err = fmt.Errorf("verified hosted favicon recovery failed: %w", err)
		}
		hostedResults <- siteFaviconDownloadResult{data: data, contentType: contentType, err: err}
	}()

	var htmlResult *siteFaviconDownloadResult
	var hostedResult *siteFaviconDownloadResult
	for {
		if htmlResult != nil && htmlResult.err == nil {
			return htmlResult.data, htmlResult.contentType, nil
		}
		if htmlResult != nil && hostedResult != nil {
			if hostedResult.err == nil {
				return hostedResult.data, hostedResult.contentType, nil
			}
			return nil, "", errors.Join(htmlResult.err, hostedResult.err)
		}

		select {
		case result := <-htmlResults:
			htmlResult = &result
			htmlResults = nil
		case result := <-hostedResults:
			hostedResult = &result
			hostedResults = nil
		case <-ctx.Done():
			if htmlResult == nil && htmlResults != nil {
				select {
				case result := <-htmlResults:
					htmlResult = &result
				default:
				}
			}
			if hostedResult == nil && hostedResults != nil {
				select {
				case result := <-hostedResults:
					hostedResult = &result
				default:
				}
			}
			if htmlResult != nil && htmlResult.err == nil {
				return htmlResult.data, htmlResult.contentType, nil
			}
			if hostedResult != nil && hostedResult.err == nil {
				return hostedResult.data, hostedResult.contentType, nil
			}
			var branchErrors []error
			if htmlResult != nil && htmlResult.err != nil {
				branchErrors = append(branchErrors, htmlResult.err)
			}
			if hostedResult != nil && hostedResult.err != nil {
				branchErrors = append(branchErrors, hostedResult.err)
			}
			branchErrors = append(branchErrors, ctx.Err())
			return nil, "", errors.Join(branchErrors...)
		}
	}
}
```

- [ ] **Step 3: Route only network-class failures through the coordinator**

In `downloadSiteFavicon`, after direct error, root-URL, and NXDOMAIN checks:

```go
attemptErrors := []error{fmt.Errorf("direct favicon fetch failed: %w", err)}
if shouldPreferHostedSiteFavicon(err) {
	data, contentType, recoveryErr := downloadConcurrentSiteFaviconRecovery(ctx, iconURL)
	if recoveryErr == nil {
		return data, contentType, nil
	}
	return nil, "", errors.Join(attemptErrors[0], recoveryErr)
}
```

Keep non-network errors sequential: call `downloadDiscoveredSiteFavicon`, then one hosted attempt, then return the aggregate error.

- [ ] **Step 4: Run the new regression and hosted fallback tests for GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'TestFetchPublicSiteFavicon(PrefersHTMLIconOverEarlierHostedSuccessAfterNetworkFailure|UsesVerifiedHostedFallbackForNetworkFailure|BoundsWholeFallbackChain)' -count=1
```

Expected: PASS. The primary HTML PNG wins after provider completion, unreachable HTML still uses the provider, and the chain remains under 11 seconds.

- [ ] **Step 5: Run the complete favicon package**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -count=1
```

Expected: PASS with no race-prone order assertions or unexpected requests.

### Task 3: Invalidate Opaque Cache Generations Without Changing Public URLs

**Files:**
- Modify: `internal/fn/favicon.go`
- Modify: `internal/fn/favicon_test.go`
- Modify: `internal/pages/home/home.go`
- Modify: `internal/pages/home/home_test.go`

**Interfaces:**
- Consumes: generation-aware `siteFaviconCacheKey`, `_inlineSiteIconRefreshScript`, and `inlineSiteIconRefreshScript(model.Application)`.
- Produces: disk generation `2026-07-primary-favicon` and browser marker `superflare.site-icon.primary-favicon:`.

- [ ] **Step 1: Add cache-generation RED coverage**

Add `TestReadCachedSiteFaviconIgnoresVerifiedHostedGeneration`, writing a valid PNG under the SHA-256 key for:

```go
"2026-07-verified-hosted" + "\x00" + strings.TrimSpace(iconURL)
```

Assert `readCachedSiteFavicon(iconURL)` returns an error and the current key differs from the previous key.

- [ ] **Step 2: Change the browser repair-prefix expectation to RED**

In `TestInlineSiteIconRefreshScriptRepairsLegacyDirectBrowserCacheOncePerSource`, require:

```go
`var repairKeyPrefix="superflare.site-icon.primary-favicon:"`
```

Also assert the script does not contain `superflare.site-icon.verified-hosted:` and keep the existing single grouped `fetch(src,{cache:"reload"})` assertion.

- [ ] **Step 3: Run generation tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn ./internal/pages/home -run 'Test(ReadCachedSiteFaviconIgnoresVerifiedHostedGeneration|InlineSiteIconRefreshScriptRepairsLegacyDirectBrowserCacheOncePerSource)' -count=1
```

Expected: FAIL because both production generation strings still use `verified-hosted`.

- [ ] **Step 4: Update only the two internal generation strings**

Change `siteIconCacheGeneration` in `internal/fn/favicon.go` to:

```go
siteIconCacheGeneration = "2026-07-primary-favicon"
```

Change `repairKeyPrefix` inside `_inlineSiteIconRefreshScript` in `internal/pages/home/home.go` to:

```js
superflare.site-icon.primary-favicon:
```

Do not change `siteIconProxyPath`, `siteIconProxyURL`, fetch cache mode, or route query construction.

- [ ] **Step 5: Run generation and public-URL tests for GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn ./internal/pages/home ./internal/resources/assets -run 'Test(ReadCachedSiteFaviconIgnoresVerifiedHostedGeneration|InlineSiteIconRefreshScriptRepairsLegacyDirectBrowserCacheOncePerSource|GetSiteFaviconAssetURL|SiteIcon)' -count=1
```

Expected: PASS; current URLs contain only `src`, and old server/browser generations are ignored once.

### Task 4: Verify Live Sources, Rendered Transparency, and Repository Health

**Files:**
- Verify: `internal/fn/favicon.go`
- Verify: `internal/fn/favicon_test.go`
- Verify: `internal/pages/home/home.go`
- Verify: `internal/pages/home/home_test.go`
- Verify: `docs/superpowers/specs/2026-07-18-primary-favicon-fidelity-design.md`
- Verify: `docs/superpowers/plans/2026-07-18-primary-favicon-fidelity.md`

**Interfaces:**
- Consumes: freshly built SuperFlare binary, test bookmark configuration, and Browser runtime.
- Produces: passing automated validation, source-byte evidence, rendered screenshot evidence, and one local implementation commit.

- [ ] **Step 1: Format and run focused validation**

Run:

```powershell
.\.tools\go\bin\gofmt.exe -w internal/fn/favicon.go internal/fn/favicon_test.go internal/pages/home/home.go internal/pages/home/home_test.go
.\.tools\go\bin\go.exe test ./internal/fn ./internal/pages/home ./internal/resources/assets -count=1
.\.tools\go\bin\go.exe vet ./internal/fn ./internal/pages/home ./internal/resources/assets
git diff --check
```

Expected: all commands exit 0.

- [ ] **Step 2: Run full tests and build**

Run:

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
.\.tools\go\bin\go.exe build ./...
```

Expected: both commands exit 0. Full `go vet ./...` is not a gate because the repository has known unrelated failures at `internal/server/server_test.go:84,87`; touched-package vet must pass.

- [ ] **Step 3: Start a fresh local server without reusing old process cache**

Build a uniquely named executable under `$env:TEMP`, start it on an unused localhost port with `--disable_login`, and verify the page identity before using its favicon route. Preserve the existing processes on ports 3649, 3654, and 3636.

- [ ] **Step 4: Verify live route bytes and fallbacks**

Request these stable routes through the fresh process:

```text
/assets/site-icons?src=https%3A%2F%2Fcupfox.love%2Ffavicon.ico
/assets/site-icons?src=https%3A%2F%2Fwallhaven.cc%2Ffavicon.ico
/assets/site-icons?src=https%3A%2F%2Fwallroom.io%2Ffavicon.ico
/assets/site-icons?src=https%3A%2F%2Fdefinitely-missing-superflare.invalid%2Ffavicon.ico
```

Require `cupfox.love` to return the 8,365-byte transparent PNG (or equivalent current primary icon bytes with at least one alpha value below 255), Wallhaven and Wallroom to return `X-SuperFlare-Site-Icon: cached` valid images, and the nonexistent host to return `fallback`, `image/svg+xml`, and `Cache-Control: no-store`.

- [ ] **Step 5: Validate the homepage in Browser**

The flow under test is: fresh localhost homepage -> `cupfox.love` bookmark -> transparent icon pixels reveal the homepage background.

Using the existing Browser binding, navigate the test tab to the fresh port and verify URL/title, meaningful DOM, no framework overlay, console warnings/errors, the unique cupfox link and its image dimensions/source, a screenshot, and one interaction-state proof (reload and confirm the icon remains complete without a placeholder transition). Also verify one mobile viewport if the Browser viewport capability is available.

- [ ] **Step 6: Review and commit only task files**

Run:

```powershell
git status --short
git diff --stat
git diff --check
git diff -- internal/fn/favicon.go internal/fn/favicon_test.go internal/pages/home/home.go internal/pages/home/home_test.go docs/superpowers/plans/2026-07-18-primary-favicon-fidelity.md
git add -- internal/fn/favicon.go internal/fn/favicon_test.go internal/pages/home/home.go internal/pages/home/home_test.go docs/superpowers/plans/2026-07-18-primary-favicon-fidelity.md
git commit -m "fix: preserve primary favicon transparency"
```

Expected: commit succeeds; `fnapp/superflare/manifest` and `tools/superflare-icon.zip` remain untouched and uncommitted, and no push occurs.
