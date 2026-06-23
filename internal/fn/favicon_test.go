package fn

import (
	"strings"
	"testing"
)

func TestGetSiteFaviconURL_ValidURL(t *testing.T) {
	out := GetSiteFaviconURL("https://github.com/junfuchang/superflare/path?q=1")
	const expected = "https://github.com/favicon.ico"
	if out != expected {
		t.Fatalf("GetSiteFaviconURL: expected %q, got %q", expected, out)
	}
}

func TestGetSiteFaviconURL_InvalidOrUnsupportedURL(t *testing.T) {
	tests := []string{"", "://invalid", "chrome-extension://abc/index.html", "/relative/path"}
	for _, input := range tests {
		if out := GetSiteFaviconURL(input); out != "" {
			t.Fatalf("GetSiteFaviconURL(%q) should be empty, got %q", input, out)
		}
	}
}

func TestGetSiteFavicon_ValidURL(t *testing.T) {
	out := GetSiteFavicon("http://example.com:8080/a/b", "fallback")
	if !strings.Contains(out, `src="/assets/site-icons?src=http%3A%2F%2Fexample.com%3A8080%2Ffavicon.ico"`) {
		t.Fatalf("GetSiteFavicon should proxy public site favicon through local route, got %q", out)
	}
	if !strings.Contains(out, `referrerpolicy="no-referrer"`) {
		t.Fatalf("GetSiteFavicon should avoid referrer leakage, got %q", out)
	}
}

func TestGetSiteFavicon_LocalURLStaysDirect(t *testing.T) {
	out := GetSiteFavicon("http://192.168.1.20:8080/a/b", "fallback")
	if !strings.Contains(out, `src="http://192.168.1.20:8080/favicon.ico"`) {
		t.Fatalf("GetSiteFavicon should keep local-network favicon direct, got %q", out)
	}
	if strings.Contains(out, "/assets/site-icons?src=") {
		t.Fatalf("GetSiteFavicon local-network favicon should not use proxy route, got %q", out)
	}
}

func TestGetSiteFaviconAssetURL_PublicUsesProxy(t *testing.T) {
	out := GetSiteFaviconAssetURL("https://github.com/junfuchang/superflare")
	const expected = `/assets/site-icons?src=https%3A%2F%2Fgithub.com%2Ffavicon.ico`
	if out != expected {
		t.Fatalf("GetSiteFaviconAssetURL public: expected %q, got %q", expected, out)
	}
}

func TestGetSiteFaviconAssetURL_LocalUsesDirectURL(t *testing.T) {
	out := GetSiteFaviconAssetURL("https://nas.local/apps")
	const expected = "https://nas.local/favicon.ico"
	if out != expected {
		t.Fatalf("GetSiteFaviconAssetURL local: expected %q, got %q", expected, out)
	}
}

func TestGetSiteFavicon_InvalidURL(t *testing.T) {
	const fallback = "fallback"
	if out := GetSiteFavicon("://invalid", fallback); out != fallback {
		t.Fatalf("GetSiteFavicon invalid URL should return fallback: got %q", out)
	}
}

func TestGetYandexFavicon_ValidURL(t *testing.T) {
	const fallback = "https://fallback/favicon.ico"
	out := GetYandexFavicon("https://github.com/soulteary", fallback)
	expected := "https://favicon.yandex.net/favicon/github.com/"
	if !strings.Contains(out, expected) {
		t.Errorf("GetYandexFavicon: expected substring %q in %q", expected, out)
	}
	if !strings.HasPrefix(out, "<img src=") {
		t.Errorf("GetYandexFavicon: expected img tag, got %q", out)
	}
}

func TestGetYandexFavicon_InvalidURL(t *testing.T) {
	const fallback = "https://fallback/favicon.ico"
	out := GetYandexFavicon("://invalid", fallback)
	if out != fallback {
		t.Errorf("GetYandexFavicon invalid URL should return fallback: got %q", out)
	}
}
