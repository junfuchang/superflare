package auth

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestCheckUserIsLogin_DisableLoginMode(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = true

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.True(t, CheckUserIsLogin(c))
}

func TestGetUserName_DisableLoginMode(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = true

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.Empty(t, GetUserName(c))
}

func TestGetUserLoginDate_DisableLoginMode(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = true

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	assert.Empty(t, GetUserLoginDate(c))
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

func TestLogout_ClearsSession(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableLoginMode = false
	define.AppFlags.CookieName = "superflare"
	define.AppFlags.Port = 3636
	define.AppFlags.User = "u"
	define.AppFlags.Pass = "p"
	define.AppFlags.CookieSecret = "logout-test-secret"

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
