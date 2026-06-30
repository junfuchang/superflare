package background

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp"

	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/netutil"
)

const (
	UploadedFullPath             = "/user-assets/background"
	UploadedPreviewPath          = "/user-assets/background-preview"
	RemoteAssetPath              = "/assets/background-image"
	uploadDir                    = "var/uploads"
	uploadStageDir               = "var/uploads-stage"
	cacheDir                     = "var/cache/backgrounds"
	previewLongEdge              = 320
	fullLongEdge                 = 2200
	sourceMaxBytes         int64 = 32 << 20
	uploadedVariantVersion       = "2"
)

const backgroundFileMode = 0644

const InlineLoaderScript = `(function(){var bg=document.querySelector('.page-background');if(!bg){return;}var preview=bg.querySelector('.page-background-preview');var full=bg.querySelector('.page-background-full');if(!full){return;}function usePreviewLayer(){if(bg.classList.contains('has-preview')){return;}bg.classList.add('has-preview');if(document.body){document.body.classList.add('has-preview-background');}}function settleBody(){if(document.body){document.body.classList.add('has-loaded-background');}}function afterReveal(){if(typeof window.requestAnimationFrame==='function'){window.requestAnimationFrame(function(){window.requestAnimationFrame(settleBody);});return;}settleBody();}function startReveal(){if(bg.classList.contains('is-loaded')){return;}usePreviewLayer();bg.classList.add('is-loaded');afterReveal();}function reveal(){if(typeof full.decode==='function'){full.decode().catch(function(){}).then(startReveal);return;}startReveal();}if(preview){if(preview.complete&&preview.naturalWidth>0){usePreviewLayer();}else{preview.addEventListener('load',usePreviewLayer,{once:true});preview.addEventListener('error',function(){if(document.body){document.body.classList.add('has-preview-background');}},{once:true});}}if(full.complete&&full.naturalWidth>0){reveal();return;}full.addEventListener('load',reveal,{once:true});full.addEventListener('error',function(){bg.classList.add('is-failed');},{once:true});}());`

type Assets struct {
	Enabled        bool
	PreviewURL     string
	PreviewDataURL string
	FullURL        string
	AccentColor    string
}

type StagedUploadedBackgroundActivation struct {
	activeDir string
	backupDir string
	hasBackup bool
	finalized bool
}

var (
	httpClient = &http.Client{
		Timeout:       15 * time.Second,
		Transport:     safeBackgroundTransport(),
		CheckRedirect: validateBackgroundRedirect,
	}
	inflight sync.Map
)

const inlinePreviewMaxBytes = 64 << 10

var ErrRemoteSourceNotAllowed = errors.New("remote background source is not allowed")

func ResolveAssets(options model.Application) Assets {
	source := strings.TrimSpace(options.BackgroundImage)
	if source == "" {
		return Assets{}
	}

	if usesUploadedSource(source, options.BackgroundImageMode) {
		_ = EnsureUploadedBackgroundVariants()
		variantLoader := func() func() ([]byte, string, error) {
			var loaded bool
			var cachedData []byte
			var cachedType string
			var cachedErr error
			return func() ([]byte, string, error) {
				if loaded {
					return cachedData, cachedType, cachedErr
				}
				cachedData, cachedType, cachedErr = FetchUploadedVariant("preview")
				loaded = true
				return cachedData, cachedType, cachedErr
			}
		}()
		assets := Assets{
			Enabled:    true,
			PreviewURL: UploadedPreviewPath,
			FullURL:    UploadedFullPath,
		}
		assets.PreviewDataURL = buildInlinePreviewDataURL(variantLoader)
		assets.AccentColor = detectAccentColor(variantLoader)
		return assets
	}

	if isHTTPSource(source) {
		WarmRemoteVariants(source)
		escaped := url.QueryEscape(source)
		cachedData, cachedType, cachedErr := readCachedVariant(source, "preview")
		variantLoader := func() ([]byte, string, error) {
			return cachedData, cachedType, cachedErr
		}
		assets := Assets{
			Enabled:    true,
			PreviewURL: RemoteAssetPath + "?variant=preview&src=" + escaped,
			FullURL:    RemoteAssetPath + "?variant=full&src=" + escaped,
		}
		assets.PreviewDataURL = buildInlinePreviewDataURL(variantLoader)
		assets.AccentColor = detectAccentColor(variantLoader)
		return assets
	}

	variantLoader := func() func() ([]byte, string, error) {
		var loaded bool
		var cachedData []byte
		var cachedType string
		var cachedErr error
		return func() ([]byte, string, error) {
			if loaded {
				return cachedData, cachedType, cachedErr
			}
			data, contentType, err := downloadSource(source)
			if err != nil {
				cachedErr = err
				loaded = true
				return nil, "", err
			}
			cachedData, cachedType, cachedErr = makePreviewOnly(data, contentType)
			loaded = true
			return cachedData, cachedType, cachedErr
		}
	}()
	assets := Assets{
		Enabled:    true,
		PreviewURL: source,
		FullURL:    source,
	}
	assets.PreviewDataURL = buildInlinePreviewDataURL(variantLoader)
	assets.AccentColor = detectAccentColor(variantLoader)
	return assets
}

func PreviewSource(assets Assets) string {
	if strings.TrimSpace(assets.PreviewDataURL) != "" {
		return assets.PreviewDataURL
	}
	return assets.PreviewURL
}

func WarmRemoteVariants(source string) {
	if !isHTTPSource(source) {
		return
	}
	if cacheExists(source, "preview") && cacheExists(source, "full") {
		return
	}
	go func() {
		_, _, _ = FetchRemoteVariant(source, "preview")
		_, _, _ = FetchRemoteVariant(source, "full")
	}()
}

func PrepareUploadedBackground(fileName string, reader io.Reader) (string, error) {
	if _, err := PrepareUploadedBackgroundStage(fileName, reader); err != nil {
		return "", err
	}
	if err := PromoteStagedUploadedBackground(); err != nil {
		_ = DiscardStagedUploadedBackgrounds()
		return "", err
	}
	return UploadedFullPath, nil
}

func PrepareUploadedBackgroundStage(fileName string, reader io.Reader) (string, error) {
	ext := strings.ToLower(filepath.Ext(strings.TrimSpace(fileName)))
	switch ext {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif":
	default:
		return "", fmt.Errorf("only jpg/png/webp/gif background images are supported")
	}

	data, err := io.ReadAll(io.LimitReader(reader, sourceMaxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > sourceMaxBytes {
		return "", fmt.Errorf("background image is too large")
	}

	root, err := os.Getwd()
	if err != nil {
		return "", err
	}
	targetDir := filepath.Join(root, uploadStageDir)
	if err := replaceUploadedVariantDir(targetDir, "background-source"+ext, data); err != nil {
		return "", err
	}

	return UploadedFullPath, nil
}

func PromoteStagedUploadedBackground() error {
	activation, err := BeginStagedUploadedBackgroundActivation()
	if err != nil {
		return err
	}
	return activation.Commit()
}

func BeginStagedUploadedBackgroundActivation() (*StagedUploadedBackgroundActivation, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	activeDir := filepath.Join(root, uploadDir)
	stageDir := filepath.Join(root, uploadStageDir)
	backupDir := filepath.Join(root, uploadDir+"-backup")

	hasStageFiles, err := hasUploadedFilesIn(stageDir)
	if err != nil {
		return nil, err
	}
	if !hasStageFiles {
		return nil, os.ErrNotExist
	}

	if err := os.RemoveAll(backupDir); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	activation := &StagedUploadedBackgroundActivation{
		activeDir: activeDir,
		backupDir: backupDir,
	}
	activeExists := false
	if stat, err := os.Stat(activeDir); err == nil && stat.IsDir() {
		activeExists = true
		activation.hasBackup = true
		if err := os.Rename(activeDir, backupDir); err != nil {
			return nil, err
		}
	}

	if err := os.Rename(stageDir, activeDir); err != nil {
		if activeExists {
			_ = os.Rename(backupDir, activeDir)
		}
		return nil, err
	}

	return activation, nil
}

func (activation *StagedUploadedBackgroundActivation) Commit() error {
	if activation == nil || activation.finalized {
		return nil
	}
	activation.finalized = true
	if activation.hasBackup {
		if err := os.RemoveAll(activation.backupDir); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func (activation *StagedUploadedBackgroundActivation) Rollback() error {
	if activation == nil || activation.finalized {
		return nil
	}
	activation.finalized = true
	var rollbackErr error
	if err := os.RemoveAll(activation.activeDir); err != nil && !os.IsNotExist(err) {
		rollbackErr = errors.Join(rollbackErr, err)
	}
	if activation.hasBackup {
		if err := os.Rename(activation.backupDir, activation.activeDir); err != nil && !os.IsNotExist(err) {
			rollbackErr = errors.Join(rollbackErr, err)
		}
	}
	return rollbackErr
}

func DiscardStagedUploadedBackgrounds() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	stageDir := filepath.Join(root, uploadStageDir)
	if err := os.RemoveAll(stageDir); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func DeleteUploadedBackgrounds() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	if err := deleteUploadedFilesIn(filepath.Join(root, uploadDir)); err != nil {
		return err
	}
	return DiscardStagedUploadedBackgrounds()
}

func DeleteRemoteVariants(source string) error {
	source = strings.TrimSpace(source)
	if !isHTTPSource(source) {
		return nil
	}

	root, err := os.Getwd()
	if err != nil {
		return err
	}

	pattern := filepath.Join(root, cacheDir, remoteCacheKey(source)+"-*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return err
	}
	for _, match := range matches {
		if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func DeleteStaleAssets(currentSource string, currentMode string, nextSource string, nextMode string) error {
	currentSource = strings.TrimSpace(currentSource)
	nextSource = strings.TrimSpace(nextSource)
	if currentSource == "" {
		return nil
	}

	if usesUploadedSource(currentSource, currentMode) {
		if usesUploadedSource(nextSource, nextMode) {
			return nil
		}
		return DeleteUploadedBackgrounds()
	}

	if isHTTPSource(currentSource) && currentSource != nextSource {
		return DeleteRemoteVariants(currentSource)
	}

	return nil
}

func FetchUploadedVariant(variant string) ([]byte, string, error) {
	variant = normalizeVariant(variant)
	if err := EnsureUploadedBackgroundVariants(); err != nil {
		return nil, "", err
	}
	root, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	targetDir := filepath.Join(root, uploadDir)

	data, contentType, err := readVariantFile(targetDir, "background-"+variant)
	if err == nil {
		return data, contentType, nil
	}

	if variant == "preview" {
		if data, contentType, fullErr := readVariantFile(targetDir, "background-full"); fullErr == nil {
			return data, contentType, nil
		}
	}

	return readUploadedOriginal(targetDir)
}

func EnsureUploadedBackgroundVariants() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	return ensureUploadedVariantsIn(filepath.Join(root, uploadDir))
}

func FetchRemoteVariant(source string, variant string) ([]byte, string, error) {
	source = strings.TrimSpace(source)
	variant = normalizeVariant(variant)
	if !isHTTPSource(source) {
		return nil, "", fmt.Errorf("unsupported background source")
	}

	if data, contentType, err := readCachedVariant(source, variant); err == nil {
		return data, contentType, nil
	}

	key := remoteCacheKey(source)
	wait := make(chan struct{})
	actual, loaded := inflight.LoadOrStore(key, wait)
	if loaded {
		if ch, ok := actual.(chan struct{}); ok {
			<-ch
		}
		return readCachedVariant(source, variant)
	}

	defer close(wait)
	defer inflight.Delete(key)

	sourceData, sourceType, err := downloadSource(source)
	if err != nil {
		return nil, "", err
	}
	fullData, fullType, previewData, previewType, err := makeVariants(sourceData, sourceType)
	if err != nil {
		return nil, "", err
	}
	if err := writeCachedVariant(source, "full", fullData, fullType); err != nil {
		return nil, "", err
	}
	if err := writeCachedVariant(source, "preview", previewData, previewType); err != nil {
		return nil, "", err
	}
	if variant == "preview" {
		return previewData, previewType, nil
	}
	return fullData, fullType, nil
}

func deleteUploadedFilesIn(targetDir string) error {
	matches, err := filepath.Glob(filepath.Join(targetDir, "background*"))
	if err != nil {
		return err
	}
	for _, match := range matches {
		if err := os.Remove(match); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func hasUploadedFilesIn(targetDir string) (bool, error) {
	matches, err := filepath.Glob(filepath.Join(targetDir, "background*"))
	if err != nil {
		return false, err
	}
	return len(matches) > 0, nil
}

func ensureUploadedVariantsIn(targetDir string) error {
	versionPath := filepath.Join(targetDir, "background.version")
	if versionData, err := os.ReadFile(versionPath); err == nil {
		if strings.TrimSpace(string(versionData)) == uploadedVariantVersion {
			return nil
		}
	}

	sourcePath, err := findUploadedSourceFile(targetDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	sourceData, err := os.ReadFile(sourcePath)
	if err != nil {
		return err
	}

	ext := filepath.Ext(sourcePath)
	if ext == "" {
		ext = ".bin"
	}
	return regenUploadedVariants(targetDir, ext, sourceData)
}

func findUploadedSourceFile(targetDir string) (string, error) {
	matches, err := filepath.Glob(filepath.Join(targetDir, "background*"))
	if err != nil {
		return "", err
	}
	sort.Strings(matches)
	for _, match := range matches {
		base := filepath.Base(match)
		if strings.HasSuffix(base, ".bin") || strings.HasSuffix(base, ".type") || strings.HasSuffix(base, ".version") {
			continue
		}
		return match, nil
	}
	return "", os.ErrNotExist
}

func regenUploadedVariants(targetDir string, ext string, data []byte) error {
	return replaceUploadedVariantDir(targetDir, "background-source"+ext, data)
}

func readUploadedOriginal(targetDir string) ([]byte, string, error) {
	matches, err := filepath.Glob(filepath.Join(targetDir, "background*"))
	if err != nil {
		return nil, "", err
	}
	sort.Strings(matches)
	for _, match := range matches {
		base := filepath.Base(match)
		if strings.HasSuffix(base, ".bin") || strings.HasSuffix(base, ".type") {
			continue
		}
		data, err := os.ReadFile(match)
		if err != nil {
			continue
		}
		return data, http.DetectContentType(data), nil
	}
	return nil, "", os.ErrNotExist
}

func readVariantFile(dir string, prefix string) ([]byte, string, error) {
	dataPath := filepath.Join(dir, prefix+".bin")
	typePath := filepath.Join(dir, prefix+".type")
	data, err := os.ReadFile(dataPath)
	if err != nil {
		return nil, "", err
	}
	contentType, err := os.ReadFile(typePath)
	if err != nil {
		return nil, "", err
	}
	return data, strings.TrimSpace(string(contentType)), nil
}

func normalizeVariant(variant string) string {
	if strings.EqualFold(strings.TrimSpace(variant), "preview") {
		return "preview"
	}
	return "full"
}

func usesUploadedSource(source string, mode string) bool {
	source = strings.TrimSpace(source)
	return strings.EqualFold(strings.TrimSpace(mode), "upload") || strings.HasPrefix(source, "/user-assets/")
}

func isHTTPSource(source string) bool {
	u, err := url.Parse(strings.TrimSpace(source))
	if err != nil || u.Hostname() == "" {
		return false
	}
	return u.Scheme == "http" || u.Scheme == "https"
}

func downloadSource(source string) ([]byte, string, error) {
	if err := validateRemoteSource(source); err != nil {
		return nil, "", err
	}
	req, err := http.NewRequest(http.MethodGet, source, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "SuperFlare background fetcher")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Errorf("unexpected background status: %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, sourceMaxBytes+1))
	if err != nil {
		return nil, "", err
	}
	if int64(len(data)) > sourceMaxBytes {
		return nil, "", fmt.Errorf("background image is too large")
	}
	if len(data) == 0 {
		return nil, "", fmt.Errorf("empty background image")
	}

	contentType := resp.Header.Get("Content-Type")
	if idx := strings.Index(contentType, ";"); idx >= 0 {
		contentType = contentType[:idx]
	}
	contentType = strings.TrimSpace(contentType)
	if contentType == "" {
		contentType = http.DetectContentType(data)
	}
	return data, contentType, nil
}

func validateBackgroundRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return fmt.Errorf("stopped after too many background redirects")
	}
	return validateRemoteSource(req.URL.String())
}

func safeBackgroundTransport() http.RoundTripper {
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	allowedProxyDials := &netutil.ProxyDialAllowList{}
	return &http.Transport{
		Proxy: func(req *http.Request) (*url.URL, error) {
			proxyURL, err := netutil.ProxyFromCurrentEnvironment(req)
			if err == nil && proxyURL != nil {
				allowedProxyDials.Remember(proxyURL)
			}
			return proxyURL, err
		},
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			if allowedProxyDials.Contains(address) {
				return dialer.DialContext(ctx, network, address)
			}
			host, port, err := net.SplitHostPort(address)
			if err != nil {
				host = address
			}
			target, err := resolveSafeDialAddress(ctx, host, port)
			if err != nil {
				return nil, err
			}
			return dialer.DialContext(ctx, network, target)
		},
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second,
	}
}

func validateRemoteSource(source string) error {
	u, err := url.Parse(strings.TrimSpace(source))
	if err != nil || u == nil || u.Hostname() == "" {
		return fmt.Errorf("%w: invalid URL", ErrRemoteSourceNotAllowed)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: unsupported scheme", ErrRemoteSourceNotAllowed)
	}
	if u.User != nil {
		return fmt.Errorf("%w: userinfo is not supported", ErrRemoteSourceNotAllowed)
	}
	return validateRemoteHost(u.Hostname())
}

func validateRemoteHost(host string) error {
	_, err := resolveSafeRemoteIPs(context.Background(), host)
	return err
}

func resolveSafeDialAddress(ctx context.Context, host string, port string) (string, error) {
	if port == "" {
		port = "80"
	}
	ips, err := resolveSafeRemoteIPs(ctx, host)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(ips[0].String(), port), nil
}

func resolveSafeRemoteIPs(ctx context.Context, host string) ([]netip.Addr, error) {
	host = strings.TrimSpace(strings.TrimSuffix(host, "."))
	if host == "" {
		return nil, fmt.Errorf("%w: empty host", ErrRemoteSourceNotAllowed)
	}
	if strings.EqualFold(host, "localhost") {
		return nil, fmt.Errorf("%w: localhost is blocked", ErrRemoteSourceNotAllowed)
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !isPublicRemoteIP(ip) {
			return nil, fmt.Errorf("%w: private or local IP is blocked", ErrRemoteSourceNotAllowed)
		}
		return []netip.Addr{ip}, nil
	}

	ips, err := net.DefaultResolver.LookupNetIP(ctx, "ip", host)
	if err != nil {
		return nil, fmt.Errorf("resolve remote background host failed: %w", err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("resolve remote background host failed: no IP addresses")
	}
	for _, ip := range ips {
		if !isPublicRemoteIP(ip) {
			return nil, fmt.Errorf("%w: host resolves to private or local IP", ErrRemoteSourceNotAllowed)
		}
	}
	return ips, nil
}

func isPublicRemoteIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return false
	}
	if ip.Is4In6() {
		ip = ip.Unmap()
	}
	return ip.IsGlobalUnicast() &&
		!ip.IsPrivate() &&
		!ip.IsLoopback() &&
		!ip.IsLinkLocalUnicast() &&
		!ip.IsLinkLocalMulticast() &&
		!ip.IsMulticast() &&
		!ip.IsUnspecified()
}

func makeVariants(sourceData []byte, sourceType string) ([]byte, string, []byte, string, error) {
	if strings.EqualFold(sourceType, "image/svg+xml") || looksLikeSVG(sourceData) {
		return sourceData, "image/svg+xml", sourceData, "image/svg+xml", nil
	}

	img, _, err := image.Decode(bytes.NewReader(sourceData))
	if err != nil {
		if strings.HasPrefix(sourceType, "image/") {
			return sourceData, sourceType, sourceData, sourceType, nil
		}
		return nil, "", nil, "", err
	}

	fullImage := resizeToLongEdge(img, fullLongEdge)
	previewImage := resizeToLongEdge(img, previewLongEdge)

	fullData, fullType, err := encodeOptimized(fullImage, 82)
	if err != nil {
		return nil, "", nil, "", err
	}
	previewData, previewType, err := encodeOptimized(previewImage, 40)
	if err != nil {
		return nil, "", nil, "", err
	}

	return fullData, fullType, previewData, previewType, nil
}

func makePreviewOnly(sourceData []byte, sourceType string) ([]byte, string, error) {
	if strings.EqualFold(sourceType, "image/svg+xml") || looksLikeSVG(sourceData) {
		return sourceData, "image/svg+xml", nil
	}

	img, _, err := image.Decode(bytes.NewReader(sourceData))
	if err != nil {
		if strings.HasPrefix(sourceType, "image/") {
			return sourceData, sourceType, nil
		}
		return nil, "", err
	}

	previewImage := resizeToLongEdge(img, previewLongEdge)
	return encodeOptimized(previewImage, 40)
}

func buildInlinePreviewDataURL(loader func() ([]byte, string, error)) string {
	data, contentType, err := loader()
	if err != nil {
		return ""
	}
	if len(data) == 0 || len(data) > inlinePreviewMaxBytes || strings.TrimSpace(contentType) == "" {
		return ""
	}
	return "data:" + contentType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func detectAccentColor(loader func() ([]byte, string, error)) string {
	data, contentType, err := loader()
	if err != nil || len(data) == 0 {
		return ""
	}
	return representativeColor(data, contentType)
}

func representativeColor(sourceData []byte, sourceType string) string {
	if strings.EqualFold(sourceType, "image/svg+xml") || looksLikeSVG(sourceData) {
		return ""
	}

	img, _, err := image.Decode(bytes.NewReader(sourceData))
	if err != nil {
		return ""
	}

	type bucket struct {
		score float64
		r     float64
		g     float64
		b     float64
		n     float64
	}

	bounds := img.Bounds()
	stepX := max(bounds.Dx()/48, 1)
	stepY := max(bounds.Dy()/48, 1)
	buckets := map[int]*bucket{}
	var avgR float64
	var avgG float64
	var avgB float64
	var avgN float64

	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			rr, gg, bb, aa := img.At(x, y).RGBA()
			if aa <= 0x4040 {
				continue
			}

			r := float64(rr >> 8)
			g := float64(gg >> 8)
			b := float64(bb >> 8)

			avgR += r
			avgG += g
			avgB += b
			avgN++

			maxChannel := maxFloat(r, g, b)
			minChannel := minFloat(r, g, b)
			saturation := 0.0
			if maxChannel > 0 {
				saturation = (maxChannel - minChannel) / maxChannel
			}
			luminance := (0.2126*r + 0.7152*g + 0.0722*b) / 255
			weight := 1 + saturation*2.8
			if luminance < 0.08 || luminance > 0.94 {
				weight *= 0.35
			} else if luminance < 0.16 || luminance > 0.88 {
				weight *= 0.68
			}

			key := (int(r)/32)<<16 | (int(g)/32)<<8 | (int(b) / 32)
			entry := buckets[key]
			if entry == nil {
				entry = &bucket{}
				buckets[key] = entry
			}
			entry.score += weight
			entry.r += r * weight
			entry.g += g * weight
			entry.b += b * weight
			entry.n += weight
		}
	}

	if len(buckets) == 0 {
		if avgN == 0 {
			return ""
		}
		return rgbHex(int(avgR/avgN), int(avgG/avgN), int(avgB/avgN))
	}

	var selected *bucket
	for _, entry := range buckets {
		if selected == nil || entry.score > selected.score {
			selected = entry
		}
	}
	if selected == nil || selected.n == 0 {
		if avgN == 0 {
			return ""
		}
		return rgbHex(int(avgR/avgN), int(avgG/avgN), int(avgB/avgN))
	}

	return rgbHex(int(selected.r/selected.n), int(selected.g/selected.n), int(selected.b/selected.n))
}

func looksLikeSVG(data []byte) bool {
	head := strings.TrimSpace(strings.ToLower(string(data)))
	return strings.HasPrefix(head, "<svg") || strings.Contains(head, "<svg")
}

func rgbHex(r int, g int, b int) string {
	return fmt.Sprintf("#%02x%02x%02x", clampColor(r), clampColor(g), clampColor(b))
}

func clampColor(value int) int {
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return value
}

func maxFloat(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	result := values[0]
	for _, value := range values[1:] {
		if value > result {
			result = value
		}
	}
	return result
}

func minFloat(values ...float64) float64 {
	if len(values) == 0 {
		return 0
	}
	result := values[0]
	for _, value := range values[1:] {
		if value < result {
			result = value
		}
	}
	return result
}

func resizeToLongEdge(src image.Image, maxLongEdge int) image.Image {
	bounds := src.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()
	if width <= 0 || height <= 0 || maxLongEdge <= 0 {
		return src
	}

	longEdge := width
	if height > longEdge {
		longEdge = height
	}
	if longEdge <= maxLongEdge {
		return src
	}

	ratio := float64(maxLongEdge) / float64(longEdge)
	targetW := int(float64(width) * ratio)
	targetH := int(float64(height) * ratio)
	if targetW < 1 {
		targetW = 1
	}
	if targetH < 1 {
		targetH = 1
	}

	dst := image.NewNRGBA(image.Rect(0, 0, targetW, targetH))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, bounds, xdraw.Over, nil)
	return dst
}

func encodeOptimized(img image.Image, jpegQuality int) ([]byte, string, error) {
	var buf bytes.Buffer
	if hasAlpha(img) {
		encoder := png.Encoder{CompressionLevel: png.DefaultCompression}
		if err := encoder.Encode(&buf, img); err != nil {
			return nil, "", err
		}
		return buf.Bytes(), "image/png", nil
	}

	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return nil, "", err
	}
	return buf.Bytes(), "image/jpeg", nil
}

func hasAlpha(img image.Image) bool {
	type opaqueChecker interface{ Opaque() bool }
	if checker, ok := img.(opaqueChecker); ok {
		return !checker.Opaque()
	}

	bounds := img.Bounds()
	stepX := max(bounds.Dx()/64, 1)
	stepY := max(bounds.Dy()/64, 1)
	for y := bounds.Min.Y; y < bounds.Max.Y; y += stepY {
		for x := bounds.Min.X; x < bounds.Max.X; x += stepX {
			_, _, _, a := img.At(x, y).RGBA()
			if a != 0xffff {
				return true
			}
		}
	}
	return false
}

func cacheExists(source string, variant string) bool {
	_, _, err := readCachedVariant(source, normalizeVariant(variant))
	return err == nil
}

func readCachedVariant(source string, variant string) ([]byte, string, error) {
	root, err := os.Getwd()
	if err != nil {
		return nil, "", err
	}
	prefix := filepath.Join(root, cacheDir, remoteCacheKey(source)+"-"+variant)
	data, err := os.ReadFile(prefix + ".bin")
	if err != nil {
		return nil, "", err
	}
	contentType, err := os.ReadFile(prefix + ".type")
	if err != nil {
		return nil, "", err
	}
	return data, strings.TrimSpace(string(contentType)), nil
}

func writeCachedVariant(source string, variant string, data []byte, contentType string) error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	dir := filepath.Join(root, cacheDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	prefix := filepath.Join(dir, remoteCacheKey(source)+"-"+variant)
	if err := writeFileAtomic(prefix+".bin", data); err != nil {
		return err
	}
	return writeFileAtomic(prefix+".type", []byte(contentType))
}

func replaceUploadedVariantDir(targetDir string, sourceFileName string, data []byte) error {
	parentDir := filepath.Dir(targetDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return err
	}

	stagedDir, err := os.MkdirTemp(parentDir, filepath.Base(targetDir)+".tmp-*")
	if err != nil {
		return err
	}
	if err := writeUploadedVariantFiles(stagedDir, sourceFileName, data); err != nil {
		_ = os.RemoveAll(stagedDir)
		return err
	}
	if err := replaceDirectoryAtomically(targetDir, stagedDir); err != nil {
		_ = os.RemoveAll(stagedDir)
		return err
	}
	return nil
}

func writeUploadedVariantFiles(targetDir string, sourceFileName string, data []byte) error {
	fullData, fullType, previewData, previewType, err := makeVariants(data, "")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(targetDir, sourceFileName), data); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(targetDir, "background-full.bin"), fullData); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(targetDir, "background-full.type"), []byte(fullType)); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(targetDir, "background-preview.bin"), previewData); err != nil {
		return err
	}
	if err := writeFileAtomic(filepath.Join(targetDir, "background-preview.type"), []byte(previewType)); err != nil {
		return err
	}
	return writeFileAtomic(filepath.Join(targetDir, "background.version"), []byte(uploadedVariantVersion))
}

func replaceDirectoryAtomically(targetDir string, replacementDir string) error {
	targetDir = filepath.Clean(targetDir)
	replacementDir = filepath.Clean(replacementDir)
	if targetDir == replacementDir {
		return nil
	}

	backupDir := targetDir + ".backup"
	if err := os.RemoveAll(backupDir); err != nil && !os.IsNotExist(err) {
		return err
	}

	targetExists := false
	if stat, err := os.Stat(targetDir); err == nil {
		if !stat.IsDir() {
			return fmt.Errorf("target path is not a directory: %s", targetDir)
		}
		targetExists = true
		if err := os.Rename(targetDir, backupDir); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}

	if err := os.Rename(replacementDir, targetDir); err != nil {
		if targetExists {
			_ = os.Rename(backupDir, targetDir)
		}
		return err
	}

	if targetExists {
		if err := os.RemoveAll(backupDir); err != nil && !os.IsNotExist(err) {
			return err
		}
	}

	return nil
}

func writeFileAtomic(filePath string, data []byte) error {
	dir := filepath.Dir(filePath)
	temp, err := os.CreateTemp(dir, "."+filepath.Base(filePath)+".tmp-*")
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
	if err := temp.Chmod(backgroundFileMode); err != nil {
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
	if err := os.Rename(tempPath, filePath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func remoteCacheKey(source string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(source)))
	return fmt.Sprintf("%x", sum)
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}
