# Bookmark Visibility, Favorites, and Tooltip Design

Date: 2026-07-15
Status: Approved for implementation

## Problem

SuperFlare already stores a description for every bookmark-like row, but normal
bookmarks do not expose that description on the home page. It also has no
usable editor control for per-item anonymous visibility, and no dedicated
favorites module for selected normal bookmark rows.

The changes must preserve existing config.yml, apps.yml, and bookmarks.yml
files. Missing new fields must retain current behavior.

## Goals

- Show a custom tooltip containing a normal bookmark's non-empty description
  after a continuous 500 ms mouse hover.
- Let both application rows and normal bookmark rows be hidden from anonymous
  visitors while remaining visible to authenticated users.
- Treat disabled login mode as trusted access, so per-item anonymous hiding is
  ignored when login is disabled.
- Let normal bookmark rows, but not application rows, be marked as favorites.
- Render a dedicated favorites module between applications and bookmarks on
  the home page when at least one favorite is visible for the current request.
- Keep favorited items in the normal bookmarks module.
- Sort the favorites module as one flat, category-free list by displayed name
  in ascending order.
- Add independent ShowFavorites and FavoritesTitle appearance settings; reuse
  bookmark new-tab behavior, icon mode, and bookmark item color.
- Preserve old configuration behavior and editor round trips.

## Non-Goals

- Removing favorited items from the normal bookmarks module.
- Adding favorites categories, subdirectories, drag ordering, or a separate
  favorites configuration file.
- Adding independent favorites colors, icon mode, or new-tab behavior.
- Showing application descriptions as delayed tooltips; applications retain
  their existing visible description layout.
- Changing global page visibility or authentication rules.

## Data Model and Compatibility

model.Bookmark will use two optional booleans:

    Private  bool yaml:"private,omitempty"
    Favorite bool yaml:"favorite,omitempty"

private: true means the row is hidden only while login is enabled and the
request has no valid authenticated session. The existing Private field is
reused and made functional; it is not renamed, so any manually authored legacy
private: true values begin working as intended.

favorite: true adds a normal bookmark row to the favorites module without
removing it from the bookmarks module. Private is valid for both apps.yml and
bookmarks.yml; Favorite is persisted only for normal rows in bookmarks.yml.

model.Application adds:

    ShowFavorites  bool   yaml:"ShowFavorites"
    FavoritesTitle string yaml:"FavoritesTitle,omitempty"

YAML missing private or favorite decodes to false, preserving item visibility
and avoiding newly favorited rows. YAML missing ShowFavorites cannot
distinguish an old file from an explicit false value, so loading uses
field-presence compatibility: if the key is absent, it defaults to true; if the
key exists, its boolean value is respected. New default configuration
explicitly includes ShowFavorites: true.

All existing bookmark and application fields, cache paths, backup contents,
and atomic save behavior remain unchanged.

## Editor Data Flow

The editor stops using the legacy conversion that deliberately removed the
Private field. It serializes the complete bookmark model to the page and
preserves both booleans across application and normal bookmark rows.

The bookmark table adds two checkbox columns before the read-only link-check
column:

1. 未登录隐藏 / Hide when signed out, mapped to Private.
2. 收藏 / Favorite, mapped to Favorite.

Private applies to application rows and normal bookmark rows. Favorite applies
only to normal bookmark rows; its checkbox is disabled for application rows,
changing a favorite row into an application clears Favorite, and the server
enforces Favorite=false before writing apps.yml. Blank/new rows default both
values to false. CSV export appends the two booleans after Desc, using stable
true/false values. Server parsing accepts:

- Existing 6-, 7-, and 8-field bookmark rows, defaulting both booleans false.
- New 10-field rows containing the existing 8-field layout followed by Private
  and Favorite.

Boolean parsing accepts only empty/false and true after normalization; invalid
values return a row-specific editor validation error. The application sentinel
category continues to determine whether a row is saved to apps.yml or
bookmarks.yml.

## Authentication and Visibility

Home rendering resolves one request-level trust state:

- Login disabled: trusted.
- Login enabled with a valid session: trusted.
- Login enabled without a valid session, with an invalid session, or with a
  session read failure: anonymous.

Every application, normal bookmark, and favorite candidate passes through the
same visibility predicate. A row with Private true is excluded only for an
anonymous request. Filtering occurs before search matching, module emptiness
checks, sorting, and HTML generation, so hidden rows cannot leak through home
search results or module markup.

The same predicate applies to the main home page, home search results, and the
existing /apps and /bookmarks source subpages. A private row is therefore not
exposed through an alternate public list while the visitor is signed out.

This is presentation visibility, not an authorization boundary. Existing
redirect/helper routes are unchanged.

## Favorites Module

The home render path loads applications and normal bookmarks once, applies
request-level visibility, and builds three projections:

- Applications: visible application rows, unchanged layout.
- Favorites: visible normal bookmark rows where Favorite is true.
- Bookmarks: visible normal bookmark rows, unchanged category/subdirectory
  layout.

Favorited rows remain in the bookmarks projection. Favorites are sorted by
displayed Name ascending using deterministic, case-insensitive Unicode string
comparison with the original name and source order as tie-breakers. Categories
and subdirectories are ignored only in the favorites projection.

The module appears between applications and bookmarks only when all of these
are true:

- ShowFavorites is enabled.
- At least one favorite remains after visibility and search filtering.

The module reuses normal bookmark item markup, new-tab behavior, encryption,
icon mode, item color, delayed description tooltip, and responsive column
layout. Its default localized title is 收藏 in Chinese and Favorites in
English; a non-empty FavoritesTitle overrides it.

The module uses its own container-favorites id plus a shared bookmark-module
class. Existing container-bookmakrs markup remains compatible while shared
item, color, and responsive selectors cover both bookmark-style modules.

The existing /apps and /bookmarks subpages remain source-specific and do not
gain a new favorites subpage.

## Delayed Description Tooltip

Only normal bookmark-style items with a non-empty trimmed Desc receive an
escaped data-bookmark-description attribute. This includes normal bookmark rows
in the bookmarks module and all rows rendered in the favorites module.
Application cards keep their existing visible description and do not use the
delayed tooltip in the applications module. Since applications cannot be
favorites, every favorite tooltip also belongs to a normal bookmark.

A single delegated home-page script manages one reusable tooltip element:

- Mouse entry starts a 500 ms timer.
- Leaving, scrolling, page hiding, or moving focus elsewhere cancels the timer
  and hides the tooltip.
- Keyboard focus shows the same tooltip without an artificial delay.
- Empty descriptions generate no attribute and no tooltip.
- Text is assigned through textContent; no description is interpreted as HTML.
- The tooltip uses role="tooltip", updates aria-describedby, stays inside the
  viewport, and does not intercept pointer events.

The script is included only when rendered content contains at least one
description-bearing bookmark item, and follows the existing CSP nonce flow.

## Appearance Settings

The appearance page places the favorites controls between the applications and
bookmarks controls:

- 显示收藏 / Show favorites maps to ShowFavorites.
- 收藏标题 / Favorites title maps to FavoritesTitle.

Saving appearance settings updates both fields while preserving unrelated
configuration. The favorites module intentionally reuses:

- OpenBookmarkNewTab.
- IconMode.
- BookmarkItemColor.
- Existing responsive HomeMaxColumns and HomeMaxWidth behavior.

## Error Handling

- Broken bookmark/application YAML continues to produce the existing rendered
  configuration error rather than partial output.
- A session read error is fail-closed for per-item visibility: private items
  remain hidden, while public items still render.
- Invalid editor boolean text is rejected before either bookmark file is
  replaced.
- Empty or whitespace-only descriptions do not create tooltip state.
- An empty visible favorites projection suppresses the module even if
  ShowFavorites is true.

## Tests

Implementation follows TDD and covers:

- Old app config without ShowFavorites defaults it to true, while explicit
  false remains false.
- Old bookmark/application YAML without new item fields still loads unchanged.
- Editor JSON, table columns, CSV export, legacy CSV parsing, new boolean CSV
  parsing, validation, and atomic split-save preserve Private for both source
  files and Favorite only for normal bookmarks; application favorite controls
  are disabled and server-side save clears any forged application favorite.
- Anonymous, authenticated, and login-disabled home requests filter private
  applications, bookmarks, and favorites correctly.
- Favorites contain only normal bookmarks, stay in the bookmarks module, sort
  deterministically, ignore categories, and disappear when empty or disabled.
- Delayed tooltip markup escapes descriptions, omits empty descriptions, uses
  one 500 ms timer, and cleans up on leave, scroll, and pagehide.
- Appearance settings render in the required order and persist visibility and
  title fields.
- Existing configuration, home, editor, backup/restore, redirect, build, and
  script checks remain green.
- Browser QA verifies the 500 ms hover behavior, anonymous/private filtering,
  favorites ordering and placement, and absence of the module when no visible
  favorites exist.
