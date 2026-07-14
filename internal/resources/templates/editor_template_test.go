package templates

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEditorTemplateControlsAreNotBroken(t *testing.T) {
	raw, err := TPL.ReadFile("html/editor.html")
	if err != nil {
		t.Fatalf("read editor template: %v", err)
	}
	page := string(raw)

	for _, expected := range []string{
		`id="check-links"`,
		`id="editor-theme-toggle"`,
		`id="editor-operation-notice"`,
		`id="editor-operation-notice-text"`,
		`id="editor-operation-notice-close"`,
		`id="editor-backup-download"`,
		`id="editor-restore-submit"`,
		`runtime.js{{.DebugAssetVersion}}`,
		`editor-page`,
		`editor-render-warning`,
		`editor-backup-file`,
		`SiteIconURL .OptionSiteIcon`,
		`FLARE_FIX_CATEGORY = ["[SuperFlare \u5e94\u7528]"]`,
		`\u4e66\u7b7e\u540d\u79f0`,
		`\u5185\u7f51\u5730\u5740`,
		`\u5b50\u76ee\u5f55`,
		`type: 'autocomplete'`,
		`remove_row`,
		`local-url-empty`,
		`editor-validation-error`,
		`--editor-table-surface`,
		`superflare-editor-theme`,
		`body{--editor-page-background:#14181d;`,
		`data-editor-theme`,
		`function parseEditorJSONFailure(response, fallbackMessage)`,
		`function summarizeEditorErrorMessage(message, fallbackMessage)`,
		`function showEditorOperationNotice(type, message, autoHide)`,
		`function clearEditorValidationState(renderTables)`,
		`function isDeletedBookmarkRow(item)`,
		`function getExportableBookmarkRows()`,
		`function focusEditorValidationCell(target)`,
		`fetch(form.action || '/editor'`,
		`行分类缺少名称`,
		`行书签缺少`,
		`function stripEditorNoticeQuery()`,
		`function parseDownloadFilename(disposition)`,
		`function replaceEditorDocumentWithHTML(html)`,
		`btn.title = detail;`,
		`setButtonState(btn, 'editor-button-state-error', '\u68c0\u67e5\u5931\u8d25');`,
		`backupSuccess`,
		`restoreMissingFile`,
		`saveRunning`,
		`--editor-table-input-text`,
		`--editor-table-accent-text`,
		`.handsontable.listbox`,
		`.htMenu.htContextMenu`,
		`<body>`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("editor template missing %q", expected)
		}
	}

	for _, broken := range []string{
		"? id=",
		"SuperFlare \ufffd",
		`style="{{.PageAppearance}}"`,
		`Handsontable.renderers.TextRenderer.apply(this, arguments);return;if`,
		`setButtonState(btn, 'editor-button-state-error', detail);`,
		`document.getElementById('form-categories').submit()`,
		`window.alert(`,
	} {
		if strings.Contains(page, broken) {
			t.Fatalf("editor template contains broken marker %q", broken)
		}
	}
}

func TestEditorTemplateSupportsPrivateAndNormalBookmarkFavorites(t *testing.T) {
	raw, err := TPL.ReadFile("html/editor.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)
	for _, expected := range []string{
		`{ data: 'Private', type: 'checkbox'`,
		`{ data: 'Favorite', type: 'checkbox'`,
		`item.Private ? 'true' : 'false'`,
		`item.Favorite && item.Category !== FLARE_FIX_CATEGORY[0] ? 'true' : 'false'`,
		`prop === 'Favorite'`,
		`this.instance.getSourceDataAtRow(row)`,
		`FLARE_FIX_CATEGORY[0]`,
		`setDataAtRowProp(row, 'Favorite', false`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("editor template missing %q", expected)
		}
	}
}

func TestEditorTemplateReappliesApplicationFavoriteInvariantAfterUndo(t *testing.T) {
	raw, err := TPL.ReadFile("html/editor.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)

	for _, expected := range []string{
		`function enforceApplicationFavoriteInvariant(row) {`,
		`instanceBookmarks.getDataAtRowProp(row, 'Category') === FLARE_FIX_CATEGORY[0]`,
		`instanceBookmarks.getDataAtRowProp(row, 'Favorite') === true`,
		`instanceBookmarks.setDataAtRowProp(row, 'Favorite', false, 'application-favorite-invariant');`,
		`if (source !== 'application-favorite-invariant') {`,
		`enforceApplicationFavoriteInvariant(row);`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("editor template missing undo-safe favorite invariant %q", expected)
		}
	}
}

func TestEditorTemplateAppliesLinkCheckResultsInBatch(t *testing.T) {
	raw, err := TPL.ReadFile("html/editor.html")
	if err != nil {
		t.Fatalf("read editor template: %v", err)
	}
	page := string(raw)

	for _, expected := range []string{
		`function applyLinkCheckResults(results) {`,
		`const nextRows = getVisualBookmarkRows();`,
		`bookmarks = nextRows;`,
		`instanceBookmarks.updateData(bookmarks);`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("editor template missing %q", expected)
		}
	}

	for _, broken := range []string{
		`instanceBookmarks.setDataAtRowProp(visualRow, 'CheckResult', checkResult, 'link-check-result');`,
		`instanceBookmarks.setDataAtRowProp(visualRow, 'CheckStatus', result.status || 'invalid', 'link-check-result');`,
	} {
		if strings.Contains(page, broken) {
			t.Fatalf("editor template still uses fragile row-by-row link-check writes: %q", broken)
		}
	}
}

func TestGeneratedEditorTemplateMatchesSourceTemplate(t *testing.T) {
	src, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "templates", "editor.html")))
	if err != nil {
		t.Fatalf("read source editor template: %v", err)
	}
	gen, err := ReadEmbeddedTemplate("editor.html")
	if err != nil {
		t.Fatalf("read generated editor template: %v", err)
	}
	minifiedSource, err := MinifyTemplateBytes(src)
	if err != nil {
		t.Fatalf("minify source editor template: %v", err)
	}
	if !bytes.Equal(bytes.ReplaceAll(minifiedSource, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(gen, []byte("\r\n"), []byte("\n"))) {
		t.Fatal("generated editor template is out of sync with embed/templates/editor.html")
	}
}

func TestGeneratedSettingsTemplateMatchesSourceTemplate(t *testing.T) {
	for _, name := range []string{"settings.html", "settings-appearance.html", "settings-theme.html", "settings-others.html", "settings-sidebar.html"} {
		src, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "templates", name)))
		if err != nil {
			t.Fatalf("read source template %s: %v", name, err)
		}
		gen, err := ReadEmbeddedTemplate(name)
		if err != nil {
			t.Fatalf("read generated template %s: %v", name, err)
		}
		minifiedSource, err := MinifyTemplateBytes(src)
		if err != nil {
			t.Fatalf("minify source template %s: %v", name, err)
		}
		if !bytes.Equal(bytes.ReplaceAll(minifiedSource, []byte("\r\n"), []byte("\n")), bytes.ReplaceAll(gen, []byte("\r\n"), []byte("\n"))) {
			t.Fatalf("generated template %s is out of sync with embed/templates/%s", name, name)
		}
	}
}

func TestSettingsAppearanceFavoritesControlsAreBetweenApplicationsAndBookmarks(t *testing.T) {
	raw, err := TPL.ReadFile("html/settings-appearance.html")
	if err != nil {
		t.Fatalf("read settings appearance template: %v", err)
	}
	page := string(raw)

	apps := strings.Index(page, `id="settings-apps-title"`)
	favorites := strings.Index(page, `id="settings-show-favorites"`)
	favoritesTitle := strings.Index(page, `id="settings-favorites-title"`)
	bookmarks := strings.Index(page, `id="settings-show-bookmarks"`)
	if apps == -1 || favorites == -1 || favoritesTitle == -1 || bookmarks == -1 {
		t.Fatalf("favorites appearance controls are incomplete: apps=%d favorites=%d favoritesTitle=%d bookmarks=%d", apps, favorites, favoritesTitle, bookmarks)
	}
	if !(apps < favorites && favorites < favoritesTitle && favoritesTitle < bookmarks) {
		t.Fatal("favorites settings are out of order")
	}
}

func TestSettingsOthersDefaultCredentialWarningIsInLoginSection(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "templates", "settings-others.html")))
	if err != nil {
		t.Fatalf("read source settings-others template: %v", err)
	}
	page := string(raw)

	loggedInLoginForm := strings.Index(page, `<form method="POST" action="{{.LogoutURI}}">`)
	warning := strings.Index(page, `default_login_credentials_warning`)
	loginConfigForm := strings.Index(page, `<form method="POST" action="{{ .SettingPages.Others.Path }}">`)
	if loggedInLoginForm == -1 {
		t.Fatal("settings-others template missing logged-in login form")
	}
	if warning == -1 {
		t.Fatal("settings-others template missing default credential warning")
	}
	if loginConfigForm == -1 {
		t.Fatal("settings-others template missing login config form")
	}
	if warning < loggedInLoginForm || warning > loginConfigForm {
		t.Fatalf("default credential warning should render in the Login section before Login Configuration; positions login=%d warning=%d loginConfig=%d", loggedInLoginForm, warning, loginConfigForm)
	}
}

func TestSettingsTemplateIncludesColorPickerHooks(t *testing.T) {
	shared, err := TPL.ReadFile("html/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	sharedPage := string(shared)
	for _, expected := range []string{
		`settings-color-trigger`,
		`settings-color-panel`,
		`settings-color-input-row`,
		`runtime.js{{.DebugAssetVersion}}`,
	} {
		if !strings.Contains(sharedPage, expected) {
			t.Fatalf("settings template missing %q", expected)
		}
	}

	for _, name := range []string{"html/settings-theme.html", "html/settings-appearance.html"} {
		raw, err := TPL.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		page := string(raw)
		if !strings.Contains(page, `data-color-picker="true"`) {
			t.Fatalf("%s missing color picker hook", name)
		}
	}

	searchRaw, err := TPL.ReadFile("html/settings-search.html")
	if err != nil {
		t.Fatalf("read html/settings-search.html: %v", err)
	}
	searchPage := string(searchRaw)
	for _, expected := range []string{
		`search-mode`,
		`search-engine`,
		`search-engine-open-mode`,
		`search-engine-custom-template`,
	} {
		if !strings.Contains(searchPage, expected) {
			t.Fatalf("settings search template missing %q", expected)
		}
	}
}

func TestSettingsColorPickerUsesAccentDrivenBorders(t *testing.T) {
	raw, err := TPL.ReadFile("html/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	page := strings.NewReplacer("\n", "", "\r", "", "\t", "", " ", "").Replace(string(raw))

	for _, expected := range []string{
		`border:1pxsolidcolor-mix(insrgb,var(--color-accent)76%,rgba(255,255,255,.18));`,
		`box-shadow:inset0001pxcolor-mix(insrgb,var(--color-accent)22%,transparent)`,
		`border:1pxsolidcolor-mix(insrgb,var(--color-accent)70%,rgba(255,255,255,.18));`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("settings color picker missing accent border style %q", expected)
		}
	}
}

func TestSettingsTemplatesSeparateRawAndRenderedFooterFields(t *testing.T) {
	settingsRaw, err := TPL.ReadFile("html/settings.html")
	if err != nil {
		t.Fatalf("read settings template: %v", err)
	}
	settingsPage := string(settingsRaw)
	if !strings.Contains(settingsPage, `{{.RenderedFooter}}`) {
		t.Fatalf("expected settings template to use sanitized rendered footer, got %s", settingsPage)
	}
	if strings.Contains(settingsPage, `<div class="footer-container">{{.OptionFooter}}`) || strings.Contains(settingsPage, `{{.OptionFooter}}</div>`) {
		t.Fatalf("expected settings footer display to stop using raw footer field, got %s", settingsPage)
	}

	appearanceRaw, err := TPL.ReadFile("html/settings-appearance.html")
	if err != nil {
		t.Fatalf("read settings appearance template: %v", err)
	}
	appearancePage := string(appearanceRaw)
	if !strings.Contains(appearancePage, `>{{.OptionFooter}}</textarea>`) {
		t.Fatalf("expected settings appearance textarea to keep raw footer text, got %s", appearancePage)
	}
}

func TestSettingsLayoutKeepsFooterAtViewportBottomWhenContentIsShort(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "assets", "css", "settings", "layout.css")))
	if err != nil {
		t.Fatalf("read settings layout css: %v", err)
	}
	css := string(raw)
	for _, expected := range []string{
		`#page-settings {`,
		`display: flex;`,
		`flex-direction: column;`,
		`min-height: 100vh;`,
		`#page-settings .container {`,
		`min-height: 100%;`,
		`display: flex;`,
		`flex-direction: column;`,
		`#page-settings .footer-container {`,
		`margin-top: auto;`,
	} {
		if !strings.Contains(css, expected) {
			t.Fatalf("expected settings sticky footer css fragment %q, got %s", expected, css)
		}
	}
}

func TestToolbarStylesDoNotBreakFollowingSettingsRules(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "assets", "css", "home", "toolbar.css")))
	if err != nil {
		t.Fatalf("read toolbar css: %v", err)
	}
	css := string(raw)
	if strings.Contains(css, ".toolbar-btn-settings{\n  margin: 0 0 10px 0;\n}\n}") {
		t.Fatalf("toolbar css still contains trailing unmatched brace: %s", css)
	}
	if strings.Count(css, "{") != strings.Count(css, "}") {
		t.Fatalf("toolbar css brace count mismatch: %s", css)
	}
}
