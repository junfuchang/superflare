package templates

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/junfuchang/superflare/config/define"
)

func TestHomeTemplateUsesWarningsModalInsteadOfStandalonePage(t *testing.T) {
	raw, err := TPL.ReadFile("html/home.html")
	if err != nil {
		t.Fatalf("read home template: %v", err)
	}
	page := string(raw)

	for _, expected := range []string{
		`id="btn-open-warnings"`,
		`page-warnings-modal`,
		`id="page-warnings-modal"`,
		`class="page-warnings-modal"`,
		`class="page-warnings-backdrop"`,
		`class="page-warnings-panel"`,
		`{{ T .Locale "warnings_title" }}`,
		`{{ T .Locale "warnings_empty" }}`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("home template missing %q", expected)
		}
	}

	for _, broken := range []string{
		`<dialog id="page-warnings-dialog"`,
		`onclick="var dialog=document.getElementById('page-warnings-dialog')`,
		`onclick="document.getElementById('page-warnings-dialog').close()"`,
		`page-warnings-dialog`,
	} {
		if strings.Contains(page, broken) {
			t.Fatalf("home template still contains deprecated warning dialog markup %q", broken)
		}
	}
}

func TestHomeTemplateUsesAppsSurfaceAndBodyLevelApplicationSubdirectoryModals(t *testing.T) {
	raw, err := TPL.ReadFile("html/home.html")
	if err != nil {
		t.Fatalf("read home template: %v", err)
	}
	page := string(raw)
	for _, expected := range []string{
		`class="apps-container clearfix apps-surface"`,
		`{{.ApplicationSubdirectoryModals}}`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("home template missing %q", expected)
		}
	}

	pageHomeStart := strings.Index(page, `<div class="pageview" id="page-home"`)
	appsStart := strings.Index(page, `id="container-apps"`)
	modalCondition := strings.Index(page, `{{ if .HasApplicationSubdirectories }}`)
	modalPlaceholder := strings.Index(page, `{{.ApplicationSubdirectoryModals}}`)
	warningsModal := strings.Index(page, `id="page-warnings-modal"`)
	if pageHomeStart == -1 || appsStart == -1 || modalCondition == -1 || modalPlaceholder == -1 || warningsModal == -1 {
		t.Fatalf("expected page, apps, application modal, and warnings markers, got page=%d apps=%d condition=%d modal=%d warnings=%d", pageHomeStart, appsStart, modalCondition, modalPlaceholder, warningsModal)
	}
	appsEndOffset := strings.Index(page[appsStart:], `{{ end }}`)
	if appsEndOffset == -1 {
		t.Fatal("expected applications module conditional to close")
	}
	appsEnd := appsStart + appsEndOffset
	pageHomeEnd := strings.LastIndex(page[:modalCondition], `</div>`)
	if pageHomeEnd == -1 {
		t.Fatal("expected page-home content to close before application modals")
	}
	pageHome := page[pageHomeStart : pageHomeEnd+len(`</div>`)]
	if opens, closes := strings.Count(pageHome, `<div`), strings.Count(pageHome, `</div>`); opens != closes {
		t.Fatalf("application modal placeholder occurs before page-home closes: div opens=%d closes=%d", opens, closes)
	}
	if !(pageHomeStart < appsStart && appsEnd < pageHomeEnd && pageHomeEnd < modalCondition && modalCondition < modalPlaceholder && modalPlaceholder < warningsModal) {
		t.Fatalf("application modals must be body-level siblings after page-home and before warnings: page=%d apps=%d appsEnd=%d pageEnd=%d condition=%d modal=%d warnings=%d", pageHomeStart, appsStart, appsEnd, pageHomeEnd, modalCondition, modalPlaceholder, warningsModal)
	}
}

func TestHomeTemplatePlacesWarningsButtonAboveSettingsWithoutVisibleText(t *testing.T) {
	raw, err := TPL.ReadFile("html/home.html")
	if err != nil {
		t.Fatalf("read home template: %v", err)
	}
	page := string(raw)
	toolbarStart := strings.Index(page, `<div class="toolbar-container">`)
	if toolbarStart == -1 {
		t.Fatal("expected toolbar container in home template")
	}
	footerStart := strings.Index(page[toolbarStart:], `<div class="footer-container">`)
	if footerStart == -1 {
		t.Fatal("expected footer after toolbar container in home template")
	}
	toolbar := page[toolbarStart : toolbarStart+footerStart]

	warningsIndex := strings.Index(toolbar, `id="btn-open-warnings"`)
	settingsIndex := strings.Index(toolbar, `id="btn-open-settings"`)
	if warningsIndex == -1 || settingsIndex == -1 {
		t.Fatalf("expected both warnings and settings buttons, got warnings=%d settings=%d", warningsIndex, settingsIndex)
	}
	if warningsIndex > settingsIndex {
		t.Fatalf("expected warnings button to render above settings button")
	}
	if strings.Contains(toolbar, `<span>{{ T .Locale "toolbar_warnings" }}</span>`) {
		t.Fatal("expected warnings button text node to be removed from toolbar entry")
	}
	if !strings.Contains(toolbar, `aria-label="{{ T .Locale "toolbar_warnings" }}"`) {
		t.Fatal("expected warnings button aria-label to preserve accessibility text")
	}
}

func TestGeneratedHomeTemplateMatchesSourceTemplate(t *testing.T) {
	src, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "templates", "home.html")))
	if err != nil {
		t.Fatalf("read source home template: %v", err)
	}
	gen, err := ReadEmbeddedTemplate("home.html")
	if err != nil {
		t.Fatalf("read generated home template: %v", err)
	}
	minifiedSource, err := MinifyTemplateBytes(src)
	if err != nil {
		t.Fatalf("minify source home template: %v", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(minifiedSource, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(gen, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated home template is out of sync with embed/templates/home.html")
	}
}

func TestHomeTemplateUsesRenderedFooterField(t *testing.T) {
	raw, err := TPL.ReadFile("html/home.html")
	if err != nil {
		t.Fatalf("read home template: %v", err)
	}
	page := string(raw)
	if !strings.Contains(page, `{{.RenderedFooter}}`) {
		t.Fatalf("expected home template to use sanitized rendered footer, got %s", page)
	}
	if strings.Contains(page, `<div class="footer-container">{{ if .OptionFooter }}`) || strings.Contains(page, `{{.OptionFooter}}</div>`) {
		t.Fatalf("expected home template footer display to stop using raw footer field, got %s", page)
	}
}

func TestBaseStylesKeepFooterPinnedToViewportWhenContentIsShort(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "assets", "css", "base.css")))
	if err != nil {
		t.Fatalf("read base.css: %v", err)
	}
	css := string(raw)
	for _, expected := range []string{
		`.pageview {`,
		`min-height: 100vh;`,
		`#page-home {`,
		`display: flex;`,
		`flex-direction: column;`,
		`#page-home.pageview .container {`,
		`min-height: 100%;`,
		`display: flex;`,
		`flex-direction: column;`,
		`#page-home .footer-container {`,
		`margin-top: auto;`,
	} {
		if !strings.Contains(css, expected) {
			t.Fatalf("expected sticky footer css fragment %q, got %s", expected, css)
		}
	}
}

func TestHomeTemplateUsesSearchHintFields(t *testing.T) {
	raw, err := TPL.ReadFile("html/home.html")
	if err != nil {
		t.Fatalf("read home template: %v", err)
	}
	page := string(raw)
	for _, expected := range []string{
		`placeholder="{{.SearchKeyword}}"`,
		`{{.SearchHintLabel}}`,
		`{{ if .SearchFormTarget }} target="{{.SearchFormTarget}}"{{ end }}`,
		`{{ if .SearchFormRel }} rel="{{.SearchFormRel}}"{{ end }}`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("expected home template to include %q", expected)
		}
	}
	if strings.Contains(page, `{{ T .Locale "search_label" }}`) {
		t.Fatalf("expected home template search label to use dynamic hint field, got %s", page)
	}
}

func TestHomeTemplateLoadsSiteIconRefreshScriptWithNonce(t *testing.T) {
	raw, err := TPL.ReadFile("html/home.html")
	if err != nil {
		t.Fatalf("read home template: %v", err)
	}
	page := string(raw)
	if !strings.Contains(page, `<script nonce="{{.ScriptNonce}}">{{.InlineSiteIconRefreshScript}}</script>`) {
		t.Fatalf("expected site icon refresh script to use page nonce, got %s", page)
	}
	if strings.Contains(page, `{{.CustomHomeStyle}}<script`) {
		t.Fatalf("expected site icon refresh script to stay outside custom style block, got %s", page)
	}
}

func TestHomeTemplateFavoritesModuleOrderAndTooltipNonce(t *testing.T) {
	raw, err := TPL.ReadFile("html/home.html")
	if err != nil {
		t.Fatalf("read home template: %v", err)
	}
	page := string(raw)
	appsIndex := strings.Index(page, `id="container-apps"`)
	favoritesIndex := strings.Index(page, `id="container-favorites"`)
	bookmarksIndex := strings.Index(page, `id="container-bookmakrs"`)
	if appsIndex == -1 || favoritesIndex == -1 || bookmarksIndex == -1 {
		t.Fatalf("expected apps, favorites, and bookmarks modules, got apps=%d favorites=%d bookmarks=%d", appsIndex, favoritesIndex, bookmarksIndex)
	}
	if !(appsIndex < favoritesIndex && favoritesIndex < bookmarksIndex) {
		t.Fatalf("expected favorites between apps and bookmarks, got apps=%d favorites=%d bookmarks=%d", appsIndex, favoritesIndex, bookmarksIndex)
	}
	for _, expected := range []string{
		`{{ if .OptionShowFavorites }}`,
		`class="plugin-container clearfix bookmark-module" id="container-favorites"`,
		`<h2>{{.FavoritesTitle}}</h2>`,
		`{{.Favorites}}`,
		`class="plugin-container clearfix bookmark-module" id="container-bookmakrs"`,
		`{{ if .InlineBookmarkTooltipScript }}`,
		`<script nonce="{{.ScriptNonce}}">{{.InlineBookmarkTooltipScript}}</script>`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("home template missing %q", expected)
		}
	}
	if got := strings.Count(page, `{{.InlineBookmarkTooltipScript}}`); got != 1 {
		t.Fatalf("expected one delegated bookmark tooltip script, got %d", got)
	}
	favoritesStart := strings.Index(page, `id="container-favorites"`)
	favoritesEnd := strings.Index(page[favoritesStart:], `{{ end }}`)
	if favoritesEnd == -1 {
		t.Fatal("expected favorites module conditional to close")
	}
	if strings.Contains(page[favoritesStart:favoritesStart+favoritesEnd], `<a href=`) {
		t.Fatal("favorites module title must not link to a new subpage")
	}
}

func TestBookmarkStylesUseSharedModuleAndTooltip(t *testing.T) {
	baseRaw, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "assets", "css", "base.css")))
	if err != nil {
		t.Fatalf("read base.css: %v", err)
	}
	bookmarkRaw, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "assets", "css", "home", "bookmarks.css")))
	if err != nil {
		t.Fatalf("read bookmarks.css: %v", err)
	}
	baseCSS := string(baseRaw)
	bookmarkCSS := string(bookmarkRaw)
	if !strings.Contains(baseCSS, `.bookmark-module`) {
		t.Fatalf("base module spacing must use shared bookmark class: %s", baseCSS)
	}
	for _, expected := range []string{
		`.bookmark-module .bookmark-group-container`,
		`.bookmark-description-tooltip`,
		`position: fixed;`,
		`z-index:`,
		`max-width:`,
		`overflow-wrap:`,
		`pointer-events: none;`,
	} {
		if !strings.Contains(bookmarkCSS, expected) {
			t.Fatalf("bookmark styles missing %q: %s", expected, bookmarkCSS)
		}
	}
	if strings.Contains(bookmarkCSS, `#container-bookmakrs`) {
		t.Fatalf("bookmark styles must use the shared module class, got %s", bookmarkCSS)
	}
}

func TestBookmarkTooltipStylesConstrainLongDescriptionsToViewport(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "assets", "css", "home", "bookmarks.css")))
	if err != nil {
		t.Fatalf("read bookmarks.css: %v", err)
	}
	css := string(raw)
	for _, expected := range []string{
		`max-height: calc(100vh - 16px);`,
		`overflow-y: auto;`,
	} {
		if !strings.Contains(css, expected) {
			t.Fatalf("bookmark tooltip source styles missing %q: %s", expected, css)
		}
	}

	generated := define.PAGE_INLINE_STYLE
	for _, expected := range []string{
		`max-height: calc(100vh - 16px)`,
		`overflow-y: auto`,
	} {
		if !strings.Contains(generated, expected) {
			t.Fatalf("generated inline styles missing %q", expected)
		}
	}
}
