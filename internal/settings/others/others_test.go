package others

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
)

func TestUpdateLoginOptionsDoesNotApplyRuntimeConfigWhenSaveFails(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.Mkdir("config.yml", 0755); err != nil {
		t.Fatalf("mkdir config.yml: %v", err)
	}

	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	t.Cleanup(func() {
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
	})
	define.AppFlags = model.Flags{User: "old-user", Pass: "old-pass"}
	define.AppBaseFlags = model.Flags{User: "old-user", Pass: "old-pass"}

	form := url.Values{}
	form.Set("login-user", "new-user")
	form.Set("login-pass", "new-pass")
	form.Set("login-pass-confirm", "new-pass")
	req := httptest.NewRequest(http.MethodPost, "/settings/others", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateLoginOptions(c); err != nil {
		t.Fatalf("updateLoginOptions: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
	if define.AppFlags.User != "old-user" || define.AppFlags.Pass != "old-pass" {
		t.Fatalf("runtime login config should remain unchanged, got user=%q pass=%q", define.AppFlags.User, define.AppFlags.Pass)
	}
}

func TestUpdateLoginOptionsReturnsStyledBadRequestWhenFormDataMissing(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/settings/others", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateLoginOptions(c); err != nil {
		t.Fatalf("updateLoginOptions: %v", err)
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

func TestApplyRuntimeLoginConfigUpdatesAuthSnapshotAtomically(t *testing.T) {
	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origLoginSnapshot := auth.SnapshotLoginRuntimeConfig()
	t.Cleanup(func() {
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
		auth.StoreLoginRuntimeConfig(origLoginSnapshot)
	})

	define.AppFlags = model.Flags{User: "old-user", Pass: "old-pass", CookieName: "superflare", Port: 3636}
	define.AppBaseFlags = model.Flags{User: "old-user", Pass: "old-pass", CookieName: "superflare", Port: 3636}
	auth.StoreLoginRuntimeConfig(auth.SnapshotLoginRuntimeConfigFromFlags(define.AppFlags))

	req := httptest.NewRequest(http.MethodGet, "/settings/others", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	applyRuntimeLoginConfig(c, "new-user", "new-pass")

	loginSnapshot := auth.SnapshotLoginRuntimeConfig()
	if loginSnapshot.User != "new-user" || loginSnapshot.Pass != "new-pass" {
		t.Fatalf("expected runtime login snapshot to update atomically, got user=%q pass=%q", loginSnapshot.User, loginSnapshot.Pass)
	}
	if define.AppBaseFlags.User != "new-user" || define.AppBaseFlags.Pass != "new-pass" {
		t.Fatalf("expected base flags to stay in sync, got user=%q pass=%q", define.AppBaseFlags.User, define.AppBaseFlags.Pass)
	}
}

func TestPageOthersReturnsStyledErrorWhenConfigBroken(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/others", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = testRenderer{}
	c := e.NewContext(req, rec)

	if err := pageOthers(c); err != nil {
		t.Fatalf("pageOthers: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestUpdateLoginOptionsReturnsStyledErrorWhenLoginConfigBroken(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)
	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.Mkdir(".env", 0755); err != nil {
		t.Fatalf("mkdir .env: %v", err)
	}

	form := url.Values{}
	form.Set("login-user", "")
	form.Set("login-pass", "")
	form.Set("login-pass-confirm", "")
	req := httptest.NewRequest(http.MethodPost, "/settings/others", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	e := echo.New()
	c := e.NewContext(req, rec)

	if err := updateLoginOptions(c); err != nil {
		t.Fatalf("updateLoginOptions: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestRenderOthersFallsBackToRuntimeLoginUserWhenPersistentConfigEmpty(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: \"\"\nLoginPass: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	t.Cleanup(func() {
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
	})
	define.AppFlags = model.Flags{User: "runtime-user", Pass: "runtime-pass", CookieName: "superflare", Port: 3636}
	define.AppBaseFlags = define.AppFlags

	req := httptest.NewRequest(http.MethodGet, "/settings/others", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = captureRenderer{}
	c := e.NewContext(req, rec)
	auth.StoreLoginRuntimeConfigForRequest(c, auth.SnapshotLoginRuntimeConfigFromFlags(define.AppFlags))

	if err := renderOthers(c, ""); err != nil {
		t.Fatalf("renderOthers: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRenderOthersFallsBackToRuntimeLoginUserWhenPersistentLoginConfigReadFails(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nLoginUser: \"\"\nLoginPass: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.Mkdir(".env", 0755); err != nil {
		t.Fatalf("mkdir .env: %v", err)
	}

	origFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	t.Cleanup(func() {
		define.AppFlags = origFlags
		define.AppBaseFlags = origBaseFlags
	})
	define.AppFlags = model.Flags{User: "runtime-user", Pass: "runtime-pass", CookieName: "superflare", Port: 3636}
	define.AppBaseFlags = define.AppFlags

	req := httptest.NewRequest(http.MethodGet, "/settings/others", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = loginFallbackRenderer{}
	c := e.NewContext(req, rec)
	auth.StoreLoginRuntimeConfigForRequest(c, auth.SnapshotLoginRuntimeConfigFromFlags(define.AppFlags))

	if err := renderOthers(c, ""); err != nil {
		t.Fatalf("renderOthers: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRenderOthersSurfacesSessionRecoveryWarningFromQuery(t *testing.T) {
	dir := t.TempDir()
	oldWd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer os.Chdir(oldWd)

	if err := os.WriteFile(filepath.Join(dir, "config.yml"), []byte("Title: SuperFlare\nLocale: en\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/settings/others?session-warning=session-invalid", nil)
	rec := httptest.NewRecorder()
	e := echo.New()
	e.Renderer = sessionWarningRenderer{}
	c := e.NewContext(req, rec)

	if err := renderOthers(c, ""); err != nil {
		t.Fatalf("renderOthers: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

type captureRenderer struct{}

func (captureRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			return nil
		}
	}
	if got, _ := m["OptionLoginUser"].(string); got != "runtime-user" {
		return echo.NewHTTPError(http.StatusInternalServerError, "unexpected OptionLoginUser: "+got)
	}
	if got, _ := m["LoginConfigError"].(string); got != "login_config_runtime_source" {
		return echo.NewHTTPError(http.StatusInternalServerError, "unexpected LoginConfigError: "+got)
	}
	if got, _ := m["LoginConfigErrorDetail"].(string); got != "" {
		return echo.NewHTTPError(http.StatusInternalServerError, "unexpected LoginConfigErrorDetail: "+got)
	}
	return nil
}

type loginFallbackRenderer struct{}

func (loginFallbackRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			return nil
		}
	}
	if got, _ := m["OptionLoginUser"].(string); got != "runtime-user" {
		return echo.NewHTTPError(http.StatusInternalServerError, "unexpected OptionLoginUser: "+got)
	}
	if got, _ := m["LoginConfigError"].(string); got != "login_config_runtime_fallback" {
		return echo.NewHTTPError(http.StatusInternalServerError, "unexpected LoginConfigError: "+got)
	}
	detail, _ := m["LoginConfigErrorDetail"].(string)
	if !strings.Contains(detail, "read login config failed") || !strings.Contains(detail, "directory") {
		return echo.NewHTTPError(http.StatusInternalServerError, "unexpected LoginConfigErrorDetail: "+detail)
	}
	return nil
}

type testRenderer struct{}

func (testRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	return nil
}

type sessionWarningRenderer struct{}

func (sessionWarningRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			return nil
		}
	}
	warnings, _ := m["RenderWarnings"].([]string)
	if len(warnings) == 0 {
		return echo.NewHTTPError(http.StatusInternalServerError, "missing render warnings")
	}
	for _, item := range warnings {
		if strings.Contains(item, "Login session data was invalid and has been cleared") {
			return nil
		}
	}
	return echo.NewHTTPError(http.StatusInternalServerError, "session recovery warning not found")
}
