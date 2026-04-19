## Context

`rdtui` is a Go TUI built with Bubble Tea that displays a list of torrents fetched from Real-Debrid. The list has 5 columns (status, progress, size, name, added date) and supports sorting. There is no search or filter capability. The `Model` struct already uses `textinput.Model` from `bubbles` for input modes (token, magnet, URL). Key handling is centralized in `handleKey()` dispatched by mode. The torrent list is stored as `[]models.Torrent` and rendered directly.

## Goals / Non-Goals

**Goals:**
- Add a search mode activated by `/` that filters the torrent list in real-time
- Fuzzy match across all 5 rendered column values (status string, progress %, size string, filename, date string)
- Minimal mode transitions: `/` enters search, `Esc` clears and exits, `Enter` exits keeping filter
- Show a visible search bar and result count indicator

**Non-Goals:**
- Column-specific filtering (only "search all columns" mode)
- Regex or exact-match search (fuzzy only)
- Persistent filters across sessions
- Highlighting matched substrings within results

## Decisions

### 1. Fuzzy matching library: `github.com/sahilm/fuzzy`

**Choice:** Use `github.com/sahilm/fuzzy` for fuzzy matching.

**Rationale:** Lightweight, well-maintained, provides ranked results, no external dependencies. Implements a simple fuzzy matching algorithm that works well for short strings like torrent names and status codes.

**Alternative considered:** Writing a custom matcher — unnecessary complexity for this use case. `sahilm/fuzzy` is battle-tested and widely used in the Go TUI ecosystem (used by `fzf`-like tools).

### 2. Filtered view via separate slice

**Choice:** Store a `filteredTorrents []models.Torrent` field on `Model`. When the search query is non-empty, `filteredTorrents` holds the matched subset; rendering and selection use this slice. When empty, `filteredTorrents` mirrors `torrents`.

**Rationale:** Avoids mutating the master list. Makes it trivial to restore the full list when the filter is cleared. Selection index (`selectedIdx`) operates on whichever slice is active.

**Alternative considered:** In-place filtering with undo — more complex, risk of data loss on the client side.

### 3. Search bar placement: below torrent list, above footer

**Choice:** Render the search input bar between the torrent table and the existing footer keybind help line.

**Rationale:** Keeps the footer consistent, places the search contextually near the data being filtered. Uses the existing `textinput` component which is already imported and used elsewhere.

### 4. New mode: `modeSearch`

**Choice:** Add a new `modeSearch` mode value. While active, keystrokes go to the text input. Only `Esc` and `Enter` are intercepted at the mode level.

**Rationale:** Follows the existing pattern (`modeTokenInput`, `modeMagnetInput`, etc.). Clean separation of key handling.

### 5. Matching on rendered column strings

**Choice:** For each torrent, build a composite string from all rendered column values (status label, formatted progress, human-readable size, filename, formatted date) and fuzzy-match against that.

**Rationale:** Ensures the user sees the same text they're matching against. No hidden fields or raw values that don't match the display.

## Risks / Trade-offs

- **Performance with large lists**: Fuzzy matching 100 items on every keystroke is negligible. `sahilm/fuzzy` is fast for this scale. No risk at current API limit of 100 torrents.
- **Selection index confusion**: When filter changes the list length, `selectedIdx` could be out of bounds. Mitigation: clamp `selectedIdx` to `[0, len(filteredTorrents)-1]` after every filter update.
- **Sort interaction**: The filtered list should also be sorted. Mitigation: apply sort to `filteredTorrents` after filtering, or filter then sort in the correct order.
