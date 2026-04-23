## Why

The current shortcut system works, but its behavior and help text are split across a large raw key-handling switch and several handwritten footer renderers. That makes the TUI harder to extend safely, because adding or changing a shortcut means updating dispatch and help in separate places, which risks drift and makes text-entry states awkward.

## What Changes

- Rework keyboard handling into modular, per-view shortcut maps with shared binding groups and state-aware visibility rules.
- Generate compact footer hints from the active shortcut definitions instead of maintaining separate handwritten footer strings.
- Add a contextual full-help overlay opened with `?`, showing all currently available shortcuts for the active view or popup.
- Hide unavailable actions from both the footer and full help instead of advertising actions that the current torrent or popup state cannot use.
- Ensure text-entry contexts keep ownership of printable keys so search and browser path editing behave like real inputs.
- Keep the work internal-only: no user remapping, config schema, or persisted keymap support in this change.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `torrent-management`: Replace handwritten shortcut/footer behavior with modular, state-aware shortcut definitions and add contextual full help.
- `fuzzy-filter`: Ensure search mode treats text input as the primary owner of printable keys and exposes only the relevant non-text shortcuts while searching.
- `import-popup-browser`: Generate browser footer/help from the same shortcut definitions used for dispatch, including a distinct editing-path subcontext.
- `managed-downloads`: Generate download-view shortcut help from the same active shortcut definitions, including completion-only actions.

## Impact

- Refactors TUI shortcut dispatch and help rendering in `internal/tui`.
- Introduces a shared shortcut-definition layer for active bindings, visibility predicates, compact footer rendering, and full-help rendering.
- Adds tests to verify that dispatch behavior and advertised help stay aligned across list, detail, popup, search, browser, and managed-download states.
- Does not change user configuration, authentication, or downloader backends.
