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

func TestGuideScriptDocumentsCurrentSuperFlareFlow(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("guide-assets", "app.js"))
	if err != nil {
		t.Fatalf("read guide app.js: %v", err)
	}
	script := string(body)
	expected := []string{
		"SuperFlare 使用向导",
		"搜索模式",
		"应用与书签",
		"子目录",
		"图标",
		"在线编辑",
		"备份与恢复",
		"主题与背景",
		"端口",
		"Docker",
		"fnOS",
	}
	for _, token := range expected {
		if !strings.Contains(script, token) {
			t.Fatalf("guide script should mention %q in %s", token, script)
		}
	}
	blocked := []string{"plugin-weather", "天气", "完全离线", "鍐", "鎼", "涔"}
	for _, token := range blocked {
		if strings.Contains(script, token) {
			t.Fatalf("guide script should not contain stale or mojibake token %q in %s", token, script)
		}
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
	origRuntime, origRuntimeSet := saveGuideRuntimeFlags()
	defer func() {
		define.AppFlags = origFlags
		restoreGuideRuntimeFlags(origRuntime, origRuntimeSet)
	}()
	define.AppFlags = model.Flags{Port: 1}
	guideRuntimeFlags.Store(guideRuntimeSnapshotFromFlags(define.AppFlags))

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

func TestGetUserHomePageUsesStoredRuntimePortAfterAppFlagsChange(t *testing.T) {
	origFetch := getGuideHTML
	origFlags := define.AppFlags
	origRuntime, origRuntimeSet := saveGuideRuntimeFlags()
	t.Cleanup(func() {
		getGuideHTML = origFetch
		define.AppFlags = origFlags
		restoreGuideRuntimeFlags(origRuntime, origRuntimeSet)
	})

	define.AppFlags = model.Flags{Port: 3636}
	guideRuntimeFlags.Store(guideRuntimeSnapshotFromFlags(define.AppFlags))
	define.AppFlags = model.Flags{Port: 3737}

	var gotURL string
	getGuideHTML = func(url string) (string, error) {
		gotURL = url
		return `<!doctype html><html><head><title>x</title></head><body><div class="pageview" id="page-home"></div></body></html>`, nil
	}

	if _, err := getUserHomePage(); err != nil {
		t.Fatalf("getUserHomePage: %v", err)
	}
	if gotURL != "http://localhost:3636/" {
		t.Fatalf("expected stored runtime port to be used, got %q", gotURL)
	}
}

func saveGuideRuntimeFlags() (guideRuntimeSnapshot, bool) {
	guideRuntimeFlags.mu.RLock()
	defer guideRuntimeFlags.mu.RUnlock()
	return guideRuntimeFlags.cfg, guideRuntimeFlags.set
}

func restoreGuideRuntimeFlags(cfg guideRuntimeSnapshot, set bool) {
	guideRuntimeFlags.mu.Lock()
	guideRuntimeFlags.cfg = cfg
	guideRuntimeFlags.set = set
	guideRuntimeFlags.mu.Unlock()
}
