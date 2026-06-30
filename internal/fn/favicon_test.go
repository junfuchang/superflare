package fn

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetSiteFaviconURL_ValidURL(t *testing.T) {
	out := GetSiteFaviconURL("https://github.com/junfuchang/superflare/path?q=1")
	const expected = "https://github.com/favicon.ico"
	if out != expected {
		t.Fatalf("GetSiteFaviconURL: expected %q, got %q", expected, out)
	}
}

func TestGetSiteFaviconURL_InvalidOrUnsupportedURL(t *testing.T) {
	tests := []string{"", "://invalid", "/relative/path"}
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

func TestGetSiteFavicon_LocalURLUsesProxyFallbackRoute(t *testing.T) {
	out := GetSiteFavicon("http://192.168.1.20:8080/a/b", "fallback")
	if !strings.Contains(out, `src="/assets/site-icons?src=http%3A%2F%2F192.168.1.20%3A8080%2Ffavicon.ico"`) {
		t.Fatalf("GetSiteFavicon should use proxy fallback route for local-network favicon, got %q", out)
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

func TestGetSiteFaviconAssetURL_LocalUsesProxyFallbackRoute(t *testing.T) {
	out := GetSiteFaviconAssetURL("https://nas.local/apps")
	const expected = `/assets/site-icons?src=https%3A%2F%2Fnas.local%2Ffavicon.ico`
	if out != expected {
		t.Fatalf("GetSiteFaviconAssetURL local: expected %q, got %q", expected, out)
	}
}

func TestGetSiteFaviconAssetURLFast_LocalCacheMissReturnsEmpty(t *testing.T) {
	out := GetSiteFaviconAssetURLFast("https://nas.local/apps")
	if out != "" {
		t.Fatalf("GetSiteFaviconAssetURLFast local cache miss should be empty, got %q", out)
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

func TestReadCachedSiteFaviconRemovesInvalidCacheFile(t *testing.T) {
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
	if err := os.WriteFile(cachePath, []byte("<html>broken</html>"), 0644); err != nil {
		t.Fatalf("WriteFile cache: %v", err)
	}

	_, _, err = readCachedSiteFavicon(iconURL)
	if err == nil {
		t.Fatal("readCachedSiteFavicon should reject invalid cached html")
	}
	if _, statErr := os.Stat(cachePath); !os.IsNotExist(statErr) {
		t.Fatalf("invalid cache file should be removed, statErr=%v", statErr)
	}
}

func TestWriteCachedSiteFaviconIsAtomicAndLeavesNoTempFiles(t *testing.T) {
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
	if err := writeCachedSiteFavicon(iconURL, []byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)); err != nil {
		t.Fatalf("writeCachedSiteFavicon: %v", err)
	}

	cachePath := filepath.Join(tmpDir, "var", "cache", "site-icons", SiteFaviconCacheKeyForTest(iconURL)+".bin")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		t.Fatalf("ReadFile cache: %v", err)
	}
	if !strings.Contains(string(data), "<svg") {
		t.Fatalf("cache file should contain svg data, got %q", string(data))
	}

	matches, err := filepath.Glob(filepath.Join(filepath.Dir(cachePath), ".*.tmp-*"))
	if err != nil {
		t.Fatalf("Glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("atomic cache write should not leave temp files, got %v", matches)
	}
}

func TestSiteFaviconCachePathReturnsErrorWhenGetwdFails(t *testing.T) {
	originalGetwd := osGetwd
	defer func() { osGetwd = originalGetwd }()

	osGetwd = func() (string, error) {
		return "", errors.New("forced getwd failure")
	}

	_, err := siteFaviconCachePath("https://example.com/favicon.ico")
	if err == nil {
		t.Fatal("expected siteFaviconCachePath to fail")
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

func TestFetchPublicSiteFaviconAllowsRedirectToPrivateTarget(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/favicon.ico", nil)

	err := validateSiteFaviconRedirect(req, []*http.Request{
		httptest.NewRequest(http.MethodGet, "https://example.com/favicon.ico", nil),
	})
	if err != nil {
		t.Fatalf("expected favicon redirect to private target to be allowed, got %v", err)
	}
}

func TestFetchPublicSiteFaviconAllowsPublicRedirectToImageAssetPath(t *testing.T) {
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

	siteIconHTTPClient = &http.Client{
		Timeout:       2 * time.Second,
		CheckRedirect: validateSiteFaviconRedirect,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/favicon.ico":
				return &http.Response{
					StatusCode: http.StatusFound,
					Header: http.Header{
						"Location": []string{"https://example.com/goofy/ies/douyin_web/public/favicon.ico"},
					},
					Body:    io.NopCloser(strings.NewReader("")),
					Request: req,
				}, nil
			case "/goofy/ies/douyin_web/public/favicon.ico":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"image/svg+xml"}},
					Body:       io.NopCloser(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)),
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
		t.Fatalf("FetchPublicSiteFavicon should allow public favicon redirect asset paths: %v", err)
	}
	if contentType != "image/svg+xml" {
		t.Fatalf("redirected favicon content type = %q", contentType)
	}
	if !strings.Contains(string(data), "<svg") {
		t.Fatalf("redirected favicon body = %q", string(data))
	}
}

func TestFetchPublicSiteFaviconAcceptsPublicImageAssetPath(t *testing.T) {
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

	siteIconHTTPClient = &http.Client{
		Timeout: 2 * time.Second,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Path != "/static/icons/favicon.svg" {
				t.Fatalf("unexpected favicon request path: %s", req.URL.Path)
			}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"image/svg+xml"}},
				Body:       io.NopCloser(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)),
				Request:    req,
			}, nil
		}),
	}

	data, contentType, err := FetchPublicSiteFavicon("https://example.com/static/icons/favicon.svg")
	if err != nil {
		t.Fatalf("FetchPublicSiteFavicon should accept public image asset paths: %v", err)
	}
	if contentType != "image/svg+xml" {
		t.Fatalf("public image asset content type = %q", contentType)
	}
	if !strings.Contains(string(data), "<svg") {
		t.Fatalf("public image asset body = %q", string(data))
	}
}

func TestFetchPublicSiteFaviconAcceptsLocalNetworkSource(t *testing.T) {
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/favicon.ico" {
			t.Fatalf("unexpected local favicon path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`))
	}))
	defer server.Close()

	oldClient := siteIconHTTPClient
	siteIconHTTPClient = server.Client()
	defer func() { siteIconHTTPClient = oldClient }()

	data, contentType, err := FetchPublicSiteFavicon(server.URL + "/favicon.ico")
	if err != nil {
		t.Fatalf("FetchPublicSiteFavicon should accept local-network favicon sources: %v", err)
	}
	if contentType != "image/svg+xml" {
		t.Fatalf("local favicon content type = %q", contentType)
	}
	if !strings.Contains(string(data), "<svg") {
		t.Fatalf("local favicon body = %q", string(data))
	}
}

func TestFetchPublicSiteFaviconDoesNotRejectNonHTTPSchemeBeforeFetch(t *testing.T) {
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	_, _, err = FetchPublicSiteFavicon("ftp://example.com/favicon.ico")
	if err == nil {
		t.Fatal("unsupported protocols should still fail when the client cannot fetch them")
	}
	if strings.Contains(err.Error(), "unsupported site favicon url") || strings.Contains(err.Error(), "unsupported scheme") {
		t.Fatalf("non-http favicon schemes should not be rejected by preflight rules, got %v", err)
	}
}

func TestFetchPublicSiteFaviconDiscoversHTMLDeclaredIconWhenRootIcoFails(t *testing.T) {
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
					Body: io.NopCloser(strings.NewReader(`<!doctype html>
						<html><head>
							<link rel="icon" href="/assets/favicon.svg">
						</head><body></body></html>`)),
					Request: req,
				}, nil
			case "/assets/favicon.svg":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"image/svg+xml"}},
					Body:       io.NopCloser(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)),
					Request:    req,
				}, nil
			default:
				t.Fatalf("unexpected favicon discovery request path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	iconURL := "https://example.com/favicon.ico"
	data, contentType, err := FetchPublicSiteFavicon(iconURL)
	if err != nil {
		t.Fatalf("FetchPublicSiteFavicon should discover html-declared favicon: %v", err)
	}
	if contentType != "image/svg+xml" {
		t.Fatalf("discovered favicon content type = %q", contentType)
	}
	if !strings.Contains(string(data), "<svg") {
		t.Fatalf("discovered favicon body = %q", string(data))
	}
	if cached, cachedType, err := readCachedSiteFavicon(iconURL); err != nil || cachedType != "image/svg+xml" || !strings.Contains(string(cached), "<svg") {
		t.Fatalf("discovered favicon should be cached under root favicon key, type=%q err=%v body=%q", cachedType, err, string(cached))
	}
}

func TestWarmSiteFaviconURLLimitsGlobalConcurrentFetches(t *testing.T) {
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

	release := make(chan struct{})
	var active int32
	var maxActive int32
	siteIconHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			current := atomic.AddInt32(&active, 1)
			for {
				seen := atomic.LoadInt32(&maxActive)
				if current <= seen || atomic.CompareAndSwapInt32(&maxActive, seen, current) {
					break
				}
			}
			<-release
			atomic.AddInt32(&active, -1)
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"image/svg+xml"}},
				Body:       io.NopCloser(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)),
				Request:    req,
			}, nil
		}),
	}

	const maxAllowedConcurrentWarmups = siteIconWarmLimit
	for i := 0; i < maxAllowedConcurrentWarmups*3; i++ {
		WarmSiteFaviconURL(fmt.Sprintf("http://93.184.216.%d/favicon.ico", 34+i))
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if atomic.LoadInt32(&maxActive) > maxAllowedConcurrentWarmups {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(release)

	if got := atomic.LoadInt32(&maxActive); got > maxAllowedConcurrentWarmups {
		t.Fatalf("expected favicon warmup concurrency <= %d, got %d", maxAllowedConcurrentWarmups, got)
	}

	waitDeadline := time.Now().Add(time.Second)
	for atomic.LoadInt32(&active) != 0 && time.Now().Before(waitDeadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if got := atomic.LoadInt32(&active); got != 0 {
		t.Fatalf("expected favicon warmup requests to finish after release, active=%d", got)
	}
}

func TestSafeSiteFaviconTransportUsesEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")

	transport, ok := safeSiteFaviconTransport().(*http.Transport)
	if !ok {
		t.Fatalf("safeSiteFaviconTransport should return *http.Transport, got %T", safeSiteFaviconTransport())
	}
	if transport.Proxy == nil {
		t.Fatal("safe site favicon transport should use environment proxy settings")
	}
}

func TestFetchPublicSiteFaviconCanUseLocalEnvironmentProxy(t *testing.T) {
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL == nil || r.URL.Host != "93.184.216.55" || r.URL.Path != "/favicon.ico" {
			t.Fatalf("unexpected proxied favicon request URL: %v", r.URL)
		}
		w.Header().Set("Content-Type", "image/svg+xml")
		_, _ = w.Write([]byte(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`))
	}))
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")

	oldClient := siteIconHTTPClient
	siteIconHTTPClient = &http.Client{
		Timeout:       2 * time.Second,
		Transport:     safeSiteFaviconTransport(),
		CheckRedirect: validateSiteFaviconRedirect,
	}
	defer func() { siteIconHTTPClient = oldClient }()

	data, contentType, err := FetchPublicSiteFavicon("http://93.184.216.55/favicon.ico")
	if err != nil {
		t.Fatalf("FetchPublicSiteFavicon with local environment proxy: %v", err)
	}
	if contentType != "image/svg+xml" {
		t.Fatalf("proxied favicon content type = %q", contentType)
	}
	if !strings.Contains(string(data), "<svg") {
		t.Fatalf("proxied favicon body = %q", string(data))
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
