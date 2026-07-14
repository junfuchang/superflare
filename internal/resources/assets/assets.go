package assets

import (
	"crypto/md5" //#nosec
	"crypto/sha256"
	"embed"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/background"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/resources/mdi"
)

//go:embed favicon.ico icons/favicon/*
var Favicon embed.FS

const (
	faviconRoutePath          = "/favicon.ico"
	appleTouchIconRoutePath   = "/apple-touch-icon.png"
	androidChrome192RoutePath = "/android-chrome-192x192.png"
	androidChrome512RoutePath = "/android-chrome-512x512.png"
	siteIconStateHeader       = "X-SuperFlare-Site-Icon"
)

type assetsRuntimeSnapshot struct {
	DebugMode bool
}

type assetsRuntimeHolder struct {
	mu  sync.RWMutex
	set bool
	cfg assetsRuntimeSnapshot
}

func (h *assetsRuntimeHolder) Load() assetsRuntimeSnapshot {
	if h == nil {
		return assetsRuntimeSnapshot{}
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if !h.set {
		return assetsRuntimeSnapshot{}
	}
	return h.cfg
}

func (h *assetsRuntimeHolder) Store(cfg assetsRuntimeSnapshot) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.set = true
	h.cfg = cfg
	h.mu.Unlock()
}

var assetsRuntimeFlags = &assetsRuntimeHolder{}

func assetsRuntimeSnapshotFromFlags(flags model.Flags) assetsRuntimeSnapshot {
	return assetsRuntimeSnapshot{DebugMode: flags.DebugMode}
}

func currentAssetsRuntime() assetsRuntimeSnapshot {
	assetsRuntimeFlags.mu.RLock()
	hasValue := assetsRuntimeFlags.set
	cfg := assetsRuntimeFlags.cfg
	assetsRuntimeFlags.mu.RUnlock()
	if hasValue {
		return cfg
	}
	cfg = assetsRuntimeSnapshotFromFlags(define.CurrentAppRuntimeFlags())
	assetsRuntimeFlags.Store(cfg)
	return cfg
}

func SetRuntimeFlags(flags model.Flags) {
	assetsRuntimeFlags.Store(assetsRuntimeSnapshotFromFlags(flags))
}

func RegisterRouting(e *echo.Echo) {
	runtime := currentAssetsRuntime()
	e.Use(optimizeResourceCacheTime())

	e.GET(faviconRoutePath, serveEmbeddedWebsiteIcon("favicon.ico", "image/x-icon"))
	e.GET(appleTouchIconRoutePath, serveEmbeddedWebsiteIcon("icons/favicon/apple-touch-icon.png", "image/png"))
	e.GET(androidChrome192RoutePath, serveEmbeddedWebsiteIcon("icons/favicon/android-chrome-192x192.png", "image/png"))
	e.GET(androidChrome512RoutePath, serveEmbeddedWebsiteIcon("icons/favicon/android-chrome-512x512.png", "image/png"))

	if runtime.DebugMode {
		e.Static("/assets/css", "embed/assets/css")
	}
	e.GET(background.RemoteAssetPath, serveRemoteBackground)
	e.GET("/assets/site-icons", serveSiteFavicon)
	e.GET(background.UploadedFullPath, serveUploadedBackground)
	e.GET(background.UploadedPreviewPath, serveUploadedBackgroundPreview)
	e.GET("/user-assets/:file", serveUserAsset)
}

func SiteIconURL(iconURLResolver func(string) string, name string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return iconURLResolver(name)
	}
	return versionedIconURL(faviconRoutePath, "favicon.ico")
}

func AppleTouchIconURL() string {
	return versionedIconURL(appleTouchIconRoutePath, "icons/favicon/apple-touch-icon.png")
}

func AndroidChrome192URL() string {
	return versionedIconURL(androidChrome192RoutePath, "icons/favicon/android-chrome-192x192.png")
}

func AndroidChrome512URL() string {
	return versionedIconURL(androidChrome512RoutePath, "icons/favicon/android-chrome-512x512.png")
}

func versionedIconURL(routePath string, assetPath string) string {
	version := embeddedAssetVersion(assetPath)
	if version == "" {
		return routePath
	}
	return routePath + "?v=" + version
}

func embeddedAssetVersion(assetPath string) string {
	data, err := fs.ReadFile(Favicon, assetPath)
	if err != nil || len(data) == 0 {
		return ""
	}
	sum := sha256.Sum256(data)
	return fmt.Sprintf("%x", sum[:6])
}

func serveEmbeddedWebsiteIcon(assetPath string, contentType string) echo.HandlerFunc {
	runtime := currentAssetsRuntime()
	return func(c *echo.Context) error {
		data, err := fs.ReadFile(Favicon, assetPath)
		if err != nil {
			return err
		}
		if runtime.DebugMode {
			c.Response().Header().Set("Cache-Control", "no-store")
		} else {
			c.Response().Header().Set("Cache-Control", "public, max-age=604800, immutable")
		}
		c.Response().Header().Del("ETag")
		return c.Blob(http.StatusOK, contentType, data)
	}
}

func serveUploadedBackground(c *echo.Context) error {
	return serveUploadedBackgroundVariant(c, "full")
}

func serveUploadedBackgroundPreview(c *echo.Context) error {
	return serveUploadedBackgroundVariant(c, "preview")
}

func serveUploadedBackgroundVariant(c *echo.Context, variant string) error {
	data, contentType, err := background.FetchUploadedVariant(variant)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if currentAssetsRuntime().DebugMode {
		c.Response().Header().Set("Cache-Control", "no-store")
	} else {
		c.Response().Header().Set("Cache-Control", "public, max-age=604800")
	}
	return c.Blob(http.StatusOK, contentType, data)
}

func serveUserAsset(c *echo.Context) error {
	fileName := filepath.Base(c.Param("file"))
	return serveUserAssetByName(c, fileName)
}

func serveUserAssetByName(c *echo.Context, fileName string) error {
	if fileName == "." || fileName == "" {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	root, err := fn.GetWorkDirE()
	if err != nil {
		return echo.NewHTTPError(http.StatusInternalServerError, "server error")
	}
	filePath := filepath.Join(root, "var", "uploads", fileName)
	content, err := os.ReadFile(filePath)
	if err != nil {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	contentType := mime.TypeByExtension(filepath.Ext(fileName))
	if contentType == "" {
		contentType = http.DetectContentType(content)
	}
	c.Response().Header().Set("Cache-Control", "no-store")
	return c.Blob(http.StatusOK, contentType, content)
}

func serveSiteFavicon(c *echo.Context) error {
	iconURL := strings.TrimSpace(c.QueryParam("src"))
	if iconURL == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing site favicon source")
	}

	data, contentType, err := fn.FetchPublicSiteFavicon(iconURL)
	if err == nil {
		if currentAssetsRuntime().DebugMode {
			c.Response().Header().Set("Cache-Control", "no-store")
		} else {
			c.Response().Header().Set("Cache-Control", "public, max-age=604800")
		}
		c.Response().Header().Set(siteIconStateHeader, "cached")
		c.Response().Header().Del("ETag")
		return c.Blob(http.StatusOK, contentType, data)
	}

	c.Response().Header().Set("Cache-Control", "no-store")
	c.Response().Header().Set(siteIconStateHeader, "fallback")
	c.Response().Header().Del("ETag")
	fallback, fallbackContentType, fallbackErr := readBuiltinBookmarkIcon()
	if fallbackErr != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "site favicon fetch failed")
	}
	return c.Blob(http.StatusOK, fallbackContentType, fallback)
}

func readBuiltinBookmarkIcon() ([]byte, string, error) {
	data, err := mdi.GetIconSVGDataByName("bookmark")
	if err != nil {
		return nil, "", err
	}
	return data, "image/svg+xml", nil
}

func serveRemoteBackground(c *echo.Context) error {
	source := strings.TrimSpace(c.QueryParam("src"))
	if source == "" {
		return echo.NewHTTPError(http.StatusBadRequest, "missing background source")
	}

	data, contentType, err := background.FetchRemoteVariant(source, c.QueryParam("variant"))
	if err != nil {
		return echo.NewHTTPError(http.StatusBadGateway, "background fetch failed")
	}

	if currentAssetsRuntime().DebugMode {
		c.Response().Header().Set("Cache-Control", "no-store")
	} else {
		c.Response().Header().Set("Cache-Control", "public, max-age=604800")
	}
	c.Response().Header().Del("ETag")
	return c.Blob(http.StatusOK, contentType, data)
}

// optimizeResourceCacheTime sets cache headers for assets and supports 304.
func optimizeResourceCacheTime() echo.MiddlewareFunc {
	data := []byte(time.Now().String())
	/* #nosec */
	etag := fmt.Sprintf("W/%x", md5.Sum(data))
	runtime := currentAssetsRuntime()
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			uri := c.Request().RequestURI
			if strings.HasPrefix(uri, background.RemoteAssetPath) {
				return next(c)
			}
			if strings.HasPrefix(uri, "/assets/site-icons") {
				return next(c)
			}
			if strings.HasPrefix(uri, "/assets/") {
				if runtime.DebugMode {
					c.Response().Header().Set("Cache-Control", "no-store")
				} else {
					c.Response().Header().Set("Cache-Control", "public, max-age=31536000")
					c.Response().Header().Set("ETag", etag)
					if match := c.Request().Header.Get("If-None-Match"); match != "" && strings.Contains(match, etag) {
						return c.NoContent(http.StatusNotModified)
					}
				}
			}
			return next(c)
		}
	}
}
