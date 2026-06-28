package auth

import (
	"crypto/subtle"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/securecookie"
	"github.com/gorilla/sessions"
	session "github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/statuspage"
)

const (
	SESSION_KEY_USER_NAME  = "USER_NAME"
	SESSION_KEY_LOGIN_DATE = "LOGIN_TIME"
	SESSION_MAX_AGE        = 90 * 24 * 60 * 60
)

const (
	sessionConfigContextKey  = "superflare.auth.session-config"
	loginConfigContextKey    = "superflare.auth.login-config"
	sessionWarningContextKey = "superflare.auth.session-warning"
)

const (
	sessionWarningQueryParam     = "session-warning"
	sessionWarningCookieInvalid  = "cookie-invalid"
	sessionWarningSessionInvalid = "session-invalid"
)

type sessionRuntimeConfig struct {
	Name         string
	CookieSecret string
	DisableLogin bool
}

type loginRuntimeConfig struct {
	User            string
	Pass            string
	UserIsGenerated bool
	PassIsGenerated bool
}

type loginRuntimeHolder struct {
	mu  sync.RWMutex
	cfg loginRuntimeConfig
}

type LoginDisplayState struct {
	ShowLoginInfo bool
	UserName      string
	LoginDate     string
}

type sessionWarning struct {
	Code   string
	Detail string
}

func newLoginRuntimeHolder(cfg loginRuntimeConfig) *loginRuntimeHolder {
	return &loginRuntimeHolder{cfg: cfg}
}

func (h *loginRuntimeHolder) Load() loginRuntimeConfig {
	if h == nil {
		return loginRuntimeConfig{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg
}

func (h *loginRuntimeHolder) Store(cfg loginRuntimeConfig) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.cfg = cfg
	h.mu.Unlock()
}

var loginRuntimeValues sync.Map
var sessionGet = session.Get
var persistSession = func(sess *sessions.Session, req *http.Request, res http.ResponseWriter) error {
	return sess.Save(req, res)
}

// RequestHandleSessionName returns the session name for the given cookie name and port (for testing or explicit wiring).
func RequestHandleSessionName(cookieName string, port int) string {
	return fmt.Sprintf("%s_%d", cookieName, port)
}

func SnapshotLoginRuntimeConfig() loginRuntimeConfig {
	return SnapshotLoginRuntimeConfigForSessionName(RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port))
}

func SnapshotLoginRuntimeConfigForSessionName(sessionName string) loginRuntimeConfig {
	if cfg, ok := loginRuntimeValues.Load(sessionName); ok {
		if loginCfg, typed := cfg.(loginRuntimeConfig); typed {
			return loginCfg
		}
	}
	return loginRuntimeConfig{}
}

func StoreLoginRuntimeConfig(cfg loginRuntimeConfig) {
	StoreLoginRuntimeConfigForSessionName(RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port), cfg)
}

func StoreLoginRuntimeConfigForSessionName(sessionName string, cfg loginRuntimeConfig) {
	if strings.TrimSpace(sessionName) == "" {
		return
	}
	loginRuntimeValues.Store(sessionName, cfg)
}

func StoreLoginRuntimeConfigForRequest(c *echo.Context, cfg loginRuntimeConfig) {
	if holder := getLoginRuntimeHolder(c); holder != nil {
		holder.Store(cfg)
	}
	StoreLoginRuntimeConfigForSessionName(SessionNameForRequest(c), cfg)
}

func SessionNameForRequest(c *echo.Context) string {
	cfg := getSessionRuntimeConfig(c)
	if strings.TrimSpace(cfg.Name) != "" {
		return cfg.Name
	}
	return RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port)
}

func SnapshotLoginRuntimeConfigForRequest(c *echo.Context) loginRuntimeConfig {
	if holder := getLoginRuntimeHolder(c); holder != nil {
		return holder.Load()
	}
	if cfg := SnapshotLoginRuntimeConfigForSessionName(SessionNameForRequest(c)); cfg != (loginRuntimeConfig{}) {
		return cfg
	}
	return SnapshotLoginRuntimeConfigFromFlags(define.AppFlags)
}

func SnapshotLoginRuntimeConfigFromFlags(flags model.Flags) loginRuntimeConfig {
	return loginRuntimeConfig{
		User:            flags.User,
		Pass:            flags.Pass,
		UserIsGenerated: flags.UserIsGenerated,
		PassIsGenerated: flags.PassIsGenerated,
	}
}

func StoreLoginRuntimeConfigFromFlags(flags model.Flags) {
	StoreLoginRuntimeConfigForSessionName(RequestHandleSessionName(flags.CookieName, flags.Port), SnapshotLoginRuntimeConfigFromFlags(flags))
}

func RequestHandle(e *echo.Echo) {
	cfg := sessionRuntimeConfig{
		Name:         RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port),
		CookieSecret: define.AppFlags.CookieSecret,
		DisableLogin: define.AppFlags.DisableLoginMode,
	}
	loginCfg := SnapshotLoginRuntimeConfigFromFlags(define.AppFlags)
	StoreLoginRuntimeConfigForSessionName(cfg.Name, loginCfg)
	e.Use(bindSessionRuntimeConfig(cfg, newLoginRuntimeHolder(loginCfg)))
	if !cfg.DisableLogin {
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

func bindSessionRuntimeConfig(cfg sessionRuntimeConfig, loginCfg *loginRuntimeHolder) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			c.Set(sessionConfigContextKey, cfg)
			if loginCfg != nil {
				c.Set(loginConfigContextKey, loginCfg)
			}
			return next(c)
		}
	}
}

func getLoginRuntimeHolder(c *echo.Context) *loginRuntimeHolder {
	if c == nil {
		return nil
	}
	if holder, ok := c.Get(loginConfigContextKey).(*loginRuntimeHolder); ok {
		return holder
	}
	return nil
}

func getSessionRuntimeConfig(c *echo.Context) sessionRuntimeConfig {
	if c != nil {
		if cfg, ok := c.Get(sessionConfigContextKey).(sessionRuntimeConfig); ok {
			if strings.TrimSpace(cfg.Name) != "" && strings.TrimSpace(cfg.CookieSecret) != "" {
				return cfg
			}
		}
	}
	return sessionRuntimeConfig{
		Name:         RequestHandleSessionName(define.AppFlags.CookieName, define.AppFlags.Port),
		CookieSecret: define.AppFlags.CookieSecret,
		DisableLogin: define.AppFlags.DisableLoginMode,
	}
}

func hasBoundSessionRuntimeConfig(c *echo.Context) bool {
	if c == nil {
		return false
	}
	_, ok := c.Get(sessionConfigContextKey).(sessionRuntimeConfig)
	return ok
}

func IsLoginDisabled(c *echo.Context) bool {
	return getSessionRuntimeConfig(c).DisableLogin
}

func ResolveLoginDisplayState(c *echo.Context) (LoginDisplayState, error) {
	state := LoginDisplayState{}
	if IsLoginDisabled(c) || !hasBoundSessionRuntimeConfig(c) {
		return state, nil
	}
	sess, err := getSession(c)
	if err != nil {
		return state, fmt.Errorf("read login session failed: %w", err)
	}
	userRaw := sess.Values[SESSION_KEY_USER_NAME]
	if userRaw == nil {
		return state, nil
	}
	userName, ok := userRaw.(string)
	if !ok {
		clearSessionCookie(c)
		recordSessionWarning(c, sessionWarningSessionInvalid, "invalid username type in login session")
		return LoginDisplayState{}, nil
	}
	userName = strings.TrimSpace(userName)
	if userName == "" {
		clearSessionCookie(c)
		recordSessionWarning(c, sessionWarningSessionInvalid, "empty username in login session")
		return LoginDisplayState{}, nil
	}
	state.ShowLoginInfo = true
	state.UserName = userName
	loginDateRaw := sess.Values[SESSION_KEY_LOGIN_DATE]
	if loginDateRaw == nil {
		return state, nil
	}
	loginDate, ok := loginDateRaw.(string)
	if !ok {
		clearSessionCookie(c)
		recordSessionWarning(c, sessionWarningSessionInvalid, "invalid login timestamp type in login session")
		return LoginDisplayState{}, nil
	}
	state.LoginDate = loginDate
	return state, nil
}

func ResolveLoginDisplayStateForView(c *echo.Context) LoginDisplayState {
	state, err := ResolveLoginDisplayState(c)
	if err != nil {
		log.Println("failed to resolve login display state:", err)
		return LoginDisplayState{}
	}
	return state
}

func ResolveLoginDisplayStateForStrictView(c *echo.Context) (LoginDisplayState, error) {
	if IsLoginDisabled(c) {
		return LoginDisplayState{}, nil
	}
	state, err := ResolveLoginDisplayState(c)
	if err != nil {
		return LoginDisplayState{}, fmt.Errorf("resolve login display state failed: %w", err)
	}
	return state, nil
}

func validateRuntimeLoginConfig(cfg loginRuntimeConfig) error {
	if err := data.ValidateLoginCredentialPair(cfg.User, cfg.Pass, "runtime login config"); err != nil {
		return err
	}
	if strings.TrimSpace(cfg.User) == "" || strings.TrimSpace(cfg.Pass) == "" {
		return fmt.Errorf("runtime login config is empty while login mode is enabled")
	}
	return nil
}

func newFreshSession(cfg sessionRuntimeConfig) *sessions.Session {
	store := sessions.NewCookieStore([]byte(cfg.CookieSecret))
	store.MaxAge(SESSION_MAX_AGE)
	fresh := sessions.NewSession(store, cfg.Name)
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
	cfg := getSessionRuntimeConfig(c)
	c.SetCookie(&http.Cookie{
		Name:     cfg.Name,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		Expires:  time.Unix(1, 0),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   c.Scheme() == "https",
	})
}

func recordSessionWarning(c *echo.Context, code string, detail string) {
	if c == nil {
		return
	}
	code = strings.TrimSpace(code)
	if code == "" {
		return
	}
	detail = strings.TrimSpace(detail)
	existing, _ := c.Get(sessionWarningContextKey).([]sessionWarning)
	for _, item := range existing {
		if item.Code == code {
			return
		}
	}
	c.Set(sessionWarningContextKey, append(existing, sessionWarning{Code: code, Detail: detail}))
}

func sessionWarningsForRequest(c *echo.Context) []sessionWarning {
	if c == nil {
		return nil
	}
	items, _ := c.Get(sessionWarningContextKey).([]sessionWarning)
	if len(items) == 0 {
		return nil
	}
	return items
}

func warningDetailSuffix(locale string, detail string) string {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return ""
	}
	if locale == "en" {
		return " Detail: " + detail
	}
	return " \u8be6\u7ec6\uff1a" + detail
}

func formatSessionWarning(locale string, item sessionWarning) string {
	switch item.Code {
	case sessionWarningCookieInvalid:
		if locale == "en" {
			return "Login session cookie was unreadable and has been cleared. Please sign in again before opening protected pages." + warningDetailSuffix(locale, item.Detail)
		}
		return "\u767b\u5f55\u4f1a\u8bdd Cookie \u65e0\u6cd5\u89e3\u6790\uff0c\u5df2\u81ea\u52a8\u6e05\u9664\u3002\u5982\u9700\u8bbf\u95ee\u53d7\u4fdd\u62a4\u9875\u9762\uff0c\u8bf7\u91cd\u65b0\u767b\u5f55\u3002" + warningDetailSuffix(locale, item.Detail)
	case sessionWarningSessionInvalid:
		if locale == "en" {
			return "Login session data was invalid and has been cleared. Please sign in again before opening protected pages." + warningDetailSuffix(locale, item.Detail)
		}
		return "\u767b\u5f55\u4f1a\u8bdd\u6570\u636e\u5f02\u5e38\uff0c\u5df2\u81ea\u52a8\u6e05\u9664\u3002\u5982\u9700\u8bbf\u95ee\u53d7\u4fdd\u62a4\u9875\u9762\uff0c\u8bf7\u91cd\u65b0\u767b\u5f55\u3002" + warningDetailSuffix(locale, item.Detail)
	default:
		return ""
	}
}

func AppendSessionWarnings(c *echo.Context, locale string, warnings []string) []string {
	items := make([]sessionWarning, 0, 2)
	items = append(items, sessionWarningsForRequest(c)...)
	if c != nil {
		if queryCode := strings.TrimSpace(c.QueryParam(sessionWarningQueryParam)); queryCode != "" {
			items = append(items, sessionWarning{Code: queryCode})
		}
	}
	if len(items) == 0 {
		return warnings
	}
	seen := make(map[string]bool, len(items))
	for _, item := range items {
		if seen[item.Code] {
			continue
		}
		seen[item.Code] = true
		if text := formatSessionWarning(locale, item); text != "" {
			warnings = append(warnings, text)
		}
	}
	return warnings
}

func sessionWarningRedirectTarget(c *echo.Context, target string) string {
	items := sessionWarningsForRequest(c)
	if len(items) == 0 || strings.TrimSpace(target) == "" {
		return target
	}
	parsed, err := url.Parse(target)
	if err != nil {
		return target
	}
	query := parsed.Query()
	if query.Get(sessionWarningQueryParam) == "" {
		query.Set(sessionWarningQueryParam, items[0].Code)
	}
	parsed.RawQuery = query.Encode()
	return parsed.String()
}

func getSession(c *echo.Context) (*sessions.Session, error) {
	cfg := getSessionRuntimeConfig(c)
	sess, err := sessionGet(cfg.Name, c)
	if err == nil {
		return sess, nil
	}

	if isDecodeSessionError(err) {
		clearSessionCookie(c)
		recordSessionWarning(c, sessionWarningCookieInvalid, err.Error())
		return newFreshSession(cfg), nil
	}

	return nil, err
}

func bindAuthStatusOptions(c *echo.Context) error {
	if err := statuspage.BindCurrentOptions(c); err != nil {
		return fmt.Errorf("settings config error: %w", err)
	}
	return nil
}

func authOptionsLoadDetail(c *echo.Context, err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}

func appendAuthPageDetail(page statuspage.Page, detail string) statuspage.Page {
	detail = strings.TrimSpace(detail)
	if detail == "" {
		return page
	}
	if strings.Contains(page.Detail, detail) {
		return page
	}
	if strings.TrimSpace(page.Detail) == "" {
		page.Detail = detail
		return page
	}
	page.Detail = strings.TrimSpace(page.Detail) + " " + detail
	return page
}

func renderAuthStatusPage(c *echo.Context, status int, page statuspage.Page, optionsErr error) error {
	page = appendAuthPageDetail(page, authOptionsLoadDetail(c, optionsErr))
	return statuspage.HTML(c, status, page)
}

func renderAuthHTTPError(c *echo.Context, status int, primary error, optionsErr error) error {
	message := ""
	if primary != nil {
		message = strings.TrimSpace(primary.Error())
	}
	detail := authOptionsLoadDetail(c, optionsErr)
	if detail != "" && !strings.Contains(message, detail) {
		if message == "" {
			message = detail
		} else {
			message += "; " + detail
		}
	}
	return statuspage.HTML(c, status, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), status, message))
}

func AuthRequired(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c *echo.Context) error {
		if IsLoginDisabled(c) {
			return next(c)
		}
		optionsErr := bindAuthStatusOptions(c)
		state, err := ResolveLoginDisplayState(c)
		if err != nil {
			clearSessionCookie(c)
			return renderAuthHTTPError(c, http.StatusInternalServerError, err, optionsErr)
		}
		if !state.ShowLoginInfo {
			return c.Redirect(http.StatusFound, sessionWarningRedirectTarget(c, define.SettingPages.Others.Path))
		}
		return next(c)
	}
}

func login(c *echo.Context) error {
	optionsErr := bindAuthStatusOptions(c)
	sess, err := getSession(c)
	if err != nil {
		log.Println("failed to get login session:", err)
		return renderAuthStatusPage(c, http.StatusInternalServerError, statuspage.BuildAuthSessionSaveErrorPage(statuspage.CurrentLocale(c)), optionsErr)
	}
	username := c.FormValue("username")
	password := c.FormValue("password")

	if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
		return renderAuthStatusPage(c, http.StatusBadRequest, statuspage.BuildAuthEmptyCredentialsPage(statuspage.CurrentLocale(c)), optionsErr)
	}

	loginCfg := SnapshotLoginRuntimeConfigForRequest(c)
	if err := validateRuntimeLoginConfig(loginCfg); err != nil {
		log.Println("invalid runtime login config:", err)
		return renderAuthHTTPError(c, http.StatusInternalServerError, err, optionsErr)
	}
	if subtle.ConstantTimeCompare([]byte(username), []byte(loginCfg.User)) != 1 ||
		subtle.ConstantTimeCompare([]byte(password), []byte(loginCfg.Pass)) != 1 {
		return renderAuthStatusPage(c, http.StatusBadRequest, statuspage.BuildAuthInvalidCredentialsPage(statuspage.CurrentLocale(c)), optionsErr)
	}

	sess.Values[SESSION_KEY_USER_NAME] = username
	sess.Values[SESSION_KEY_LOGIN_DATE] = time.Now().Format("2006-01-02 15:04:05 MST")
	sess.Options = buildSessionOptions(c, SESSION_MAX_AGE)

	if err := persistSession(sess, c.Request(), c.Response()); err != nil {
		log.Println("failed to save login session:", err)
		return renderAuthStatusPage(c, http.StatusInternalServerError, statuspage.BuildAuthSessionSaveErrorPage(statuspage.CurrentLocale(c)), optionsErr)
	}

	return c.Redirect(http.StatusFound, define.SettingPages.Others.Path)
}

func logout(c *echo.Context) error {
	optionsErr := bindAuthStatusOptions(c)
	sess, err := getSession(c)
	if err != nil {
		log.Println("failed to get logout session:", err)
		return renderAuthStatusPage(c, http.StatusInternalServerError, statuspage.BuildAuthSessionSaveErrorPage(statuspage.CurrentLocale(c)), optionsErr)
	}
	sess.Values = map[interface{}]interface{}{}
	sess.Options = buildSessionOptions(c, -1)

	if err := persistSession(sess, c.Request(), c.Response()); err != nil {
		log.Println("failed to save logout session:", err)
		return renderAuthStatusPage(c, http.StatusInternalServerError, statuspage.BuildAuthSessionSaveErrorPage(statuspage.CurrentLocale(c)), optionsErr)
	}
	clearSessionCookie(c)
	return c.Redirect(http.StatusFound, define.SettingPages.Others.Path)
}
