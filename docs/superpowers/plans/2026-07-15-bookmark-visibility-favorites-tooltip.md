# Bookmark Visibility, Favorites, and Tooltip Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add delayed bookmark-description tooltips, signed-out visibility controls for applications and bookmarks, and a configurable favorites module containing only normal bookmarks while preserving all existing configuration files.

**Architecture:** Extend the existing YAML models with optional item flags and presence-compatible application settings, then keep the editor's 10-column CSV as the single POST boundary. Home rendering resolves one request trust state, filters both source collections before search/rendering, and builds normal-bookmark and flat favorites projections from one `bookmarks.yml` load. The home template renders favorites between applications and bookmarks and enables one nonce-protected delegated tooltip script only when rendered bookmark markup contains a non-empty description.

**Tech Stack:** Go 1.24, Echo v5, `gopkg.in/yaml.v2`, Go `html/template`, Handsontable JavaScript, embedded HTML/CSS assets, JSON i18n catalogs.

## Global Constraints

- `Private` applies to applications and normal bookmarks; it hides an item only when login is enabled and the request is not authenticated.
- Disabled login mode is trusted access, so private items remain visible.
- `Favorite` applies only to normal bookmarks; application controls are disabled and server-side saves force application `Favorite` to `false`.
- Favorites remain in the normal bookmarks module, are category-free, and sort by normalized displayed name ascending.
- The favorites module appears between applications and bookmarks only when enabled and non-empty after visibility and search filtering.
- Favorites reuse `OpenBookmarkNewTab`, `IconMode`, `BookmarkItemColor`, `HomeMaxColumns`, and `HomeMaxWidth`.
- Missing `Private` and `Favorite` fields default to `false`; missing `ShowFavorites` defaults to `true`, while explicit `ShowFavorites: false` remains false.
- The editor continues accepting legacy 6-, 7-, and 8-field rows and adds a strict 10-field format ending in `Private,Favorite`.
- No new configuration or cache file is introduced; backup/restore file membership stays unchanged.
- Work remains on `main`, is committed locally, and is not pushed.

---

### Task 1: Add compatible model and application defaults

**Files:**
- Modify: `config/model/bookmark.go`
- Modify: `config/model/application.go`
- Modify: `config/defaults/config.yml`
- Modify: `config/data/config.go`
- Test: `config/data/config_test.go`

**Interfaces:**
- Produces: `model.Bookmark.Favorite bool`, `model.Application.ShowFavorites bool`, and `model.Application.FavoritesTitle string`.
- Produces: both file and raw application-config loaders return `ShowFavorites=true` when the YAML key is absent and preserve an explicit false value.

- [ ] **Step 1: Write failing compatibility tests**

```go
func TestLoadAppConfigFromRawDefaultsMissingShowFavoritesToTrue(t *testing.T) {
	options, err := LoadAppConfigFromRaw([]byte("Title: SuperFlare\nLocale: zh\nTheme: onedark\n"))
	require.NoError(t, err)
	assert.True(t, options.ShowFavorites)
}

func TestLoadAppConfigFromRawPreservesExplicitShowFavoritesFalse(t *testing.T) {
	options, err := LoadAppConfigFromRaw([]byte("Title: SuperFlare\nLocale: zh\nTheme: onedark\nShowFavorites: false\n"))
	require.NoError(t, err)
	assert.False(t, options.ShowFavorites)
}
```

Extend `TestLoadAppConfigFromYamlDefaultValues` to assert `ShowFavorites` is true.

- [ ] **Step 2: Run the tests and confirm RED**

Run: `go test ./config/data -run "TestLoadAppConfigFromRaw(Default|Preserves)|TestLoadAppConfigFromYamlDefaultValues" -count=1`

Expected: FAIL because `ShowFavorites` does not exist or decodes to false.

- [ ] **Step 3: Implement the model and presence-compatible decoding**

```go
// config/model/bookmark.go
Private  bool `yaml:"private,omitempty"`
Favorite bool `yaml:"favorite,omitempty"`

// config/model/application.go
ShowApps       bool   `yaml:"ShowApps"`
ShowFavorites  bool   `yaml:"ShowFavorites"`
ShowBookmarks  bool   `yaml:"ShowBookmarks"`
AppsTitle      string `yaml:"AppsTitle,omitempty"`
FavoritesTitle string `yaml:"FavoritesTitle,omitempty"`
BookmarksTitle string `yaml:"BookmarksTitle,omitempty"`
```

Initialize both decode targets before unmarshalling:

```go
result := model.Application{ShowFavorites: true}
if err := yaml.Unmarshal(raw, &result); err != nil {
	return result, err
}
```

Add `ShowFavorites: true` between `ShowApps` and `ShowBookmarks` in `config/defaults/config.yml`.

- [ ] **Step 4: Run focused and package tests**

Run: `go test ./config/data ./config/model -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add config/model/bookmark.go config/model/application.go config/defaults/config.yml config/data/config.go config/data/config_test.go
git commit -m "feat: add bookmark visibility and favorite settings"
```

---

### Task 2: Preserve flags through the editor backend and CSV boundary

**Files:**
- Modify: `config/data/editor.go`
- Test: `config/data/editor_test.go`

**Interfaces:**
- Consumes: `model.Bookmark.Private` and `model.Bookmark.Favorite` from Task 1.
- Produces: editor JSON containing both booleans and strict parsing of 10-field rows.
- Produces: application rows always have `Favorite=false` before `apps.yml` is staged.

- [ ] **Step 1: Write failing editor backend tests**

Add tests that cover all of these concrete rows:

```go
const tenFieldRows = "1,App,https://app.example.com,,[SuperFlare 应用],,home,App,true,true\n" +
	"2,Bookmark,https://bookmark.example.com,,Links,,link,Bookmark,true,true"
```

Assert the application result has `Private=true, Favorite=false`, the normal result has `Private=true, Favorite=true`, and invalid values such as `yes` return an error containing the row number and field name. Add a fixture with YAML `private: true` and `favorite: true`, call `GetBookmarksForEditor`, and assert the returned JSON contains `"Private":true` and `"Favorite":true`. Retain explicit tests for 6-, 7-, and 8-field rows defaulting both flags to false.

- [ ] **Step 2: Run the tests and confirm RED**

Run: `go test ./config/data -run "Test(GetBookmarksForEditorPreservesFlags|GetBookmarksFromCSVParsesVisibilityAndFavorite|GetBookmarksFromCSVRejectsInvalidBoolean|GetBookmarksFromCSVLegacy)" -count=1`

Expected: FAIL because flags are removed and 10 fields are rejected.

- [ ] **Step 3: Replace the lossy editor projection and add boolean parsing**

Delete `_BOOKMARK_REMOVE_PRIVATE`, `removePrivateProp`, and `restorePrivateProp`. Marshal `mixedBookmarks` directly in `GetBookmarksForEditor`.

Add a strict parser:

```go
func parseEditorBool(value string, row int, field string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "false":
		return false, nil
	case "true":
		return true, nil
	default:
		return false, fmt.Errorf("bookmark row %d has invalid %s value %q", row, field, value)
	}
}
```

In `parseBookmarkCSVRecord`, keep the 6/7/8 cases and add a 10-field case using the existing 8-field positions plus:

```go
bookmark.Private, err = parseEditorBool(csvRecordValue(record, 8), row, "Private")
if err != nil { return bookmark, err }
bookmark.Favorite, err = parseEditorBool(csvRecordValue(record, 9), row, "Favorite")
if err != nil { return bookmark, err }
```

In the application branch of `getBookmarksFromCSV`, set `bookmark.Favorite = false` before append.

- [ ] **Step 4: Run editor data tests**

Run: `go test ./config/data -run "Editor|BookmarksFromCSV" -count=1`

Expected: PASS, including rollback and atomic-save tests.

- [ ] **Step 5: Commit**

```powershell
git add config/data/editor.go config/data/editor_test.go
git commit -m "feat: persist bookmark visibility and favorites in editor"
```

---

### Task 3: Add editor checkbox columns and disable application favorites

**Files:**
- Modify: `embed/templates/editor.html`
- Modify generated: `internal/resources/templates/html/editor.html`
- Test: `internal/resources/templates/editor_template_test.go`

**Interfaces:**
- Consumes: editor JSON and 10-field CSV from Task 2.
- Produces: `Private` and `Favorite` checkbox columns before `CheckResult`.
- Produces: changing a row category to `[SuperFlare 应用]` immediately clears and disables `Favorite`.

- [ ] **Step 1: Write a failing template contract test**

```go
func TestEditorTemplateSupportsPrivateAndNormalBookmarkFavorites(t *testing.T) {
	raw, err := TPL.ReadFile("html/editor.html")
	if err != nil { t.Fatal(err) }
	page := string(raw)
	for _, expected := range []string{
		`{ data: 'Private', type: 'checkbox'`,
		`{ data: 'Favorite', type: 'checkbox'`,
		`item.Private ? 'true' : 'false'`,
		`item.Favorite && item.Category !== FLARE_FIX_CATEGORY[0] ? 'true' : 'false'`,
		`prop === 'Favorite'`,
		`FLARE_FIX_CATEGORY[0]`,
		`setDataAtRowProp(row, 'Favorite', false`,
	} {
		if !strings.Contains(page, expected) { t.Fatalf("editor template missing %q", expected) }
	}
}
```

- [ ] **Step 2: Run the test and confirm RED**

Run: `go test ./internal/resources/templates -run TestEditorTemplateSupportsPrivateAndNormalBookmarkFavorites -count=1`

Expected: FAIL on the first missing checkbox fragment.

- [ ] **Step 3: Update the source editor template**

Normalize initial values:

```js
item.Private = item.Private === true;
item.Favorite = item.Category === FLARE_FIX_CATEGORY[0] ? false : item.Favorite === true;
```

Add column headers `未登录隐藏` and `收藏` before `检查结果`; add checkbox columns before the read-only result column; include both booleans in `getBookmarkVisualRowDataFromTable`; and export them after `Desc` as lowercase strings. Add dynamic cell properties so a `Favorite` cell is read-only for application rows, plus an `afterChange` branch that clears `Favorite` whenever `Category` becomes the application sentinel.

- [ ] **Step 4: Regenerate embedded resources and run tests**

Run: `go run .\build\build.go`

Run: `go test ./internal/resources/templates ./internal/pages/editor -count=1`

Expected: PASS, including generated/source template sync.

- [ ] **Step 5: Commit**

```powershell
git add embed/templates/editor.html internal/resources/templates/html/editor.html internal/resources/templates/editor_template_test.go
git commit -m "feat: add visibility and favorite controls to editor"
```

---

### Task 4: Filter private items and build the favorites projection

**Files:**
- Modify: `internal/pages/home/application.go`
- Modify: `internal/pages/home/bookmark.go`
- Modify: `internal/pages/home/home.go`
- Test: `internal/pages/home/home_test.go`

**Interfaces:**
- Consumes: `Private`, `Favorite`, and request login state.
- Produces: `canViewPrivateItems(*echo.Context) bool`, which returns true for disabled login or a valid authenticated session and false on anonymous/invalid/session-read-error paths.
- Produces: one normal-bookmark load yielding regular and favorites HTML projections; favorites include only `bookmarks.yml` rows.

- [ ] **Step 1: Write failing projection and visibility tests**

Create table-driven tests proving:

```go
items := []model.Bookmark{
	{Name: "Zulu", URL: "https://z.example", Favorite: true},
	{Name: "alpha", URL: "https://a.example", Favorite: true, Private: true},
	{Name: "Beta", URL: "https://b.example", Favorite: true},
}
```

- anonymous rendering excludes `alpha` from the bookmarks and favorites HTML;
- trusted rendering includes it;
- favorites order is `alpha`, `Beta`, `Zulu` for trusted access and does not render category headings;
- an `apps.yml` row with `Favorite: true` never appears in favorites;
- a favorited normal bookmark remains present in regular bookmarks;
- search filtering is applied after private filtering;
- disabled-login request state is trusted, authenticated state is trusted, and session read failure is anonymous.

- [ ] **Step 2: Run the tests and confirm RED**

Run: `go test ./internal/pages/home -run "Test(Private|Favorites|CanViewPrivate)" -count=1`

Expected: FAIL because current generators do not accept visibility and no favorites projection exists.

- [ ] **Step 3: Implement filtering and projections**

Add the common predicate:

```go
func bookmarkVisible(item model.Bookmark, canViewPrivate bool) bool {
	return canViewPrivate || !item.Private
}
```

Resolve trust once per handler:

```go
func canViewPrivateItems(c *echo.Context) bool {
	if auth.IsLoginDisabled(c) { return true }
	return auth.ResolveLoginDisplayStateForView(c).ShowLoginInfo
}
```

Refactor the normal bookmark generator so one `LoadNormalBookmarks()` call applies dynamic URLs, visibility, and search once, then renders:

```go
type bookmarkModules struct {
	Bookmarks template.HTML
	Favorites template.HTML
	HasDescriptions bool
}
```

Copy favorite candidates before stable sorting by `strings.ToLower(strings.TrimSpace(Name))`, then original `Name`; render them through the same item renderer without categories. Add a visibility-aware application generator that filters before search. Keep existing exported generator wrappers for compatibility and route request handlers through the visibility-aware paths.

- [ ] **Step 4: Run home tests**

Run: `go test ./internal/pages/home -count=1`

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/pages/home/application.go internal/pages/home/bookmark.go internal/pages/home/home.go internal/pages/home/home_test.go
git commit -m "feat: filter private home items and project favorites"
```

---

### Task 5: Render the favorites module and delayed accessible tooltip

**Files:**
- Modify: `embed/templates/home.html`
- Modify generated: `internal/resources/templates/html/home.html`
- Modify: `embed/assets/css/base.css`
- Modify: `embed/assets/css/home/bookmarks.css`
- Modify generated: `config/define/style.go`
- Modify: `internal/pages/home/bookmark.go`
- Modify: `internal/pages/home/home.go`
- Modify: `internal/i18n/locales/zh.json`
- Modify: `internal/i18n/locales/en.json`
- Test: `internal/pages/home/home_test.go`
- Test: `internal/resources/templates/home_template_test.go`
- Test: `internal/i18n/i18n_test.go`

**Interfaces:**
- Consumes: the bookmark module projections from Task 4.
- Produces: `data-bookmark-description` only for non-empty trimmed descriptions.
- Produces: `InlineBookmarkTooltipScript` only when rendered bookmark/favorite content contains a description.

- [ ] **Step 1: Write failing markup, placement, CSP, and CSS tests**

Add tests asserting that descriptions are HTML-attribute escaped, whitespace-only descriptions emit no data attribute, and the script contains `500`, `textContent`, delegated pointer/focus handling, scroll/pagehide cleanup, `role="tooltip"`, and viewport positioning. Add a template test that verifies `container-favorites` occurs after `container-apps` and before `container-bookmakrs`, and that the tooltip script uses `nonce="{{.ScriptNonce}}"`. Extend CSS tests to require `.bookmark-module`, `.bookmark-description-tooltip`, and `pointer-events: none`. Require a non-empty `favorites` translation in both locales.

- [ ] **Step 2: Run tests and confirm RED**

Run: `go test ./internal/pages/home ./internal/resources/templates -run "Tooltip|FavoritesModule|BookmarkStyles" -count=1`

Expected: FAIL because no tooltip markup, favorites container, or shared styles exist.

- [ ] **Step 3: Add safe tooltip item markup and delegated script**

In `renderBookmarkItem`, emit the attribute only when `strings.TrimSpace(bookmark.Desc) != ""`:

```go
b.WriteString(` data-bookmark-description="`)
b.WriteString(template.HTMLEscapeString(strings.TrimSpace(bookmark.Desc)))
b.WriteString(`"`)
```

Add one inline script constant that creates a reusable body-level tooltip, waits 500 ms for pointer hover, shows immediately on focus, assigns description with `textContent`, manages `aria-describedby`, clamps its position to the viewport, and hides/cancels on pointer leave, focusout, scroll, Escape, visibility change, and pagehide.

- [ ] **Step 4: Render modules and share styles**

Insert this module between applications and bookmarks:

```html
{{ if .OptionShowFavorites }}
<div class="plugin-container clearfix bookmark-module" id="container-favorites">
  <h2>{{.FavoritesTitle}}</h2>{{.Favorites}}
</div>
{{ end }}
```

Add `bookmark-module` to the existing bookmarks container. Change bookmark CSS, adaptive columns, dynamic bookmark colors, and base module spacing from the single misspelled ID selector to the shared class while keeping `id="container-bookmakrs"` unchanged. Add tooltip styles with fixed positioning, high z-index, max width, wrapping, theme colors, and `pointer-events:none`.

- [ ] **Step 5: Bind CSP/template fields and regenerate resources**

Add `favorites` as `收藏` in Chinese and `Favorites` in English, plus `resolveFavoritesTitle` returning the trimmed custom title or `i18n.T(locale, "favorites")`. Bind `Favorites`, `FavoritesTitle`, `OptionShowFavorites`, and `InlineBookmarkTooltipScript`. Include tooltip presence in `maybeMakeScriptNonce(...)` before setting CSP. Run:

```powershell
go run .\build\build.go
go test ./internal/pages/home ./internal/resources/templates ./internal/i18n -count=1
```

Expected: PASS and source/generated assets remain synchronized.

- [ ] **Step 6: Commit**

```powershell
git add embed/templates/home.html internal/resources/templates/html/home.html embed/assets/css/base.css embed/assets/css/home/bookmarks.css config/define/style.go internal/pages/home/bookmark.go internal/pages/home/home.go internal/i18n/locales/zh.json internal/i18n/locales/en.json internal/pages/home/home_test.go internal/resources/templates/home_template_test.go internal/i18n/i18n_test.go
git commit -m "feat: render favorites and bookmark description tooltips"
```

---

### Task 6: Add favorites appearance settings and translations

**Files:**
- Modify: `config/data/settings.go`
- Modify: `internal/settings/appearance/appearance.go`
- Modify: `embed/templates/settings-appearance.html`
- Modify generated: `internal/resources/templates/html/settings-appearance.html`
- Modify: `internal/i18n/locales/zh.json`
- Modify: `internal/i18n/locales/en.json`
- Test: `config/data/settings_test.go`
- Test: `internal/settings/appearance/appearance_test.go`
- Test: `internal/resources/templates/editor_template_test.go`
- Test: `internal/i18n/i18n_test.go`

**Interfaces:**
- Consumes: `ShowFavorites` and `FavoritesTitle` from Task 1.
- Produces: form fields `show-favorites` and `favorites-title` placed between application and bookmark controls.
- Produces: form-label keys `show_favorites` and `custom_favorites_title` in both locales; Task 5 already provides the `favorites` title key.

- [ ] **Step 1: Write failing persistence, form-order, and i18n tests**

Extend `TestUpdateAppearance` to save `ShowFavorites=false` and `FavoritesTitle="Pinned"`, reload, and compare both. Add a POST test with `show-favorites=1&favorites-title=Pinned` and assert the persisted settings. Add a template-order assertion:

```go
apps := strings.Index(page, `id="settings-apps-title"`)
favorites := strings.Index(page, `id="settings-show-favorites"`)
bookmarks := strings.Index(page, `id="settings-show-bookmarks"`)
if !(apps < favorites && favorites < bookmarks) { t.Fatal("favorites settings are out of order") }
```

Require non-empty `show_favorites` and `custom_favorites_title` translations in both locales.

- [ ] **Step 2: Run tests and confirm RED**

Run: `go test ./config/data ./internal/settings/appearance ./internal/resources/templates ./internal/i18n -run "Favorites|UpdateAppearance" -count=1`

Expected: FAIL because the update path and form omit the fields.

- [ ] **Step 3: Implement settings persistence and bindings**

Add to the POST body and update object:

```go
OptionShowFavorites  bool   `form:"show-favorites"`
OptionFavoritesTitle string `form:"favorites-title"`

update.ShowFavorites = body.OptionShowFavorites
update.FavoritesTitle = strings.TrimSpace(body.OptionFavoritesTitle)
```

Copy both fields in `data.UpdateAppearance`, bind `OptionShowFavorites` and `OptionFavoritesTitle` on GET, add the two controls between applications and bookmarks, and add both new labels to each locale file.

- [ ] **Step 4: Regenerate resources and run focused tests**

Run:

```powershell
go run .\build\build.go
go test ./config/data ./internal/settings/appearance ./internal/resources/templates ./internal/i18n ./internal/pages/home -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add config/data/settings.go internal/settings/appearance/appearance.go embed/templates/settings-appearance.html internal/resources/templates/html/settings-appearance.html internal/i18n/locales/zh.json internal/i18n/locales/en.json config/data/settings_test.go internal/settings/appearance/appearance_test.go internal/resources/templates/editor_template_test.go internal/i18n/i18n_test.go internal/pages/home/home.go
git commit -m "feat: add favorites appearance settings"
```

---

### Task 7: Verify compatibility and rendered behavior

**Files:**
- Verify only; change files only to fix failures with a new failing regression test first.

**Interfaces:**
- Consumes: all prior tasks.
- Produces: fresh evidence that legacy configs, generated assets, all Go packages, scripts, and browser interactions pass.

- [ ] **Step 1: Run formatting and generated-resource checks**

Run:

```powershell
gofmt -w config/model/bookmark.go config/model/application.go config/data/config.go config/data/editor.go config/data/settings.go internal/pages/home/application.go internal/pages/home/bookmark.go internal/pages/home/home.go internal/settings/appearance/appearance.go
go run .\build\build.go
git diff --check
```

Expected: all commands exit 0 and a second `go run .\build\build.go` produces no additional diff.

- [ ] **Step 2: Run focused regression suites**

Run:

```powershell
go test ./config/data ./internal/pages/editor ./internal/pages/home ./internal/settings/appearance ./internal/resources/templates ./internal/auth ./internal/i18n -count=1
```

Expected: PASS with zero failures.

- [ ] **Step 3: Run the complete test suite**

Run: `go test ./... -count=1`

Expected: PASS with zero failures.

- [ ] **Step 4: Run repository safety checks**

Run:

```powershell
pwsh -File .\tools\test-script-safety.ps1
pwsh -File .\tools\test-all-scripts.ps1
```

Expected: both commands exit 0; record any explicit platform-specific skips from their output.

- [ ] **Step 5: Browser QA**

Start SuperFlare with a temporary test configuration and verify in a real browser:

- a non-empty bookmark description appears only after 500 ms continuous hover;
- leaving before 500 ms prevents the tooltip;
- keyboard focus shows it and Escape/scroll hides it;
- anonymous access hides private applications and bookmarks;
- authenticated access and disabled-login mode show private items;
- applications cannot be favorited in `/editor`;
- visible favorites sort ascending, appear between applications and bookmarks, and remain in bookmarks;
- no favorites module appears when disabled or empty after visibility filtering.

- [ ] **Step 6: Review requirements and commit final fixes**

Re-read `docs/superpowers/specs/2026-07-15-bookmark-visibility-favorites-tooltip-design.md`, map every goal and error case to a passing test or browser observation, then run `git status --short` and inspect `git diff c7d5413..HEAD`. If verification required fixes, commit them with an accurate message; otherwise leave the task commits unchanged.
