package home

import (
	"crypto/tls"
	"errors"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/background"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/i18n"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
)

func saveAppFlags() model.Flags { return define.AppFlags }

func restoreAppFlags(f model.Flags) {
	define.AppFlags = f
}

func writeEmptyBookmarkFixtures(t *testing.T, dir string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, "apps.yml"), []byte("links: []\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bookmarks.yml"), []byte("categories: []\nlinks: []\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}
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

func TestMaybeMakeScriptNonceDisabled(t *testing.T) {
	nonce, err := maybeMakeScriptNonce(false)
	if err != nil {
		t.Fatalf("maybeMakeScriptNonce(false): %v", err)
	}
	if nonce != "" {
		t.Fatalf("expected empty nonce when disabled, got %q", nonce)
	}
}

func TestMaybeMakeScriptNonceReturnsErrorWhenCryptoRandFails(t *testing.T) {
	original := cryptoRandRead
	cryptoRandRead = func(p []byte) (int, error) {
		return 0, errors.New("entropy unavailable")
	}
	defer func() { cryptoRandRead = original }()

	nonce, err := maybeMakeScriptNonce(true)
	if err == nil {
		t.Fatal("expected nonce generation to fail")
	}
	if nonce != "" {
		t.Fatalf("expected empty nonce on failure, got %q", nonce)
	}
	if !strings.Contains(err.Error(), "generate script nonce failed") {
		t.Fatalf("expected wrapped nonce error, got %v", err)
	}
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

func TestResolveHomeAssetsReturnsErrorWhenUploadedBackgroundMissing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	_, err = resolveHomeAssets(model.Application{
		BackgroundImage:     background.UploadedFullPath,
		BackgroundImageMode: "upload",
	})
	if err == nil {
		t.Fatal("expected uploaded background asset resolution to fail")
	}
	if !strings.Contains(err.Error(), "resolve uploaded background asset failed") {
		t.Fatalf("expected wrapped uploaded background error, got %v", err)
	}
}

func TestPageHomeReturnsStyledErrorWhenUploadedBackgroundMissing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	config := strings.Join([]string{
		"Title: SuperFlare",
		"Locale: zh",
		"Theme: blackboard",
		"BackgroundImage: " + background.UploadedFullPath,
		"BackgroundImageMode: upload",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(config), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links: []\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories: []\nlinks: []\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageHome(c); err != nil {
		t.Fatalf("pageHome: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "500") {
		t.Fatalf("expected 500 status content, got %s", rec.Body.String())
	}
}

func TestPageAppearanceUsesPreviewDataURL(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	styleCSS, styleWarning, err := pageAppearance(model.Application{BackgroundBlur: 12}, background.Assets{
		Enabled:        true,
		PreviewDataURL: "data:image/jpeg;base64,abc",
	})
	if err != nil {
		t.Fatalf("pageAppearance: %v", err)
	}
	if styleWarning != "" {
		t.Fatalf("expected no style warning, got %q", styleWarning)
	}
	style := string(styleCSS)

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

func TestSplitGreetingOptions(t *testing.T) {
	got := splitGreetingOptions(" 你好 ; ; 中午好; 下午好 ; ")
	assert.Equal(t, []string{"你好", "中午好", "下午好"}, got)
}

func TestGetGreeting_EmptyFallsBackToDefault(t *testing.T) {
	assert.Equal(t, "Hello", getGreeting("", "en"))
}

func TestGetGreeting_SingleReturnsValue(t *testing.T) {
	assert.Equal(t, "你好", getGreeting("你好", "zh"))
}

func TestGetGreeting_RandomModeAlwaysReturnsProvidedValue(t *testing.T) {
	options := map[string]bool{
		"甲": false,
		"乙": false,
		"丙": false,
	}
	for i := 0; i < 50; i++ {
		got := getGreeting("甲;乙;丙", "zh")
		_, ok := options[got]
		if !ok {
			t.Fatalf("unexpected random greeting: %q", got)
		}
		options[got] = true
	}
}

func TestGenerateTemplatesUseRequestScopedDynamicURL(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links:\n- name: App A\n  link: \"{origin}/app\"\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories:\n- id: default\n  title: 默认\nlinks:\n- name: Bookmark A\n  category: default\n  link: \"{origin}/bookmark\"\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	options := model.Application{}
	firstURL := fn.ParseRequestURLTo(mustRequestForHost(t, "http://alpha.example.test/"))
	secondURL := fn.ParseRequestURLTo(mustRequestForHost(t, "https://beta.example.test:9443/"))

	const iterations = 40
	errCh := make(chan error, iterations*2)
	var wg sync.WaitGroup
	for i := 0; i < iterations; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			html, err := GenerateApplicationsTemplateWithLocalAndURLErr("", &options, false, &firstURL)
			if err != nil {
				errCh <- fmt.Errorf("applications first request failed: %w", err)
				return
			}
			if !strings.Contains(string(html), "http://alpha.example.test/app") {
				errCh <- assertContainsError("applications first request", string(html), "http://alpha.example.test/app")
			}
		}()
		go func() {
			defer wg.Done()
			html, err := GenerateBookmarkTemplateWithLocalAndURLErr("", &options, false, &secondURL)
			if err != nil {
				errCh <- fmt.Errorf("bookmarks second request failed: %w", err)
				return
			}
			if !strings.Contains(string(html), "https://beta.example.test:9443/bookmark") {
				errCh <- assertContainsError("bookmarks second request", string(html), "https://beta.example.test:9443/bookmark")
			}
		}()
	}
	wg.Wait()
	close(errCh)

	for err := range errCh {
		if err != nil {
			t.Fatal(err)
		}
	}
}

func TestGenerateTemplatesDoNotLoadSettingsWhenOptionsNil(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links:\n- name: App A\n  link: https://app.example.com\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories:\n- id: default\n  title: 默认\nlinks:\n- name: Bookmark A\n  category: default\n  link: https://bookmark.example.com\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write broken config.yml: %v", err)
	}

	applicationsHTML, err := GenerateApplicationsTemplateErr("", nil)
	if err != nil {
		t.Fatalf("GenerateApplicationsTemplateErr: %v", err)
	}
	if html := string(applicationsHTML); !strings.Contains(html, "App A") {
		t.Fatalf("expected applications template to render without loading settings, got %s", html)
	}
	bookmarksHTML, err := GenerateBookmarkTemplateErr("", nil)
	if err != nil {
		t.Fatalf("GenerateBookmarkTemplateErr: %v", err)
	}
	if html := string(bookmarksHTML); !strings.Contains(html, "Bookmark A") {
		t.Fatalf("expected bookmarks template to render without loading settings, got %s", html)
	}
}

func TestGenerateApplicationsTemplateErrReturnsErrorWhenDataBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links: [broken\n"), 0644); err != nil {
		t.Fatalf("write broken apps.yml: %v", err)
	}

	_, err = GenerateApplicationsTemplateErr("", nil)
	if err == nil {
		t.Fatal("expected applications template generation to fail")
	}
}

func TestGenerateBookmarkTemplateErrReturnsErrorWhenDataBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories: [broken\n"), 0644); err != nil {
		t.Fatalf("write broken bookmarks.yml: %v", err)
	}

	_, err = GenerateBookmarkTemplateErr("", nil)
	if err == nil {
		t.Fatal("expected bookmarks template generation to fail")
	}
}

func TestGenerateApplicationsTemplateEscapesHTMLSensitiveFields(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte(strings.Join([]string{
		"links:",
		`- name: 'App "Alpha" <script>alert(1)</script>'`,
		`  link: 'https://app.example.com/?q=<unsafe>'`,
		`  icon: 'mdi" onclick="alert(1)'`,
		`  desc: 'Desc <b>unsafe</b> & more'`,
	}, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}

	html, err := GenerateApplicationsTemplateErr("", &model.Application{})
	if err != nil {
		t.Fatalf("GenerateApplicationsTemplateErr: %v", err)
	}
	got := string(html)

	if strings.Contains(got, `<script>alert(1)</script>`) || strings.Contains(got, `onclick="alert(1)`) || strings.Contains(got, `<b>unsafe</b>`) {
		t.Fatalf("expected application template to escape unsafe html, got %s", got)
	}
	if !strings.Contains(got, `App &#34;Alpha&#34; &lt;script&gt;alert(1)&lt;/script&gt;`) {
		t.Fatalf("expected escaped application name, got %s", got)
	}
	if !strings.Contains(got, `Desc &lt;b&gt;unsafe&lt;/b&gt; &amp; more`) {
		t.Fatalf("expected escaped application description, got %s", got)
	}
}

func TestGenerateApplicationsTemplateOmitsIconContainerWhenIconsHidden(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte(strings.Join([]string{
		"links:",
		`- name: "App A"`,
		`  link: https://app.example.com`,
		`  desc: "Demo"`,
	}, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}

	html, err := GenerateApplicationsTemplateErr("", &model.Application{
		IconMode:      define.IconModeHidden,
		OpenAppNewTab: true,
	})
	if err != nil {
		t.Fatalf("GenerateApplicationsTemplateErr: %v", err)
	}
	got := string(html)

	if strings.Contains(got, `class="app-icon"`) {
		t.Fatalf("expected hidden icon mode to omit icon container, got %s", got)
	}
	if strings.Contains(got, `class="app-text has-icon"`) {
		t.Fatalf("expected hidden icon mode to omit icon spacing class, got %s", got)
	}
	if !strings.Contains(got, `class="app-text"`) {
		t.Fatalf("expected application text container to remain, got %s", got)
	}
}

func TestGenerateApplicationsTemplateKeepsIconContainerWhenIconsVisible(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte(strings.Join([]string{
		"links:",
		`- name: "App A"`,
		`  link: https://app.example.com`,
		`  icon: "bookmark"`,
		`  desc: "Demo"`,
	}, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}

	html, err := GenerateApplicationsTemplateErr("", &model.Application{
		IconMode:      define.IconModeMissingFill,
		OpenAppNewTab: true,
	})
	if err != nil {
		t.Fatalf("GenerateApplicationsTemplateErr: %v", err)
	}
	got := string(html)

	if !strings.Contains(got, `class="app-icon"`) {
		t.Fatalf("expected visible icon mode to keep icon container, got %s", got)
	}
	if !strings.Contains(got, `class="app-text has-icon"`) {
		t.Fatalf("expected visible icon mode to keep icon spacing class, got %s", got)
	}
}

func TestGenerateBookmarkTemplateEscapesHTMLSensitiveFields(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte(strings.Join([]string{
		"categories:",
		`- id: default`,
		`  title: 'Group <img src=x onerror=alert(1)>'`,
		"links:",
		`- name: 'Bookmark "Unsafe" <svg/onload=alert(1)>'`,
		`  category: default`,
		`  link: 'https://bookmark.example.com/?q=<unsafe>'`,
	}, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	html, err := GenerateBookmarkTemplateErr("", &model.Application{})
	if err != nil {
		t.Fatalf("GenerateBookmarkTemplateErr: %v", err)
	}
	got := string(html)

	if strings.Contains(got, `<img src=x onerror=alert(1)>`) || strings.Contains(got, `<svg/onload=alert(1)>`) {
		t.Fatalf("expected bookmark template to escape unsafe html, got %s", got)
	}
	if !strings.Contains(got, `Group &lt;img src=x onerror=alert(1)&gt;`) {
		t.Fatalf("expected escaped category title, got %s", got)
	}
	if !strings.Contains(got, `Bookmark &#34;Unsafe&#34; &lt;svg/onload=alert(1)&gt;`) {
		t.Fatalf("expected escaped bookmark name, got %s", got)
	}
}

func TestGenerateBookmarkTemplatePlacesUncategorizedBookmarksInSeparateGroup(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte(strings.Join([]string{
		"categories:",
		"- id: cat-1",
		`  title: "Category 1"`,
		"- id: cat-2",
		`  title: "Category 2"`,
		"links:",
		`- name: "Grouped One"`,
		`  category: cat-1`,
		`  link: https://grouped.example.com`,
		`- name: "Ungrouped One"`,
		`  link: https://ungrouped.example.com`,
	}, "\n")+"\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	html, err := GenerateBookmarkTemplateErr("", &model.Application{})
	if err != nil {
		t.Fatalf("GenerateBookmarkTemplateErr: %v", err)
	}
	got := string(html)

	if !strings.Contains(got, "Category 1") || !strings.Contains(got, "Ungrouped") && !strings.Contains(got, "未分类") {
		t.Fatalf("expected named and uncategorized groups, got %s", got)
	}
	if !strings.Contains(got, "Ungrouped One") {
		t.Fatalf("expected ungrouped bookmark to be rendered, got %s", got)
	}
	if strings.Contains(got, "Ungrouped One</span></a></li></ul></div><div class=\"bookmark-group-container pull-left\"><h3 class=\"bookmark-group-title\">Category 1") {
		t.Fatalf("expected ungrouped bookmark not to be merged into first category, got %s", got)
	}
}

func TestRenderReturnsStyledErrorPageWhenBookmarksBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links:\n- name: App A\n  link: https://app.example.com\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories: [broken\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c, ""); err != nil {
		t.Fatalf("render: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestRenderReturnsStyledErrorPageWhenBookmarksCategoriesInvalid(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links:\n- name: App A\n  link: https://app.example.com\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories:\n- id: default\n  title: 默认\nlinks:\n- name: Bookmark A\n  category: missing\n  link: https://bookmark.example.com\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c, ""); err != nil {
		t.Fatalf("render: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestPageApplicationReturnsStyledErrorPageWhenAppsBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links: [broken\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories:\n- id: default\n  title: 默认\nlinks:\n- name: Bookmark A\n  category: default\n  link: https://bookmark.example.com\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/applications", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageApplication(c); err != nil {
		t.Fatalf("pageApplication: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestRenderHelpReturnsStyledErrorWhenConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := renderHelp(c); err != nil {
		t.Fatalf("renderHelp: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestRenderReturnsStyledErrorPageWhenConfigValuesInvalid(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: fr\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c, ""); err != nil {
		t.Fatalf("render: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestRenderReturnsStyledErrorPageWhenThemeInvalid(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: mystery\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c, ""); err != nil {
		t.Fatalf("render: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestRenderWarnsAndFallsBackWhenSiteIconInvalid(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: en\nTheme: blackboard\nSiteIcon: not-a-real-icon\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links: []\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories: []\nlinks: []\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	e := echo.New()
	e.Renderer = homeCaptureRenderer{t: t}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageHome(c); err != nil {
		t.Fatalf("pageHome: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRenderWarnsWhenConfiguredAppIconFallsBack(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: en\nTheme: blackboard\nIconMode: FILLING\nShowApps: true\nShowBookmarks: true\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links:\n- name: App A\n  link: https://app.example.com\n  icon: not-a-real-icon\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories: []\nlinks: []\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	e := echo.New()
	e.Renderer = homeConfiguredIconWarningRenderer{t: t}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageHome(c); err != nil {
		t.Fatalf("pageHome: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestPageHomeExposesWarningsToolbarStateWhenWarningsExist(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: en\nTheme: blackboard\nIconMode: FILLING\nShowApps: true\nShowBookmarks: false\nHideWarningsButton: false\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links:\n- name: App A\n  link: https://app.example.com\n  icon: not-a-real-icon\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories: []\nlinks: []\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	e := echo.New()
	e.Renderer = homeWarningsToolbarRenderer{t: t, expectWarnings: true, expectHidden: false}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageHome(c); err != nil {
		t.Fatalf("pageHome: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestPageHomeExposesWarningsToolbarStateWhenHidden(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: en\nTheme: blackboard\nIconMode: FILLING\nShowApps: true\nShowBookmarks: false\nHideWarningsButton: true\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links:\n- name: App A\n  link: https://app.example.com\n  icon: not-a-real-icon\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories: []\nlinks: []\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	e := echo.New()
	e.Renderer = homeWarningsToolbarRenderer{t: t, expectWarnings: true, expectHidden: true}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageHome(c); err != nil {
		t.Fatalf("pageHome: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestPageHomeSanitizesCustomFooterBeforeRender(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	rawFooter := `</textarea><script>alert(1)</script><a href="javascript:alert(1)">bad</a><em>ok</em><a href="/help">help</a>`
	configBody := "Title: SuperFlare\nLocale: en\nTheme: blackboard\nFooter: |\n  " + strings.ReplaceAll(rawFooter, "\n", "\n  ") + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(configBody), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links: []\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte("categories: []\nlinks: []\n"), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}

	e := echo.New()
	e.Renderer = homeFooterRenderer{t: t, rawFooter: rawFooter}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageHome(c); err != nil {
		t.Fatalf("pageHome: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestPageSearchReturnsStyledBadRequestWhenFormDataMissing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("{"))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageSearch(c); err != nil {
		t.Fatalf("pageSearch: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "missing form data") {
		t.Fatalf("expected missing form data detail, got %s", rec.Body.String())
	}
}

func TestPageSearchReturnsStyledBadRequestWhenQueryTooLong(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	form := url.Values{}
	form.Set("search", strings.Repeat("a", 51))
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageSearch(c); err != nil {
		t.Fatalf("pageSearch: %v", err)
	}
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "search query too long") {
		t.Fatalf("expected search query too long detail, got %s", rec.Body.String())
	}
}

func TestPageSearchRedirectsToPresetEngineWhenSearchModeIsEngine(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	configBody := "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nSearchMode: engine\nSearchEngine: bing\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(configBody), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	writeEmptyBookmarkFixtures(t, tmpDir)

	form := url.Values{}
	form.Set("search", "test keyword")
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageSearch(c); err != nil {
		t.Fatalf("pageSearch: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "bing.com/search") || !strings.Contains(location, "test+keyword") {
		t.Fatalf("unexpected redirect location: %s", location)
	}
}

func TestPageSearchRedirectsToPresetEngineWhenSearchModeIsEngine_DefaultsToBing(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	configBody := "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nSearchMode: engine\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(configBody), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	writeEmptyBookmarkFixtures(t, tmpDir)

	form := url.Values{}
	form.Set("search", "test keyword")
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageSearch(c); err != nil {
		t.Fatalf("pageSearch: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if !strings.Contains(location, "bing.com/search") || !strings.Contains(location, "test+keyword") {
		t.Fatalf("unexpected redirect location: %s", location)
	}
}

func TestPageSearchRedirectsToCustomEngineWhenConfigured(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	configBody := "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nSearchMode: engine\nSearchEngine: custom\nSearchEngineCustomTemplate: https://example.com/find?q=%s&src=sf\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(configBody), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	writeEmptyBookmarkFixtures(t, tmpDir)

	form := url.Values{}
	form.Set("search", "hello world")
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageSearch(c); err != nil {
		t.Fatalf("pageSearch: %v", err)
	}
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	location := rec.Header().Get("Location")
	if location != "https://example.com/find?q=hello+world&src=sf" {
		t.Fatalf("unexpected redirect location: %s", location)
	}
}

func TestRenderUsesEngineSearchHintsWhenModeIsEngine(t *testing.T) {
	t.Skip("legacy encoded-body assertion replaced by renderer-based coverage")
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	configBody := "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nSearchMode: engine\nSearchEngine: google\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(configBody), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c, ""); err != nil {
		t.Fatalf("render: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "使用 Google 搜索") {
		t.Fatalf("expected engine search placeholder in body, got %s", body)
	}
	if !strings.Contains(body, "输入关键词后将跳转到 Google 搜索") {
		t.Fatalf("expected engine search hint label in body, got %s", body)
	}
}

func TestPageSearchReturnsStyledErrorWhenConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	form := url.Values{}
	form.Set("search", "test")
	e := echo.New()
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(form.Encode()))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageSearch(c); err != nil {
		t.Fatalf("pageSearch: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestRenderUsesEngineSearchHintsWhenModeIsEngineRenderer(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	configBody := "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nSearchMode: engine\nSearchEngine: google\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(configBody), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	writeEmptyBookmarkFixtures(t, tmpDir)

	e := echo.New()
	e.Renderer = homeSearchHintRendererI18n{t: t}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c, ""); err != nil {
		t.Fatalf("render: %v", err)
	}
}

func TestRenderUsesEngineSearchHintsAndTargetWhenModeIsEngineNewTab(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	configBody := "Title: SuperFlare\nLocale: zh\nTheme: blackboard\nSearchMode: engine\nSearchEngine: bing\nSearchEngineOpenMode: new-tab\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(configBody), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	writeEmptyBookmarkFixtures(t, tmpDir)

	e := echo.New()
	e.Renderer = homeSearchHintTargetRenderer{t: t}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c, ""); err != nil {
		t.Fatalf("render: %v", err)
	}
}

func TestPageBookmarkReturnsStyledErrorWhenConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/bookmarks", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageBookmark(c); err != nil {
		t.Fatalf("pageBookmark: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestPageApplicationReturnsStyledErrorWhenConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/applications", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := pageApplication(c); err != nil {
		t.Fatalf("pageApplication: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func mustRequestForHost(t *testing.T, rawURL string) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	req.Host = req.URL.Host
	if req.URL.Scheme == "https" {
		req.TLS = &tls.ConnectionState{}
	}
	return req
}

func assertContainsError(name string, haystack string, needle string) error {
	return fmt.Errorf("%s missing expected fragment %s in %s", name, needle, haystack)
}

type homeCaptureRenderer struct {
	t *testing.T
}

func (r homeCaptureRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected template data type %T", data)
		}
	}
	if name != "home.html" {
		r.t.Fatalf("unexpected template name %q", name)
	}
	if got, _ := m["OptionSiteIcon"].(string); got != "" {
		r.t.Fatalf("expected invalid site icon to be sanitized, got %q", got)
	}
	warnings, ok := m["RenderWarnings"].([]string)
	if !ok || len(warnings) == 0 {
		r.t.Fatalf("expected render warnings, got %#v", m["RenderWarnings"])
	}
	for _, item := range warnings {
		if strings.Contains(item, "Site icon config error") && strings.Contains(item, "default website icon") {
			return nil
		}
	}
	r.t.Fatalf("expected site icon fallback warning, got %#v", warnings)
	return nil
}

type homeConfiguredIconWarningRenderer struct {
	t *testing.T
}

func (r homeConfiguredIconWarningRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected template data type %T", data)
		}
	}
	warnings, _ := m["RenderWarnings"].([]string)
	for _, item := range warnings {
		if strings.Contains(item, "App icon config fallback: 1 app entry uses invalid custom icon names and is temporarily using fallback icons.") {
			return nil
		}
	}
	r.t.Fatalf("expected configured app icon fallback warning, got %#v", warnings)
	return nil
}

type homeWarningsToolbarRenderer struct {
	t              *testing.T
	expectWarnings bool
	expectHidden   bool
}

type homeSearchHintRenderer struct {
	t *testing.T
}

type homeSearchHintRendererI18n struct {
	t *testing.T
}

type homeSearchHintTargetRenderer struct {
	t *testing.T
}

type homeFooterRenderer struct {
	t         *testing.T
	rawFooter string
}

func (r homeFooterRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected template data type %T", data)
		}
	}
	rawValue, ok := m["OptionFooter"].(string)
	if !ok {
		r.t.Fatalf("expected OptionFooter to stay a string, got %T", m["OptionFooter"])
	}
	if strings.TrimSpace(rawValue) != strings.TrimSpace(r.rawFooter) {
		r.t.Fatalf("expected raw footer %q, got %q", r.rawFooter, rawValue)
	}
	rendered, ok := m["RenderedFooter"].(template.HTML)
	if !ok {
		r.t.Fatalf("expected RenderedFooter to be trusted html, got %T", m["RenderedFooter"])
	}
	renderedText := string(rendered)
	for _, broken := range []string{`<script`, `javascript:`, `alert(1)`} {
		if strings.Contains(strings.ToLower(renderedText), broken) {
			r.t.Fatalf("expected sanitized rendered footer without %q, got %q", broken, renderedText)
		}
	}
	for _, expected := range []string{`bad`, `<em>ok</em>`, `href="/help"`} {
		if !strings.Contains(renderedText, expected) {
			r.t.Fatalf("expected rendered footer to contain %q, got %q", expected, renderedText)
		}
	}
	_, err := io.WriteString(w, "ok")
	return err
}

func (r homeWarningsToolbarRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected template data type %T", data)
		}
	}
	if name != "home.html" {
		r.t.Fatalf("unexpected template name %q", name)
	}
	warnings, _ := m["RenderWarnings"].([]string)
	hasWarnings, _ := m["HasRenderWarnings"].(bool)
	hideWarningsButton, _ := m["OptionHideWarningsButton"].(bool)
	if hasWarnings != r.expectWarnings {
		r.t.Fatalf("expected HasRenderWarnings=%v, got %v", r.expectWarnings, hasWarnings)
	}
	if hideWarningsButton != r.expectHidden {
		r.t.Fatalf("expected OptionHideWarningsButton=%v, got %v", r.expectHidden, hideWarningsButton)
	}
	if r.expectWarnings && len(warnings) == 0 {
		r.t.Fatalf("expected warnings, got %#v", m["RenderWarnings"])
	}
	if !r.expectWarnings && len(warnings) != 0 {
		r.t.Fatalf("expected no warnings, got %#v", warnings)
	}
	return nil
}

func (r homeSearchHintRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected template data type %T", data)
		}
	}
	placeholder, _ := m["SearchKeyword"].(template.HTML)
	label, _ := m["SearchHintLabel"].(string)
	if string(placeholder) != "使用 Google 搜索" {
		r.t.Fatalf("expected Google search placeholder, got %q", string(placeholder))
	}
	if label != "输入关键词后将跳转到 Google 搜索" {
		r.t.Fatalf("expected Google search label, got %q", label)
	}
	return nil
}

func (r homeSearchHintRendererI18n) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected template data type %T", data)
		}
	}
	placeholder, _ := m["SearchKeyword"].(template.HTML)
	label, _ := m["SearchHintLabel"].(string)
	if string(placeholder) != i18n.Tf("zh", "search_engine_placeholder", "Google") {
		r.t.Fatalf("expected Google search placeholder, got %q", string(placeholder))
	}
	if label != i18n.Tf("zh", "search_engine_label", "Google") {
		r.t.Fatalf("expected Google search label, got %q", label)
	}
	return nil
}

func (r homeSearchHintTargetRenderer) Render(c *echo.Context, w io.Writer, name string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		if typed, ok := data.(map[string]interface{}); ok {
			m = typed
		} else {
			r.t.Fatalf("unexpected template data type %T", data)
		}
	}
	placeholder, _ := m["SearchKeyword"].(template.HTML)
	label, _ := m["SearchHintLabel"].(string)
	target, _ := m["SearchFormTarget"].(string)
	rel, _ := m["SearchFormRel"].(string)
	if string(placeholder) != i18n.Tf("zh", "search_engine_placeholder", "Bing") {
		r.t.Fatalf("expected Bing search placeholder, got %q", string(placeholder))
	}
	if label != i18n.Tf("zh", "search_engine_label_new_tab", "Bing") {
		r.t.Fatalf("expected Bing new-tab search label, got %q", label)
	}
	if target != "_blank" {
		r.t.Fatalf("expected _blank target, got %q", target)
	}
	if rel != "noopener noreferrer" {
		r.t.Fatalf("expected rel noopener noreferrer, got %q", rel)
	}
	return nil
}

func TestAppendConfiguredIconWarningsReportsLoadError(t *testing.T) {
	origLoadFavorite := loadFavoriteBookmarks
	origLoadNormal := loadNormalBookmarks
	loadFavoriteBookmarks = func() (model.Bookmarks, error) {
		return model.Bookmarks{}, fmt.Errorf("apps config missing")
	}
	loadNormalBookmarks = func() (model.Bookmarks, error) {
		return model.Bookmarks{}, fmt.Errorf("bookmarks config missing")
	}
	defer func() {
		loadFavoriteBookmarks = origLoadFavorite
		loadNormalBookmarks = origLoadNormal
	}()

	warnings := appendConfiguredIconWarnings("zh", "FILLING", true, true, nil)
	if len(warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %#v", warnings)
	}
	if !strings.Contains(warnings[0], "读取失败") || !strings.Contains(warnings[1], "读取失败") {
		t.Fatalf("expected load failure warnings, got %#v", warnings)
	}
}

func TestRenderBookmarkIconFallsBackToBuiltinBookmarkWhenFaviconMissing(t *testing.T) {
	prepareIconTest(t)
	origGetter := getSiteFaviconFast
	getSiteFaviconFast = func(string, string) string { return "" }
	defer func() { getSiteFaviconFast = origGetter }()

	out := renderBookmarkIcon("", "https://example.com/path", "FILLING")
	if !strings.Contains(out, `bookmark.svg`) {
		t.Fatalf("expected builtin bookmark fallback, got %q", out)
	}
}

func TestRenderBookmarkItemUsesDedicatedLabelSpan(t *testing.T) {
	var b strings.Builder

	renderBookmarkItem(&b, model.Bookmark{
		Name: "Example",
		URL:  "https://example.com",
		Icon: "bookmark",
	}, false, false, "", false)

	html := b.String()
	if !strings.Contains(html, `class="bookmark-label"`) {
		t.Fatalf("expected bookmark label span class, got %s", html)
	}
}

func TestInlineStyleUsesStableBookmarkHoverUnderline(t *testing.T) {
	styleBytes, err := os.ReadFile(filepath.Join("..", "..", "..", "embed", "assets", "css", "home", "bookmarks.css"))
	if err != nil {
		t.Fatalf("read bookmarks.css: %v", err)
	}
	style := string(styleBytes)

	if !strings.Contains(style, `.bookmark-list a.bookmark {`) || !strings.Contains(style, `display: flex;`) {
		t.Fatalf("expected bookmark links to use stable flex layout, got %s", style)
	}
	if !strings.Contains(style, `.bookmark-list a.bookmark:hover .bookmark-label {`) || !strings.Contains(style, `text-decoration: underline;`) {
		t.Fatalf("expected bookmark hover underline to target label span with native underline positioning, got %s", style)
	}
	if strings.Contains(style, `.bookmark-list a.bookmark:hover {text-decoration: underline;`) {
		t.Fatalf("expected legacy anchor underline rule to be removed, got %s", style)
	}
}
