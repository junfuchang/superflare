package theme

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/background"
	"github.com/junfuchang/superflare/internal/pool"
)

func RegisterRouting(e *echo.Echo) {
	e.GET(define.SettingPages.Theme.Path, pageTheme, auth.AuthRequired)
	e.POST(define.SettingPages.Theme.Path, updateThemes, auth.AuthRequired)
}

func updateThemes(c *echo.Context) error {
	var body struct {
		Action                string `form:"action"`
		Theme                 string `form:"theme"`
		CustomThemeBackground string `form:"custom-theme-background"`
		CustomThemePrimary    string `form:"custom-theme-primary"`
		CustomThemeAccent     string `form:"custom-theme-accent"`
		BackgroundImage       string `form:"background-image"`
		BackgroundBlur        string `form:"background-blur"`
		BackgroundOpacity     string `form:"background-opacity"`
		GlassEffect           string `form:"glass-effect"`
		GlassIntensity        string `form:"glass-intensity"`
	}
	if err := c.Bind(&body); err != nil {
		return c.JSON(http.StatusForbidden, "missing form data")
	}
	themeName := strings.TrimSpace(body.Theme)
	if themeName == "" {
		themeName = "blackboard"
	}
	if themeName == "custom" && strings.TrimSpace(body.Action) == "custom-theme" {
		currentOptions, err := data.GetAllSettingsOptions()
		if err != nil {
			return c.String(http.StatusInternalServerError, "config error")
		}
		update := model.Application{
			Theme:                 themeName,
			CustomThemeBackground: define.SafeCSSColor(body.CustomThemeBackground, "rgba(26, 26, 26, 1)"),
			CustomThemePrimary:    define.SafeCSSColor(body.CustomThemePrimary, "rgba(255, 253, 234, 1)"),
			CustomThemeAccent:     define.SafeCSSColor(body.CustomThemeAccent, "rgba(92, 92, 92, 1)"),
			BackgroundImage:       strings.TrimSpace(body.BackgroundImage),
			BackgroundImageMode:   "url",
			BackgroundBlur:        clampFormInt(body.BackgroundBlur, 0, 80, 0),
			BackgroundOpacity:     clampFormInt(body.BackgroundOpacity, 0, 100, 100),
			GlassEffect:           normalizeGlassEffect(body.GlassEffect),
			GlassIntensity:        clampFormInt(body.GlassIntensity, 0, 100, 0),
		}
		if strings.HasPrefix(update.BackgroundImage, "/user-assets/") {
			update.BackgroundImageMode = "upload"
		}
		uploadedBackground, uploadErr := saveUploadedBackground(c)
		if uploadErr != nil {
			return uploadErr
		}
		if uploadedBackground != "" {
			update.BackgroundImage = uploadedBackground
			update.BackgroundImageMode = "upload"
		}
		if update.BackgroundImage == "" {
			update.BackgroundImageMode = "url"
		}
		if err := background.DeleteStaleAssets(
			currentOptions.BackgroundImage,
			currentOptions.BackgroundImageMode,
			update.BackgroundImage,
			update.BackgroundImageMode,
		); err != nil {
			return err
		}
		data.UpdateThemeAndBackgroundSettings(update)
	} else if themeName == "custom" {
		data.UpdateThemeSettings(
			themeName,
			define.SafeCSSColor(body.CustomThemeBackground, "rgba(26, 26, 26, 1)"),
			define.SafeCSSColor(body.CustomThemePrimary, "rgba(255, 253, 234, 1)"),
			define.SafeCSSColor(body.CustomThemeAccent, "rgba(92, 92, 92, 1)"),
		)
	} else {
		data.UpdateThemeName(themeName)
	}
	define.UpdatePagePalettes()
	return pageTheme(c)
}

func saveUploadedBackground(c *echo.Context) (string, error) {
	file, err := c.FormFile("background-file")
	if err != nil {
		return "", nil
	}
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	savedPath, err := background.PrepareUploadedBackground(file.Filename, src)
	if err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return savedPath, nil
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

func normalizeGlassEffect(input string) string {
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "frosted", "liquid":
		return strings.ToLower(strings.TrimSpace(input))
	default:
		return "none"
	}
}

func pageTheme(c *echo.Context) error {
	themes := define.ThemePalettes
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		return c.String(http.StatusInternalServerError, "config error")
	}
	locale := options.Locale
	if locale == "" {
		locale = "zh"
	}
	showLoginInfo := false
	if !define.AppFlags.DisableLoginMode {
		showLoginInfo = auth.CheckUserIsLogin(c)
	}
	m := pool.GetTemplateMap()
	defer pool.PutTemplateMap(m)
	m["Locale"] = locale
	m["DebugMode"] = define.AppFlags.DebugMode
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["PageAppearance"] = define.GetAppBodyStyle()
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["PageName"] = "Theme"
	m["SettingPages"] = define.SettingPages
	m["Themes"] = themes
	m["OptionTitle"] = options.Title
	m["OptionSiteIcon"] = options.SiteIcon
	m["ShowLoginInfo"] = showLoginInfo
	m["UserIsLogin"] = showLoginInfo
	m["UserName"] = auth.GetUserName(c)
	m["LoginDate"] = auth.GetUserLoginDate(c)
	m["OptionTheme"] = options.Theme
	m["CustomThemeBackground"] = options.CustomThemeBackground
	m["CustomThemePrimary"] = options.CustomThemePrimary
	m["CustomThemeAccent"] = options.CustomThemeAccent
	m["OptionBackgroundImage"] = options.BackgroundImage
	m["OptionBackgroundBlur"] = options.BackgroundBlur
	m["OptionBackgroundOpacity"] = options.BackgroundOpacity
	m["OptionGlassEffect"] = options.GlassEffect
	m["OptionGlassIntensity"] = options.GlassIntensity
	return c.Render(http.StatusOK, "settings.html", m)
}
