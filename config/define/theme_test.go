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
	previousSnapshot := GetThemeRuntimeSnapshot()
	previousCachedPrimary := CACHE_APP_CURRENT_THEME_PRIMARY_COLOR
	previousCachedThemeName := _CACHE_PREV_THEME_NAME
	originalGetwd := dataOsGetwdForThemeTest()
	defer func() {
		restoreDataOsGetwdForThemeTest(originalGetwd)
		CACHE_APP_CURRENT_THEME_PRIMARY_COLOR = previousCachedPrimary
		_CACHE_PREV_THEME_NAME = previousCachedThemeName
		StoreThemeRuntimeSnapshot(previousSnapshot)
		if previousSnapshot == (ThemeRuntimeSnapshot{}) {
			StoreThemeRuntimeSnapshot(ThemeRuntimeSnapshot{
				Name:      previousTheme,
				Primary:   previousPrimary,
				BodyStyle: previousBodyStyle,
			})
		}
	}()

	if previousBodyStyle == "" {
		StoreThemeRuntimeSnapshot(ThemeRuntimeSnapshot{
			Name:      "blackboard",
			Primary:   "#FFFDEA",
			BodyStyle: `--color-background:#1a1a1a;--color-primary:#FFFDEA;--color-accent:#5c5c5c;`,
		})
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

func TestThemeRuntimeSnapshotReflectsCachedPaletteState(t *testing.T) {
	previousBodyStyle := GetAppBodyStyle()
	previousTheme := ThemeCurrent
	previousPrimary := ThemePrimaryColor
	previousSnapshot := GetThemeRuntimeSnapshot()
	defer func() {
		StoreThemeRuntimeSnapshot(previousSnapshot)
		if previousSnapshot == (ThemeRuntimeSnapshot{}) {
			StoreThemeRuntimeSnapshot(ThemeRuntimeSnapshot{
				Name:      previousTheme,
				Primary:   previousPrimary,
				BodyStyle: previousBodyStyle,
			})
		}
	}()

	StoreThemeRuntimeSnapshot(ThemeRuntimeSnapshot{
		Name:      "gazette",
		Primary:   "#000000",
		BodyStyle: `--color-background:#F2F7FF;--color-primary:#000000;--color-accent:#5c5c5c;`,
	})

	snapshot := GetThemeRuntimeSnapshot()
	if snapshot.Name != "gazette" {
		t.Fatalf("unexpected theme name: %q", snapshot.Name)
	}
	if snapshot.Primary != "#000000" {
		t.Fatalf("unexpected primary color: %q", snapshot.Primary)
	}
	if string(snapshot.BodyStyle) != `--color-background:#F2F7FF;--color-primary:#000000;--color-accent:#5c5c5c;` {
		t.Fatalf("unexpected body style: %q", snapshot.BodyStyle)
	}
}

func TestThemeRuntimeSnapshotIgnoresLaterCompatibilityGlobalMutation(t *testing.T) {
	previousBodyStyle := GetAppBodyStyle()
	previousTheme := ThemeCurrent
	previousPrimary := ThemePrimaryColor
	previousSnapshot := GetThemeRuntimeSnapshot()
	defer func() {
		StoreThemeRuntimeSnapshot(previousSnapshot)
		if previousSnapshot == (ThemeRuntimeSnapshot{}) {
			StoreThemeRuntimeSnapshot(ThemeRuntimeSnapshot{
				Name:      previousTheme,
				Primary:   previousPrimary,
				BodyStyle: previousBodyStyle,
			})
		}
	}()

	StoreThemeRuntimeSnapshot(ThemeRuntimeSnapshot{
		Name:      "custom",
		Primary:   "#123456",
		BodyStyle: `--color-background:#000000;--color-primary:#123456;--color-accent:#abcdef;`,
	})
	ThemeCurrent = "mutated-global"
	ThemePrimaryColor = "#ffffff"
	_CACHE_PAGE_BODY_THEME_NAME = `--color-background:#ffffff;--color-primary:#ffffff;--color-accent:#ffffff;`

	snapshot := GetThemeRuntimeSnapshot()
	if snapshot.Name != "custom" || snapshot.Primary != "#123456" {
		t.Fatalf("expected stored snapshot to survive global mutation, got %#v", snapshot)
	}
	if string(snapshot.BodyStyle) != `--color-background:#000000;--color-primary:#123456;--color-accent:#abcdef;` {
		t.Fatalf("expected stored body style to survive global mutation, got %q", snapshot.BodyStyle)
	}
}
