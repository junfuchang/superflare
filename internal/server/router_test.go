package server

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
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

func newTestFlags(disableLogin bool, visibility string, enableEditor bool) model.Flags {
	env := define.GetDefaultEnvVars()
	return model.Flags{
		Port:             env.Port,
		EnableGuide:      false,
		EnableEditor:     enableEditor,
		DisableLoginMode: disableLogin,
		Visibility:       visibility,
		DebugMode:        false,
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

func TestNewRouter_SiteIconProxyFallbacksToBundledFavicon(t *testing.T) {
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
	assert.NotEmpty(t, rec.Header().Get("Content-Type"))
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
