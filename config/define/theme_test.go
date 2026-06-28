package define

import (
	"errors"
	"testing"

	"github.com/junfuchang/superflare/config/model"
)

func TestUpdatePagePalettesReturnsErrorWithoutClearingPreviousCache(t *testing.T) {
	previousBodyStyle := GetAppBodyStyle()
	previousTheme := ThemeCurrent
	previousPrimary := ThemePrimaryColor
	previousCachedPrimary := CACHE_APP_CURRENT_THEME_PRIMARY_COLOR
	previousCachedThemeName := _CACHE_PREV_THEME_NAME
	originalGetwd := dataOsGetwdForThemeTest()
	defer func() {
		restoreDataOsGetwdForThemeTest(originalGetwd)
		ThemeCurrent = previousTheme
		ThemePrimaryColor = previousPrimary
		CACHE_APP_CURRENT_THEME_PRIMARY_COLOR = previousCachedPrimary
		_CACHE_PREV_THEME_NAME = previousCachedThemeName
		_CACHE_PAGE_BODY_THEME_NAME = previousBodyStyle
	}()

	if previousBodyStyle == "" {
		_CACHE_PAGE_BODY_THEME_NAME = `--color-background:#1a1a1a;--color-primary:#FFFDEA;--color-accent:#5c5c5c;`
	}
	before := GetAppBodyStyle()
	setDataOsGetwdForThemeTest(func() (string, error) {
		return "", errors.New("forced getwd failure")
	})

	err := UpdatePagePalettes()
	if err == nil {
		t.Fatal("expected UpdatePagePalettes to fail")
	}
	if GetAppBodyStyle() != before {
		t.Fatal("expected theme body style cache to remain unchanged on error")
	}
}

func TestGetCustomPaletteFallsBackToThemeBasePalette(t *testing.T) {
	palette, err := getCustomPalette(model.Application{
		Theme:                 "custom",
		ThemeBase:             "gazette",
		CustomThemeBackground: "",
		CustomThemePrimary:    "",
		CustomThemeAccent:     "",
	})
	if err != nil {
		t.Fatalf("getCustomPalette: %v", err)
	}
	if palette.Background != "#F2F7FF" || palette.Primary != "#000000" || palette.Accent != "#5c5c5c" {
		t.Fatalf("unexpected palette: %#v", palette)
	}
}
