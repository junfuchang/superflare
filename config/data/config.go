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
# Link display
EnableEncryptedLink: false
IconMode: "FILLING"
KeepLetterCase: false
# Theme
Theme: "blackboard"
CustomThemeBackground: "rgba(26, 26, 26, 1)"
CustomThemePrimary: "rgba(255, 253, 234, 1)"
CustomThemeAccent: "rgba(92, 92, 92, 1)"
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

	ok := saveFile(filePath, out)
	if !ok {
		log.Println("init default app config failed")
		return result, fmt.Errorf("init default app config failed: %s", filePath)
	}

	parseErr := yaml.Unmarshal(out, &result)
	if parseErr != nil {
		return result, fmt.Errorf("parse default app config failed: %w", parseErr)
	}

	return result, nil
}

func saveAppConfigToYamlFile(name string, result model.Application) bool {
	out, err := yaml.Marshal(result)
	if err != nil {
		log.Println("marshal app config failed")
		return false
	}

	filePath := getConfigPath(name)
	ok := saveFile(filePath, out)
	if !ok {
		log.Println("save app config failed")
		return false
	}
	invalidateFileCache(name)
	return true
}

func loadAppConfigFromYaml(name string) (model.Application, error) {
	var result model.Application
	filePath := getConfigPath(name)

	if !checkExists(filePath) {
		fmt.Println("config file not found, creating default config: " + name)
		var createErr error
		result, createErr = initAppConfig(filePath)
		if createErr != nil {
			return result, fmt.Errorf("create app config %s failed: %w", name, createErr)
		}
		return result, nil
	}
	configFile, err := readFileCached(name, func() ([]byte, error) { return readFile(filePath) })
	if err != nil {
		return result, fmt.Errorf("read config %s failed: %w", name, err)
	}
	parseErr := yaml.Unmarshal(configFile, &result)
	if parseErr != nil {
		return result, fmt.Errorf("parse config %s failed: %w", name, parseErr)
	}
	return result, nil
}
