# Favicon Refresh and Default Fallback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make ordinary refreshes populate SuperFlare's site-icon cache and retain the built-in bookmark icon whenever an origin has no usable favicon.

**Architecture:** Keep the stable local `/assets/site-icons?src=...` route and persistent success cache. Force only the asynchronous placeholder request to revalidate through SuperFlare, remove the synthetic hosted provider, and add a five-minute process-local negative cache so origin failures do not cause repeated upstream work or repeated homepage replacement attempts.

**Tech Stack:** Go 1.26.x, Echo v5, Go `net/http`, `sync.Map`, server-rendered inline JavaScript, Go unit/integration tests.

## Global Constraints

- Public site-icon URLs must contain only `src`; do not add `v` or another version parameter.
- Successful site-icon responses retain normal immutable browser caching.
- Automatic favicon retrieval may use only the destination's `/favicon.ico` or an icon declared by the destination HTML.
- DNS, connection, TLS, timeout, HTTP, parsing, unsupported-content, and missing-icon failures retain the built-in bookmark SVG.
- Failure suppression lasts five minutes in memory and must permit a later retry.
- Preserve source validation, redirect validation, the 4 MiB payload limit, the 512 KiB HTML scan limit, concurrent-fetch coalescing, and persistent success caching.
- Do not add configuration fields, migrations, or Windows/Linux/Docker/fnapp-specific branches.
- Preserve user-owned changes in `fnapp/superflare/manifest` and `tools/superflare-icon.zip`.
- Work directly on local `main`; do not push.

## File Map

- `internal/pages/home/home.go`: grouped asynchronous site-icon request cache mode.
- `internal/pages/home/home_test.go`: inline script contract regression coverage.
- `internal/fn/favicon.go`: origin-only fallback chain, internal cache generation, and finite failure cooldown.
- `internal/fn/favicon_test.go`: provider removal, negative cache, retry, and cache-generation regression coverage.
- `docs/superpowers/specs/2026-07-17-favicon-refresh-and-default-fallback-design.md`: approved behavior and compatibility design.
- `docs/superpowers/plans/2026-07-17-favicon-refresh-and-default-fallback.md`: executable implementation steps.

---

### Task 1: Force Placeholder Fetches Through SuperFlare

**Files:**
- Modify: `internal/pages/home/home_test.go:1466`
- Modify: `internal/pages/home/home.go:95`

**Interfaces:**
- Consumes: `_inlineSiteIconRefreshScript` and `inlineSiteIconRefreshScript(model.Application)`.
- Produces: one grouped `fetch(src,{cache:"reload"})` call per unique source URL.

- [ ] **Step 1: Tighten the inline-script regression test**

Change the fetch assertions in `TestInlineSiteIconRefreshScriptDeduplicatesSourcesWithoutPolling` to:

```go
	if got := strings.Count(script, `fetch(src,{cache:"reload"})`); got != 1 {
		t.Fatalf("favicon refresh script should contain one grouped reload fetch call, got %d: %s", got, script)
	}
	for _, unexpected := range []string{`fetch(src)`, `setTimeout`, `var left=`, `cache:"no-store"`} {
		if strings.Contains(script, unexpected) {
			t.Fatalf("favicon refresh script should not contain %q: %s", unexpected, script)
		}
	}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/pages/home -run TestInlineSiteIconRefreshScriptDeduplicatesSourcesWithoutPolling -count=1
```

Expected: FAIL because the script still contains `fetch(src)` and no reload cache mode.

- [ ] **Step 3: Set the grouped fetch cache mode**

In `_inlineSiteIconRefreshScript`, replace the only fetch expression with:

```js
fetch(src,{cache:"reload"})
```

Leave deduplication, response header checks, blob decoding, DOM replacement,
and object URL cleanup unchanged.

- [ ] **Step 4: Run the focused home tests and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/pages/home -run 'TestInlineSiteIconRefreshScript' -count=1
```

Expected: PASS for all inline site-icon script tests.

### Task 2: Remove Synthetic Provider Fallbacks

**Files:**
- Modify: `internal/fn/favicon_test.go:1256-1435`
- Modify: `internal/fn/favicon.go:33-615`

**Interfaces:**
- Consumes: `downloadSiteFaviconDirect`, `discoverSiteFaviconFromHTML`, `isRootHTTPFaviconURL`, and `isDefinitiveSiteFaviconDNSNotFound`.
- Produces: `downloadSiteFavicon(context.Context, string) ([]byte, string, error)` with an origin-only chain.

- [ ] **Step 1: Replace provider-success expectations with an origin-only failure test**

Replace `TestHostedSiteFaviconURLUsesOnlyPublicDomainNames` and
`TestFetchPublicSiteFaviconUsesHostedFallbackWhenOriginUnavailable` with a
test whose transport serves `404` for `/favicon.ico`, `503` for `/`, and fails
the test if `req.URL.Host == "icon.horse"`. Assert that
`FetchPublicSiteFavicon` returns an error and that no generated cache file
exists:

```go
func withSiteIconTempWorkingDir(t *testing.T) {
	t.Helper()
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(oldWD); err != nil {
			t.Errorf("restore working directory: %v", err)
		}
	})
}

func TestFetchPublicSiteFaviconUsesBuiltinFallbackWhenOriginUnavailable(t *testing.T) {
	withSiteIconTempWorkingDir(t)

	oldClient := siteIconHTTPClient
	t.Cleanup(func() { siteIconHTTPClient = oldClient })
	var originRequests int32
	siteIconHTTPClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == "icon.horse" {
			t.Fatalf("origin-only favicon fetching must not contact Icon Horse: %s", req.URL)
		}
		atomic.AddInt32(&originRequests, 1)
		status := http.StatusNotFound
		if req.URL.Path == "/" {
			status = http.StatusServiceUnavailable
		}
		return &http.Response{
			StatusCode: status,
			Header:     http.Header{"Content-Type": []string{"text/plain"}},
			Body:       io.NopCloser(strings.NewReader("unavailable")),
			Request:    req,
		}, nil
	})}

	iconURL := "https://origin-unavailable.test-domain.com/favicon.ico"
	_, _, err := FetchPublicSiteFavicon(iconURL)
	if err == nil {
		t.Fatal("unavailable origin should use the route's built-in fallback")
	}
	if got := atomic.LoadInt32(&originRequests); got != 2 {
		t.Fatalf("origin requests = %d, want direct icon and HTML page", got)
	}
	cachePath, pathErr := siteFaviconCachePath(iconURL)
	if pathErr != nil {
		t.Fatalf("siteFaviconCachePath: %v", pathErr)
	}
	if _, statErr := os.Stat(cachePath); !os.IsNotExist(statErr) {
		t.Fatalf("failed origin must not produce a cache file: %v", statErr)
	}
}
```

Use the helper only in the newly added tests; leave unrelated existing test
setup unchanged.

- [ ] **Step 2: Update the timeout-chain expectation**

In `TestFetchPublicSiteFaviconBoundsWholeFallbackChain`, record only requests
to `slow.test-domain.com`. The four-second HTTP client timeout permits the HTML
discovery request to use the remainder of the eight-second overall deadline,
so expect the direct favicon request followed by the page request:

```go
	if got := strings.Join(requests, ","); got != "slow.test-domain.com/favicon.ico,slow.test-domain.com/" {
		t.Fatalf("timeout fallback request order = %q", got)
	}
```

- [ ] **Step 3: Run origin-failure tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'TestFetchPublicSiteFavicon(UsesBuiltinFallbackWhenOriginUnavailable|BoundsWholeFallbackChain)' -count=1
```

Expected: FAIL because current code still contacts `icon.horse`.

- [ ] **Step 4: Remove the hosted provider path**

Delete `siteIconFallbackHost`, `shouldPreferHostedSiteFavicon`,
`downloadHostedSiteFavicon`, `hostedSiteFaviconURL`, and
`isReservedSiteFaviconHost`. Reduce `downloadSiteFavicon` to direct fetch,
NXDOMAIN short circuit, and bounded HTML discovery:

```go
func downloadSiteFavicon(ctx context.Context, iconURL string) ([]byte, string, error) {
	data, contentType, err := downloadSiteFaviconDirect(ctx, iconURL)
	if err == nil {
		return data, contentType, nil
	}
	if !isRootHTTPFaviconURL(iconURL) {
		return nil, "", err
	}
	if isDefinitiveSiteFaviconDNSNotFound(err) {
		return nil, "", fmt.Errorf("favicon host does not resolve: %w", err)
	}
	attemptErrors := []error{fmt.Errorf("direct favicon fetch failed: %w", err)}
	if err := ctx.Err(); err != nil {
		return nil, "", errors.Join(append(attemptErrors, err)...)
	}

	discoveredURL, discoverErr := discoverSiteFaviconFromHTML(ctx, iconURL)
	if discoverErr == nil && discoveredURL != "" && discoveredURL != iconURL {
		data, contentType, discoveredErr := downloadSiteFaviconDirect(ctx, discoveredURL)
		if discoveredErr == nil {
			return data, contentType, nil
		}
		attemptErrors = append(attemptErrors, fmt.Errorf("html favicon fetch failed: %w", discoveredErr))
	} else if discoverErr != nil {
		attemptErrors = append(attemptErrors, fmt.Errorf("html favicon discovery failed: %w", discoverErr))
	}
	return nil, "", errors.Join(attemptErrors...)
}
```

- [ ] **Step 5: Run the origin and existing discovery tests and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'TestFetchPublicSiteFavicon(UsesBuiltinFallbackWhenOriginUnavailable|SkipsHostedFallbackForNXDOMAIN|BoundsWholeFallbackChain|DiscoversHTMLDeclaredIconWhenRootIcoFails|DiscoversIconBeforeLargeHTMLBodyLimit)' -count=1
```

Expected: PASS after renaming the NXDOMAIN test to remove the obsolete hosted-provider wording if needed.

### Task 3: Add the Finite Failure Cooldown and Invalidate Provider Caches

**Files:**
- Modify: `internal/fn/favicon_test.go`
- Modify: `internal/fn/favicon.go:33-472`

**Interfaces:**
- Produces: `siteIconFailureTTL`, `siteIconFailures`, `siteFaviconFailureActive(string) bool`, `recordSiteFaviconFailure(string)`, and `clearSiteFaviconFailure(string)`.
- Consumes: `siteFaviconCacheKey`, `readCachedSiteFaviconContext`, `downloadSiteFavicon`, and `writeCachedSiteFavicon`.

- [ ] **Step 1: Add RED tests for cooldown suppression and expiry**

Add package-level tests using unique icon URLs and deleting their map keys in
`t.Cleanup`:

```go
func TestFetchPublicSiteFaviconSuppressesImmediateRetryAfterFailure(t *testing.T) {
	withSiteIconTempWorkingDir(t)
	iconURL := "https://cooldown.test-domain.com/favicon.ico"
	key := siteFaviconCacheKey(iconURL)
	defer siteIconFailures.Delete(key)

	oldClient := siteIconHTTPClient
	defer func() { siteIconHTTPClient = oldClient }()
	var requests int32
	siteIconHTTPClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&requests, 1)
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: &net.DNSError{Err: "no such host", Name: req.URL.Hostname(), IsNotFound: true}}
	})}

	if _, _, err := FetchPublicSiteFavicon(iconURL); err == nil {
		t.Fatal("first unavailable favicon request should fail")
	}
	firstCount := atomic.LoadInt32(&requests)
	if _, _, err := FetchPublicSiteFavicon(iconURL); err == nil {
		t.Fatal("cooldown request should report the prior failure")
	}
	if got := atomic.LoadInt32(&requests); got != firstCount {
		t.Fatalf("cooldown performed another network request: before=%d after=%d", firstCount, got)
	}
}

func TestExpiredSiteFaviconFailureAllowsRetry(t *testing.T) {
	withSiteIconTempWorkingDir(t)
	iconURL := "https://retry.test-domain.com/favicon.ico"
	key := siteFaviconCacheKey(iconURL)
	siteIconFailures.Store(key, time.Now().Add(-time.Second))
	defer siteIconFailures.Delete(key)

	oldClient := siteIconHTTPClient
	defer func() { siteIconHTTPClient = oldClient }()
	pngData := encodeTestFavicon(t, "png")
	var requests int32
	siteIconHTTPClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		atomic.AddInt32(&requests, 1)
		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"image/png"}}, Body: io.NopCloser(bytes.NewReader(pngData)), Request: req}, nil
	})}

	if _, _, err := FetchPublicSiteFavicon(iconURL); err != nil {
		t.Fatalf("expired cooldown should allow retry: %v", err)
	}
	if got := atomic.LoadInt32(&requests); got != 1 {
		t.Fatalf("retry requests = %d, want 1", got)
	}
	if siteFaviconFailureActive(iconURL) {
		t.Fatal("successful retry should clear the cooldown")
	}
}
```

- [ ] **Step 2: Add a RED homepage asset-URL suppression test**

```go
func TestGetSiteFaviconAssetURLReturnsEmptyDuringFailureCooldown(t *testing.T) {
	bookmarkLink := "https://no-icon.test-domain.com/path"
	iconURL := GetSiteFaviconURL(bookmarkLink)
	key := siteFaviconCacheKey(iconURL)
	siteIconFailures.Store(key, time.Now().Add(time.Minute))
	defer siteIconFailures.Delete(key)

	if got := GetSiteFaviconAssetURL(bookmarkLink); got != "" {
		t.Fatalf("active failure should retain plain built-in bookmark icon, got %q", got)
	}
}
```

- [ ] **Step 3: Add a RED previous-generation cache test**

Write a valid icon under a key calculated with the literal previous generation
`2026-07-nxdomain`, then assert `readCachedSiteFavicon(iconURL)` fails and the
current key differs:

```go
	previousSum := sha256.Sum256([]byte("2026-07-nxdomain" + "\x00" + strings.TrimSpace(iconURL)))
	previousKey := fmt.Sprintf("%x", previousSum)
```

- [ ] **Step 4: Run the cooldown and generation tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'Test(FetchPublicSiteFaviconSuppressesImmediateRetryAfterFailure|ExpiredSiteFaviconFailureAllowsRetry|GetSiteFaviconAssetURLReturnsEmptyDuringFailureCooldown|ReadCachedSiteFaviconIgnoresPreviousProviderGeneration)' -count=1
```

Expected: build or assertion failure because the failure-cache symbols do not
exist and the generation still equals `2026-07-nxdomain`.

- [ ] **Step 5: Implement cooldown helpers and generation change**

Set:

```go
	siteIconCacheGeneration = "2026-07-origin-only"
	siteIconFailureTTL       = 5 * time.Minute
```

Add `siteIconFailures sync.Map` and:

```go
func siteFaviconFailureActive(iconURL string) bool {
	key := siteFaviconCacheKey(iconURL)
	value, ok := siteIconFailures.Load(key)
	if !ok {
		return false
	}
	retryAfter, ok := value.(time.Time)
	if !ok || !time.Now().Before(retryAfter) {
		siteIconFailures.CompareAndDelete(key, value)
		return false
	}
	return true
}

func recordSiteFaviconFailure(iconURL string) {
	siteIconFailures.Store(siteFaviconCacheKey(iconURL), time.Now().Add(siteIconFailureTTL))
}

func clearSiteFaviconFailure(iconURL string) {
	siteIconFailures.Delete(siteFaviconCacheKey(iconURL))
}
```

In `GetSiteFaviconAssetURL`, return empty when the proxyable icon URL has an
active cooldown. In `fetchAndCacheSiteFavicon`, retain the initial persistent
cache read, then check the cooldown before creating an in-flight request.
Record a failure only when `downloadSiteFavicon` returns an error. Clear the
failure only after `writeCachedSiteFavicon` succeeds.

- [ ] **Step 6: Run the focused favicon package and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -count=1
```

Expected: PASS with origin-only retrieval, finite cooldown, retry, and
generation invalidation coverage.

### Task 4: Regression and Integration Verification

**Files:**
- Verify: `internal/resources/assets/assets_test.go`
- Verify: `internal/pages/home/icon_test.go`
- Verify: all modified files and docs.

**Interfaces:**
- Consumes: the final site-icon fetch/cache/render behavior from Tasks 1-3.
- Produces: a locally committed, buildable fix with no unrelated staged files.

- [ ] **Step 1: Format modified Go files**

Run:

```powershell
.\.tools\go\bin\gofmt.exe -w internal\fn\favicon.go internal\fn\favicon_test.go internal\pages\home\home.go internal\pages\home\home_test.go
```

Expected: no formatting errors.

- [ ] **Step 2: Run focused cross-package favicon tests**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn ./internal/pages/home ./internal/resources/assets -count=1
```

Expected: PASS.

- [ ] **Step 3: Run full repository verification**

Run:

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
.\.tools\go\bin\go.exe vet ./...
.\.tools\go\bin\go.exe build ./...
git diff --check
```

Expected: all commands exit zero.

- [ ] **Step 4: Inspect the scoped diff and user-owned files**

Run:

```powershell
git diff -- internal/fn/favicon.go internal/fn/favicon_test.go internal/pages/home/home.go internal/pages/home/home_test.go docs/superpowers/specs/2026-07-17-favicon-refresh-and-default-fallback-design.md docs/superpowers/plans/2026-07-17-favicon-refresh-and-default-fallback.md
git status --short
```

Expected: only the planned favicon files are part of this change;
`fnapp/superflare/manifest` remains modified and
`tools/superflare-icon.zip` remains untracked but untouched.

- [ ] **Step 5: Commit only the favicon fix locally**

Run:

```powershell
git add -- internal/fn/favicon.go internal/fn/favicon_test.go internal/pages/home/home.go internal/pages/home/home_test.go docs/superpowers/specs/2026-07-17-favicon-refresh-and-default-fallback-design.md docs/superpowers/plans/2026-07-17-favicon-refresh-and-default-fallback.md
git commit -m "fix: stabilize favicon refresh fallback"
```

Expected: commit succeeds on local `main`; no remote push occurs and the two
user-owned workspace changes remain unstaged.
