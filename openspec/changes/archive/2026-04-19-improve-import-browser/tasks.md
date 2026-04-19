## 1. Browser State & Data Model

- [x] 1.1 Add `ShowHidden`, `VisualMode`, `VisualAnchor` fields to `fileBrowserState` struct in `browser.go`
- [x] 1.2 Add `FileInfo` field to `browserEntry` struct for file size data

## 2. Browser Logic

- [x] 2.1 Update `reload()` to filter hidden entries based on `ShowHidden` flag and populate file size for `.torrent` entries
- [x] 2.2 Add `toggleAll()` method — selects all `.torrent` files if not all selected, deselects all if all are selected
- [x] 2.3 Add `clearSelection()` method — clears the `Selected` map entirely
- [x] 2.4 Add `toggleVisual()` method — enters/exits visual mode, sets/clears `VisualAnchor`
- [x] 2.5 Add `updateVisualSelection()` method — when visual mode active, selects all `.torrent` files between `VisualAnchor` and `Cursor`
- [x] 2.6 Update `move()` to call `updateVisualSelection()` when visual mode is active

## 3. Browser View Rendering

- [x] 3.1 Update `view()` to render checkbox markers `[x]`/`[ ]` instead of `*` for selected files
- [x] 3.2 Update `view()` to show file size next to `.torrent` filenames
- [x] 3.3 Update `view()` to always render a footer with full keybinding hints and selection count at the bottom
- [x] 3.4 Update `view()` to show visual mode indicator in footer when active

## 4. Keybinding Handlers

- [x] 4.1 Change start directory from `userHomeDir()` to `"."` in `model.go` `modeMain` `i` handler
- [x] 4.2 Add `V` key handler in `modeFileBrowser` — call `toggleVisual()`
- [x] 4.3 Add `ctrl+a` key handler in `modeFileBrowser` — call `toggleAll()`
- [x] 4.4 Add `ctrl+d` key handler in `modeFileBrowser` — call `clearSelection()`
- [x] 4.5 Add `H` key handler in `modeFileBrowser` — toggle `ShowHidden` and call `reload()`

## 5. Popup Overlay Rendering

- [x] 5.1 Update `renderView()` — when `modeFileBrowser`, render background view first then overlay popup using `lipgloss.Place()` centered
- [x] 5.2 Remove `modeFileBrowser` case from `renderModal()` in `view.go`
- [x] 5.3 Add popup sizing logic (~70% width, appropriate height) with minimum fallback for small terminals

## 6. Verification

- [x] 6.1 Build and run the application to verify popup renders centered over main view
- [x] 6.2 Test multi-select: space toggle, Ctrl+A select all, Ctrl+D clear, V visual mode range select
- [x] 6.3 Test hidden file toggle with `H`
- [x] 6.4 Test import flow end-to-end with selected files
- [x] 6.5 Run `go vet` and `go build` to ensure no compilation errors
