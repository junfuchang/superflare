# Editor Row Alignment and Link Check Height Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Keep Handsontable row numbers aligned with variable-height editor rows, refit the bookmark table after public-link checks, and align cell content left and vertically centered.

**Architecture:** Extend the existing rendered-height pass so it also copies each master data cell's computed content height to the matching left-clone row header before measuring the table. Schedule that same pass after link-check batch updates, and add scoped editor-table alignment CSS without changing data or request formats.

**Tech Stack:** Go 1.x, Go templates, Handsontable 6.2.2, embedded assets, repository build generator, Browser runtime.

## Global Constraints

- Apply row-header height synchronization to both category and bookmark tables.
- Preserve variable wrapped row heights, full-content container growth, and horizontal scrolling.
- Keep data cells left aligned and vertically centered; keep headers horizontally centered and vertically centered.
- Do not change bookmark/category configuration, CSV, JSON, or link-check request formats.
- Leave `fnapp/superflare/manifest` untouched.
- Commit locally on `main`; do not push.

---

### Task 1: Fix Row Alignment and Link Check Height

**Files:**
- Modify: `internal/resources/templates/editor_template_test.go`
- Modify: `embed/templates/editor.html`
- Generate: `internal/resources/templates/html/editor.html`

**Interfaces:**
- Consumes: embedded `TPL` editor template, source `embed/templates/editor.html`, `fitEditorTableHeight(instance)`, `applyLinkCheckResults(results)`, and Handsontable master/left-clone DOM.
- Produces: regression contracts plus `syncEditorRowHeaderHeights(instance)`, link-check layout scheduling, and scoped cell alignment.

- [ ] **Step 1: Add the row-header and link-check regression test**

Add after `TestEditorTemplateTablesGrowToRenderedHeight`:

```go
func TestEditorTemplateSynchronizesRowHeadersAndRefitsLinkChecks(t *testing.T) {
	raw, err := TPL.ReadFile("html/editor.html")
	if err != nil {
		t.Fatal(err)
	}
	page := string(raw)

	for _, expected := range []string{
		`function syncEditorRowHeaderHeights(instance) {`,
		`const masterRows = instance.rootElement.querySelectorAll('.ht_master .htCore tbody tr');`,
		`const headerRows = instance.rootElement.querySelectorAll('.ht_clone_left .htCore tbody tr');`,
		`const rowCount = Math.min(masterRows.length, headerRows.length);`,
		`const masterCell = masterRows[rowIndex].querySelector('td');`,
		`const headerCell = headerRows[rowIndex].querySelector('th');`,
		`const masterHeight = window.getComputedStyle(masterCell).height;`,
		`if (masterHeight && headerCell.style.height !== masterHeight) {`,
		`headerCell.style.height = masterHeight;`,
		`syncEditorRowHeaderHeights(instance);`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("editor template missing row-header synchronization %q", expected)
		}
	}

	linkCheckStart := strings.Index(page, `function applyLinkCheckResults(results) {`)
	linkCheckEnd := strings.Index(page, `document.getElementById('back-home').addEventListener`)
	if linkCheckStart == -1 || linkCheckEnd == -1 || linkCheckEnd <= linkCheckStart {
		t.Fatal("editor template missing link-check result block")
	}
	if !strings.Contains(page[linkCheckStart:linkCheckEnd], `scheduleTableLayoutSync();`) {
		t.Fatal("link-check results must schedule a full-content height update")
	}
}
```

- [ ] **Step 2: Add the cell-alignment regression test**

```go
func TestEditorTemplateAlignsCellsLeftAndMiddle(t *testing.T) {
	raw, err := os.ReadFile(filepath.Clean(filepath.Join("..", "..", "..", "embed", "templates", "editor.html")))
	if err != nil {
		t.Fatalf("read source editor template: %v", err)
	}
	page := string(raw)
	requireBlock := func(selector string, properties ...string) {
		t.Helper()
		start := strings.Index(page, selector+" {")
		if start == -1 {
			t.Fatalf("editor template missing CSS selector %q", selector)
		}
		end := strings.Index(page[start:], "}")
		if end == -1 {
			t.Fatalf("editor template CSS selector %q has no closing brace", selector)
		}
		block := page[start : start+end]
		for _, property := range properties {
			if !strings.Contains(block, property) {
				t.Fatalf("editor template CSS selector %q missing %q: %s", selector, property, block)
			}
		}
	}

	requireBlock(`#container-category .handsontable td,
        #container-bookmarks .handsontable td`, `text-align: left !important;`, `vertical-align: middle !important;`)
	requireBlock(`#container-category .handsontable th,
        #container-bookmarks .handsontable th`, `vertical-align: middle !important;`)
}
```

- [ ] **Step 3: Run the focused tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/resources/templates -run 'TestEditorTemplateSynchronizesRowHeadersAndRefitsLinkChecks|TestEditorTemplateAlignsCellsLeftAndMiddle' -count=1
```

Expected: FAIL because the row-header helper, link-check schedule, and alignment rules are absent.

- [ ] **Step 4: Add scoped cell alignment**

Extend the existing data-cell rule and add a header rule:

```css
#container-category .handsontable td,
#container-bookmarks .handsontable td {
    background: var(--editor-table-surface) !important;
    white-space: pre-wrap !important;
    overflow-wrap: anywhere;
    word-break: normal;
    text-align: left !important;
    vertical-align: middle !important;
}

#container-category .handsontable th,
#container-bookmarks .handsontable th {
    vertical-align: middle !important;
}
```

- [ ] **Step 5: Add the row-header synchronization helper**

Add immediately before `fitEditorTableHeight`:

```javascript
function syncEditorRowHeaderHeights(instance) {
    if (!instance || !instance.rootElement) { return; }
    const masterRows = instance.rootElement.querySelectorAll('.ht_master .htCore tbody tr');
    const headerRows = instance.rootElement.querySelectorAll('.ht_clone_left .htCore tbody tr');
    const rowCount = Math.min(masterRows.length, headerRows.length);
    for (let rowIndex = 0; rowIndex < rowCount; rowIndex += 1) {
        const masterCell = masterRows[rowIndex].querySelector('td');
        const headerCell = headerRows[rowIndex].querySelector('th');
        if (!masterCell || !headerCell) { continue; }
        const masterHeight = window.getComputedStyle(masterCell).height;
        if (masterHeight && headerCell.style.height !== masterHeight) {
            headerCell.style.height = masterHeight;
        }
    }
}
```

Call it after `instance.render()` in `fitEditorTableHeight`:

```javascript
instance.render();
syncEditorRowHeaderHeights(instance);
```

- [ ] **Step 6: Refit after batched link-check updates**

At the end of `applyLinkCheckResults`, after rendering, add:

```javascript
instanceBookmarks.render();
scheduleTableLayoutSync();
```

- [ ] **Step 7: Regenerate the embedded template and verify GREEN**

Run:

```powershell
.\.tools\go\bin\go.exe run ./build
.\.tools\go\bin\go.exe test ./internal/resources/templates -run 'TestEditorTemplateSynchronizesRowHeadersAndRefitsLinkChecks|TestEditorTemplateAlignsCellsLeftAndMiddle|TestEditorTemplateTablesGrowToRenderedHeight|TestGeneratedEditorTemplateMatchesSourceTemplate' -count=1
```

Expected: PASS with the source and generated templates synchronized.

- [ ] **Step 8: Commit the implementation**

```powershell
git add -- embed/templates/editor.html internal/resources/templates/editor_template_test.go internal/resources/templates/html/editor.html
git commit -m "fix: keep editor rows aligned after updates"
```

---

### Task 2: Full Verification and Browser Regression

**Files:**
- Verify only: repository and local `/editor` runtime.

**Interfaces:**
- Consumes: completed editor template implementation.
- Produces: automated and rendered evidence that the three reported regressions are fixed.

- [ ] **Step 1: Run automated verification**

```powershell
.\.tools\go\bin\go.exe test ./... -count=1
.\.tools\go\bin\go.exe build ./...
.\.tools\go\bin\go.exe vet ./internal/pages/editor ./internal/resources/templates
git diff --check
```

Expected: every command exits with code 0.

- [ ] **Step 2: Run Browser regression at DPR 1.5**

At `http://127.0.0.1:3649/editor` with a 1280 by 720 viewport:

1. Insert at least 20 empty bookmark rows through the context menu.
2. Assert row headers are consecutive, row-header/content top offsets do not
   accumulate, and existing bookmark names remain on the expected rows.
3. Click `检查公网链接状态`, wait for completion, and assert the bookmark
   holder has `scrollHeight - clientHeight == 0` and its last row is inside the
   container.
4. Assert bookmark data cells compute to `text-align: left` and
   `vertical-align: middle`; assert header cells compute to
   `vertical-align: middle`.
5. Repeat the alignment and no-vertical-scroll checks for the category table.
6. Reload without saving and verify original data is restored.
7. Check page identity, meaningful DOM, framework overlays, and console errors.

Expected: all assertions pass; the Browser screenshot command may be reported
as unavailable if the selected in-app Browser does not expose it.

- [ ] **Step 3: Request final read-only review**

Review the implementation against
`docs/superpowers/specs/2026-07-17-editor-row-alignment-and-link-check-height-design.md`,
confirm no Critical or Important findings, and verify that
`fnapp/superflare/manifest` remains the only unrelated working-tree change.
