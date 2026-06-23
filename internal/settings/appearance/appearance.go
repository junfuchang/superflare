package appearance

import (
	"html/template"
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/pool"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

func RegisterRouting(e *echo.Echo) {
	e.GET(define.SettingPages.Appearance.Path, pageAppearance, auth.AuthRequired)
	e.POST(define.SettingPages.Appearance.Path, updateAppearanceOptions, auth.AuthRequired)
}

func updateAppearanceOptions(c *echo.Context) error {
	var body struct {
		OptionTitle              string `form:"title"`
		OptionFooter             string `form:"footer"`
		OptionSiteIcon           string `form:"site-icon"`
		OptionOpenAppNewTab      bool   `form:"open-app-newtab"`
		OptionOpenBookmarkNewTab bool   `form:"open-bookmark-newtab"`
		OptionShowTitle          bool   `form:"show-title"`
		OptionGreetings          string `form:"greetings"`
		OptionShowDateTime       bool   `form:"show-datetime"`
		OptionShowApps           bool   `form:"show-apps"`
		OptionShowBookmarks      bool   `form:"show-bookmarks"`
		OptionAppsTitle          string `form:"apps-title"`
		OptionBookmarksTitle     string `form:"bookmarks-title"`
		BookmarkCategoryColor    string `form:"bookmark-category-color"`
		BookmarkItemColor        string `form:"bookmark-item-color"`
		HideSettingsButton       bool   `form:"hide-settings-button"`
		HideHelpButton           bool   `form:"hide-help-button"`
		EnableEncryptedLink      bool   `form:"enable-encrypted-link"`
		IconMode                 string `form:"icon-mode"`
		KeepLetterCase           bool   `form:"keep-letter-case"`
		Locale                   string `form:"locale"`
		HomeMaxColumns           string `form:"home-max-columns"`
		HomeMaxWidth             string `form:"home-max-width"`
		OptionCustomDay          string `form:"custom-day"`
		OptionCustomMonth        string `form:"custom-month"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusForbidden, "提交数据缺失")
	}
	var update model.Application
	update.Title = body.OptionTitle
	update.Footer = body.OptionFooter
	update.SiteIcon = strings.TrimSpace(body.OptionSiteIcon)
	update.SiteIconMode = "mdi"
	update.OpenAppNewTab = body.OptionOpenAppNewTab
	update.OpenBookmarkNewTab = body.OptionOpenBookmarkNewTab
	update.ShowTitle = body.OptionShowTitle
	update.Greetings = body.OptionGreetings
	update.ShowDateTime = body.OptionShowDateTime
	update.ShowApps = body.OptionShowApps
	update.ShowBookmarks = body.OptionShowBookmarks
	update.AppsTitle = strings.TrimSpace(body.OptionAppsTitle)
	update.BookmarksTitle = strings.TrimSpace(body.OptionBookmarksTitle)
	update.BookmarkCategoryColor = define.SafeCSSColor(body.BookmarkCategoryColor, "")
	update.BookmarkItemColor = define.SafeCSSColor(body.BookmarkItemColor, "")
	update.HideSettingsButton = body.HideSettingsButton
	update.HideHelpButton = body.HideHelpButton
	update.EnableEncryptedLink = body.EnableEncryptedLink
	update.KeepLetterCase = body.KeepLetterCase
	requestIconMode := strings.ToUpper(body.IconMode)
	if requestIconMode != "DEFAULT" && requestIconMode != "FILLING" {
		update.IconMode = "DEFAULT"
	} else {
		update.IconMode = requestIconMode
	}
	if body.Locale != "" {
		update.Locale = body.Locale
	}
	update.HomeMaxColumns = clampFormInt(body.HomeMaxColumns, 0, 8, 0)
	update.HomeMaxWidth = clampFormInt(body.HomeMaxWidth, 0, 2400, 0)
	data.UpdateAppearance(update)
	define.UpdatePagePalettes()
	return pageAppearance(c)
}

func clampFormInt(input string, min int, max int, fallback int) int {
	value, err := strconv.Atoi(strings.TrimSpace(input))
	if err != nil {
		return fallback
	}
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func pageAppearance(c *echo.Context) error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		return c.String(http.StatusInternalServerError, "config error")
	}
	IconModeDefault := options.IconMode == "DEFAULT"
	IconModeFilling := options.IconMode == "FILLING"
	showLoginInfo := false
	if !define.AppFlags.DisableLoginMode {
		showLoginInfo = auth.CheckUserIsLogin(c)
	}
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["DebugMode"] = define.AppFlags.DebugMode
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["PageName"] = "Appearance"
	m["PageAppearance"] = define.GetAppBodyStyle()
	m["SettingPages"] = define.SettingPages
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["ShowLoginInfo"] = showLoginInfo
	m["UserIsLogin"] = showLoginInfo
	m["UserName"] = auth.GetUserName(c)
	m["LoginDate"] = auth.GetUserLoginDate(c)
	m["OptionTitle"] = options.Title
	m["OptionFooter"] = template.HTML(options.Footer)
	m["OptionSiteIcon"] = options.SiteIcon
	m["SiteIconOptions"] = renderSiteIconOptions()
	m["OptionOpenAppNewTab"] = options.OpenAppNewTab
	m["OptionOpenBookmarkNewTab"] = options.OpenBookmarkNewTab
	m["OptionShowTitle"] = options.ShowTitle
	m["OptionGreetings"] = options.Greetings
	m["OptionShowDateTime"] = options.ShowDateTime
	m["OptionShowApps"] = options.ShowApps
	m["OptionShowBookmarks"] = options.ShowBookmarks
	m["OptionAppsTitle"] = options.AppsTitle
	m["OptionBookmarksTitle"] = options.BookmarksTitle
	m["OptionBookmarkCategoryColor"] = options.BookmarkCategoryColor
	m["OptionBookmarkItemColor"] = options.BookmarkItemColor
	m["OptionHideSettingsButton"] = options.HideSettingsButton
	m["OptionHideHelpButton"] = options.HideHelpButton
	m["OptionEnableEncryptedLink"] = options.EnableEncryptedLink
	m["OptionKeepLetterCase"] = options.KeepLetterCase
	m["OptionIconModeDefault"] = IconModeDefault
	m["OptionIconModeFilling"] = IconModeFilling
	m["OptionLocale"] = options.Locale
	m["OptionHomeMaxColumns"] = options.HomeMaxColumns
	m["OptionHomeMaxWidth"] = options.HomeMaxWidth
	m["Locale"] = options.Locale
	return c.Render(http.StatusOK, "settings.html", m)
}

func renderSiteIconOptions() template.HTML {
	var b strings.Builder
	for _, name := range mdi.IconNames() {
		b.WriteString(`<option value="`)
		b.WriteString(template.HTMLEscapeString(name))
		b.WriteString(`"></option>`)
	}
	return template.HTML(b.String())
}
