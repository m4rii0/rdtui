## MODIFIED Requirements

### Requirement: Activate search mode
The system SHALL enter search mode when the user presses `/` while in the main torrent list view (`modeMain`). The search input SHALL be focused on entry. If no filter is currently applied, the input SHALL be empty on entry. If a filter is already applied, the existing query SHALL be preserved for editing.

#### Scenario: Pressing slash in main mode without an active filter
- **WHEN** the user presses `/` and the current mode is `modeMain`
- **THEN** the system transitions to `modeSearch`, focuses the text input, and displays a search bar with an empty query

#### Scenario: Pressing slash with an active filter
- **WHEN** the user presses `/` and the current mode is `modeMain` with a filter already applied
- **THEN** the system transitions to `modeSearch`
- **AND** preserves the existing query in the focused input for editing

### Requirement: Exit search mode
The system SHALL exit search mode when the user presses `Esc` or `Enter`. While the search input is focused, printable keys SHALL be handled as text input rather than being interpreted as list-navigation shortcuts.

#### Scenario: Escape clears filter
- **WHEN** the user presses `Esc` while in `modeSearch`
- **THEN** the system clears the search query, restores the full unfiltered torrent list, resets `selectedIdx` to 0, and returns to `modeMain`

#### Scenario: Enter keeps filter
- **WHEN** the user presses `Enter` while in `modeSearch` with a non-empty query
- **THEN** the system returns to `modeMain` and retains the filtered torrent list. The search query remains visible but the input is no longer focused.

#### Scenario: Enter with empty query
- **WHEN** the user presses `Enter` while in `modeSearch` with an empty query
- **THEN** the system returns to `modeMain` with the full unfiltered torrent list

#### Scenario: Search input owns printable keys
- **WHEN** the user is in `modeSearch` and types printable characters such as `j`, `k`, or `?`
- **THEN** the system inserts those characters into the search input
- **AND** does not treat them as movement or help shortcuts

## ADDED Requirements

### Requirement: Search help reflects the active search context
When search mode is active, the application SHALL show only search-relevant non-text shortcut hints and SHALL not advertise unrelated list actions while the input is focused.

#### Scenario: Search footer hides unrelated list actions
- **WHEN** the user is in `modeSearch`
- **THEN** the footer shows only search-relevant non-text shortcuts for confirming, clearing, or leaving search
- **AND** does not show unrelated list actions such as torrent deletion or managed download

#### Scenario: Full help is unavailable while actively typing in search
- **WHEN** the search input is focused
- **THEN** pressing `?` SHALL be handled as text input rather than opening the full-help overlay
