package home

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

func prepareIconTest(t *testing.T) {
	t.Helper()
	if err := mdi.Init(); err != nil {
		t.Fatalf("mdi.Init: %v", err)
	}
	define.StoreThemeRuntimeSnapshot(define.ThemeRuntimeSnapshot{
		Name:    "blackboard",
		Primary: "rgba(255, 253, 234, 1)",
	})
}

func TestRenderBookmarkIcon_EmptyIconUsesSiteFaviconInFillingMode(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("", "https://example.com/path?q=1", "FILLING")
	if !strings.Contains(out, `/assets/mdi/`) || !strings.Contains(out, `bookmark.svg`) {
		t.Fatalf("empty bookmark icon should return builtin bookmark icon before cache is ready, got %q", out)
	}
}

func TestRenderBookmarkIcon_CacheMissMarksFallbackForAsyncSiteFavicon(t *testing.T) {
	prepareIconTest(t)
	origFast := getSiteFaviconFast
	origAssetURL := getSiteFaviconAssetURL
	getSiteFaviconFast = func(_ string, fallback string) string {
		return fallback
	}
	getSiteFaviconAssetURL = func(link string) string {
		if link != "https://example.com/path" {
			t.Fatalf("unexpected favicon link: %s", link)
		}
		return "/assets/site-icons?src=https%3A%2F%2Fexample.com%2Ffavicon.ico"
	}
	defer func() {
		getSiteFaviconFast = origFast
		getSiteFaviconAssetURL = origAssetURL
	}()

	out := renderBookmarkIcon("", "https://example.com/path", "FILLING")
	if !strings.Contains(out, `bookmark.svg`) {
		t.Fatalf("cache miss should still render builtin bookmark fallback, got %q", out)
	}
	if !strings.Contains(out, `data-site-icon-src="/assets/site-icons?src=https%3A%2F%2Fexample.com%2Ffavicon.ico"`) {
		t.Fatalf("cache miss fallback should be marked for async favicon refresh, got %q", out)
	}
}

func TestRenderBookmarkIcon_MinimumRequestCacheMissMarksInlineFallbackForAsyncSiteFavicon(t *testing.T) {
	prepareIconTest(t)
	mdi.SetRuntimeFlags(model.Flags{EnableMinimumRequest: true})
	t.Cleanup(func() { mdi.SetRuntimeFlags(model.Flags{}) })

	origFast := getSiteFaviconFast
	origAssetURL := getSiteFaviconAssetURL
	getSiteFaviconFast = func(_ string, fallback string) string {
		return fallback
	}
	getSiteFaviconAssetURL = func(_ string) string {
		return "/assets/site-icons?src=https%3A%2F%2Fexample.com%2Ffavicon.ico"
	}
	defer func() {
		getSiteFaviconFast = origFast
		getSiteFaviconAssetURL = origAssetURL
	}()

	out := renderBookmarkIcon("", "https://example.com/path", "FILLING")
	if !strings.HasPrefix(out, "<svg ") {
		t.Fatalf("minimum request cache miss should keep inline svg fallback, got %q", out)
	}
	if !strings.Contains(out, `data-site-icon-src="/assets/site-icons?src=https%3A%2F%2Fexample.com%2Ffavicon.ico"`) {
		t.Fatalf("minimum request inline fallback should be marked for async favicon refresh, got %q", out)
	}
}

func TestRenderBookmarkIcon_CacheHitRendersDirectlyWithoutAsyncFallback(t *testing.T) {
	prepareIconTest(t)
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	iconURL := "https://example.com/favicon.ico"
	cachePath := filepath.Join(tmpDir, "var", "cache", "site-icons", fn.SiteFaviconCacheKeyForTest(iconURL)+".bin")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatalf("MkdirAll cache: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0644); err != nil {
		t.Fatalf("WriteFile cache: %v", err)
	}

	out := renderBookmarkIcon("", "https://example.com/path", "FILLING")
	if !strings.Contains(out, `src="/assets/site-icons?src=https%3A%2F%2Fexample.com%2Ffavicon.ico"`) {
		t.Fatalf("cached favicon should render directly from the validated cache, got %q", out)
	}
	if strings.Contains(out, `data-site-icon-src=`) {
		t.Fatalf("cached favicon should not use the asynchronous fallback path, got %q", out)
	}
	if !strings.Contains(out, `decoding="sync"`) || strings.Contains(out, `decoding="async"`) {
		t.Fatalf("cached favicon should decode synchronously before the first paint, got %q", out)
	}
	for _, expected := range []string{
		`data-site-icon-direct="1"`,
		`data-site-icon-fallback-src="/assets/mdi/blackboard-bookmark.svg"`,
	} {
		if !strings.Contains(out, expected) {
			t.Fatalf("cached favicon should retain browser error fallback %q, got %q", expected, out)
		}
	}
}

func TestRenderBookmarkIcon_InvalidCacheKeepsAsyncFallback(t *testing.T) {
	prepareIconTest(t)
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	iconURL := "https://example.com/favicon.ico"
	cachePath := filepath.Join(tmpDir, "var", "cache", "site-icons", fn.SiteFaviconCacheKeyForTest(iconURL)+".bin")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatalf("MkdirAll cache: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte(`<!doctype html><html><body><svg></svg></body></html>`), 0644); err != nil {
		t.Fatalf("WriteFile cache: %v", err)
	}

	out := renderBookmarkIcon("", "https://example.com/path", "FILLING")
	if !strings.Contains(out, `data-site-icon-src="/assets/site-icons?src=https%3A%2F%2Fexample.com%2Ffavicon.ico"`) {
		t.Fatalf("invalid cache should keep the asynchronous favicon path, got %q", out)
	}
	if !strings.Contains(out, `src="/assets/mdi/blackboard-bookmark.svg"`) {
		t.Fatalf("invalid cache should keep the builtin fallback visible, got %q", out)
	}
}

func TestRenderBookmarkIcon_ExplicitImageWins(t *testing.T) {
	prepareIconTest(t)
	const icon = "https://cdn.example.com/icon.png"
	out := renderBookmarkIcon(icon, "https://example.com/path", "DEFAULT")
	if !strings.Contains(out, `src="https://cdn.example.com/icon.png"`) {
		t.Fatalf("explicit image icon should be used, got %q", out)
	}
	if strings.Contains(out, "example.com/favicon.ico") {
		t.Fatalf("explicit image icon should not be replaced by site favicon, got %q", out)
	}
}

func TestRenderBookmarkIcon_InvalidMDIIconFallsBackToSiteFavicon(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("definitely-not-a-real-mdi-icon", "https://example.com/path", "FILLING")
	if !strings.Contains(out, `/assets/mdi/`) || !strings.Contains(out, `bookmark.svg`) {
		t.Fatalf("invalid mdi icon should fall back to builtin bookmark icon before cache is ready, got %q", out)
	}
}

func TestRenderBookmarkIcon_InvalidURLFallsBackToBuiltinIconInFillingMode(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("", "not-a-valid-url", "FILLING")
	if !strings.Contains(out, `/assets/mdi/`) {
		t.Fatalf("filling mode should fall back to builtin icon when favicon fetch fails, got %q", out)
	}
	if !strings.Contains(out, `bookmark.svg`) {
		t.Fatalf("filling mode should use builtin bookmark icon when favicon fetch fails, got %q", out)
	}
}

func TestRenderBookmarkIcon_LocalURLFallsBackToBuiltinIconInFillingModeOnCacheMiss(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("", "http://192.168.0.250:65530", "FILLING")
	if !strings.Contains(out, `/assets/mdi/`) {
		t.Fatalf("local network favicon cache miss should still fall back to builtin icon, got %q", out)
	}
	if !strings.Contains(out, `bookmark.svg`) {
		t.Fatalf("local network favicon cache miss should use builtin bookmark icon, got %q", out)
	}
}

func TestRenderBookmarkIcon_InvalidExplicitIconFallsBackToBuiltinIconInFillingMode(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("definitely-not-a-real-mdi-icon", "not-a-valid-url", "FILLING")
	if !strings.Contains(out, `/assets/mdi/`) {
		t.Fatalf("invalid explicit icon should fall back to builtin icon in filling mode, got %q", out)
	}
	if !strings.Contains(out, `bookmark.svg`) {
		t.Fatalf("invalid explicit icon should use builtin bookmark icon in filling mode, got %q", out)
	}
}

func TestRenderBookmarkIcon_DefaultModeKeepsEmptyIconBlank(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("", "https://example.com/path?q=1", "DEFAULT")
	if out != "" {
		t.Fatalf("default mode should keep empty icon blank, got %q", out)
	}
}

func TestRenderBookmarkIcon_DefaultModeFallsBackForInvalidIcon(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("definitely-not-a-real-mdi-icon", "https://example.com/path", "DEFAULT")
	if !strings.Contains(out, `/assets/mdi/`) || !strings.Contains(out, `bookmark.svg`) {
		t.Fatalf("default mode should fall back to builtin bookmark icon for invalid icon names, got %q", out)
	}
}

func TestRenderBookmarkIcon_NoneModeHidesExplicitImageIcon(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("https://cdn.example.com/icon.png", "https://example.com/path", "NONE")
	if out != "" {
		t.Fatalf("none mode should hide all icons, got %q", out)
	}
}

func TestRenderBookmarkIcon_NoneModeHidesBuiltInIcon(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("git", "https://example.com/path", "NONE")
	if out != "" {
		t.Fatalf("none mode should hide built-in icons, got %q", out)
	}
}
