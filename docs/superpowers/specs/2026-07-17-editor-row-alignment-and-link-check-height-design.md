# Editor Row Alignment and Link Check Height Design

## Scope

This change fixes three related `/editor` table regressions without changing
bookmark data or configuration formats:

1. Inserting many empty bookmark rows must not make row numbers drift away from
   their corresponding row content or make link-check results attach to the
   wrong visual row.
2. Applying public-link check results must grow the bookmark table container
   instead of reintroducing an internal vertical scrollbar.
3. Data-cell content must be left aligned and vertically centered. Row and
   column headers remain horizontally centered but are vertically centered.

The existing dirty `fnapp/superflare/manifest` file is outside this scope. The
completed changes remain local on `main` and are not pushed.

## Reproduced Evidence

At a 1280 by 720 viewport with `devicePixelRatio == 1.5`:

- Handsontable's master data rows were one CSS pixel taller than the matching
  left-clone row headers. After inserting 12 empty rows, the last row number was
  20 pixels above its row content even though both layers contained 21 rows.
- A public-link check added wrapped status text and increased the rendered
  master table from about 690 pixels to 750 pixels. The container stayed at 706
  pixels and exposed 59 pixels of internal vertical scrolling.
- Empty rows were removed and reindexed for the submitted CSV, but returned
  result row numbers were applied directly to unfiltered visual rows. A result
  for `wallroom` at compact row 9 was written to an empty visual row 9 instead
  of the bookmark's visual row 21.
- Data cells computed to `text-align: start` and `vertical-align: top`.

## Root Causes

### Row-number drift

Handsontable 6.2.2 `AutoRowSize` measures wrapped master data cells correctly,
but the left-clone row-header cells receive inline content heights one CSS pixel
shorter at fractional device scaling. Because the master and clone are separate
tables, the one-pixel difference accumulates after every row. Empty rows make
the issue especially visible but do not corrupt or reorder bookmark data.

### Link-check overflow

`applyLinkCheckResults` updates the full bookmark data set and renders the
instance, but it does not call the existing scheduled layout synchronizer.
Wrapped error details therefore change the master table height without updating
the numeric Handsontable container height.

### Link-check row mapping

`exportBookmarksCSV` filters deleted/empty rows and assigns compact row numbers.
`applyLinkCheckResults` instead interprets each returned row number as an index
into the unfiltered visual table. Any empty row before a checked bookmark shifts
the result onto the wrong row.

Even with a compact-to-visual map, rebuilding that map only when the response
arrives is unsafe if users can edit, insert, remove, or move bookmark rows while
the request is running. The result row numbers describe the submitted snapshot,
not a later table state.

### Cell alignment

The editor table CSS defines wrapping and colors but does not define horizontal
or vertical cell alignment, so Handsontable's default top alignment remains.

## Design

### Synchronize row-header heights

Add `syncEditorRowHeaderHeights(instance)` and call it from
`fitEditorTableHeight(instance)` immediately after rendering and before the
container height is measured.

The helper reads matching rows from the master and left-clone `.htCore` tables.
For each row, it copies the computed content-box height of the first master data
cell to the matching row-header `th`. This corrects the clone's one-pixel inline
height error while preserving variable row heights, wrapped content, row
insertion/removal, and Handsontable's existing row-number behavior. A guarded
assignment avoids unnecessary style writes.

This is preferred over fixed row heights, which would break wrapped content,
and over replacing Handsontable row headers, which would duplicate selection,
context-menu, and accessibility behavior.

### Refit after link checks

After `applyLinkCheckResults` updates and renders the bookmark table, call
`scheduleTableLayoutSync()`. The existing animation-frame coalescing and
unchanged-height guard handle the asynchronous batch without introducing an
update loop.

### Map compact check rows back to visual rows

While clearing prior results, build an ordered array of visual row indexes for
rows that pass the existing `isDeletedBookmarkRow` filter. Interpret each
returned `result.row` as an index into that compact array, then apply the status
to the resolved visual row. This mirrors the export order without changing the
backend payload or writing cells one by one.

### Lock bookmark structure during a check

Immediately after validation and CSV binding, temporarily update the bookmark
Handsontable settings to `readOnly: true`, disable manual row movement, and
disable its context menu. Restore the normal settings in the request promise's
`finally` handler so success, HTTP errors, JSON errors, and network failures all
unlock the table.

The table remains visible and scrollable while the check runs. Locking only the
bookmark editor is preferred over introducing persistent client row IDs or
changing the backend payload, and it guarantees that the compact-to-visual map
describes the same row order for the request and response.

### Align cells

Apply `text-align: left !important` and `vertical-align: middle !important` to
data `td` cells in both editor tables. Apply only vertical centering to table
`th` cells so row numbers and headers keep their current horizontal alignment.

## Compatibility and Error Handling

- No bookmark, category, YAML, JSON, or CSV schema changes.
- Empty rows remain editable and continue to be excluded by existing save/check
  validation rules when they contain no bookmark data.
- Link-check results use the same empty-row filter and ordering as CSV export.
- Row-header synchronization exits safely when an instance, clone, row, or cell
  is unavailable.
- The link-check request and result schema are unchanged.
- Bookmark editing and structural operations are restored after every settled
  link-check request, including failures.
- No development, Docker, Linux, Windows, or fnapp-specific behavior is added.

## Verification

Automated template coverage will verify:

- both tables call the row-header synchronization helper during height fitting;
- the helper maps master data cells to left-clone row-header cells and guards
  unchanged assignments;
- batched link-check results schedule another table layout pass;
- compact link-check result rows map back to their matching non-empty visual
  rows when empty rows are present;
- the bookmark table is locked before the request and always unlocked in a
  promise `finally` handler;
- data cells are left aligned and vertically centered, while header cells are
  vertically centered;
- the generated embedded editor template matches the source template.

Browser QA will verify at DPR 1.5:

- at least 20 inserted empty rows keep row-header and content top positions
  aligned with no cumulative drift;
- row numbers remain consecutive and existing bookmark content stays on the
  expected row;
- returned results appear on the matching bookmark rows, never on empty rows;
- bookmark cells cannot be edited or structurally changed while a link check is
  pending and become editable again after it settles;
- public-link check results leave `scrollHeight - clientHeight == 0` while the
  container grows to the full rendered height;
- both tables report the required computed cell alignment;
- the page has meaningful content, no framework overlay, and no application
  console errors.
