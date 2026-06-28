package data

import (
	"os"
	"testing"

	"github.com/junfuchang/superflare/config/model"
)

func withTempWorkingDir(t *testing.T) {
	t.Helper()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("Chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
	})
}

func TestGetAndSetThemeName(t *testing.T) {
	withTempWorkingDir(t)
	const target = "gazette"
	if err := UpdateThemeName(target); err != nil {
		t.Fatalf("UpdateThemeName: %v", err)
	}
	theme, err := GetThemeNameErr()
	if err != nil {
		t.Fatalf("GetThemeNameErr: %v", err)
	}
	if theme != target {
		t.Fatal("GetThemeName Error")
	}

	filePath := getConfigPath("config")
	os.Remove(filePath)
}

func TestGetThemeNameErrReturnsErrorWhenConfigBroken(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.Mkdir("config.yml", 0755); err != nil {
		t.Fatalf("mkdir config.yml: %v", err)
	}
	if _, err := GetThemeNameErr(); err == nil {
		t.Fatal("expected GetThemeNameErr to fail")
	}
}

func TestUpdateSearchAndGetAllSettingsOptions(t *testing.T) {
	withTempWorkingDir(t)
	showSearchComponent := true
	disabledSearchAutoFocus := false
	searchMode := "engine"
	searchEngine := "bing"
	searchEngineOpenMode := "new-tab"
	searchEngineCustomTemplate := "https://example.com/search?q=%s"

	if err := UpdateSearch(showSearchComponent, disabledSearchAutoFocus, searchMode, searchEngine, searchEngineOpenMode, searchEngineCustomTemplate); err != nil {
		t.Fatalf("UpdateSearch Error: %v", err)
	}

	options, err := GetAllSettingsOptions()
	if err != nil {
		t.Fatalf("GetAllSettingsOptions: %v", err)
	}
	if options.ShowSearchComponent != showSearchComponent || options.DisabledSearchAutoFocus != disabledSearchAutoFocus {
		t.Fatal("GetAllSettingsOptions Error")
	}
	if options.SearchMode != searchMode || options.SearchEngine != searchEngine || options.SearchEngineOpenMode != searchEngineOpenMode || options.SearchEngineCustomTemplate != searchEngineCustomTemplate {
		t.Fatal("UpdateSearch did not persist engine search settings")
	}

	filePath := getConfigPath("config")
	os.Remove(filePath)
}

func TestGetAllSettingsOptionsDefaultsSearchSettings(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	options, err := GetAllSettingsOptions()
	if err != nil {
		t.Fatalf("GetAllSettingsOptions: %v", err)
	}
	if options.SearchMode != "bookmarks" {
		t.Fatalf("expected default search mode bookmarks, got %q", options.SearchMode)
	}
	if options.SearchEngine != "bing" {
		t.Fatalf("expected default search engine bing, got %q", options.SearchEngine)
	}
	if options.SearchEngineOpenMode != "same-tab" {
		t.Fatalf("expected default search engine open mode same-tab, got %q", options.SearchEngineOpenMode)
	}
	if options.SearchEngineCustomTemplate != "" {
		t.Fatalf("expected empty custom template by default, got %q", options.SearchEngineCustomTemplate)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenSearchModeInvalid(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nSearchMode: mystery\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	_, err := GetAllSettingsOptions()
	if err == nil {
		t.Fatal("expected GetAllSettingsOptions to fail")
	}
	if err.Error() != "invalid search mode value: mystery" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenSearchEngineInvalid(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nSearchEngine: mystery\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	_, err := GetAllSettingsOptions()
	if err == nil {
		t.Fatal("expected GetAllSettingsOptions to fail")
	}
	if err.Error() != "invalid search engine value: mystery" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenSearchEngineOpenModeInvalid(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nSearchEngineOpenMode: popup\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	_, err := GetAllSettingsOptions()
	if err == nil {
		t.Fatal("expected GetAllSettingsOptions to fail")
	}
	if err.Error() != "invalid search engine open mode value: popup" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenCustomSearchTemplateMissingPlaceholder(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nSearchMode: engine\nSearchEngine: custom\nSearchEngineCustomTemplate: https://example.com/search\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	_, err := GetAllSettingsOptions()
	if err == nil {
		t.Fatal("expected GetAllSettingsOptions to fail")
	}
	if err.Error() != "custom search engine template must contain %s placeholder" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateAppearance(t *testing.T) {
	withTempWorkingDir(t)
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
	if err := UpdateThemeAndBackgroundSettings(themeUpdate); err != nil {
		t.Fatalf("UpdateThemeAndBackgroundSettings Error: %v", err)
	}
	var update model.Application
	update.Title = Title
	update.AppsTitle = "Services"
	update.BookmarksTitle = "Links"
	update.BookmarkCategoryColor = "#112233"
	update.BookmarkItemColor = "rgba(10, 20, 30, 0.5)"

	if err := UpdateAppearance(update); err != nil {
		t.Fatalf("UpdateAppearance Error: %v", err)
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
	withTempWorkingDir(t)
	update := model.Application{
		Theme:                 "custom",
		ThemeBase:             "gazette",
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
	if err := UpdateThemeAndBackgroundSettings(update); err != nil {
		t.Fatalf("UpdateThemeAndBackgroundSettings Error: %v", err)
	}

	options, err := GetAllSettingsOptions()
	if err != nil {
		t.Fatalf("GetAllSettingsOptions: %v", err)
	}
	if options.Theme != update.Theme || options.ThemeBase != update.ThemeBase || options.BackgroundImage != update.BackgroundImage || options.GlassEffect != update.GlassEffect {
		t.Fatal("UpdateThemeAndBackgroundSettings did not persist theme background settings")
	}

	filePath := getConfigPath("config")
	os.Remove(filePath)
}

func TestUpdateLoginConfig(t *testing.T) {
	withTempWorkingDir(t)
	if err := UpdateLoginConfig("admin", "admin"); err != nil {
		t.Fatalf("UpdateLoginConfig Error: %v", err)
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

func TestGetAllSettingsOptionsReturnsErrorWhenLocaleInvalid(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: fr\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	_, err := GetAllSettingsOptions()
	if err == nil {
		t.Fatal("expected GetAllSettingsOptions to fail")
	}
	if err != nil && err.Error() != "invalid locale value: fr" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenIconModeInvalid(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nIconMode: BOGUS\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	_, err := GetAllSettingsOptions()
	if err == nil {
		t.Fatal("expected GetAllSettingsOptions to fail")
	}
	if err != nil && err.Error() != "invalid icon mode value: BOGUS" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenHomeMaxColumnsOutOfRange(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nHomeMaxColumns: 99\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	_, err := GetAllSettingsOptions()
	if err == nil {
		t.Fatal("expected GetAllSettingsOptions to fail")
	}
	if err != nil && err.Error() != "home max columns must be between 0 and 8" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenHomeMaxWidthOutOfRange(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nHomeMaxWidth: 9999\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	_, err := GetAllSettingsOptions()
	if err == nil {
		t.Fatal("expected GetAllSettingsOptions to fail")
	}
	if err != nil && err.Error() != "home max width must be between 0 and 2400" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenThemeInvalid(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: mystery\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	_, err := GetAllSettingsOptions()
	if err == nil {
		t.Fatal("expected GetAllSettingsOptions to fail")
	}
	if err.Error() != "invalid theme value: mystery" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenCustomThemeColorMissing(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: custom\nThemeBase: blackboard\nCustomThemePrimary: rgba(255, 253, 234, 1)\nCustomThemeAccent: rgba(92, 92, 92, 1)\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	options, err := GetAllSettingsOptions()
	if err != nil {
		t.Fatalf("GetAllSettingsOptions: %v", err)
	}
	if options.CustomThemeBackground != "" {
		t.Fatalf("expected empty custom background, got %q", options.CustomThemeBackground)
	}
}

func TestGetAllSettingsOptionsPreservesEmptyCustomThemeFieldsForRender(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: custom\nThemeBase: gazette\nCustomThemeBackground: \"\"\nCustomThemePrimary: \"\"\nCustomThemeAccent: \"\"\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	options, err := GetAllSettingsOptions()
	if err != nil {
		t.Fatalf("GetAllSettingsOptions: %v", err)
	}
	if options.ThemeBase != "gazette" {
		t.Fatalf("expected theme base gazette, got %q", options.ThemeBase)
	}
	if options.CustomThemeBackground != "" || options.CustomThemePrimary != "" || options.CustomThemeAccent != "" {
		t.Fatalf("expected empty custom theme fields, got %#v", options)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenThemeBaseInvalid(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: custom\nThemeBase: custom\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	_, err := GetAllSettingsOptions()
	if err == nil {
		t.Fatal("expected GetAllSettingsOptions to fail")
	}
	if err.Error() != "invalid theme base value: custom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenBookmarkColorInvalid(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.WriteFile("config.yml", []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\nBookmarkCategoryColor: bad-color\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	_, err := GetAllSettingsOptions()
	if err == nil {
		t.Fatal("expected GetAllSettingsOptions to fail")
	}
	if err.Error() != "invalid bookmark category color: bad-color" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGetAllSettingsOptionsReturnsErrorWhenBackgroundAndGlassSettingsInvalid(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
	}{
		{
			name:    "background image mode",
			content: "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nBackgroundImageMode: mystery\n",
			want:    "invalid background image mode value: mystery",
		},
		{
			name:    "background blur",
			content: "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nBackgroundBlur: 81\n",
			want:    "background blur must be between 0 and 80",
		},
		{
			name:    "background opacity",
			content: "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nBackgroundOpacity: 101\n",
			want:    "background opacity must be between 0 and 100",
		},
		{
			name:    "glass effect",
			content: "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nGlassEffect: shimmer\n",
			want:    "invalid glass effect value: shimmer",
		},
		{
			name:    "glass intensity",
			content: "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nGlassIntensity: 101\n",
			want:    "glass intensity must be between 0 and 100",
		},
		{
			name:    "site icon mode",
			content: "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nSiteIconMode: custom\n",
			want:    "invalid site icon mode value: custom",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withTempWorkingDir(t)
			if err := os.WriteFile("config.yml", []byte(tc.content), 0644); err != nil {
				t.Fatalf("write config.yml: %v", err)
			}

			_, err := GetAllSettingsOptions()
			if err == nil {
				t.Fatal("expected GetAllSettingsOptions to fail")
			}
			if err.Error() != tc.want {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
