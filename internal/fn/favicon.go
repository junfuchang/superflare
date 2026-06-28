package fn

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	siteIconProxyPath = "/assets/site-icons"
	siteIconCacheDir  = "var/cache/site-icons"
	siteIconMaxBytes  = 256 * 1024
)

var (
	siteIconHTTPClient = &http.Client{Timeout: 4 * time.Second}
	siteIconInflight   sync.Map
)

func GetSiteFaviconURL(bookmarkLink string) string {
	bookmarkLink = strings.TrimSpace(bookmarkLink)
	if bookmarkLink == "" {
		return ""
	}
	u, err := url.Parse(bookmarkLink)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/favicon.ico"}).String()
}

func isProxyableSiteFaviconURL(iconURL string) bool {
	u, err := url.Parse(strings.TrimSpace(iconURL))
	if err != nil || u.Hostname() == "" {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	if HostLooksLocalNetwork(u.Host) {
		return false
	}
	return u.Path == "/favicon.ico"
}

func GetSiteFaviconAssetURL(bookmarkLink string) string {
	iconURL := GetSiteFaviconURL(bookmarkLink)
	if iconURL == "" {
		return ""
	}
	if isProxyableSiteFaviconURL(iconURL) {
		WarmSiteFavicon(bookmarkLink)
		return siteIconProxyPath + "?src=" + url.QueryEscape(iconURL)
	}
	return iconURL
}

func GetSiteFaviconAssetURLFast(bookmarkLink string) string {
	iconURL := GetSiteFaviconURL(bookmarkLink)
	if iconURL == "" {
		return ""
	}
	if parsed, err := url.Parse(iconURL); err == nil && HostLooksLocalNetwork(parsed.Host) {
		return ""
	}
	if isProxyableSiteFaviconURL(iconURL) {
		if _, _, err := readCachedSiteFavicon(iconURL); err == nil {
			return siteIconProxyPath + "?src=" + url.QueryEscape(iconURL)
		}
		WarmSiteFaviconURL(iconURL)
		return ""
	}
	return iconURL
}

func GetSiteFavicon(bookmarkLink string, fallback string) string {
	iconURL := GetSiteFaviconAssetURL(bookmarkLink)
	if iconURL == "" {
		return fallback
	}
	return `<img src="` + html.EscapeString(iconURL) + `" referrerpolicy="no-referrer" decoding="async" alt="">`
}

func GetSiteFaviconFast(bookmarkLink string, fallback string) string {
	iconURL := GetSiteFaviconAssetURLFast(bookmarkLink)
	if iconURL == "" {
		return fallback
	}
	return `<img src="` + html.EscapeString(iconURL) + `" referrerpolicy="no-referrer" decoding="async" alt="">`
}

func GetYandexFavicon(bookmarkLink string, fallback string) string {
	u, err := url.Parse(bookmarkLink)
	if err != nil || u.Hostname() == "" {
		return fallback
	}
	return `<img src="https://favicon.yandex.net/favicon/` + u.Hostname() + `/"/>`
}

func detectSiteFaviconContentType(data []byte, headerContentType string) (string, bool) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "", false
	}

	lower := bytes.ToLower(trimmed)
	if bytes.Contains(lower, []byte("<svg")) {
		return "image/svg+xml", true
	}
	if bytes.HasPrefix(trimmed, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return "image/png", true
	}
	if len(trimmed) >= 3 && trimmed[0] == 0xff && trimmed[1] == 0xd8 && trimmed[2] == 0xff {
		return "image/jpeg", true
	}
	if bytes.HasPrefix(trimmed, []byte("GIF87a")) || bytes.HasPrefix(trimmed, []byte("GIF89a")) {
		return "image/gif", true
	}
	if len(trimmed) >= 12 && bytes.Equal(trimmed[:4], []byte("RIFF")) && bytes.Equal(trimmed[8:12], []byte("WEBP")) {
		return "image/webp", true
	}
	if bytes.HasPrefix(trimmed, []byte("BM")) {
		return "image/bmp", true
	}
	if len(trimmed) >= 12 && bytes.Equal(trimmed[:4], []byte{0x00, 0x00, 0x01, 0x00}) {
		return "image/x-icon", true
	}
	if len(trimmed) >= 12 && bytes.Equal(trimmed[:4], []byte{0x00, 0x00, 0x02, 0x00}) {
		return "image/x-icon", true
	}
	if len(trimmed) >= 12 && bytes.Equal(trimmed[4:8], []byte("ftyp")) {
		brand := string(trimmed[8:12])
		switch brand {
		case "avif", "avis":
			return "image/avif", true
		}
	}

	detected := strings.TrimSpace(http.DetectContentType(trimmed))
	if idx := strings.Index(detected, ";"); idx >= 0 {
		detected = detected[:idx]
	}
	if strings.HasPrefix(detected, "image/") {
		return detected, true
	}

	headerContentType = strings.TrimSpace(headerContentType)
	if idx := strings.Index(headerContentType, ";"); idx >= 0 {
		headerContentType = headerContentType[:idx]
	}
	headerContentType = strings.TrimSpace(headerContentType)
	if strings.HasPrefix(headerContentType, "image/") {
		lowerText := strings.ToLower(string(trimmed))
		if strings.HasPrefix(lowerText, "<!doctype html") || strings.HasPrefix(lowerText, "<html") {
			return "", false
		}
		return headerContentType, true
	}

	return "", false
}

func WarmSiteFavicon(bookmarkLink string) {
	iconURL := GetSiteFaviconURL(bookmarkLink)
	WarmSiteFaviconURL(iconURL)
}

func WarmSiteFaviconURL(iconURL string) {
	if !isProxyableSiteFaviconURL(iconURL) {
		return
	}
	if _, _, err := readCachedSiteFavicon(iconURL); err == nil {
		return
	}
	go func() {
		_, _, _ = fetchAndCacheSiteFavicon(iconURL)
	}()
}

func ReadCachedPublicSiteFavicon(iconURL string) ([]byte, string, error) {
	if !isProxyableSiteFaviconURL(iconURL) {
		return nil, "", fmt.Errorf("unsupported site favicon url")
	}
	return readCachedSiteFavicon(iconURL)
}

func FetchPublicSiteFavicon(iconURL string) ([]byte, string, error) {
	if !isProxyableSiteFaviconURL(iconURL) {
		return nil, "", fmt.Errorf("unsupported site favicon url")
	}
	return fetchAndCacheSiteFavicon(iconURL)
}

func fetchAndCacheSiteFavicon(iconURL string) ([]byte, string, error) {
	if data, contentType, err := readCachedSiteFavicon(iconURL); err == nil {
		return data, contentType, nil
	}

	key := siteFaviconCacheKey(iconURL)
	wait := make(chan struct{})
	actual, loaded := siteIconInflight.LoadOrStore(key, wait)
	if loaded {
		ch, ok := actual.(chan struct{})
		if ok {
			<-ch
		}
		return readCachedSiteFavicon(iconURL)
	}

	defer close(wait)
	defer siteIconInflight.Delete(key)

	data, contentType, err := downloadSiteFavicon(iconURL)
	if err != nil {
		return nil, "", err
	}
	if err := writeCachedSiteFavicon(iconURL, data); err != nil {
		return nil, "", err
	}
	return data, contentType, nil
}

func downloadSiteFavicon(iconURL string) ([]byte, string, error) {
	req, err := http.NewRequest(http.MethodGet, iconURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "SuperFlare favicon fetcher")

	resp, err := siteIconHTTPClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	reader := io.LimitReader(resp.Body, siteIconMaxBytes+1)
	data, err := io.ReadAll(reader)
	if err != nil {
		return nil, "", err
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty favicon response")
	}
	if len(data) > siteIconMaxBytes {
		return nil, "", fmt.Errorf("favicon too large")
	}

	contentType, ok := detectSiteFaviconContentType(data, resp.Header.Get("Content-Type"))
	if !ok {
		return nil, "", fmt.Errorf("unexpected content type: %s", contentType)
	}

	return data, contentType, nil
}

func readCachedSiteFavicon(iconURL string) ([]byte, string, error) {
	cachePath, err := siteFaviconCachePath(iconURL)
	if err != nil {
		return nil, "", err
	}
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, "", err
	}
	contentType, ok := detectSiteFaviconContentType(data, "")
	if !ok {
		_ = os.Remove(cachePath)
		return nil, "", fmt.Errorf("unexpected cached content type")
	}
	return data, contentType, nil
}

func writeCachedSiteFavicon(iconURL string, data []byte) error {
	cachePath, err := siteFaviconCachePath(iconURL)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return err
	}
	return writeSiteFaviconCacheAtomic(cachePath, data)
}

func siteFaviconCachePath(iconURL string) (string, error) {
	root, err := GetWorkDirE()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, siteIconCacheDir, siteFaviconCacheKey(iconURL)+".bin"), nil
}

func siteFaviconCacheKey(iconURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(iconURL)))
	return fmt.Sprintf("%x", sum)
}

func SiteFaviconCacheKeyForTest(iconURL string) string {
	return siteFaviconCacheKey(iconURL)
}

func writeSiteFaviconCacheAtomic(cachePath string, data []byte) error {
	dir := filepath.Dir(cachePath)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(cachePath)+".tmp-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}

	if _, err := temp.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := temp.Chmod(0644); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, cachePath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}
