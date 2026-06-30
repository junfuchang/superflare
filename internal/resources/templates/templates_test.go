package templates

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/labstack/echo/v5"
)

func withTemplateDebugTestWorkingDir(t *testing.T) {
	t.Helper()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	repoRoot := filepath.Clean(filepath.Join(origWd, "..", "..", ".."))
	tmpDir := t.TempDir()
	if err := copyTemplateDir(filepath.Join(repoRoot, "embed", "templates"), filepath.Join(tmpDir, "embed", "templates")); err != nil {
		t.Fatalf("copy source templates: %v", err)
	}
	if err := copyTemplateDir(filepath.Join(repoRoot, "internal", "resources", "templates", "html"), filepath.Join(tmpDir, "internal", "resources", "templates", "html")); err != nil {
		t.Fatalf("copy generated templates: %v", err)
	}
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(origWd)
	})
}

func copyTemplateDir(src string, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode())
	})
}

func saveTemplateRuntimeFlags() (templateRuntimeSnapshot, bool) {
	templateRuntimeFlags.mu.RLock()
	defer templateRuntimeFlags.mu.RUnlock()
	return templateRuntimeFlags.cfg, templateRuntimeFlags.set
}

func restoreTemplateRuntimeFlags(cfg templateRuntimeSnapshot, set bool) {
	templateRuntimeFlags.mu.Lock()
	templateRuntimeFlags.cfg = cfg
	templateRuntimeFlags.set = set
	templateRuntimeFlags.mu.Unlock()
}

func TestRegisterRoutingUsesStoredRuntimeDebugModeAfterAppFlagsChange(t *testing.T) {
	withTemplateDebugTestWorkingDir(t)

	origFlags := define.AppFlags
	origRuntime, origRuntimeSet := saveTemplateRuntimeFlags()
	defer func() {
		define.AppFlags = origFlags
		restoreTemplateRuntimeFlags(origRuntime, origRuntimeSet)
	}()

	define.AppFlags = model.Flags{DebugMode: true}
	templateRuntimeFlags.Store(templateRuntimeSnapshotFromFlags(define.AppFlags))
	define.AppFlags = model.Flags{DebugMode: false}

	e := echo.New()
	if err := RegisterRouting(e); err != nil {
		t.Fatalf("RegisterRouting: %v", err)
	}

	rec := httptest.NewRecorder()
	c := e.NewContext(httptest.NewRequest("GET", "/", nil), rec)
	if err := e.Renderer.Render(c, rec, "home.html", map[string]any{
		"Locale":                   "zh",
		"DebugMode":                true,
		"OptionTitle":              "SuperFlare",
		"OptionSiteIcon":           "",
		"PageInlineStyle":          "",
		"CustomHomeStyle":          "",
		"PageAppearance":           "",
		"BodyClassName":            "",
		"ShowSearchComponent":      false,
		"OptionShowDateTime":       false,
		"OptionShowTitle":          false,
		"OptionShowApps":           false,
		"OptionShowBookmarks":      false,
		"OptionHideSettingsButton": true,
		"OptionHideHelpButton":     true,
		"OptionHideWarningsButton": true,
		"RenderedFooter":           "",
		"HasRenderWarnings":        false,
		"HasBackgroundAssets":      false,
		"SearchKeyword":            "",
		"SearchHintLabel":          "",
		"BookmarksURI":             "/bookmarks",
		"ApplicationsURI":          "/applications",
		"SettingsURI":              "/settings",
		"AppsTitle":                "",
		"BookmarksTitle":           "",
		"Applications":             "",
		"Bookmarks":                "",
	}); err != nil {
		t.Fatalf("render home template: %v", err)
	}
	body := rec.Body.String()
	if !strings.Contains(body, `<link rel="stylesheet" href="/assets/css/base.css">`) {
		t.Fatalf("expected debug template variant to keep linked css assets, got %s", body)
	}
	if strings.Contains(body, "<style></style>") {
		t.Fatalf("expected debug template variant to avoid inline css fallback, got %s", body)
	}
}
