package define

import (
	"fmt"
	"html/template"
	"strings"
	"sync"

	"github.com/junfuchang/superflare/config/data"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/config/validation"
)

var ThemePalettes = getDefaultThemePalettes()
var ThemeCurrent = ""
var ThemePrimaryColor = ""
var themeCompatMu sync.RWMutex

type ThemeRuntimeSnapshot struct {
	Name      string
	Primary   string
	BodyStyle template.CSS
}

type themeRuntimeHolder struct {
	mu  sync.RWMutex
	set bool
	cfg ThemeRuntimeSnapshot
}

func (h *themeRuntimeHolder) Load() (ThemeRuntimeSnapshot, bool) {
	if h == nil {
		return ThemeRuntimeSnapshot{}, false
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.cfg, h.set
}

func (h *themeRuntimeHolder) Store(cfg ThemeRuntimeSnapshot) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.set = true
	h.cfg = cfg
	h.mu.Unlock()
}

var themeRuntime = &themeRuntimeHolder{}

func Init() {
	if err := InitE(); err != nil {
		panic(err)
	}
}

func InitE() error {
	initPageInlineStyle()
	return UpdatePagePalettes()
}

var _CACHE_PAGE_INLINE_STYLE template.CSS

var CACHE_APP_CURRENT_THEME_PRIMARY_COLOR string
var _CACHE_PREV_THEME_NAME string

func GetPageInlineStyle() template.CSS {
	themeCompatMu.RLock()
	defer themeCompatMu.RUnlock()
	return _CACHE_PAGE_INLINE_STYLE
}

func initPageInlineStyle() {
	var style template.CSS
	if CurrentAppRuntimeFlags().DebugMode {
		style = ""
	} else {
		style = template.CSS(PAGE_INLINE_STYLE)
	}

	themeCompatMu.Lock()
	_CACHE_PAGE_INLINE_STYLE = style
	themeCompatMu.Unlock()
}

var _CACHE_PAGE_BODY_THEME_NAME template.CSS

func GetAppBodyStyle() template.CSS {
	if snapshot, ok := themeRuntime.Load(); ok {
		return snapshot.BodyStyle
	}
	themeCompatMu.RLock()
	defer themeCompatMu.RUnlock()
	return _CACHE_PAGE_BODY_THEME_NAME
}

func GetThemeRuntimeSnapshot() ThemeRuntimeSnapshot {
	if snapshot, ok := themeRuntime.Load(); ok {
		return snapshot
	}
	themeCompatMu.RLock()
	defer themeCompatMu.RUnlock()
	return ThemeRuntimeSnapshot{
		Name:      ThemeCurrent,
		Primary:   ThemePrimaryColor,
		BodyStyle: _CACHE_PAGE_BODY_THEME_NAME,
	}
}

func StoreThemeRuntimeSnapshot(snapshot ThemeRuntimeSnapshot) {
	themeRuntime.Store(snapshot)
	themeCompatMu.Lock()
	ThemeCurrent = snapshot.Name
	ThemePrimaryColor = snapshot.Primary
	_CACHE_PAGE_BODY_THEME_NAME = snapshot.BodyStyle
	if strings.TrimSpace(snapshot.Name) != "" && strings.TrimSpace(snapshot.Primary) != "" {
		CACHE_APP_CURRENT_THEME_PRIMARY_COLOR = snapshot.Primary
		_CACHE_PREV_THEME_NAME = snapshot.Name
	}
	themeCompatMu.Unlock()
}

func GetThemePrimaryColor(theme string) string {
	themeCompatMu.RLock()
	if _CACHE_PREV_THEME_NAME == theme {
		defer themeCompatMu.RUnlock()
		return CACHE_APP_CURRENT_THEME_PRIMARY_COLOR
	}
	themeCompatMu.RUnlock()
	for _, themePresent := range ThemePalettes {
		if themePresent.Name == theme {
			themeCompatMu.Lock()
			CACHE_APP_CURRENT_THEME_PRIMARY_COLOR = themePresent.Colors.Primary
			_CACHE_PREV_THEME_NAME = theme
			color := CACHE_APP_CURRENT_THEME_PRIMARY_COLOR
			themeCompatMu.Unlock()
			return color
		}
	}
	themeCompatMu.RLock()
	defer themeCompatMu.RUnlock()
	return CACHE_APP_CURRENT_THEME_PRIMARY_COLOR
}

const emptyPageBodyStyle = template.CSS(``)

func UpdatePagePalettes() error {
	options, err := data.GetAllSettingsOptions()
	if err != nil {
		return err
	}
	themeName := strings.TrimSpace(options.Theme)
	if themeName == "" {
		themeName = "blackboard"
	}
	palette := model.Palette{}
	if themeName == "custom" {
		palette, err = getCustomPalette(options)
		if err != nil {
			return err
		}
	} else {
		found := false
		for _, themePresent := range ThemePalettes {
			if themePresent.Name == themeName {
				palette = themePresent.Colors
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("invalid theme value: %s", options.Theme)
		}
	}
	if palette.Background == "" || palette.Primary == "" || palette.Accent == "" {
		return fmt.Errorf("theme palette is incomplete for theme: %s", themeName)
	}
	StoreThemeRuntimeSnapshot(ThemeRuntimeSnapshot{
		Name:      themeName,
		Primary:   palette.Primary,
		BodyStyle: template.CSS(`--color-background:` + palette.Background + `;--color-primary:` + palette.Primary + `;--color-accent:` + palette.Accent + `;`),
	})
	return nil
}

func getCustomPalette(options model.Application) (model.Palette, error) {
	basePalette, err := getThemePaletteByName(resolveThemeBase(options))
	if err != nil {
		return model.Palette{}, err
	}

	background := strings.TrimSpace(options.CustomThemeBackground)
	if background == "" {
		background = basePalette.Background
	} else if validation.SafeCSSColor(background, "") == "" {
		return model.Palette{}, fmt.Errorf("invalid custom theme background color: %s", options.CustomThemeBackground)
	}

	primary := strings.TrimSpace(options.CustomThemePrimary)
	if primary == "" {
		primary = basePalette.Primary
	} else if validation.SafeCSSColor(primary, "") == "" {
		return model.Palette{}, fmt.Errorf("invalid custom theme primary color: %s", options.CustomThemePrimary)
	}

	accent := strings.TrimSpace(options.CustomThemeAccent)
	if accent == "" {
		accent = basePalette.Accent
	} else if validation.SafeCSSColor(accent, "") == "" {
		return model.Palette{}, fmt.Errorf("invalid custom theme accent color: %s", options.CustomThemeAccent)
	}

	return model.Palette{
		Background: background,
		Primary:    primary,
		Accent:     accent,
	}, nil
}

func resolveThemeBase(options model.Application) string {
	themeBase := strings.ToLower(strings.TrimSpace(options.ThemeBase))
	if themeBase == "" || themeBase == "custom" {
		themeBase = strings.ToLower(strings.TrimSpace(options.Theme))
	}
	if themeBase == "" || themeBase == "custom" {
		themeBase = "blackboard"
	}
	return themeBase
}

func getThemePaletteByName(themeName string) (model.Palette, error) {
	for _, themePresent := range ThemePalettes {
		if themePresent.Name == themeName {
			return themePresent.Colors, nil
		}
	}
	return model.Palette{}, fmt.Errorf("invalid theme base value: %s", themeName)
}

func SafeCSSColor(input string, fallback string) string {
	return validation.SafeCSSColor(input, fallback)
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
