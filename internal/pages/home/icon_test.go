package home

import (
	"strings"
	"testing"
)

func TestRenderBookmarkIcon_EmptyIconUsesSiteFavicon(t *testing.T) {
	out := renderBookmarkIcon("", "https://example.com/path?q=1", "DEFAULT")
	if !strings.Contains(out, `src="/assets/site-icons?src=https%3A%2F%2Fexample.com%2Ffavicon.ico"`) {
		t.Fatalf("empty bookmark icon should use site favicon, got %q", out)
	}
}

func TestRenderBookmarkIcon_ExplicitImageWins(t *testing.T) {
	const icon = "https://cdn.example.com/icon.png"
	out := renderBookmarkIcon(icon, "https://example.com/path", "DEFAULT")
	if !strings.Contains(out, `src="https://cdn.example.com/icon.png"`) {
		t.Fatalf("explicit image icon should be used, got %q", out)
	}
	if strings.Contains(out, "example.com/favicon.ico") {
		t.Fatalf("explicit image icon should not be replaced by site favicon, got %q", out)
	}
}

func TestRenderBookmarkIcon_UnsupportedURLFallsBackToFillingMode(t *testing.T) {
	out := renderBookmarkIcon("", "chrome-extension://abc/index.html", "FILLING")
	if !strings.Contains(out, "https://favicon.yandex.net/favicon/abc/") {
		t.Fatalf("unsupported site favicon URL should keep filling-mode fallback, got %q", out)
	}
}
