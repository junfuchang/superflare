package guide

import (
	"embed"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/soulteary/memfs"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/statuspage"
)

var MemFs *memfs.FS
var getGuideHTML = fn.GetHTML

const _ASSETS_BASE_DIR = "assets/guide"
const _ASSETS_WEB_URI = "/" + _ASSETS_BASE_DIR

//go:embed guide-assets
var IntroAssets embed.FS

func Init() error {
	MemFs = memfs.New()
	err := MemFs.MkdirAll(_ASSETS_BASE_DIR, 0777)
	if err != nil {
		return err
	}
	return nil
}

func RegisterRouting(e *echo.Echo) {
	if define.AppFlags.DebugMode && registerLocalVendorAssets(e) {
		// Local development can run without copying generated assets first.
	} else if introAssets, err := fs.Sub(IntroAssets, "guide-assets"); err == nil {
		e.StaticFS(_ASSETS_WEB_URI, introAssets)
	}
	e.GET(define.RegularPages.Guide.Path, render)
}

func registerLocalVendorAssets(e *echo.Echo) bool {
	const vendorAssets = "embed/assets/vendor/guide-assets"
	if stat, err := os.Stat(vendorAssets); err == nil && stat.IsDir() {
		e.Static(_ASSETS_WEB_URI, vendorAssets)
		return true
	}
	return false
}

func render(c *echo.Context) error {
	if err := statuspage.BindCurrentOptions(c); err != nil {
		return statuspage.HTML(c, http.StatusInternalServerError, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusInternalServerError, err.Error()))
	}
	body, err := getUserHomePage()
	if err != nil {
		return statuspage.HTML(c, http.StatusBadGateway, statuspage.BuildHTTPErrorPage(statuspage.CurrentLocale(c), http.StatusBadGateway, err.Error()))
	}
	return c.HTMLBlob(http.StatusOK, []byte(body))
}

func getUserHomePage() (string, error) {
	port := strconv.Itoa(define.AppFlags.Port)
	body, err := getGuideHTML("http://localhost:" + port + "/")
	if err != nil {
		return "", err
	}
	if !strings.Contains(body, `id="page-home"`) {
		return "", errors.New("guide source page is not the home page")
	}
	content, err := injectGuideAssets(body)
	if err != nil {
		return "", err
	}
	return content, nil
}

func injectGuideAssets(body string) (string, error) {
	ruleHead := regexp.MustCompile("</head>")
	ruleBody := regexp.MustCompile("</body>")
	rulePageView := regexp.MustCompile(`class="pageview"`)
	if !ruleHead.MatchString(body) || !ruleBody.MatchString(body) {
		return "", errors.New("guide source page structure is incomplete")
	}
	content := ruleHead.ReplaceAllString(body, `<link rel="stylesheet" href="/assets/guide/introjs.min.css"><link rel="stylesheet" href="/assets/guide/app.css"><script src="/assets/guide/intro.min.js"></script></head>`)
	content = ruleBody.ReplaceAllString(content, `<script src="/assets/guide/app.js"></script></body>`)
	content = rulePageView.ReplaceAllString(content, `class="pageview" style="position:inherit;"`)
	return content, nil
}
