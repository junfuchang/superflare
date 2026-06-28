package statuspage

import (
	"errors"
	"html/template"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/model"
	"github.com/labstack/echo/v5"
)

func TestRequireConfiguredBodyStyleReturnsCachedStyle(t *testing.T) {
	originalGet := getAppBodyStyle
	originalUpdate := updatePagePalette
	t.Cleanup(func() {
		getAppBodyStyle = originalGet
		updatePagePalette = originalUpdate
	})

	updateCalled := false
	getAppBodyStyle = func() template.CSS {
		return template.CSS(`--color-background:#111;--color-primary:#eee;--color-accent:#f90;`)
	}
	updatePagePalette = func() error {
		updateCalled = true
		return nil
	}

	style, err := RequireConfiguredBodyStyle()
	if err != nil {
		t.Fatalf("RequireConfiguredBodyStyle: %v", err)
	}
	if string(style) == "" {
		t.Fatal("expected cached body style")
	}
	if updateCalled {
		t.Fatal("did not expect palette refresh when cached style exists")
	}
}

func TestRequireConfiguredBodyStyleRefreshesCache(t *testing.T) {
	originalGet := getAppBodyStyle
	originalUpdate := updatePagePalette
	t.Cleanup(func() {
		getAppBodyStyle = originalGet
		updatePagePalette = originalUpdate
	})

	callCount := 0
	getAppBodyStyle = func() template.CSS {
		callCount++
		if callCount == 1 {
			return ""
		}
		return template.CSS(`--color-background:#111;--color-primary:#eee;--color-accent:#f90;`)
	}
	updateCalled := false
	updatePagePalette = func() error {
		updateCalled = true
		return nil
	}

	style, err := RequireConfiguredBodyStyle()
	if err != nil {
		t.Fatalf("RequireConfiguredBodyStyle: %v", err)
	}
	if string(style) == "" {
		t.Fatal("expected refreshed body style")
	}
	if !updateCalled {
		t.Fatal("expected palette refresh when cached style is empty")
	}
}

func TestRequireConfiguredBodyStyleForRenderReturnsRecoveryWarning(t *testing.T) {
	originalGet := getAppBodyStyle
	originalUpdate := updatePagePalette
	t.Cleanup(func() {
		getAppBodyStyle = originalGet
		updatePagePalette = originalUpdate
	})

	callCount := 0
	getAppBodyStyle = func() template.CSS {
		callCount++
		if callCount == 1 {
			return ""
		}
		return template.CSS(`--color-background:#111;--color-primary:#eee;--color-accent:#f90;`)
	}
	updatePagePalette = func() error { return nil }

	style, warning, err := RequireConfiguredBodyStyleForRender("en", "home")
	if err != nil {
		t.Fatalf("RequireConfiguredBodyStyleForRender: %v", err)
	}
	if string(style) == "" {
		t.Fatal("expected refreshed body style")
	}
	if !strings.Contains(warning, "Theme cache was empty and has been rebuilt") {
		t.Fatalf("expected explicit recovery warning, got %q", warning)
	}
	if !strings.Contains(warning, "This page is now using the refreshed theme style") {
		t.Fatalf("expected home-scope recovery detail, got %q", warning)
	}
}

func TestRequireConfiguredBodyStyleReturnsErrorWhenRefreshFails(t *testing.T) {
	originalGet := getAppBodyStyle
	originalUpdate := updatePagePalette
	t.Cleanup(func() {
		getAppBodyStyle = originalGet
		updatePagePalette = originalUpdate
	})

	getAppBodyStyle = func() template.CSS { return "" }
	updatePagePalette = func() error { return errors.New("boom") }

	_, err := RequireConfiguredBodyStyle()
	if err == nil {
		t.Fatal("expected refresh failure")
	}
	if err.Error() != "refresh page theme cache failed: boom" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireConfiguredBodyStyleReturnsErrorWhenCacheStillEmptyAfterRefresh(t *testing.T) {
	originalGet := getAppBodyStyle
	originalUpdate := updatePagePalette
	t.Cleanup(func() {
		getAppBodyStyle = originalGet
		updatePagePalette = originalUpdate
	})

	getAppBodyStyle = func() template.CSS { return "" }
	updatePagePalette = func() error { return nil }

	_, err := RequireConfiguredBodyStyle()
	if err == nil {
		t.Fatal("expected empty cache error")
	}
	if err.Error() != "page theme cache is empty after refresh" {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCurrentLocaleUsesBoundOptionsWithoutReloadingConfig(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	BindOptions(c, model.Application{Locale: "en", Title: "Example"})

	if got := CurrentLocale(c); got != "en" {
		t.Fatalf("CurrentLocale() = %q, want en", got)
	}
}

func TestCurrentLocaleFallsBackToRequestLocaleWhenOptionsErrorBound(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	BindOptionsLoadError(c, errors.New("broken config"))

	if got := CurrentLocale(c); got != "en" {
		t.Fatalf("CurrentLocale() = %q, want en", got)
	}
}

func TestCurrentLocaleDoesNotReloadConfigWhenNothingBound(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: Config Title\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if got := CurrentLocale(c); got != "en" {
		t.Fatalf("CurrentLocale() = %q, want en", got)
	}
}

func TestHTMLUsesBoundOptionsWhenRenderingStatusPage(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	BindOptions(c, model.Application{Locale: "en", Title: "Bound Title"})

	if err := HTML(c, http.StatusInternalServerError, BuildHTTPErrorPage("en", http.StatusInternalServerError, "boom")); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `lang="en"`) {
		t.Fatalf("expected bound locale in html, got %s", body)
	}
	if !strings.Contains(body, "Bound Title") {
		t.Fatalf("expected bound title in html, got %s", body)
	}
}

func TestHTMLDoesNotReloadConfigWhenOptionsUnbound(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: Config Title\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := HTML(c, http.StatusInternalServerError, BuildHTTPErrorPage("en", http.StatusInternalServerError, "boom")); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := rec.Body.String()
	if strings.Contains(body, "Config Title") {
		t.Fatalf("did not expect implicit config reload in html, got %s", body)
	}
	if !strings.Contains(body, "SuperFlare") {
		t.Fatalf("expected fallback brand in html, got %s", body)
	}
}

func TestHTMLSurfacesThemeCacheFallback(t *testing.T) {
	originalGet := getAppBodyStyle
	originalUpdate := updatePagePalette
	t.Cleanup(func() {
		getAppBodyStyle = originalGet
		updatePagePalette = originalUpdate
	})

	getAppBodyStyle = func() template.CSS { return "" }
	updatePagePalette = func() error { return errors.New("theme cache unavailable") }

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	BindOptions(c, model.Application{Locale: "en", Title: "Bound Title"})

	if err := HTML(c, http.StatusInternalServerError, BuildHTTPErrorPage("en", http.StatusInternalServerError, "boom")); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Theme cache error: refresh page theme cache failed: theme cache unavailable.") {
		t.Fatalf("expected explicit theme cache warning, got %s", body)
	}
	if !strings.Contains(body, "using the built-in default style") {
		t.Fatalf("expected explicit default-style fallback note, got %s", body)
	}
}

func TestHTMLSurfacesOptionsLoadError(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Accept-Language", "en-US,en;q=0.8")
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	BindOptionsLoadError(c, errors.New("broken config"))

	if err := HTML(c, http.StatusInternalServerError, BuildHTTPErrorPage("en", http.StatusInternalServerError, "boom")); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Configuration read failed: broken config.") {
		t.Fatalf("expected explicit config read warning, got %s", body)
	}
	if !strings.Contains(body, "request-derived defaults") {
		t.Fatalf("expected request-derived default note, got %s", body)
	}
}

func TestHTMLSurfacesUploadedBackgroundFallback(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir tmp: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	BindOptions(c, model.Application{
		Locale:              "en",
		Title:               "Bound Title",
		BackgroundImage:     "/user-assets/background",
		BackgroundImageMode: "upload",
	})

	if err := HTML(c, http.StatusInternalServerError, BuildHTTPErrorPage("en", http.StatusInternalServerError, "boom")); err != nil {
		t.Fatalf("HTML: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Background asset error: resolve uploaded background asset failed: file does not exist.") {
		t.Fatalf("expected explicit background warning, got %s", body)
	}
	if !strings.Contains(body, "rendering without the configured background") {
		t.Fatalf("expected explicit background fallback note, got %s", body)
	}
}

func TestBuildHTTPErrorPageSurfacesSpecificInternalErrorDetail(t *testing.T) {
	page := BuildHTTPErrorPage("en", http.StatusInternalServerError, "runtime login config is empty while login mode is enabled")
	if !strings.Contains(page.Detail, "runtime login config is empty while login mode is enabled") {
		t.Fatalf("expected detailed 500 error message, got %#v", page)
	}
}

func TestValidatePageRenderOptionsReturnsErrorWhenSiteIconInvalid(t *testing.T) {
	err := ValidatePageRenderOptions(model.Application{Locale: "en", SiteIcon: "not-a-real-icon"})
	if err == nil {
		t.Fatal("expected invalid site icon error")
	}
	if !strings.Contains(err.Error(), "invalid site icon value") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPrepareSettingsOptionsForRenderWarnsAndSanitizesInvalidSiteIcon(t *testing.T) {
	options, warnings := PrepareSettingsOptionsForRender(model.Application{
		Locale:   "en",
		SiteIcon: "not-a-real-icon",
	})
	if options.SiteIcon != "" {
		t.Fatalf("expected invalid site icon to be cleared, got %q", options.SiteIcon)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %#v", warnings)
	}
	if !strings.Contains(warnings[0], "Site icon config error") {
		t.Fatalf("unexpected warning: %v", warnings[0])
	}
}

func TestPrepareHomeOptionsForRenderWarnsAndSanitizesInvalidSiteIcon(t *testing.T) {
	options, warnings := PrepareHomeOptionsForRender(model.Application{
		Locale:   "en",
		SiteIcon: "not-a-real-icon",
	})
	if options.SiteIcon != "" {
		t.Fatalf("expected invalid site icon to be cleared, got %q", options.SiteIcon)
	}
	if len(warnings) != 1 {
		t.Fatalf("expected 1 warning, got %#v", warnings)
	}
	if !strings.Contains(warnings[0], "Site icon config error") {
		t.Fatalf("unexpected warning: %v", warnings[0])
	}
	if !strings.Contains(warnings[0], "default website icon") {
		t.Fatalf("expected explicit homepage fallback detail, got %v", warnings[0])
	}
}
