package fn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/junfuchang/superflare/internal/netutil"
	xhtml "golang.org/x/net/html"
)

const (
	siteIconProxyPath      = "/assets/site-icons"
	siteIconCacheDir       = "var/cache/site-icons"
	siteIconFallbackHost   = "icon.horse"
	siteIconMaxBytes       = 4 * 1024 * 1024
	siteIconHTMLBytes      = 512 * 1024
	siteIconWarmLimit      = 8
	siteIconRequestTimeout = 4 * time.Second
	siteIconOverallTimeout = 8 * time.Second
)

var (
	siteIconHTTPClient = &http.Client{
		Timeout:       siteIconRequestTimeout,
		Transport:     safeSiteFaviconTransport(),
		CheckRedirect: validateSiteFaviconRedirect,
	}
	siteIconInflight     sync.Map
	siteIconWarmLimiter  = make(chan struct{}, siteIconWarmLimit)
	siteIconFetchLimiter = make(chan struct{}, siteIconWarmLimit)
)

var errSiteFaviconSourceNotAllowed = errors.New("site favicon source is not allowed")

func GetSiteFaviconURL(bookmarkLink string) string {
	bookmarkLink = strings.TrimSpace(bookmarkLink)
	if bookmarkLink == "" {
		return ""
	}
	u, err := url.Parse(bookmarkLink)
	if err != nil || u.Hostname() == "" {
		return ""
	}
	if strings.TrimSpace(u.Scheme) == "" {
		return ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host, Path: "/favicon.ico"}).String()
}

func isProxyableSiteFaviconURL(iconURL string) bool {
	u, err := url.Parse(strings.TrimSpace(iconURL))
	if err != nil || u.Hostname() == "" {
		return false
	}
	if strings.TrimSpace(u.Scheme) == "" {
		return false
	}
	if u.User != nil {
		return false
	}
	return true
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
	select {
	case siteIconWarmLimiter <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-siteIconWarmLimiter }()
		ctx, cancel := context.WithTimeout(context.Background(), siteIconOverallTimeout)
		defer cancel()
		_, _, _ = fetchAndCacheSiteFavicon(ctx, iconURL)
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
	ctx, cancel := context.WithTimeout(context.Background(), siteIconOverallTimeout)
	defer cancel()
	return fetchAndCacheSiteFavicon(ctx, iconURL)
}

func fetchAndCacheSiteFavicon(ctx context.Context, iconURL string) ([]byte, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if data, contentType, err := readCachedSiteFavicon(iconURL); err == nil {
		return data, contentType, nil
	}

	key := siteFaviconCacheKey(iconURL)
	wait := make(chan struct{})
	actual, loaded := siteIconInflight.LoadOrStore(key, wait)
	if loaded {
		ch, ok := actual.(chan struct{})
		if ok {
			select {
			case <-ch:
			case <-ctx.Done():
				return nil, "", ctx.Err()
			}
		}
		return readCachedSiteFavicon(iconURL)
	}

	defer close(wait)
	defer siteIconInflight.Delete(key)
	select {
	case siteIconFetchLimiter <- struct{}{}:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	defer func() { <-siteIconFetchLimiter }()

	data, contentType, err := downloadSiteFavicon(ctx, iconURL)
	if err != nil {
		return nil, "", err
	}
	if err := writeCachedSiteFavicon(iconURL, data); err != nil {
		return nil, "", err
	}
	return data, contentType, nil
}

func downloadSiteFavicon(ctx context.Context, iconURL string) ([]byte, string, error) {
	data, contentType, err := downloadSiteFaviconDirect(ctx, iconURL)
	if err == nil {
		return data, contentType, nil
	}
	if !isRootHTTPFaviconURL(iconURL) {
		return nil, "", err
	}
	attemptErrors := []error{fmt.Errorf("direct favicon fetch failed: %w", err)}
	providerAttempted := false
	if shouldPreferHostedSiteFavicon(err) {
		providerAttempted = true
		if data, contentType, fallbackErr := downloadHostedSiteFavicon(ctx, iconURL); fallbackErr == nil {
			return data, contentType, nil
		} else if fallbackErr != nil {
			attemptErrors = append(attemptErrors, fmt.Errorf("hosted favicon fallback failed: %w", fallbackErr))
		}
	}
	if err := ctx.Err(); err != nil {
		return nil, "", errors.Join(append(attemptErrors, err)...)
	}

	discoveredURL, discoverErr := discoverSiteFaviconFromHTML(ctx, iconURL)
	if discoverErr == nil && discoveredURL != "" && discoveredURL != iconURL {
		data, contentType, discoveredErr := downloadSiteFaviconDirect(ctx, discoveredURL)
		if discoveredErr == nil {
			return data, contentType, nil
		}
		attemptErrors = append(attemptErrors, fmt.Errorf("html favicon fetch failed: %w", discoveredErr))
	} else if discoverErr != nil {
		attemptErrors = append(attemptErrors, fmt.Errorf("html favicon discovery failed: %w", discoverErr))
	}

	if !providerAttempted {
		if data, contentType, fallbackErr := downloadHostedSiteFavicon(ctx, iconURL); fallbackErr == nil {
			return data, contentType, nil
		} else if fallbackErr != nil {
			attemptErrors = append(attemptErrors, fmt.Errorf("hosted favicon fallback failed: %w", fallbackErr))
		}
	}
	return nil, "", errors.Join(attemptErrors...)
}

func shouldPreferHostedSiteFavicon(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkErr net.Error
	return errors.As(err, &networkErr)
}

func downloadHostedSiteFavicon(ctx context.Context, rootIconURL string) ([]byte, string, error) {
	fallbackURL := hostedSiteFaviconURL(rootIconURL)
	if fallbackURL == "" {
		return nil, "", fmt.Errorf("hosted favicon fallback is unavailable")
	}
	return downloadSiteFaviconDirect(ctx, fallbackURL)
}

func downloadSiteFaviconDirect(ctx context.Context, iconURL string) ([]byte, string, error) {
	if err := validateSiteFaviconSource(iconURL); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iconURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SuperFlare favicon fetcher)")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")

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

func isRootHTTPFaviconURL(iconURL string) bool {
	u, err := url.Parse(strings.TrimSpace(iconURL))
	if err != nil || u == nil {
		return false
	}
	return (u.Scheme == "http" || u.Scheme == "https") && u.Hostname() != "" && u.Path == "/favicon.ico"
}

func hostedSiteFaviconURL(rootIconURL string) string {
	u, err := url.Parse(strings.TrimSpace(rootIconURL))
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	host := strings.TrimSuffix(strings.ToLower(strings.TrimSpace(u.Hostname())), ".")
	// The fallback service receives this hostname, so never use it for local or reserved targets.
	if host == "" || HostLooksLocalNetwork(host) || net.ParseIP(host) != nil || isReservedSiteFaviconHost(host) {
		return ""
	}
	return (&url.URL{Scheme: "https", Host: siteIconFallbackHost, Path: "/icon/" + host}).String()
}

func isReservedSiteFaviconHost(host string) bool {
	if host == "example.com" || host == "example.net" || host == "example.org" ||
		strings.HasSuffix(host, ".example.com") || strings.HasSuffix(host, ".example.net") || strings.HasSuffix(host, ".example.org") {
		return true
	}
	return strings.HasSuffix(host, ".invalid") || strings.HasSuffix(host, ".test") || strings.HasSuffix(host, ".example") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".onion") || strings.HasSuffix(host, ".alt")
}

func discoverSiteFaviconFromHTML(ctx context.Context, rootIconURL string) (string, error) {
	base, err := url.Parse(strings.TrimSpace(rootIconURL))
	if err != nil || base == nil || base.Hostname() == "" {
		return "", fmt.Errorf("invalid favicon URL")
	}
	if base.Scheme != "http" && base.Scheme != "https" {
		return "", fmt.Errorf("unsupported page URL scheme")
	}
	base.Path = "/"
	base.RawQuery = ""
	base.Fragment = ""

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SuperFlare favicon fetcher)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := siteIconHTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return "", fmt.Errorf("unexpected page status: %d", resp.StatusCode)
	}

	hrefs, err := collectFaviconHrefs(io.LimitReader(resp.Body, siteIconHTMLBytes))
	if err != nil {
		return "", err
	}
	pageURL := base
	if resp.Request != nil && resp.Request.URL != nil {
		pageURL = resp.Request.URL
	}
	for _, href := range hrefs {
		ref, err := url.Parse(strings.TrimSpace(href))
		if err != nil || ref == nil {
			continue
		}
		resolved := pageURL.ResolveReference(ref)
		if isProxyableSiteFaviconURL(resolved.String()) {
			return resolved.String(), nil
		}
	}
	return "", fmt.Errorf("no html favicon found")
}

func collectFaviconHrefs(reader io.Reader) ([]string, error) {
	tokenizer := xhtml.NewTokenizer(reader)
	var out []string
	for {
		switch tokenizer.Next() {
		case xhtml.ErrorToken:
			err := tokenizer.Err()
			if err != nil && !errors.Is(err, io.EOF) {
				return nil, err
			}
			return out, nil
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
			token := tokenizer.Token()
			if !strings.EqualFold(token.Data, "link") {
				continue
			}
			if href, ok := faviconHrefFromAttributes(token.Attr); ok {
				out = append(out, href)
			}
		case xhtml.EndTagToken:
			if strings.EqualFold(tokenizer.Token().Data, "head") {
				return out, nil
			}
		}
	}
}

func faviconHrefFromAttributes(attributes []xhtml.Attribute) (string, bool) {
	var rel string
	var href string
	for _, attr := range attributes {
		switch strings.ToLower(strings.TrimSpace(attr.Key)) {
		case "rel":
			rel = attr.Val
		case "href":
			href = strings.TrimSpace(attr.Val)
		}
	}
	if href == "" || !relLooksLikeFavicon(rel) {
		return "", false
	}
	return href, true
}

func relLooksLikeFavicon(rel string) bool {
	rel = strings.ToLower(strings.TrimSpace(rel))
	if rel == "" {
		return false
	}
	for _, token := range strings.Fields(strings.ReplaceAll(rel, ",", " ")) {
		if token == "icon" || strings.Contains(token, "-icon") {
			return true
		}
	}
	return false
}

func validateSiteFaviconRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("stopped after too many site favicon redirects")
	}
	if req == nil || req.URL == nil {
		return fmt.Errorf("%w: invalid redirect URL", errSiteFaviconSourceNotAllowed)
	}
	return validateSiteFaviconSource(req.URL.String())
}

func safeSiteFaviconTransport() http.RoundTripper {
	dialer := &net.Dialer{Timeout: siteIconRequestTimeout}
	return &http.Transport{
		Proxy:                 netutil.ProxyFromCurrentEnvironment,
		DialContext:           dialer.DialContext,
		TLSHandshakeTimeout:   siteIconRequestTimeout,
		ResponseHeaderTimeout: siteIconRequestTimeout,
	}
}

func validateSiteFaviconSource(iconURL string) error {
	u, err := url.Parse(strings.TrimSpace(iconURL))
	if err != nil || u == nil || u.Hostname() == "" {
		return fmt.Errorf("%w: invalid URL", errSiteFaviconSourceNotAllowed)
	}
	if strings.TrimSpace(u.Scheme) == "" {
		return fmt.Errorf("%w: missing scheme", errSiteFaviconSourceNotAllowed)
	}
	if u.User != nil {
		return fmt.Errorf("%w: userinfo is not supported", errSiteFaviconSourceNotAllowed)
	}
	return nil
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
