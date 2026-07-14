package assets

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

func TestDefaultIconURLsContainVersion(t *testing.T) {
	if got := SiteIconURL(func(string) string { return "/mdi.svg" }, ""); !strings.HasPrefix(got, "/favicon.ico?v=") {
		t.Fatalf("SiteIconURL default = %q", got)
	}
	if got := AppleTouchIconURL(); !strings.HasPrefix(got, "/apple-touch-icon.png?v=") {
		t.Fatalf("AppleTouchIconURL = %q", got)
	}
	if got := AndroidChrome192URL(); !strings.HasPrefix(got, "/android-chrome-192x192.png?v=") {
		t.Fatalf("AndroidChrome192URL = %q", got)
	}
	if got := AndroidChrome512URL(); !strings.HasPrefix(got, "/android-chrome-512x512.png?v=") {
		t.Fatalf("AndroidChrome512URL = %q", got)
	}
}

func TestWebsiteIconRoutesServeEmbeddedAssets(t *testing.T) {
	setupAssetsConfigDir(t)
	define.Init()
	define.AppFlags.DebugMode = true

	e := echo.New()
	RegisterRouting(e)

	for _, path := range []string{
		"/favicon.ico",
		"/apple-touch-icon.png",
		"/android-chrome-192x192.png",
		"/android-chrome-512x512.png",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d", path, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("%s returned empty body", path)
		}
	}
}

func TestWebsiteIconRoutesKeepDebugCachePolicyAfterAppFlagsChange(t *testing.T) {
	setupAssetsConfigDir(t)
	define.Init()
	originalFlags := define.AppFlags
	defer func() { define.AppFlags = originalFlags }()

	define.AppFlags.DebugMode = true
	e := echo.New()
	RegisterRouting(e)

	define.AppFlags.DebugMode = false

	req := httptest.NewRequest(http.MethodGet, "/favicon.ico", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("favicon status = %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected debug cache policy to stay bound to route registration, got %q", got)
	}
}

func TestOptimizeResourceCacheTimeKeepsDebugModeAfterAppFlagsChange(t *testing.T) {
	setupAssetsConfigDir(t)
	define.Init()
	originalFlags := define.AppFlags
	defer func() { define.AppFlags = originalFlags }()

	define.AppFlags.DebugMode = true
	e := echo.New()
	e.Use(optimizeResourceCacheTime())
	e.GET("/assets/demo.css", func(c *echo.Context) error {
		return c.String(http.StatusOK, "body{}")
	})

	define.AppFlags.DebugMode = false

	req := httptest.NewRequest(http.MethodGet, "/assets/demo.css", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("asset status = %d", rec.Code)
	}
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("expected debug middleware cache policy to stay bound, got %q", got)
	}
}

func TestSiteIconProxyFallsBackToBuiltinBookmarkIcon(t *testing.T) {
	setupAssetsConfigDir(t)
	define.Init()
	define.AppFlags.DebugMode = true
	define.StoreThemeRuntimeSnapshot(define.ThemeRuntimeSnapshot{
		Name:    "blackboard",
		Primary: "rgba(255, 253, 234, 1)",
	})
	if err := mdi.Init(); err != nil {
		t.Fatalf("mdi.Init: %v", err)
	}

	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "/assets/site-icons?src=https://example.invalid/favicon.ico", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("site icon proxy fallback status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "image/svg+xml") {
		t.Fatalf("site icon proxy fallback content-type = %q", got)
	}
	if got := rec.Header().Get(siteIconStateHeader); got != "fallback" {
		t.Fatalf("site icon proxy fallback state header = %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<svg") {
		t.Fatalf("site icon proxy fallback should return builtin svg icon, got %q", body)
	}
	if strings.Contains(strings.ToLower(body), "superflare") {
		t.Fatalf("site icon proxy fallback should not return project favicon, got %q", body)
	}
}

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

func TestSiteIconProxyFallsBackToBuiltinBookmarkIconWithoutMDICache(t *testing.T) {
	setupAssetsConfigDir(t)
	define.Init()
	define.AppFlags.DebugMode = true
	define.StoreThemeRuntimeSnapshot(define.ThemeRuntimeSnapshot{
		Name:    "blackboard",
		Primary: "",
	})

	originalMemFs := mdi.MemFs
	mdi.MemFs = nil
	defer func() { mdi.MemFs = originalMemFs }()

	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "/assets/site-icons?src=https://example.invalid/favicon.ico", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("site icon proxy fallback without mdi cache status = %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "image/svg+xml") {
		t.Fatalf("site icon proxy fallback without mdi cache content-type = %q", got)
	}
	if got := rec.Header().Get(siteIconStateHeader); got != "fallback" {
		t.Fatalf("site icon proxy fallback without mdi cache state header = %q", got)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "<svg") {
		t.Fatalf("site icon proxy fallback without mdi cache should return builtin svg icon, got %q", body)
	}
	if strings.Contains(strings.ToLower(body), "superflare") {
		t.Fatalf("site icon proxy fallback without mdi cache should not return project favicon, got %q", body)
	}
}

func TestSiteIconProxyCacheHitServesCachedData(t *testing.T) {
	setupAssetsConfigDir(t)
	define.Init()
	define.AppFlags.DebugMode = true
	define.StoreThemeRuntimeSnapshot(define.ThemeRuntimeSnapshot{
		Name:    "blackboard",
		Primary: "rgba(255, 253, 234, 1)",
	})
	if err := mdi.Init(); err != nil {
		t.Fatalf("mdi.Init: %v", err)
	}

	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() {
		_ = os.Chdir(oldWD)
	}()

	cacheDir := filepath.Join(tmpDir, "var", "cache", "site-icons")
	if err := os.MkdirAll(cacheDir, 0755); err != nil {
		t.Fatalf("MkdirAll cache: %v", err)
	}
	iconURL := "https://example.com/favicon.ico"
	cacheFile := filepath.Join(cacheDir, fn.SiteFaviconCacheKeyForTest(iconURL)+".bin")
	cachedSVG := []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"><rect width="16" height="16"/></svg>`)
	if err := os.WriteFile(cacheFile, cachedSVG, 0644); err != nil {
		t.Fatalf("WriteFile cache: %v", err)
	}

	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "/assets/site-icons?src=https://example.com/favicon.ico", nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("site icon proxy cache-hit status = %d", rec.Code)
	}
	if rec.Body.String() != string(cachedSVG) {
		t.Fatalf("site icon proxy cache-hit body = %q", rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); !strings.Contains(got, "image/svg+xml") {
		t.Fatalf("site icon proxy cache-hit content-type = %q", got)
	}
	if got := rec.Header().Get(siteIconStateHeader); got != "cached" {
		t.Fatalf("site icon proxy cache-hit state header = %q", got)
	}
}

func setupAssetsConfigDir(t *testing.T) string {
	t.Helper()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("WriteFile config.yml: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(oldWD)
	})
	return tmpDir
}

func TestServeUserAssetReturnsServerErrorWhenGetwdFails(t *testing.T) {
	originalGetwd := fnGetwdForAssetsTest()
	defer restoreFnGetwdForAssetsTest(originalGetwd)

	setFnGetwdForAssetsTest(func() (string, error) {
		return "", os.ErrPermission
	})

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/user-assets/icon.png", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	err := serveUserAssetByName(c, "icon.png")
	httpErr, ok := err.(*echo.HTTPError)
	if !ok {
		t.Fatalf("expected HTTPError, got %T", err)
	}
	if httpErr.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", httpErr.Code)
	}
}
