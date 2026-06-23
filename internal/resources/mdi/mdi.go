package mdi

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"path"
	"sort"
	"strings"
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

//go:embed mdi-cheat-sheets
var MdiExampleAssets embed.FS

func Init() error {
	MemFs = memfs.New()
	err := MemFs.MkdirAll(_ASSETS_BASE_DIR, 0777)
	if err != nil {
		return err
	}
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

func GetIconURLByName(name string) string {
	iconName := normalizeIconName(name)
	if iconName == "" {
		return _DEFAULT_FAVICON
	}
	icon := iconMap[iconName]
	if icon == "" || MemFs == nil {
		return _DEFAULT_FAVICON
	}
	cacheKey := define.ThemeCurrent + "-" + iconName
	svgFile := path.Join(_ASSETS_BASE_DIR, cacheKey+".svg")
	if _CACHE_MDI_ICON_EXIST == nil {
		_CACHE_MDI_ICON_EXIST = make(map[string]bool)
	}
	if !_CACHE_MDI_ICON_EXIST[cacheKey] {
		content := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="` + icon + `" style="fill:` + define.ThemePrimaryColor + `;"></path></svg>`
		if err := MemFs.WriteFile(svgFile, []byte(content), 0755); err != nil {
			log.Println("cache mdi favicon error:", err)
			return _DEFAULT_FAVICON
		}
		_CACHE_MDI_ICON_EXIST[cacheKey] = true
	}
	svgURL := "/" + svgFile
	if define.AppFlags.DebugMode {
		svgURL += "?v=dev"
	}
	return svgURL
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
	content := ""
	if _CACHE_MDI_ICON_EXIST == nil {
		_CACHE_MDI_ICON_EXIST = make(map[string]bool)
	}
	if _CACHE_MDI_ICON_DATA == nil {
		_CACHE_MDI_ICON_DATA = make(map[string]string)
	}
	if define.AppFlags.EnableMinimumRequest {
		cacheKey := "inline-" + iconName
		if !_CACHE_MDI_ICON_EXIST[cacheKey] {
			content = `<svg viewBox="0 0 24 24"><path d="` + icon + `" style="fill: var(--color-primary);"></path></svg>`
			_CACHE_MDI_ICON_DATA[cacheKey] = content
			_CACHE_MDI_ICON_EXIST[cacheKey] = true
		}
		return _CACHE_MDI_ICON_DATA[cacheKey]
	}
	if MemFs == nil {
		return _EMPTY_ICON
	}
	cacheKey := define.ThemeCurrent + "-" + iconName
	svgFile := path.Join(_ASSETS_BASE_DIR, cacheKey+".svg")
	if !_CACHE_MDI_ICON_EXIST[cacheKey] {
		content = `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="` + icon + `" style="fill:` + define.ThemePrimaryColor + `;"></path></svg>`
		err := MemFs.WriteFile(svgFile, []byte(content), 0755)
		if err != nil {
			log.Println("缓存内置图标出错:", err)
		}
		_, err = fs.ReadFile(MemFs, svgFile)
		if err != nil {
			log.Println("读取内置图标缓存出错:", err)
			return _EMPTY_ICON
		}
		_CACHE_MDI_ICON_EXIST[cacheKey] = true
	}
	svgURL := "/" + svgFile
	if define.AppFlags.DebugMode {
		svgURL += "?v=dev"
	}
	return `<img src="` + svgURL + `" width="68" height="68" alt="">`
}
