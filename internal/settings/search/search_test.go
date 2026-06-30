package search

import (
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

type searchTestRenderer struct{}

func (searchTestRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	_, err := io.WriteString(w, `<div class="status-panel">ok</div>`)
	return err
}

func TestUpdateSearchOptionsReturnsServerErrorWhenSaveFails(t *testing.T) {
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
	form.Set("show-search-component", "true")
	form.Set("search-mode", "engine")
	form.Set("search-engine", "google")
	form.Set("search-engine-open-mode", "new-tab")
	form.Set("search-engine-custom-template", "https://example.com/search?q=%s")
	req := httptest.NewRequest(http.MethodPost, "/settings/search", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = searchTestRenderer{}
	c := e.NewContext(req, rec)

	if err := updateSearchOptions(c); err != nil {
		t.Fatalf("updateSearchOptions: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestUpdateSearchOptionsReturnsStyledBadRequestWhenCustomEngineTemplateInvalid(t *testing.T) {
	form := url.Values{}
	form.Set("show-search-component", "true")
	form.Set("search-mode", "engine")
	form.Set("search-engine", "custom")
	form.Set("search-engine-open-mode", "same-tab")
	form.Set("search-engine-custom-template", "https://example.com/search")
	req := httptest.NewRequest(http.MethodPost, "/settings/search", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = searchTestRenderer{}
	c := e.NewContext(req, rec)

	if err := updateSearchOptions(c); err != nil {
		t.Fatalf("updateSearchOptions: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "custom search engine template must contain %s placeholder") {
		t.Fatalf("expected custom template validation error, got %s", rec.Body.String())
	}
}

func TestUpdateSearchOptionsReturnsStyledBadRequestWhenFormDataMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/settings/search", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateSearchOptions(c); err != nil {
		t.Fatalf("updateSearchOptions: %v", err)
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

func TestPageSearchReturnsStyledErrorWhenConfigBroken(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/search", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := pageSearch(c); err != nil {
		t.Fatalf("pageSearch: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestPageSearchKeepsStoredRuntimeDebugModeAfterAppFlagsChange(t *testing.T) {
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

	req := httptest.NewRequest(http.MethodGet, "/settings/search", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = searchCaptureRenderer{t: t, expectDebug: true}
	c := e.NewContext(req, rec)

	if err := pageSearch(c); err != nil {
		t.Fatalf("pageSearch: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

type searchCaptureRenderer struct {
	t           *testing.T
	expectDebug bool
}

func (r searchCaptureRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected renderer data type %T", data)
		}
	}
	got, _ := m["DebugMode"].(bool)
	if got != r.expectDebug {
		r.t.Fatalf("expected DebugMode=%v, got %v", r.expectDebug, got)
	}
	return nil
}
