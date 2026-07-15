# Application Subdirectory Modals Design

Date: 2026-07-15
Status: Approved for implementation under the user's existing plan-to-implementation authorization

## Problem

Application rows already preserve `subdir`, but the home application renderer ignores it. Users therefore cannot group application shortcuts even though the editor and YAML retain the value.

## Goals

- Render one folder card for every non-empty visible application subdirectory.
- Put folder cards before ungrouped application cards in the same application grid, without forcing a separate row.
- Sort folders by trimmed display name using deterministic case-insensitive ascending order, then the original name as a tie-breaker.
- Open a hash-target modal, consistent with `#page-warnings-modal`, when a folder card is activated.
- Render only that folder's application cards inside its modal.
- Give the modal explicit minimum and maximum dimensions so small collections do not collapse it and large collections do not expand the whole overlay.
- Keep the modal header fixed while only its content region scrolls.
- Preserve visibility, search, dynamic URL, local URL, icon mode, encrypted-link, and new-tab behavior.
- Preserve all existing configuration files without schema changes.

## Non-Goals

- Nested application folders.
- A separate folder configuration file or appearance settings.
- Drag ordering of folders.
- Moving ordinary bookmark subdirectories to the new modal UI.
- Invalidating every open modal when another browser tab changes configuration.

## Alternatives

### Server-rendered hash-target modals with focus management (selected)

Generate folder triggers and modal markup together with the application projection. CSS `:target` opens and closes each modal, matching the existing warnings modal. A small nonce-protected inline enhancement moves focus into the active dialog, makes background siblings inert, traps keyboard focus, closes on Escape, and restores focus to the invoking folder. It uses the existing CSP nonce path and adds no client-side data serialization or CSP policy change.

### One reusable JavaScript modal

Render folder metadata in data attributes and populate one modal on click. This reduces repeated modal markup but adds delegated JavaScript, CSP integration, focus/state cleanup, and a second client-side rendering path.

### Native dialog elements

Render one `<dialog>` per folder and call `showModal()` from JavaScript. This has stronger native modal semantics but still requires script wiring and would diverge from the current hash-modal interaction model.

## Data Projection

`generateApplicationProjectionWithLocalAndURLErr` continues to load `apps.yml` once. It applies private-item visibility, dynamic URL resolution, and search filtering before grouping.

Search matching adds `Subdir` to the existing name, public URL, local URL, and description fields. A search that matches a folder name includes all otherwise-visible applications in that folder. A search that matches one application includes only that matching application in the folder modal.

After filtering:

- Empty or whitespace-only `Subdir` rows become ungrouped applications.
- Non-empty trimmed `Subdir` rows are grouped by the exact trimmed display name.
- Folder groups are sorted by normalized display name, then original display name, then first source position.
- Applications retain source order inside each folder and in the ungrouped list.

Folder cards are emitted first in the main application HTML, followed immediately by ungrouped application cards in the same `.apps-surface` grid. Grouped applications are not duplicated in the main list. The projection separately returns body-level modal markup. Its diagnostic `items` slice still contains every visible filtered application, including grouped rows.

The same projection is used by the home page, home search results, and `/applications`, so source-specific application views remain consistent.

## Markup and Interaction

Each folder card uses the normal application-card footprint and a built-in folder icon. Its link points to a generated ID such as `#application-subdir-modal-0`; raw folder names never become DOM IDs.

Each modal contains:

- A full-viewport fixed overlay.
- A backdrop link and close link targeting `#`.
- A labelled header showing the escaped folder name.
- A dedicated `.application-subdir-content` region containing the folder's application cards.

The panel is programmatically focusable. The backdrop is pointer-accessible but excluded from sequential focus because the visible close control provides the keyboard command. Folder triggers expose `aria-expanded`, and the focus enhancement keeps it synchronized with the active hash target.

The panel uses a fixed responsive width with explicit minimum and maximum widths. Its height also has explicit responsive minimum and maximum bounds. The panel itself uses `overflow: hidden`; the content region uses `min-height: 0` and `overflow: auto`. A targeted modal prevents the page body from scrolling in browsers that support `:has()`.

Application cards in the main module and modal share `.apps-surface`, so existing app-card appearance, responsive columns, uppercase mode, icon layout, link behavior, and dynamic column settings remain aligned.

When a folder opens, every body-level sibling outside the active modal becomes inert. Tab and Shift+Tab stay inside the active panel, Escape removes the hash and closes it, and close operations restore focus to the folder trigger. Direct hash navigation resolves the corresponding trigger for the same restoration behavior.

## Visibility and Security

Private applications are removed before grouping. Anonymous users therefore cannot infer a hidden subdirectory from a folder card, modal title, modal markup, search result, or icon diagnostic. If all applications in a subdirectory are filtered out, neither its folder nor its modal is rendered.

Names, descriptions, IDs, URLs, and attributes are escaped with `html/template` helpers. The feature uses one constant nonce-protected focus-management script, serializes no application data to JavaScript, and leaves the existing CSP policy unchanged.

## Error Handling

- Existing `apps.yml` load and validation failures continue to render the standard configuration error.
- Empty `Subdir` values remain ordinary applications.
- Duplicate exact trimmed subdirectory names merge into one folder.
- A folder with zero visible search results is omitted.
- Missing application icons continue through the existing icon fallback and diagnostic paths.

## Tests

- Projection tests cover folders before applications, deterministic folder sorting, grouping, no main-list duplication, source order inside a modal, escaping, empty subdirectories, privacy, and search-by-folder behavior.
- Handler/template tests cover body-level modal binding on home and `/applications`, stable modal IDs, source/generated template synchronization, and the shared `.apps-surface` class.
- CSS contract tests cover explicit min/max panel dimensions, panel overflow containment, internally scrolling content, and desktop/mobile sizing.
- Browser QA covers folder order, same-grid placement, modal open/close, correct application membership, a one-item folder that retains useful size, a large folder whose content scrolls without panel/page overflow, and one mobile viewport.
- The full Go suite, build, generated-resource idempotence, and script checks run before completion.
