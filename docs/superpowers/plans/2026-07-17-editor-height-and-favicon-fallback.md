# Editor Auto Height and Favicon Fallback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make both editor tables expand to their full rendered height, remove the public favicon `v=2` marker, and keep the built-in bookmark icon for definitively nonexistent domains.

**Architecture:** Use Handsontable's native auto-height mode instead of fixed row arithmetic. Keep browser URLs stable with only `src`, invalidate unsafe legacy disk entries through an internal cache generation, and stop the hosted favicon fallback only when the direct error chain contains a definitive DNS NXDOMAIN.

**Tech Stack:** Go 1.x, Go templates, Handsontable 6.2.2, embedded assets, Go `net`/`net/http`, repository build generator.

## Global Constraints

- Apply auto height to both the category table and the application/bookmark table.
- Preserve horizontal table scrolling and the existing 320-pixel maximum column width.
- Do not add any browser-visible favicon cache-version parameter.
- Do not special-case development, Docker, Linux, Windows, or fnapp environments.
- Preserve all current configuration formats and bookmark data.
- Leave `fnapp/superflare/manifest` untouched.
- Commit locally on `main`; do not push.

---

### Task 1: Make Both Editor Tables Auto Height

**Files:**
- Modify: `internal/resources/templates/editor_template_test.go`
- Modify: `embed/templates/editor.html`
- Generate: `internal/resources/templates/html/editor.html`

**Interfaces:**
- Consumes: existing Handsontable instances and `scheduleTableLayoutSync()` calls.
- Produces: both tables configured with `height: 'auto'`, `renderAllRows: true`, and no fixed-height updates.

- [ ] **Step 1: Write the failing template contract test**

Add after `TestEditorTemplateConstrainsTableCellWidths`:

```go
func TestEditorTemplateTablesGrowToRenderedHeight(t *testing.T) {
	raw, err := TPL.ReadFile("html/editor.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)

	if got := strings.Count(page, `height: 'auto',`); got != 2 {
		t.Fatalf("expected both editor tables to use auto height, got %d settings", got)
	}
	if got := strings.Count(page, `renderAllRows: true,`); got != 2 {
		t.Fatalf("expected both editor tables to render all rows, got %d settings", got)
	}
	for _, unexpected := range []string{
		`TABLE_ROW_HEIGHT`,
		`TABLE_HEADER_HEIGHT`,
		`TABLE_FRAME_HEIGHT`,
		`tableHeightForRows`,
		`lastCategoryTableHeight`,
		`lastBookmarkTableHeight`,
	} {
		if strings.Contains(page, unexpected) {
			t.Fatalf("editor template should not retain fixed-height behavior %q", unexpected)
		}
	}
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/resources/templates -run '^TestEditorTemplateTablesGrowToRenderedHeight$' -count=1
```

Expected: FAIL because neither table currently has `height: 'auto'` and the fixed-height constants remain.

- [ ] **Step 3: Replace fixed sizing with native auto height**

In `embed/templates/editor.html`:

- remove `TABLE_ROW_HEIGHT`, `TABLE_HEADER_HEIGHT`, and `TABLE_FRAME_HEIGHT`;
- remove `tableHeightForRows`;
- set both table configurations to:

```javascript
height: 'auto',
renderAllRows: true,
autoRowSize: true,
wordWrap: true,
```

- remove `lastCategoryTableHeight` and `lastBookmarkTableHeight`;
- replace `syncTableLayouts` with:

```javascript
function syncTableLayouts() {
    instanceCategories.render();
    instanceBookmarks.render();
}
```

- keep `scheduleTableLayoutSync` unchanged;
- remove the `height` property from `updateDataTable`:

```javascript
instanceBookmarks.updateSettings({
    columns: buildDataTableColumns()
})
```

- [ ] **Step 4: Regenerate templates and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe run ./build
.\.tools\go\bin\go.exe test ./internal/resources/templates -run 'TestEditorTemplateTablesGrowToRenderedHeight|TestEditorTemplateConstrainsTableCellWidths' -count=1
```

Expected: PASS. The generated template is synchronized and both the height and width contracts remain enforced.

- [ ] **Step 5: Commit the editor change**

```powershell
git add -- embed/templates/editor.html internal/resources/templates/editor_template_test.go internal/resources/templates/html/editor.html
git commit -m "fix: let editor tables grow to full height"
```

---

### Task 2: Remove the Public Favicon Version and Invalidate Legacy Disk Entries

**Files:**
- Modify: `internal/fn/favicon.go`
- Modify: `internal/fn/favicon_test.go`
- Modify: `internal/pages/home/icon_test.go`

**Interfaces:**
- Consumes: `siteIconProxyURL(string) string` and the existing disk cache directory.
- Produces: `/assets/site-icons?src=...` URLs and deterministic generation-scoped cache filenames.

- [ ] **Step 1: Change existing URL expectations and add a failing legacy-cache test**

Update the six current favicon URL expectations in `internal/fn/favicon_test.go` and `internal/pages/home/icon_test.go` to contain only the encoded `src` value, with no `v=2` suffix.

Add `crypto/sha256` to `internal/fn/favicon_test.go`, then add:

```go
func TestReadCachedSiteFaviconIgnoresLegacyCacheKey(t *testing.T) {
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	iconURL := "https://invalid-letter-cache.example/favicon.ico"
	legacySum := sha256.Sum256([]byte(strings.TrimSpace(iconURL)))
	legacyKey := fmt.Sprintf("%x", legacySum)
	legacyPath := filepath.Join(tmpDir, "var", "cache", "site-icons", legacyKey+".bin")
	if err := os.MkdirAll(filepath.Dir(legacyPath), 0755); err != nil {
		t.Fatalf("MkdirAll cache: %v", err)
	}
	if err := os.WriteFile(legacyPath, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0644); err != nil {
		t.Fatalf("WriteFile legacy cache: %v", err)
	}

	if _, _, err := readCachedSiteFavicon(iconURL); err == nil {
		t.Fatal("legacy favicon cache entry should be ignored")
	}
	if got := SiteFaviconCacheKeyForTest(iconURL); got == legacyKey {
		t.Fatal("current favicon cache key should differ from the legacy URL-only key")
	}
}
```

- [ ] **Step 2: Run focused tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'TestGetSiteFavicon_ValidURL|TestGetSiteFaviconAssetURL_PublicUsesProxy|TestGetSiteFaviconAssetURL_LocalUsesProxyFallbackRoute|TestReadCachedSiteFaviconIgnoresLegacyCacheKey' -count=1
.\.tools\go\bin\go.exe test ./internal/pages/home -run 'TestRenderBookmarkIcon_CacheHitRendersDirectlyWithoutAsyncFallback|TestRenderBookmarkIcon_InvalidCacheKeepsAsyncFallback' -count=1
```

Expected: FAIL because generated proxy URLs still include `v=2` and the URL-only disk key still reads the legacy file.

- [ ] **Step 3: Implement the stable URL and internal generation**

Replace the public version constant with an internal generation:

```go
siteIconCacheGeneration = "2026-07-nxdomain"
```

Replace `siteIconProxyURL` and update the cache key:

```go
func siteIconProxyURL(iconURL string) string {
	query := url.Values{"src": {iconURL}}
	return siteIconProxyPath + "?" + query.Encode()
}

func siteFaviconCacheKey(iconURL string) string {
	normalizedURL := strings.TrimSpace(iconURL)
	sum := sha256.Sum256([]byte(siteIconCacheGeneration + "\x00" + normalizedURL))
	return fmt.Sprintf("%x", sum)
}
```

- [ ] **Step 4: Run focused and package tests and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'TestGetSiteFavicon_ValidURL|TestGetSiteFaviconAssetURL_PublicUsesProxy|TestGetSiteFaviconAssetURL_LocalUsesProxyFallbackRoute|TestReadCachedSiteFaviconIgnoresLegacyCacheKey' -count=1
.\.tools\go\bin\go.exe test ./internal/pages/home -run 'TestRenderBookmarkIcon_CacheHitRendersDirectlyWithoutAsyncFallback|TestRenderBookmarkIcon_InvalidCacheKeepsAsyncFallback' -count=1
.\.tools\go\bin\go.exe test ./internal/fn ./internal/pages/home -count=1
```

Expected: PASS and no generated homepage site-icon URL contains `v=2`.

- [ ] **Step 5: Commit the URL and cache change**

```powershell
git add -- internal/fn/favicon.go internal/fn/favicon_test.go internal/pages/home/icon_test.go
git commit -m "fix: keep favicon cache versions internal"
```

---

### Task 3: Stop Hosted Fallback for Definitive NXDOMAIN

**Files:**
- Modify: `internal/fn/favicon.go`
- Modify: `internal/fn/favicon_test.go`

**Interfaces:**
- Consumes: the direct root favicon error returned by `downloadSiteFaviconDirect`.
- Produces: an error for definitive DNS name-not-found results, allowing the existing asset route to return its built-in SVG fallback.

- [ ] **Step 1: Write the failing NXDOMAIN provider test**

Add `net` to `internal/fn/favicon_test.go`, then add:

```go
func TestFetchPublicSiteFaviconSkipsHostedFallbackForNXDOMAIN(t *testing.T) {
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	oldClient := siteIconHTTPClient
	defer func() { siteIconHTTPClient = oldClient }()
	var providerRequests int32
	siteIconHTTPClient = &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host == siteIconFallbackHost {
			atomic.AddInt32(&providerRequests, 1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"image/png"}},
				Body:       io.NopCloser(bytes.NewReader(encodeTestFavicon(t, "png"))),
				Request:    req,
			}, nil
		}
		return nil, &net.OpError{
			Op:  "dial",
			Net: "tcp",
			Err: &net.DNSError{Err: "no such host", Name: req.URL.Hostname(), IsNotFound: true},
		}
	})}

	iconURL := "https://definitely-missing.test-domain.com/favicon.ico"
	_, _, err = FetchPublicSiteFavicon(iconURL)
	if err == nil {
		t.Fatal("NXDOMAIN favicon fetch should fail and use the built-in route fallback")
	}
	var dnsErr *net.DNSError
	if !errors.As(err, &dnsErr) || !dnsErr.IsNotFound {
		t.Fatalf("favicon error should retain NXDOMAIN, got %v", err)
	}
	if got := atomic.LoadInt32(&providerRequests); got != 0 {
		t.Fatalf("hosted fallback requests = %d, want 0", got)
	}
	if _, statErr := os.Stat(filepath.Join(tmpDir, siteIconCacheDir, SiteFaviconCacheKeyForTest(iconURL)+".bin")); !os.IsNotExist(statErr) {
		t.Fatalf("NXDOMAIN favicon should not be cached, statErr=%v", statErr)
	}
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run '^TestFetchPublicSiteFaviconSkipsHostedFallbackForNXDOMAIN$' -count=1
```

Expected: FAIL because the current network-error path calls Icon Horse and accepts its PNG.

- [ ] **Step 3: Implement definitive DNS detection**

Add:

```go
func isDefinitiveSiteFaviconDNSNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}
```

Immediately after the direct root favicon request fails and the root URL check passes, add:

```go
if isDefinitiveSiteFaviconDNSNotFound(err) {
	return nil, "", fmt.Errorf("favicon host does not resolve: %w", err)
}
```

Do not change timeout, connection, HTTP-status, HTML discovery, or hosted-provider behavior for any other error.

- [ ] **Step 4: Run focused and package tests and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'TestFetchPublicSiteFaviconSkipsHostedFallbackForNXDOMAIN|TestFetchPublicSiteFaviconUsesHostedFallbackWhenOriginUnavailable|TestFetchPublicSiteFaviconBoundsWholeFallbackChain' -count=1
.\.tools\go\bin\go.exe test ./internal/fn -count=1
.\.tools\go\bin\go.exe test ./internal/resources/assets -run 'TestSiteIconProxyFallsBackToBuiltinBookmarkIcon|TestSiteIconProxyCacheHitServesCachedData' -count=1
```

Expected: PASS. NXDOMAIN skips the provider, while a valid-domain origin failure still uses the provider and route failures still serve the built-in bookmark SVG.

- [ ] **Step 5: Commit the DNS fix**

```powershell
git add -- internal/fn/favicon.go internal/fn/favicon_test.go
git commit -m "fix: reject hosted icons for missing domains"
```

---

### Task 4: Full Verification and Browser QA

**Files:**
- Verify only: files changed in Tasks 1-3.

**Interfaces:**
- Consumes: completed editor and favicon changes.
- Produces: fresh automated and rendered evidence; no new production interface.

- [ ] **Step 1: Verify formatting and generated-template synchronization**

Run:

```powershell
.\.tools\go\bin\gofmt.exe -s -l internal\fn\favicon.go internal\fn\favicon_test.go internal\pages\home\icon_test.go internal\resources\templates\editor_template_test.go
.\.tools\go\bin\go.exe run ./build
git diff --check
```

Expected: no Go files are listed by `gofmt`, template generation produces no new uncommitted drift, and `git diff --check` reports no errors.

- [ ] **Step 2: Run targeted verification**

```powershell
.\.tools\go\bin\go.exe test ./internal/resources/templates -count=1
.\.tools\go\bin\go.exe test ./internal/fn -count=1
.\.tools\go\bin\go.exe test ./internal/pages/home -count=1
.\.tools\go\bin\go.exe test ./internal/resources/assets -count=1
.\.tools\go\bin\go.exe vet ./internal/fn ./internal/pages/home ./internal/resources/assets ./internal/resources/templates
```

Expected: all commands pass.

- [ ] **Step 3: Run full verification**

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
.\.tools\go\bin\go.exe build ./...
```

Expected: all tests pass and the complete project builds.

- [ ] **Step 4: Run rendered browser checks**

Start a locally built SuperFlare instance on an unused port. In the browser:

1. Open `/editor` and put long wrapping content in both tables.
2. Insert and remove rows in both tables.
3. Verify neither table has a vertical scrollbar and each panel reaches the final row.
4. Verify the bookmark table still contains its horizontal scrollbar when needed.
5. Open the homepage and inspect site-icon `src` and `data-site-icon-src` values; none may contain `v=2`.
6. Use an NXDOMAIN bookmark and verify the visible icon remains the built-in bookmark SVG after the request finishes.
7. Reload and verify a valid cached favicon renders directly without a placeholder transition.
8. Check the console for relevant errors and capture desktop screenshots of `/editor` and the homepage.

- [ ] **Step 5: Confirm repository scope**

Run:

```powershell
git status --short --branch
git diff HEAD^ --check
git log --oneline --decorate -n 8
```

Expected: `fnapp/superflare/manifest` remains the only unrelated dirty file, all task changes are committed locally on `main`, and nothing is pushed.
