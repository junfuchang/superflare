package auth

import (
	"errors"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gorilla/sessions"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func saveAppFlags() model.Flags {
	return define.AppFlags
}

func restoreAppFlags(f model.Flags) {
	define.AppFlags = f
}

func syncLoginRuntimeFromAppFlags() {
	StoreLoginRuntimeConfigFromFlags(define.AppFlags)
}

func restoreAuthTestHooks() func() {
	originalSessionGet := sessionGet
	originalPersistSession := persistSession
	return func() {
		sessionGet = originalSessionGet
		persistSession = originalPersistSession
	}
}

func TestAuthRequired_DisableLoginMode(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = true

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	called := false
	next := func(c *echo.Context) error {
		called = true
		return nil
	}
	handler := AuthRequired(next)
	err := handler(c)
	assert.NoError(t, err)
	assert.True(t, called)
}

func TestRequestHandle_DisableLoginMode(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = true
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636

	e := echo.New()
	RequestHandle(e)
	assert.NotNil(t, e)
}

func TestAuthRequired_LoginRequired_RedirectsWhenNoSession(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	next := func(c *echo.Context) error { return nil }
	handler := AuthRequired(next)
	err := handler(c)
	assert.NoError(t, err)
	assert.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, define.SettingPages.Others.Path, rec.Header().Get("Location"))
}

func TestLogin_Success_RedirectsAndSetsSession(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636
	define.AppFlags.User = "testuser"
	define.AppFlags.Pass = "testpass"
	define.AppFlags.CookieSecret = "test-secret-for-session"
	syncLoginRuntimeFromAppFlags()

	e := echo.New()
	RequestHandle(e)
	e.GET("/protected", func(c *echo.Context) error { return c.String(http.StatusOK, "ok") }, AuthRequired)

	loginBody := strings.NewReader("username=testuser&password=testpass")
	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, loginBody)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, define.SettingPages.Others.Path, rec.Header().Get("Location"))
	cookie := rec.Header().Get("Set-Cookie")
	require.NotEmpty(t, cookie)

	req2 := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req2.Header.Set("Cookie", cookie)
	rec2 := httptest.NewRecorder()
	e.ServeHTTP(rec2, req2)
	assert.Equal(t, http.StatusOK, rec2.Code)
}

func TestLogin_WrongPassword_ReturnsStyledPage(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636
	define.AppFlags.User = "u"
	define.AppFlags.Pass = "p"
	define.AppFlags.CookieSecret = "wrong-pw-test-secret"
	syncLoginRuntimeFromAppFlags()

	e := echo.New()
	RequestHandle(e)

	body := strings.NewReader("username=u&password=wrong")
	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "请输入正确的用户名和密码")
	assert.Contains(t, rec.Body.String(), "status-panel")
}

func TestLogin_InvalidSessionCookie_AllowsReLogin(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636
	define.AppFlags.User = "admin"
	define.AppFlags.Pass = "123456"
	define.AppFlags.CookieSecret = "invalid-cookie-relogin-secret"
	syncLoginRuntimeFromAppFlags()

	e := echo.New()
	RequestHandle(e)

	body := strings.NewReader("username=admin&password=123456")
	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Cookie", RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port)+"=invalid-cookie-value")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, define.SettingPages.Others.Path, rec.Header().Get("Location"))
	assert.Contains(t, rec.Header().Get("Set-Cookie"), RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port)+"=")
}

func TestLogin_EmptyCredentials_ReturnsStyledPage(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636
	define.AppFlags.CookieSecret = "empty-test-secret"
	syncLoginRuntimeFromAppFlags()

	e := echo.New()
	RequestHandle(e)

	body := strings.NewReader("username=&password=any")
	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "用户名或密码不能为空")
	assert.Contains(t, rec.Body.String(), "status-panel")
}

func TestLogin_Returns500WhenGetSessionFails(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	restoreHooks := restoreAuthTestHooks()
	defer restoreHooks()

	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636
	define.AppFlags.User = "admin"
	define.AppFlags.Pass = "secret"
	define.AppFlags.CookieSecret = "login-get-session-fail-secret"
	syncLoginRuntimeFromAppFlags()

	sessionGet = func(name string, c *echo.Context) (*sessions.Session, error) {
		return nil, errors.New("session backend unavailable")
	}

	e := echo.New()
	RequestHandle(e)

	body := strings.NewReader("username=admin&password=secret")
	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "status-panel")
}

func TestLogin_Returns500WhenSessionSaveFails(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	restoreHooks := restoreAuthTestHooks()
	defer restoreHooks()

	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636
	define.AppFlags.User = "admin"
	define.AppFlags.Pass = "secret"
	define.AppFlags.CookieSecret = "login-save-session-fail-secret"
	syncLoginRuntimeFromAppFlags()

	persistSession = func(sess *sessions.Session, req *http.Request, res http.ResponseWriter) error {
		return errors.New("session save failed")
	}

	e := echo.New()
	RequestHandle(e)

	body := strings.NewReader("username=admin&password=secret")
	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "status-panel")
}

func TestLogin_Returns500WhenRuntimeLoginConfigIncomplete(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)

	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636
	define.AppFlags.User = "admin"
	define.AppFlags.Pass = ""
	define.AppFlags.CookieSecret = "login-incomplete-runtime-secret"
	syncLoginRuntimeFromAppFlags()

	e := echo.New()
	RequestHandle(e)

	body := strings.NewReader("username=admin&password=secret")
	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "status-panel")
	assert.Contains(t, rec.Body.String(), "runtime login config")
}

func TestLogout_ClearsSession(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636
	define.AppFlags.User = "u"
	define.AppFlags.Pass = "p"
	define.AppFlags.CookieSecret = "logout-test-secret"
	syncLoginRuntimeFromAppFlags()

	e := echo.New()
	RequestHandle(e)
	e.GET("/protected", func(c *echo.Context) error { return c.String(http.StatusOK, "ok") }, AuthRequired)

	loginBody := strings.NewReader("username=u&password=p")
	reqLogin := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, loginBody)
	reqLogin.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recLogin := httptest.NewRecorder()
	e.ServeHTTP(recLogin, reqLogin)
	require.Equal(t, http.StatusFound, recLogin.Code)
	cookie := recLogin.Header().Get("Set-Cookie")
	require.NotEmpty(t, cookie)

	reqLogout := httptest.NewRequest(http.MethodPost, define.MiscPages.Logout.Path, nil)
	reqLogout.Header.Set("Cookie", cookie)
	recLogout := httptest.NewRecorder()
	e.ServeHTTP(recLogout, reqLogout)
	assert.Equal(t, http.StatusFound, recLogout.Code)
	cookieAfterLogout := recLogout.Header().Get("Set-Cookie")
	require.NotEmpty(t, cookieAfterLogout)
	assert.Contains(t, cookieAfterLogout, RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port)+"=")
	assert.Contains(t, cookieAfterLogout, "Path=/")
	assert.True(t, strings.Contains(cookieAfterLogout, "Max-Age=0") || strings.Contains(cookieAfterLogout, "Max-Age=-1"))

	reqGet := httptest.NewRequest(http.MethodGet, "/protected", nil)
	reqGet.Header.Set("Cookie", cookieAfterLogout)
	recGet := httptest.NewRecorder()
	e.ServeHTTP(recGet, reqGet)
	assert.Equal(t, http.StatusFound, recGet.Code)
	assert.Equal(t, define.SettingPages.Others.Path, recGet.Header().Get("Location"))
}

func TestLogout_Returns500WhenGetSessionFails(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	restoreHooks := restoreAuthTestHooks()
	defer restoreHooks()

	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636
	define.AppFlags.CookieSecret = "logout-get-session-fail-secret"
	syncLoginRuntimeFromAppFlags()

	sessionGet = func(name string, c *echo.Context) (*sessions.Session, error) {
		return nil, errors.New("session backend unavailable")
	}

	e := echo.New()
	RequestHandle(e)

	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Logout.Path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "status-panel")
}

func TestLogout_Returns500WhenSessionSaveFails(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	restoreHooks := restoreAuthTestHooks()
	defer restoreHooks()

	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636
	define.AppFlags.User = "admin"
	define.AppFlags.Pass = "secret"
	define.AppFlags.CookieSecret = "logout-save-session-fail-secret"
	syncLoginRuntimeFromAppFlags()

	persistSession = func(sess *sessions.Session, req *http.Request, res http.ResponseWriter) error {
		return errors.New("session save failed")
	}

	e := echo.New()
	RequestHandle(e)

	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Logout.Path, nil)
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "status-panel")
}

func TestRequestHandle_KeepsSessionConfigBoundPerRouter(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)

	define.AppFlags.DisableLoginMode = false
	define.AppFlags.User = "router-user"
	define.AppFlags.Pass = "router-pass"
	define.AppFlags.CookieSecret = "router-a-secret"
	define.AppFlags.CookieName = "superflare-a"
	define.AppFlags.Port = 3636
	syncLoginRuntimeFromAppFlags()

	routerA := echo.New()
	RequestHandle(routerA)
	routerA.GET("/protected", func(c *echo.Context) error { return c.String(http.StatusOK, "ok") }, AuthRequired)

	loginBody := strings.NewReader("username=router-user&password=router-pass")
	reqLoginA := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, loginBody)
	reqLoginA.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recLoginA := httptest.NewRecorder()
	routerA.ServeHTTP(recLoginA, reqLoginA)
	require.Equal(t, http.StatusFound, recLoginA.Code)

	cookieA := recLoginA.Header().Get("Set-Cookie")
	require.NotEmpty(t, cookieA)
	assert.Contains(t, cookieA, RequestHandleSessionName("superflare-a", 3636)+"=")

	define.AppFlags.CookieSecret = "router-b-secret"
	define.AppFlags.CookieName = "superflare-b"
	define.AppFlags.Port = 3737

	routerB := echo.New()
	RequestHandle(routerB)

	reqProtectedA := httptest.NewRequest(http.MethodGet, "/protected", nil)
	reqProtectedA.Header.Set("Cookie", cookieA)
	recProtectedA := httptest.NewRecorder()
	routerA.ServeHTTP(recProtectedA, reqProtectedA)
	assert.Equal(t, http.StatusOK, recProtectedA.Code)

	reqLogoutA := httptest.NewRequest(http.MethodPost, define.MiscPages.Logout.Path, nil)
	reqLogoutA.Header.Set("Cookie", cookieA)
	recLogoutA := httptest.NewRecorder()
	routerA.ServeHTTP(recLogoutA, reqLogoutA)
	assert.Equal(t, http.StatusFound, recLogoutA.Code)
	assert.Contains(t, recLogoutA.Header().Get("Set-Cookie"), RequestHandleSessionName("superflare-a", 3636)+"=")
	assert.NotContains(t, recLogoutA.Header().Get("Set-Cookie"), RequestHandleSessionName("superflare-b", 3737)+"=")

	reqProtectedAAfterLogout := httptest.NewRequest(http.MethodGet, "/protected", nil)
	reqProtectedAAfterLogout.Header.Set("Cookie", recLogoutA.Header().Get("Set-Cookie"))
	recProtectedAAfterLogout := httptest.NewRecorder()
	routerA.ServeHTTP(recProtectedAAfterLogout, reqProtectedAAfterLogout)
	assert.Equal(t, http.StatusFound, recProtectedAAfterLogout.Code)
	assert.Equal(t, define.SettingPages.Others.Path, recProtectedAAfterLogout.Header().Get("Location"))
}

func TestAuthRequired_KeepsDisableLoginModeBoundPerRouter(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)

	define.AppFlags.DisableLoginMode = false
	define.AppFlags.User = "bound-user"
	define.AppFlags.Pass = "bound-pass"
	define.AppFlags.CookieSecret = "bound-login-secret"
	define.AppFlags.CookieName = "bound-cookie"
	define.AppFlags.Port = 3636
	syncLoginRuntimeFromAppFlags()

	routerA := echo.New()
	RequestHandle(routerA)
	routerA.GET("/protected", func(c *echo.Context) error { return c.String(http.StatusOK, "ok") }, AuthRequired)

	define.AppFlags.DisableLoginMode = true
	define.AppFlags.CookieName = "other-cookie"
	define.AppFlags.Port = 3737

	reqA := httptest.NewRequest(http.MethodGet, "/protected", nil)
	recA := httptest.NewRecorder()
	routerA.ServeHTTP(recA, reqA)
	assert.Equal(t, http.StatusFound, recA.Code)
	assert.Equal(t, define.SettingPages.Others.Path, recA.Header().Get("Location"))

	routerB := echo.New()
	RequestHandle(routerB)
	routerB.GET("/protected", func(c *echo.Context) error { return c.String(http.StatusOK, "ok") }, AuthRequired)

	reqB := httptest.NewRequest(http.MethodGet, "/protected", nil)
	recB := httptest.NewRecorder()
	routerB.ServeHTTP(recB, reqB)
	assert.Equal(t, http.StatusOK, recB.Code)
}

func TestLogin_KeepsCredentialsBoundPerRouter(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)

	define.AppFlags = model.Flags{
		DisableLoginMode: false,
		User:             "router-a-user",
		Pass:             "router-a-pass",
		CookieSecret:     "router-a-secret",
		CookieName:       "router-a",
		Port:             3636,
	}
	syncLoginRuntimeFromAppFlags()

	routerA := echo.New()
	RequestHandle(routerA)

	define.AppFlags = model.Flags{
		DisableLoginMode: false,
		User:             "router-b-user",
		Pass:             "router-b-pass",
		CookieSecret:     "router-b-secret",
		CookieName:       "router-b",
		Port:             3737,
	}
	syncLoginRuntimeFromAppFlags()

	routerB := echo.New()
	RequestHandle(routerB)

	bodyA := strings.NewReader("username=router-a-user&password=router-a-pass")
	reqA := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, bodyA)
	reqA.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recA := httptest.NewRecorder()
	routerA.ServeHTTP(recA, reqA)
	require.Equal(t, http.StatusFound, recA.Code)

	bodyBWrong := strings.NewReader("username=router-a-user&password=router-a-pass")
	reqBWrong := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, bodyBWrong)
	reqBWrong.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recBWrong := httptest.NewRecorder()
	routerB.ServeHTTP(recBWrong, reqBWrong)
	assert.Equal(t, http.StatusBadRequest, recBWrong.Code)

	bodyB := strings.NewReader("username=router-b-user&password=router-b-pass")
	reqB := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, bodyB)
	reqB.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recB := httptest.NewRecorder()
	routerB.ServeHTTP(recB, reqB)
	require.Equal(t, http.StatusFound, recB.Code)
}

func TestLogin_UsesRouterBoundLoginSnapshotInsteadOfLaterAppFlags(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)

	define.AppFlags = model.Flags{
		DisableLoginMode: false,
		CookieName:       "superflare",
		Port:             3636,
		CookieSecret:     "snapshot-login-secret",
		User:             "snapshot-user",
		Pass:             "snapshot-pass",
	}

	e := echo.New()
	RequestHandle(e)

	define.AppFlags.User = "new-user"
	define.AppFlags.Pass = "new-pass"
	StoreLoginRuntimeConfigFromFlags(define.AppFlags)

	body := strings.NewReader("username=snapshot-user&password=snapshot-pass")
	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, define.SettingPages.Others.Path, rec.Header().Get("Location"))
}

func TestLogin_SucceedsWhenConfigBrokenButRuntimeCredentialsValid(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags = model.Flags{
		DisableLoginMode: false,
		CookieName:       "superflare",
		Port:             3636,
		CookieSecret:     "broken-config-login-secret",
		User:             "repair-user",
		Pass:             "repair-pass",
	}
	syncLoginRuntimeFromAppFlags()

	e := echo.New()
	RequestHandle(e)

	body := strings.NewReader("username=repair-user&password=repair-pass")
	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	require.Equal(t, http.StatusFound, rec.Code)
	assert.Equal(t, define.SettingPages.Others.Path, rec.Header().Get("Location"))
}

func TestLogin_WrongPasswordSurfacesSettingsConfigErrorWhenConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags = model.Flags{
		DisableLoginMode: false,
		CookieName:       "superflare",
		Port:             3636,
		CookieSecret:     "broken-config-wrong-password-secret",
		User:             "repair-user",
		Pass:             "repair-pass",
	}
	syncLoginRuntimeFromAppFlags()

	e := echo.New()
	RequestHandle(e)

	body := strings.NewReader("username=repair-user&password=wrong")
	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Login.Path, body)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "status-panel")
	assert.Contains(t, rec.Body.String(), "settings config error")
	assert.Contains(t, rec.Body.String(), "parse config config failed")
}

func TestAuthRequired_SessionReadErrorSurfacesSettingsConfigErrorWhenConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	restoreHooks := restoreAuthTestHooks()
	defer restoreHooks()
	define.AppFlags = model.Flags{
		DisableLoginMode: false,
		CookieName:       "superflare",
		Port:             3636,
		CookieSecret:     "broken-config-authrequired-secret",
		User:             "repair-user",
		Pass:             "repair-pass",
	}
	syncLoginRuntimeFromAppFlags()
	sessionGet = func(name string, c *echo.Context) (*sessions.Session, error) {
		return nil, errors.New("session backend unavailable")
	}

	e := echo.New()
	RequestHandle(e)
	e.GET("/protected", func(c *echo.Context) error { return c.String(http.StatusOK, "ok") }, AuthRequired)

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "status-panel")
	assert.Contains(t, rec.Body.String(), "session backend unavailable")
	assert.Contains(t, rec.Body.String(), "settings config error")
	assert.Contains(t, rec.Body.String(), "parse config config failed")
}

func TestLogout_GetSessionFailureSurfacesSettingsConfigErrorWhenConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	restoreHooks := restoreAuthTestHooks()
	defer restoreHooks()
	define.AppFlags = model.Flags{
		DisableLoginMode: false,
		CookieName:       "superflare",
		Port:             3636,
		CookieSecret:     "broken-config-logout-secret",
		User:             "repair-user",
		Pass:             "repair-pass",
	}
	syncLoginRuntimeFromAppFlags()
	sessionGet = func(name string, c *echo.Context) (*sessions.Session, error) {
		return nil, errors.New("session backend unavailable")
	}

	e := echo.New()
	RequestHandle(e)

	req := httptest.NewRequest(http.MethodPost, define.MiscPages.Logout.Path, nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Contains(t, rec.Body.String(), "status-panel")
	assert.Contains(t, rec.Body.String(), "settings config error")
	assert.Contains(t, rec.Body.String(), "parse config config failed")
}

func TestAuthRequired_RedirectsWhenSessionUserTypeInvalid(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	restoreHooks := restoreAuthTestHooks()
	defer restoreHooks()

	define.AppFlags = model.Flags{
		DisableLoginMode: false,
		User:             "admin",
		Pass:             "secret",
		CookieSecret:     "invalid-session-type-secret",
		CookieName:       "superflare",
		Port:             3636,
	}
	syncLoginRuntimeFromAppFlags()

	e := echo.New()
	RequestHandle(e)
	e.GET("/protected", func(c *echo.Context) error { return c.String(http.StatusOK, "ok") }, AuthRequired)
	e.GET("/seed-invalid", func(c *echo.Context) error {
		sess, err := getSession(c)
		if err != nil {
			return err
		}
		sess.Values[SESSION_KEY_USER_NAME] = 123
		sess.Options = buildSessionOptions(c, SESSION_MAX_AGE)
		return sess.Save(c.Request(), c.Response())
	})

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}
	server := httptest.NewServer(e)
	defer server.Close()

	resp, err := client.Get(server.URL + "/seed-invalid")
	if err != nil {
		t.Fatalf("seed invalid session: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected seed route 200, got %d", resp.StatusCode)
	}

	noRedirectClient := &http.Client{
		Jar: jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	protectedResp, err := noRedirectClient.Get(server.URL + "/protected")
	if err != nil {
		t.Fatalf("protected request: %v", err)
	}
	defer protectedResp.Body.Close()
	if protectedResp.StatusCode != http.StatusFound {
		t.Fatalf("expected redirect for invalid session type, got %d", protectedResp.StatusCode)
	}
	expectedLocation := define.SettingPages.Others.Path + "?" + sessionWarningQueryParam + "=" + sessionWarningSessionInvalid
	if location := protectedResp.Header.Get("Location"); location != expectedLocation {
		t.Fatalf("expected redirect to %s, got %s", expectedLocation, location)
	}
	clearedCookie := protectedResp.Header.Get("Set-Cookie")
	if !strings.Contains(clearedCookie, RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port)+"=") {
		t.Fatalf("expected invalid session cookie to be cleared, got %q", clearedCookie)
	}
	if !strings.Contains(clearedCookie, "Max-Age=0") && !strings.Contains(clearedCookie, "Max-Age=-1") {
		t.Fatalf("expected cleared cookie max-age, got %q", clearedCookie)
	}
}

func TestResolveLoginDisplayStateClearsInvalidLoginDateType(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	restoreHooks := restoreAuthTestHooks()
	defer restoreHooks()

	define.AppFlags = model.Flags{
		DisableLoginMode: false,
		CookieSecret:     "invalid-login-date-secret",
		CookieName:       "superflare",
		Port:             3636,
	}
	syncLoginRuntimeFromAppFlags()

	sessionGet = func(name string, c *echo.Context) (*sessions.Session, error) {
		return &sessions.Session{
			Values: map[interface{}]interface{}{
				SESSION_KEY_USER_NAME:  "admin",
				SESSION_KEY_LOGIN_DATE: 123,
			},
		}, nil
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/settings/others", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(sessionConfigContextKey, sessionRuntimeConfig{
		Name:         RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port),
		CookieSecret: define.AppFlags.CookieSecret,
	})

	state, err := ResolveLoginDisplayState(c)
	if err != nil {
		t.Fatalf("ResolveLoginDisplayState: %v", err)
	}
	if state.ShowLoginInfo || state.UserName != "" || state.LoginDate != "" {
		t.Fatalf("expected invalid login-date session to be discarded, got %#v", state)
	}
	clearedCookie := rec.Header().Get("Set-Cookie")
	if !strings.Contains(clearedCookie, RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port)+"=") {
		t.Fatalf("expected invalid session cookie to be cleared, got %q", clearedCookie)
	}
	if !strings.Contains(clearedCookie, "Max-Age=0") && !strings.Contains(clearedCookie, "Max-Age=-1") {
		t.Fatalf("expected cleared cookie max-age, got %q", clearedCookie)
	}
	warnings := AppendSessionWarnings(c, "en", nil)
	if len(warnings) != 1 {
		t.Fatalf("expected one session warning, got %#v", warnings)
	}
	if !strings.Contains(warnings[0], "Login session data was invalid and has been cleared") {
		t.Fatalf("unexpected session warning: %v", warnings)
	}
}

func TestResolveLoginDisplayStateForViewReturnsEmptyOnSessionReadError(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	restoreHooks := restoreAuthTestHooks()
	defer restoreHooks()

	define.AppFlags = model.Flags{
		DisableLoginMode: false,
		CookieSecret:     "view-session-error-secret",
		CookieName:       "superflare",
		Port:             3636,
	}
	syncLoginRuntimeFromAppFlags()

	sessionGet = func(name string, c *echo.Context) (*sessions.Session, error) {
		return nil, errors.New("session backend unavailable")
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/settings/others", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(sessionConfigContextKey, sessionRuntimeConfig{
		Name:         RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port),
		CookieSecret: define.AppFlags.CookieSecret,
	})

	state := ResolveLoginDisplayStateForView(c)
	if state.ShowLoginInfo || state.UserName != "" || state.LoginDate != "" {
		t.Fatalf("expected empty login display state on session read error, got %#v", state)
	}
}

func TestResolveLoginDisplayStateForStrictViewReturnsErrorOnSessionReadError(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	restoreHooks := restoreAuthTestHooks()
	defer restoreHooks()

	define.AppFlags = model.Flags{
		DisableLoginMode: false,
		CookieSecret:     "strict-view-session-error-secret",
		CookieName:       "superflare",
		Port:             3636,
	}
	syncLoginRuntimeFromAppFlags()

	sessionGet = func(name string, c *echo.Context) (*sessions.Session, error) {
		return nil, errors.New("session backend unavailable")
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/settings/theme", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	c.Set(sessionConfigContextKey, sessionRuntimeConfig{
		Name:         RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port),
		CookieSecret: define.AppFlags.CookieSecret,
	})

	_, err := ResolveLoginDisplayStateForStrictView(c)
	if err == nil {
		t.Fatal("expected strict view resolver to return error")
	}
	if !strings.Contains(err.Error(), "resolve login display state failed") {
		t.Fatalf("expected wrapped strict view error, got %v", err)
	}
}

func TestResolveLoginDisplayStateForStrictViewReturnsEmptyWhenLoginDisabled(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)

	define.AppFlags = model.Flags{DisableLoginMode: true}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/settings/theme", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	state, err := ResolveLoginDisplayStateForStrictView(c)
	if err != nil {
		t.Fatalf("ResolveLoginDisplayStateForStrictView: %v", err)
	}
	if state.ShowLoginInfo || state.UserName != "" || state.LoginDate != "" {
		t.Fatalf("expected empty login display state when login disabled, got %#v", state)
	}
}
