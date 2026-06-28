package guide

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/labstack/echo/v5"
)

func TestInjectGuideAssetsRequiresExpectedHTMLStructure(t *testing.T) {
	_, err := injectGuideAssets("<html><body>missing head and closing body")
	if err == nil {
		t.Fatal("expected malformed html to fail")
	}
	if !strings.Contains(err.Error(), "structure") {
		t.Fatalf("expected structure error, got %v", err)
	}
}

func TestInjectGuideAssetsAddsGuideResources(t *testing.T) {
	body, err := injectGuideAssets(`<!doctype html><html><head><title>x</title></head><body><main class="pageview"></main></body></html>`)
	if err != nil {
		t.Fatalf("injectGuideAssets: %v", err)
	}
	if !strings.Contains(body, `/assets/guide/introjs.min.css`) {
		t.Fatalf("expected intro css injection, got %s", body)
	}
	if !strings.Contains(body, `/assets/guide/app.js`) {
		t.Fatalf("expected app js injection, got %s", body)
	}
	if !strings.Contains(body, `class="pageview" style="position:inherit;"`) {
		t.Fatalf("expected pageview style injection, got %s", body)
	}
	if strings.Contains(body, `</head></body></html>`) {
		t.Fatalf("expected body-closing script injection to preserve closing body tag, got %s", body)
	}
	if !strings.Contains(body, `<script src="/assets/guide/app.js"></script></body>`) {
		t.Fatalf("expected app.js before closing body, got %s", body)
	}
}

func TestGetUserHomePageRejectsNonHomePageContent(t *testing.T) {
	origFetch := getGuideHTML
	getGuideHTML = func(url string) (string, error) {
		return `<!doctype html><html><head><title>x</title></head><body><div class="pageview" id="page-settings"></div></body></html>`, nil
	}
	t.Cleanup(func() {
		getGuideHTML = origFetch
	})

	origFlags := define.AppFlags
	t.Cleanup(func() { define.AppFlags = origFlags })
	define.AppFlags.Port = 3636

	_, err := getUserHomePage()
	if err == nil {
		t.Fatal("expected non-home page content to fail")
	}
	if !strings.Contains(err.Error(), "not the home page") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRenderReturnsStyledErrorPageWhenGuideSourceUnavailable(t *testing.T) {
	origFlags := define.AppFlags
	defer func() { define.AppFlags = origFlags }()
	define.AppFlags = model.Flags{Port: 1}

	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: SuperFlare\nLocale: zh\nTheme: blackboard\n"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c); err != nil {
		t.Fatalf("render: %v", err)
	}
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("expected 502, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}

func TestRenderReturnsStyledErrorPageWhenConfigBroken(t *testing.T) {
	tmpDir := t.TempDir()
	oldWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	defer func() { _ = os.Chdir(oldWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/help", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	if err := render(c); err != nil {
		t.Fatalf("render: %v", err)
	}
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "status-panel") {
		t.Fatalf("expected styled status page, got %s", rec.Body.String())
	}
}
