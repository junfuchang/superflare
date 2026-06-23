package define

import (
	"html/template"
	"regexp"
	"strconv"
	"strings"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/model"
)

var ThemePalettes = getDefaultThemePalettes()
var ThemeCurrent = ""
var ThemePrimaryColor = ""

func Init() {
	initPageInlineStyle()
	UpdatePagePalettes()
}

var _CACHE_PAGE_INLINE_STYLE template.CSS

var CACHE_APP_CURRENT_THEME_PRIMARY_COLOR string
var _CACHE_PREV_THEME_NAME string

func GetPageInlineStyle() template.CSS {
	return _CACHE_PAGE_INLINE_STYLE
}

func initPageInlineStyle() {
	if AppFlags.DebugMode {
		return
	}

	_CACHE_PAGE_INLINE_STYLE = template.CSS(PAGE_INLINE_STYLE)
}

var _CACHE_PAGE_BODY_THEME_NAME template.CSS

func GetAppBodyStyle() template.CSS {
	return _CACHE_PAGE_BODY_THEME_NAME
}

func GetThemePrimaryColor(theme string) string {
	if _CACHE_PREV_THEME_NAME == theme {
		return CACHE_APP_CURRENT_THEME_PRIMARY_COLOR
	}
	for _, themePresent := range ThemePalettes {
		if themePresent.Name == theme {
			CACHE_APP_CURRENT_THEME_PRIMARY_COLOR = themePresent.Colors.Primary
			_CACHE_PREV_THEME_NAME = theme
			return CACHE_APP_CURRENT_THEME_PRIMARY_COLOR
		}
	}
	return CACHE_APP_CURRENT_THEME_PRIMARY_COLOR
}

const emptyPageBodyStyle = template.CSS(``)

func UpdatePagePalettes() {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		_CACHE_PAGE_BODY_THEME_NAME = emptyPageBodyStyle
		return
	}
	palette := model.Palette{}
	if options.Theme == "custom" {
		palette = getCustomPalette(options)
	} else {
		for _, themePresent := range ThemePalettes {
			if themePresent.Name == options.Theme {
				palette = themePresent.Colors
				break
			}
		}
	}
	if palette.Background == "" || palette.Primary == "" || palette.Accent == "" {
		_CACHE_PAGE_BODY_THEME_NAME = emptyPageBodyStyle
		return
	}
	ThemeCurrent = options.Theme
	ThemePrimaryColor = palette.Primary
	CACHE_APP_CURRENT_THEME_PRIMARY_COLOR = palette.Primary
	_CACHE_PREV_THEME_NAME = options.Theme
	_CACHE_PAGE_BODY_THEME_NAME = template.CSS(`--color-background:` + palette.Background + `;--color-primary:` + palette.Primary + `;--color-accent:` + palette.Accent + `;`)
}

func getCustomPalette(options model.Application) model.Palette {
	return model.Palette{
		Background: safeCSSColor(options.CustomThemeBackground, "rgba(26, 26, 26, 1)"),
		Primary:    safeCSSColor(options.CustomThemePrimary, "rgba(255, 253, 234, 1)"),
		Accent:     safeCSSColor(options.CustomThemeAccent, "rgba(92, 92, 92, 1)"),
	}
}

var (
	hexColorPattern  = regexp.MustCompile(`^#[0-9a-fA-F]{3,8}$`)
	rgbColorPattern  = regexp.MustCompile(`^rgba?\((.*)\)$`)
	cssNumberPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)
)

func SafeCSSColor(input string, fallback string) string {
	return safeCSSColor(input, fallback)
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

func getDefaultThemePalettes() []model.Theme {
	return []model.Theme{
		{
			Name:   "blackboard",
			Colors: model.Palette{Background: "#1a1a1a", Primary: "#FFFDEA", Accent: "#5c5c5c"},
		},
		{
			Name:   "gazette",
			Colors: model.Palette{Background: "#F2F7FF", Primary: "#000000", Accent: "#5c5c5c"},
		},
		{
			Name:   "espresso",
			Colors: model.Palette{Background: "#21211F", Primary: "#D1B59A", Accent: "#4E4E4E"},
		},
		{
			Name:   "cab",
			Colors: model.Palette{Background: "#F6D305", Primary: "#1F1F1F", Accent: "#424242"},
		},
		{
			Name:   "cloud",
			Colors: model.Palette{Background: "#f1f2f0", Primary: "#35342f", Accent: "#37bbe4"},
		},
		{
			Name:   "lime",
			Colors: model.Palette{Background: "#263238", Primary: "#AABBC3", Accent: "#aeea00"},
		},
		{
			Name:   "white",
			Colors: model.Palette{Background: "#ffffff", Primary: "#222222", Accent: "#dddddd"},
		},
		{
			Name:   "tron",
			Colors: model.Palette{Background: "#242B33", Primary: "#EFFBFF", Accent: "#6EE2FF"},
		},
		{
			Name:   "blues",
			Colors: model.Palette{Background: "#2B2C56", Primary: "#EFF1FC", Accent: "#6677EB"},
		},
		{
			Name:   "passion",
			Colors: model.Palette{Background: "#f5f5f5", Primary: "#12005e", Accent: "#8e24aa"},
		},
		{
			Name:   "chalk",
			Colors: model.Palette{Background: "#263238", Primary: "#AABBC3", Accent: "#FF869A"},
		},
		{
			Name:   "paper",
			Colors: model.Palette{Background: "#F8F6F1", Primary: "#4C432E", Accent: "#AA9A73"},
		},
		{
			Name:   "neon",
			Colors: model.Palette{Background: "#091833", Primary: "#EFFBFF", Accent: "#ea00d9"},
		},
		{
			Name:   "pumpkin",
			Colors: model.Palette{Background: "#2d3436", Primary: "#EFFBFF", Accent: "#ffa500"},
		},
		{
			Name:   "onedark",
			Colors: model.Palette{Background: "#282c34", Primary: "#dfd9d6", Accent: "#98c379"},
		},
	}
}
