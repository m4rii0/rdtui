## 1. Dependencies & State

- [x] 1.1 Add `github.com/sahilm/fuzzy` dependency to `go.mod`
- [x] 1.2 Add `modeSearch` constant to the mode enum in `model.go`
- [x] 1.3 Add fields to `Model` struct: `searchInput textinput.Model`, `filteredTorrents []models.Torrent`, `filterApplied bool`
- [x] 1.4 Initialize `searchInput` in `NewModel()` with appropriate placeholder and width

## 2. Fuzzy Matching Logic

- [x] 2.1 Create a helper function `torrentMatchString(t models.Torrent) string` in `torrent_table.go` that returns the composite rendered string of all 5 columns (status label, formatted progress, human-readable size, filename, formatted date)
- [x] 2.2 Create a `filterTorrents(torrents []models.Torrent, query string) []models.Torrent` function that uses `sahilm/fuzzy` to match against `torrentMatchString` for each torrent, returning ranked results

## 3. Key Handling

- [x] 3.1 Add `/` keybinding in `modeMain` handler: enter `modeSearch`, focus `searchInput`, preserve existing query if filter was already applied
- [x] 3.2 Add `modeSearch` handler in `handleKey()`: route printable keys and backspace to `searchInput.Update()`, recalculate `filteredTorrents` on every change, clamp `selectedIdx`
- [x] 3.3 Handle `Esc` in `modeSearch`: clear query, clear `filteredTorrents` (mirror `torrents`), reset `selectedIdx` to 0, return to `modeMain`
- [x] 3.4 Handle `Enter` in `modeSearch`: if query non-empty set `filterApplied = true`, return to `modeMain`; if empty clear filter, return to `modeMain`
- [x] 3.5 Handle `up`/`down`/`j`/`k` in `modeSearch`: allow navigation within filtered results while staying in search mode

## 4. Filtering Integration

- [x] 4.1 Update `torrentsMsg` handler to also refresh `filteredTorrents` when a filter is active (re-apply filter after new data arrives)
- [x] 4.2 Update all rendering and selection logic to use `filteredTorrents` (or `torrents` when no filter) instead of always using `torrents`
- [x] 4.3 Ensure filtered results are sorted with the current sort column/direction after filtering

## 5. Rendering

- [x] 5.1 Add search bar rendering in `view.go`: show `/ ` prompt with text input content, match count indicator (e.g., "5/100"), rendered between torrent list and footer
- [x] 5.2 Render "No matches" empty state when filtered list is empty
- [x] 5.3 Show active filter indicator (search bar with query and count) when `filterApplied` is true and mode is `modeMain`
- [x] 5.4 Update footer keybind help: add `/` to the main mode footer, update help text in search mode

## 6. Tests

- [x] 6.1 Test `torrentMatchString` produces expected composite strings for sample torrents
- [x] 6.2 Test `filterTorrents` returns correct fuzzy-matched results
- [x] 6.3 Test mode transitions: `modeMain` → `/` → `modeSearch` → `Esc` → `modeMain`
- [x] 6.4 Test mode transitions: `modeSearch` → `Enter` (non-empty) → `modeMain` with filter retained
- [x] 6.5 Test `selectedIdx` clamping after filter changes
- [x] 6.6 Test that filtered results respect current sort order
