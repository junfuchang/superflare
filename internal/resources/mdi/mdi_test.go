package mdi

import (
	"io/fs"
	"path"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
)

func TestGetIconByNameNormalizesMDIInput(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	define.ThemeCurrent = "blackboard"
	define.ThemePrimaryColor = "rgba(255, 253, 234, 1)"

	for _, name := range []string{"home-circle", "homeCircle", "home_circle"} {
		got := GetIconByName(name)
		if !strings.Contains(got, "/assets/mdi/blackboard-homecircle.svg") {
			t.Fatalf("GetIconByName(%q) did not resolve homecircle icon: %s", name, got)
		}
	}

	if got := GetIconURLByName("home-circle"); got != "/assets/mdi/blackboard-homecircle.svg" {
		t.Fatalf("GetIconURLByName did not resolve normalized favicon icon: %s", got)
	}
}

func TestGetIconByNameCustomThemeIncludesPrimaryColorInCacheKey(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	define.ThemeCurrent = "custom"
	define.ThemePrimaryColor = "rgba(255, 0, 0, 1)"

	first := GetIconByName("bookmark")
	if !strings.Contains(first, "/assets/mdi/custom-") || !strings.Contains(first, "bookmark.svg") {
		t.Fatalf("custom theme icon path should include custom namespace, got %s", first)
	}

	define.ThemePrimaryColor = "rgba(0, 128, 255, 1)"
	second := GetIconByName("bookmark")
	if !strings.Contains(second, "/assets/mdi/custom-") || !strings.Contains(second, "bookmark.svg") {
		t.Fatalf("custom theme icon path should include custom namespace, got %s", second)
	}
	if first == second {
		t.Fatalf("custom theme icon path should change when primary color changes, got first=%q second=%q", first, second)
	}
}

func TestGetIconByNameFallsBackToInlineSVGWhenMemFsUnavailable(t *testing.T) {
	originalMemFs := MemFs
	MemFs = nil
	defer func() { MemFs = originalMemFs }()

	define.AppFlags.EnableMinimumRequest = false
	got := GetIconByName("bookmark")
	if !strings.Contains(got, "<svg") || !strings.Contains(got, "var(--color-primary)") {
		t.Fatalf("expected inline fallback svg, got %s", got)
	}
}

func TestGetIconURLByNameUsesFallbackThemeColorWhenPrimaryMissing(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	define.ThemeCurrent = "blackboard"
	define.ThemePrimaryColor = ""

	url := GetIconURLByName("bookmark")
	if !strings.Contains(url, "/assets/mdi/blackboard-bookmark.svg") {
		t.Fatalf("expected bookmark icon asset url, got %s", url)
	}

	raw, err := fs.ReadFile(MemFs, path.Join(_ASSETS_BASE_DIR, "blackboard-bookmark.svg"))
	if err != nil {
		t.Fatalf("read generated svg: %v", err)
	}
	if !strings.Contains(string(raw), fallbackThemePrimaryColor) {
		t.Fatalf("expected fallback theme color in svg, got %s", string(raw))
	}
}

func TestGetIconSVGDataByNameWorksWithoutMemFs(t *testing.T) {
	originalMemFs := MemFs
	MemFs = nil
	defer func() { MemFs = originalMemFs }()

	define.ThemePrimaryColor = ""
	raw, err := GetIconSVGDataByName("bookmark")
	if err != nil {
		t.Fatalf("GetIconSVGDataByName: %v", err)
	}
	if !strings.Contains(string(raw), "<svg") {
		t.Fatalf("expected svg payload, got %s", string(raw))
	}
	if !strings.Contains(string(raw), fallbackThemePrimaryColor) {
		t.Fatalf("expected fallback theme color in svg payload, got %s", string(raw))
	}
}
