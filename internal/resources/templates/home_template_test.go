package templates

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
