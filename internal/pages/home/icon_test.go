package home

import (
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

func prepareIconTest(t *testing.T) {
	t.Helper()
	if err := mdi.Init(); err != nil {
		t.Fatalf("mdi.Init: %v", err)
	}
	define.ThemeCurrent = "blackboard"
	define.ThemePrimaryColor = "rgba(255, 253, 234, 1)"
}

func TestRenderBookmarkIcon_EmptyIconUsesSiteFavicon(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("", "https://example.com/path?q=1", "DEFAULT")
	if !strings.Contains(out, `src="/assets/site-icons?src=https%3A%2F%2Fexample.com%2Ffavicon.ico"`) {
		t.Fatalf("empty bookmark icon should use site favicon, got %q", out)
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
	out := renderBookmarkIcon("definitely-not-a-real-mdi-icon", "https://example.com/path", "DEFAULT")
	if !strings.Contains(out, `src="/assets/site-icons?src=https%3A%2F%2Fexample.com%2Ffavicon.ico"`) {
		t.Fatalf("invalid mdi icon should fall back to site favicon, got %q", out)
	}
}

func TestRenderBookmarkIcon_UnsupportedURLFallsBackToFillingMode(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("", "chrome-extension://abc/index.html", "FILLING")
	if !strings.Contains(out, "https://favicon.yandex.net/favicon/abc/") {
		t.Fatalf("unsupported site favicon URL should keep filling-mode fallback, got %q", out)
	}
}

func TestRenderBookmarkIcon_InvalidURLFallsBackToBuiltinIcon(t *testing.T) {
	prepareIconTest(t)
	out := renderBookmarkIcon("", "not-a-valid-url", "DEFAULT")
	if !strings.Contains(out, `/assets/mdi/`) {
		t.Fatalf("invalid bookmark url should fall back to builtin icon, got %q", out)
	}
	if !strings.Contains(out, `bookmark.svg`) {
		t.Fatalf("invalid bookmark url should use builtin bookmark icon, got %q", out)
	}
}
