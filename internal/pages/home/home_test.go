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

	"github.com/gorilla/sessions"
	"github.com/junfuchang/superflare/config/define"
	"github.com/junfuchang/superflare/config/model"
	"github.com/junfuchang/superflare/internal/auth"
	"github.com/junfuchang/superflare/internal/background"
	"github.com/junfuchang/superflare/internal/fn"
	"github.com/junfuchang/superflare/internal/i18n"
	echosession "github.com/labstack/echo-contrib/v5/session"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
)

func saveAppFlags() model.Flags { return define.AppFlags }

func restoreAppFlags(f model.Flags) {
	define.AppFlags = f
}

func saveHomeRuntimeFlags() (homeRuntimeSnapshot, bool) {
	homeRuntimeFlags.mu.RLock()
	defer homeRuntimeFlags.mu.RUnlock()
	return homeRuntimeFlags.cfg, homeRuntimeFlags.set
}

func restoreHomeRuntimeFlags(cfg homeRuntimeSnapshot, set bool) {
	homeRuntimeFlags.mu.Lock()
	homeRuntimeFlags.cfg = cfg
	homeRuntimeFlags.set = set
	homeRuntimeFlags.mu.Unlock()
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

func usePrivateProjectionFixtures(t *testing.T) {
	t.Helper()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	config := strings.Join([]string{
		"Title: SuperFlare",
		"Locale: en",
		"Theme: blackboard",
		"ShowApps: true",
		"ShowFavorites: true",
		"ShowBookmarks: true",
		"ShowDateTime: false",
		"IconMode: NONE",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(config), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}

	apps := strings.Join([]string{
		"links:",
		`- name: "App Favorite Impostor"`,
		"  link: https://impostor.example",
		"  favorite: true",
		`- name: "Public App"`,
		"  link: https://public-app.example",
		`- name: "Private App"`,
		"  link: https://private-app.example",
		"  private: true",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte(apps), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}

	bookmarks := strings.Join([]string{
		"categories:",
		"- id: main",
		`  title: "Private Projection Group"`,
		"links:",
		`- name: "Zulu"`,
		"  link: https://z.example",
		"  category: main",
		"  favorite: true",
		`- name: "alpha"`,
		"  link: https://a.example",
		"  category: main",
		`  desc: "private details"`,
		"  favorite: true",
		"  private: true",
		`- name: "Beta"`,
		"  link: https://b.example",
		"  category: main",
		"  favorite: true",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte(bookmarks), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}
}

func usePrivateIconWarningFixtures(t *testing.T) {
	t.Helper()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })

	config := strings.Join([]string{
		"Title: SuperFlare",
		"Locale: en",
		"Theme: blackboard",
		"ShowApps: true",
		"ShowFavorites: true",
		"ShowBookmarks: true",
		"IconMode: DEFAULT",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(config), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	apps := strings.Join([]string{
		"links:",
		`- name: "Public App"`,
		"  link: https://public-app.example",
		`- name: "Private Invalid App"`,
		"  link: https://private-app.example",
		"  icon: definitely-not-an-mdi-icon",
		"  private: true",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "apps.yml"), []byte(apps), 0644); err != nil {
		t.Fatalf("write apps.yml: %v", err)
	}
	bookmarks := strings.Join([]string{
		"categories: []",
		"links:",
		`- name: "Public Bookmark"`,
		"  link: https://public-bookmark.example",
		`- name: "Private Invalid Bookmark"`,
		"  link: https://private-bookmark.example",
		"  icon: definitely-not-an-mdi-icon",
		"  private: true",
		"  favorite: true",
	}, "\n") + "\n"
	if err := os.WriteFile(filepath.Join(tmpDir, "bookmarks.yml"), []byte(bookmarks), 0644); err != nil {
		t.Fatalf("write bookmarks.yml: %v", err)
	}
}

func assertTextOrder(t *testing.T, text string, values ...string) {
	t.Helper()
	last := -1
	for _, value := range values {
		index := strings.Index(text, value)
		if index < 0 {
			t.Fatalf("expected %q in %s", value, text)
		}
		if index <= last {
			t.Fatalf("expected %q after prior values in %s", value, text)
		}
		last = index
	}
}

func TestFavoritesProjectionFiltersPrivateSortsAndKeepsRegularBookmarks(t *testing.T) {
	usePrivateProjectionFixtures(t)
	options := model.Application{ShowBookmarks: true, ShowFavorites: true, IconMode: define.IconModeHidden}

	anonymous, err := generateBookmarkModulesWithLocalAndURLErr("", &options, false, nil, false)
	if err != nil {
		t.Fatalf("generate anonymous bookmark modules: %v", err)
	}
	if strings.Contains(string(anonymous.Bookmarks), "alpha") || strings.Contains(string(anonymous.Favorites), "alpha") {
		t.Fatalf("anonymous projections must exclude private bookmark: bookmarks=%s favorites=%s", anonymous.Bookmarks, anonymous.Favorites)
	}
	if anonymous.HasDescriptions {
		t.Fatal("private descriptions must not affect anonymous projection state")
	}

	trusted, err := generateBookmarkModulesWithLocalAndURLErr("", &options, false, nil, true)
	if err != nil {
		t.Fatalf("generate trusted bookmark modules: %v", err)
	}
	bookmarksHTML := string(trusted.Bookmarks)
	favoritesHTML := string(trusted.Favorites)
	assertTextOrder(t, bookmarksHTML, "Zulu", "alpha", "Beta")
	assertTextOrder(t, favoritesHTML, "alpha", "Beta", "Zulu")
	if strings.Contains(favoritesHTML, "bookmark-group-title") || strings.Contains(favoritesHTML, "Private Projection Group") {
		t.Fatalf("favorites projection must not render category headings: %s", favoritesHTML)
	}
	if !trusted.HasDescriptions {
		t.Fatal("trusted projection should report its visible bookmark description")
	}
	for _, name := range []string{"Zulu", "alpha", "Beta"} {
		if !strings.Contains(bookmarksHTML, name) {
			t.Fatalf("favorited normal bookmark %q must remain in regular bookmarks: %s", name, bookmarksHTML)
		}
	}
}

func TestFavoritesProjectionIgnoresApplicationFavoriteFlag(t *testing.T) {
	usePrivateProjectionFixtures(t)
	modules, err := generateBookmarkModulesWithLocalAndURLErr("", &model.Application{IconMode: define.IconModeHidden}, false, nil, true)
	if err != nil {
		t.Fatalf("generate bookmark modules: %v", err)
	}
	if strings.Contains(string(modules.Favorites), "App Favorite Impostor") {
		t.Fatalf("apps.yml favorite flag must not enter normal-bookmark favorites: %s", modules.Favorites)
	}
}

func TestFavoritesProjectionReturnsEmptyWithoutVisibleMatches(t *testing.T) {
	tests := []struct {
		name           string
		items          []model.Bookmark
		filter         string
		canViewPrivate bool
	}{
		{
			name:           "source has no favorites",
			items:          []model.Bookmark{{Name: "Regular", URL: "https://regular.example"}},
			canViewPrivate: true,
		},
		{
			name: "anonymous filters every favorite",
			items: []model.Bookmark{
				{Name: "Public Regular", URL: "https://public.example"},
				{Name: "Private Favorite", URL: "https://private.example", Private: true, Favorite: true},
			},
		},
		{
			name: "search has no favorite match",
			items: []model.Bookmark{
				{Name: "Visible Match", URL: "https://visible.example"},
				{Name: "Favorite Other", URL: "https://favorite.example", Favorite: true},
			},
			filter:         "Visible Match",
			canViewPrivate: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalLoader := loadNormalBookmarks
			loadNormalBookmarks = func() (model.Bookmarks, error) {
				return model.Bookmarks{Items: tt.items}, nil
			}
			t.Cleanup(func() { loadNormalBookmarks = originalLoader })

			modules, err := generateBookmarkModulesWithLocalAndURLErr(tt.filter, &model.Application{
				ShowFavorites: true,
				IconMode:      define.IconModeHidden,
			}, false, nil, tt.canViewPrivate)
			if err != nil {
				t.Fatalf("generate bookmark modules: %v", err)
			}
			if modules.Favorites != "" {
				t.Fatalf("empty favorites projection must return empty HTML, got %q", modules.Favorites)
			}
		})
	}
}

type emptyFavoritesRenderer struct {
	t *testing.T
}

func (r emptyFavoritesRenderer) Render(_ *echo.Context, _ io.Writer, _ string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		r.t.Fatalf("unexpected template data type %T", data)
	}
	if favorites, _ := m["Favorites"].(template.HTML); favorites != "" {
		r.t.Fatalf("expected empty favorites HTML, got %q", favorites)
	}
	if showFavorites, _ := m["OptionShowFavorites"].(bool); showFavorites {
		r.t.Fatal("empty favorites projection must disable the favorites module")
	}
	return nil
}

func TestFavoritesHandlerDisablesEmptyModule(t *testing.T) {
	usePrivateProjectionFixtures(t)
	originalRuntime := auth.SnapshotAuthRuntimeConfig()
	t.Cleanup(func() { auth.StoreAuthRuntimeConfig(originalRuntime) })

	tests := []struct {
		name         string
		items        []model.Bookmark
		disableLogin bool
		search       string
	}{
		{
			name:         "source has no favorites",
			items:        []model.Bookmark{{Name: "Regular", URL: "https://regular.example"}},
			disableLogin: true,
		},
		{
			name: "anonymous filters every favorite",
			items: []model.Bookmark{
				{Name: "Public Regular", URL: "https://public.example"},
				{Name: "Private Favorite", URL: "https://private.example", Private: true, Favorite: true},
			},
		},
		{
			name: "search has no favorite match",
			items: []model.Bookmark{
				{Name: "Visible Match", URL: "https://visible.example"},
				{Name: "Favorite Other", URL: "https://favorite.example", Favorite: true},
			},
			disableLogin: true,
			search:       "Visible Match",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalAppsLoader := loadFavoriteBookmarks
			originalBookmarksLoader := loadNormalBookmarks
			loadFavoriteBookmarks = func() (model.Bookmarks, error) { return model.Bookmarks{}, nil }
			loadNormalBookmarks = func() (model.Bookmarks, error) { return model.Bookmarks{Items: tt.items}, nil }
			t.Cleanup(func() {
				loadFavoriteBookmarks = originalAppsLoader
				loadNormalBookmarks = originalBookmarksLoader
			})

			e := echo.New()
			e.Renderer = emptyFavoritesRenderer{t: t}
			auth.RequestHandleWithFlags(e, model.Flags{
				DisableLoginMode: tt.disableLogin,
				CookieName:       "empty-favorites",
				CookieSecret:     "empty-favorites-secret",
				Port:             3636,
			})
			method := http.MethodGet
			handler := pageHome
			var body io.Reader
			if tt.search != "" {
				method = http.MethodPost
				handler = pageSearch
				form := url.Values{}
				form.Set("search", tt.search)
				body = strings.NewReader(form.Encode())
			}
			e.Add(method, "/", handler)
			req := httptest.NewRequest(method, "/", body)
			if tt.search != "" {
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestFavoritesDescriptionStateUsesOnlyDisplayedModules(t *testing.T) {
	tests := []struct {
		name    string
		options model.Application
		items   []model.Bookmark
		want    bool
	}{
		{
			name:    "all bookmark modules hidden",
			options: model.Application{IconMode: define.IconModeHidden},
			items:   []model.Bookmark{{Name: "Described", URL: "https://described.example", Desc: "details"}},
		},
		{
			name:    "favorites ignore regular-only descriptions",
			options: model.Application{ShowFavorites: true, IconMode: define.IconModeHidden},
			items: []model.Bookmark{
				{Name: "Described Regular", URL: "https://regular.example", Desc: "details"},
				{Name: "Favorite Without Description", URL: "https://favorite.example", Favorite: true},
			},
		},
		{
			name:    "visible favorite has description",
			options: model.Application{ShowFavorites: true, IconMode: define.IconModeHidden},
			items:   []model.Bookmark{{Name: "Described Favorite", URL: "https://favorite.example", Desc: "details", Favorite: true}},
			want:    true,
		},
		{
			name:    "visible regular bookmark has description",
			options: model.Application{ShowBookmarks: true, IconMode: define.IconModeHidden},
			items:   []model.Bookmark{{Name: "Described Regular", URL: "https://regular.example", Desc: "details"}},
			want:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalLoader := loadNormalBookmarks
			loadNormalBookmarks = func() (model.Bookmarks, error) { return model.Bookmarks{Items: tt.items}, nil }
			t.Cleanup(func() { loadNormalBookmarks = originalLoader })

			modules, err := generateBookmarkModulesWithLocalAndURLErr("", &tt.options, false, nil, true)
			if err != nil {
				t.Fatalf("generate bookmark modules: %v", err)
			}
			if modules.HasDescriptions != tt.want {
				t.Fatalf("HasDescriptions = %v, want %v", modules.HasDescriptions, tt.want)
			}
		})
	}
}

func useDescriptionStateFixtures(t *testing.T, showBookmarks bool, showFavorites bool) {
	t.Helper()
	origWd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	tmpDir := t.TempDir()
	if err := os.Chdir(tmpDir); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(origWd) })
	config := fmt.Sprintf("Title: SuperFlare\nLocale: en\nTheme: blackboard\nShowApps: false\nShowFavorites: %t\nShowBookmarks: %t\nIconMode: NONE\n", showFavorites, showBookmarks)
	if err := os.WriteFile(filepath.Join(tmpDir, "config.yml"), []byte(config), 0644); err != nil {
		t.Fatalf("write config.yml: %v", err)
	}
	writeEmptyBookmarkFixtures(t, tmpDir)
}

type descriptionStateRenderer struct {
	t    *testing.T
	want bool
}

func (r descriptionStateRenderer) Render(_ *echo.Context, _ io.Writer, _ string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		r.t.Fatalf("unexpected template data type %T", data)
	}
	got, _ := m["HasBookmarkDescriptions"].(bool)
	if got != r.want {
		r.t.Fatalf("HasBookmarkDescriptions = %v, want %v", got, r.want)
	}
	return nil
}

func TestFavoritesDescriptionHandlerStateUsesOnlyRenderedModules(t *testing.T) {
	originalRuntime := auth.SnapshotAuthRuntimeConfig()
	t.Cleanup(func() { auth.StoreAuthRuntimeConfig(originalRuntime) })
	tests := []struct {
		name          string
		path          string
		handler       echo.HandlerFunc
		showBookmarks bool
		showFavorites bool
		items         []model.Bookmark
		want          bool
	}{
		{
			name:          "home favorites ignore regular-only description",
			path:          "/",
			handler:       pageHome,
			showFavorites: true,
			items: []model.Bookmark{
				{Name: "Described Regular", URL: "https://regular.example", Desc: "details"},
				{Name: "Favorite Without Description", URL: "https://favorite.example", Favorite: true},
			},
		},
		{
			name:          "home visible favorite description",
			path:          "/",
			handler:       pageHome,
			showFavorites: true,
			items:         []model.Bookmark{{Name: "Described Favorite", URL: "https://favorite.example", Desc: "details", Favorite: true}},
			want:          true,
		},
		{
			name:    "home hidden bookmark modules",
			path:    "/",
			handler: pageHome,
			items:   []model.Bookmark{{Name: "Described Regular", URL: "https://regular.example", Desc: "details"}},
		},
		{
			name:          "bookmarks subpage hidden",
			path:          define.RegularPages.Bookmarks.Path,
			handler:       pageBookmark,
			showFavorites: true,
			items:         []model.Bookmark{{Name: "Described Favorite", URL: "https://favorite.example", Desc: "details", Favorite: true}},
		},
		{
			name:          "bookmarks subpage visible",
			path:          define.RegularPages.Bookmarks.Path,
			handler:       pageBookmark,
			showBookmarks: true,
			items:         []model.Bookmark{{Name: "Described Regular", URL: "https://regular.example", Desc: "details"}},
			want:          true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useDescriptionStateFixtures(t, tt.showBookmarks, tt.showFavorites)
			originalAppsLoader := loadFavoriteBookmarks
			originalBookmarksLoader := loadNormalBookmarks
			loadFavoriteBookmarks = func() (model.Bookmarks, error) { return model.Bookmarks{}, nil }
			loadNormalBookmarks = func() (model.Bookmarks, error) { return model.Bookmarks{Items: tt.items}, nil }
			t.Cleanup(func() {
				loadFavoriteBookmarks = originalAppsLoader
				loadNormalBookmarks = originalBookmarksLoader
			})

			e := echo.New()
			e.Renderer = descriptionStateRenderer{t: t, want: tt.want}
			auth.RequestHandleWithFlags(e, model.Flags{
				DisableLoginMode: true,
				CookieName:       "description-state",
				Port:             3636,
			})
			e.GET(tt.path, tt.handler)
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

type bookmarkTooltipRenderer struct {
	t          *testing.T
	wantScript bool
}

func (r bookmarkTooltipRenderer) Render(c *echo.Context, _ io.Writer, _ string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		r.t.Fatalf("unexpected template data type %T", data)
	}
	script, _ := m["InlineBookmarkTooltipScript"].(template.JS)
	nonce, _ := m["ScriptNonce"].(string)
	csp := c.Response().Header().Get("Content-Security-Policy")
	if !r.wantScript {
		if script != "" {
			r.t.Fatalf("tooltip script must be empty without a rendered description, got %q", script)
		}
		if nonce != "" {
			r.t.Fatalf("tooltip-only page must not receive a nonce without a rendered description, got %q", nonce)
		}
		if csp != getCSPValue("") {
			r.t.Fatalf("CSP = %q, want %q", csp, getCSPValue(""))
		}
		return nil
	}
	if script == "" {
		r.t.Fatal("expected tooltip script for a rendered bookmark description")
	}
	for _, expected := range []string{
		`500`,
		`textContent`,
		`getElementById`,
		`createElement("div")`,
		`document.body.appendChild`,
		`setAttribute("role","tooltip")`,
		`aria-describedby`,
		`document.addEventListener("pointerover"`,
		`document.addEventListener("pointerout"`,
		`document.addEventListener("focusin"`,
		`document.addEventListener("focusout"`,
		`window.addEventListener("scroll"`,
		`document.addEventListener("keydown"`,
		`visibilitychange`,
		`pagehide`,
		`Escape`,
		`innerWidth`,
		`innerHeight`,
		`Math.min`,
		`Math.max`,
	} {
		if !strings.Contains(string(script), expected) {
			r.t.Fatalf("tooltip script missing %q: %s", expected, script)
		}
	}
	if nonce == "" {
		r.t.Fatal("expected tooltip script to receive a CSP nonce")
	}
	if !strings.Contains(csp, "'nonce-"+nonce+"'") {
		r.t.Fatalf("CSP %q does not authorize tooltip nonce %q", csp, nonce)
	}
	return nil
}

func TestBookmarkTooltipHandlerBindsScriptAndNonceOnlyForRenderedDescriptions(t *testing.T) {
	originalRuntime := auth.SnapshotAuthRuntimeConfig()
	t.Cleanup(func() { auth.StoreAuthRuntimeConfig(originalRuntime) })
	tests := []struct {
		name          string
		path          string
		handler       echo.HandlerFunc
		showBookmarks bool
		showFavorites bool
		items         []model.Bookmark
		wantScript    bool
	}{
		{
			name:          "home favorite description",
			path:          "/",
			handler:       pageHome,
			showFavorites: true,
			items:         []model.Bookmark{{Name: "Favorite", URL: "https://favorite.example", Desc: "details", Favorite: true}},
			wantScript:    true,
		},
		{
			name:          "home whitespace-only bookmark description",
			path:          "/",
			handler:       pageHome,
			showBookmarks: true,
			items:         []model.Bookmark{{Name: "Blank", URL: "https://blank.example", Desc: " \t\n "}},
		},
		{
			name:          "bookmark subpage description",
			path:          define.RegularPages.Bookmarks.Path,
			handler:       pageBookmark,
			showBookmarks: true,
			items:         []model.Bookmark{{Name: "Subpage", URL: "https://subpage.example", Desc: "details"}},
			wantScript:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useDescriptionStateFixtures(t, tt.showBookmarks, tt.showFavorites)
			originalAppsLoader := loadFavoriteBookmarks
			originalBookmarksLoader := loadNormalBookmarks
			loadFavoriteBookmarks = func() (model.Bookmarks, error) { return model.Bookmarks{}, nil }
			loadNormalBookmarks = func() (model.Bookmarks, error) { return model.Bookmarks{Items: tt.items}, nil }
			t.Cleanup(func() {
				loadFavoriteBookmarks = originalAppsLoader
				loadNormalBookmarks = originalBookmarksLoader
			})

			e := echo.New()
			e.Renderer = bookmarkTooltipRenderer{t: t, wantScript: tt.wantScript}
			auth.RequestHandleWithFlags(e, model.Flags{
				DisableLoginMode: true,
				CookieName:       "bookmark-tooltip",
				Port:             3636,
			})
			e.GET(tt.path, tt.handler)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, tt.path, nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestBookmarkTooltipScriptKeepsHoverAndFocusChannelsIndependent(t *testing.T) {
	script := _inlineBookmarkTooltipScript
	for _, expected := range []string{
		`var hoverTarget=null`,
		`var hoverReady=false`,
		`var hoverTimer=null`,
		`var focusTarget=null`,
		`function clearHover()`,
		`function hideRendered()`,
		`function reconcile()`,
		`focusTarget||(hoverReady?hoverTarget:null)`,
		`if(target===hoverTarget){clearHover();reconcile();}`,
		`if(target===focusTarget){focusTarget=null;reconcile();}`,
		`function reset(){clearHover();focusTarget=null;hideRendered();}`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("tooltip state machine missing %q: %s", expected, script)
		}
	}
	for _, legacy := range []string{`var pendingTarget=null`, `function hide(){cancelTimer()`} {
		if strings.Contains(script, legacy) {
			t.Fatalf("tooltip state machine still shares destructive state %q: %s", legacy, script)
		}
	}
}

func TestBookmarkTooltipScriptCleansDisconnectedTargets(t *testing.T) {
	script := _inlineBookmarkTooltipScript
	for _, expected := range []string{
		`if(!target||!target.isConnected)`,
		`!scheduledTarget.isConnected`,
		`new MutationObserver`,
		`function cleanupDisconnectedTargets()`,
		`activeTarget&&!activeTarget.isConnected`,
		`observer.observe(document.body,{childList:true,subtree:true})`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("tooltip script missing disconnected-target cleanup %q: %s", expected, script)
		}
	}
}

type favoritesModuleRenderer struct {
	t         *testing.T
	wantTitle string
}

func (r favoritesModuleRenderer) Render(_ *echo.Context, _ io.Writer, _ string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		r.t.Fatalf("unexpected template data type %T", data)
	}
	if show, _ := m["OptionShowFavorites"].(bool); !show {
		r.t.Fatal("expected non-empty enabled favorites module")
	}
	if favorites, _ := m["Favorites"].(template.HTML); favorites == "" {
		r.t.Fatal("expected rendered favorites HTML")
	}
	if title, _ := m["FavoritesTitle"].(string); title != r.wantTitle {
		r.t.Fatalf("FavoritesTitle = %q, want %q", title, r.wantTitle)
	}
	return nil
}

func TestFavoritesModuleHandlerBindsTrimmedCustomAndLocalizedTitles(t *testing.T) {
	originalRuntime := auth.SnapshotAuthRuntimeConfig()
	t.Cleanup(func() { auth.StoreAuthRuntimeConfig(originalRuntime) })
	tests := []struct {
		name        string
		customTitle string
		wantTitle   string
	}{
		{name: "localized default", wantTitle: "Favorites"},
		{name: "trimmed custom title", customTitle: `FavoritesTitle: "  Pinned  "` + "\n", wantTitle: "Pinned"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			useDescriptionStateFixtures(t, false, true)
			if tt.customTitle != "" {
				config, err := os.ReadFile("config.yml")
				if err != nil {
					t.Fatalf("read config.yml: %v", err)
				}
				if err := os.WriteFile("config.yml", append(config, tt.customTitle...), 0644); err != nil {
					t.Fatalf("write custom favorites title: %v", err)
				}
			}
			originalAppsLoader := loadFavoriteBookmarks
			originalBookmarksLoader := loadNormalBookmarks
			loadFavoriteBookmarks = func() (model.Bookmarks, error) { return model.Bookmarks{}, nil }
			loadNormalBookmarks = func() (model.Bookmarks, error) {
				return model.Bookmarks{Items: []model.Bookmark{{Name: "Favorite", URL: "https://favorite.example", Favorite: true}}}, nil
			}
			t.Cleanup(func() {
				loadFavoriteBookmarks = originalAppsLoader
				loadNormalBookmarks = originalBookmarksLoader
			})

			e := echo.New()
			e.Renderer = favoritesModuleRenderer{t: t, wantTitle: tt.wantTitle}
			auth.RequestHandleWithFlags(e, model.Flags{DisableLoginMode: true, CookieName: "favorites-title", Port: 3636})
			e.GET("/", pageHome)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestPrivateSearchFiltersBeforeRendering(t *testing.T) {
	usePrivateProjectionFixtures(t)
	options := model.Application{IconMode: define.IconModeHidden}

	anonymousBookmarks, err := generateBookmarkModulesWithLocalAndURLErr("alpha", &options, false, nil, false)
	if err != nil {
		t.Fatalf("generate anonymous filtered bookmarks: %v", err)
	}
	if strings.Contains(string(anonymousBookmarks.Bookmarks), "alpha") || strings.Contains(string(anonymousBookmarks.Favorites), "alpha") {
		t.Fatalf("search must not reveal a private bookmark: bookmarks=%s favorites=%s", anonymousBookmarks.Bookmarks, anonymousBookmarks.Favorites)
	}

	anonymousApps, err := generateApplicationsTemplateWithLocalAndURLErr("Private App", &options, false, nil, false)
	if err != nil {
		t.Fatalf("generate anonymous filtered applications: %v", err)
	}
	if strings.Contains(string(anonymousApps), "Private App") {
		t.Fatalf("search must not reveal a private application: %s", anonymousApps)
	}

	trustedBookmarks, err := generateBookmarkModulesWithLocalAndURLErr("alpha", &options, false, nil, true)
	if err != nil {
		t.Fatalf("generate trusted filtered bookmarks: %v", err)
	}
	trustedApps, err := generateApplicationsTemplateWithLocalAndURLErr("Private App", &options, false, nil, true)
	if err != nil {
		t.Fatalf("generate trusted filtered applications: %v", err)
	}
	if !strings.Contains(string(trustedBookmarks.Bookmarks), "alpha") || !strings.Contains(string(trustedBookmarks.Favorites), "alpha") {
		t.Fatalf("trusted search should include private bookmark: bookmarks=%s favorites=%s", trustedBookmarks.Bookmarks, trustedBookmarks.Favorites)
	}
	if !strings.Contains(string(trustedApps), "Private App") {
		t.Fatalf("trusted search should include private application: %s", trustedApps)
	}
}

type failingSessionStore struct{}

func (failingSessionStore) Get(*http.Request, string) (*sessions.Session, error) {
	return nil, errors.New("session backend unavailable")
}

func (store failingSessionStore) New(_ *http.Request, name string) (*sessions.Session, error) {
	return sessions.NewSession(store, name), nil
}

func (failingSessionStore) Save(*http.Request, http.ResponseWriter, *sessions.Session) error {
	return nil
}

func authenticatedCookie(t *testing.T, flags model.Flags) *http.Cookie {
	t.Helper()
	store := sessions.NewCookieStore([]byte(flags.CookieSecret))
	store.MaxAge(auth.SESSION_MAX_AGE)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	sess, err := store.New(req, auth.RequestHandleSessionName(flags.CookieName, flags.Port))
	if err != nil {
		t.Fatalf("create authenticated session: %v", err)
	}
	sess.Values[auth.SESSION_KEY_USER_NAME] = "admin"
	sess.Values[auth.SESSION_KEY_LOGIN_DATE] = "2026-07-15 12:00:00 CST"
	sess.Options = &sessions.Options{Path: "/", MaxAge: auth.SESSION_MAX_AGE}
	rec := httptest.NewRecorder()
	if err := store.Save(req, rec, sess); err != nil {
		t.Fatalf("save authenticated session: %v", err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected one authenticated session cookie, got %d", len(cookies))
	}
	return cookies[0]
}

func TestCanViewPrivateItemsTrustState(t *testing.T) {
	originalRuntime := auth.SnapshotAuthRuntimeConfig()
	t.Cleanup(func() { auth.StoreAuthRuntimeConfig(originalRuntime) })

	baseFlags := model.Flags{
		CookieName:   "private-visibility",
		CookieSecret: "private-visibility-secret",
		Port:         3636,
		User:         "admin",
		Pass:         "password",
	}
	tests := []struct {
		name           string
		disableLogin   bool
		authenticated  bool
		invalidCookie  bool
		failingSession bool
		want           bool
	}{
		{name: "disabled login is trusted", disableLogin: true, want: true},
		{name: "anonymous enabled login is untrusted", want: false},
		{name: "authenticated enabled login is trusted", authenticated: true, want: true},
		{name: "invalid session is untrusted", invalidCookie: true, want: false},
		{name: "session read failure is untrusted", failingSession: true, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flags := baseFlags
			flags.DisableLoginMode = tt.disableLogin
			e := echo.New()
			auth.RequestHandleWithFlags(e, flags)
			if tt.failingSession {
				e.Use(echosession.Middleware(failingSessionStore{}))
			}
			e.GET("/visibility", func(c *echo.Context) error {
				if canViewPrivateItems(c) {
					return c.String(http.StatusOK, "trusted")
				}
				return c.String(http.StatusOK, "untrusted")
			})

			req := httptest.NewRequest(http.MethodGet, "/visibility", nil)
			switch {
			case tt.authenticated:
				req.AddCookie(authenticatedCookie(t, flags))
			case tt.invalidCookie:
				req.AddCookie(&http.Cookie{
					Name:  auth.RequestHandleSessionName(flags.CookieName, flags.Port),
					Value: "invalid-cookie-value",
				})
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			got := rec.Body.String() == "trusted"
			if got != tt.want {
				t.Fatalf("canViewPrivateItems() = %v, want %v", got, tt.want)
			}
		})
	}
}

type privateProjectionRenderer struct {
	t              *testing.T
	wantPrivate    bool
	checkApps      bool
	checkBookmarks bool
	checkFavorites bool
}

func (r privateProjectionRenderer) Render(_ *echo.Context, _ io.Writer, _ string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		r.t.Fatalf("unexpected template data type %T", data)
	}
	checks := []struct {
		enabled     bool
		key         string
		publicName  string
		privateName string
	}{
		{enabled: r.checkApps, key: "Applications", publicName: "Public App", privateName: "Private App"},
		{enabled: r.checkBookmarks, key: "Bookmarks", publicName: "Zulu", privateName: "alpha"},
		{enabled: r.checkFavorites, key: "Favorites", publicName: "Zulu", privateName: "alpha"},
	}
	for _, check := range checks {
		if !check.enabled {
			continue
		}
		htmlValue, ok := m[check.key].(template.HTML)
		if !ok {
			r.t.Fatalf("expected %s HTML, got %T", check.key, m[check.key])
		}
		htmlText := string(htmlValue)
		if !strings.Contains(htmlText, check.publicName) {
			r.t.Fatalf("expected %s to contain public item %q: %s", check.key, check.publicName, htmlText)
		}
		if got := strings.Contains(htmlText, check.privateName); got != r.wantPrivate {
			r.t.Fatalf("%s private visibility = %v, want %v: %s", check.key, got, r.wantPrivate, htmlText)
		}
	}
	return nil
}

func TestPrivateItemsAreFilteredOnHomeAndSubpages(t *testing.T) {
	usePrivateProjectionFixtures(t)
	originalRuntime := auth.SnapshotAuthRuntimeConfig()
	t.Cleanup(func() { auth.StoreAuthRuntimeConfig(originalRuntime) })

	tests := []struct {
		name           string
		path           string
		method         string
		handler        echo.HandlerFunc
		search         string
		disableLogin   bool
		checkApps      bool
		checkBookmarks bool
		checkFavorites bool
	}{
		{name: "anonymous home", path: "/", handler: pageHome, checkApps: true, checkBookmarks: true, checkFavorites: true},
		{name: "anonymous home search", path: "/", method: http.MethodPost, handler: pageSearch, search: "example", checkApps: true, checkBookmarks: true, checkFavorites: true},
		{name: "anonymous applications subpage", path: define.RegularPages.Applications.Path, handler: pageApplication, checkApps: true},
		{name: "anonymous bookmarks subpage", path: define.RegularPages.Bookmarks.Path, handler: pageBookmark, checkBookmarks: true},
		{name: "disabled-login home", path: "/", handler: pageHome, disableLogin: true, checkApps: true, checkBookmarks: true, checkFavorites: true},
		{name: "disabled-login applications subpage", path: define.RegularPages.Applications.Path, handler: pageApplication, disableLogin: true, checkApps: true},
		{name: "disabled-login bookmarks subpage", path: define.RegularPages.Bookmarks.Path, handler: pageBookmark, disableLogin: true, checkBookmarks: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			method := tt.method
			if method == "" {
				method = http.MethodGet
			}
			flags := model.Flags{
				DisableLoginMode: tt.disableLogin,
				CookieName:       "handler-private-visibility",
				CookieSecret:     "handler-private-visibility-secret",
				Port:             3636,
			}
			e := echo.New()
			e.Renderer = privateProjectionRenderer{
				t:              t,
				wantPrivate:    tt.disableLogin,
				checkApps:      tt.checkApps,
				checkBookmarks: tt.checkBookmarks,
				checkFavorites: tt.checkFavorites,
			}
			auth.RequestHandleWithFlags(e, flags)
			e.Add(method, tt.path, tt.handler)
			var body io.Reader
			if tt.search != "" {
				form := url.Values{}
				form.Set("search", tt.search)
				body = strings.NewReader(form.Encode())
			}
			req := httptest.NewRequest(method, tt.path, body)
			if tt.search != "" {
				req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationForm)
			}
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

type privateIconWarningRenderer struct {
	t                   *testing.T
	wantAppWarning      bool
	wantBookmarkWarning bool
}

func (r privateIconWarningRenderer) Render(_ *echo.Context, _ io.Writer, _ string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		r.t.Fatalf("unexpected template data type %T", data)
	}
	warnings, _ := m["RenderWarnings"].([]string)
	joined := strings.Join(warnings, "\n")
	if got := strings.Contains(joined, "App icon config fallback"); got != r.wantAppWarning {
		r.t.Fatalf("app warning present = %v, want %v: %#v", got, r.wantAppWarning, warnings)
	}
	if got := strings.Contains(joined, "Bookmark icon config fallback"); got != r.wantBookmarkWarning {
		r.t.Fatalf("bookmark warning present = %v, want %v: %#v", got, r.wantBookmarkWarning, warnings)
	}
	return nil
}

func TestPrivateIconWarningsUseOnlyVisibleHandlerItems(t *testing.T) {
	usePrivateIconWarningFixtures(t)
	originalRuntime := auth.SnapshotAuthRuntimeConfig()
	t.Cleanup(func() { auth.StoreAuthRuntimeConfig(originalRuntime) })

	tests := []struct {
		name                string
		path                string
		handler             echo.HandlerFunc
		disableLogin        bool
		wantAppWarning      bool
		wantBookmarkWarning bool
	}{
		{name: "anonymous home", path: "/", handler: pageHome},
		{name: "trusted home", path: "/", handler: pageHome, disableLogin: true, wantAppWarning: true, wantBookmarkWarning: true},
		{name: "anonymous applications subpage", path: define.RegularPages.Applications.Path, handler: pageApplication},
		{name: "trusted applications subpage", path: define.RegularPages.Applications.Path, handler: pageApplication, disableLogin: true, wantAppWarning: true},
		{name: "anonymous bookmarks subpage", path: define.RegularPages.Bookmarks.Path, handler: pageBookmark},
		{name: "trusted bookmarks subpage", path: define.RegularPages.Bookmarks.Path, handler: pageBookmark, disableLogin: true, wantBookmarkWarning: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := echo.New()
			e.Renderer = privateIconWarningRenderer{
				t:                   t,
				wantAppWarning:      tt.wantAppWarning,
				wantBookmarkWarning: tt.wantBookmarkWarning,
			}
			auth.RequestHandleWithFlags(e, model.Flags{
				DisableLoginMode: tt.disableLogin,
				CookieName:       "private-icon-warning",
				CookieSecret:     "private-icon-warning-secret",
				Port:             3636,
			})
			e.GET(tt.path, tt.handler)
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
		})
	}
}

type sourceLoadRenderer struct {
	t              *testing.T
	checkApps      bool
	checkBookmarks bool
}

func (r sourceLoadRenderer) Render(_ *echo.Context, _ io.Writer, _ string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		r.t.Fatalf("unexpected template data type %T", data)
	}
	if r.checkApps {
		htmlValue, _ := m["Applications"].(template.HTML)
		if !strings.Contains(string(htmlValue), "Injected App") {
			r.t.Fatalf("application handler did not render injected source: %s", htmlValue)
		}
	}
	if r.checkBookmarks {
		htmlValue, _ := m["Bookmarks"].(template.HTML)
		if !strings.Contains(string(htmlValue), "Injected Bookmark") {
			r.t.Fatalf("bookmark handler did not render injected source: %s", htmlValue)
		}
	}
	return nil
}

type applicationSubdirectoryRenderer struct {
	t            *testing.T
	wantModalApp string
}

func (r applicationSubdirectoryRenderer) Render(c *echo.Context, _ io.Writer, _ string, data any) error {
	r.t.Helper()
	m, ok := data.(map[string]any)
	if !ok {
		r.t.Fatalf("unexpected template data type %T", data)
	}
	modals, ok := m["ApplicationSubdirectoryModals"].(template.HTML)
	if !ok {
		r.t.Fatalf("expected ApplicationSubdirectoryModals HTML, got %T", m["ApplicationSubdirectoryModals"])
	}
	hasDirectories, ok := m["HasApplicationSubdirectories"].(bool)
	if !ok {
		r.t.Fatalf("expected HasApplicationSubdirectories bool, got %T", m["HasApplicationSubdirectories"])
	}
	modalScript, ok := m["InlineApplicationSubdirectoryModalScript"].(template.JS)
	if !ok {
		r.t.Fatalf("expected InlineApplicationSubdirectoryModalScript JS, got %T", m["InlineApplicationSubdirectoryModalScript"])
	}
	nonce, ok := m["ScriptNonce"].(string)
	if !ok {
		r.t.Fatalf("expected ScriptNonce string, got %T", m["ScriptNonce"])
	}
	csp := c.Response().Header().Get("Content-Security-Policy")
	if r.wantModalApp == "" {
		if modals != "" || hasDirectories || modalScript != "" {
			r.t.Fatalf("expected no application subdirectory modal behavior, got hasDirectories=%v modals=%s script=%s", hasDirectories, modals, modalScript)
		}
		if nonce != "" || csp != getCSPValue("") {
			r.t.Fatalf("modal-free page nonce=%q CSP=%q, want no nonce and %q", nonce, csp, getCSPValue(""))
		}
		return nil
	}
	if !hasDirectories || !strings.Contains(string(modals), r.wantModalApp) {
		r.t.Fatalf("expected application subdirectory modal for %q, got hasDirectories=%v modals=%s", r.wantModalApp, hasDirectories, modals)
	}
	for _, expected := range []string{
		`window.addEventListener("hashchange"`,
		`/^application-subdir-modal-\d+$/`,
		`var id=window.location.hash.slice(1)`,
		`document.getElementById(id)`,
		`event.key==="Escape"`,
		`event.key!=="Tab"`,
		`origin.closest(closeSelector)`,
		`event.preventDefault();closeActiveModal();return;`,
		`window.location.hash=""`,
		`window.setTimeout(function(){trigger.focus({preventScroll:true});},0)`,
		`setAttribute("inert","")`,
		`panel.focus({preventScroll:true})`,
		`trigger.focus({preventScroll:true})`,
	} {
		if !strings.Contains(string(modalScript), expected) {
			r.t.Fatalf("application subdirectory modal script missing %q: %s", expected, modalScript)
		}
	}
	for _, unexpected := range []string{`history.replaceState`, `querySelector(window.location.hash)`} {
		if strings.Contains(string(modalScript), unexpected) {
			r.t.Fatalf("application subdirectory modal script must not contain %q: %s", unexpected, modalScript)
		}
	}
	if nonce == "" || csp != getCSPValue(nonce) {
		r.t.Fatalf("modal page nonce=%q CSP=%q, want a matching nonce CSP", nonce, csp)
	}
	return nil
}

func TestApplicationSubdirectoryHandlerBindings(t *testing.T) {
	usePrivateProjectionFixtures(t)
	originalRuntime := auth.SnapshotAuthRuntimeConfig()
	originalLoader := loadFavoriteBookmarks
	t.Cleanup(func() {
		auth.StoreAuthRuntimeConfig(originalRuntime)
		loadFavoriteBookmarks = originalLoader
	})

	handlers := []struct {
		name    string
		path    string
		handler echo.HandlerFunc
	}{
		{name: "home", path: "/", handler: func(c *echo.Context) error { return render(c, "") }},
		{name: "search", path: "/search", handler: func(c *echo.Context) error { return render(c, "Operations") }},
		{name: "applications", path: define.RegularPages.Applications.Path, handler: pageApplication},
	}
	scenarios := []struct {
		name         string
		items        []model.Bookmark
		wantModalApp string
	}{
		{
			name: "visible directory",
			items: []model.Bookmark{{
				Name: "Operations Tool", URL: "https://operations.example", Subdir: "Operations",
			}},
			wantModalApp: "Operations Tool",
		},
		{
			name: "only ungrouped or filtered private matches",
			items: []model.Bookmark{
				{Name: "Operations Plain", URL: "https://plain.example"},
				{Name: "Operations Secret", URL: "https://secret.example", Subdir: "Operations", Private: true},
			},
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			loadFavoriteBookmarks = func() (model.Bookmarks, error) {
				return model.Bookmarks{Items: scenario.items}, nil
			}
			for _, handler := range handlers {
				t.Run(handler.name, func(t *testing.T) {
					e := echo.New()
					e.Renderer = applicationSubdirectoryRenderer{t: t, wantModalApp: scenario.wantModalApp}
					auth.RequestHandleWithFlags(e, model.Flags{
						CookieName:   "application-subdirectory-handler",
						CookieSecret: "application-subdirectory-handler-secret",
						Port:         3636,
					})
					e.GET(handler.path, handler.handler)
					req := httptest.NewRequest(http.MethodGet, handler.path, nil)
					rec := httptest.NewRecorder()
					e.ServeHTTP(rec, req)
					if rec.Code != http.StatusOK {
						t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
					}
				})
			}
		})
	}
}

func TestPrivateHandlersLoadEachVisibleSourceOnce(t *testing.T) {
	usePrivateProjectionFixtures(t)
	config := strings.Join([]string{
		"Title: SuperFlare",
		"Locale: en",
		"Theme: blackboard",
		"ShowApps: true",
		"ShowFavorites: true",
		"ShowBookmarks: true",
		"IconMode: DEFAULT",
	}, "\n") + "\n"
	if err := os.WriteFile("config.yml", []byte(config), 0644); err != nil {
		t.Fatalf("write visible-icon config: %v", err)
	}
	originalRuntime := auth.SnapshotAuthRuntimeConfig()
	t.Cleanup(func() { auth.StoreAuthRuntimeConfig(originalRuntime) })

	tests := []struct {
		name              string
		path              string
		handler           echo.HandlerFunc
		checkApps         bool
		checkBookmarks    bool
		wantAppsLoads     int
		wantBookmarkLoads int
	}{
		{name: "home", path: "/", handler: pageHome, checkApps: true, checkBookmarks: true, wantAppsLoads: 1, wantBookmarkLoads: 1},
		{name: "applications subpage", path: define.RegularPages.Applications.Path, handler: pageApplication, checkApps: true, wantAppsLoads: 1},
		{name: "bookmarks subpage", path: define.RegularPages.Bookmarks.Path, handler: pageBookmark, checkBookmarks: true, wantBookmarkLoads: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			originalAppsLoader := loadFavoriteBookmarks
			originalBookmarksLoader := loadNormalBookmarks
			appsLoads := 0
			bookmarkLoads := 0
			loadFavoriteBookmarks = func() (model.Bookmarks, error) {
				appsLoads++
				return model.Bookmarks{Items: []model.Bookmark{{Name: "Injected App", URL: "https://injected-app.example"}}}, nil
			}
			loadNormalBookmarks = func() (model.Bookmarks, error) {
				bookmarkLoads++
				return model.Bookmarks{Items: []model.Bookmark{{Name: "Injected Bookmark", URL: "https://injected-bookmark.example", Favorite: true}}}, nil
			}
			t.Cleanup(func() {
				loadFavoriteBookmarks = originalAppsLoader
				loadNormalBookmarks = originalBookmarksLoader
			})

			e := echo.New()
			e.Renderer = sourceLoadRenderer{t: t, checkApps: tt.checkApps, checkBookmarks: tt.checkBookmarks}
			auth.RequestHandleWithFlags(e, model.Flags{
				DisableLoginMode: true,
				CookieName:       "single-source-load",
				Port:             3636,
			})
			e.GET(tt.path, tt.handler)
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			e.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
			}
			if appsLoads != tt.wantAppsLoads || bookmarkLoads != tt.wantBookmarkLoads {
				t.Fatalf("source loads = apps:%d bookmarks:%d, want apps:%d bookmarks:%d", appsLoads, bookmarkLoads, tt.wantAppsLoads, tt.wantBookmarkLoads)
			}
		})
	}
}

func TestSetCSPHeader_WhenDisableCSPFalse_SetsHeader(t *testing.T) {
	orig := saveAppFlags()
	origRuntime, origRuntimeSet := saveHomeRuntimeFlags()
	defer restoreAppFlags(orig)
	defer restoreHomeRuntimeFlags(origRuntime, origRuntimeSet)
	define.AppFlags.DisableCSP = false
	homeRuntimeFlags.Store(homeRuntimeSnapshotFromFlags(define.AppFlags))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	setCSPHeader(c, "nonce-value")

	assert.Equal(t, "script-src 'nonce-nonce-value'; "+_cspValue, rec.Header().Get("Content-Security-Policy"))
}

func TestSetCSPHeader_WhenDisableCSPTrue_NoHeader(t *testing.T) {
	orig := saveAppFlags()
	origRuntime, origRuntimeSet := saveHomeRuntimeFlags()
	defer restoreAppFlags(orig)
	defer restoreHomeRuntimeFlags(origRuntime, origRuntimeSet)
	define.AppFlags.DisableCSP = true
	homeRuntimeFlags.Store(homeRuntimeSnapshotFromFlags(define.AppFlags))

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	setCSPHeader(c, "")

	assert.Empty(t, rec.Header().Get("Content-Security-Policy"))
}

func TestSetCSPHeader_UsesStoredRuntimeDisableCSPAfterAppFlagsChange(t *testing.T) {
	orig := saveAppFlags()
	origRuntime, origRuntimeSet := saveHomeRuntimeFlags()
	defer restoreAppFlags(orig)
	defer restoreHomeRuntimeFlags(origRuntime, origRuntimeSet)

	define.AppFlags.DisableCSP = false
	homeRuntimeFlags.Store(homeRuntimeSnapshotFromFlags(define.AppFlags))
	define.AppFlags.DisableCSP = true

	e := echo.New()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)

	setCSPHeader(c, "nonce-value")

	assert.Equal(t, "script-src 'nonce-nonce-value'; "+_cspValue, rec.Header().Get("Content-Security-Policy"))
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

func TestInlineSiteIconRefreshScriptDeduplicatesSourcesWithoutPolling(t *testing.T) {
	script := string(inlineSiteIconRefreshScript(model.Application{IconMode: define.IconModeMissingFill}))
	for _, expected := range []string{
		`var groups=new Map()`,
		`groups.get(src)`,
		`groups.set(src,[node])`,
		`groups.forEach(function(group,src)`,
		`group.forEach(function(node)`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("favicon refresh script should contain %q: %s", expected, script)
		}
	}
	if got := strings.Count(script, `fetch(src)`); got != 1 {
		t.Fatalf("favicon refresh script should contain one grouped fetch call, got %d: %s", got, script)
	}
	for _, unexpected := range []string{`setTimeout`, `var left=`, `cache:"no-store"`} {
		if strings.Contains(script, unexpected) {
			t.Fatalf("favicon refresh script should not contain %q: %s", unexpected, script)
		}
	}
}

func TestInlineSiteIconRefreshScriptDecodesBlobBeforeReplacingFallback(t *testing.T) {
	script := string(inlineSiteIconRefreshScript(model.Application{IconMode: define.IconModeMissingFill}))
	for _, expected := range []string{
		`var probe=new Image()`,
		`probe.decode().then(apply,discard)`,
		`}else{probe.onload=apply;probe.onerror=discard;`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("favicon refresh script should validate fetched image with %q: %s", expected, script)
		}
	}
}

func TestInlineSiteIconRefreshScriptReplacesInlineFallbackAfterDecode(t *testing.T) {
	script := string(inlineSiteIconRefreshScript(model.Application{IconMode: define.IconModeMissingFill}))
	for _, expected := range []string{
		`querySelectorAll("[data-site-icon-src]")`,
		`node.tagName==="IMG"`,
		`node.replaceWith(img)`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("favicon refresh script should support inline fallback with %q: %s", expected, script)
		}
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
	assert.Contains(t, style, ".apps-surface{display:grid;grid-template-columns:repeat(auto-fill,minmax(max(180px,calc((100% - (4 - 1) * 18px) / 4)),1fr));column-gap:18px;row-gap:0;align-items:start;}")
	assert.Contains(t, style, ".apps-surface .app-container{float:none;width:auto;min-width:0;}")
	assert.NotContains(t, style, "#container-apps .apps-container{display:grid")
	assert.Contains(t, style, ".bookmark-module .bookmark-groups{display:grid;grid-template-columns:repeat(auto-fill,minmax(max(180px,calc((100% - (4 - 1) * 18px) / 4)),1fr));column-count:auto;column-gap:18px;gap:18px;align-items:start;}")
	assert.Contains(t, style, ".bookmark-module .bookmark-group-container{break-inside:auto;display:block;width:auto;max-width:none;min-width:0;")
	assert.Contains(t, style, "@media (max-width:560px){.bookmark-module .bookmark-groups{display:block;column-count:2;column-gap:18px;}")
	assert.NotContains(t, style, ";};}")
}

func TestApplicationSubdirectoryModalStyleContracts(t *testing.T) {
	style := define.PAGE_INLINE_STYLE

	for _, expected := range []string{
		`.apps-surface {`,
		`.apps-surface .app-container {`,
		`.apps-surface .app-item {`,
		`.app-content-uppercase .apps-surface .app-text {`,
		`.application-subdirectory-trigger .app-text {`,
		`.application-subdirectory-trigger .app-title {`,
		`.application-subdirectory-modal {`,
		`.application-subdirectory-modal:target {`,
		`.application-subdirectory-backdrop {`,
		`.application-subdirectory-panel {`,
		`.application-subdirectory-header {`,
		`.application-subdirectory-close {`,
		`.application-subdirectory-content {`,
		`body:has(.application-subdirectory-modal:target) {`,
	} {
		if !strings.Contains(style, expected) {
			t.Errorf("generated home CSS missing %q", expected)
		}
	}
	baseTitleIndex := strings.Index(style, `.apps-surface .app-title {`)
	folderTitleIndex := strings.Index(style, `.application-subdirectory-trigger .app-title {`)
	if baseTitleIndex < 0 || folderTitleIndex < baseTitleIndex {
		t.Errorf("folder title override must follow the base application title rule")
	}

	contracts := []struct {
		name string
		css  string
	}{
		{name: "folder text fills card", css: `.application-subdirectory-trigger .app-text {display: flex;align-items: center;height: 100%;}`},
		{name: "folder title fills text width", css: `.application-subdirectory-trigger .app-title {width: 100%;margin: 0;}`},
		{name: "overlay containment", css: `.application-subdirectory-modal {position: fixed;inset: 0;z-index: 40;display: flex;align-items: center;justify-content: center;padding: 16px;overflow: hidden;visibility: hidden;`},
		{name: "target visibility", css: `.application-subdirectory-modal:target {visibility: visible;opacity: 1;pointer-events: auto;`},
		{name: "bounded panel", css: `.application-subdirectory-panel {position: relative;z-index: 1;display: flex;flex-direction: column;width: min(760px, calc(100vw - 32px));min-width: min(420px, calc(100vw - 32px));max-width: min(760px, calc(100vw - 32px));height: min(68vh, 680px);min-height: min(320px, calc(100vh - 32px));max-height: min(82vh, 760px);overflow: hidden;`},
		{name: "scrollable content", css: `.application-subdirectory-content {flex: 1 1 auto;min-height: 0;overflow: auto;overscroll-behavior: contain;}`},
		{name: "body scroll lock", css: `body:has(.application-subdirectory-modal:target) {overflow: hidden;}`},
	}
	for _, contract := range contracts {
		if !strings.Contains(style, contract.css) {
			t.Errorf("generated home CSS missing %s contract %q", contract.name, contract.css)
		}
	}
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
	}, false, false, "", false, nil)

	html := b.String()
	if !strings.Contains(html, `class="bookmark-label"`) {
		t.Fatalf("expected bookmark label span class, got %s", html)
	}
}

func TestBookmarkTooltipDescriptionAttributeEscapesAndTrims(t *testing.T) {
	var b strings.Builder

	renderBookmarkItem(&b, model.Bookmark{
		Name: "Example",
		URL:  "https://example.com",
		Desc: `  status "quoted" & <tag> 'single'  `,
	}, false, false, define.IconModeHidden, false, nil)

	html := b.String()
	want := `data-bookmark-description="status &#34;quoted&#34; &amp; &lt;tag&gt; &#39;single&#39;"`
	if !strings.Contains(html, want) {
		t.Fatalf("expected escaped trimmed description attribute %q, got %s", want, html)
	}
	for _, unsafe := range []string{`status "quoted"`, `<tag>`, `  status`} {
		if strings.Contains(html, unsafe) {
			t.Fatalf("bookmark description leaked unsafe or untrimmed text %q: %s", unsafe, html)
		}
	}
}

func TestBookmarkTooltipDescriptionAttributeOmittedWhenBlank(t *testing.T) {
	var b strings.Builder
	renderBookmarkItem(&b, model.Bookmark{
		Name: "Example",
		URL:  "https://example.com",
		Desc: " \t\r\n ",
	}, false, false, define.IconModeHidden, false, nil)

	if strings.Contains(b.String(), "data-bookmark-description") {
		t.Fatalf("whitespace-only description must not emit tooltip data: %s", b.String())
	}
}

func TestBookmarkTooltipIsNotAddedToApplicationCards(t *testing.T) {
	originalLoader := loadFavoriteBookmarks
	loadFavoriteBookmarks = func() (model.Bookmarks, error) {
		return model.Bookmarks{Items: []model.Bookmark{{
			Name: "Application",
			URL:  "https://application.example",
			Desc: "visible application description",
		}}}, nil
	}
	t.Cleanup(func() { loadFavoriteBookmarks = originalLoader })

	projection, err := generateApplicationProjectionWithLocalAndURLErr("", &model.Application{IconMode: define.IconModeHidden}, false, nil, true)
	if err != nil {
		t.Fatalf("generate application projection: %v", err)
	}
	if strings.Contains(string(projection.HTML), "data-bookmark-description") {
		t.Fatalf("application cards must not receive bookmark tooltip data: %s", projection.HTML)
	}
}

func generateApplicationProjectionForItems(t *testing.T, items []model.Bookmark, filter string, canViewPrivate bool) applicationProjection {
	t.Helper()
	originalLoader := loadFavoriteBookmarks
	loadFavoriteBookmarks = func() (model.Bookmarks, error) {
		return model.Bookmarks{Items: items}, nil
	}
	t.Cleanup(func() { loadFavoriteBookmarks = originalLoader })

	projection, err := generateApplicationProjectionWithLocalAndURLErr(filter, &model.Application{IconMode: define.IconModeHidden}, false, nil, canViewPrivate)
	if err != nil {
		t.Fatalf("generate application projection: %v", err)
	}
	return projection
}

func TestApplicationProjectionRendersSortedDirectoriesBeforeUngroupedApps(t *testing.T) {
	projection := generateApplicationProjectionForItems(t, []model.Bookmark{
		{Name: "Zulu One", URL: "https://zulu-one.example", Subdir: "zeta"},
		{Name: "Plain", URL: "https://plain.example"},
		{Name: "Alpha Two", URL: "https://alpha-two.example", Subdir: " Alpha "},
		{Name: "Alpha One", URL: "https://alpha-one.example", Subdir: "Alpha"},
	}, "", true)

	mainHTML := string(projection.HTML)
	modalHTML := string(projection.Modals)
	alpha := strings.Index(mainHTML, `data-application-subdirectory="Alpha"`)
	zeta := strings.Index(mainHTML, `data-application-subdirectory="zeta"`)
	plain := strings.Index(mainHTML, `title="Plain"`)
	if alpha < 0 || zeta < 0 || plain < 0 || !(alpha < zeta && zeta < plain) {
		t.Fatalf("unexpected main application order: %s", mainHTML)
	}
	if !projection.HasDirectories {
		t.Fatal("expected directory projection flag")
	}
	for _, name := range []string{"Alpha One", "Alpha Two", "Zulu One"} {
		if strings.Contains(mainHTML, name) {
			t.Fatalf("grouped application %q must not be duplicated in the main list: %s", name, mainHTML)
		}
		if strings.Count(modalHTML, `title="`+name+`"`) != 1 {
			t.Fatalf("expected one modal item %q: %s", name, modalHTML)
		}
	}
	if strings.Index(modalHTML, `title="Alpha Two"`) > strings.Index(modalHTML, `title="Alpha One"`) {
		t.Fatalf("modal applications must keep source order: %s", modalHTML)
	}
}

func TestApplicationDirectoryTriggerUsesFullHeightSingleLineMarkup(t *testing.T) {
	projection := generateApplicationProjectionForItems(t, []model.Bookmark{{
		Name: "Folder App", URL: "https://folder.example", Subdir: "Operations",
	}}, "", true)

	mainHTML := string(projection.HTML)
	triggerEnd := strings.Index(mainHTML, `</a></div>`)
	if triggerEnd < 0 {
		t.Fatalf("directory trigger closing markup missing: %s", mainHTML)
	}
	triggerHTML := mainHTML[:triggerEnd+len(`</a></div>`)]
	if strings.Contains(triggerHTML, `class="app-desc"`) {
		t.Fatalf("directory trigger must not reserve a description row: %s", triggerHTML)
	}
	if strings.Count(triggerHTML, `class="app-title"`) != 1 {
		t.Fatalf("directory trigger must contain exactly one title: %s", triggerHTML)
	}
}

func TestApplicationProjectionTreatsWhitespaceSubdirAsUngrouped(t *testing.T) {
	projection := generateApplicationProjectionForItems(t, []model.Bookmark{{
		Name: "Loose", URL: "https://loose.example", Subdir: " \t\r\n ",
	}}, "", true)

	mainHTML := string(projection.HTML)
	if !strings.Contains(mainHTML, `title="Loose"`) {
		t.Fatalf("whitespace-only subdirectory must remain an ordinary card: %s", mainHTML)
	}
	if strings.Contains(mainHTML, "data-application-subdirectory") || projection.HasDirectories || projection.Modals != "" {
		t.Fatalf("whitespace-only subdirectory must not create directory markup: main=%s modals=%s", mainHTML, projection.Modals)
	}
}

func TestApplicationProjectionOmitsPrivateOnlyDirectoryForAnonymous(t *testing.T) {
	projection := generateApplicationProjectionForItems(t, []model.Bookmark{
		{Name: "Visible App", URL: "https://visible.example", Subdir: "Visible"},
		{Name: "Secret App", URL: "https://secret.example", Subdir: "Secret", Private: true},
	}, "", false)

	mainHTML := string(projection.HTML)
	modalHTML := string(projection.Modals)
	if !projection.HasDirectories || !strings.Contains(mainHTML, `data-application-subdirectory="Visible"`) {
		t.Fatalf("expected visible directory trigger: %s", mainHTML)
	}
	for _, html := range []string{mainHTML, modalHTML} {
		if strings.Contains(html, "Secret") {
			t.Fatalf("private-only directory leaked into anonymous projection: %s", html)
		}
	}
	if strings.Count(modalHTML, `class="application-subdirectory-modal"`) != 1 {
		t.Fatalf("expected only the visible directory modal: %s", modalHTML)
	}
}

func TestApplicationProjectionDirectorySearchIncludesAllVisibleItems(t *testing.T) {
	projection := generateApplicationProjectionForItems(t, []model.Bookmark{
		{Name: "First Tool", URL: "https://first.example", Subdir: "Operations"},
		{Name: "Second Tool", URL: "https://second.example", Subdir: "Operations"},
		{Name: "Private Tool", URL: "https://private.example", Subdir: "Operations", Private: true},
		{Name: "Unrelated", URL: "https://unrelated.example"},
	}, "operations", false)

	modalHTML := string(projection.Modals)
	for _, name := range []string{"First Tool", "Second Tool"} {
		if !strings.Contains(modalHTML, `title="`+name+`"`) {
			t.Fatalf("directory search must include visible item %q: %s", name, modalHTML)
		}
	}
	for _, name := range []string{"Private Tool", "Unrelated"} {
		if strings.Contains(modalHTML, name) {
			t.Fatalf("directory search included unavailable item %q: %s", name, modalHTML)
		}
	}
	if len(projection.items) != 2 {
		t.Fatalf("expected two visible directory matches, got %#v", projection.items)
	}
}

func TestApplicationProjectionItemSearchNarrowsDirectoryModal(t *testing.T) {
	projection := generateApplicationProjectionForItems(t, []model.Bookmark{
		{Name: "Needle Tool", URL: "https://needle.example", Subdir: "Utilities"},
		{Name: "Haystack Tool", URL: "https://haystack.example", Subdir: "Utilities"},
	}, "needle", true)

	modalHTML := string(projection.Modals)
	if !strings.Contains(modalHTML, `title="Needle Tool"`) || strings.Contains(modalHTML, "Haystack Tool") {
		t.Fatalf("application search must narrow directory modal items: %s", modalHTML)
	}
	if len(projection.items) != 1 || projection.items[0].Name != "Needle Tool" {
		t.Fatalf("unexpected filtered diagnostic items: %#v", projection.items)
	}
}

func TestApplicationProjectionEscapesDirectoryNamesAndUsesGeneratedIDs(t *testing.T) {
	folderName := `Team <Ops> & "Admin"`
	projection := generateApplicationProjectionForItems(t, []model.Bookmark{{
		Name: "Sensitive", URL: "https://sensitive.example", Subdir: folderName,
	}}, "", true)

	mainHTML := string(projection.HTML)
	modalHTML := string(projection.Modals)
	escapedName := template.HTMLEscapeString(folderName)
	if !strings.Contains(mainHTML, `data-application-subdirectory="`+escapedName+`"`) {
		t.Fatalf("expected escaped folder data attribute: %s", mainHTML)
	}
	for _, expected := range []string{
		`href="#application-subdir-modal-0"`,
		`id="application-subdir-modal-0"`,
		`id="application-subdir-title-0"`,
		`aria-labelledby="application-subdir-title-0"`,
		`class="application-subdirectory-backdrop" aria-label="Close" tabindex="-1" aria-hidden="true"`,
		`class="application-subdirectory-panel" tabindex="-1" role="dialog"`,
	} {
		if !strings.Contains(mainHTML+modalHTML, expected) {
			t.Fatalf("expected generated modal reference %q: main=%s modals=%s", expected, mainHTML, modalHTML)
		}
	}
	if strings.Contains(mainHTML+modalHTML, folderName) || strings.Contains(mainHTML+modalHTML, `id="Team`) || strings.Contains(mainHTML+modalHTML, `href="#Team`) {
		t.Fatalf("raw directory name must be escaped and must not become an ID: main=%s modals=%s", mainHTML, modalHTML)
	}
}

func TestApplicationProjectionItemsRetainDirectoryAndUngroupedApps(t *testing.T) {
	projection := generateApplicationProjectionForItems(t, []model.Bookmark{
		{Name: "Grouped", URL: "https://grouped.example", Subdir: " Folder "},
		{Name: "Plain", URL: "https://plain.example"},
		{Name: "Private", URL: "https://private.example", Subdir: "Folder", Private: true},
	}, "", false)

	if len(projection.items) != 2 {
		t.Fatalf("expected grouped and ungrouped visible diagnostics, got %#v", projection.items)
	}
	if projection.items[0].Name != "Grouped" || projection.items[0].Subdir != " Folder " || projection.items[1].Name != "Plain" {
		t.Fatalf("diagnostic items must retain visible filtered source order: %#v", projection.items)
	}
}

func TestBookmarkStylesDynamicSelectorsUseSharedModuleClass(t *testing.T) {
	style := string(customHomeStyle(model.Application{
		BookmarkCategoryColor: "#123456",
		BookmarkItemColor:     "#abcdef",
	}, background.Assets{}))
	if !strings.Contains(style, `.bookmark-module .bookmark-group-container h3.bookmark-group-title`) {
		t.Fatalf("expected shared bookmark module selector for category color, got %s", style)
	}
	if !strings.Contains(style, `.bookmark-module .bookmark-group-container .bookmark-list a.bookmark`) {
		t.Fatalf("expected shared bookmark module selector for item color, got %s", style)
	}
	if strings.Contains(style, `#container-bookmakrs`) {
		t.Fatalf("dynamic bookmark colors must not target only the legacy bookmark id: %s", style)
	}

	var b strings.Builder
	appendAdaptiveColumnStyle(&b, 4)
	adaptive := b.String()
	if !strings.Contains(adaptive, `.bookmark-module .bookmark-groups`) || !strings.Contains(adaptive, `.bookmark-module .bookmark-group-container`) {
		t.Fatalf("adaptive bookmark columns must target the shared module class: %s", adaptive)
	}
	if strings.Contains(adaptive, `#container-bookmakrs`) {
		t.Fatalf("adaptive bookmark columns must not target only the legacy bookmark id: %s", adaptive)
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
