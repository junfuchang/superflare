# Application Subdirectory Modals Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Group application rows with `subdir` into sorted folder cards at the front of the home application grid and open bounded, internally scrollable modals containing those applications.

**Architecture:** Extend the existing single-load application projection to split visible filtered applications into sorted directory groups and ungrouped rows. Render folder triggers in the main application HTML and return modal markup separately for body-level template placement, while sharing application-card markup and responsive grid styles through `.apps-surface`. Keep CSS `:target` as the open/close state and add a nonce-protected focus-management enhancement for inert background content, focus containment, Escape, and trigger restoration.

**Tech Stack:** Go 1.24, Echo v5, Go `html/template`, embedded HTML/CSS resources, CSS `:target` modals, existing MDI icon renderer.

## Global Constraints

- Keep `apps.yml`, `config.yml`, editor CSV, backup/restore membership, and all model fields unchanged.
- Apply private-item visibility and search filtering before grouping so hidden applications and empty folders never leak.
- Treat whitespace-only `Subdir` as ungrouped and group exact trimmed names.
- Sort directories case-insensitively by trimmed display name, then original display name, then first source position.
- Keep source order for applications inside each modal and for ungrouped applications.
- Do not duplicate grouped applications in the main application list.
- Put folder cards and ungrouped applications in one `.apps-surface` grid, with every folder card first.
- Reuse application new-tab, local URL, encrypted-link, icon, dynamic URL, color, uppercase, and column behavior.
- Use CSS hash-target modals with the existing CSP nonce path; do not change CSP policy or serialize application data to JavaScript.
- Give the modal panel explicit min/max width and height; keep panel overflow hidden and content overflow automatic.
- Apply the same projection on home, search results, and `/applications`.
- Work on local `main`, commit locally, and do not push.

---

### Task 1: Build the application directory projection

**Files:**
- Modify: `internal/pages/home/application.go`
- Test: `internal/pages/home/home_test.go`

**Interfaces:**
- Produces: `applicationProjection.Modals template.HTML` and `applicationProjection.HasDirectories bool`.
- Produces: `applicationDirectory{Name string, Items []model.Bookmark, SourceIndex int}`.
- Produces: `renderApplicationItem(*strings.Builder, model.Bookmark, *model.Application, bool, *fn.DynamicURL)` for both main and modal cards.
- Preserves: `applicationProjection.items` contains every visible filtered application for icon diagnostics.

- [ ] **Step 1: Write failing projection tests**

Add focused tests with injected `loadFavoriteBookmarks` data:

```go
func TestApplicationProjectionRendersSortedDirectoriesBeforeUngroupedApps(t *testing.T) {
	items := []model.Bookmark{
		{Name: "Zulu One", URL: "https://zulu-one.example", Subdir: "zeta"},
		{Name: "Plain", URL: "https://plain.example"},
		{Name: "Alpha Two", URL: "https://alpha-two.example", Subdir: " Alpha "},
		{Name: "Alpha One", URL: "https://alpha-one.example", Subdir: "Alpha"},
	}
	originalLoader := loadFavoriteBookmarks
	loadFavoriteBookmarks = func() (model.Bookmarks, error) {
		return model.Bookmarks{Items: items}, nil
	}
	t.Cleanup(func() { loadFavoriteBookmarks = originalLoader })

	projection, err := generateApplicationProjectionWithLocalAndURLErr("", &model.Application{IconMode: define.IconModeHidden}, false, nil, true)
	if err != nil { t.Fatalf("generate projection: %v", err) }
	mainHTML := string(projection.HTML)
	modalHTML := string(projection.Modals)
	alpha := strings.Index(mainHTML, `data-application-subdirectory="Alpha"`)
	zeta := strings.Index(mainHTML, `data-application-subdirectory="zeta"`)
	plain := strings.Index(mainHTML, `title="Plain"`)
	if alpha < 0 || zeta < 0 || plain < 0 || !(alpha < zeta && zeta < plain) {
		t.Fatalf("unexpected main application order: %s", mainHTML)
	}
	if strings.Contains(mainHTML, "Alpha One") || strings.Contains(mainHTML, "Alpha Two") || strings.Contains(mainHTML, "Zulu One") {
		t.Fatalf("grouped applications must not be duplicated in the main list: %s", mainHTML)
	}
	if strings.Index(modalHTML, "Alpha Two") > strings.Index(modalHTML, "Alpha One") {
		t.Fatalf("modal applications must keep source order: %s", modalHTML)
	}
	for _, name := range []string{"Alpha One", "Alpha Two", "Zulu One"} {
		if strings.Count(modalHTML, name) != 1 { t.Fatalf("expected one modal item %q: %s", name, modalHTML) }
	}
	}
}
```

Add tests proving:

- whitespace-only subdirectories remain ordinary application cards;
- anonymous rendering omits a private-only folder and its modal;
- searching for a subdirectory name includes all visible applications in that directory;
- searching for one application includes only that matching item in the modal;
- `<`, `&`, and quotes in folder names are escaped and raw names are not used as IDs;
- `projection.items` still contains grouped and ungrouped visible applications.

- [ ] **Step 2: Run the tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/pages/home -run "TestApplicationProjection.*(Directories|Subdir|Directory)" -count=1
```

Expected: FAIL because `applicationProjection` has no modal projection and application `Subdir` is ignored.

- [ ] **Step 3: Implement filtering, grouping, sorting, and shared card rendering**

Extend the projection and add a directory type:

```go
type applicationProjection struct {
	HTML           template.HTML
	Modals         template.HTML
	HasDirectories bool
	items          []model.Bookmark
}

type applicationDirectory struct {
	Name        string
	Items       []model.Bookmark
	SourceIndex int
}
```

Add `Subdir` to filter matching, then split filtered rows with `map[string]int` indexes so slice reallocation cannot invalidate group references:

```go
index, ok := directoryIndexes[subdir]
if !ok {
	index = len(directories)
	directoryIndexes[subdir] = index
	directories = append(directories, applicationDirectory{Name: subdir, SourceIndex: sourceIndex})
}
directories[index].Items = append(directories[index].Items, app)
```

Sort using normalized full display names, which gives the required first-character ordering and deterministic ties:

```go
sort.SliceStable(directories, func(i, j int) bool {
	left := strings.ToLower(strings.TrimSpace(directories[i].Name))
	right := strings.ToLower(strings.TrimSpace(directories[j].Name))
	if left != right { return left < right }
	if directories[i].Name != directories[j].Name { return directories[i].Name < directories[j].Name }
	return directories[i].SourceIndex < directories[j].SourceIndex
})
```

Extract the current application card renderer into `renderApplicationItem`. Render every directory trigger before ungrouped cards. Use generated IDs `application-subdir-modal-<index>` and title IDs `application-subdir-title-<index>`. Render folder icons with `mdi.GetIconByName("folder")` and escape every text/attribute value.

Render modal cards through the same `renderApplicationItem` function into `projection.Modals`. Keep all filtered applications in `projection.items`.

- [ ] **Step 4: Run focused and package tests**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/pages/home -run "TestApplicationProjection" -count=1
.\.tools\go\bin\go.exe test ./internal/pages/home -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/pages/home/application.go internal/pages/home/home_test.go
git commit -m "feat: group applications into subdirectory folders"
```

---

### Task 2: Bind folder modals into home and application pages

**Files:**
- Modify: `internal/pages/home/home.go`
- Modify: `embed/templates/home.html`
- Modify generated: `internal/resources/templates/html/home.html`
- Test: `internal/pages/home/home_test.go`
- Test: `internal/resources/templates/home_template_test.go`

**Interfaces:**
- Consumes: `applicationProjection.Modals` and `HasDirectories` from Task 1.
- Produces: template fields `ApplicationSubdirectoryModals` and `HasApplicationSubdirectories` on home, search, and `/applications`.
- Produces: one shared `.apps-surface` class on the main application grid and modal grids.

- [ ] **Step 1: Write failing handler and template tests**

Add handler renderer assertions that injected subdirectory applications produce non-empty `ApplicationSubdirectoryModals` on both `render(c, "")` and `pageApplication(c)`. Require no modal field when all matching applications are ungrouped or filtered private.

Extend the home template contract with:

```go
for _, expected := range []string{
	`class="apps-container clearfix apps-surface"`,
	`{{.ApplicationSubdirectoryModals}}`,
} {
	if !strings.Contains(page, expected) { t.Fatalf("home template missing %q", expected) }
}
```

Assert the modal placeholder occurs after the closing `page-home` content and outside `#container-apps`, so fixed overlays are body-level siblings.

- [ ] **Step 2: Run tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/pages/home ./internal/resources/templates -run "ApplicationSubdirector|AppsSurface" -count=1
```

Expected: FAIL because handlers and the template do not bind modal markup or `.apps-surface`.

- [ ] **Step 3: Bind projection fields and template markup**

In `render` and `pageApplication`, bind:

```go
m["ApplicationSubdirectoryModals"] = applications.Modals
m["HasApplicationSubdirectories"] = applications.HasDirectories
```

Change the main grid to:

```html
<div class="apps-container clearfix apps-surface">
```

Place body-level modal markup after `#page-home` and before the warning modal:

```html
{{ if .HasApplicationSubdirectories }}
{{.ApplicationSubdirectoryModals}}
{{ end }}
```

The projection owns modal structure; the template only controls placement.

- [ ] **Step 4: Regenerate templates and run tests**

Run:

```powershell
.\.tools\go\bin\go.exe run .\build\build.go
.\.tools\go\bin\go.exe test ./internal/pages/home ./internal/resources/templates -count=1
```

Expected: PASS and generated/source template synchronization remains clean.

- [ ] **Step 5: Commit**

```powershell
git add internal/pages/home/home.go embed/templates/home.html internal/resources/templates/html/home.html internal/pages/home/home_test.go internal/resources/templates/home_template_test.go
git commit -m "feat: render application subdirectory modals"
```

---

### Task 3: Constrain modal dimensions and internal scrolling

**Files:**
- Modify: `embed/assets/css/home/apps.css`
- Modify: `internal/pages/home/home.go`
- Modify generated: `config/define/style.go`
- Test: `internal/pages/home/home_test.go`

**Interfaces:**
- Consumes: `.apps-surface`, `.application-subdirectory-modal`, `.application-subdirectory-panel`, and `.application-subdirectory-content` from Tasks 1-2.
- Produces: shared application card styling and responsive grid behavior in the main module and every modal.

- [ ] **Step 1: Write failing CSS contract tests**

Extend the existing style assertions to require these exact behavioral contracts in generated CSS or custom home style:

```text
.apps-surface
.application-subdirectory-modal:target
.application-subdirectory-panel
min-width:
max-width:
min-height:
max-height:
overflow:hidden
.application-subdirectory-content
min-height:0
overflow:auto
body:has(.application-subdirectory-modal:target)
```

Also assert the dynamic column CSS targets `.apps-surface` instead of only `#container-apps .apps-container`.

- [ ] **Step 2: Run tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/pages/home -run "ApplicationSubdirectory.*Style|CustomHomeStyle" -count=1
```

Expected: FAIL because no modal sizing/scroll styles or shared surface selector exist.

- [ ] **Step 3: Implement shared app and modal styles**

Replace application-card selectors in `apps.css` with `.apps-surface` equivalents while retaining the existing main markup classes for custom CSS compatibility.

Add modal rules with these constraints:

```css
.application-subdirectory-panel {
  position: relative;
  display: flex;
  flex-direction: column;
  width: min(760px, calc(100vw - 32px));
  min-width: min(420px, calc(100vw - 32px));
  max-width: min(760px, calc(100vw - 32px));
  height: min(68vh, 680px);
  min-height: min(320px, calc(100vh - 32px));
  max-height: min(82vh, 760px);
  overflow: hidden;
}

.application-subdirectory-content {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
  overscroll-behavior: contain;
}
```

Add fixed overlay, backdrop, header, close control, and `:target` states matching the warning modal visual language. Add:

```css
body:has(.application-subdirectory-modal:target) {
  overflow: hidden;
}
```

Change `customHomeStyle` dynamic selectors to `.apps-surface` so HomeMaxColumns and HomeMaxWidth apply inside folder modals too.

- [ ] **Step 4: Regenerate CSS and run tests**

Run:

```powershell
.\.tools\go\bin\go.exe run .\build\build.go
.\.tools\go\bin\go.exe test ./internal/pages/home ./internal/resources/templates ./config/define -count=1
```

Expected: PASS; a second build produces no additional generated diff.

- [ ] **Step 5: Commit**

```powershell
git add embed/assets/css/home/apps.css internal/pages/home/home.go config/define/style.go internal/pages/home/home_test.go
git commit -m "style: constrain application subdirectory modals"
```

---

### Task 4: Verify rendered behavior and compatibility

**Files:**
- Verify only; add a failing regression test before any corrective code change.

**Interfaces:**
- Consumes: all prior tasks.
- Produces: fresh build, test, browser, and review evidence.

- [ ] **Step 1: Run formatting and generated-resource checks**

Run:

```powershell
.\.tools\go\bin\gofmt.exe -w internal\pages\home\application.go internal\pages\home\home.go internal\pages\home\home_test.go internal\resources\templates\home_template_test.go
.\.tools\go\bin\go.exe run .\build\build.go
git diff --check
```

Run the build generator a second time and confirm it creates no further diff.

- [ ] **Step 2: Run focused and full suites**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/pages/home ./internal/resources/templates ./config/define -count=1
.\.tools\go\bin\go.exe test ./... -count=1
.\.tools\go\bin\go.exe build ./...
```

Expected: all commands exit 0.

- [ ] **Step 3: Run repository script checks**

Run:

```powershell
& .\tools\test-script-safety.ps1
& .\tools\test-all-scripts.ps1
```

Expected: PASS; record explicit Docker/platform skips.

- [ ] **Step 4: Browser QA**

Start SuperFlare from an isolated temporary configuration containing:

- ungrouped applications;
- one single-item `Alpha` directory;
- a multi-item `zeta` directory large enough to overflow the modal content;
- a private-only directory for anonymous verification;
- names/descriptions long enough to exercise truncation.

Use the in-app Browser at 1280x720 and 375x667. Verify:

- folder order is `Alpha`, `zeta`, then ungrouped applications in one grid;
- grouped applications do not appear in the main list;
- each folder opens the correct hash modal and backdrop/close controls dismiss it;
- the one-item modal keeps its minimum dimensions;
- the large modal panel remains bounded while only `.application-subdirectory-content` scrolls;
- opening a modal prevents page-body scrolling;
- anonymous users cannot see a private-only folder or modal, while login-disabled/trusted access can;
- mobile layout has no horizontal overflow or overlapping controls;
- console warning/error logs are empty.

- [ ] **Step 5: Review and final commit state**

Request one independent read-only review of the implementation range. Fix every Critical or Important finding with a failing regression test, rerun verification, confirm `git status --short` is clean, and keep all commits local on `main` without pushing.

---

### Task 5: Add modal keyboard focus management

**Files:**
- Modify: `internal/pages/home/application.go`
- Modify: `internal/pages/home/home.go`
- Modify: `embed/templates/home.html`
- Modify generated: `internal/resources/templates/html/home.html`
- Test: `internal/pages/home/home_test.go`
- Test: `internal/resources/templates/home_template_test.go`

**Interfaces:**
- Keeps CSS `:target` as the modal open state.
- Produces a nonce-protected `InlineApplicationSubdirectoryModalScript` only when folder modals exist.
- Makes panels programmatically focusable and keeps backdrop links outside sequential focus.

- [ ] **Step 1: Write failing handler, markup, and template tests**

Require every relevant handler to bind a non-empty focus-management script only when visible folder modals exist. Require the script contract to handle hash changes, background `inert`, panel focus, Tab/Shift+Tab containment, Escape, and trigger focus restoration. Require panel `tabindex="-1"`, backdrop `tabindex="-1"`, and the nonce-protected template script.

- [ ] **Step 2: Verify RED**

Run focused handler, projection-markup, and template tests. Expected: FAIL because no modal focus script or focusable panel exists.

- [ ] **Step 3: Implement focus management**

Track the invoking trigger, synchronize `aria-expanded`, make body-level siblings outside the active modal inert, focus the panel after a hash target opens, contain Tab and Shift+Tab within the panel, close on Escape, and restore focus to the invoking trigger. Preserve close-link and backdrop hash behavior and use the existing page CSP nonce.

- [ ] **Step 4: Regenerate and verify**

Run focused tests, package/full tests, generator idempotence, build, script checks, and desktop keyboard Browser QA. Verify direct hash navigation, focus cycling, Escape, focus restoration, inert cleanup, and console health.

- [ ] **Step 5: Commit and re-review**

Commit locally, request read-only re-review of the final Important finding, fix any remaining Critical or Important issue, and do not push.
