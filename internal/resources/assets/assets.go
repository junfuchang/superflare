package assets

import (
	"crypto/md5" //#nosec
	"embed"
	"fmt"
	"io/fs"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/labstack/echo/v5"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/internal/background"
	"github.com/junfuchang/superflare/internal/fn"
)

//go:embed favicon.ico
var Favicon embed.FS

func RegisterRouting(e *echo.Echo) {
	e.Use(optimizeResourceCacheTime())

	e.GET("/favicon.ico", func(c *echo.Context) error {
		c.Response().Header().Set("Cache-Control", "public, max-age=31536000")
		data, err := fs.ReadFile(Favicon, "favicon.ico")
		if err != nil {
			return err
		}
		return c.Blob(http.StatusOK, "image/x-icon", data)
	})

	if define.AppFlags.DebugMode {
		e.Static("/assets/css", "embed/assets/css")
	}
	e.GET(background.RemoteAssetPath, serveRemoteBackground)
	e.GET("/assets/site-icons", serveSiteFavicon)
	e.GET(background.UploadedFullPath, serveUploadedBackground)
	e.GET(background.UploadedPreviewPath, serveUploadedBackgroundPreview)
	e.GET("/user-assets/:file", serveUserAsset)
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
	if define.AppFlags.DebugMode {
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
	root, err := os.Getwd()
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
	if err != nil {
		c.Response().Header().Set("Cache-Control", "no-store")
		c.Response().Header().Del("ETag")
		fallback, readErr := fs.ReadFile(Favicon, "favicon.ico")
		if readErr != nil {
			return err
		}
		return c.Blob(http.StatusOK, "image/x-icon", fallback)
	}

	if define.AppFlags.DebugMode {
		c.Response().Header().Set("Cache-Control", "no-store")
	} else {
		c.Response().Header().Set("Cache-Control", "public, max-age=604800")
	}
	c.Response().Header().Del("ETag")
	return c.Blob(http.StatusOK, contentType, data)
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

	if define.AppFlags.DebugMode {
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
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			uri := c.Request().RequestURI
			if strings.HasPrefix(uri, background.RemoteAssetPath) {
				return next(c)
			}
			if strings.HasPrefix(uri, "/assets/site-icons") {
				return next(c)
			}
			if strings.HasPrefix(uri, "/assets/") || strings.HasPrefix(uri, "/favicon.ico") {
				if define.AppFlags.DebugMode {
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
