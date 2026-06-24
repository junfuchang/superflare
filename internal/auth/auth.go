package auth

import (
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	session "github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/internal/statuspage"
)

const (
	SESSION_KEY_USER_NAME  = "USER_NAME"
	SESSION_KEY_LOGIN_DATE = "LOGIN_TIME"
	SESSION_MAX_AGE        = 90 * 24 * 60 * 60
)

// sessionName is set by RequestHandle and used by session.Get. Prefer passing name via RequestHandleSessionName.
var sessionName string

// RequestHandleSessionName returns the session name for the given cookie name and port (for testing or explicit wiring).
func RequestHandleSessionName(cookieName string, port int) string {
	return fmt.Sprintf("%s_%d", cookieName, port)
}

func RequestHandle(e *echo.Echo) {
	sessionName = RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port)
	if !define.AppFlags.DisableLoginMode {
		if define.AppFlags.CookieSecret == define.DEFAULT_COOKIE_SECRET {
			log.Println("[auth] warning: login mode is enabled but CookieSecret is still the default value; set FLARE_COOKIE_SECRET or --cookie-secret before production use")
		}
		store := sessions.NewCookieStore([]byte(define.AppFlags.CookieSecret))
		store.MaxAge(SESSION_MAX_AGE)
		e.Use(session.Middleware(store))
		e.POST(define.MiscPages.Login.Path, login)
		e.POST(define.MiscPages.Logout.Path, logout)
	}
}

func isDecodeSessionError(err error) bool {
	if err == nil {
		return false
	}
	if cookieErr, ok := err.(securecookie.Error); ok && cookieErr.IsDecode() {
		return true
	}
	if errs, ok := err.(interface{ Unwrap() []error }); ok {
		for _, item := range errs.Unwrap() {
			if isDecodeSessionError(item) {
				return true
			}
		}
	}
	return strings.Contains(err.Error(), "securecookie:")
}

func newFreshSession() *sessions.Session {
	store := sessions.NewCookieStore([]byte(define.AppFlags.CookieSecret))
	store.MaxAge(SESSION_MAX_AGE)
	fresh := sessions.NewSession(store, sessionName)
	opts := *store.Options
	fresh.Options = &opts
	fresh.IsNew = true
	return fresh
}

func buildSessionOptions(c *echo.Context, maxAge int) *sessions.Options {
	return &sessions.Options{
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Scheme() == "https",
	}
}

func clearSessionCookie(c *echo.Context) {
	c.SetCookie(&http.Cookie{
		Name:     sessionName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Scheme() == "https",
	})
}

func getSession(c *echo.Context) (*sessions.Session, error) {
	sess, err := session.Get(sessionName, c)
	if err == nil {
		return sess, nil
	}

	if isDecodeSessionError(err) {
		clearSessionCookie(c)
		return newFreshSession(), nil
	}

	return nil, err
}

func AuthRequired(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if !define.AppFlags.DisableLoginMode {
			sess, err := getSession(c)
			if err != nil {
				return c.Redirect(http.StatusFound, define.SettingPages.Others.Path)
			}
			user := sess.Values[SESSION_KEY_USER_NAME]
			if user == nil {
				return c.Redirect(http.StatusFound, define.SettingPages.Others.Path)
			}
		}
		return next(c)
	}
}

func CheckUserIsLogin(c *echo.Context) bool {
	if !define.AppFlags.DisableLoginMode {
		sess, err := getSession(c)
		if err != nil {
			return false
		}
		user := sess.Values[SESSION_KEY_USER_NAME]
		return user != nil
	}
	return true
}

func GetUserName(c *echo.Context) string {
	if !define.AppFlags.DisableLoginMode {
		sess, err := getSession(c)
		if err != nil {
			return ""
		}
		if v, ok := sess.Values[SESSION_KEY_USER_NAME].(string); ok {
			return v
		}
	}
	return ""
}

func GetUserLoginDate(c *echo.Context) string {
	if !define.AppFlags.DisableLoginMode {
		sess, err := getSession(c)
		if err != nil {
			return ""
		}
		if v, ok := sess.Values[SESSION_KEY_LOGIN_DATE].(string); ok {
			return v
		}
	}
	return ""
}

func login(c *echo.Context) error {
	sess, err := getSession(c)
	if err != nil {
		log.Println("failed to get login session:", err)
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildAuthSessionSaveErrorPage(statuspage.CurrentLocale(c)))
	}
	username := c.FormValue("username")
	password := c.FormValue("password")

	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildAuthEmptyCredentialsPage(statuspage.CurrentLocale(c)))
	}

	if subtle.ConstantTimeCompare([]byte(username), []byte(define.AppFlags.User)) != 1 ||
		subtle.ConstantTimeCompare([]byte(password), []byte(define.AppFlags.Pass)) != 1 {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildAuthInvalidCredentialsPage(statuspage.CurrentLocale(c)))
	}

	sess.Values[SESSION_KEY_USER_NAME] = username
	sess.Values[SESSION_KEY_LOGIN_DATE] = time.Now().Format("2006-01-02 15:04:05 MST")
	sess.Options = buildSessionOptions(c, SESSION_MAX_AGE)

	if err := sess.Save(c.Request(), c.Response()); err != nil {
		log.Println("failed to save login session:", err)
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildAuthSessionSaveErrorPage(statuspage.CurrentLocale(c)))
	}

	return c.Redirect(http.StatusFound, define.SettingPages.Others.Path)
}

func logout(c *echo.Context) error {
	sess, err := getSession(c)
	if err != nil {
		clearSessionCookie(c)
		return c.Redirect(http.StatusFound, define.SettingPages.Others.Path)
	}
	sess.Values = map[interface{}]interface{}{}
	sess.Options = buildSessionOptions(c, -1)

	if err := sess.Save(c.Request(), c.Response()); err != nil {
		log.Println("failed to save logout session:", err)
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildAuthSessionSaveErrorPage(statuspage.CurrentLocale(c)))
	}
	clearSessionCookie(c)
	return c.Redirect(http.StatusFound, define.SettingPages.Others.Path)
}
