package others

import (
	"html/template"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/pool"
	version "github.com/soulteary/version-kit"
)

func RegisterRouting(e *echo.Echo) {
	e.GET(define.SettingPages.Others.Path, pageOthers)
	e.POST(define.SettingPages.Others.Path, updateLoginOptions, auth.AuthRequired)
}

func updateLoginOptions(c *echo.Context) error {
	var body struct {
		LoginUser        string `form:"login-user"`
		LoginPass        string `form:"login-pass"`
		LoginPassConfirm string `form:"login-pass-confirm"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusForbidden, "提交数据缺失")
	}
	user := strings.TrimSpace(body.LoginUser)
	pass := strings.TrimSpace(body.LoginPass)
	confirm := strings.TrimSpace(body.LoginPassConfirm)
	if pass != confirm {
		return renderOthers(c, "login_pass_confirm_error")
	}
	data.UpdateLoginConfig(user, pass)
	applyRuntimeLoginConfig(user, pass)
	return pageOthers(c)
}

func applyRuntimeLoginConfig(user string, pass string) {
	next := define.AppBaseFlags
	if next.Port == 0 {
		next = define.AppFlags
	}
	if user != "" {
		next.User = user
		next.UserIsGenerated = false
	}
	if pass != "" {
		next.Pass = pass
		next.PassIsGenerated = false
	}
	define.AppFlags.User = next.User
	define.AppFlags.Pass = next.Pass
	define.AppFlags.UserIsGenerated = next.UserIsGenerated
	define.AppFlags.PassIsGenerated = next.PassIsGenerated
}

func pageOthers(c *echo.Context) error {
	return renderOthers(c, "")
}

func renderOthers(c *echo.Context, loginConfigError string) error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		return c.String(http.StatusInternalServerError, "config error")
	}
	locale := options.Locale
	if locale == "" {
		locale = "zh"
	}
	isLogined := false
	if !define.AppFlags.DisableLoginMode {
		isLogined = auth.CheckUserIsLogin(c)
	}
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["Locale"] = locale
	m["DebugMode"] = define.AppFlags.DebugMode
	m["DisableLoginMode"] = define.AppFlags.DisableLoginMode
	m["UserIsLogin"] = isLogined
	m["ShowLoginInfo"] = isLogined
	m["UserName"] = auth.GetUserName(c)
	m["LoginDate"] = auth.GetUserLoginDate(c)
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["PageAppearance"] = define.GetAppBodyStyle()
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["LoginURI"] = define.MiscPages.Login.Path
	m["LogoutURI"] = define.MiscPages.Logout.Path
	m["PageName"] = "Others"
	m["SettingPages"] = define.SettingPages
	m["OptionTitle"] = options.Title
	m["OptionSiteIcon"] = options.SiteIcon
	m["OptionLoginUser"] = options.LoginUser
	m["OptionLoginPass"] = options.LoginPass
	m["LoginConfigError"] = loginConfigError
	m["Version"] = version.Version
	m["BuildDate"] = version.BuildDate
	m["COMMIT"] = version.Commit
	m["OptionFooter"] = template.HTML(options.Footer)
	return c.Render(http.StatusOK, "settings.html", m)
}
