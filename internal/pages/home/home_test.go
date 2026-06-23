package home

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/background"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
)

func saveAppFlags() model.Flags { return define.AppFlags }

func restoreAppFlags(f model.Flags) {
	define.AppFlags = f
}

func TestSetCSPHeader_WhenDisableCSPFalse_SetsHeader(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableCSP = false

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	setCSPHeader(c, "nonce-value")

	assert.Equal(t, "script-src 'nonce-nonce-value'; "+_cspValue, rec.Header().Get("Content-Security-Policy"))
}

func TestSetCSPHeader_WhenDisableCSPTrue_NoHeader(t *testing.T) {
	orig := saveAppFlags()
	defer restoreAppFlags(orig)
	define.AppFlags.DisableCSP = true

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	setCSPHeader(c, "")

	assert.Empty(t, rec.Header().Get("Content-Security-Policy"))
}

func TestGetCSPValueWithoutScriptNonce(t *testing.T) {
	assert.Equal(t, _cspScriptNone+_cspValue, getCSPValue(""))
}

func TestCustomHomeStyleWithBackgroundAssets(t *testing.T) {
	options := model.Application{
		BackgroundImage:   background.UploadedFullPath,
		BackgroundOpacity: 85,
		BackgroundBlur:    12,
	}
	assets := background.Assets{
		Enabled:        true,
		PreviewURL:     background.UploadedPreviewPath,
		PreviewDataURL: "data:image/jpeg;base64,abc",
		FullURL:        background.UploadedFullPath,
		AccentColor:    "#3478c0",
	}

	html := string(customHomeStyle(options, assets))
	assert.Contains(t, html, ".page-background-preview")
	assert.Contains(t, html, ".page-background-full")
	assert.Contains(t, html, "opacity:0.85")
	assert.Contains(t, html, "body.has-preview-background")
	assert.Contains(t, html, "body{--scrollbar-accent:#3478c0;}")
	assert.NotContains(t, html, "transition:opacity")
}

func TestRenderBackgroundHTMLUsesPreviewDataURL(t *testing.T) {
	html := string(renderBackgroundHTML(background.Assets{
		Enabled:        true,
		PreviewURL:     "/user-assets/background-preview",
		PreviewDataURL: "data:image/jpeg;base64,abc",
		FullURL:        "/user-assets/background",
	}))

	assert.Contains(t, html, `class="page-background-preview"`)
	assert.Contains(t, html, `data:image/jpeg;base64,abc`)
	assert.Contains(t, html, `src="/user-assets/background"`)
}

func TestPageAppearanceUsesPreviewDataURL(t *testing.T) {
	style := string(pageAppearance(model.Application{BackgroundBlur: 12}, background.Assets{
		Enabled:        true,
		PreviewDataURL: "data:image/jpeg;base64,abc",
	}))

	assert.Contains(t, style, "background-image:linear-gradient(")
	assert.Contains(t, style, "url('data:image/jpeg;base64,abc')")
	assert.Contains(t, style, "background-size:cover")
	assert.NotContains(t, style, "background-attachment:fixed")
}

func TestAppendAdaptiveColumnStyleUsesDesktopWrapAndMobileWaterfall(t *testing.T) {
	var b strings.Builder
	appendAdaptiveColumnStyle(&b, 4)
	style := b.String()

	assert.Contains(t, style, "@media (min-width:1201px){#page-home.pageview .container{padding-left:clamp(40px,4vw,250px);padding-right:clamp(40px,4vw,250px);}}")
	assert.Contains(t, style, "#container-apps .apps-container{display:grid;grid-template-columns:repeat(auto-fill,minmax(max(180px,calc((100% - (4 - 1) * 18px) / 4)),1fr));column-gap:18px;row-gap:0;align-items:start;}")
	assert.Contains(t, style, "#container-bookmakrs .bookmark-groups{display:grid;grid-template-columns:repeat(auto-fill,minmax(max(180px,calc((100% - (4 - 1) * 18px) / 4)),1fr));column-count:auto;column-gap:18px;gap:18px;align-items:start;}")
	assert.Contains(t, style, "#container-bookmakrs .bookmark-group-container{break-inside:auto;display:block;width:auto;max-width:none;min-width:0;")
	assert.Contains(t, style, "@media (max-width:767px){#container-bookmakrs .bookmark-groups{display:block;column-count:2;column-gap:18px;}")
	assert.NotContains(t, style, ";};}")
}
