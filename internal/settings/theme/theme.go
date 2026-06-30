package theme

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/background"
	"github.com/junfuchang/superflare/internal/pool"
	settingsroot "github.com/junfuchang/superflare/internal/settings"
	"github.com/junfuchang/superflare/internal/statuspage"
)

var beginUploadedBackgroundActivation = background.BeginStagedUploadedBackgroundActivation
var discardUploadedBackgroundStage = background.DiscardStagedUploadedBackgrounds

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
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, "missing form data"))
	}
	if strings.TrimSpace(body.Theme) == "" {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, "missing theme value"))
	}
	themeName, err := normalizeThemeName(body.Theme)
	if err != nil {
		return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
	}
	var (
		backgroundColor   string
		primaryColor      string
		accentColor       string
		backgroundBlur    int
		backgroundOpacity int
		glassEffect       string
		glassIntensity    int
	)
	if themeName == "custom" {
		backgroundColor, err = settingsroot.ParseOptionalColor(body.CustomThemeBackground, "custom-theme-background")
		if err != nil {
			return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
		}
		primaryColor, err = settingsroot.ParseOptionalColor(body.CustomThemePrimary, "custom-theme-primary")
		if err != nil {
			return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
		}
		accentColor, err = settingsroot.ParseOptionalColor(body.CustomThemeAccent, "custom-theme-accent")
		if err != nil {
			return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
		}
		if body.BackgroundBlur != "" {
			backgroundBlur, err = settingsroot.ParseOptionalRangedInt(body.BackgroundBlur, 0, 80, "background-blur")
			if err != nil {
				return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
			}
		}
		if body.BackgroundOpacity != "" {
			backgroundOpacity, err = settingsroot.ParseOptionalRangedInt(body.BackgroundOpacity, 0, 100, "background-opacity")
			if err != nil {
				return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
			}
		}
		if body.GlassEffect != "" {
			glassEffect, err = normalizeGlassEffect(body.GlassEffect)
			if err != nil {
				return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
			}
		}
		if body.GlassIntensity != "" {
			glassIntensity, err = settingsroot.ParseOptionalRangedInt(body.GlassIntensity, 0, 100, "glass-intensity")
			if err != nil {
				return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, err.Error()))
			}
		}
	}
	if themeName == "custom" && strings.TrimSpace(body.Action) == "custom-theme" {
		currentOptions, err := data.GetAllSettingsOptions()
		if err != nil {
			statuspage.BindOptionsLoadError(c, err)
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
		}
		statuspage.BindOptions(c, currentOptions)
		locale := currentOptions.Locale
		hasBackgroundImageField := hasFormField(c, "background-image")
		backgroundImage := strings.TrimSpace(body.BackgroundImage)
		backgroundImageMode := "url"
		if !hasBackgroundImageField {
			backgroundImage = currentOptions.BackgroundImage
			backgroundImageMode = currentOptions.BackgroundImageMode
		}
		update := model.Application{
			Theme:                 themeName,
			ThemeBase:             resolveThemeBase(currentOptions),
			CustomThemeBackground: backgroundColor,
			CustomThemePrimary:    primaryColor,
			CustomThemeAccent:     accentColor,
			BackgroundImage:       backgroundImage,
			BackgroundImageMode:   backgroundImageMode,
			BackgroundBlur:        backgroundBlur,
			BackgroundOpacity:     backgroundOpacity,
			GlassEffect:           glassEffect,
			GlassIntensity:        glassIntensity,
		}
		if strings.HasPrefix(update.BackgroundImage, "/user-assets/") {
			update.BackgroundImageMode = "upload"
		}
		stagedUpload := false
		uploadedBackground, uploadErr := saveUploadedBackground(c)
		if uploadErr != nil {
			return statuspage.HTML(c, http.StatusBadRequest, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadRequest, uploadErr.Error()))
		}
		if uploadedBackground != "" {
			stagedUpload = true
			update.BackgroundImage = uploadedBackground
			update.BackgroundImageMode = "upload"
		}
		if update.BackgroundImage == "" {
			update.BackgroundImageMode = "url"
		}
		var activation *background.StagedUploadedBackgroundActivation
		if stagedUpload {
			activation, err = beginUploadedBackgroundActivation()
			if err != nil {
				if discardErr := discardUploadedBackgroundStage(); discardErr != nil {
					log.Printf("discard staged background after failed activation: %v", discardErr)
				}
				log.Printf("begin staged background activation failed: %v", err)
				return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
			}
		}
		if err := data.UpdateThemeAndBackgroundSettings(update); err != nil {
			if activation != nil {
				if err := activation.Rollback(); err != nil {
					log.Printf("rollback activated background after failed config save: %v", err)
				}
			} else if stagedUpload {
				if err := discardUploadedBackgroundStage(); err != nil {
					log.Printf("discard staged background after failed config save: %v", err)
				}
			}
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(locale, http.StatusInternalServerError, err.Error()))
		}
		if activation != nil {
			if err := activation.Commit(); err != nil {
				log.Printf("commit activated background backup cleanup failed: %v", err)
			}
		}
		if err := background.DeleteStaleAssets(
			currentOptions.BackgroundImage,
			currentOptions.BackgroundImageMode,
			update.BackgroundImage,
			update.BackgroundImageMode,
		); err != nil {
			log.Printf("delete stale background assets failed: %v", err)
		}
	} else if themeName == "custom" {
		if err := data.UpdateThemeSettings(
			themeName,
			"",
			backgroundColor,
			primaryColor,
			accentColor,
		); err != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
		}
	} else {
		if err := data.UpdateThemeName(themeName); err != nil {
			return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
		}
	}
	if err := define.UpdatePagePalettes(); err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	return pageTheme(c)
}

func saveUploadedBackground(c *echo.Context) (string, error) {
	file, err := c.FormFile("background-file")
	if err != nil {
		if errors.Is(err, http.ErrMissingFile) {
			return "", nil
		}
		if errors.Is(err, http.ErrNotMultipart) || errors.Is(err, http.ErrMissingBoundary) {
			return "", nil
		}
		return "", echo.NewHTTPError(http.StatusBadRequest, "parse background upload failed: "+err.Error())
	}
	src, err := file.Open()
	if err != nil {
		return "", err
	}
	defer src.Close()
	savedPath, err := background.PrepareUploadedBackgroundStage(file.Filename, src)
	if err != nil {
		return "", echo.NewHTTPError(http.StatusBadRequest, err.Error())
	}
	return savedPath, nil
}

func normalizeThemeName(input string) (string, error) {
	themeName := strings.TrimSpace(input)
	if themeName == "" {
		return "", fmt.Errorf("missing theme value")
	}
	if themeName == "custom" {
		return themeName, nil
	}
	for _, themePresent := range define.ThemePalettes {
		if themePresent.Name == themeName {
			return themeName, nil
		}
	}
	return "", fmt.Errorf("invalid theme value: %s", input)
}

func normalizeGlassEffect(input string) (string, error) {
	input = strings.ToLower(strings.TrimSpace(input))
	if input == "" {
		return "none", nil
	}
	switch strings.ToLower(strings.TrimSpace(input)) {
	case "frosted", "liquid":
		return input, nil
	case "none":
		return "none", nil
	default:
		return "", fmt.Errorf("invalid glass-effect value: %s", input)
	}
}

func resolveThemeBase(options model.Application) string {
	themeBase := strings.TrimSpace(options.ThemeBase)
	if themeBase != "" && !strings.EqualFold(themeBase, "custom") {
		return strings.ToLower(themeBase)
	}
	themeName := strings.ToLower(strings.TrimSpace(options.Theme))
	if themeName != "" && themeName != "custom" {
		return themeName
	}
	return "blackboard"
}

func hasFormField(c *echo.Context, field string) bool {
	if c == nil || c.Request() == nil {
		return false
	}
	req := c.Request()
	if req.Form == nil {
		if err := req.ParseForm(); err != nil {
			return false
		}
	}
	if req.Form == nil {
		return false
	}
	_, ok := req.Form[field]
	return ok
}

func pageTheme(c *echo.Context) error {
	themes := define.ThemePalettes
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		statuspage.BindOptionsLoadError(c, err)
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	statuspage.BindOptions(c, options)
	options, renderWarnings := statuspage.PrepareSettingsOptionsForRender(options)
	locale := options.Locale
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
	m["Locale"] = locale
	m["DebugMode"] = settingsroot.CurrentRuntime().DebugMode
	m["PageInlineStyle"] = define.GetPageInlineStyle()
	m["PageAppearance"] = pageStyle
	m["SettingsURI"] = define.RegularPages.Settings.Path
	m["PageName"] = "Theme"
	m["SettingPages"] = define.SettingPages
	m["ShowSettingsSidebar"] = true
	m["Themes"] = themes
	m["OptionTitle"] = options.Title
	m["OptionSiteIcon"] = options.SiteIcon
	m["ShowLoginInfo"] = showLoginInfo
	m["UserIsLogin"] = showLoginInfo
	m["UserName"] = userName
	m["LoginDate"] = loginDate
	m["OptionTheme"] = options.Theme
	if options.Theme == "custom" {
		m["CustomThemeBackground"] = options.CustomThemeBackground
		m["CustomThemePrimary"] = options.CustomThemePrimary
		m["CustomThemeAccent"] = options.CustomThemeAccent
	} else {
		m["CustomThemeBackground"] = ""
		m["CustomThemePrimary"] = ""
		m["CustomThemeAccent"] = ""
	}
	m["OptionBackgroundImage"] = options.BackgroundImage
	m["OptionBackgroundBlur"] = options.BackgroundBlur
	m["OptionBackgroundOpacity"] = options.BackgroundOpacity
	m["OptionGlassEffect"] = options.GlassEffect
	m["OptionGlassIntensity"] = options.GlassIntensity
	m["RenderWarnings"] = renderWarnings
	return c.Render(http.StatusOK, "settings.html", m)
}
