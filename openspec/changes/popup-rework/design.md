## Context

rdtui is a Go TUI for Real-Debrid torrent management using Bubble Tea (Elm Architecture) + Lip Gloss. Current popup/dialog system uses a mode-based state machine with 14 modes. Inline modals (`renderModal()`) are appended below content with `boxStyle` borders — not overlaid. Only the file browser uses centered overlay via `lipgloss.Place(center, center)`. This inconsistency makes the UI feel disjointed. The reference repo k9s demonstrates a clean factory-based dialog pattern with tview, but since rdtui uses Bubble Tea, we adapt the pattern to the Elm Architecture.

Key files: `internal/tui/view.go` (rendering), `internal/tui/model.go` (state/modes).

## Goals / Non-Goals

**Goals:**
- All popups render as centered overlays with consistent border/styling
- Reusable popup factory: `renderConfirmPopup()`, `renderSelectPopup()`, `renderInfoPopup()`
- File selection popup gains file sizes, select-all/deselect-all, scroll indicator, footer hints
- Delete confirmation gains danger styling, batch count display, optional confirm-with-ack
- Transient flash messages for success/error feedback
- Each popup has its own footer with keybinding hints
- Small terminal fallback (full-width, no centering padding)

**Non-Goals:**
- Switching from Bubble Tea to tview
- Theming/skin system (keep current Lip Gloss style definitions)
- Touch/mouse support in popups
- Plugin system or external popup registration

## Decisions

### D1: Centered overlay for all popups (not inline)

**Choice:** Every popup uses `lipgloss.Place(width, height, center, center, popupBox)` — same pattern as existing file browser.

**Why:** Inline modals appended below content break spatial consistency. Centered overlay clearly separates "popup context" from "background context."

**Alternative:** Keep inline for simple confirmations. Rejected — two popup patterns is worse than one.

### D2: Popup rendering in dedicated functions (not in renderModal switch)

**Choice:** Replace monolithic `renderModal()` with per-popup render functions: `renderSelectFilesPopup()`, `renderDeletePopup()`, `renderOverwritePopup()`, `renderTargetPickerPopup()`, `renderShowURLPopup()`. Each handles its own centering, sizing, and styling.

**Why:** The current 80-line switch in `renderModal()` mixes concerns. Dedicated functions are easier to test and extend.

**Alternative:** Build a generic popup wrapper that accepts content. Partially adopted — shared helpers for centering/footer/border, but each popup still has its own render function for content customization.

### D3: Shared popup helpers in popup.go

**Choice:** Create `internal/tui/popup.go` with:
- `renderOverlay(width, height int, content string) string` — centers content in terminal
- `popupBox(title, content string, width int) string` — wraps content in bordered box with title
- `popupFooter(shortcuts ...shortcutHint) string` — renders keybinding footer
- `popupSize(termW, termH, ratioW, ratioH float64) (w, h int)` — calculates popup dimensions

**Why:** Avoids duplicating the centering/boxing pattern across every popup render function.

### D4: File selection popup enhancements

**Choice:**
- Show file size next to each file (already exists in current code via `humanBytes`)
- Add `Ctrl+A` for select-all, `Ctrl+D` for deselect-all
- Show scroll indicator (e.g., `↓3/15` in header)
- Add footer with all keybinding hints
- Keep checkbox style `[x]`/`[ ]` with cursor `>`

**Why:** Matches the import popup browser UX. Select-all/deselect-all are common operations when users want all files or none.

### D5: Delete confirmation enhancements

**Choice:**
- Danger styling: red-tinted border or header for delete popups
- Batch: show count + list first N filenames
- Single: show filename
- Keep `y/Enter` to confirm, `n/Esc` to cancel
- No confirm-with-ack for now (add later if needed — YAGNI)

**Why:** The current delete popup is minimal. Better context (what's being deleted) reduces mistakes. Confirm-with-ack can be added incrementally.

### D6: Flash message system

**Choice:** Create `internal/tui/flash.go` with:
- `flashState` struct: message, level (info/success/error/warn), timestamp
- `Model.flash` field
- `flashMsg` tea.Msg triggered by a `tea.Tick` after 3 seconds to auto-clear
- Rendered as a single styled line below the main content (above footer)
- Styles: green for success, red for error, yellow for warn, blue for info

**Why:** Current status/error messages are persistent until manually cleared. Flash messages give transient feedback for completed actions (delete succeeded, files selected, etc.) without cluttering the view.

**Alternative:** Inline status messages (current approach). Rejected — they persist too long and conflict with mode changes.

### D7: View routing change

**Choice:** Modify `renderView()` to always render the background view first, then overlay any active popup on top. Remove the `modeFileBrowser` special case. All popup modes go through a common overlay path.

```go
func renderView(m Model) string {
    bg := renderBackground(m)
    if isPopupMode(m.mode) {
        return renderOverlay(m.width, m.height, renderPopupContent(m))
    }
    return appStyle.Render(bg)
}
```

**Why:** Eliminates the inconsistent routing where file browser is checked first and other popups are inline.

## Risks / Trade-offs

- **Screen flicker on popup open/close** → Re-render entire view on mode change (already the case with Bubble Tea). No new risk.
- **Popup sizing on small terminals** → Fallback to full-width rendering when terminal < 40 cols or 12 rows. Same as file browser pattern.
- **Scroll handling in file selection popup** → Need viewport/cursor tracking for large file lists. Add `viewportOffset` to `selectFilesState`, render visible window only.
- **Flash message timing conflicts with mode changes** → Clear flash on mode change. Flash only persists within the same mode.
