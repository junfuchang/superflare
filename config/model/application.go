package model

// Application Data Model
type Application struct {
	Title                      string `yaml:"Title"`
	Footer                     string `yaml:"Footer"`
	SiteIcon                   string `yaml:"SiteIcon,omitempty"`
	SiteIconMode               string `yaml:"SiteIconMode,omitempty"`
	OpenAppNewTab              bool   `yaml:"OpenAppNewTab"`
	OpenBookmarkNewTab         bool   `yaml:"OpenBookmarkNewTab"`
	ShowTitle                  bool   `yaml:"ShowTitle"`
	Greetings                  string `yaml:"Greetings"`
	ShowSearchComponent        bool   `yaml:"ShowSearchComponent"`
	DisabledSearchAutoFocus    bool   `yaml:"DisabledSearchAutoFocus"`
	SearchMode                 string `yaml:"SearchMode,omitempty"`
	SearchEngine               string `yaml:"SearchEngine,omitempty"`
	SearchEngineOpenMode       string `yaml:"SearchEngineOpenMode,omitempty"`
	SearchEngineCustomTemplate string `yaml:"SearchEngineCustomTemplate,omitempty"`
	ShowDateTime               bool   `yaml:"ShowDateTime"`
	ShowApps                   bool   `yaml:"ShowApps"`
	ShowFavorites              bool   `yaml:"ShowFavorites"`
	ShowBookmarks              bool   `yaml:"ShowBookmarks"`
	AppsTitle                  string `yaml:"AppsTitle,omitempty"`
	FavoritesTitle             string `yaml:"FavoritesTitle,omitempty"`
	BookmarksTitle             string `yaml:"BookmarksTitle,omitempty"`
	BookmarkCategoryColor      string `yaml:"BookmarkCategoryColor,omitempty"`
	BookmarkItemColor          string `yaml:"BookmarkItemColor,omitempty"`
	HideSettingsButton         bool   `yaml:"HideSettingButton"`
	HideHelpButton             bool   `yaml:"HideHelpButton"`
	HideWarningsButton         bool   `yaml:"HideWarningsButton"`
	Theme                      string `yaml:"Theme"`
	ThemeBase                  string `yaml:"ThemeBase,omitempty"`
	CustomThemeBackground      string `yaml:"CustomThemeBackground,omitempty"`
	CustomThemePrimary         string `yaml:"CustomThemePrimary,omitempty"`
	CustomThemeAccent          string `yaml:"CustomThemeAccent,omitempty"`
	EnableEncryptedLink        bool   `yaml:"EnableEncryptedLink"`
	IconMode                   string `yaml:"IconMode"`
	KeepLetterCase             bool   `yaml:"KeepLetterCase"`
	Locale                     string `yaml:"Locale"`
	LoginUser                  string `yaml:"LoginUser,omitempty"`
	LoginPass                  string `yaml:"LoginPass,omitempty"`
	BackgroundImage            string `yaml:"BackgroundImage,omitempty"`
	BackgroundImageMode        string `yaml:"BackgroundImageMode,omitempty"`
	BackgroundBlur             int    `yaml:"BackgroundBlur,omitempty"`
	BackgroundOpacity          int    `yaml:"BackgroundOpacity,omitempty"`
	GlassEffect                string `yaml:"GlassEffect,omitempty"`
	GlassIntensity             int    `yaml:"GlassIntensity,omitempty"`
	HomeMaxColumns             int    `yaml:"HomeMaxColumns,omitempty"`
	HomeMaxWidth               int    `yaml:"HomeMaxWidth,omitempty"`
}
