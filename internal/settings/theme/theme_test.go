package theme

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/internal/background"
)

func TestUpdateThemesReturnsServerErrorWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.Mkdir("config.yml", 0755); err != nil {
		t.Fatalf("mkdir config.yml: %v", err)
	}

	form := url.Values{}
	form.Set("theme", "blackboard")
	req := httptest.NewRequest(http.MethodPost, "/settings/theme", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestUpdateThemesReturnsStyledBadRequestWhenFormDataMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/settings/theme", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "missing form data") {
		t.Fatalf("expected missing form data detail, got %s", rec.Body.String())
	}
}

func TestUpdateThemesReturnsStyledBadRequestWhenBackgroundUploadParsingFails(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/settings/theme", strings.NewReader("--broken"))
	req.Header.Set(echo.HeaderContentType, "multipart/form-data; boundary=broken")
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = themeTestRenderer{}
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "missing form data") {
		t.Fatalf("expected missing form data detail, got %s", rec.Body.String())
	}
}

func TestPageThemeReturnsStyledErrorWhenConfigBroken(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/theme", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := pageTheme(c); err != nil {
		t.Fatalf("pageTheme: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestPageThemeReturnsStyledErrorWhenConfigValuesInvalid(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: custom\nCustomThemeBackground: bad-color\nCustomThemePrimary: rgba(255, 253, 234, 1)\nCustomThemeAccent: rgba(92, 92, 92, 1)\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/theme", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := pageTheme(c); err != nil {
		t.Fatalf("pageTheme: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestUpdateThemesReturnsStyledBadRequestWhenThemeInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("theme", "unknown-theme")
	req := httptest.NewRequest(http.MethodPost, "/settings/theme", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid theme value") {
		t.Fatalf("expected invalid theme detail, got %s", rec.Body.String())
	}
}

func TestUpdateThemesReturnsStyledBadRequestWhenThemeMissing(t *testing.T) {
	form := url.Values{}
	req := httptest.NewRequest(http.MethodPost, "/settings/theme", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "missing theme value") {
		t.Fatalf("expected missing theme detail, got %s", rec.Body.String())
	}
}

func TestUpdateThemesReturnsStyledBadRequestWhenCustomColorInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("theme", "custom")
	form.Set("custom-theme-background", "bad-color")
	form.Set("custom-theme-primary", "rgba(255, 253, 234, 1)")
	form.Set("custom-theme-accent", "rgba(92, 92, 92, 1)")
	req := httptest.NewRequest(http.MethodPost, "/settings/theme", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid custom-theme-background value") {
		t.Fatalf("expected invalid color detail, got %s", rec.Body.String())
	}
}

func TestUpdateThemesReturnsStyledBadRequestWhenGlassEffectInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("theme", "custom")
	form.Set("custom-theme-background", "rgba(26, 26, 26, 1)")
	form.Set("custom-theme-primary", "rgba(255, 253, 234, 1)")
	form.Set("custom-theme-accent", "rgba(92, 92, 92, 1)")
	form.Set("glass-effect", "mystery")
	req := httptest.NewRequest(http.MethodPost, "/settings/theme", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid glass-effect value") {
		t.Fatalf("expected invalid glass effect detail, got %s", rec.Body.String())
	}
}

func TestUpdateThemesReturnsStyledBadRequestWhenNumberInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("theme", "custom")
	form.Set("custom-theme-background", "rgba(26, 26, 26, 1)")
	form.Set("custom-theme-primary", "rgba(255, 253, 234, 1)")
	form.Set("custom-theme-accent", "rgba(92, 92, 92, 1)")
	form.Set("background-blur", "abc")
	req := httptest.NewRequest(http.MethodPost, "/settings/theme", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid background-blur value") {
		t.Fatalf("expected invalid numeric detail, got %s", rec.Body.String())
	}
}

func TestUpdateThemesReturnsStyledErrorWhenBackgroundActivationFails(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: custom\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	originalBegin := beginUploadedBackgroundActivation
	originalDiscard := discardUploadedBackgroundStage
	beginUploadedBackgroundActivation = func() (*background.StagedUploadedBackgroundActivation, error) {
		return nil, errors.New("simulated activation failure")
	}
	discardUploadedBackgroundStage = func() error { return nil }
	defer func() {
		beginUploadedBackgroundActivation = originalBegin
		discardUploadedBackgroundStage = originalDiscard
	}()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("action", "custom-theme"); err != nil {
		t.Fatalf("write action: %v", err)
	}
	if err := writer.WriteField("theme", "custom"); err != nil {
		t.Fatalf("write theme: %v", err)
	}
	if err := writer.WriteField("custom-theme-background", "rgba(26, 26, 26, 1)"); err != nil {
		t.Fatalf("write background: %v", err)
	}
	if err := writer.WriteField("custom-theme-primary", "rgba(255, 253, 234, 1)"); err != nil {
		t.Fatalf("write primary: %v", err)
	}
	if err := writer.WriteField("custom-theme-accent", "rgba(92, 92, 92, 1)"); err != nil {
		t.Fatalf("write accent: %v", err)
	}

	part, err := writer.CreateFormFile("background-file", "wallpaper.png")
	if err != nil {
		t.Fatalf("create background file: %v", err)
	}
	if _, err := part.Write(testPNGBytes(t)); err != nil {
		t.Fatalf("write background file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/theme", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = themeTestRenderer{}
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "请求异常") && !strings.Contains(rec.Body.String(), "页面暂时不可用") {
		t.Fatalf("expected error page content, got %s", rec.Body.String())
	}
}

func TestUpdateThemesPreservesExistingRemoteBackgroundWhenFieldOmitted(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	config := "Title: SuperFlare\nLocale: zh\nTheme: custom\nThemeBase: blackboard\nCustomThemeBackground: \"\"\nCustomThemePrimary: \"\"\nCustomThemeAccent: \"\"\nBackgroundImage: https://example.com/background.jpg\nBackgroundImageMode: url\nBackgroundOpacity: 100\nGlassEffect: none\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(config), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("action", "custom-theme"); err != nil {
		t.Fatalf("write action: %v", err)
	}
	if err := writer.WriteField("theme", "custom"); err != nil {
		t.Fatalf("write theme: %v", err)
	}
	if err := writer.WriteField("custom-theme-background", ""); err != nil {
		t.Fatalf("write background: %v", err)
	}
	if err := writer.WriteField("custom-theme-primary", ""); err != nil {
		t.Fatalf("write primary: %v", err)
	}
	if err := writer.WriteField("custom-theme-accent", ""); err != nil {
		t.Fatalf("write accent: %v", err)
	}
	if err := writer.WriteField("background-opacity", "100"); err != nil {
		t.Fatalf("write background-opacity: %v", err)
	}
	if err := writer.WriteField("glass-effect", "none"); err != nil {
		t.Fatalf("write glass-effect: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/theme", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = themeTestRenderer{}
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	raw, err := os.ReadFile(filepath.Join(dir, "config.yml"))
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, "BackgroundImage: https://example.com/background.jpg") {
		t.Fatalf("expected background image to be preserved, got %s", content)
	}
	if !strings.Contains(content, "ThemeBase: blackboard") {
		t.Fatalf("expected theme base to be preserved, got %s", content)
	}
}

func TestUpdateThemesClearsExistingRemoteBackgroundWhenFieldExplicitlyBlank(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	config := "Title: SuperFlare\nLocale: zh\nTheme: custom\nThemeBase: blackboard\nCustomThemeBackground: \"\"\nCustomThemePrimary: \"\"\nCustomThemeAccent: \"\"\nBackgroundImage: https://example.com/background.jpg\nBackgroundImageMode: url\nBackgroundOpacity: 100\nGlassEffect: none\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(config), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("action", "custom-theme"); err != nil {
		t.Fatalf("write action: %v", err)
	}
	if err := writer.WriteField("theme", "custom"); err != nil {
		t.Fatalf("write theme: %v", err)
	}
	if err := writer.WriteField("custom-theme-background", ""); err != nil {
		t.Fatalf("write background: %v", err)
	}
	if err := writer.WriteField("custom-theme-primary", ""); err != nil {
		t.Fatalf("write primary: %v", err)
	}
	if err := writer.WriteField("custom-theme-accent", ""); err != nil {
		t.Fatalf("write accent: %v", err)
	}
	if err := writer.WriteField("background-image", ""); err != nil {
		t.Fatalf("write background-image: %v", err)
	}
	if err := writer.WriteField("background-opacity", "100"); err != nil {
		t.Fatalf("write background-opacity: %v", err)
	}
	if err := writer.WriteField("glass-effect", "none"); err != nil {
		t.Fatalf("write glass-effect: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/theme", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = themeTestRenderer{}
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	raw, err := os.ReadFile(filepath.Join(dir, "config.yml"))
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	content := string(raw)
	if strings.Contains(content, "BackgroundImage: https://example.com/background.jpg") {
		t.Fatalf("expected background image to be cleared, got %s", content)
	}
}

func TestUpdateThemesClearsCustomThemeFieldsWhenSubmittingBlankCustomColors(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	config := "Title: SuperFlare\nLocale: zh\nTheme: onedark\nThemeBase: onedark\nCustomThemeBackground: rgba(26, 26, 26, 1)\nCustomThemePrimary: rgba(118, 255, 71, 1)\nCustomThemeAccent: rgba(110, 255, 204, 1)\nBackgroundOpacity: 100\nGlassEffect: none\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(config), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	if err := writer.WriteField("action", "custom-theme"); err != nil {
		t.Fatalf("write action: %v", err)
	}
	if err := writer.WriteField("theme", "custom"); err != nil {
		t.Fatalf("write theme: %v", err)
	}
	if err := writer.WriteField("custom-theme-background", ""); err != nil {
		t.Fatalf("write background: %v", err)
	}
	if err := writer.WriteField("custom-theme-primary", ""); err != nil {
		t.Fatalf("write primary: %v", err)
	}
	if err := writer.WriteField("custom-theme-accent", ""); err != nil {
		t.Fatalf("write accent: %v", err)
	}
	if err := writer.WriteField("background-opacity", "100"); err != nil {
		t.Fatalf("write background-opacity: %v", err)
	}
	if err := writer.WriteField("glass-effect", "none"); err != nil {
		t.Fatalf("write glass-effect: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/settings/theme", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = themeTestRenderer{}
	c := e.NewContext(req, rec)

	if err := updateThemes(c); err != nil {
		t.Fatalf("updateThemes: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	raw, err := os.ReadFile(filepath.Join(dir, "config.yml"))
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	content := string(raw)
	if !strings.Contains(content, "Theme: custom") {
		t.Fatalf("expected theme to become custom, got %s", content)
	}
	if strings.Contains(content, "CustomThemeBackground: rgba(26, 26, 26, 1)") {
		t.Fatalf("expected custom background to be cleared, got %s", content)
	}
	if strings.Contains(content, "CustomThemePrimary: rgba(118, 255, 71, 1)") {
		t.Fatalf("expected custom primary to be cleared, got %s", content)
	}
	if strings.Contains(content, "CustomThemeAccent: rgba(110, 255, 204, 1)") {
		t.Fatalf("expected custom accent to be cleared, got %s", content)
	}
}

func testPNGBytes(t *testing.T) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 32, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			img.Set(x, y, color.NRGBA{R: 42, G: 96, B: 180, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

type themeTestRenderer struct{}

func (themeTestRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	return nil
}
