## 1. Shared Popup Infrastructure

- [x] 1.1 Create `internal/tui/popup.go` with shared helpers: `renderOverlay(width, height, content)`, `popupBox(title, content, width)`, `popupFooter(shortcuts)`, `popupSize(termW, termH, ratioW, ratioH)`
- [x] 1.2 Add `dangerStyle` Lip Gloss variable in `popup.go` with red-tinted border for delete/danger popups
- [x] 1.3 Add `isPopupMode(mode) bool` helper that returns true for all popup modes (selectFiles, delete, chooseTarget, overwrite, showURL, fileBrowser)

## 2. View Routing Rework

- [x] 2.1 Refactor `renderView()` in `view.go`: extract background rendering into `renderBackground(m)` that returns the underlying view without popups
- [x] 2.2 Update `renderView()` to call `renderBackground()` first, then overlay popup via `renderOverlay()` if `isPopupMode()` is true
- [x] 2.3 Remove the `modeFileBrowser` early-return special case in `renderView()` — route it through the same popup overlay path
- [x] 2.4 Remove all `renderModal()` calls from `renderMain()`, `renderDetailView()`, `renderDownloadView()`

## 3. Delete Confirmation Popup

- [x] 3.1 Create `renderDeletePopup(m Model) string` in `view.go` using `popupBox` with `dangerStyle`, showing torrent name for single deletes or count + first 5 filenames for batch
- [x] 3.2 Add popup footer: `y/enter=delete  n/esc=cancel`
- [x] 3.3 Wire `renderDeletePopup` into the popup routing in `renderView()`
- [x] 3.4 Delete the `modeDelete` case from `renderModal()`

## 4. File Selection Popup

- [x] 4.1 Add `ViewportOffset int` field to `selectFilesState` in `model.go` for scroll tracking in large file lists
- [x] 4.2 Create `renderSelectFilesPopup(m Model) string` in `view.go` using `popupBox`, centered overlay, file sizes, `[x]`/`[ ]` checkboxes, cursor `>`, scroll indicator in header
- [x] 4.3 Add popup footer: `space=toggle  ctrl+a=all  ctrl+d=clear  enter=confirm  esc=cancel`
- [x] 4.4 Add `Ctrl+A` handler in `handleSelectFilesKeys()` to select all files
- [x] 4.5 Add `Ctrl+D` handler in `handleSelectFilesKeys()` to deselect all files
- [x] 4.6 Implement viewport scrolling: adjust `ViewportOffset` when cursor moves beyond visible area
- [x] 4.7 Wire `renderSelectFilesPopup` into popup routing in `renderView()`
- [x] 4.8 Delete the `modeSelectFiles` case from `renderModal()`

## 5. Target Picker Popup

- [x] 5.1 Create `renderTargetPickerPopup(m Model) string` in `view.go` using `popupBox`, centered overlay, cursor-based list
- [x] 5.2 Add popup footer: `↑↓=navigate  enter=confirm  esc=cancel`
- [x] 5.3 Wire into popup routing, delete `modeChooseTarget` case from `renderModal()`

## 6. Overwrite Confirmation Popup

- [x] 6.1 Create `renderOverwritePopup(m Model) string` using `popupBox`, centered overlay, file details (name, path, sizes, diff)
- [x] 6.2 Add popup footer: `y/enter=download again  n/esc=cancel`
- [x] 6.3 Wire into popup routing, delete `modeOverwrite` case from `renderModal()`

## 7. URL Display Popup

- [x] 7.1 Create `renderShowURLPopup(m Model) string` using `popupBox`, centered overlay, URL display
- [x] 7.2 Add popup footer: `enter/esc=close`
- [x] 7.3 Wire into popup routing, delete `modeShowURL` case from `renderModal()`

## 8. File Browser Popup Alignment

- [x] 8.1 Refactor `renderFileBrowserPopup()` to use shared `popupBox()` and `renderOverlay()` helpers from `popup.go`
- [x] 8.2 Ensure file browser goes through standard popup routing (no special case in `renderView()`)

## 9. Flash Message System

- [x] 9.1 Create `internal/tui/flash.go` with `flashState` struct (message, level, setAt) and `flashLevel` type (success/error/warn/info)
- [x] 9.2 Add `flash flashState` field to `Model` struct in `model.go`
- [x] 9.3 Add `setFlash(level, msg)` method that sets flash state and returns a `tea.Tick(3s)` command for auto-dismiss
- [x] 9.4 Add `clearFlash()` method and handle `flashTimeoutMsg` in `Update()` to clear expired flashes
- [x] 9.5 Add flash rendering in `renderView()`: single styled line between content and footer (green=success, red=error, yellow=warn, blue=info)
- [x] 9.6 Clear flash on mode change in `Update()` when `m.mode` transitions

## 10. Cleanup

- [x] 10.1 Delete `renderModal()` function from `view.go`
- [x] 10.2 Verify all popup modes render correctly: selectFiles, delete, chooseTarget, overwrite, showURL, fileBrowser
- [x] 10.3 Test small terminal fallback (< 40 cols or 12 rows) for all popups
- [x] 10.4 Run `go build ./...` and `go vet ./...` to verify compilation
