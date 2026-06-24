package data

import (
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/i18n"
)

func GetThemeName() string {
	opts, err := GetAllSettingsOptions()
	if err != nil {
		return ""
	}
	return opts.Theme
}

func UpdateThemeName(theme string) bool {
	return UpdateThemeSettings(theme, "", "", "")
}

func UpdateThemeSettings(theme string, background string, primary string, accent string) bool {
	options, err := GetAllSettingsOptions()
	if err != nil {
		return false
	}
	options.Theme = theme
	if background != "" {
		options.CustomThemeBackground = background
	}
	if primary != "" {
		options.CustomThemePrimary = primary
	}
	if accent != "" {
		options.CustomThemeAccent = accent
	}
	return saveAppConfigToYamlFile("config", options)
}

func UpdateThemeAndBackgroundSettings(update model.Application) bool {
	options, err := GetAllSettingsOptions()
	if err != nil {
		return false
	}
	options.Theme = update.Theme
	if update.CustomThemeBackground != "" {
		options.CustomThemeBackground = update.CustomThemeBackground
	}
	if update.CustomThemePrimary != "" {
		options.CustomThemePrimary = update.CustomThemePrimary
	}
	if update.CustomThemeAccent != "" {
		options.CustomThemeAccent = update.CustomThemeAccent
	}
	options.BackgroundImage = update.BackgroundImage
	options.BackgroundImageMode = update.BackgroundImageMode
	options.BackgroundBlur = update.BackgroundBlur
	options.BackgroundOpacity = update.BackgroundOpacity
	options.GlassEffect = update.GlassEffect
	options.GlassIntensity = update.GlassIntensity
	return saveAppConfigToYamlFile("config", options)
}

func UpdateSearch(showSearchComponent bool, disabledSearchAutoFocus bool) bool {
	options, err := GetAllSettingsOptions()
	if err != nil {
		return false
	}
	options.ShowSearchComponent = showSearchComponent
	options.DisabledSearchAutoFocus = disabledSearchAutoFocus
	return saveAppConfigToYamlFile("config", options)
}

func GetAllSettingsOptions() (model.Application, error) {
	options, err := loadAppConfigFromYaml("config")
	if err != nil {
		return options, err
	}
	options.Locale = i18n.NormalizeLocale(options.Locale)
	if options.SiteIconMode == "" {
		options.SiteIconMode = "mdi"
	}
	if options.BackgroundImageMode == "" {
		options.BackgroundImageMode = "url"
	}
	if options.CustomThemeBackground == "" {
		options.CustomThemeBackground = "rgba(26, 26, 26, 1)"
	}
	if options.CustomThemePrimary == "" {
		options.CustomThemePrimary = "rgba(255, 253, 234, 1)"
	}
	if options.CustomThemeAccent == "" {
		options.CustomThemeAccent = "rgba(92, 92, 92, 1)"
	}
	if options.BackgroundOpacity == 0 && options.BackgroundImage == "" {
		options.BackgroundOpacity = 100
	}
	if options.GlassEffect == "" {
		options.GlassEffect = "none"
	}
	return options, nil
}

func UpdateAppearance(update model.Application) bool {
	options, err := GetAllSettingsOptions()
	if err != nil {
		return false
	}

	options.Title = update.Title
	options.Footer = update.Footer
	options.SiteIcon = update.SiteIcon
	options.SiteIconMode = update.SiteIconMode
	options.OpenAppNewTab = update.OpenAppNewTab
	options.OpenBookmarkNewTab = update.OpenBookmarkNewTab
	options.ShowTitle = update.ShowTitle
	options.Greetings = update.Greetings
	options.ShowDateTime = update.ShowDateTime
	options.ShowApps = update.ShowApps
	options.ShowBookmarks = update.ShowBookmarks
	options.AppsTitle = update.AppsTitle
	options.BookmarksTitle = update.BookmarksTitle
	options.BookmarkCategoryColor = update.BookmarkCategoryColor
	options.BookmarkItemColor = update.BookmarkItemColor
	options.HideSettingsButton = update.HideSettingsButton
	options.HideHelpButton = update.HideHelpButton
	options.EnableEncryptedLink = update.EnableEncryptedLink
	options.IconMode = update.IconMode
	options.KeepLetterCase = update.KeepLetterCase
	options.Locale = i18n.NormalizeLocale(update.Locale)
	options.HomeMaxColumns = update.HomeMaxColumns
	options.HomeMaxWidth = update.HomeMaxWidth

	return saveAppConfigToYamlFile("config", options)
}
