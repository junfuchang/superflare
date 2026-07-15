# Application Folder Single-Line Layout Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the unused description row from application-folder cards and let the folder title use the full text-area height.

**Architecture:** Keep the shared application card structure and add a folder-only single-line variant through existing `.application-subdirectory-trigger` markup. Remove the empty description node at render time, then use scoped CSS to make the folder text container full-height and vertically centered without changing ordinary application cards.

**Tech Stack:** Go 1.24, Go `html/template` string rendering, generated embedded CSS, existing Go test suite, in-app Browser QA.

## Global Constraints

- Do not change application configuration, grouping, sorting, privacy, search, modal IDs, modal behavior, or ordinary application-card layout.
- Keep the application card height at `40px`.
- Folder cards must contain no empty `.app-desc` element.
- Folder title text must preserve single-line ellipsis behavior.
- Work on local `main`, commit locally, and do not push.

---

### Task 1: Render application folders as full-height single-line cards

**Files:**
- Modify: `internal/pages/home/application.go:131-149`
- Modify: `embed/assets/css/home/apps.css:49-78`
- Modify generated: `config/define/style.go`
- Test: `internal/pages/home/home_test.go`

**Interfaces:**
- Consumes: existing `.application-subdirectory-trigger`, `.app-text`, and `.app-title` markup.
- Produces: folder trigger markup with exactly one title paragraph and no description paragraph.
- Produces: folder-scoped full-height Flex alignment without affecting ordinary `.app-item` cards.

- [ ] **Step 1: Write failing markup and CSS contract tests**

Add a focused projection test that renders one folder and checks its trigger fragment:

```go
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
```

Extend `TestApplicationSubdirectoryModalStyleContracts` with these generated-CSS contracts:

```go
{name: "folder text fills card", css: `.application-subdirectory-trigger .app-text {display: flex;align-items: center;height: 100%;}`},
{name: "folder title fills text width", css: `.application-subdirectory-trigger .app-title {width: 100%;margin: 0;}`},
```

Also require the folder-title rule to occur after `.apps-surface .app-title` in generated CSS so the folder-specific `margin: 0` wins the cascade.

- [ ] **Step 2: Run tests and verify RED**

Run:

```powershell
.\.tools\go\bin\go.exe test ./internal/pages/home -run "TestApplicationDirectoryTriggerUsesFullHeightSingleLineMarkup|TestApplicationSubdirectoryModalStyleContracts" -count=1
```

Expected: FAIL because folder markup still contains an empty `.app-desc` and generated CSS has no folder-specific full-height layout.

- [ ] **Step 3: Implement the minimal markup and CSS change**

Change the end of `renderApplicationDirectory` to render only its title:

```go
b.WriteString(`</p></div></a></div>`)
```

Add scoped styles after the general `.apps-surface .app-desc` rule so the folder-title margin overrides the base title margin:

```css
.application-subdirectory-trigger .app-text {
  display: flex;
  align-items: center;
  height: 100%;
}

.application-subdirectory-trigger .app-title {
  width: 100%;
  margin: 0;
}
```

- [ ] **Step 4: Format, regenerate, and verify GREEN**

Run:

```powershell
.\.tools\go\bin\gofmt.exe -w internal\pages\home\application.go internal\pages\home\home_test.go
.\.tools\go\bin\go.exe run .\build\build.go
.\.tools\go\bin\go.exe test ./internal/pages/home ./config/define -count=1
.\.tools\go\bin\go.exe test ./... -count=1
.\.tools\go\bin\go.exe build ./...
git diff --check
```

Run the generator a second time and confirm the working diff is unchanged.

- [ ] **Step 5: Browser QA**

At `1280x720` and `375x667`, verify a folder card with a long name:

- the trigger contains one visible text line and no reserved second row;
- the `.app-text` box fills the card height and the title is vertically centered;
- long text ellipsizes without overlapping the icon or adjacent card;
- opening the folder modal still works;
- console warning/error logs are empty.

- [ ] **Step 6: Commit locally**

```powershell
git add internal/pages/home/application.go embed/assets/css/home/apps.css config/define/style.go internal/pages/home/home_test.go docs/superpowers/plans/2026-07-15-application-folder-single-line-layout.md
git commit -m "style: use full-height application folder labels"
```
