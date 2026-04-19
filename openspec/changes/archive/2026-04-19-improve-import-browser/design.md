## Context

The import feature (`i` key) opens a file browser that lets users select `.torrent` files to upload to Real-Debrid. Currently:

- **Start directory**: Always opens in `$HOME`, requiring navigation even when the user launched `rdtui` from a directory containing torrent files
- **Rendering**: The browser content is appended as text lines at the bottom of the main view via `renderModal()`, wrapped in `boxStyle`. It is not visually distinct as a popup overlay
- **Multi-select**: Basic toggle via `Space` with `*` markers. No range select, no select-all, no visual mode
- **Hidden files**: Always shown, cluttering the listing
- **File metadata**: No file sizes displayed for `.torrent` files

The project uses Bubble Tea (charmbracelet) for TUI rendering with lipgloss for styling. There is no tview dependency — all rendering is string-based.

## Goals / Non-Goals

**Goals:**
- Make the file browser a centered popup overlay that renders on top of the background view
- Default to the current working directory for faster access to local `.torrent` files
- Add visual mode for range selection (inspired by k9s `SelectTable.SpanMark` and vim visual mode)
- Add bulk operations: select-all and clear-selection
- Hide dotfiles by default with a toggle
- Show file sizes in the listing

**Non-Goals:**
- Adding new file type support beyond `.torrent`
- Recursive directory scanning (user navigates manually)
- File preview or content inspection
- Search/filter within the file listing

## Decisions

### 1. Popup overlay via `lipgloss.Place()` (not a separate page/component)

**Choice**: Render the browser as a centered `lipgloss.Place()` overlay on top of the background view string.

**Why**: The project uses Bubble Tea (not tview). k9s uses tview's `Pages` system with `ModalForm`/`ModalList` primitives, which handle z-ordering natively. Bubble Tea has no such abstraction — the `View()` function returns a single string. The cleanest approach is to render the background view, then use `lipgloss.Place()` with `lipgloss.Center` to center a bordered box over it.

**Alternative considered**: Splitting the terminal into regions using `lipgloss.JoinHorizontal`/`JoinVertical` — rejected because it doesn't support overlapping/z-ordering needed for a popup.

### 2. Visual mode with anchor tracking (not span-mark)

**Choice**: Press `V` to enter visual mode. An anchor is set at the current cursor position. Moving with `j`/`k` selects all `.torrent` files between the anchor and cursor. Press `V` again to exit.

**Why**: This mirrors vim's visual mode, which is natural for a terminal UI. k9s's `SpanMark` (Ctrl+Space) scans backwards for a previous mark — this is less intuitive and harder to predict. Visual mode gives clear visual feedback about what will be selected.

### 3. Checkbox markers `[x]`/`[ ]` instead of `*`

**Choice**: Use `[x]` for selected files and `[ ]` for unselected files.

**Why**: This matches the existing `modeSelectFiles` pattern already in the codebase (`model.go:296-299`), creating visual consistency. The `*` marker is ambiguous with the cursor `>` prefix.

### 4. Hidden file filtering in `reload()`

**Choice**: Filter entries starting with `.` during `reload()` based on a `ShowHidden` bool field. Default `false`.

**Why**: Keeps the filtering logic in one place. The `H` key toggles the field and re-runs `reload()`. This is simpler than maintaining a separate filtered slice.

## Risks / Trade-offs

- **Terminal size sensitivity**: A centered popup needs reasonable terminal dimensions. On very small terminals (< 40 cols, < 12 rows), the popup may be cramped. → Mitigation: Set minimum popup size of 40x10; if terminal is too small, fall back to full-width rendering
- **Visual mode confusion**: Users unfamiliar with vim may not discover `V`. → Mitigation: Include `V=visual` in the popup footer keybinding hints
- **Shift+Space unavailable**: Terminal emulators cannot distinguish Shift+Space from Space. → Mitigation: Use `V` for visual mode instead (already decided)
