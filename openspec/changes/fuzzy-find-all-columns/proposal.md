## Why

The torrent list can grow large (up to 100 items fetched). There is currently no way to search or filter the list — users must visually scan or sort by column. A fuzzy-find triggered by `/` would let users quickly narrow down torrents by typing a query matched across all columns (status, progress, size, name, date).

## What Changes

- Add a new search mode activated by pressing `/` in the main torrent list view
- While in search mode, typing filters the displayed torrent list using fuzzy matching across all columns (status, progress %, size, filename, added date)
- Pressing `Esc` or `Enter` exits search mode; `Enter` keeps the filter applied, `Esc` clears it
- The search bar is rendered at the bottom of the torrent list area
- An active filter is visually indicated (e.g., result count shown)

## Capabilities

### New Capabilities
- `fuzzy-filter`: Fuzzy filtering of the torrent list across all columns, triggered by `/`, with real-time matching and a search bar UI

### Modified Capabilities
<!-- No existing capabilities to modify -->

## Impact

- **internal/tui/model.go**: New mode (`modeSearch`), `/` keybinding in `modeMain`, filtered torrent slice, search input state
- **internal/tui/view.go**: Search bar rendering in the footer/list area, result count indicator
- **internal/tui/torrent_table.go**: Row matching logic (fuzzy match each column's rendered string)
- **go.mod**: Likely add a fuzzy matching library (e.g., `github.com/sahilm/fuzzy`)
- **internal/tui/model_test.go**: Tests for the new search mode transitions and filtering
- **internal/tui/view_test.go**: Tests for search bar rendering
