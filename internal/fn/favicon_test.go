package fn

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
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

	"golang.org/x/image/bmp"
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
	if !strings.Contains(out, `src="/assets/site-icons?src=http%3A%2F%2Fexample.com%3A8080%2Ffavicon.ico&amp;v=2"`) {
		t.Fatalf("GetSiteFavicon should proxy public site favicon through local route, got %q", out)
	}
	if !strings.Contains(out, `referrerpolicy="no-referrer"`) {
		t.Fatalf("GetSiteFavicon should avoid referrer leakage, got %q", out)
	}
}

func TestGetSiteFavicon_LocalURLUsesProxyFallbackRoute(t *testing.T) {
	out := GetSiteFavicon("http://192.168.1.20:8080/a/b", "fallback")
	if !strings.Contains(out, `src="/assets/site-icons?src=http%3A%2F%2F192.168.1.20%3A8080%2Ffavicon.ico&amp;v=2"`) {
		t.Fatalf("GetSiteFavicon should use proxy fallback route for local-network favicon, got %q", out)
	}
}

func TestGetSiteFaviconAssetURL_PublicUsesProxy(t *testing.T) {
	out := GetSiteFaviconAssetURL("https://github.com/junfuchang/superflare")
	const expected = `/assets/site-icons?src=https%3A%2F%2Fgithub.com%2Ffavicon.ico&v=2`
	if out != expected {
		t.Fatalf("GetSiteFaviconAssetURL public: expected %q, got %q", expected, out)
	}
}

func TestGetSiteFaviconAssetURLDoesNotStartDuplicateBackgroundFetch(t *testing.T) {
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
	requested := make(chan struct{}, 1)
	siteIconHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			requested <- struct{}{}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"image/svg+xml"}},
				Body:       io.NopCloser(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)),
				Request:    req,
			}, nil
		}),
	}

	_ = GetSiteFaviconAssetURL("https://example.com/path")
	select {
	case <-requested:
		t.Fatal("asset URL generation should not duplicate the browser favicon request")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestGetSiteFaviconAssetURLFast_PublicCacheMissReturnsEmpty(t *testing.T) {
	out := GetSiteFaviconAssetURLFast("https://github.com/junfuchang/superflare")
	if out != "" {
		t.Fatalf("GetSiteFaviconAssetURLFast public cache miss should be empty, got %q", out)
	}
}

func TestGetSiteFaviconAssetURLFastDoesNotWaitForBusyDecodeSlots(t *testing.T) {
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	bookmarkLink := "https://example.com/path"
	writeValidSiteFaviconCacheForTest(t, bookmarkLink)
	releaseSlots := occupySiteIconDecodeSlots(t)
	defer releaseSlots()

	result := make(chan string, 1)
	go func() { result <- GetSiteFaviconAssetURLFast(bookmarkLink) }()
	select {
	case got := <-result:
		if got != "" {
			t.Fatalf("busy fast cache lookup should fall back to the async path, got %q", got)
		}
	case <-time.After(200 * time.Millisecond):
		releaseSlots()
		<-result
		t.Fatal("fast cache lookup blocked waiting for a decode slot")
	}
}

func TestGetSiteFaviconAssetURLFastReusesValidatedCacheMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	bookmarkLink := "https://example.com/path"
	writeValidSiteFaviconCacheForTest(t, bookmarkLink)
	want := siteIconProxyURL(GetSiteFaviconURL(bookmarkLink))
	if got := GetSiteFaviconAssetURLFast(bookmarkLink); got != want {
		t.Fatalf("initial fast cache lookup = %q, want %q", got, want)
	}

	releaseSlots := occupySiteIconDecodeSlots(t)
	defer releaseSlots()
	result := make(chan string, 1)
	go func() { result <- GetSiteFaviconAssetURLFast(bookmarkLink) }()
	select {
	case got := <-result:
		if got != want {
			t.Fatalf("validated cache metadata lookup = %q, want %q", got, want)
		}
	case <-time.After(200 * time.Millisecond):
		releaseSlots()
		<-result
		t.Fatal("validated cache metadata was not reused")
	}
}

func TestGetSiteFaviconAssetURLFastRevalidatesChangedCacheFile(t *testing.T) {
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	bookmarkLink := "https://example.com/path"
	iconURL := GetSiteFaviconURL(bookmarkLink)
	writeValidSiteFaviconCacheForTest(t, bookmarkLink)
	if got := GetSiteFaviconAssetURLFast(bookmarkLink); got == "" {
		t.Fatal("initial valid cache should be available to the fast lookup")
	}

	cachePath, err := siteFaviconCachePath(iconURL)
	if err != nil {
		t.Fatalf("siteFaviconCachePath: %v", err)
	}
	invalid := []byte(`<!doctype html><html><body><svg></svg></body></html>`)
	if err := os.WriteFile(cachePath, invalid, 0644); err != nil {
		t.Fatalf("replace cache file: %v", err)
	}
	if got := GetSiteFaviconAssetURLFast(bookmarkLink); got != "" {
		t.Fatalf("changed invalid cache should be revalidated, got %q", got)
	}
	if _, err := os.Stat(cachePath); !os.IsNotExist(err) {
		t.Fatalf("changed invalid cache should be removed, stat err=%v", err)
	}
}

func writeValidSiteFaviconCacheForTest(t *testing.T, bookmarkLink string) {
	t.Helper()
	iconURL := GetSiteFaviconURL(bookmarkLink)
	cachePath, err := siteFaviconCachePath(iconURL)
	if err != nil {
		t.Fatalf("siteFaviconCachePath: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		t.Fatalf("MkdirAll cache: %v", err)
	}
	if err := os.WriteFile(cachePath, encodeTestFavicon(t, "png"), 0644); err != nil {
		t.Fatalf("WriteFile cache: %v", err)
	}
}

func occupySiteIconDecodeSlots(t *testing.T) func() {
	t.Helper()
	for index := 0; index < siteIconDecodeLimit; index++ {
		siteIconDecodeLimiter <- struct{}{}
	}
	released := false
	return func() {
		if released {
			return
		}
		released = true
		for index := 0; index < siteIconDecodeLimit; index++ {
			<-siteIconDecodeLimiter
		}
	}
}

func TestGetSiteFaviconAssetURLFastDoesNotStartDuplicateBackgroundFetch(t *testing.T) {
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
	requested := make(chan struct{}, 1)
	siteIconHTTPClient = &http.Client{
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			requested <- struct{}{}
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"image/svg+xml"}},
				Body:       io.NopCloser(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)),
				Request:    req,
			}, nil
		}),
	}

	if out := GetSiteFaviconAssetURLFast("https://example.com/path"); out != "" {
		t.Fatalf("cache miss should stay empty, got %q", out)
	}
	select {
	case <-requested:
		t.Fatal("fast asset URL lookup should not duplicate the browser favicon request")
	case <-time.After(100 * time.Millisecond):
	}
}

func TestGetSiteFaviconAssetURL_LocalUsesProxyFallbackRoute(t *testing.T) {
	out := GetSiteFaviconAssetURL("https://nas.local/apps")
	const expected = `/assets/site-icons?src=https%3A%2F%2Fnas.local%2Ffavicon.ico&v=2`
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
	pngData := encodeTestFavicon(t, "png")
	jpegData := encodeTestFavicon(t, "jpeg")
	gifData := encodeTestFavicon(t, "gif")
	bmpData := encodeTestFavicon(t, "bmp")
	webpData, err := base64.StdEncoding.DecodeString("UklGRrIBAABXRUJQVlA4TKUBAAAvSsAYAA8w//M///MfeJAkbXvaSG7m8Q3GfYSBJekwQztm/IcZlgwnmWImn2BK7aFmBtnVir6q//8VOkFE/xm4baTIu8c48ArEo6+B3zFKYln3pqClSCKX0begFTAXFOLXHSyF8cCNcZEG4OywuA4KVVfJCiArU7GAgJI8+lJP/OKMT/fBAjevg1cYB7YVkFuWga2lyPi5I0HFy5YTpWIHg0RZpkniRVW9odHAKOwosWuOGdxIyn2OvaCDvhg/we6TwadPBPbqBV58MsLmMJ8yZnOWk8SRz4N+QoyPL+MnamzMvcE1rHNEr91F9GKZPVUcS9w7PhhH36suB9qPeYb/oLk6cuTiJ0wOK3m5h1cKjW6EVZCYMK7dxcKCBdgP9HkKr9gkAO2P8GKZGWVdIAatQa+1IDpt6qyorVwdy01xdW8Jkfk6xjEXmVQQ+HQdFr6OKhIN34dXWq0+0qr6EJSCeeVLH9+gvGTLyqM65PQ44ihzlTXxQKjKbAvshXgir7Lil9w4L2bvMycmjQcqXaMCO6BlY28i+FOLzbfI1vEqxAhotocAAA==")
	if err != nil {
		t.Fatalf("decode webp fixture: %v", err)
	}
	icoData := encodeTestICO(pngData)
	icoDIBData := encodeTestICODIB()

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
			name:       "svg with document preamble",
			data:       []byte("\xef\xbb\xbf<?xml version=\"1.0\"?><!-- icon --><!DOCTYPE svg><svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"),
			headerType: "application/xml",
			wantType:   "image/svg+xml",
		},
		{
			name:       "png",
			data:       pngData,
			headerType: "application/octet-stream",
			wantType:   "image/png",
		},
		{
			name:       "jpeg",
			data:       jpegData,
			headerType: "",
			wantType:   "image/jpeg",
		},
		{
			name:       "gif",
			data:       gifData,
			headerType: "",
			wantType:   "image/gif",
		},
		{
			name:       "webp",
			data:       webpData,
			headerType: "application/octet-stream",
			wantType:   "image/webp",
		},
		{
			name:       "bmp",
			data:       bmpData,
			headerType: "image/bmp",
			wantType:   "image/bmp",
		},
		{
			name:       "ico",
			data:       icoData,
			headerType: "application/octet-stream",
			wantType:   "image/x-icon",
		},
		{
			name:       "ico dib",
			data:       icoDIBData,
			headerType: "image/x-icon",
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

func TestDetectSiteFaviconContentTypeRejectsInvalidImagePayloads(t *testing.T) {
	prefixedPNG := append([]byte(" \n"), encodeTestFavicon(t, "png")...)
	completePNG := encodeTestFavicon(t, "png")
	truncatedPNGBody := completePNG[:len(completePNG)-12]
	invalidICOBody := encodeTestICO([]byte("not an icon image"))
	invalidICODIB := encodeTestICO(append(makeTestICODIBHeader(2, 2), 0))
	avifFtypOnly := encodeTestAVIFFtypOnly()
	tests := []struct {
		name       string
		data       []byte
		headerType string
	}{
		{name: "mislabeled text", data: []byte("not an image"), headerType: "image/png"},
		{name: "truncated png", data: []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0x00}, headerType: "image/png"},
		{name: "truncated png body", data: truncatedPNGBody, headerType: "image/png"},
		{name: "truncated ico", data: []byte{0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x10, 0x10, 0x00, 0x00, 0x01, 0x00}, headerType: "image/x-icon"},
		{name: "invalid ico body", data: invalidICOBody, headerType: "image/x-icon"},
		{name: "ico dib without pixels", data: invalidICODIB, headerType: "image/x-icon"},
		{name: "truncated avif", data: []byte{0x00, 0x00, 0x00, 0x18, 'f', 't', 'y', 'p', 'a', 'v', 'i', 'f'}, headerType: "image/avif"},
		{name: "avif without image boxes", data: avifFtypOnly, headerType: "image/avif"},
		{name: "unvalidated avif payload", data: encodeTestAVIF(), headerType: "image/avif"},
		{name: "truncated svg", data: []byte(`<svg xmlns="http://www.w3.org/2000/svg"><path>`), headerType: "image/svg+xml"},
		{name: "uppercase svg root", data: []byte(`<SVG xmlns="http://www.w3.org/2000/svg"></SVG>`), headerType: "image/svg+xml"},
		{name: "prefixed png", data: prefixedPNG, headerType: "image/png"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, ok := detectSiteFaviconContentType(tt.data, tt.headerType); ok || got != "" {
				t.Fatalf("invalid image payload should be rejected, got ok=%v type=%q", ok, got)
			}
		})
	}
}

func TestDetectSiteFaviconContentTypeRejectsOversizedDecodedImage(t *testing.T) {
	data := encodeTestFaviconSize(t, "png", 2049, 2049)
	if got, ok := detectSiteFaviconContentType(data, "image/png"); ok || got != "" {
		t.Fatalf("oversized decoded image should be rejected, got ok=%v type=%q", ok, got)
	}
}

func encodeTestFavicon(t *testing.T, format string) []byte {
	return encodeTestFaviconSize(t, format, 2, 2)
}

func encodeTestFaviconSize(t *testing.T, format string, width int, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	var out bytes.Buffer
	var err error
	switch format {
	case "png":
		err = png.Encode(&out, img)
	case "jpeg":
		err = jpeg.Encode(&out, img, nil)
	case "gif":
		err = gif.Encode(&out, img, nil)
	case "bmp":
		err = bmp.Encode(&out, img)
	default:
		t.Fatalf("unsupported test favicon format: %s", format)
	}
	if err != nil {
		t.Fatalf("encode %s favicon: %v", format, err)
	}
	return out.Bytes()
}

func encodeTestICO(pngData []byte) []byte {
	const directorySize = 6 + 16
	out := make([]byte, directorySize, directorySize+len(pngData))
	binary.LittleEndian.PutUint16(out[2:4], 1)
	binary.LittleEndian.PutUint16(out[4:6], 1)
	out[6] = 2
	out[7] = 2
	binary.LittleEndian.PutUint16(out[10:12], 1)
	binary.LittleEndian.PutUint16(out[12:14], 32)
	binary.LittleEndian.PutUint32(out[14:18], uint32(len(pngData)))
	binary.LittleEndian.PutUint32(out[18:22], directorySize)
	return append(out, pngData...)
}

func encodeTestICODIB() []byte {
	const (
		width        = 2
		height       = 2
		xorBytes     = 16
		andMaskBytes = 8
	)
	payload := append(makeTestICODIBHeader(width, height), make([]byte, xorBytes+andMaskBytes)...)
	return encodeTestICO(payload)
}

func makeTestICODIBHeader(width uint32, height uint32) []byte {
	const headerSize = 40
	header := make([]byte, headerSize)
	binary.LittleEndian.PutUint32(header[0:4], headerSize)
	binary.LittleEndian.PutUint32(header[4:8], width)
	binary.LittleEndian.PutUint32(header[8:12], height*2)
	binary.LittleEndian.PutUint16(header[12:14], 1)
	binary.LittleEndian.PutUint16(header[14:16], 32)
	return header
}

func encodeTestAVIF() []byte {
	out := encodeTestAVIFFtypOnly()
	meta := make([]byte, 12)
	binary.BigEndian.PutUint32(meta[0:4], uint32(len(meta)))
	copy(meta[4:8], "meta")
	mdat := make([]byte, 9)
	binary.BigEndian.PutUint32(mdat[0:4], uint32(len(mdat)))
	copy(mdat[4:8], "mdat")
	mdat[8] = 1
	out = append(out, meta...)
	out = append(out, mdat...)
	return out
}

func encodeTestAVIFFtypOnly() []byte {
	out := make([]byte, 24)
	binary.BigEndian.PutUint32(out[0:4], uint32(len(out)))
	copy(out[4:8], "ftyp")
	copy(out[8:12], "avif")
	copy(out[16:20], "mif1")
	copy(out[20:24], "avif")
	return out
}

func TestDetectSiteFaviconContentTypeRejectsHTML(t *testing.T) {
	got, ok := detectSiteFaviconContentType([]byte("<!doctype html><html><body>no icon</body></html>"), "image/png")
	if ok || got != "" {
		t.Fatalf("html payload should be rejected, got ok=%v type=%q", ok, got)
	}
}

func TestDetectSiteFaviconContentTypeRejectsHTMLContainingInlineSVG(t *testing.T) {
	data := []byte(`<!doctype html><html><body><svg viewBox="0 0 16 16"></svg></body></html>`)
	got, ok := detectSiteFaviconContentType(data, "text/html; charset=utf-8")
	if ok || got != "" {
		t.Fatalf("html payload containing inline svg should be rejected, got ok=%v type=%q", ok, got)
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
	invalidHTML := []byte(`<!doctype html><html><body><svg viewBox="0 0 16 16"></svg></body></html>`)
	if err := os.WriteFile(cachePath, invalidHTML, 0644); err != nil {
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

func TestFetchAndCacheSiteFaviconCacheHitHonorsCanceledContextBeforeRead(t *testing.T) {
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
	if err := writeCachedSiteFavicon(iconURL, encodeTestFavicon(t, "png")); err != nil {
		t.Fatalf("writeCachedSiteFavicon: %v", err)
	}
	for index := 0; index < siteIconDecodeLimit; index++ {
		siteIconDecodeLimiter <- struct{}{}
	}
	released := make(chan struct{})
	go func() {
		time.Sleep(100 * time.Millisecond)
		for index := 0; index < siteIconDecodeLimit; index++ {
			<-siteIconDecodeLimiter
		}
		close(released)
	}()
	defer func() { <-released }()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = fetchAndCacheSiteFavicon(ctx, iconURL)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled cache read should return context cancellation, got %v", err)
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

func TestFetchPublicSiteFaviconRejectsHTMLWithInlineSVGAndUsesDeclaredIcon(t *testing.T) {
	tmpDir := t.TempDir()
	oldWD, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(oldWD) }()

	const iconBody = `<svg xmlns="http://www.w3.org/2000/svg"><title>declared-icon</title></svg>`
	const pageBody = `<!doctype html><html><head><link rel="icon" href="/assets/icon.svg"></head><body><svg viewBox="0 0 16 16"></svg></body></html>`
	oldClient := siteIconHTTPClient
	defer func() { siteIconHTTPClient = oldClient }()
	siteIconHTTPClient = &http.Client{
		Timeout: 2 * time.Second,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			switch req.URL.Path {
			case "/favicon.ico", "/":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
					Body:       io.NopCloser(strings.NewReader(pageBody)),
					Request:    req,
				}, nil
			case "/assets/icon.svg":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"image/svg+xml"}},
					Body:       io.NopCloser(strings.NewReader(iconBody)),
					Request:    req,
				}, nil
			default:
				t.Fatalf("unexpected favicon request path: %s", req.URL.Path)
				return nil, nil
			}
		}),
	}

	iconURL := "https://example.com/favicon.ico"
	data, contentType, err := FetchPublicSiteFavicon(iconURL)
	if err != nil {
		t.Fatalf("FetchPublicSiteFavicon: %v", err)
	}
	if contentType != "image/svg+xml" || string(data) != iconBody {
		t.Fatalf("favicon type=%q body=%q, want declared icon", contentType, data)
	}
	if cached, cachedType, err := readCachedSiteFavicon(iconURL); err != nil || cachedType != "image/svg+xml" || string(cached) != iconBody {
		t.Fatalf("cached favicon type=%q err=%v body=%q, want declared icon", cachedType, err, cached)
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

	const htmlPrefix = `<!doctype html><html><head>`
	const iconLink = `<link rel="icon" href="/assets/favicon.svg">`
	largeHTML := htmlPrefix + strings.Repeat(" ", siteIconHTMLBytes-len(htmlPrefix)-len(iconLink)) + iconLink +
		`</head><body>` + strings.Repeat("x", 1024) + `</body></html>`
	htmlReader := &countingReader{reader: strings.NewReader(largeHTML)}
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
					Body:       io.NopCloser(htmlReader),
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
	if got := htmlReader.bytesRead; got > siteIconHTMLBytes {
		t.Fatalf("favicon HTML discovery read %d bytes, want <= %d", got, siteIconHTMLBytes)
	}
}

func TestCollectFaviconHrefsSupportsReportedDeclarations(t *testing.T) {
	tests := []struct {
		name string
		link string
		want string
	}{
		{
			name: "absolute icon",
			link: `<link rel="icon" href="https://picx.zhimg.com/80/v2-5393cb76a824b11d7771ecdce592c87d.png">`,
			want: "https://picx.zhimg.com/80/v2-5393cb76a824b11d7771ecdce592c87d.png",
		},
		{
			name: "absolute shortcut icon",
			link: `<link rel="shortcut icon" href="https://wallhaven.cc/favicon.ico">`,
			want: "https://wallhaven.cc/favicon.ico",
		},
		{
			name: "relative typed shortcut icon",
			link: `<link rel="shortcut icon" type="image/ico" href="/img/favicon.ico">`,
			want: "/img/favicon.ico",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hrefs, err := collectFaviconHrefs(strings.NewReader("<html><head>" + tt.link + "</head><body></body></html>"))
			if err != nil {
				t.Fatalf("collectFaviconHrefs: %v", err)
			}
			if len(hrefs) != 1 || hrefs[0] != tt.want {
				t.Fatalf("favicon hrefs = %#v, want %#v", hrefs, []string{tt.want})
			}
		})
	}
}

func TestFetchPublicSiteFaviconResolvesRelativeHTMLIconAgainstRedirectedPage(t *testing.T) {
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
			switch req.URL.String() {
			case "https://redirect.example/favicon.ico":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     http.Header{"Content-Type": []string{"text/plain"}},
					Body:       io.NopCloser(strings.NewReader("not found")),
					Request:    req,
				}, nil
			case "https://redirect.example/":
				return &http.Response{
					StatusCode: http.StatusFound,
					Header:     http.Header{"Location": []string{"https://www.redirect.example/app/"}},
					Body:       io.NopCloser(strings.NewReader("")),
					Request:    req,
				}, nil
			case "https://www.redirect.example/app/":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"text/html"}},
					Body:       io.NopCloser(strings.NewReader(`<html><head><link rel="shortcut icon" href="icons/favicon.svg"></head></html>`)),
					Request:    req,
				}, nil
			case "https://www.redirect.example/app/icons/favicon.svg":
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"image/svg+xml"}},
					Body:       io.NopCloser(strings.NewReader(`<svg xmlns="http://www.w3.org/2000/svg"></svg>`)),
					Request:    req,
				}, nil
			default:
				t.Fatalf("unexpected favicon request URL: %s", req.URL)
				return nil, nil
			}
		}),
	}

	data, contentType, err := FetchPublicSiteFavicon("https://redirect.example/favicon.ico")
	if err != nil {
		t.Fatalf("FetchPublicSiteFavicon: %v", err)
	}
	if contentType != "image/svg+xml" || !strings.Contains(string(data), "<svg") {
		t.Fatalf("redirected relative favicon type=%q data=%q", contentType, data)
	}
}

func TestHostedSiteFaviconURLUsesOnlyPublicDomainNames(t *testing.T) {
	if got := hostedSiteFaviconURL("https://wallroom.io/favicon.ico"); got != "https://icon.horse/icon/wallroom.io" {
		t.Fatalf("public fallback URL = %q", got)
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
			t.Fatalf("local fallback URL for %q = %q, want empty", input, got)
		}
	}
}

func TestFetchPublicSiteFaviconUsesHostedFallbackWhenOriginUnavailable(t *testing.T) {
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
	pngData := encodeTestFavicon(t, "png")
	siteIconHTTPClient = &http.Client{
		Timeout: 2 * time.Second,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.URL.Host == "blocked-favicon.test-domain.com" && req.URL.Path == "/favicon.ico":
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     http.Header{"Content-Type": []string{"text/plain"}},
					Body:       io.NopCloser(strings.NewReader("not found")),
					Request:    req,
				}, nil
			case req.URL.Host == "blocked-favicon.test-domain.com" && req.URL.Path == "/":
				return &http.Response{
					StatusCode: http.StatusServiceUnavailable,
					Header:     http.Header{"Content-Type": []string{"text/plain"}},
					Body:       io.NopCloser(strings.NewReader("unavailable")),
					Request:    req,
				}, nil
			case req.URL.Host == "icon.horse" && req.URL.Path == "/icon/blocked-favicon.test-domain.com":
				atomic.AddInt32(&providerRequests, 1)
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     http.Header{"Content-Type": []string{"image/png"}},
					Body:       io.NopCloser(bytes.NewReader(pngData)),
					Request:    req,
				}, nil
			default:
				t.Fatalf("unexpected favicon request URL: %s", req.URL)
				return nil, nil
			}
		}),
	}

	iconURL := "https://blocked-favicon.test-domain.com/favicon.ico"
	for attempt := 0; attempt < 2; attempt++ {
		data, contentType, err := FetchPublicSiteFavicon(iconURL)
		if err != nil {
			t.Fatalf("FetchPublicSiteFavicon attempt %d: %v", attempt+1, err)
		}
		if contentType != "image/png" || !bytes.Equal(data, pngData) {
			t.Fatalf("hosted fallback attempt %d type=%q data=%v", attempt+1, contentType, data)
		}
	}
	if got := atomic.LoadInt32(&providerRequests); got != 1 {
		t.Fatalf("hosted fallback requests = %d, want 1", got)
	}
}

func TestFetchPublicSiteFaviconBoundsWholeFallbackChain(t *testing.T) {
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
	requests := make([]string, 0, 2)
	siteIconHTTPClient = &http.Client{
		Timeout: 4 * time.Second,
		Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
			if req.URL.Host == "slow.test-domain.com" || req.URL.Host == siteIconFallbackHost {
				requests = append(requests, req.URL.Host+req.URL.Path)
			}
			<-req.Context().Done()
			return nil, req.Context().Err()
		}),
	}

	started := time.Now()
	_, _, err = FetchPublicSiteFavicon("https://slow.test-domain.com/favicon.ico")
	elapsed := time.Since(started)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected overall deadline error, got %v", err)
	}
	if elapsed > 9*time.Second {
		t.Fatalf("fallback chain took %v, want less than 9s", elapsed)
	}
	if got := strings.Join(requests, ","); got != "slow.test-domain.com/favicon.ico,icon.horse/icon/slow.test-domain.com" {
		t.Fatalf("timeout fallback request order = %q", got)
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

type countingReader struct {
	reader    io.Reader
	bytesRead int
}

func (r *countingReader) Read(p []byte) (int, error) {
	n, err := r.reader.Read(p)
	r.bytesRead += n
	return n, err
}

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
