package background

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"testing"

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
