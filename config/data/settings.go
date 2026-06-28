package data

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/i18n"
)

const (
	iconModeMissingBlank = "DEFAULT"
	iconModeMissingFill  = "FILLING"
	iconModeHidden       = "NONE"
)

const (
	searchModeBookmarks = "bookmarks"
	searchModeEngine    = "engine"
	searchEngineBaidu   = "baidu"
	searchEngineBing    = "bing"
	searchEngineGoogle  = "google"
	searchEngineDuck    = "duckduckgo"
	searchEngineCustom  = "custom"
	searchEngineOpenSameTab = "same-tab"
	searchEngineOpenNewTab  = "new-tab"
)

var (
	validThemeNames = map[string]struct{}{
		"blackboard": {},
		"gazette":    {},
		"espresso":   {},
		"cab":        {},
		"cloud":      {},
		"lime":       {},
		"white":      {},
		"tron":       {},
		"blues":      {},
		"passion":    {},
		"chalk":      {},
		"paper":      {},
		"neon":       {},
		"pumpkin":    {},
		"onedark":    {},
		"custom":     {},
	}
	hexColorPattern  = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)
	rgbColorPattern  = regexp.MustCompile(`^rgba?\((.*)\)$`)
	cssNumberPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)
)

func GetThemeNameErr() (string, error) {
	opts, err := GetAllSettingsOptions()
	if err != nil {
		return "", err
	}
	return opts.Theme, nil
}

func UpdateThemeName(theme string) error {
	return UpdateThemeSettings(theme, "", "", "", "")
}

func UpdateThemeSettings(theme string, themeBase string, background string, primary string, accent string) error {
	if err := EnsureAppConfigExists(); err != nil {
		return err
	}
	options, err := GetAllSettingsOptions()
	if err != nil {
		return err
	}
	options.Theme = theme
	if theme != "custom" {
		options.ThemeBase = theme
	} else if strings.TrimSpace(themeBase) != "" {
		options.ThemeBase = themeBase
	}
	if theme == "custom" || background != "" {
		options.CustomThemeBackground = background
	}
	if theme == "custom" || primary != "" {
		options.CustomThemePrimary = primary
	}
	if theme == "custom" || accent != "" {
		options.CustomThemeAccent = accent
	}
	return saveAppConfigToYamlFile("config", options)
}

func UpdateThemeAndBackgroundSettings(update model.Application) error {
	if err := EnsureAppConfigExists(); err != nil {
		return err
	}
	options, err := GetAllSettingsOptions()
	if err != nil {
		return err
	}
	options.Theme = update.Theme
	if update.Theme != "custom" {
		options.ThemeBase = update.Theme
	} else if strings.TrimSpace(update.ThemeBase) != "" {
		options.ThemeBase = update.ThemeBase
	}
	if update.Theme == "custom" || update.CustomThemeBackground != "" {
		options.CustomThemeBackground = update.CustomThemeBackground
	}
	if update.Theme == "custom" || update.CustomThemePrimary != "" {
		options.CustomThemePrimary = update.CustomThemePrimary
	}
	if update.Theme == "custom" || update.CustomThemeAccent != "" {
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

func UpdateSearch(showSearchComponent bool, disabledSearchAutoFocus bool, searchMode string, searchEngine string, searchEngineOpenMode string, searchEngineCustomTemplate string) error {
	if err := EnsureAppConfigExists(); err != nil {
		return err
	}
	options, err := GetAllSettingsOptions()
	if err != nil {
		return err
	}
	options.ShowSearchComponent = showSearchComponent
	options.DisabledSearchAutoFocus = disabledSearchAutoFocus
	options.SearchMode = searchMode
	options.SearchEngine = searchEngine
	if strings.TrimSpace(searchEngineOpenMode) == "" {
		searchEngineOpenMode = searchEngineOpenSameTab
	}
	options.SearchEngineOpenMode = searchEngineOpenMode
	options.SearchEngineCustomTemplate = strings.TrimSpace(searchEngineCustomTemplate)
	if err := validateSettingsOptions(options); err != nil {
		return err
	}
	return saveAppConfigToYamlFile("config", options)
}

func GetAllSettingsOptions() (model.Application, error) {
	options, err := loadAppConfigFromYaml("config")
	if err != nil {
		return options, err
	}
	if err := validateSettingsOptions(options); err != nil {
		return options, err
	}
	options.Locale = i18n.NormalizeLocale(options.Locale)
	if options.SiteIconMode == "" {
		options.SiteIconMode = "mdi"
	}
	if options.IconMode == "" {
		options.IconMode = "FILLING"
	}
	if options.BackgroundImageMode == "" {
		options.BackgroundImageMode = "url"
	}
	if strings.TrimSpace(options.SearchMode) == "" {
		options.SearchMode = searchModeBookmarks
	}
	if strings.TrimSpace(options.SearchEngine) == "" {
		options.SearchEngine = searchEngineBing
	}
	if strings.TrimSpace(options.SearchEngineOpenMode) == "" {
		options.SearchEngineOpenMode = searchEngineOpenSameTab
	}
	if options.ThemeBase == "" {
		if strings.TrimSpace(options.Theme) == "" || strings.EqualFold(strings.TrimSpace(options.Theme), "custom") {
			options.ThemeBase = "blackboard"
		} else {
			options.ThemeBase = strings.ToLower(strings.TrimSpace(options.Theme))
		}
	}
	if options.BackgroundOpacity == 0 && options.BackgroundImage == "" {
		options.BackgroundOpacity = 100
	}
	if options.GlassEffect == "" {
		options.GlassEffect = "none"
	}
	return options, nil
}

func validateSettingsOptions(options model.Application) error {
	locale := strings.TrimSpace(options.Locale)
	if locale != "" {
		normalized := i18n.NormalizeLocale(locale)
		if !strings.HasPrefix(strings.ToLower(locale), normalized) {
			return fmt.Errorf("invalid locale value: %s", options.Locale)
		}
	}

	iconMode := strings.ToUpper(strings.TrimSpace(options.IconMode))
	switch iconMode {
	case "", iconModeMissingBlank, iconModeMissingFill, iconModeHidden:
	default:
		return fmt.Errorf("invalid icon mode value: %s", options.IconMode)
	}

	themeName := strings.ToLower(strings.TrimSpace(options.Theme))
	if themeName == "" {
		return fmt.Errorf("missing theme value")
	}
	if _, ok := validThemeNames[themeName]; !ok {
		return fmt.Errorf("invalid theme value: %s", options.Theme)
	}
	themeBase := strings.ToLower(strings.TrimSpace(options.ThemeBase))
	if themeBase == "" {
		if themeName == "custom" {
			themeBase = "blackboard"
		} else {
			themeBase = themeName
		}
	}
	if themeBase == "custom" {
		return fmt.Errorf("invalid theme base value: %s", options.ThemeBase)
	}
	if _, ok := validThemeNames[themeBase]; !ok {
		return fmt.Errorf("invalid theme base value: %s", options.ThemeBase)
	}

	siteIconMode := strings.ToLower(strings.TrimSpace(options.SiteIconMode))
	switch siteIconMode {
	case "", "mdi":
	default:
		return fmt.Errorf("invalid site icon mode value: %s", options.SiteIconMode)
	}

	backgroundImageMode := strings.ToLower(strings.TrimSpace(options.BackgroundImageMode))
	switch backgroundImageMode {
	case "", "url", "upload":
	default:
		return fmt.Errorf("invalid background image mode value: %s", options.BackgroundImageMode)
	}

	searchMode := strings.ToLower(strings.TrimSpace(options.SearchMode))
	switch searchMode {
	case "", searchModeBookmarks, searchModeEngine:
	default:
		return fmt.Errorf("invalid search mode value: %s", options.SearchMode)
	}

	searchEngine := strings.ToLower(strings.TrimSpace(options.SearchEngine))
	switch searchEngine {
	case "", searchEngineBaidu, searchEngineBing, searchEngineGoogle, searchEngineDuck, searchEngineCustom:
	default:
		return fmt.Errorf("invalid search engine value: %s", options.SearchEngine)
	}
	searchEngineOpenMode := strings.ToLower(strings.TrimSpace(options.SearchEngineOpenMode))
	switch searchEngineOpenMode {
	case "", searchEngineOpenSameTab, searchEngineOpenNewTab:
	default:
		return fmt.Errorf("invalid search engine open mode value: %s", options.SearchEngineOpenMode)
	}
	if searchMode == searchModeEngine && searchEngine == searchEngineCustom {
		if !strings.Contains(strings.TrimSpace(options.SearchEngineCustomTemplate), "%s") {
			return fmt.Errorf("custom search engine template must contain %%s placeholder")
		}
	}

	if options.BackgroundBlur < 0 || options.BackgroundBlur > 80 {
		return fmt.Errorf("background blur must be between 0 and 80")
	}
	if options.BackgroundOpacity < 0 || options.BackgroundOpacity > 100 {
		return fmt.Errorf("background opacity must be between 0 and 100")
	}

	glassEffect := strings.ToLower(strings.TrimSpace(options.GlassEffect))
	switch glassEffect {
	case "", "none", "frosted", "liquid":
	default:
		return fmt.Errorf("invalid glass effect value: %s", options.GlassEffect)
	}

	if options.GlassIntensity < 0 || options.GlassIntensity > 100 {
		return fmt.Errorf("glass intensity must be between 0 and 100")
	}

	if color := strings.TrimSpace(options.BookmarkCategoryColor); color != "" && safeCSSColor(color, "") == "" {
		return fmt.Errorf("invalid bookmark category color: %s", options.BookmarkCategoryColor)
	}
	if color := strings.TrimSpace(options.BookmarkItemColor); color != "" && safeCSSColor(color, "") == "" {
		return fmt.Errorf("invalid bookmark item color: %s", options.BookmarkItemColor)
	}

	if themeName == "custom" {
		background := strings.TrimSpace(options.CustomThemeBackground)
		if background != "" && safeCSSColor(background, "") == "" {
			return fmt.Errorf("invalid custom theme background color: %s", options.CustomThemeBackground)
		}

		primary := strings.TrimSpace(options.CustomThemePrimary)
		if primary != "" && safeCSSColor(primary, "") == "" {
			return fmt.Errorf("invalid custom theme primary color: %s", options.CustomThemePrimary)
		}

		accent := strings.TrimSpace(options.CustomThemeAccent)
		if accent != "" && safeCSSColor(accent, "") == "" {
			return fmt.Errorf("invalid custom theme accent color: %s", options.CustomThemeAccent)
		}
	}

	if options.HomeMaxColumns < 0 || options.HomeMaxColumns > 8 {
		return fmt.Errorf("home max columns must be between 0 and 8")
	}
	if options.HomeMaxWidth < 0 || options.HomeMaxWidth > 2400 {
		return fmt.Errorf("home max width must be between 0 and 2400")
	}

	return nil
}

func safeCSSColor(input string, fallback string) string {
	input = strings.TrimSpace(input)
	if hexColorPattern.MatchString(input) || safeRGBColor(input) {
		return input
	}
	return fallback
}

func safeRGBColor(input string) bool {
	match := rgbColorPattern.FindStringSubmatch(input)
	if match == nil {
		return false
	}
	parts := strings.Split(match[1], ",")
	if len(parts) != 3 && len(parts) != 4 {
		return false
	}
	for i := 0; i < 3; i++ {
		value := strings.TrimSpace(parts[i])
		if strings.HasSuffix(value, "%") {
			num, err := strconv.ParseFloat(strings.TrimSuffix(value, "%"), 64)
			if err != nil || num < 0 || num > 100 {
				return false
			}
			continue
		}
		if !cssNumberPattern.MatchString(value) {
			return false
		}
		num, err := strconv.Atoi(strings.Split(value, ".")[0])
		if err != nil || num < 0 || num > 255 {
			return false
		}
		if strings.Contains(value, ".") {
			parsed, err := strconv.ParseFloat(value, 64)
			if err != nil || parsed < 0 || parsed > 255 {
				return false
			}
		}
	}
	if len(parts) == 4 {
		alpha := strings.TrimSpace(parts[3])
		if strings.HasSuffix(alpha, "%") {
			num, err := strconv.ParseFloat(strings.TrimSuffix(alpha, "%"), 64)
			return err == nil && num >= 0 && num <= 100
		}
		num, err := strconv.ParseFloat(alpha, 64)
		return err == nil && num >= 0 && num <= 1
	}
	return true
}

func UpdateAppearance(update model.Application) error {
	if err := EnsureAppConfigExists(); err != nil {
		return err
	}
	options, err := GetAllSettingsOptions()
	if err != nil {
		return err
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
	options.HideWarningsButton = update.HideWarningsButton
	options.EnableEncryptedLink = update.EnableEncryptedLink
	options.IconMode = update.IconMode
	options.KeepLetterCase = update.KeepLetterCase
	options.Locale = i18n.NormalizeLocale(update.Locale)
	options.HomeMaxColumns = update.HomeMaxColumns
	options.HomeMaxWidth = update.HomeMaxWidth

	return saveAppConfigToYamlFile("config", options)
}
