package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func withRouterTestWorkingDir(t *testing.T) {
	t.Helper()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	tmpDir := t.TempDir()
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
	})

	origEnv := os.Getenv("FLARE_BASELINE")
	require.NoError(t, os.Setenv("FLARE_BASELINE", "1"))
	t.Cleanup(func() {
		if origEnv == "" {
			_ = os.Unsetenv("FLARE_BASELINE")
		} else {
			_ = os.Setenv("FLARE_BASELINE", origEnv)
		}
	})
}

func withRouterDebugTestWorkingDir(t *testing.T) {
	t.Helper()
	origWd, err := os.Getwd()
	require.NoError(t, err)
	repoRoot := filepath.Clean(filepath.Join(origWd, "..", ".."))
	tmpDir := t.TempDir()
	require.NoError(t, copyDir(filepath.Join(repoRoot, "embed", "templates"), filepath.Join(tmpDir, "embed", "templates")))
	require.NoError(t, copyDir(filepath.Join(repoRoot, "internal", "resources", "templates", "html"), filepath.Join(tmpDir, "internal", "resources", "templates", "html")))
	require.NoError(t, os.Chdir(tmpDir))
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
	})

	origEnv := os.Getenv("FLARE_BASELINE")
	require.NoError(t, os.Setenv("FLARE_BASELINE", "1"))
	t.Cleanup(func() {
		if origEnv == "" {
			_ = os.Unsetenv("FLARE_BASELINE")
		} else {
			_ = os.Setenv("FLARE_BASELINE", origEnv)
		}
	})
}

func copyDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func newTestFlags(disableLogin bool, visibility string, enableEditor bool) model.Flags {
	env := define.GetDefaultEnvVars()
	return model.Flags{
		Port:             env.Port,
		EnableGuide:      false,
		EnableEditor:     enableEditor,
		DisableLoginMode: disableLogin,
		Visibility:       visibility,
		DebugMode:        false,
		User:             env.User,
		Pass:             env.Pass,
		CookieName:       env.CookieName,
		CookieSecret:     env.CookieSecret,
	}
}

func TestNewRouter_Smoke(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(true, "DEFAULT", false)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "GET / should return 200")
	for _, name := range []string{"apps.yml", "bookmarks.yml", "ports.yaml"} {
		if _, err := os.Stat(filepath.Join(".", name)); err != nil {
			t.Fatalf("expected runtime data file %s to be initialized: %v", name, err)
		}
	}
}

func TestNewRouter_PrivateVisibility_AllowsAnonymousHome(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(false, "PRIVATE", true)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "GET / should stay public in PRIVATE visibility")
}

func TestNewRouter_PrivateVisibility_ProtectsEditor(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(false, "PRIVATE", true)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/editor", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code, "GET /editor should require login")
	assert.Equal(t, define.SettingPages.Others.Path, rec.Header().Get("Location"))
}

func TestNewRouter_SettingsThemeRequiresLoginAndLoadsAfterLogin(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(false, "DEFAULT", true)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, define.SettingPages.Theme.Path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, define.SettingPages.Others.Path, rec.Header().Get("Location"))

	loginBody := strings.NewReader("username=admin&password=admin")
	reqLogin := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, loginBody)
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recLogin := httptest.NewRecorder()
	handler.ServeHTTP(recLogin, reqLogin)

	require.Equal(t, http.StatusFound, recLogin.Code)
	cookie := recLogin.Header().Get("Set-Cookie")
	require.NotEmpty(t, cookie)

	reqTheme := httptest.NewRequest(http.MethodGet, define.SettingPages.Theme.Path, nil)
	reqTheme.Header.Set("Cookie", cookie)
	recTheme := httptest.NewRecorder()
	handler.ServeHTTP(recTheme, reqTheme)

	assert.Equal(t, http.StatusOK, recTheme.Code)
	assert.Contains(t, recTheme.Body.String(), "custom-theme-background")
	assert.Contains(t, recTheme.Body.String(), "data-color-picker")
}

func TestNewRouter_LoginDisabledHidesLoginConfigAndPortsEntry(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(true, "DEFAULT", true)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, define.SettingPages.Others.Path, nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.NotContains(t, body, `id="settings-login-user"`)
	assert.NotContains(t, body, `name="login-user"`)
	assert.NotContains(t, body, `href="`+define.SettingPages.Ports.Path+`"`)
}

func TestNewRouter_LoginDisabledBlocksSensitiveSettingsRoutes(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(true, "DEFAULT", true)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	for _, tc := range []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{name: "login config save", method: http.MethodPost, path: define.SettingPages.Others.Path, body: "login-user=new&login-pass=new&login-pass-confirm=new"},
		{name: "page", method: http.MethodGet, path: define.SettingPages.Ports.Path},
		{name: "data", method: http.MethodGet, path: define.SettingPages.Ports.Path + "/data"},
		{name: "save", method: http.MethodPost, path: define.SettingPages.Ports.Path, body: "ports=[]"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
			if tc.method == http.MethodPost {
				req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			}
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			assert.Equal(t, http.StatusNotFound, rec.Code)
		})
	}
}

func TestNewRouter_SettingsThemeHidesStoredCustomColorsWhenActiveThemeIsPreset(t *testing.T) {
	withRouterTestWorkingDir(t)

	config := "Title: SuperFlare\nLocale: zh\nTheme: onedark\nThemeBase: onedark\nCustomThemeBackground: rgba(26, 26, 26, 1)\nCustomThemePrimary: rgba(118, 255, 71, 1)\nCustomThemeAccent: rgba(110, 255, 204, 1)\n"
	require.NoError(t, os.WriteFile(filepath.Join(".", "config.yml"), []byte(config), 0644))

	flags := newTestFlags(false, "DEFAULT", true)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	loginBody := strings.NewReader("username=admin&password=admin")
	reqLogin := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, loginBody)
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recLogin := httptest.NewRecorder()
	handler.ServeHTTP(recLogin, reqLogin)

	require.Equal(t, http.StatusFound, recLogin.Code)
	cookie := recLogin.Header().Get("Set-Cookie")
	require.NotEmpty(t, cookie)

	reqTheme := httptest.NewRequest(http.MethodGet, define.SettingPages.Theme.Path, nil)
	reqTheme.Header.Set("Cookie", cookie)
	recTheme := httptest.NewRecorder()
	handler.ServeHTTP(recTheme, reqTheme)

	require.Equal(t, http.StatusOK, recTheme.Code)
	body := recTheme.Body.String()
	assert.Contains(t, body, `name="custom-theme-background"`)
	assert.Contains(t, body, `name="custom-theme-primary"`)
	assert.Contains(t, body, `name="custom-theme-accent"`)
	assert.Contains(t, body, `name="custom-theme-background" id="custom-theme-background" class="option-input" data-color-picker="true" placeholder="rgba(26, 26, 26, 1)" value=""`)
	assert.Contains(t, body, `name="custom-theme-primary" id="custom-theme-primary" class="option-input" data-color-picker="true" placeholder="rgba(255, 253, 234, 1)" value=""`)
	assert.Contains(t, body, `name="custom-theme-accent" id="custom-theme-accent" class="option-input" data-color-picker="true" placeholder="rgba(92, 92, 92, 1)" value=""`)
}

func TestNewRouter_LoginUsesProvidedFlagsSnapshot(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(false, "DEFAULT", true)
	flags.User = "router-admin"
	flags.Pass = "router-pass"
	flags.CookieSecret = "router-flags-secret"

	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	loginBody := strings.NewReader("username=router-admin&password=router-pass")
	reqLogin := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, loginBody)
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recLogin := httptest.NewRecorder()
	handler.ServeHTTP(recLogin, reqLogin)

	assert.Equal(t, http.StatusFound, recLogin.Code)
	assert.Equal(t, define.SettingPages.Others.Path, recLogin.Header().Get("Location"))
	assert.NotEmpty(t, recLogin.Header().Get("Set-Cookie"))
}

func TestNewRouter_ResetsBaseFlagsFromProvidedFlags(t *testing.T) {
	withRouterTestWorkingDir(t)

	origAppFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	t.Cleanup(func() {
		define.AppFlags = origAppFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
	})

	define.AppBaseFlags = model.Flags{
		Port:       9999,
		User:       "stale-user",
		Pass:       "stale-pass",
		CookieName: "stale-cookie",
	}

	flags := newTestFlags(false, "DEFAULT", true)
	flags.Port = 3636
	flags.User = "fresh-user"
	flags.Pass = "fresh-pass"
	flags.CookieName = "fresh-cookie"
	flags.CookieSecret = "fresh-secret"

	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	if define.AppBaseFlags.Port != 3636 || define.AppBaseFlags.User != "fresh-user" || define.AppBaseFlags.Pass != "fresh-pass" || define.AppBaseFlags.CookieName != "fresh-cookie" {
		t.Fatalf("expected base flags reset from provided flags, got %#v", define.AppBaseFlags)
	}
	if define.AppSourceFlags.Port != 3636 || define.AppSourceFlags.User != "fresh-user" || define.AppSourceFlags.Pass != "fresh-pass" || define.AppSourceFlags.CookieName != "fresh-cookie" {
		t.Fatalf("expected source flags reset from provided flags, got %#v", define.AppSourceFlags)
	}
}

func TestNewRouter_ReinitializesInlineStyleForDebugMode(t *testing.T) {
	withRouterDebugTestWorkingDir(t)

	origAppFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	t.Cleanup(func() {
		define.AppFlags = origAppFlags
		define.AppBaseFlags = origBaseFlags
	})

	normalFlags := newTestFlags(true, "DEFAULT", false)
	normalFlags.DebugMode = false
	handler, err := NewRouter(&normalFlags)
	require.NoError(t, err)
	require.NotNil(t, handler)
	if define.GetPageInlineStyle() == "" {
		t.Fatal("expected non-debug router to populate inline style cache")
	}

	debugFlags := newTestFlags(true, "DEFAULT", false)
	debugFlags.DebugMode = true
	handler, err = NewRouter(&debugFlags)
	require.NoError(t, err)
	require.NotNil(t, handler)
	if define.GetPageInlineStyle() != "" {
		t.Fatalf("expected debug router to clear inline style cache, got %q", string(define.GetPageInlineStyle()))
	}
}

func TestNewRouter_BindsEditorAvailabilityToProvidedFlagsSnapshot(t *testing.T) {
	withRouterTestWorkingDir(t)

	origAppFlags := define.AppFlags
	t.Cleanup(func() {
		define.AppFlags = origAppFlags
	})

	flags := newTestFlags(true, "DEFAULT", false)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	define.AppFlags.EnableEditor = true

	req := httptest.NewRequest(http.MethodGet, "/editor", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code, "router should keep initial editor-disabled route set")
}

func TestNewRouter_PrivateVisibility_AllowsAnonymousHelp(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(false, "PRIVATE", true)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "GET /help should stay public in PRIVATE visibility")
	assert.NotContains(t, rec.Body.String(), define.SettingPages.Theme.Path)
	assert.NotContains(t, rec.Body.String(), define.SettingPages.Search.Path)
	assert.NotContains(t, rec.Body.String(), define.SettingPages.Appearance.Path)
	assert.NotContains(t, rec.Body.String(), define.SettingPages.Others.Path)
	assert.NotContains(t, rec.Body.String(), "https://github.com/junfuchang/superflare/issues")
}

func TestNewRouter_CompressesHomeWhenClientSupportsGzip(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(true, "DEFAULT", false)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code, "GET / should return 200")
	assert.Equal(t, "gzip", rec.Header().Get("Content-Encoding"))
	assert.Contains(t, rec.Header().Get("Vary"), "Accept-Encoding")

	reader, err := gzip.NewReader(rec.Body)
	require.NoError(t, err)
	defer reader.Close()

	body, err := io.ReadAll(reader)
	require.NoError(t, err)
	assert.Contains(t, string(body), "<!doctype html>")
	assert.Contains(t, string(body), `<div class="pageview" id="page-home">`)
}

func TestNewRouter_SiteIconProxyFallbacksToBuiltinBookmarkIcon(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(true, "DEFAULT", false)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/assets/site-icons?src=https://example.com/favicon.ico", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, rec.Body.Bytes())
	assert.Contains(t, rec.Header().Get("Content-Type"), "image/svg+xml")
	assert.Contains(t, rec.Body.String(), "<svg")
}

func TestNewRouter_MissingUploadedBackgroundReturnsNotFound(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(true, "DEFAULT", false)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/user-assets/background", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestNewRouter_NotFoundHTMLUsesStyledStatusPage(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(true, "DEFAULT", false)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/missing-page", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
	assert.Contains(t, rec.Body.String(), "页面不存在")
	assert.Contains(t, rec.Body.String(), "status-panel")
}

func TestNewRouterReturnsErrorWhenThemeStateInitFails(t *testing.T) {
	withRouterTestWorkingDir(t)

	if err := os.WriteFile(filepath.Join(".", "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	flags := newTestFlags(true, "DEFAULT", false)
	_, err := NewRouter(&flags)
	if err == nil {
		t.Fatal("expected NewRouter to fail")
	}
	if !strings.Contains(err.Error(), "initialize theme state") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRouterReturnsErrorWhenPortInvalid(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(true, "DEFAULT", false)
	flags.Port = 0

	_, err := NewRouter(&flags)
	if err == nil {
		t.Fatal("expected NewRouter to fail")
	}
	if !strings.Contains(err.Error(), "validate router flags") || !strings.Contains(err.Error(), "invalid port 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRouterReturnsErrorWhenCookieSecretEmptyWithLoginEnabled(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(false, "DEFAULT", false)
	flags.CookieSecret = ""

	_, err := NewRouter(&flags)
	if err == nil {
		t.Fatal("expected NewRouter to fail")
	}
	if !strings.Contains(err.Error(), "validate router flags") || !strings.Contains(err.Error(), "cookie secret") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRouterReplacesDefaultCookieSecretAtRuntime(t *testing.T) {
	withRouterTestWorkingDir(t)

	origAppFlags := define.AppFlags
	origBaseFlags := define.AppBaseFlags
	origSourceFlags := define.AppSourceFlags
	t.Cleanup(func() {
		define.AppFlags = origAppFlags
		define.AppBaseFlags = origBaseFlags
		define.AppSourceFlags = origSourceFlags
	})

	flags := newTestFlags(false, "DEFAULT", false)
	flags.CookieSecret = define.DEFAULT_COOKIE_SECRET

	handler, err := NewRouter(&flags)
	if err != nil {
		t.Fatalf("expected NewRouter to accept default cookie secret by replacing it at runtime, got %v", err)
	}
	if handler == nil {
		t.Fatal("expected router handler")
	}
	if define.AppFlags.CookieSecret == define.DEFAULT_COOKIE_SECRET || define.AppFlags.CookieSecret == "" {
		t.Fatalf("expected runtime app flags to use a generated cookie secret, got %q", define.AppFlags.CookieSecret)
	}
	if define.AppBaseFlags.CookieSecret == define.DEFAULT_COOKIE_SECRET || define.AppBaseFlags.CookieSecret == "" {
		t.Fatalf("expected runtime base flags to use a generated cookie secret, got %q", define.AppBaseFlags.CookieSecret)
	}
	if define.AppSourceFlags.CookieSecret == define.DEFAULT_COOKIE_SECRET || define.AppSourceFlags.CookieSecret == "" {
		t.Fatalf("expected runtime source flags to use a generated cookie secret, got %q", define.AppSourceFlags.CookieSecret)
	}
}

func TestNewRouterReturnsErrorWhenLoginCredentialsIncomplete(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(false, "DEFAULT", false)
	flags.User = "admin"
	flags.Pass = ""

	_, err := NewRouter(&flags)
	if err == nil {
		t.Fatal("expected NewRouter to fail")
	}
	if !strings.Contains(err.Error(), "validate router flags") || !strings.Contains(err.Error(), "login credentials") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRouterReturnsErrorWhenVisibilityInvalid(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(true, "mystery", false)

	_, err := NewRouter(&flags)
	if err == nil {
		t.Fatal("expected NewRouter to fail")
	}
	if !strings.Contains(err.Error(), "validate router flags") || !strings.Contains(err.Error(), "invalid visibility") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNewRouterNormalizesLowercasePrivateVisibility(t *testing.T) {
	withRouterTestWorkingDir(t)

	flags := newTestFlags(false, "private", true)
	handler, err := NewRouter(&flags)
	require.NoError(t, err)
	require.NotNil(t, handler)

	req := httptest.NewRequest(http.MethodGet, "/editor", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusFound, rec.Code, "GET /editor should require login")
	assert.Equal(t, define.SettingPages.Others.Path, rec.Header().Get("Location"))
}
