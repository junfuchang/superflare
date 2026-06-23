package fn

import (
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

func GetSiteFavicon(bookmarkLink string, fallback string) string {
	iconURL := GetSiteFaviconAssetURL(bookmarkLink)
	if iconURL == "" {
		return fallback
	}
	return `<img src="` + html.EscapeString(iconURL) + `" referrerpolicy="no-referrer" decoding="async" alt="">`
}

func GetYandexFavicon(bookmarkLink string, fallback string) string {
	u, err := url.Parse(bookmarkLink)
	if err != nil {
		return fallback
	}
	return `<img src="https://favicon.yandex.net/favicon/` + u.Hostname() + `/"/>`
}

func WarmSiteFavicon(bookmarkLink string) {
	iconURL := GetSiteFaviconURL(bookmarkLink)
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

	contentType := resp.Header.Get("Content-Type")
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = contentType[:idx]
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	if !strings.HasPrefix(contentType, "image/") && contentType != "application/octet-stream" {
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
	return data, http.DetectContentType(data), nil
}

func writeCachedSiteFavicon(iconURL string, data []byte) error {
	cachePath, err := siteFaviconCachePath(iconURL)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return err
	}
	return os.WriteFile(cachePath, data, 0644)
}

func siteFaviconCachePath(iconURL string) (string, error) {
	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Join(root, siteIconCacheDir, siteFaviconCacheKey(iconURL)+".bin"), nil
}

func siteFaviconCacheKey(iconURL string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(iconURL)))
	return fmt.Sprintf("%x", sum)
}
