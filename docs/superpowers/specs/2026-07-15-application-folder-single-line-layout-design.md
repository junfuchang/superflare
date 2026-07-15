# Application Folder Single-Line Layout Design

## Goal

Application-folder cards on the home application surface must not reserve a second line for description text. The folder name should use the full height of the text area beside the folder icon.

## Selected Design

Remove the empty `.app-desc` element from `renderApplicationDirectory`. Keep the existing folder icon, link semantics, modal attributes, card dimensions, sorting, and modal behavior unchanged.

Add folder-specific CSS that makes `.application-subdirectory-trigger .app-text` a full-height flex container and vertically centers its only `.app-title` child. Reset the folder title margin and give it the available width so long names keep the existing ellipsis behavior.

Ordinary application cards continue to use their current title-and-description layout. Modal application cards are also unchanged.

## Compatibility

This is a presentation-only markup and CSS change. It does not alter configuration fields, grouping, search, privacy, routes, modal IDs, or JavaScript behavior.

## Testing

- Projection markup tests reject an empty folder `.app-desc` and require one folder title.
- CSS contract tests require the folder text container to fill the card height and vertically center the title.
- Browser QA checks desktop and mobile folder cards for full-height text, vertical alignment, ellipsis, overlap, and console errors.
