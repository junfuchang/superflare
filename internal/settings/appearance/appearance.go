package appearance

import (
	"fmt"
	"html/template"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/footer"
	"github.com/junfuchang/superflare/internal/pool"
	"github.com/junfuchang/superflare/internal/resources/mdi"
	settingsroot "github.com/junfuchang/superflare/internal/settings"
	"github.com/junfuchang/superflare/internal/statuspage"
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
		HideWarningsButton       bool   `form:"hide-warnings-button"`
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
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, "missing form data"))
	}
	if strings.TrimSpace(body.IconMode) == "" {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, "missing icon mode value"))
	}
	if strings.TrimSpace(body.Locale) == "" {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, "missing locale value"))
	}
	categoryColor, err := settingsroot.ParseOptionalColor(body.BookmarkCategoryColor, "")
	if err != nil {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
	}
	siteIcon, err := normalizeOptionalSiteIcon(body.OptionSiteIcon)
	if err != nil {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
	}
	itemColor, err := settingsroot.ParseOptionalColor(body.BookmarkItemColor, "")
	if err != nil {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
	}
	iconMode, err := normalizeIconMode(body.IconMode)
	if err != nil {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
	}
	locale, err := normalizeLocaleOption(body.Locale)
	if err != nil {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
	}
	homeMaxColumns, err := settingsroot.ParseOptionalRangedInt(body.HomeMaxColumns, 0, 8, "home-max-columns")
	if err != nil {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
	}
	homeMaxWidth, err := settingsroot.ParseOptionalRangedInt(body.HomeMaxWidth, 0, 2400, "home-max-width")
	if err != nil {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
	}
	var update model.Application
	update.Title = body.OptionTitle
	update.Footer = footer.Sanitize(body.OptionFooter)
	update.SiteIcon = siteIcon
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
	update.BookmarkCategoryColor = categoryColor
	update.BookmarkItemColor = itemColor
	update.HideSettingsButton = body.HideSettingsButton
	update.HideHelpButton = body.HideHelpButton
	update.HideWarningsButton = body.HideWarningsButton
	update.EnableEncryptedLink = body.EnableEncryptedLink
	update.KeepLetterCase = body.KeepLetterCase
	update.IconMode = iconMode
	if locale != "" {
		update.Locale = locale
	}
	update.HomeMaxColumns = homeMaxColumns
	update.HomeMaxWidth = homeMaxWidth
	if err := data.UpdateAppearance(update); err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	if err := define.UpdatePagePalettes(); err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	return pageAppearance(c)
}

func normalizeOptionalSiteIcon(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", nil
	}
	if !mdi.IconExists(input) {
		return "", fmt.Errorf("invalid site icon value: %s", input)
	}
	return input, nil
}

func normalizeIconMode(input string) (string, error) {
	mode := strings.ToUpper(strings.TrimSpace(input))
	if mode == "" {
		return define.IconModeMissingFill, nil
	}
	switch mode {
	case define.IconModeMissingBlank, define.IconModeMissingFill, define.IconModeHidden:
		return mode, nil
	default:
		return "", fmt.Errorf("invalid icon mode: %s", input)
	}
}

func normalizeLocaleOption(input string) (string, error) {
	locale := strings.ToLower(strings.TrimSpace(input))
	if locale == "" {
		return "", nil
	}
	switch locale {
	case "zh", "en":
		return locale, nil
	default:
		return "", fmt.Errorf("invalid locale value: %s", input)
	}
}

func pageAppearance(c *echo.Context) error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		statuspage.BindOptionsLoadError(c, err)
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	statuspage.BindOptions(c, options)
	options, renderWarnings := statuspage.PrepareSettingsOptionsForRender(options)
	locale := options.Locale
	iconMode := strings.ToUpper(strings.TrimSpace(options.IconMode))
	if iconMode == "" {
		iconMode = define.IconModeMissingFill
	}
	showLoginInfo := false
	userName := ""
	loginDate := ""
	loginDisplay, err := auth.ResolveLoginDisplayStateForStrictView(c)
	if err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
	}
	showLoginInfo = loginDisplay.ShowLoginInfo
	userName = loginDisplay.UserName
	loginDate = loginDisplay.LoginDate
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
	m["DebugMode"] = settingsroot.CurrentRuntime().DebugMode
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["PageName"] = "Appearance"
	m["PageAppearance"] = pageStyle
	m["SettingPages"] = define.SettingPages
	m["ShowSettingsSidebar"] = true
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["ShowLoginInfo"] = showLoginInfo
	m["UserIsLogin"] = showLoginInfo
	m["UserName"] = userName
	m["LoginDate"] = loginDate
	m["OptionTitle"] = options.Title
	footer.BindTemplateData(m, options.Footer)
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
	m["OptionHideWarningsButton"] = options.HideWarningsButton
	m["OptionEnableEncryptedLink"] = options.EnableEncryptedLink
	m["OptionKeepLetterCase"] = options.KeepLetterCase
	m["OptionIconMode"] = iconMode
	m["OptionLocale"] = locale
	m["OptionHomeMaxColumns"] = options.HomeMaxColumns
	m["OptionHomeMaxWidth"] = options.HomeMaxWidth
	m["Locale"] = locale
	m["RenderWarnings"] = renderWarnings
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
