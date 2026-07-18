# Verified Hosted Favicon Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Restore real favicons for origins unreachable from SuperFlare while rejecting provider-generated and unknown fallback images.

**Architecture:** Keep the existing origin and HTML discovery paths, then add one fail-closed `favicon.im` recovery path using its strict 404 mode and positive provenance header. Clone the existing HTTP client for provider requests so redirects can be restricted without changing ordinary origin behavior, and cache verified bytes under the original destination key.

**Tech Stack:** Go 1.26.x, `net/http`, Echo v5 route integration, Go `httptest` and custom `RoundTripper` tests, server-rendered inline JavaScript.

## Global Constraints

- Direct `/favicon.ico` and HTML-declared icons remain preferred sources.
- Provider responses are accepted only for `X-Favicon-Source: origin`, `cache-fresh`, or `cache-stale`.
- Provider `default`, missing/unknown provenance, non-2xx status, invalid image, and unsafe redirect responses must retain the built-in bookmark SVG.
- Never send local, private, IP-address, reserved, or definitive NXDOMAIN hosts to a provider.
- Provider redirects are HTTPS-only and limited to `favicon.im` and `a.favicon.im` with unchanged path and strict-mode query.
- Keep a ten-second overall deadline, four-second origin timeout, six-second hosted timeout, 4 MiB body limit, 4-megapixel decoded limit, 512 KiB HTML scan, five-minute failure cooldown, 1024 failure-entry limit, in-flight coalescing, and atomic success cache.
- Do not add configuration fields, migrations, platform-specific proxy discovery, or public `v` parameters.
- Preserve user-owned `fnapp/superflare/manifest` and `tools/superflare-icon.zip` changes.
- Work directly on local `main`; do not push.

## File Map

- `internal/fn/favicon.go`: provider URL/privacy policy, strict provenance validation, provider-specific redirect policy, retrieval ordering, and cache generation.
- `internal/fn/favicon_test.go`: provider, redirect, ordering, rejection, cache, and generation regression tests.
- `internal/pages/home/home.go`: browser repair marker generation only.
- `internal/pages/home/home_test.go`: repair marker and stable public URL assertions.
- `docs/superpowers/specs/2026-07-18-verified-hosted-favicon-recovery-design.md`: approved design and compatibility contract.
- `docs/superpowers/plans/2026-07-18-verified-hosted-favicon-recovery.md`: executable implementation steps.

---

### Task 1: Define the Provider Privacy and Provenance Contract

**Files:**
- Modify: `internal/fn/favicon_test.go`
- Modify: `internal/fn/favicon.go`

**Interfaces:**
- Produces: `hostedSiteFaviconURL(string) string`, `isReservedSiteFaviconHost(string) bool`, `isTrustedHostedSiteFaviconSource(string) bool`, and `validateHostedSiteFaviconResponse(*http.Response) error`.
- Consumes: `HostLooksLocalNetwork`, `net.ParseIP`, and the root destination favicon URL.

- [ ] **Step 1: Add provider URL and source RED tests**

Add table tests requiring:

```go
func TestHostedSiteFaviconURLUsesOnlyPublicDomainNames(t *testing.T) {
	if got := hostedSiteFaviconURL("https://wallroom.io/favicon.ico"); got != "https://favicon.im/wallroom.io?throw-error-on-404=true" {
		t.Fatalf("public hosted favicon URL = %q", got)
	}
	for _, input := range []string{
		"http://192.168.1.20/favicon.ico",
		"https://localhost/favicon.ico",
		"https://nas.local/favicon.ico",
		"https://intranet/favicon.ico",
		"https://example.invalid/favicon.ico",
		"https://service.test/favicon.ico",
		"https://icon.example/favicon.ico",
		"https://service.internal/favicon.ico",
		"https://hidden.onion/favicon.ico",
		"https://preview.alt/favicon.ico",
	} {
		if got := hostedSiteFaviconURL(input); got != "" {
			t.Fatalf("private hosted favicon URL for %q = %q, want empty", input, got)
		}
	}
}

func TestTrustedHostedSiteFaviconSourcesFailClosed(t *testing.T) {
	for _, source := range []string{"origin", " cache-fresh ", "CACHE-STALE"} {
		if !isTrustedHostedSiteFaviconSource(source) {
			t.Fatalf("expected trusted hosted source %q", source)
		}
	}
	for _, source := range []string{"", "default", "cache", "unknown", "origin,default"} {
		if isTrustedHostedSiteFaviconSource(source) {
			t.Fatalf("unexpected trusted hosted source %q", source)
		}
	}
}
```

- [ ] **Step 2: Run provider contract tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'Test(HostedSiteFaviconURLUsesOnlyPublicDomainNames|TrustedHostedSiteFaviconSourcesFailClosed)' -count=1
```

Expected: build failure because the provider helpers do not exist.

- [ ] **Step 3: Implement the provider URL and source allowlist**

Add constants for `favicon.im`, `a.favicon.im`, `X-Favicon-Source`, and the
strict query. Reintroduce the previous public-host filtering logic with the new
provider URL:

```go
func hostedSiteFaviconURL(rootIconURL string) string {
	u, err := url.Parse(strings.TrimSpace(rootIconURL))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(u.Hostname())), ".")
	if host == "" || HostLooksLocalNetwork(host) || net.ParseIP(host) != nil || isReservedSiteFaviconHost(host) {
		return ""
	}
	query := url.Values{"throw-error-on-404": {"true"}}
	return (&url.URL{Scheme: "https", Host: "favicon.im", Path: "/" + host, RawQuery: query.Encode()}).String()
}

func isTrustedHostedSiteFaviconSource(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "origin", "cache-fresh", "cache-stale":
		return true
	default:
		return false
	}
}
```

`validateHostedSiteFaviconResponse` must reject nil responses, duplicate source
headers, and any source outside that allowlist.

- [ ] **Step 4: Run the provider contract tests and verify GREEN**

Run the Step 2 command again. Expected: PASS.

### Task 2: Restrict Provider Redirects

**Files:**
- Modify: `internal/fn/favicon_test.go`
- Modify: `internal/fn/favicon.go`

**Interfaces:**
- Produces: `validateHostedSiteFaviconRedirect(*http.Request, []*http.Request) error` and `hostedSiteFaviconHTTPClient() *http.Client`.
- Consumes: `siteIconHTTPClient` transport/timeout and the initial provider request in `via[0]`.

- [ ] **Step 1: Add redirect-policy RED tests**

Create requests for the initial
`https://favicon.im/wallroom.io?throw-error-on-404=true` URL. Assert that a
redirect to the same path/query on `https://a.favicon.im` succeeds. Table-test
rejection of:

```text
http://a.favicon.im/wallroom.io?throw-error-on-404=true
https://evil.example/wallroom.io?throw-error-on-404=true
https://a.favicon.im/changed.example?throw-error-on-404=true
https://a.favicon.im/wallroom.io
https://user@a.favicon.im/wallroom.io?throw-error-on-404=true
https://a.favicon.im:444/wallroom.io?throw-error-on-404=true
```

- [ ] **Step 2: Run redirect tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run TestValidateHostedSiteFaviconRedirect -count=1
```

Expected: build failure because the strict redirect validator does not exist.

- [ ] **Step 3: Implement strict redirect validation and client cloning**

The validator must enforce the redirect count, scheme, exact host allowlist,
empty userinfo/default port, and path/query equality with `via[0].URL`.

Clone the ordinary client so tests and configured transport behavior are
preserved while only the redirect callback changes:

```go
func hostedSiteFaviconHTTPClient() *http.Client {
	client := *siteIconHTTPClient
	client.CheckRedirect = validateHostedSiteFaviconRedirect
	return &client
}
```

- [ ] **Step 4: Run redirect tests and verify GREEN**

Run the Step 2 command again. Expected: PASS.

### Task 3: Add Verified Hosted Recovery to the Retrieval Chain

**Files:**
- Modify: `internal/fn/favicon_test.go`
- Modify: `internal/fn/favicon.go`

**Interfaces:**
- Produces: `downloadHostedSiteFavicon(context.Context, string) ([]byte, string, error)`, `shouldPreferHostedSiteFavicon(error) bool`, `siteIconHostedTimeout`, and a shared response-download helper.
- Consumes: provider helpers from Tasks 1-2 and existing direct/HTML fetch functions.

- [ ] **Step 1: Add verified success and ordering RED tests**

Add `TestFetchPublicSiteFaviconUsesVerifiedHostedFallbackForNetworkFailure`.
The fake transport must return a `net.OpError` for the destination favicon,
then return a valid icon for `favicon.im` with:

```go
http.Header{
	"Content-Type":     []string{"image/png"},
	"X-Favicon-Source": []string{"cache-fresh"},
}
```

Assert request order is destination `/favicon.ico`, then provider, with no
destination HTML request. Call `FetchPublicSiteFavicon` twice and assert the
provider is requested once because success is cached under the destination
key.

- [ ] **Step 2: Add provider rejection RED tests**

Table-test `default`, blank, and `unknown` provenance on otherwise valid image
bodies. Also test provider HTTP 404. Every case must return an error, write no
destination cache file, and retain the provider request in the aggregate error.

- [ ] **Step 3: Preserve HTML preference with a RED ordering test**

For a direct 404, return HTML declaring `/real.svg`, serve a valid SVG there,
and fail the test if any provider host is contacted. This locks in origin HTML
preference for non-network failures.

Also cover favicon redirects rejected for userinfo or a non-HTTP(S) scheme.
Mark all ordinary redirect policy failures with `errSiteFaviconRedirectRejected` and make
`shouldPreferHostedSiteFavicon` return false for that sentinel through the
outer `url.Error`; the HTML-declared icon must still win.

Create a real `http.Client` malformed-Location error and require HTML ordering.
Recursively unwrap `url.Error` before classifying its underlying error as
network-related. Keep explicit early recovery for deadline, `net.Error`, EOF,
TLS certificate verification, and TLS record-header failures, with a dedicated
TLS regression test.

- [ ] **Step 4: Run recovery tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run 'TestFetchPublicSiteFavicon(UsesVerifiedHostedFallbackForNetworkFailure|RejectsHosted|PrefersHTMLDiscoveryOverHosted)' -count=1
```

Expected: failures because no hosted recovery is attempted.

- [ ] **Step 5: Refactor response reading without changing validation**

Extract the body/status/image logic from `downloadSiteFaviconDirect` into:

```go
func downloadSiteFaviconWithClient(
	ctx context.Context,
	iconURL string,
	client *http.Client,
	validateResponse func(*http.Response) error,
) ([]byte, string, error)
```

`downloadSiteFaviconDirect` calls it with `siteIconHTTPClient` and nil.
`downloadHostedSiteFavicon` calls it with the cloned provider client and
`validateHostedSiteFaviconResponse`. Run the validator after 2xx status and
before reading the body.

- [ ] **Step 6: Restore ordered hosted recovery**

In `downloadSiteFavicon`, keep NXDOMAIN short-circuiting. For network errors or
deadlines, attempt verified hosted recovery before HTML discovery. For other
errors, try HTML first and hosted recovery last. Attempt the provider at most
once and join labeled errors from every attempted path.

- [ ] **Step 7: Reserve enough time for a cold provider redirect chain**

Add `TestSiteFaviconTimeoutBudgetReservesVerifiedHostedRecovery`, requiring a
ten-second `siteIconOverallTimeout` and a six-second timeout on
`hostedSiteFaviconHTTPClient()`. Keep `siteIconRequestTimeout` at four seconds.
Set the cloned provider client's `Timeout` explicitly to
`siteIconHostedTimeout` so it does not inherit the shorter origin budget.

- [ ] **Step 8: Run all focused favicon tests and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -count=1
```

Expected: PASS, including existing cooldown, limits, origin discovery, proxy,
and cache tests.

### Task 4: Invalidate the Previous Cache and Browser Repair Marker

**Files:**
- Modify: `internal/fn/favicon_test.go`
- Modify: `internal/fn/favicon.go`
- Modify: `internal/pages/home/home_test.go`
- Modify: `internal/pages/home/home.go`

**Interfaces:**
- Produces: internal generation `2026-07-verified-hosted` and browser repair prefix `superflare.site-icon.verified-hosted:`.
- Consumes: existing generation-aware SHA-256 cache key and grouped reload script.

- [ ] **Step 1: Add previous-generation and marker RED assertions**

Write a valid image under the literal
`2026-07-origin-only\x00<iconURL>` hash and assert the current reader ignores
it. Update the inline-script test to require:

```go
`var repairKeyPrefix="superflare.site-icon.verified-hosted:"`
```

Continue asserting one grouped `fetch(src,{cache:"reload"})` and no public
version query.

- [ ] **Step 2: Run generation tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn -run TestReadCachedSiteFaviconIgnoresOriginOnlyGeneration -count=1
.\.tools\go\bin\go.exe test ./internal/pages/home -run TestInlineSiteIconRefreshScriptRepairsLegacyDirectBrowserCacheOncePerSource -count=1
```

Expected: FAIL because both generation strings still use `origin-only`.

- [ ] **Step 3: Update internal and browser generations**

Change only the two private generation strings. Do not add a query parameter or
modify the stable `/assets/site-icons?src=...` URL.

- [ ] **Step 4: Run generation and homepage packages and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/fn ./internal/pages/home -count=1
```

Expected: PASS.

### Task 5: Full Verification and Local Commit

**Files:**
- Verify: all modified code and documentation.

**Interfaces:**
- Produces: a locally committed fix on `main`, with no remote push.

- [ ] **Step 1: Format and run focused cross-package tests**

Run:

```powershell
.\.tools\go\bin\gofmt.exe -w internal\fn\favicon.go internal\fn\favicon_test.go internal\pages\home\home.go internal\pages\home\home_test.go
.\.tools\go\bin\go.exe test ./internal/fn ./internal/pages/home ./internal/resources/assets -count=1
```

Expected: PASS.

- [ ] **Step 2: Run repository verification**

Run:

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
.\.tools\go\bin\go.exe vet ./internal/fn ./internal/pages/home ./internal/resources/assets
.\.tools\go\bin\go.exe build ./...
git diff --check
```

Expected: all commands exit zero. Full `go vet ./...` may continue to report the
pre-existing non-test-goroutine findings in `internal/server/server_test.go`;
the touched-package vet command must pass.

- [ ] **Step 3: Request an independent read-only review**

Review retrieval ordering, provider source validation, redirect restrictions,
privacy filtering, cache-generation behavior, failure cooldown interaction,
and test coverage. Resolve every Critical or Important finding.

- [ ] **Step 4: Build and run a fresh local server for live verification**

Through `/assets/site-icons`, assert:

```text
wallroom.io     -> cached, image/x-icon, 1150 bytes
wallhaven.cc    -> cached, image/x-icon, 1354 bytes
nonexistent     -> fallback, image/svg+xml, no-store
```

Confirm the homepage contains `cache:"reload"`, the verified-hosted repair
prefix, and no site-icon `v` parameter.

- [ ] **Step 5: Commit only scoped files locally**

Run:

```powershell
git add -- docs/superpowers/specs/2026-07-18-verified-hosted-favicon-recovery-design.md docs/superpowers/plans/2026-07-18-verified-hosted-favicon-recovery.md internal/fn/favicon.go internal/fn/favicon_test.go internal/pages/home/home.go internal/pages/home/home_test.go
git commit -m "fix: restore verified hosted favicons"
```

Expected: local commit succeeds; `fnapp/superflare/manifest` remains modified,
`tools/superflare-icon.zip` remains untracked, and no remote push occurs.
