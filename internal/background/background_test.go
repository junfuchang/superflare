package background

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/junfuchang/superflare/config/model"
)

func TestRepresentativeColorPrefersDominantSaturatedColor(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, 48, 48))
	for y := 0; y < 48; y++ {
		for x := 0; x < 48; x++ {
			img.Set(x, y, color.NRGBA{R: 18, G: 22, B: 28, A: 255})
		}
	}
	for y := 0; y < 48; y++ {
		for x := 0; x < 30; x++ {
			img.Set(x, y, color.NRGBA{R: 12, G: 136, B: 212, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	got := representativeColor(buf.Bytes(), "image/png")
	if got == "" {
		t.Fatal("expected representative color")
	}
	if got[0] != '#' {
		t.Fatalf("expected hex representative color, got %q", got)
	}
}

func TestResolveAssetsForUploadedBackground(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	var buf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.NRGBA{R: 46, G: 122, B: 214, A: 255})
		}
	}
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareUploadedBackground("wallpaper.png", bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("PrepareUploadedBackground: %v", err)
	}

	assets := ResolveAssets(model.Application{
		BackgroundImage:     UploadedFullPath,
		BackgroundImageMode: "upload",
	})
	if !assets.Enabled {
		t.Fatal("expected uploaded background assets to be enabled")
	}
	if assets.PreviewURL != UploadedPreviewPath {
		t.Fatalf("expected preview path %q, got %q", UploadedPreviewPath, assets.PreviewURL)
	}
	if assets.FullURL != UploadedFullPath {
		t.Fatalf("expected full path %q, got %q", UploadedFullPath, assets.FullURL)
	}
	if assets.PreviewDataURL == "" {
		t.Fatal("expected uploaded background preview data url to be populated")
	}
	if assets.AccentColor == "" {
		t.Fatal("expected uploaded background accent color to be detected")
	}
}

func TestEnsureUploadedBackgroundVariantsRegeneratesLegacyPreview(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	targetDir := filepath.Join(tmpDir, uploadDir)
	if err := os.MkdirAll(targetDir, 0755); err != nil {
		t.Fatal(err)
	}

	img := image.NewNRGBA(image.Rect(0, 0, 3840, 2880))
	for y := 0; y < 2880; y++ {
		for x := 0; x < 3840; x += 32 {
			img.Set(x, y, color.NRGBA{R: uint8((x / 32) % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	var source bytes.Buffer
	if err := jpeg.Encode(&source, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "background-source.jpg"), source.Bytes(), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "background-preview.bin"), []byte("legacy"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "background-preview.type"), []byte("image/jpeg"), 0644); err != nil {
		t.Fatal(err)
	}

	if err := EnsureUploadedBackgroundVariants(); err != nil {
		t.Fatalf("EnsureUploadedBackgroundVariants: %v", err)
	}

	previewData, _, err := FetchUploadedVariant("preview")
	if err != nil {
		t.Fatalf("FetchUploadedVariant preview: %v", err)
	}
	previewImage, _, err := image.Decode(bytes.NewReader(previewData))
	if err != nil {
		t.Fatalf("decode preview image: %v", err)
	}
	if previewImage.Bounds().Dx() != 320 {
		t.Fatalf("expected regenerated preview width 320, got %d", previewImage.Bounds().Dx())
	}
	if matches, err := filepath.Glob(filepath.Join(tmpDir, "var", "uploads.tmp-*")); err != nil {
		t.Fatal(err)
	} else if len(matches) != 0 {
		t.Fatalf("expected no leftover staged upload dirs, got %v", matches)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, uploadDir+".backup")); !os.IsNotExist(err) {
		t.Fatalf("expected no leftover upload backup dir, got err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "background-source.jpg")); err != nil {
		t.Fatalf("expected regenerated upload dir to preserve source file, got err=%v", err)
	}
}

func TestResolveAssetsForRemoteBackground(t *testing.T) {
	assets := ResolveAssets(model.Application{
		BackgroundImage:     "https://example.com/background.jpg",
		BackgroundImageMode: "url",
	})
	if !assets.Enabled {
		t.Fatal("expected remote background assets to be enabled")
	}
	if assets.PreviewURL == assets.FullURL {
		t.Fatal("expected preview and full URLs to differ for remote background")
	}
	if assets.PreviewDataURL != "" {
		t.Fatal("remote background resolve should not synchronously inline uncached preview data")
	}
	if assets.AccentColor != "" {
		t.Fatal("remote background resolve should not synchronously detect accent color on cache miss")
	}
}

func TestResolveAssetsForRemoteBackgroundUsesCachedPreviewWithoutFetching(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	source := "https://example.com/background.jpg"
	preview := solidPNGBuffer(color.NRGBA{R: 60, G: 120, B: 180, A: 255})
	if err := writeCachedVariant(source, "preview", preview.Bytes(), "image/png"); err != nil {
		t.Fatalf("write cached preview: %v", err)
	}

	assets := ResolveAssets(model.Application{
		BackgroundImage:     source,
		BackgroundImageMode: "url",
	})
	if assets.PreviewDataURL == "" {
		t.Fatal("expected cached remote preview to be inlined")
	}
	if assets.AccentColor == "" {
		t.Fatal("expected cached remote preview to provide accent color")
	}
}

func TestResolveAssetsForRemoteBackgroundDoesNotBlockOnSlowFetch(t *testing.T) {
	origClient := httpClient
	httpClient = &http.Client{
		Timeout:   time.Second,
		Transport: blockingRoundTripper{done: make(chan struct{})},
	}
	defer func() { httpClient = origClient }()

	started := time.Now()
	assets := ResolveAssets(model.Application{
		BackgroundImage:     "https://example.com/slow-background.jpg",
		BackgroundImageMode: "url",
	})
	if !assets.Enabled {
		t.Fatal("expected remote background assets")
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("ResolveAssets blocked on remote fetch for %s", elapsed)
	}
}

func TestFetchRemoteVariantRejectsPrivateAndLocalTargets(t *testing.T) {
	tests := []string{
		"http://127.0.0.1/background.jpg",
		"http://localhost/background.jpg",
		"http://[::1]/background.jpg",
		"http://169.254.169.254/latest/meta-data",
		"http://192.168.0.10/background.jpg",
	}
	for _, source := range tests {
		t.Run(source, func(t *testing.T) {
			_, _, err := FetchRemoteVariant(source, "preview")
			if err == nil {
				t.Fatal("expected private or local source to be rejected")
			}
			if !strings.Contains(err.Error(), "not allowed") {
				t.Fatalf("expected not allowed error, got %v", err)
			}
		})
	}
}

func TestFetchRemoteVariantRejectsRedirectToPrivateTarget(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/background.jpg", nil)
	err := validateBackgroundRedirect(req, []*http.Request{
		httptest.NewRequest(http.MethodGet, "https://example.com/background.jpg", nil),
	})
	if err == nil {
		t.Fatal("expected redirect to private target to be rejected")
	}
	if !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected not allowed error, got %v", err)
	}
}

func TestSafeBackgroundTransportUsesEnvironmentProxy(t *testing.T) {
	t.Setenv("HTTP_PROXY", "http://127.0.0.1:9")
	t.Setenv("HTTPS_PROXY", "http://127.0.0.1:9")

	transport, ok := safeBackgroundTransport().(*http.Transport)
	if !ok {
		t.Fatalf("safeBackgroundTransport should return *http.Transport, got %T", safeBackgroundTransport())
	}
	if transport.Proxy == nil {
		t.Fatal("safe background transport should use environment proxy settings")
	}
}

func TestFetchRemoteVariantCanUseLocalEnvironmentProxy(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	var source bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 8, 8))
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			img.Set(x, y, color.NRGBA{R: 60, G: 120, B: 180, A: 255})
		}
	}
	if err := png.Encode(&source, img); err != nil {
		t.Fatal(err)
	}

	var sawTargetRequest bool
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL != nil && r.URL.Host == "93.184.216.56" && r.URL.Path == "/background.png" {
			sawTargetRequest = true
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(source.Bytes())
			return
		}
		http.Error(w, "unexpected proxy request", http.StatusBadGateway)
	}))
	defer proxy.Close()

	t.Setenv("HTTP_PROXY", proxy.URL)
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")

	origClient := httpClient
	httpClient = &http.Client{
		Timeout:       2 * time.Second,
		Transport:     safeBackgroundTransport(),
		CheckRedirect: validateBackgroundRedirect,
	}
	defer func() { httpClient = origClient }()

	data, contentType, err := FetchRemoteVariant("http://93.184.216.56/background.png", "preview")
	if err != nil {
		t.Fatalf("FetchRemoteVariant with local environment proxy: %v", err)
	}
	if contentType != "image/jpeg" && contentType != "image/png" {
		t.Fatalf("proxied background content type = %q", contentType)
	}
	if len(data) == 0 {
		t.Fatal("proxied background should return data")
	}
	if !sawTargetRequest {
		t.Fatal("expected background fetch to use the configured environment proxy")
	}
}

func TestResolveSafeDialAddressReturnsVerifiedIPAddress(t *testing.T) {
	got, err := resolveSafeDialAddress(context.Background(), "93.184.216.34", "443")
	if err != nil {
		t.Fatalf("resolveSafeDialAddress: %v", err)
	}
	if got != "93.184.216.34:443" {
		t.Fatalf("expected dial address to use verified IP, got %q", got)
	}
}

func TestPrepareUploadedBackgroundCreatesVariants(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	img := image.NewNRGBA(image.Rect(0, 0, 320, 180))
	for y := 0; y < 180; y++ {
		for x := 0; x < 320; x++ {
			img.Set(x, y, color.NRGBA{R: uint8(x % 255), G: uint8(y % 255), B: 120, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}

	saved, err := PrepareUploadedBackground("wallpaper.png", bytes.NewReader(buf.Bytes()))
	if err != nil {
		t.Fatalf("PrepareUploadedBackground: %v", err)
	}
	if saved != UploadedFullPath {
		t.Fatalf("expected saved path %q, got %q", UploadedFullPath, saved)
	}

	if _, _, err := FetchUploadedVariant("preview"); err != nil {
		t.Fatalf("FetchUploadedVariant preview: %v", err)
	}
	if _, _, err := FetchUploadedVariant("full"); err != nil {
		t.Fatalf("FetchUploadedVariant full: %v", err)
	}
}

func TestPrepareUploadedBackgroundStageDoesNotReplaceActiveBeforePromote(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	active := solidPNGBuffer(color.NRGBA{R: 32, G: 96, B: 200, A: 254})
	if _, err := PrepareUploadedBackground("active.png", bytes.NewReader(active.Bytes())); err != nil {
		t.Fatalf("PrepareUploadedBackground active: %v", err)
	}

	beforeData, _, err := FetchUploadedVariant("full")
	if err != nil {
		t.Fatalf("FetchUploadedVariant before stage: %v", err)
	}
	if got := firstPixel(beforeData, t); !approxColor(got, color.NRGBA{R: 32, G: 96, B: 200, A: 254}, 1) {
		t.Fatalf("unexpected active color before stage: %#v", got)
	}

	staged := solidPNGBuffer(color.NRGBA{R: 210, G: 48, B: 64, A: 254})
	if _, err := PrepareUploadedBackgroundStage("staged.png", bytes.NewReader(staged.Bytes())); err != nil {
		t.Fatalf("PrepareUploadedBackgroundStage: %v", err)
	}
	if matches, err := filepath.Glob(filepath.Join(tmpDir, "var", "uploads-stage.tmp-*")); err != nil {
		t.Fatal(err)
	} else if len(matches) != 0 {
		t.Fatalf("expected no leftover staged upload temp dirs, got %v", matches)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, uploadStageDir+".backup")); !os.IsNotExist(err) {
		t.Fatalf("expected no leftover upload stage backup dir, got err=%v", err)
	}

	duringData, _, err := FetchUploadedVariant("full")
	if err != nil {
		t.Fatalf("FetchUploadedVariant during stage: %v", err)
	}
	if got := firstPixel(duringData, t); !approxColor(got, color.NRGBA{R: 32, G: 96, B: 200, A: 254}, 1) {
		t.Fatalf("staged upload should not replace active before promote, got %#v", got)
	}

	if err := PromoteStagedUploadedBackground(); err != nil {
		t.Fatalf("PromoteStagedUploadedBackground: %v", err)
	}

	afterData, _, err := FetchUploadedVariant("full")
	if err != nil {
		t.Fatalf("FetchUploadedVariant after promote: %v", err)
	}
	if got := firstPixel(afterData, t); !approxColor(got, color.NRGBA{R: 210, G: 48, B: 64, A: 254}, 1) {
		t.Fatalf("expected promoted staged upload color, got %#v", got)
	}
}

func TestBeginStagedUploadedBackgroundActivationRollbackRestoresActive(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	active := solidPNGBuffer(color.NRGBA{R: 24, G: 84, B: 164, A: 254})
	if _, err := PrepareUploadedBackground("active.png", bytes.NewReader(active.Bytes())); err != nil {
		t.Fatalf("PrepareUploadedBackground active: %v", err)
	}
	staged := solidPNGBuffer(color.NRGBA{R: 196, G: 52, B: 72, A: 254})
	if _, err := PrepareUploadedBackgroundStage("staged.png", bytes.NewReader(staged.Bytes())); err != nil {
		t.Fatalf("PrepareUploadedBackgroundStage: %v", err)
	}

	activation, err := BeginStagedUploadedBackgroundActivation()
	if err != nil {
		t.Fatalf("BeginStagedUploadedBackgroundActivation: %v", err)
	}

	duringData, _, err := FetchUploadedVariant("full")
	if err != nil {
		t.Fatalf("FetchUploadedVariant during activation: %v", err)
	}
	if got := firstPixel(duringData, t); !approxColor(got, color.NRGBA{R: 196, G: 52, B: 72, A: 254}, 1) {
		t.Fatalf("expected staged upload to be active during activation, got %#v", got)
	}

	if err := activation.Rollback(); err != nil {
		t.Fatalf("Rollback activation: %v", err)
	}

	afterData, _, err := FetchUploadedVariant("full")
	if err != nil {
		t.Fatalf("FetchUploadedVariant after rollback: %v", err)
	}
	if got := firstPixel(afterData, t); !approxColor(got, color.NRGBA{R: 24, G: 84, B: 164, A: 254}, 1) {
		t.Fatalf("expected rollback to restore original active upload, got %#v", got)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, uploadStageDir)); err == nil {
		t.Fatalf("expected staging directory to be consumed during activation")
	}
	if _, err := os.Stat(filepath.Join(tmpDir, uploadDir+"-backup")); !os.IsNotExist(err) {
		t.Fatalf("expected backup directory to be removed after rollback, got err=%v", err)
	}
}

func TestDeleteUploadedBackgroundsRemovesSourceAndVariants(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	var buf bytes.Buffer
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	if _, err := PrepareUploadedBackground("wallpaper.png", bytes.NewReader(buf.Bytes())); err != nil {
		t.Fatalf("PrepareUploadedBackground: %v", err)
	}

	if err := DeleteUploadedBackgrounds(); err != nil {
		t.Fatalf("DeleteUploadedBackgrounds: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(tmpDir, uploadDir, "background*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected uploaded background files to be deleted, got %v", matches)
	}
}

func TestDeleteUploadedBackgroundsAlsoRemovesStagedFiles(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	active := solidPNGBuffer(color.NRGBA{R: 50, G: 80, B: 110, A: 254})
	if _, err := PrepareUploadedBackground("active.png", bytes.NewReader(active.Bytes())); err != nil {
		t.Fatalf("PrepareUploadedBackground: %v", err)
	}
	staged := solidPNGBuffer(color.NRGBA{R: 180, G: 40, B: 90, A: 254})
	if _, err := PrepareUploadedBackgroundStage("staged.png", bytes.NewReader(staged.Bytes())); err != nil {
		t.Fatalf("PrepareUploadedBackgroundStage: %v", err)
	}

	if err := DeleteUploadedBackgrounds(); err != nil {
		t.Fatalf("DeleteUploadedBackgrounds: %v", err)
	}

	activeMatches, err := filepath.Glob(filepath.Join(tmpDir, uploadDir, "background*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(activeMatches) != 0 {
		t.Fatalf("expected active uploaded background files to be deleted, got %v", activeMatches)
	}

	stageMatches, err := filepath.Glob(filepath.Join(tmpDir, uploadStageDir, "background*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(stageMatches) != 0 {
		t.Fatalf("expected staged uploaded background files to be deleted, got %v", stageMatches)
	}
}

func TestDeleteRemoteVariantsRemovesCachedFiles(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	source := "https://example.com/background.jpg"
	if err := writeCachedVariant(source, "preview", []byte("preview"), "image/jpeg"); err != nil {
		t.Fatalf("writeCachedVariant preview: %v", err)
	}
	if err := writeCachedVariant(source, "full", []byte("full"), "image/jpeg"); err != nil {
		t.Fatalf("writeCachedVariant full: %v", err)
	}

	if err := DeleteRemoteVariants(source); err != nil {
		t.Fatalf("DeleteRemoteVariants: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(tmpDir, cacheDir, remoteCacheKey(source)+"-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected remote background cache files to be deleted, got %v", matches)
	}
}

func TestDeleteStaleAssetsRemovesOldRemoteCacheOnSourceChange(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	oldSource := "https://example.com/background-a.jpg"
	newSource := "https://example.com/background-b.jpg"
	if err := writeCachedVariant(oldSource, "preview", []byte("preview"), "image/jpeg"); err != nil {
		t.Fatalf("writeCachedVariant preview: %v", err)
	}
	if err := writeCachedVariant(oldSource, "full", []byte("full"), "image/jpeg"); err != nil {
		t.Fatalf("writeCachedVariant full: %v", err)
	}

	if err := DeleteStaleAssets(oldSource, "url", newSource, "url"); err != nil {
		t.Fatalf("DeleteStaleAssets: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(tmpDir, cacheDir, remoteCacheKey(oldSource)+"-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected old remote cache files to be deleted, got %v", matches)
	}
}

func TestWriteCachedVariantOverwritesWithoutTempLeak(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(origWd)

	source := "https://example.com/background.jpg"
	requireWriteCachedVariant(t, source, "preview", []byte("old"), "image/jpeg")
	requireWriteCachedVariant(t, source, "preview", []byte("new"), "image/png")

	data, contentType, err := readCachedVariant(source, "preview")
	if err != nil {
		t.Fatalf("readCachedVariant: %v", err)
	}
	if string(data) != "new" || contentType != "image/png" {
		t.Fatalf("unexpected cached variant content=%q type=%q", string(data), contentType)
	}

	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	matches, err := filepath.Glob(filepath.Join(root, cacheDir, ".*.tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 0 {
		t.Fatalf("expected no leftover temp cache files, got %v", matches)
	}
}

func solidPNGBuffer(fill color.NRGBA) bytes.Buffer {
	img := image.NewNRGBA(image.Rect(0, 0, 64, 64))
	for y := 0; y < 64; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, fill)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return buf
}

func firstPixel(data []byte, t *testing.T) color.NRGBA {
	t.Helper()
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode image: %v", err)
	}
	r, g, b, a := img.At(0, 0).RGBA()
	return color.NRGBA{R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8)}
}

func approxColor(got color.NRGBA, want color.NRGBA, delta uint8) bool {
	return withinDelta(got.R, want.R, delta) &&
		withinDelta(got.G, want.G, delta) &&
		withinDelta(got.B, want.B, delta) &&
		withinDelta(got.A, want.A, delta)
}

func withinDelta(got uint8, want uint8, delta uint8) bool {
	if got > want {
		return got-want <= delta
	}
	return want-got <= delta
}

func requireWriteCachedVariant(t *testing.T, source string, variant string, data []byte, contentType string) {
	t.Helper()
	if err := writeCachedVariant(source, variant, data, contentType); err != nil {
		t.Fatalf("writeCachedVariant(%s,%s): %v", source, variant, err)
	}
}

type blockingRoundTripper struct {
	done chan struct{}
}

func (b blockingRoundTripper) RoundTrip(*http.Request) (*http.Response, error) {
	<-b.done
	return nil, http.ErrHandlerTimeout
}
