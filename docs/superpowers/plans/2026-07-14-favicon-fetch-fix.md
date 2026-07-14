# Favicon Fetch Fix Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make favicon discovery work for large HTML pages and limit each page load to one browser request per unique favicon source while preserving the default placeholder on upstream failures.

**Architecture:** Keep the existing local favicon proxy and persistent cache. Change HTML discovery to scan a bounded stream without rejecting a large response, make the proxy route wait for the existing coalesced fetch on cache misses, and replace fixed browser polling with one deduplicated request per source URL.

**Tech Stack:** Go 1.26.x, Echo v5, `golang.org/x/net/html`, Go `httptest`, server-rendered inline JavaScript, Codex in-app Browser/CDP.

## Global Constraints

- Do not read Windows system proxy settings or add platform-specific proxy discovery.
- Preserve the built-in bookmark placeholder for DNS, connection, TLS, proxy, timeout, HTTP, parsing, and unsupported-content failures.
- Inspect no more than 512 KiB of an HTML page while discovering a favicon.
- Keep a finite 4 MiB favicon limit, source validation, redirect validation, cache location, and concurrent-fetch coalescing.
- Issue at most one browser request per unique favicon source during one page load.
- Keep behavior consistent across native Windows, Linux, Docker, and fnapp deployments.

## File Map

- `internal/fn/favicon.go`: bounded streaming HTML favicon discovery and upstream fetch/cache behavior.
- `internal/fn/favicon_test.go`: large-page discovery regression test.
- `internal/resources/assets/assets.go`: `/assets/site-icons` cache-miss behavior and placeholder response.
- `internal/resources/assets/assets_test.go`: route-level cache-miss success regression test; existing tests retain failure coverage.
- `internal/pages/home/home.go`: deduplicated one-shot favicon refresh script.
- `internal/pages/home/home_test.go`: script contract regression test.

---

### Task 1: Discover Favicons in a Bounded Prefix of Large HTML Pages

**Files:**
- Modify: `internal/fn/favicon.go:316-425`
- Test: `internal/fn/favicon_test.go:486-549`

**Interfaces:**
- Consumes: `siteIconHTMLBytes` and `relLooksLikeFavicon(string)`.
- Produces: `collectFaviconHrefs(io.Reader) ([]string, error)`, `faviconHrefFromAttributes([]xhtml.Attribute) (string, bool)`, and unchanged `discoverSiteFaviconFromHTML(string) (string, error)` behavior for callers.

- [ ] **Step 1: Write the failing large-page discovery test**

Add this test after `TestFetchPublicSiteFaviconDiscoversHTMLDeclaredIconWhenRootIcoFails`:

```go
func TestFetchPublicSiteFaviconDiscoversIconBeforeLargeHTMLBodyLimit(t *testing.T) {
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

	largeHTML := `<!doctype html><html><head><link rel="icon" href="/assets/favicon.svg"></head><body>` +
		strings.Repeat("x", siteIconHTMLBytes+1024) + `</body></html>`
	siteIconHTTPClient = &http.Client{
		Timeout: 2 * time.Second,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/favicon.ico":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     http.Header{"Content-Type": []string{"text/plain"}},
					Body:       io.NopCloser(strings.NewReader("not found")),
					Request:    req,
				}, nil
			case "/":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
					Body:       io.NopCloser(strings.NewReader(largeHTML)),
					Request:    req,
				}, nil
			case "/assets/favicon.svg":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"image/svg+xml"}},
					Body:       io.NopCloser(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg"><title>large-page-icon</title></svg>`)),
					Request:    req,
				}, nil
			default:
				t.Fatalf("unexpected favicon request path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	data, contentType, err := FetchPublicSiteFavicon("https://example.com/favicon.ico")
	if err != nil {
		t.Fatalf("FetchPublicSiteFavicon should discover an icon before the HTML limit: %v", err)
	}
	if contentType != "image/svg+xml" {
		t.Fatalf("discovered favicon content type = %q", contentType)
	}
	if !strings.Contains(string(data), "large-page-icon") {
		t.Fatalf("unexpected discovered favicon body: %q", string(data))
	}
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run TestFetchPublicSiteFaviconDiscoversIconBeforeLargeHTMLBodyLimit -count=1
```

Expected: FAIL because the current `discoverSiteFaviconFromHTML` returns `page response too large` internally and the original root favicon error reaches the test.

- [ ] **Step 3: Replace whole-document parsing with bounded tokenization**

In `discoverSiteFaviconFromHTML`, replace the `io.LimitReader(...+1)`, `io.ReadAll`, size rejection, and `xhtml.Parse` block with:

```go
	hrefs, err := collectFaviconHrefs(io.LimitReader(resp.Body, siteIconHTMLBytes))
	if err != nil {
		return "", err
	}
	for _, href := range hrefs {
		ref, err := url.Parse(strings.TrimSpace(href))
		if err != nil || ref == nil {
			continue
		}
		resolved := base.ResolveReference(ref)
		if isProxyableSiteFaviconURL(resolved.String()) {
			return resolved.String(), nil
		}
	}
	return "", fmt.Errorf("no html favicon found")
```

Replace the DOM-walking `collectFaviconHrefs` and `faviconHrefFromLinkNode` functions with:

```go
func collectFaviconHrefs(reader io.Reader) ([]string, error) {
	tokenizer := xhtml.NewTokenizer(reader)
	var out []string
	for {
		switch tokenizer.Next() {
		case xhtml.ErrorToken:
			err := tokenizer.Err()
			if err != nil && !errors.Is(err, io.EOF) {
				return nil, err
			}
			return out, nil
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			token := tokenizer.Token()
			if !strings.EqualFold(token.Data, "link") {
				continue
			}
			if href, ok := faviconHrefFromAttributes(token.Attr); ok {
				out = append(out, href)
			}
		case xhtml.EndTagToken:
			if strings.EqualFold(tokenizer.Token().Data, "head") {
				return out, nil
			}
		}
	}
}

func faviconHrefFromAttributes(attributes []xhtml.Attribute) (string, bool) {
	var rel string
	var href string
	for _, attr := range attributes {
		switch strings.ToLower(strings.TrimSpace(attr.Key)) {
		case "rel":
			rel = attr.Val
		case "href":
			href = strings.TrimSpace(attr.Val)
		}
	}
	if href == "" || !relLooksLikeFavicon(rel) {
		return "", false
	}
	return href, true
}
```

- [ ] **Step 4: Run focused and package tests and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'TestFetchPublicSiteFaviconDiscoversIconBeforeLargeHTMLBodyLimit|TestFetchPublicSiteFaviconDiscoversHTMLDeclaredIconWhenRootIcoFails' -count=1
.\.tools\go\bin\go.exe test ./internal/fn -count=1
```

Expected: PASS for both commands.

- [ ] **Step 5: Commit the discovery change**

```powershell
git add -- internal/fn/favicon.go internal/fn/favicon_test.go
git commit -m "fix: discover favicons in large pages"
```

---

### Task 2: Return the Fetched Icon on the First Proxy Request

**Files:**
- Modify: `internal/resources/assets/assets.go:212-238`
- Modify: `internal/resources/assets/assets_test.go:1-12`
- Test: `internal/resources/assets/assets_test.go:110-239`

**Interfaces:**
- Consumes: `fn.FetchPublicSiteFavicon(string) ([]byte, string, error)` and `readBuiltinBookmarkIcon()`.
- Produces: unchanged `/assets/site-icons?src=...` route with `cached` for successful cache-hit or cache-miss fetches and `fallback` for failures.

- [ ] **Step 1: Write the failing cache-miss route test**

Add `net/url` to the imports in `internal/resources/assets/assets_test.go`, then add:

```go
func TestSiteIconProxyCacheMissWaitsForSuccessfulFetch(t *testing.T) {
	setupAssetsConfigDir(t)
	define.Init()
	define.AppFlags.DebugMode = true

	const iconBody = `<svg xmlns="http://www.w3.org/2000/svg"><title>fetched-icon</title></svg>`
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/favicon.ico" {
			t.Fatalf("unexpected upstream path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(iconBody))
	}))
	defer upstream.Close()

	e := echo.New()
	RegisterRouting(e)
	iconURL := upstream.URL + "/favicon.ico"
	req := httptest.NewRequest(http.MethodGet, "/assets/site-icons?src="+url.QueryEscape(iconURL), nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("site icon proxy status = %d", rec.Code)
	}
	if got := rec.Header().Get(siteIconStateHeader); got != "cached" {
		t.Fatalf("site icon proxy state = %q, want cached", got)
	}
	if rec.Body.String() != iconBody {
		t.Fatalf("site icon proxy body = %q", rec.Body.String())
	}
}
```

- [ ] **Step 2: Run the route test and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/resources/assets -run TestSiteIconProxyCacheMissWaitsForSuccessfulFetch -count=1
```

Expected: FAIL because the current handler returns `X-SuperFlare-Site-Icon: fallback` before the warm-up fetch finishes.

- [ ] **Step 3: Make the route wait for the coalesced fetch**

Replace `serveSiteFavicon` with:

```go
func serveSiteFavicon(c *echo.Context) error {
	iconURL := strings.TrimSpace(c.QueryParam("src"))
	if iconURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing site favicon source")
	}

	data, contentType, err := fn.FetchPublicSiteFavicon(iconURL)
	if err == nil {
		if currentAssetsRuntime().DebugMode {
			c.Response().Header().Set("Cache-Control", "no-store")
		} else {
			c.Response().Header().Set("Cache-Control", "public, max-age=604800")
		}
		c.Response().Header().Set(siteIconStateHeader, "cached")
		c.Response().Header().Del("ETag")
		return c.Blob(http.StatusOK, contentType, data)
	}

	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set(siteIconStateHeader, "fallback")
	c.Response().Header().Del("ETag")
	fallback, fallbackContentType, fallbackErr := readBuiltinBookmarkIcon()
	if fallbackErr != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "site favicon fetch failed")
	}
	return c.Blob(http.StatusOK, fallbackContentType, fallback)
}
```

- [ ] **Step 4: Run route and package tests and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/resources/assets -run 'TestSiteIconProxyCacheMissWaitsForSuccessfulFetch|TestSiteIconProxyFallsBackToBuiltinBookmarkIcon|TestSiteIconProxyCacheHitServesCachedData' -count=1
.\.tools\go\bin\go.exe test ./internal/resources/assets -count=1
```

Expected: PASS for both commands. The existing failure test must still return the built-in SVG with `fallback`.

- [ ] **Step 5: Commit the route change**

```powershell
git add -- internal/resources/assets/assets.go internal/resources/assets/assets_test.go
git commit -m "fix: resolve favicon cache misses in one request"
```

---

### Task 3: Deduplicate Browser Requests and Remove Polling

**Files:**
- Modify: `internal/pages/home/home.go:95`
- Test: `internal/pages/home/home_test.go:112-142`

**Interfaces:**
- Consumes: `img[data-site-icon-src]` markers and `X-SuperFlare-Site-Icon` response state.
- Produces: `_inlineSiteIconRefreshScript` with one `fetch(src)` per unique source and no timer-based retry.

- [ ] **Step 1: Write the failing script contract test**

Add after the script nonce tests:

```go
func TestInlineSiteIconRefreshScriptDeduplicatesSourcesWithoutPolling(t *testing.T) {
	script := string(inlineSiteIconRefreshScript(model.Application{IconMode: define.IconModeMissingFill}))
	for _, expected := range []string{
		`var groups=new Map()`,
		`groups.get(src)`,
		`groups.set(src,[img])`,
		`groups.forEach(function(group,src)`,
		`group.forEach(function(img)`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("favicon refresh script should contain %q: %s", expected, script)
		}
	}
	if got := strings.Count(script, `fetch(src)`); got != 1 {
		t.Fatalf("favicon refresh script should contain one grouped fetch call, got %d: %s", got, script)
	}
	for _, unexpected := range []string{`setTimeout`, `var left=`, `cache:"no-store"`} {
		if strings.Contains(script, unexpected) {
			t.Fatalf("favicon refresh script should not contain %q: %s", unexpected, script)
		}
	}
}
```

- [ ] **Step 2: Run the script test and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/pages/home -run TestInlineSiteIconRefreshScriptDeduplicatesSourcesWithoutPolling -count=1
```

Expected: FAIL because the current script has no grouped request map and contains `setTimeout` polling.

- [ ] **Step 3: Replace the polling script with grouped one-shot fetching**

Replace `_inlineSiteIconRefreshScript` with:

```go
const _inlineSiteIconRefreshScript = `(function(){var nodes=Array.prototype.slice.call(document.querySelectorAll("img[data-site-icon-src]"));if(!nodes.length||!window.fetch||!window.URL||!URL.createObjectURL){return;}var groups=new Map();nodes.forEach(function(img){var src=img.dataset.siteIconSrc;if(!src){return;}var group=groups.get(src);if(group){group.push(img);}else{groups.set(src,[img]);}});var objectUrls=[];groups.forEach(function(group,src){fetch(src).then(function(res){if(!res.ok||res.headers.get("X-SuperFlare-Site-Icon")!=="cached"){return null;}return res.blob();}).then(function(blob){if(!blob){return;}var objectUrl=URL.createObjectURL(blob);objectUrls.push(objectUrl);group.forEach(function(img){img.src=objectUrl;img.dataset.siteIconDone="1";});}).catch(function(){});});window.addEventListener("pagehide",function(){objectUrls.forEach(function(objectUrl){URL.revokeObjectURL(objectUrl);});},{once:true});}());`
```

- [ ] **Step 4: Run focused and package tests and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/pages/home -run 'TestInlineSiteIconRefreshScriptDeduplicatesSourcesWithoutPolling|TestRenderBookmarkIcon_CacheMissMarksFallbackForAsyncSiteFavicon' -count=1
.\.tools\go\bin\go.exe test ./internal/pages/home -count=1
```

Expected: PASS for both commands.

- [ ] **Step 5: Commit the browser scheduling change**

```powershell
git add -- internal/pages/home/home.go internal/pages/home/home_test.go
git commit -m "fix: deduplicate favicon refresh requests"
```

---

### Task 4: Full Verification and Rendered Browser QA

**Files:**
- Verify only: all files changed in Tasks 1-3.

**Interfaces:**
- Consumes: completed discovery, proxy route, and browser scheduling changes.
- Produces: verification evidence for the complete favicon flow; no new production interface.

- [ ] **Step 1: Check formatting without changing unrelated files**

Run:

```powershell
$files = @(
  'internal/fn/favicon.go',
  'internal/fn/favicon_test.go',
  'internal/resources/assets/assets.go',
  'internal/resources/assets/assets_test.go',
  'internal/pages/home/home.go',
  'internal/pages/home/home_test.go'
)
.\.tools\go\bin\gofmt.exe -s -l $files
git diff --check
```

Expected: no output from either command.

- [ ] **Step 2: Run all automated verification**

Run:

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
.\.tools\go\bin\go.exe build ./...
.\tools\test-all-scripts.ps1
.\tools\test-script-safety.ps1
```

Expected: all Go tests, build, and script checks pass. Docker Compose validation may report that Docker is unavailable and skip only that optional check.

- [ ] **Step 3: Start a local verification server**

Build a temporary executable outside the repository and start it on an unused local port with no platform-specific proxy configuration:

```powershell
.\.tools\go\bin\go.exe build -o "$env:TEMP\superflare-favicon-verify.exe" .
$verificationProcess = Start-Process -FilePath "$env:TEMP\superflare-favicon-verify.exe" -ArgumentList '--port=3649' -WorkingDirectory (Get-Location) -WindowStyle Hidden -PassThru
$verificationProcess.Id
```

Expected: `http://127.0.0.1:3649/` returns the SuperFlare home page.

- [ ] **Step 4: Verify one request and stable placeholder in the in-app Browser**

Use the Browser/CDP network event flow on `http://127.0.0.1:3649/`:

1. Enable `Network` events before reloading.
2. Reload the page and observe for at least 10 seconds.
3. Filter `Network.requestWillBeSent` events for the known unreachable bookmark source `www.bing123google.com/favicon.ico`.
4. Assert exactly one `/assets/site-icons` request for that source.
5. Assert its marked image still uses the built-in `bookmark.svg` and has no `data-site-icon-done` attribute.
6. Confirm page URL/title, meaningful DOM content, no framework overlay, no relevant console errors, and capture a viewport screenshot.

Expected: one request, visible placeholder, functional page, and no relevant console errors.

- [ ] **Step 5: Confirm the working tree contains only intended changes**

Stop the exact temporary server process, remove its temporary executable, and inspect Git:

```powershell
$listener = Get-NetTCPConnection -LocalPort 3649 -State Listen -ErrorAction SilentlyContinue
if ($listener) {
  $process = Get-Process -Id $listener.OwningProcess -ErrorAction Stop
  $expectedPath = [System.IO.Path]::GetFullPath("$env:TEMP\superflare-favicon-verify.exe")
  if ([System.IO.Path]::GetFullPath($process.Path) -ne $expectedPath) {
    throw "Port 3649 belongs to an unexpected process: $($process.Path)"
  }
  Stop-Process -Id $process.Id -Force
  $process.WaitForExit()
}
Remove-Item -LiteralPath "$env:TEMP\superflare-favicon-verify.exe" -Force -ErrorAction SilentlyContinue
git status --short --branch
git log --oneline --decorate -n 6
```

Expected: clean working tree after the three implementation commits, with the current branch ahead of `origin/main` only by the design, plan, and favicon-fix commits.

---

### Task 5: Relax the Favicon Payload Limit and Preserve Cache Reuse

**Files:**
- Modify: `internal/fn/favicon.go:23-29`
- Modify: `internal/fn/favicon_test.go`
- Modify: `docs/superpowers/specs/2026-07-14-favicon-fetch-design.md`
- Modify: `docs/superpowers/plans/2026-07-14-favicon-fetch-fix.md`

**Interfaces:**
- Consumes: `FetchPublicSiteFavicon(string) ([]byte, string, error)` and the existing persistent favicon cache.
- Produces: unchanged fetch/cache API with a 4 MiB payload limit.

- [ ] **Step 1: Write the failing large-icon cache test**

Add to `internal/fn/favicon_test.go`:

```go
func TestFetchPublicSiteFaviconAcceptsLargeIconAndCachesIt(t *testing.T) {
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	const formerLimit = 256 * 1024
	iconBody := `<svg xmlns="http://www.w3.org/2000/svg">` +
		strings.Repeat("x", formerLimit*4) + `</svg>`
	var upstreamRequests int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&upstreamRequests, 1)
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(iconBody))
	}))
	defer server.Close()

	oldClient := siteIconHTTPClient
	siteIconHTTPClient = server.Client()
	defer func() { siteIconHTTPClient = oldClient }()

	iconURL := server.URL + "/favicon.svg"
	for attempt := 0; attempt < 2; attempt++ {
		data, contentType, err := FetchPublicSiteFavicon(iconURL)
		if err != nil {
			t.Fatalf("FetchPublicSiteFavicon attempt %d: %v", attempt+1, err)
		}
		if contentType != "image/svg+xml" {
			t.Fatalf("large favicon content type = %q", contentType)
		}
		if len(data) != len(iconBody) {
			t.Fatalf("large favicon size = %d, want %d", len(data), len(iconBody))
		}
	}
	if got := atomic.LoadInt32(&upstreamRequests); got != 1 {
		t.Fatalf("large favicon upstream requests = %d, want 1", got)
	}
}
```

- [ ] **Step 2: Run the focused test and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run '^TestFetchPublicSiteFaviconAcceptsLargeIconAndCachesIt$' -count=1 -v
```

Expected: FAIL with `favicon too large` because the payload exceeds 256 KiB.

- [ ] **Step 3: Raise the bounded payload limit**

Change the constant in `internal/fn/favicon.go`:

```go
siteIconMaxBytes = 4 * 1024 * 1024
```

Keep the existing `io.LimitReader(resp.Body, siteIconMaxBytes+1)` and oversize
check unchanged so untrusted responses remain bounded.

- [ ] **Step 4: Run focused, package, and full tests**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run '^TestFetchPublicSiteFaviconAcceptsLargeIconAndCachesIt$' -count=1 -v
.\.tools\go\bin\go.exe test ./internal/fn -count=1
.\.tools\go\bin\go.exe test ./... -count=1
```

Expected: PASS for all commands; the test records one upstream request across
two fetch calls.

- [ ] **Step 5: Verify formatting, build, and create a local commit**

Run:

```powershell
.\.tools\go\bin\gofmt.exe -s -l internal\fn\favicon.go internal\fn\favicon_test.go
git diff --check
.\.tools\go\bin\go.exe build ./...
git add -- internal/fn/favicon.go internal/fn/favicon_test.go docs/superpowers/specs/2026-07-14-favicon-fetch-design.md docs/superpowers/plans/2026-07-14-favicon-fetch-fix.md
git commit -m "fix: allow larger favicon payloads"
```

Expected: formatting and build checks pass; commit remains local and is not
pushed to any remote.
