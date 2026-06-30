package appearance

import (
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	settingsroot "github.com/junfuchang/superflare/internal/settings"
	"github.com/labstack/echo/v5"
)

func TestUpdateAppearanceOptionsReturnsServerErrorWhenSaveFails(t *testing.T) {
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
	form.Set("title", "SuperFlare")
	form.Set("icon-mode", "FILLING")
	form.Set("locale", "zh")
	req := httptest.NewRequest(http.MethodPost, "/settings/appearance", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateAppearanceOptions(c); err != nil {
		t.Fatalf("updateAppearanceOptions: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestUpdateAppearanceOptionsReturnsStyledBadRequestWhenFormDataMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/settings/appearance", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateAppearanceOptions(c); err != nil {
		t.Fatalf("updateAppearanceOptions: %v", err)
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

func TestPageAppearanceReturnsStyledErrorWhenConfigBroken(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/appearance", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := pageAppearance(c); err != nil {
		t.Fatalf("pageAppearance: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestUpdateAppearanceOptionsReturnsStyledBadRequestWhenColorInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("icon-mode", "FILLING")
	form.Set("locale", "zh")
	form.Set("bookmark-category-color", "not-a-color")
	req := httptest.NewRequest(http.MethodPost, "/settings/appearance", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateAppearanceOptions(c); err != nil {
		t.Fatalf("updateAppearanceOptions: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid color value") {
		t.Fatalf("expected invalid color detail, got %s", rec.Body.String())
	}
}

func TestUpdateAppearanceOptionsReturnsStyledBadRequestWhenIconModeInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("icon-mode", "bogus")
	form.Set("locale", "zh")
	req := httptest.NewRequest(http.MethodPost, "/settings/appearance", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateAppearanceOptions(c); err != nil {
		t.Fatalf("updateAppearanceOptions: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid icon mode") {
		t.Fatalf("expected invalid icon mode detail, got %s", rec.Body.String())
	}
}

func TestUpdateAppearanceOptionsReturnsStyledBadRequestWhenSiteIconInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("icon-mode", "FILLING")
	form.Set("locale", "zh")
	form.Set("site-icon", "not-a-real-icon")
	req := httptest.NewRequest(http.MethodPost, "/settings/appearance", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateAppearanceOptions(c); err != nil {
		t.Fatalf("updateAppearanceOptions: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid site icon value") {
		t.Fatalf("expected invalid site icon detail, got %s", rec.Body.String())
	}
}

func TestUpdateAppearanceOptionsReturnsStyledBadRequestWhenLocaleInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("icon-mode", "FILLING")
	form.Set("locale", "fr")
	req := httptest.NewRequest(http.MethodPost, "/settings/appearance", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateAppearanceOptions(c); err != nil {
		t.Fatalf("updateAppearanceOptions: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid locale value") {
		t.Fatalf("expected invalid locale detail, got %s", rec.Body.String())
	}
}

func TestUpdateAppearanceOptionsReturnsStyledBadRequestWhenLayoutNumberInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("icon-mode", "FILLING")
	form.Set("locale", "zh")
	form.Set("home-max-columns", "abc")
	req := httptest.NewRequest(http.MethodPost, "/settings/appearance", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateAppearanceOptions(c); err != nil {
		t.Fatalf("updateAppearanceOptions: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid home-max-columns value") {
		t.Fatalf("expected invalid layout number detail, got %s", rec.Body.String())
	}
}

func TestUpdateAppearanceOptionsReturnsStyledBadRequestWhenIconModeMissing(t *testing.T) {
	form := url.Values{}
	form.Set("locale", "zh")
	req := httptest.NewRequest(http.MethodPost, "/settings/appearance", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateAppearanceOptions(c); err != nil {
		t.Fatalf("updateAppearanceOptions: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "missing icon mode value") {
		t.Fatalf("expected missing icon mode detail, got %s", rec.Body.String())
	}
}

func TestUpdateAppearanceOptionsReturnsStyledBadRequestWhenLocaleMissing(t *testing.T) {
	form := url.Values{}
	form.Set("icon-mode", "FILLING")
	req := httptest.NewRequest(http.MethodPost, "/settings/appearance", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateAppearanceOptions(c); err != nil {
		t.Fatalf("updateAppearanceOptions: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "missing locale value") {
		t.Fatalf("expected missing locale detail, got %s", rec.Body.String())
	}
}

func TestPageAppearanceWarnsAndSanitizesInvalidSiteIcon(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: en\nTheme: blackboard\nSiteIcon: not-a-real-icon\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/appearance", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = appearanceCaptureRenderer{t: t}
	c := e.NewContext(req, rec)

	if err := pageAppearance(c); err != nil {
		t.Fatalf("pageAppearance: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestPageAppearanceSeparatesRawAndSanitizedFooter(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	rawFooter := `</textarea><script>alert(1)</script><a href="javascript:alert(1)">bad</a><strong>ok</strong><a href="https://example.com" target="_blank">link</a>`
	configBody := "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nFooter: |\n  " + strings.ReplaceAll(rawFooter, "\n", "\n  ") + "\n"
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte(configBody), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/appearance", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = appearanceFooterRenderer{t: t, rawFooter: rawFooter}
	c := e.NewContext(req, rec)

	if err := pageAppearance(c); err != nil {
		t.Fatalf("pageAppearance: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestUpdateAppearanceOptionsPersistsHideWarningsButton(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	form := url.Values{}
	form.Set("title", "SuperFlare")
	form.Set("icon-mode", "FILLING")
	form.Set("locale", "zh")
	form.Set("hide-warnings-button", "true")
	req := httptest.NewRequest(http.MethodPost, "/settings/appearance", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = appearanceSuccessRenderer{}
	c := e.NewContext(req, rec)

	if err := updateAppearanceOptions(c); err != nil {
		t.Fatalf("updateAppearanceOptions: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body=%s", rec.Code, rec.Body.String())
	}

	raw, err := os.ReadFile(filepath.Join(dir, "config.yml"))
	if err != nil {
		t.Fatalf("read config.yml: %v", err)
	}
	if !strings.Contains(string(raw), "HideWarningsButton: true") {
		t.Fatalf("expected HideWarningsButton to be saved, got %s", string(raw))
	}
}

func TestPageAppearanceKeepsStoredRuntimeDebugModeAfterAppFlagsChange(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	origFlags := define.AppFlags
	defer func() {
		define.AppFlags = origFlags
		settingsroot.SetRuntimeFlags(origFlags)
	}()

	define.AppFlags = model.Flags{DebugMode: true}
	settingsroot.SetRuntimeFlags(define.AppFlags)
	define.AppFlags = model.Flags{DebugMode: false}

	req := httptest.NewRequest(http.MethodGet, "/settings/appearance", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = appearanceDebugRenderer{t: t}
	c := e.NewContext(req, rec)

	if err := pageAppearance(c); err != nil {
		t.Fatalf("pageAppearance: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

type appearanceSuccessRenderer struct{}

func (appearanceSuccessRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	_, err := io.WriteString(w, "ok")
	return err
}

type appearanceCaptureRenderer struct {
	t *testing.T
}

func (r appearanceCaptureRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected renderer data type %T", data)
		}
	}
	if got, _ := m["OptionSiteIcon"].(string); got != "" {
		r.t.Fatalf("expected sanitized OptionSiteIcon, got %q", got)
	}
	warnings, ok := m["RenderWarnings"].([]string)
	if !ok || len(warnings) == 0 {
		r.t.Fatalf("expected render warnings, got %#v", m["RenderWarnings"])
	}
	for _, item := range warnings {
		if strings.Contains(item, "Site icon config error") {
			return nil
		}
	}
	r.t.Fatalf("unexpected render warnings: %#v", warnings)
	return nil
}

type appearanceFooterRenderer struct {
	t         *testing.T
	rawFooter string
}

type appearanceDebugRenderer struct {
	t *testing.T
}

func (r appearanceDebugRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected renderer data type %T", data)
		}
	}
	if got, _ := m["DebugMode"].(bool); !got {
		r.t.Fatalf("expected stored DebugMode=true, got %#v", m["DebugMode"])
	}
	_, err := io.WriteString(w, "ok")
	return err
}

func (r appearanceFooterRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected renderer data type %T", data)
		}
	}
	rawValue, ok := m["OptionFooter"].(string)
	if !ok {
		r.t.Fatalf("expected OptionFooter to stay a string, got %T", m["OptionFooter"])
	}
	if strings.TrimSpace(rawValue) != strings.TrimSpace(r.rawFooter) {
		r.t.Fatalf("expected raw footer %q, got %q", r.rawFooter, rawValue)
	}
	rendered, ok := m["RenderedFooter"].(template.HTML)
	if !ok {
		r.t.Fatalf("expected RenderedFooter to be trusted html, got %T", m["RenderedFooter"])
	}
	renderedText := string(rendered)
	for _, broken := range []string{`<script`, `javascript:`, `alert(1)`} {
		if strings.Contains(strings.ToLower(renderedText), broken) {
			r.t.Fatalf("expected sanitized rendered footer without %q, got %q", broken, renderedText)
		}
	}
	for _, expected := range []string{`bad`, `<strong>ok</strong>`, `href="https://example.com"`, `rel="noopener noreferrer"`} {
		if !strings.Contains(renderedText, expected) {
			r.t.Fatalf("expected rendered footer to contain %q, got %q", expected, renderedText)
		}
	}
	_, err := io.WriteString(w, "ok")
	return err
}
