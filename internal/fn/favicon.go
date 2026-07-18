package fn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/binary"
	"encoding/xml"
	"errors"
	"fmt"
	"html"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
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
	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
	xhtml "golang.org/x/net/html"
)

const (
	siteIconProxyPath          = "/assets/site-icons"
	siteIconCacheGeneration    = "2026-07-verified-hosted"
	siteIconCacheDir           = "var/cache/site-icons"
	siteIconMaxBytes           = 4 * 1024 * 1024
	siteIconMaxDecodedPixels   = 4 * 1024 * 1024
	siteIconHTMLBytes          = 512 * 1024
	siteIconWarmLimit          = 8
	siteIconDecodeLimit        = 2
	siteIconRequestTimeout     = 4 * time.Second
	siteIconOverallTimeout     = 10 * time.Second
	siteIconHostedTimeout      = 6 * time.Second
	siteIconFailureTTL         = 5 * time.Minute
	siteIconFailureMaxEntries  = 1024
	siteIconHostedHost         = "favicon.im"
	siteIconHostedAssetHost    = "a.favicon.im"
	siteIconHostedSourceHeader = "X-Favicon-Source"
)

var (
	siteIconHTTPClient = &http.Client{
		Timeout:       siteIconRequestTimeout,
		Transport:     safeSiteFaviconTransport(),
		CheckRedirect: validateSiteFaviconRedirect,
	}
	siteIconInflight      sync.Map
	siteIconFailures      siteIconFailureCache
	siteIconValidated     sync.Map
	siteIconWarmLimiter   = make(chan struct{}, siteIconWarmLimit)
	siteIconFetchLimiter  = make(chan struct{}, siteIconWarmLimit)
	siteIconDecodeLimiter = make(chan struct{}, siteIconDecodeLimit)
)

var (
	errSiteFaviconSourceNotAllowed = errors.New("site favicon source is not allowed")
	errSiteFaviconRetryCooldown    = errors.New("site favicon fetch is temporarily suppressed after a recent failure")
	errSiteFaviconRedirectRejected = errors.New("site favicon redirect was rejected by policy")
)

type siteIconFailureCache struct {
	mu      sync.Mutex
	entries map[string]time.Time
}

func (c *siteIconFailureCache) Store(key string, retryAfter time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		c.entries = make(map[string]time.Time)
	}
	if _, exists := c.entries[key]; !exists && len(c.entries) >= siteIconFailureMaxEntries {
		now := time.Now()
		for existingKey, existingRetryAfter := range c.entries {
			if !now.Before(existingRetryAfter) {
				delete(c.entries, existingKey)
			}
		}
		if len(c.entries) >= siteIconFailureMaxEntries {
			var oldestKey string
			var oldestRetryAfter time.Time
			for existingKey, existingRetryAfter := range c.entries {
				if oldestKey == "" || existingRetryAfter.Before(oldestRetryAfter) {
					oldestKey = existingKey
					oldestRetryAfter = existingRetryAfter
				}
			}
			delete(c.entries, oldestKey)
		}
	}
	c.entries[key] = retryAfter
}

func (c *siteIconFailureCache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

func (c *siteIconFailureCache) Active(key string, now time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	retryAfter, ok := c.entries[key]
	if !ok {
		return false
	}
	if !now.Before(retryAfter) {
		delete(c.entries, key)
		return false
	}
	return true
}

func (c *siteIconFailureCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

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
		if siteFaviconFailureActive(iconURL) {
			return ""
		}
		return siteIconProxyURL(iconURL)
	}
	return iconURL
}

func GetSiteFaviconAssetURLFast(bookmarkLink string) string {
	iconURL := GetSiteFaviconURL(bookmarkLink)
	if iconURL == "" {
		return ""
	}
	if isProxyableSiteFaviconURL(iconURL) {
		if hasValidatedSiteFaviconFast(iconURL) {
			return siteIconProxyURL(iconURL)
		}
		return ""
	}
	return iconURL
}

type siteIconCacheStamp struct {
	size             int64
	modifiedUnixNano int64
}

func siteIconStamp(info os.FileInfo) (siteIconCacheStamp, bool) {
	if info == nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > siteIconMaxBytes {
		return siteIconCacheStamp{}, false
	}
	return siteIconCacheStamp{
		size:             info.Size(),
		modifiedUnixNano: info.ModTime().UnixNano(),
	}, true
}

func hasValidatedSiteFaviconFast(iconURL string) bool {
	cachePath, err := siteFaviconCachePath(iconURL)
	if err != nil {
		return false
	}
	info, err := os.Stat(cachePath)
	if err != nil {
		siteIconValidated.Delete(cachePath)
		return false
	}
	stamp, ok := siteIconStamp(info)
	if !ok {
		siteIconValidated.Delete(cachePath)
		return false
	}
	if cached, loaded := siteIconValidated.Load(cachePath); loaded && cached == stamp {
		return true
	}

	select {
	case siteIconDecodeLimiter <- struct{}{}:
		defer func() { <-siteIconDecodeLimiter }()
	default:
		return false
	}
	_, _, err = readCachedSiteFaviconFile(cachePath)
	return err == nil
}

func siteIconProxyURL(iconURL string) string {
	query := url.Values{"src": {iconURL}}
	return siteIconProxyPath + "?" + query.Encode()
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
	return `<img src="` + html.EscapeString(iconURL) + `" referrerpolicy="no-referrer" decoding="sync" alt="">`
}

func GetYandexFavicon(bookmarkLink string, fallback string) string {
	u, err := url.Parse(bookmarkLink)
	if err != nil || u.Hostname() == "" {
		return fallback
	}
	return `<img src="https://favicon.yandex.net/favicon/` + u.Hostname() + `/"/>`
}

func detectSiteFaviconContentType(data []byte, _ string) (string, bool) {
	svgData := bytes.TrimSpace(data)
	svgData = bytes.TrimPrefix(svgData, []byte{0xef, 0xbb, 0xbf})
	svgData = bytes.TrimSpace(svgData)
	if len(svgData) == 0 {
		return "", false
	}

	if svgData[0] == '<' && isStandaloneSVGDocument(svgData) {
		return "image/svg+xml", true
	}
	if bytes.HasPrefix(data, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
		return detectDecodedSiteFavicon(data, "png", "image/png")
	}
	if len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff {
		return detectDecodedSiteFavicon(data, "jpeg", "image/jpeg")
	}
	if bytes.HasPrefix(data, []byte("GIF87a")) || bytes.HasPrefix(data, []byte("GIF89a")) {
		return detectDecodedSiteFavicon(data, "gif", "image/gif")
	}
	if len(data) >= 12 && bytes.Equal(data[:4], []byte("RIFF")) && bytes.Equal(data[8:12], []byte("WEBP")) {
		return detectDecodedSiteFavicon(data, "webp", "image/webp")
	}
	if bytes.HasPrefix(data, []byte("BM")) {
		return detectDecodedSiteFavicon(data, "bmp", "image/bmp")
	}
	if validSiteFaviconICO(data, 1) {
		return "image/x-icon", true
	}
	if validSiteFaviconICO(data, 2) {
		return "image/x-icon", true
	}
	return "", false
}

func isStandaloneSVGDocument(data []byte) bool {
	decoder := xml.NewDecoder(bytes.NewReader(data))
	foundRoot := false
	closedRoot := false
	depth := 0
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			return foundRoot && closedRoot && depth == 0
		}
		if err != nil {
			return false
		}
		switch current := token.(type) {
		case xml.StartElement:
			if !foundRoot {
				if current.Name.Local != "svg" ||
					(current.Name.Space != "" && current.Name.Space != "http://www.w3.org/2000/svg") {
					return false
				}
				foundRoot = true
			} else if closedRoot {
				return false
			}
			depth++
		case xml.EndElement:
			if !foundRoot || closedRoot || depth == 0 {
				return false
			}
			depth--
			if depth == 0 {
				closedRoot = true
			}
		case xml.CharData:
			if (!foundRoot || closedRoot) && len(bytes.TrimSpace(current)) != 0 {
				return false
			}
		}
	}
}

func detectDecodedSiteFavicon(data []byte, expectedFormat string, contentType string) (string, bool) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil || format != expectedFormat || config.Width <= 0 || config.Height <= 0 {
		return "", false
	}
	if uint64(config.Width)*uint64(config.Height) > uint64(siteIconMaxDecodedPixels) {
		return "", false
	}
	decoded, format, err := image.Decode(bytes.NewReader(data))
	if err != nil || format != expectedFormat || decoded.Bounds().Dx() <= 0 || decoded.Bounds().Dy() <= 0 {
		return "", false
	}
	return contentType, true
}

func validSiteFaviconICO(data []byte, expectedType uint16) bool {
	if len(data) < 6 || binary.LittleEndian.Uint16(data[0:2]) != 0 || binary.LittleEndian.Uint16(data[2:4]) != expectedType {
		return false
	}
	count := int(binary.LittleEndian.Uint16(data[4:6]))
	directoryEnd := 6 + count*16
	if count == 0 || directoryEnd > len(data) {
		return false
	}
	for index := 0; index < count; index++ {
		entry := data[6+index*16 : 6+(index+1)*16]
		size := binary.LittleEndian.Uint32(entry[8:12])
		offset := binary.LittleEndian.Uint32(entry[12:16])
		end := uint64(offset) + uint64(size)
		if size == 0 || uint64(offset) < uint64(directoryEnd) || end > uint64(len(data)) {
			return false
		}
		payload := data[int(offset):int(end)]
		if bytes.HasPrefix(payload, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n'}) {
			if _, ok := detectDecodedSiteFavicon(payload, "png", "image/png"); !ok {
				return false
			}
		} else if !validSiteFaviconICOBitmap(payload) {
			return false
		}
	}
	return true
}

func validSiteFaviconICOBitmap(data []byte) bool {
	if len(data) < 12 {
		return false
	}
	headerSize := int(binary.LittleEndian.Uint32(data[0:4]))
	if headerSize == 12 {
		width := uint64(binary.LittleEndian.Uint16(data[4:6]))
		encodedHeight := uint64(binary.LittleEndian.Uint16(data[6:8]))
		planes := binary.LittleEndian.Uint16(data[8:10])
		bitsPerPixel := binary.LittleEndian.Uint16(data[10:12])
		if width == 0 || encodedHeight == 0 || encodedHeight%2 != 0 || planes != 1 || !validSiteFaviconICOBitDepth(bitsPerPixel) {
			return false
		}
		paletteBytes := uint64(0)
		if bitsPerPixel <= 8 {
			paletteBytes = uint64(1<<bitsPerPixel) * 3
		}
		return validSiteFaviconICOBitmapDataLength(data, uint64(headerSize)+paletteBytes, width, encodedHeight/2, bitsPerPixel)
	}
	if headerSize < 40 || headerSize > len(data) {
		return false
	}
	width := int64(int32(binary.LittleEndian.Uint32(data[4:8])))
	encodedHeight := int64(int32(binary.LittleEndian.Uint32(data[8:12])))
	planes := binary.LittleEndian.Uint16(data[12:14])
	bitsPerPixel := binary.LittleEndian.Uint16(data[14:16])
	compression := binary.LittleEndian.Uint32(data[16:20])
	if width <= 0 || encodedHeight == 0 || planes != 1 || !validSiteFaviconICOBitDepth(bitsPerPixel) ||
		(compression != 0 && compression != 3) {
		return false
	}
	if encodedHeight < 0 {
		encodedHeight = -encodedHeight
	}
	if encodedHeight%2 != 0 {
		return false
	}
	pixelOffset := uint64(headerSize)
	if compression == 3 && headerSize == 40 {
		pixelOffset += 12
	}
	if bitsPerPixel <= 8 {
		paletteCount := uint64(binary.LittleEndian.Uint32(data[32:36]))
		maximumPaletteCount := uint64(1 << bitsPerPixel)
		if paletteCount == 0 {
			paletteCount = maximumPaletteCount
		} else if paletteCount > maximumPaletteCount {
			return false
		}
		pixelOffset += paletteCount * 4
	}
	return validSiteFaviconICOBitmapDataLength(data, pixelOffset, uint64(width), uint64(encodedHeight/2), bitsPerPixel)
}

func validSiteFaviconICOBitDepth(bitsPerPixel uint16) bool {
	switch bitsPerPixel {
	case 1, 4, 8, 16, 24, 32:
		return true
	default:
		return false
	}
}

func validSiteFaviconICOBitmapDataLength(data []byte, pixelOffset uint64, width uint64, height uint64, bitsPerPixel uint16) bool {
	if width == 0 || height == 0 || width*height > uint64(siteIconMaxDecodedPixels) || pixelOffset > uint64(len(data)) {
		return false
	}
	xorRowBytes := ((width*uint64(bitsPerPixel) + 31) / 32) * 4
	andMaskRowBytes := ((width + 31) / 32) * 4
	xorEnd := pixelOffset + xorRowBytes*height
	if xorEnd > uint64(len(data)) {
		return false
	}
	if xorEnd == uint64(len(data)) {
		return true
	}
	return xorEnd+andMaskRowBytes*height <= uint64(len(data))
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

func siteFaviconFailureActive(iconURL string) bool {
	key := siteFaviconCacheKey(iconURL)
	return siteIconFailures.Active(key, time.Now())
}

func recordSiteFaviconFailure(iconURL string) {
	siteIconFailures.Store(siteFaviconCacheKey(iconURL), time.Now().Add(siteIconFailureTTL))
}

func clearSiteFaviconFailure(iconURL string) {
	siteIconFailures.Delete(siteFaviconCacheKey(iconURL))
}

func fetchAndCacheSiteFavicon(ctx context.Context, iconURL string) ([]byte, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if data, contentType, err := readCachedSiteFaviconContext(ctx, iconURL); err == nil {
		return data, contentType, nil
	} else if ctx.Err() != nil {
		return nil, "", ctx.Err()
	}
	if siteFaviconFailureActive(iconURL) {
		return nil, "", errSiteFaviconRetryCooldown
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
		data, contentType, err := readCachedSiteFaviconContext(ctx, iconURL)
		if err != nil && siteFaviconFailureActive(iconURL) {
			return nil, "", errSiteFaviconRetryCooldown
		}
		return data, contentType, err
	}

	defer close(wait)
	defer siteIconInflight.Delete(key)
	select {
	case siteIconFetchLimiter <- struct{}{}:
	case <-ctx.Done():
		return nil, "", ctx.Err()
	}
	defer func() { <-siteIconFetchLimiter }()
	if siteFaviconFailureActive(iconURL) {
		return nil, "", errSiteFaviconRetryCooldown
	}

	data, contentType, err := downloadSiteFavicon(ctx, iconURL)
	if err != nil {
		recordSiteFaviconFailure(iconURL)
		return nil, "", err
	}
	if err := writeCachedSiteFavicon(iconURL, data); err != nil {
		return nil, "", err
	}
	clearSiteFaviconFailure(iconURL)
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
	if isDefinitiveSiteFaviconDNSNotFound(err) {
		return nil, "", fmt.Errorf("favicon host does not resolve: %w", err)
	}
	attemptErrors := []error{fmt.Errorf("direct favicon fetch failed: %w", err)}
	providerAttempted := false
	if shouldPreferHostedSiteFavicon(err) {
		providerAttempted = true
		if data, contentType, hostedErr := downloadHostedSiteFavicon(ctx, iconURL); hostedErr == nil {
			return data, contentType, nil
		} else {
			attemptErrors = append(attemptErrors, fmt.Errorf("verified hosted favicon recovery failed: %w", hostedErr))
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
		if data, contentType, hostedErr := downloadHostedSiteFavicon(ctx, iconURL); hostedErr == nil {
			return data, contentType, nil
		} else {
			attemptErrors = append(attemptErrors, fmt.Errorf("verified hosted favicon recovery failed: %w", hostedErr))
		}
	}

	return nil, "", errors.Join(attemptErrors...)
}

func isDefinitiveSiteFaviconDNSNotFound(err error) bool {
	var dnsErr *net.DNSError
	return errors.As(err, &dnsErr) && dnsErr.IsNotFound
}

func shouldPreferHostedSiteFavicon(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errSiteFaviconRedirectRejected) {
		return false
	}
	err = unwrapSiteFaviconURLError(err)
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var networkErr net.Error
	if errors.As(err, &networkErr) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var certificateErr *tls.CertificateVerificationError
	if errors.As(err, &certificateErr) {
		return true
	}
	var recordHeaderErr tls.RecordHeaderError
	return errors.As(err, &recordHeaderErr)
}

func unwrapSiteFaviconURLError(err error) error {
	for {
		var urlErr *url.Error
		if !errors.As(err, &urlErr) || urlErr == nil || urlErr.Err == nil || urlErr.Err == err {
			return err
		}
		err = urlErr.Err
	}
}

func downloadHostedSiteFavicon(ctx context.Context, rootIconURL string) ([]byte, string, error) {
	hostedURL := hostedSiteFaviconURL(rootIconURL)
	if hostedURL == "" {
		return nil, "", fmt.Errorf("verified hosted favicon recovery is unavailable")
	}
	return downloadSiteFaviconWithClient(ctx, hostedURL, hostedSiteFaviconHTTPClient(), validateHostedSiteFaviconResponse)
}

func downloadSiteFaviconDirect(ctx context.Context, iconURL string) ([]byte, string, error) {
	return downloadSiteFaviconWithClient(ctx, iconURL, siteIconHTTPClient, nil)
}

func downloadSiteFaviconWithClient(
	ctx context.Context,
	iconURL string,
	client *http.Client,
	validateResponse func(*http.Response) error,
) ([]byte, string, error) {
	if err := validateSiteFaviconSource(iconURL); err != nil {
		return nil, "", err
	}
	if client == nil {
		return nil, "", fmt.Errorf("site favicon HTTP client is unavailable")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, iconURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; SuperFlare favicon fetcher)")
	req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}
	if validateResponse != nil {
		if err := validateResponse(resp); err != nil {
			return nil, "", err
		}
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

	if err := acquireSiteIconDecodeSlot(ctx); err != nil {
		return nil, "", err
	}
	defer func() { <-siteIconDecodeLimiter }()
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
	if host == "" || HostLooksLocalNetwork(host) || net.ParseIP(host) != nil || isReservedSiteFaviconHost(host) {
		return ""
	}
	query := url.Values{"throw-error-on-404": {"true"}}
	return (&url.URL{
		Scheme:   "https",
		Host:     siteIconHostedHost,
		Path:     "/" + host,
		RawQuery: query.Encode(),
	}).String()
}

func isReservedSiteFaviconHost(host string) bool {
	if host == "example.com" || host == "example.net" || host == "example.org" ||
		strings.HasSuffix(host, ".example.com") || strings.HasSuffix(host, ".example.net") || strings.HasSuffix(host, ".example.org") {
		return true
	}
	return strings.HasSuffix(host, ".invalid") || strings.HasSuffix(host, ".test") || strings.HasSuffix(host, ".example") ||
		strings.HasSuffix(host, ".internal") || strings.HasSuffix(host, ".onion") || strings.HasSuffix(host, ".alt")
}

func isTrustedHostedSiteFaviconSource(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "origin", "cache-fresh", "cache-stale":
		return true
	default:
		return false
	}
}

func validateHostedSiteFaviconResponse(resp *http.Response) error {
	if resp == nil {
		return fmt.Errorf("invalid hosted favicon response")
	}
	if resp.Request == nil || !isAllowedHostedSiteFaviconURL(resp.Request.URL) {
		return fmt.Errorf("hosted favicon response URL is not allowed")
	}
	sources := resp.Header.Values(siteIconHostedSourceHeader)
	if len(sources) != 1 {
		return fmt.Errorf("hosted favicon response must contain exactly one source header")
	}
	source := sources[0]
	if !isTrustedHostedSiteFaviconSource(source) {
		return fmt.Errorf("untrusted hosted favicon source: %q", strings.TrimSpace(source))
	}
	return nil
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
		return fmt.Errorf("%w: stopped after too many redirects", errSiteFaviconRedirectRejected)
	}
	if req == nil || req.URL == nil {
		return fmt.Errorf("%w: invalid redirect URL", errSiteFaviconRedirectRejected)
	}
	if req.URL.Scheme != "http" && req.URL.Scheme != "https" {
		return fmt.Errorf("%w: unsupported redirect scheme", errSiteFaviconRedirectRejected)
	}
	if err := validateSiteFaviconSource(req.URL.String()); err != nil {
		return fmt.Errorf("%w: %w", errSiteFaviconRedirectRejected, err)
	}
	return nil
}

func validateHostedSiteFaviconRedirect(req *http.Request, via []*http.Request) error {
	if len(via) == 0 {
		return fmt.Errorf("hosted favicon redirect is missing its initial request")
	}
	if len(via) >= 5 {
		return fmt.Errorf("stopped after too many hosted favicon redirects")
	}
	if req == nil || req.URL == nil || via[0] == nil || via[0].URL == nil {
		return fmt.Errorf("invalid hosted favicon redirect URL")
	}
	if !isAllowedHostedSiteFaviconURL(req.URL) || !isAllowedHostedSiteFaviconURL(via[0].URL) {
		return fmt.Errorf("hosted favicon redirect target is not allowed")
	}
	if req.URL.EscapedPath() != via[0].URL.EscapedPath() || req.URL.RawQuery != via[0].URL.RawQuery {
		return fmt.Errorf("hosted favicon redirect changed the requested domain")
	}
	return nil
}

func isAllowedHostedSiteFaviconURL(u *url.URL) bool {
	if u == nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" {
		return false
	}
	host := strings.ToLower(strings.TrimSpace(u.Hostname()))
	if host != siteIconHostedHost && host != siteIconHostedAssetHost {
		return false
	}
	if port := strings.TrimSpace(u.Port()); port != "" && port != "443" {
		return false
	}
	query := u.Query()
	return len(query) == 1 && len(query["throw-error-on-404"]) == 1 && query.Get("throw-error-on-404") == "true"
}

func hostedSiteFaviconHTTPClient() *http.Client {
	client := *siteIconHTTPClient
	client.Timeout = siteIconHostedTimeout
	client.CheckRedirect = validateHostedSiteFaviconRedirect
	return &client
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
	return readCachedSiteFaviconContext(context.Background(), iconURL)
}

func readCachedSiteFaviconContext(ctx context.Context, iconURL string) ([]byte, string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cachePath, err := siteFaviconCachePath(iconURL)
	if err != nil {
		return nil, "", err
	}
	if _, err := os.Stat(cachePath); err != nil {
		siteIconValidated.Delete(cachePath)
		return nil, "", err
	}
	if err := acquireSiteIconDecodeSlot(ctx); err != nil {
		return nil, "", err
	}
	defer func() { <-siteIconDecodeLimiter }()
	return readCachedSiteFaviconFile(cachePath)
}

func readCachedSiteFaviconFile(cachePath string) ([]byte, string, error) {
	file, err := os.Open(cachePath)
	if err != nil {
		siteIconValidated.Delete(cachePath)
		return nil, "", err
	}
	info, statErr := file.Stat()
	data, readErr := io.ReadAll(io.LimitReader(file, siteIconMaxBytes+1))
	closeErr := file.Close()
	if statErr != nil {
		siteIconValidated.Delete(cachePath)
		return nil, "", statErr
	}
	if readErr != nil {
		siteIconValidated.Delete(cachePath)
		return nil, "", readErr
	}
	if closeErr != nil {
		siteIconValidated.Delete(cachePath)
		return nil, "", closeErr
	}
	if len(data) > siteIconMaxBytes {
		siteIconValidated.Delete(cachePath)
		_ = os.Remove(cachePath)
		return nil, "", fmt.Errorf("cached favicon too large")
	}
	contentType, ok := detectSiteFaviconContentType(data, "")
	if !ok {
		siteIconValidated.Delete(cachePath)
		_ = os.Remove(cachePath)
		return nil, "", fmt.Errorf("unexpected cached content type")
	}
	if stamp, ok := siteIconStamp(info); ok {
		siteIconValidated.Store(cachePath, stamp)
	}
	return data, contentType, nil
}

func acquireSiteIconDecodeSlot(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case siteIconDecodeLimiter <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
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
	normalizedURL := strings.TrimSpace(iconURL)
	sum := sha256.Sum256([]byte(siteIconCacheGeneration + "\x00" + normalizedURL))
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
