package data

import (
	"fmt"
	"log"

	"gopkg.in/yaml.v2"

	"github.com/junfuchang/superflare/config/model"
)

func initAppConfig(filePath string) (result model.Application, err error) {
	out := []byte(`
# Application title
Title: "superflare"
# Application footer HTML/text
Footer: ""
# MDI site icon name. Leave empty to use /favicon.ico.
SiteIcon: ""
SiteIconMode: "mdi"
# Open links in a new tab
OpenAppNewTab: true
OpenBookmarkNewTab: true
# Home modules
ShowTitle: true
Greetings: "hello"
ShowSearchComponent: true
DisabledSearchAutoFocus: false
SearchMode: "bookmarks"
SearchEngine: "bing"
SearchEngineOpenMode: "same-tab"
SearchEngineCustomTemplate: ""
ShowDateTime: true
ShowApps: true
ShowBookmarks: true
AppsTitle: ""
BookmarksTitle: ""
BookmarkCategoryColor: ""
BookmarkItemColor: ""
# Home toolbar buttons
HideSettingButton: false
HideHelpButton: false
HideWarningsButton: false
# Link display
EnableEncryptedLink: false
IconMode: "FILLING"
KeepLetterCase: false
# Theme
Theme: "blackboard"
ThemeBase: "blackboard"
CustomThemeBackground: ""
CustomThemePrimary: ""
CustomThemeAccent: ""
# UI language (zh / en)
Locale: "zh"
# Login config. Leave empty to use environment variables, CLI flags, or generated defaults.
LoginUser: ""
LoginPass: ""
# Home background image
BackgroundImage: ""
BackgroundImageMode: "url"
BackgroundBlur: 0
BackgroundOpacity: 100
GlassEffect: "none"
GlassIntensity: 0
# Home layout
HomeMaxColumns: 0
HomeMaxWidth: 0
`)

	if err := saveFile(filePath, out); err != nil {
		log.Println("init default app config failed")
		return result, fmt.Errorf("init default app config failed: %w", err)
	}

	parseErr := yaml.Unmarshal(out, &result)
	if parseErr != nil {
		return result, fmt.Errorf("parse default app config failed: %w", parseErr)
	}

	return result, nil
}

func saveAppConfigToYamlFile(name string, result model.Application) error {
	out, err := yaml.Marshal(result)
	if err != nil {
		log.Println("marshal app config failed")
		return fmt.Errorf("marshal app config failed: %w", err)
	}

	filePath, err := configPath(name)
	if err != nil {
		return err
	}
	if err := saveFile(filePath, out); err != nil {
		log.Println("save app config failed")
		return fmt.Errorf("save app config failed: %w", err)
	}
	invalidateFileCachePath(filePath)
	return nil
}

func loadAppConfigFromYaml(name string) (model.Application, error) {
	var result model.Application
	filePath, err := configPath(name)
	if err != nil {
		return result, err
	}

	exists, err := pathExists(filePath)
	if err != nil {
		return result, fmt.Errorf("stat config %s failed: %w", name, err)
	}
	if !exists {
		return result, fmt.Errorf("config %s is missing", name)
	}
	configFile, err := readFileCached(filePath, func() ([]byte, error) { return readFile(filePath) })
	if err != nil {
		return result, fmt.Errorf("read config %s failed: %w", name, err)
	}
	parseErr := yaml.Unmarshal(configFile, &result)
	if parseErr != nil {
		return result, fmt.Errorf("parse config %s failed: %w", name, parseErr)
	}
	return result, nil
}

func LoadAppConfigFromRaw(raw []byte) (model.Application, error) {
	var result model.Application
	if err := yaml.Unmarshal(raw, &result); err != nil {
		return result, fmt.Errorf("parse config raw failed: %w", err)
	}
	if err := validateSettingsOptions(result); err != nil {
		return result, err
	}
	return result, nil
}
