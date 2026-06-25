package fn

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
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

func TestGetSiteFaviconAssetURLFast_PublicCacheMissReturnsEmpty(t *testing.T) {
	out := GetSiteFaviconAssetURLFast("https://github.com/junfuchang/superflare")
	if out != "" {
		t.Fatalf("GetSiteFaviconAssetURLFast public cache miss should be empty, got %q", out)
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

func TestDetectSiteFaviconContentTypeSupportsCommonImageFormats(t *testing.T) {
	tests := []struct {
		name       string
		data       []byte
		headerType string
		wantType   string
	}{
		{
			name:       "svg mislabeled as text",
			data:       []byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"></svg>`),
			headerType: "text/plain; charset=utf-8",
			wantType:   "image/svg+xml",
		},
		{
			name:       "png header",
			data:       []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00},
			headerType: "application/octet-stream",
			wantType:   "image/png",
		},
		{
			name:       "jpeg header",
			data:       []byte{0xff, 0xd8, 0xff, 0xe0, 0x00},
			headerType: "",
			wantType:   "image/jpeg",
		},
		{
			name:       "gif header",
			data:       []byte("GIF89a\x01\x00\x01\x00"),
			headerType: "",
			wantType:   "image/gif",
		},
		{
			name:       "webp header",
			data:       []byte("RIFF1234WEBPVP8 "),
			headerType: "application/octet-stream",
			wantType:   "image/webp",
		},
		{
			name:       "ico header",
			data:       []byte{0x00, 0x00, 0x01, 0x00, 0x02, 0x00, 0x10, 0x10, 0x00, 0x00, 0x01, 0x00},
			headerType: "application/octet-stream",
			wantType:   "image/x-icon",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := detectSiteFaviconContentType(tt.data, tt.headerType)
			if !ok {
				t.Fatalf("detectSiteFaviconContentType should accept %s", tt.name)
			}
			if got != tt.wantType {
				t.Fatalf("detectSiteFaviconContentType = %q, want %q", got, tt.wantType)
			}
		})
	}
}

func TestDetectSiteFaviconContentTypeRejectsHTML(t *testing.T) {
	got, ok := detectSiteFaviconContentType([]byte("<!doctype html><html><body>no icon</body></html>"), "image/png")
	if ok || got != "" {
		t.Fatalf("html payload should be rejected, got ok=%v type=%q", ok, got)
	}
}

func TestReadCachedSiteFaviconRecognizesSVG(t *testing.T) {
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
	cachePath := filepath.Join(tmpDir, "var", "cache", "site-icons", SiteFaviconCacheKeyForTest(iconURL)+".bin")
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatalf("MkdirAll cache: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`), 0644); err != nil {
		t.Fatalf("WriteFile cache: %v", err)
	}

	_, contentType, err := readCachedSiteFavicon(iconURL)
	if err != nil {
		t.Fatalf("readCachedSiteFavicon: %v", err)
	}
	if contentType != "image/svg+xml" {
		t.Fatalf("cached svg content type = %q", contentType)
	}
}

func TestFetchPublicSiteFaviconAcceptsSVGWithTextPlainHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16"></svg>`))
	}))
	defer server.Close()

	oldClient := siteIconHTTPClient
	defer func() { siteIconHTTPClient = oldClient }()

	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("Parse server url: %v", err)
	}
	baseTransport, ok := server.Client().Transport.(*http.Transport)
	if !ok {
		t.Fatal("server client transport is not *http.Transport")
	}
	transport := baseTransport.Clone()
	transport.Proxy = nil
	siteIconHTTPClient = &http.Client{
		Timeout: server.Client().Timeout,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			clone.URL.Scheme = serverURL.Scheme
			clone.URL.Host = serverURL.Host
			clone.Host = "example.com"
			return transport.RoundTrip(clone)
		}),
	}

	iconURL := "https://example.com/favicon.ico"
	data, contentType, err := FetchPublicSiteFavicon(iconURL)
	if err != nil {
		t.Fatalf("FetchPublicSiteFavicon: %v", err)
	}
	if !strings.Contains(string(data), "<svg") {
		t.Fatalf("FetchPublicSiteFavicon should keep svg body, got %q", string(data))
	}
	if contentType != "image/svg+xml" {
		t.Fatalf("FetchPublicSiteFavicon content type = %q", contentType)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
