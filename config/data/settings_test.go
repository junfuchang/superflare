package data

import (
	"os"
	"testing"

	"github.com/junfuchang/superflare/config/model"
)

func TestGetAndSetThemeName(t *testing.T) {
	const target = "test"
	UpdateThemeName(target)
	theme := GetThemeName()
	if theme != target {
		t.Fatal("GetThemeName Error")
	}

	filePath := getConfigPath("config")
	os.Remove(filePath)
}

func TestUpdateSearchAndGetAllSettingsOptions(t *testing.T) {
	showSearchComponent := true
	disabledSearchAutoFocus := false

	ok := UpdateSearch(showSearchComponent, disabledSearchAutoFocus)
	if !ok {
		t.Fatal("UpdateSearch Error")
	}

	options, err := GetAllSettingsOptions()
	if err != nil {
		t.Fatalf("GetAllSettingsOptions: %v", err)
	}
	if options.ShowSearchComponent != showSearchComponent && options.DisabledSearchAutoFocus != disabledSearchAutoFocus {
		t.Fatal("GetAllSettingsOptions Error")
	}

	filePath := getConfigPath("config")
	os.Remove(filePath)
}

func TestUpdateAppearance(t *testing.T) {
	const Title = "Test"
	themeUpdate := model.Application{
		Theme:                 "custom",
		CustomThemeBackground: "rgba(1, 2, 3, 1)",
		CustomThemePrimary:    "rgba(4, 5, 6, 1)",
		CustomThemeAccent:     "rgba(7, 8, 9, 1)",
		BackgroundImage:       "/user-assets/background.png",
		BackgroundImageMode:   "upload",
		BackgroundBlur:        12,
		BackgroundOpacity:     88,
		GlassEffect:           "frosted",
		GlassIntensity:        30,
	}
	if !UpdateThemeAndBackgroundSettings(themeUpdate) {
		t.Fatal("UpdateThemeAndBackgroundSettings Error")
	}
	var update model.Application
	update.Title = Title
	update.AppsTitle = "Services"
	update.BookmarksTitle = "Links"
	update.BookmarkCategoryColor = "#112233"
	update.BookmarkItemColor = "rgba(10, 20, 30, 0.5)"

	ok := UpdateAppearance(update)
	if !ok {
		t.Fatal("UpdateAppearance Error")
	}

	options, err := GetAllSettingsOptions()
	if err != nil {
		t.Fatalf("GetAllSettingsOptions: %v", err)
	}
	if options.Title != Title {
		t.Fatal("GetAllSettingsOptions Error")
	}
	if options.AppsTitle != update.AppsTitle || options.BookmarksTitle != update.BookmarksTitle {
		t.Fatal("UpdateAppearance did not persist custom module titles")
	}
	if options.BookmarkCategoryColor != update.BookmarkCategoryColor || options.BookmarkItemColor != update.BookmarkItemColor {
		t.Fatal("UpdateAppearance did not persist bookmark colors")
	}
	if options.BackgroundImage != themeUpdate.BackgroundImage || options.GlassEffect != themeUpdate.GlassEffect {
		t.Fatal("UpdateAppearance should not overwrite background settings")
	}

	filePath := getConfigPath("config")
	os.Remove(filePath)
}

func TestUpdateThemeAndBackgroundSettings(t *testing.T) {
	update := model.Application{
		Theme:                 "custom",
		CustomThemeBackground: "rgba(1, 2, 3, 1)",
		CustomThemePrimary:    "rgba(4, 5, 6, 1)",
		CustomThemeAccent:     "rgba(7, 8, 9, 1)",
		BackgroundImage:       "https://example.com/background.jpg",
		BackgroundImageMode:   "url",
		BackgroundBlur:        8,
		BackgroundOpacity:     75,
		GlassEffect:           "liquid",
		GlassIntensity:        45,
	}
	if !UpdateThemeAndBackgroundSettings(update) {
		t.Fatal("UpdateThemeAndBackgroundSettings Error")
	}

	options, err := GetAllSettingsOptions()
	if err != nil {
		t.Fatalf("GetAllSettingsOptions: %v", err)
	}
	if options.Theme != update.Theme || options.BackgroundImage != update.BackgroundImage || options.GlassEffect != update.GlassEffect {
		t.Fatal("UpdateThemeAndBackgroundSettings did not persist theme background settings")
	}

	filePath := getConfigPath("config")
	os.Remove(filePath)
}

func TestUpdateLoginConfig(t *testing.T) {
	ok := UpdateLoginConfig("admin", "admin")
	if !ok {
		t.Fatal("UpdateLoginConfig Error")
	}

	options, err := GetAllSettingsOptions()
	if err != nil {
		t.Fatalf("GetAllSettingsOptions: %v", err)
	}
	if options.LoginUser != "admin" || options.LoginPass != "admin" {
		t.Fatal("UpdateLoginConfig did not persist credentials")
	}

	filePath := getConfigPath("config")
	os.Remove(filePath)
}
