package background

import (
	"bytes"
	"encoding/base64"
	"crypto/sha256"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
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
)

const (
	UploadedFullPath          = "/user-assets/background"
	UploadedPreviewPath       = "/user-assets/background-preview"
	RemoteAssetPath           = "/assets/background-image"
	uploadDir                 = "var/uploads"
	cacheDir                  = "var/cache/backgrounds"
	previewLongEdge           = 320
	fullLongEdge              = 2200
	sourceMaxBytes      int64 = 32 << 20
	uploadedVariantVersion    = "2"
)

const InlineLoaderScript = `(function(){var bg=document.querySelector('.page-background');if(!bg){return;}var preview=bg.querySelector('.page-background-preview');var full=bg.querySelector('.page-background-full');if(!full){return;}function usePreviewLayer(){if(bg.classList.contains('has-preview')){return;}bg.classList.add('has-preview');if(document.body){document.body.classList.add('has-preview-background');}}function settleBody(){if(document.body){document.body.classList.add('has-loaded-background');}}function afterReveal(){if(typeof window.requestAnimationFrame==='function'){window.requestAnimationFrame(function(){window.requestAnimationFrame(settleBody);});return;}settleBody();}function startReveal(){if(bg.classList.contains('is-loaded')){return;}usePreviewLayer();bg.classList.add('is-loaded');afterReveal();}function reveal(){if(typeof full.decode==='function'){full.decode().catch(function(){}).then(startReveal);return;}startReveal();}if(preview){if(preview.complete&&preview.naturalWidth>0){usePreviewLayer();}else{preview.addEventListener('load',usePreviewLayer,{once:true});preview.addEventListener('error',function(){if(document.body){document.body.classList.add('has-preview-background');}},{once:true});}}if(full.complete&&full.naturalWidth>0){reveal();return;}full.addEventListener('load',reveal,{once:true});full.addEventListener('error',function(){bg.classList.add('is-failed');},{once:true});}());`

type Assets struct {
	Enabled        bool
	PreviewURL     string
	PreviewDataURL string
	FullURL        string
	AccentColor    string
}

var (
	httpClient = &http.Client{Timeout: 15 * time.Second}
	inflight   sync.Map
)

const inlinePreviewMaxBytes = 64 << 10

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
		variantLoader := func() func() ([]byte, string, error) {
			var loaded bool
			var cachedData []byte
			var cachedType string
			var cachedErr error
			return func() ([]byte, string, error) {
				if loaded {
					return cachedData, cachedType, cachedErr
				}
				cachedData, cachedType, cachedErr = FetchRemoteVariant(source, "preview")
				loaded = true
				return cachedData, cachedType, cachedErr
			}
		}()
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
	targetDir := filepath.Join(root, uploadDir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		return "", err
	}
	if err := deleteUploadedFilesIn(targetDir); err != nil {
		return "", err
	}

	return writeUploadedVariants(targetDir, ext, data)
}

func writeUploadedVariants(targetDir string, ext string, data []byte) (string, error) {
	fullData, fullType, previewData, previewType, err := makeVariants(data, "")
	if err != nil {
		return "", err
	}

	if err := os.WriteFile(filepath.Join(targetDir, "background-source"+ext), data, 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "background-full.bin"), fullData, 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "background-full.type"), []byte(fullType), 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "background-preview.bin"), previewData, 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "background-preview.type"), []byte(previewType), 0644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "background.version"), []byte(uploadedVariantVersion), 0644); err != nil {
		return "", err
	}

	return UploadedFullPath, nil
}

func DeleteUploadedBackgrounds() error {
	root, err := os.Getwd()
	if err != nil {
		return err
	}
	return deleteUploadedFilesIn(filepath.Join(root, uploadDir))
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
	fullData, fullType, previewData, previewType, err := makeVariants(data, "")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(targetDir, "background-full.bin"), fullData, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "background-full.type"), []byte(fullType), 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "background-preview.bin"), previewData, 0644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(targetDir, "background-preview.type"), []byte(previewType), 0644); err != nil {
		return err
	}
	if _, err := os.Stat(filepath.Join(targetDir, "background-source"+ext)); os.IsNotExist(err) {
		if err := os.WriteFile(filepath.Join(targetDir, "background-source"+ext), data, 0644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(targetDir, "background.version"), []byte(uploadedVariantVersion), 0644)
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
	if err := os.WriteFile(prefix+".bin", data, 0644); err != nil {
		return err
	}
	return os.WriteFile(prefix+".type", []byte(contentType), 0644)
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
