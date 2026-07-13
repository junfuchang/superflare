package others

import (
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/appver"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/footer"
	"github.com/junfuchang/superflare/internal/pool"
	settingsroot "github.com/junfuchang/superflare/internal/settings"
	"github.com/junfuchang/superflare/internal/statuspage"
)

func RegisterRouting(e *echo.Echo) {
	e.GET(define.SettingPages.Others.Path, pageOthers)
	e.POST(define.SettingPages.Others.Path, updateLoginOptions, auth.AuthRequired)
}

func updateLoginOptions(c *echo.Context) error {
	if auth.IsLoginDisabled(c) {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	var body struct {
		LoginUser        string `form:"login-user"`
		LoginPass        string `form:"login-pass"`
		LoginPassConfirm string `form:"login-pass-confirm"`
	}
	if err := c.Bind(&body); err != nil {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, "missing form data"))
	}
	user := strings.TrimSpace(body.LoginUser)
	pass := strings.TrimSpace(body.LoginPass)
	confirm := strings.TrimSpace(body.LoginPassConfirm)
	if user == "" {
		return renderOthers(c, "login_user_required_error")
	}
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	currentLoginConfig, err := resolveCurrentLoginConfig(c, statuspage.CurrentLocale(c), options)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	if pass == "" && confirm == "" {
		pass = strings.TrimSpace(currentLoginConfig.Pass)
		confirm = pass
	}
	if pass != confirm {
		return renderOthers(c, "login_pass_confirm_error")
	}
	if err := data.UpdateLoginConfig(user, pass); err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	applyRuntimeLoginConfig(c, user, pass)
	return pageOthers(c)
}

func applyRuntimeLoginConfig(c *echo.Context, user string, pass string) {
	next := define.BaseAppRuntimeFlags()
	if user != "" {
		next.User = user
		next.UserIsGenerated = false
	}
	if pass != "" {
		next.Pass = pass
		next.PassIsGenerated = false
	}
	auth.StoreLoginRuntimeConfig(auth.SnapshotLoginRuntimeConfigFromFlags(next))
	auth.StoreLoginRuntimeConfigForRequest(c, auth.SnapshotLoginRuntimeConfigFromFlags(next))
}

func pageOthers(c *echo.Context) error {
	return renderOthers(c, "")
}

type resolvedLoginConfig struct {
	User          string
	Pass          string
	WarningKey    string
	WarningDetail string
}

func renderOthers(c *echo.Context, loginConfigError string) error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		statuspage.BindOptionsLoadError(c, err)
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	statuspage.BindOptions(c, options)
	options, renderWarnings := statuspage.PrepareSettingsOptionsForRender(options)
	locale := options.Locale
	disableLoginMode := auth.IsLoginDisabled(c)
	isLogined := false
	userName := ""
	loginDate := ""
	loginDisplay, err := auth.ResolveLoginDisplayStateForStrictView(c)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	isLogined = loginDisplay.ShowLoginInfo
	userName = loginDisplay.UserName
	loginDate = loginDisplay.LoginDate
	canManageSettings := disableLoginMode || isLogined
	canConfigureLogin := isLogined && !disableLoginMode
	renderWarnings = auth.AppendSessionWarnings(c, locale, renderWarnings)
	pageStyle, styleWarning, err := statuspage.RequireConfiguredBodyStyleForRender(locale, "settings")
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	if styleWarning != "" {
		renderWarnings = append(renderWarnings, styleWarning)
	}
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["Locale"] = locale
	m["DebugMode"] = settingsroot.CurrentRuntime().DebugMode
	m["DisableLoginMode"] = disableLoginMode
	m["UserIsLogin"] = canManageSettings
	m["ShowLoginInfo"] = isLogined
	m["UserName"] = userName
	m["LoginDate"] = loginDate
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["PageAppearance"] = pageStyle
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["LoginURI"] = define.MiscPages.Login.Path
	m["LogoutURI"] = define.MiscPages.Logout.Path
	m["PageName"] = "Others"
	m["SettingPages"] = define.SettingPages
	m["ShowSettingsSidebar"] = canManageSettings
	m["ShowPortsSettings"] = !disableLoginMode
	m["CanConfigureLogin"] = canConfigureLogin
	if canManageSettings {
		m["OthersPageMode"] = "settings"
	} else {
		m["OthersPageMode"] = "login"
	}
	m["OptionTitle"] = options.Title
	m["OptionSiteIcon"] = options.SiteIcon
	m["OptionLoginUser"] = ""
	m["DefaultLoginCredentialsActive"] = false
	m["LoginConfigError"] = loginConfigError
	m["LoginConfigErrorDetail"] = ""
	if canConfigureLogin {
		currentLoginConfig, err := resolveCurrentLoginConfig(c, locale, options)
		if err != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
		}
		if loginConfigError == "" && currentLoginConfig.WarningKey != "" {
			loginConfigError = currentLoginConfig.WarningKey
		}
		m["OptionLoginUser"] = currentLoginConfig.User
		m["DefaultLoginCredentialsActive"] = loginCredentialsAreDefault(currentLoginConfig.User, currentLoginConfig.Pass)
		m["LoginConfigError"] = loginConfigError
		m["LoginConfigErrorDetail"] = currentLoginConfig.WarningDetail
	}
	m["Version"] = appver.DisplayVersion()
	footer.BindTemplateData(m, options.Footer)
	m["RenderWarnings"] = renderWarnings
	return c.Render(http.StatusOK, "settings.html", m)
}

func loginCredentialsAreDefault(user string, pass string) bool {
	return strings.TrimSpace(user) == define.DEFAULT_LOGIN_USER && strings.TrimSpace(pass) == define.DEFAULT_LOGIN_PASS
}

func resolveCurrentLoginConfig(c *echo.Context, locale string, options model.Application) (resolvedLoginConfig, error) {
	runtimeCfg := auth.SnapshotLoginRuntimeConfigForRequest(c)
	runtimeUser := strings.TrimSpace(runtimeCfg.User)
	runtimePass := strings.TrimSpace(runtimeCfg.Pass)

	user, pass, err := data.GetLoginConfig()
	if err != nil {
		if runtimeUser == "" && runtimePass == "" {
			return resolvedLoginConfig{}, err
		}
		log.Printf("fallback to request runtime login config after persistent login config read failed: %v", err)
		if strings.TrimSpace(user) == "" {
			user = runtimeUser
		}
		if strings.TrimSpace(pass) == "" {
			pass = runtimePass
		}
		return resolvedLoginConfig{
			User:          strings.TrimSpace(user),
			Pass:          strings.TrimSpace(pass),
			WarningKey:    "login_config_runtime_fallback",
			WarningDetail: formatLoginConfigFallbackDetail(locale, err),
		}, nil
	}

	if strings.TrimSpace(user) == "" && runtimeUser != "" {
		user = runtimeUser
	}
	if strings.TrimSpace(pass) == "" && runtimePass != "" {
		pass = runtimePass
	}
	warningKey := ""
	if shouldWarnRuntimeLoginSource(options, runtimeUser, runtimePass, user, pass) {
		warningKey = "login_config_runtime_source"
	}
	return resolvedLoginConfig{
		User:       strings.TrimSpace(user),
		Pass:       strings.TrimSpace(pass),
		WarningKey: warningKey,
	}, nil
}

func shouldWarnRuntimeLoginSource(options model.Application, runtimeUser string, runtimePass string, user string, pass string) bool {
	if strings.TrimSpace(options.LoginUser) != "" || strings.TrimSpace(options.LoginPass) != "" {
		return false
	}
	runtimeUser = strings.TrimSpace(runtimeUser)
	runtimePass = strings.TrimSpace(runtimePass)
	if runtimeUser == "" || runtimePass == "" {
		return false
	}
	return strings.TrimSpace(user) == runtimeUser && strings.TrimSpace(pass) == runtimePass
}

func formatLoginConfigFallbackDetail(locale string, err error) string {
	if err == nil {
		return ""
	}
	detail := strings.TrimSpace(err.Error())
	if detail == "" {
		return ""
	}
	if locale == "en" {
		return "Current read error: " + detail
	}
	return "当前读取错误: " + detail
}
