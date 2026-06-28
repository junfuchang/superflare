package mdi

import (
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"path"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/labstack/echo/v5"
	"github.com/soulteary/memfs"

	"github.com/junfuchang/superflare/config/define"
)

var MemFs *memfs.FS

const _ASSETS_BASE_DIR = "assets/mdi"
const _ASSETS_WEB_URI = "/" + _ASSETS_BASE_DIR

var _CACHE_MDI_ICON_EXIST map[string]bool
var _CACHE_MDI_ICON_DATA map[string]string
var cacheMu sync.RWMutex

//go:embed mdi-cheat-sheets
var MdiExampleAssets embed.FS

func Init() error {
	MemFs = memfs.New()
	err := MemFs.MkdirAll(_ASSETS_BASE_DIR, 0777)
	if err != nil {
		return err
	}
	cacheMu.Lock()
	defer cacheMu.Unlock()
	_CACHE_MDI_ICON_EXIST = make(map[string]bool)
	_CACHE_MDI_ICON_DATA = make(map[string]string)
	return nil
}

func RegisterRouting(e *echo.Echo) {
	e.GET(_ASSETS_WEB_URI+"/*", serveGeneratedIcon)
	if mdiExample, err := fs.Sub(MdiExampleAssets, "mdi-cheat-sheets"); err == nil {
		e.StaticFS(define.RegularPages.Icons.Path, mdiExample)
	}
}

func serveGeneratedIcon(c *echo.Context) error {
	fileName := strings.TrimPrefix(path.Clean("/"+c.Param("*")), "/")
	if fileName == "." || fileName == "" || strings.Contains(fileName, "/") {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	data, err := fs.ReadFile(MemFs, path.Join(_ASSETS_BASE_DIR, fileName))
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	return c.Blob(http.StatusOK, "image/svg+xml", data)
}

const _EMPTY_ICON = ""
const _DEFAULT_FAVICON = "/favicon.ico"
const fallbackThemePrimaryColor = "#FFFDEA"

func IconNames() []string {
	names := make([]string, 0, len(iconMap))
	for name := range iconMap {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func normalizeIconName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	var b strings.Builder
	b.Grow(len(name))
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

func iconSVGContent(icon string, fill string) string {
	return `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="` + icon + `" style="fill:` + fill + `;"></path></svg>`
}

func themedIconFillColor() string {
	fill := strings.TrimSpace(define.ThemePrimaryColor)
	if fill == "" {
		return fallbackThemePrimaryColor
	}
	return fill
}

func inlineIconMarkup(icon string) string {
	return `<svg viewBox="0 0 24 24"><path d="` + icon + `" style="fill: var(--color-primary);"></path></svg>`
}

func GetIconSVGDataByName(name string) ([]byte, error) {
	iconName := normalizeIconName(name)
	if iconName == "" {
		return nil, fmt.Errorf("icon name is empty")
	}
	icon := iconMap[iconName]
	if icon == "" {
		return nil, fmt.Errorf("icon %q not found", name)
	}
	return []byte(iconSVGContent(icon, themedIconFillColor())), nil
}

func themeCacheNamespace() string {
	themeName := strings.TrimSpace(define.ThemeCurrent)
	if themeName == "" {
		return "default"
	}
	if themeName != "custom" {
		return themeName
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(define.ThemePrimaryColor)))
	return themeName + "-" + fmt.Sprintf("%x", sum[:4])
}

func themeCacheKey(iconName string) string {
	return themeCacheNamespace() + "-" + iconName
}

func ensureCacheMapsLocked() {
	if _CACHE_MDI_ICON_EXIST == nil {
		_CACHE_MDI_ICON_EXIST = make(map[string]bool)
	}
	if _CACHE_MDI_ICON_DATA == nil {
		_CACHE_MDI_ICON_DATA = make(map[string]string)
	}
}

func iconCacheExists(key string) bool {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return _CACHE_MDI_ICON_EXIST[key]
}

func getInlineCacheValue(key string) string {
	cacheMu.RLock()
	defer cacheMu.RUnlock()
	return _CACHE_MDI_ICON_DATA[key]
}

func setInlineCacheValue(key string, value string) {
	cacheMu.Lock()
	defer cacheMu.Unlock()
	ensureCacheMapsLocked()
	_CACHE_MDI_ICON_DATA[key] = value
	_CACHE_MDI_ICON_EXIST[key] = true
}

func GetIconURLByName(name string) string {
	iconName := normalizeIconName(name)
	if iconName == "" {
		return _DEFAULT_FAVICON
	}
	icon := iconMap[iconName]
	if icon == "" || MemFs == nil {
		return _DEFAULT_FAVICON
	}
	cacheKey := themeCacheKey(iconName)
	svgFile := path.Join(_ASSETS_BASE_DIR, cacheKey+".svg")
	if !iconCacheExists(cacheKey) {
		cacheMu.Lock()
		ensureCacheMapsLocked()
		if !_CACHE_MDI_ICON_EXIST[cacheKey] {
			content := iconSVGContent(icon, themedIconFillColor())
			if err := MemFs.WriteFile(svgFile, []byte(content), 0755); err != nil {
				cacheMu.Unlock()
				log.Println("cache mdi favicon error:", err)
				return _DEFAULT_FAVICON
			}
			_CACHE_MDI_ICON_EXIST[cacheKey] = true
		}
		cacheMu.Unlock()
	}
	svgURL := "/" + svgFile
	if define.AppFlags.DebugMode {
		svgURL += "?v=dev"
	}
	return svgURL
}

func IconExists(name string) bool {
	iconName := normalizeIconName(name)
	if iconName == "" {
		return false
	}
	_, ok := iconMap[iconName]
	return ok
}

func GetIconByName(name string) string {
	iconName := normalizeIconName(name)
	if iconName == "" {
		return _EMPTY_ICON
	}
	icon := iconMap[iconName]
	if icon == "" {
		return _EMPTY_ICON
	}
	if define.AppFlags.EnableMinimumRequest {
		cacheKey := "inline-" + iconName
		if !iconCacheExists(cacheKey) {
			setInlineCacheValue(cacheKey, inlineIconMarkup(icon))
		}
		return getInlineCacheValue(cacheKey)
	}
	if MemFs == nil {
		return inlineIconMarkup(icon)
	}
	cacheKey := themeCacheKey(iconName)
	svgFile := path.Join(_ASSETS_BASE_DIR, cacheKey+".svg")
	if !iconCacheExists(cacheKey) {
		cacheMu.Lock()
		ensureCacheMapsLocked()
		if !_CACHE_MDI_ICON_EXIST[cacheKey] {
			content := iconSVGContent(icon, themedIconFillColor())
			err := MemFs.WriteFile(svgFile, []byte(content), 0755)
			if err != nil {
				cacheMu.Unlock()
				log.Println("cache builtin icon failed:", err)
				return inlineIconMarkup(icon)
			}
			_, err = fs.ReadFile(MemFs, svgFile)
			if err != nil {
				cacheMu.Unlock()
				log.Println("read cached builtin icon failed:", err)
				return inlineIconMarkup(icon)
			}
			_CACHE_MDI_ICON_EXIST[cacheKey] = true
		}
		cacheMu.Unlock()
	}
	svgURL := "/" + svgFile
	if define.AppFlags.DebugMode {
		svgURL += "?v=dev"
	}
	return `<img src="` + svgURL + `" width="68" height="68" alt="">`
}
