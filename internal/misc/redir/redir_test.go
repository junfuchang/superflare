package redir

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/labstack/echo/v5"
)

func TestBookmarksContainLocalPairWithDynamicURL(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://192.168.1.10:3636/redir/local", nil)
	req.Host = "192.168.1.10:3636"
	requestURL := fn.ParseRequestURLTo(req)
	bookmarks := []model.Bookmark{
		{URL: "{origin}/app", LocalURL: "http://192.168.1.20/app"},
	}
	if !bookmarksContainLocalPair(bookmarks, &requestURL, "http://192.168.1.10:3636/app", "http://192.168.1.20/app") {
		t.Fatal("expected bookmark local URL pair to match")
	}
}

func TestRenderLocalRedirectPageContainsFallbackAndLocalURL(t *testing.T) {
	raw, err := renderLocalRedirectPage("https://public.example.com/app", "http://192.168.1.20/app", model.Application{Locale: "en"}, `--color-background:#111;--color-primary:#eee;--color-accent:#f90;`, nil)
	if err != nil {
		t.Fatalf("renderLocalRedirectPage: %v", err)
	}
	page := string(raw)
	for _, token := range []string{
		"https://public.example.com/app",
		"http://192.168.1.20/app",
		"Use source address",
		"fetch(localURL",
		"var seconds=3",
		"startFallbackCountdown",
	} {
		if !strings.Contains(page, token) {
			t.Fatalf("redirect page missing %q in %s", token, page)
		}
	}
}

func TestRenderLocalRedirectPageDefaultsToChinese(t *testing.T) {
	raw, err := renderLocalRedirectPage("https://public.example.com/app", "http://192.168.1.20/app", model.Application{}, `--color-background:#111;--color-primary:#eee;--color-accent:#f90;`, nil)
	if err != nil {
		t.Fatalf("renderLocalRedirectPage: %v", err)
	}
	page := string(raw)
	for _, token := range []string{
		`lang="zh-CN"`,
		"正在连接内网地址",
		"打开源书签地址",
		"秒后打开源书签地址",
	} {
		if !strings.Contains(page, token) {
			t.Fatalf("redirect page missing %q in %s", token, page)
		}
	}
}

func TestRenderLocalRedirectPageUsesEnglishLocale(t *testing.T) {
	raw, err := renderLocalRedirectPage("https://public.example.com/app", "http://192.168.1.20/app", model.Application{Locale: "en"}, `--color-background:#111;--color-primary:#eee;--color-accent:#f90;`, nil)
	if err != nil {
		t.Fatalf("renderLocalRedirectPage: %v", err)
	}
	page := string(raw)
	for _, token := range []string{
		`lang="en"`,
		"Connecting to the local address",
		"Opening the source bookmark in ",
	} {
		if !strings.Contains(page, token) {
			t.Fatalf("redirect page missing %q in %s", token, page)
		}
	}
}

func TestRenderLocalRedirectPageShowsRenderWarnings(t *testing.T) {
	raw, err := renderLocalRedirectPage(
		"https://public.example.com/app",
		"http://192.168.1.20/app",
		model.Application{Locale: "en"},
		`--color-background:#111;--color-primary:#eee;--color-accent:#f90;`,
		[]string{"Theme cache was empty and has been rebuilt. The refreshed theme style is now active."},
	)
	if err != nil {
		t.Fatalf("renderLocalRedirectPage: %v", err)
	}
	page := string(raw)
	for _, token := range []string{
		"local-redirect-warnings",
		"Theme cache was empty and has been rebuilt. The refreshed theme style is now active.",
	} {
		if !strings.Contains(page, token) {
			t.Fatalf("redirect page missing %q in %s", token, page)
		}
	}
}

func TestMarshalRedirectJSValueReturnsWrappedError(t *testing.T) {
	invalid := string([]byte{0xff})
	_, err := marshalRedirectJSValue("source url", invalid)
	if err == nil {
		t.Fatal("expected marshalRedirectJSValue to fail for invalid utf-8")
	}
	if !strings.Contains(err.Error(), "marshal redirect source url failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRedirHelperInvalidTargetUsesStyledStatusPage(t *testing.T) {
	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "/redir/url", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	body := rec.Body.String()
	for _, token := range []string{"跳转地址无效", "status-panel", "返回首页"} {
		if !strings.Contains(body, token) {
			t.Fatalf("redirect error page missing %q in %s", token, body)
		}
	}
}

func TestRedirHelperReturnsStyledServerErrorWhenBookmarksConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links: [broken\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}

	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "/redir/url?go=aHR0cHMlM0ElMkYlMkZleGFtcGxlLmNvbQ%3D%3D", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "status-panel") {
		t.Fatalf("expected styled status page, got %s", body)
	}
}

func TestRedirHelperAcceptsDynamicBookmarkURL(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links:\n- name: App A\n  link: \"{origin}/app\"\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}

	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "http://localhost:3636/redir/url?go=aHR0cDovL2xvY2FsaG9zdDozNjM2L2FwcA%3D%3D", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "http://localhost:3636/app" {
		t.Fatalf("unexpected redirect location: %s", location)
	}
}

func TestRedirHelperReturnsStyledServerErrorWhenSettingsConfigBrokenAndTargetInvalid(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "/redir/url", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "status-panel") {
		t.Fatalf("expected styled status page, got %s", body)
	}
}

func TestRedirLocalReturnsStyledServerErrorWhenBookmarksConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links: [broken\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}

	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "http://localhost:3636/redir/local?go=aHR0cHM6Ly9wdWJsaWMuZXhhbXBsZS5jb20vYXBw&local=aHR0cDovLzE5Mi4xNjguMS4yMC9hcHA%3D", nil)
	req.Header.Set("Accept", "text/html")
	req.RemoteAddr = "127.0.0.1:54321"
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "status-panel") {
		t.Fatalf("expected styled status page, got %s", body)
	}
}

func TestRedirLocalReturnsStyledServerErrorWhenSettingsConfigBroken(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	originalRequestLooksLocalNetwork := requestLooksLocalNetwork
	requestLooksLocalNetwork = func(r *http.Request) bool { return true }
	defer func() { requestLooksLocalNetwork = originalRequestLooksLocalNetwork }()
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links:\n- name: App A\n  link: https://public.example.com/app\n  local_link: http://192.168.1.20/app\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}

	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "/redir/local?go=aHR0cHM6Ly9wdWJsaWMuZXhhbXBsZS5jb20vYXBw&local=aHR0cDovLzE5Mi4xNjguMS4yMC9hcHA%3D", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "status-panel") {
		t.Fatalf("expected styled status page, got %s", body)
	}
}

func TestRedirLocalRedirectsToSourceWhenLocalURLCannotShareNetwork(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	originalRequestLooksLocalNetwork := requestLooksLocalNetwork
	requestLooksLocalNetwork = func(r *http.Request) bool { return true }
	defer func() { requestLooksLocalNetwork = originalRequestLooksLocalNetwork }()
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte("links:\n- name: App A\n  link: https://public.example.com/app\n  local_link: http://192.168.10.1/app\n"), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}

	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "http://192.168.0.10:3636/redir/local?go=aHR0cHM6Ly9wdWJsaWMuZXhhbXBsZS5jb20vYXBw&local=aHR0cDovLzE5Mi4xNjguMTAuMS9hcHA%3D", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d body=%s", rec.Code, rec.Body.String())
	}
	if location := rec.Header().Get("Location"); location != "https://public.example.com/app" {
		t.Fatalf("unexpected redirect location: %s", location)
	}
}

func TestRedirLocalReturnsStyledServerErrorWhenSettingsConfigBrokenAndTargetInvalid(t *testing.T) {
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	defer func() { _ = os.Chdir(origWd) }()

	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte("Title: [broken"), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "/redir/local", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "status-panel") {
		t.Fatalf("expected styled status page, got %s", body)
	}
}

func TestRedirLocalRejectsNonHTTPSourceURL(t *testing.T) {
	e := echo.New()
	RegisterRouting(e)

	req := httptest.NewRequest(http.MethodGet, "/redir/local?go=Y2hyb21lLWV4dGVuc2lvbjovL2FiYy9pbmRleC5odG1s&local=aHR0cDovLzE5Mi4xNjguMS4yMC9hcHA%3D", nil)
	req.Header.Set("Accept", "text/html")
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "status-panel") {
		t.Fatalf("expected styled status page, got %s", body)
	}
}
