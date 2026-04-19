## Why

The import file browser opens in the user's home directory instead of the current working directory, requires unnecessary navigation. It renders as appended text at the bottom of the main view rather than as a proper centered popup, making it visually disconnected from the rest of the TUI. Multi-select exists but lacks range selection, select-all, and visual feedback comparable to other TUI file managers.

## What Changes

- Default browser start directory changes from `$HOME` to `.` (current working directory)
- File browser renders as a centered popup overlay on top of the main/detail view using `lipgloss.Place()`
- Multi-select enhanced with visual mode (`V` key) for range selection, `Ctrl+A` for select all, `Ctrl+D` for clear selection
- Selection markers changed from `*` to `[x]`/`[ ]` checkbox style
- Hidden files/directories hidden by default, toggled with `H` key
- `.torrent` files show file sizes in the listing
- Popup includes a title bar with the current directory path and a footer with keybinding hints

## Capabilities

### New Capabilities
- `import-popup-browser`: Centered popup overlay file browser with enhanced multi-select (visual mode, select-all, clear), hidden file toggle, file size display, and CWD default

### Modified Capabilities
<!-- No existing specs to modify -->

## Impact

- `internal/tui/model.go` — start directory, new keybindings for `V`, `Ctrl+A`, `Ctrl+D`, `H`
- `internal/tui/browser.go` — visual mode state, hidden toggle, bulk select methods, checkbox rendering, file size display
- `internal/tui/view.go` — popup overlay rendering via `lipgloss.Place()`, removal of browser from `renderModal()`
