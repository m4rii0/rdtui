## ADDED Requirements

### Requirement: Activate search mode
The system SHALL enter search mode when the user presses `/` while in the main torrent list view (`modeMain`). The search input SHALL be focused and empty on entry.

#### Scenario: Pressing slash in main mode
- **WHEN** the user presses `/` and the current mode is `modeMain`
- **THEN** the system transitions to `modeSearch`, focuses an empty text input, and displays a search bar

#### Scenario: Pressing slash in other modes
- **WHEN** the user presses `/` and the current mode is not `modeMain`
- **THEN** the system SHALL ignore the keypress

### Requirement: Exit search mode
The system SHALL exit search mode when the user presses `Esc` or `Enter`. `Esc` SHALL clear the filter and restore the full torrent list. `Enter` SHALL keep the current filter applied and return to `modeMain`.

#### Scenario: Escape clears filter
- **WHEN** the user presses `Esc` while in `modeSearch`
- **THEN** the system clears the search query, restores the full unfiltered torrent list, resets `selectedIdx` to 0, and returns to `modeMain`

#### Scenario: Enter keeps filter
- **WHEN** the user presses `Enter` while in `modeSearch` with a non-empty query
- **THEN** the system returns to `modeMain` and retains the filtered torrent list. The search query remains visible but the input is no longer focused.

#### Scenario: Enter with empty query
- **WHEN** the user presses `Enter` while in `modeSearch` with an empty query
- **THEN** the system returns to `modeMain` with the full unfiltered torrent list

### Requirement: Fuzzy filter across all columns
The system SHALL filter the displayed torrent list in real-time as the user types in the search input. The filter SHALL perform fuzzy matching against a composite of all rendered column values for each torrent: status label, progress percentage, human-readable size, filename, and formatted date.

#### Scenario: Typing narrows results
- **WHEN** the user types "doc" in the search input
- **THEN** the torrent list is filtered to show only torrents where "doc" fuzzy-matches any rendered column value (e.g., filename containing "documentary", status "DL" matching "d", etc.)

#### Scenario: Empty query shows all
- **WHEN** the search input is empty
- **THEN** the full unfiltered torrent list is displayed

#### Scenario: No matches
- **WHEN** the search query matches no torrents
- **THEN** the torrent list area SHALL display an empty state message (e.g., "No matches") and `selectedIdx` SHALL be clamped to 0

#### Scenario: Selection clamped after filter
- **WHEN** the filter changes and `selectedIdx` is greater than or equal to the filtered list length
- **THEN** `selectedIdx` SHALL be clamped to the last valid index (`len(filtered) - 1`), or 0 if the list is empty

### Requirement: Search bar UI
The system SHALL render a search bar below the torrent list when search mode is active or a filter is applied. The search bar SHALL display the current query and show a match count indicator (e.g., "5/100").

#### Scenario: Search bar visible during search
- **WHEN** the user is in `modeSearch`
- **THEN** a search bar with a prompt character (`/`) and the text input is rendered below the torrent list, along with a match count

#### Scenario: Active filter indicator in main mode
- **WHEN** the user has applied a filter (pressed `Enter` in search mode) and is back in `modeMain`
- **THEN** the search bar SHALL still be visible showing the active query and match count, with a visual indicator that a filter is active

#### Scenario: Clear active filter
- **WHEN** the user presses `/` while a filter is already applied in `modeMain`
- **THEN** the system SHALL enter search mode with the existing query preserved in the input, allowing the user to modify it

### Requirement: Filtered list respects sort order
The filtered torrent list SHALL maintain the current sort column and direction. After applying or updating a filter, results SHALL be sorted using the same active sort.

#### Scenario: Sorted filtered results
- **WHEN** the user is sorting by Name ascending and applies a filter
- **THEN** the filtered results SHALL appear sorted by Name ascending
