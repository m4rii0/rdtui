## ADDED Requirements

### Requirement: User can select files for marked torrents in batch mode
The application SHALL allow the user to press `s` in batch mode to select Real-Debrid files for marked torrents that are eligible for file selection. Eligible torrents SHALL be prompted in the same order they appear in the current visible torrent table, including active search filters. Marked torrents that are not eligible for file selection SHALL be skipped instead of blocking the operation.

#### Scenario: Eligible marked torrents start bulk file selection
- **WHEN** the user is in batch mode and has marked one or more visible torrents whose status is `waiting_files_selection` or `magnet_conversion`
- **AND** the user presses `s`
- **THEN** the application starts a bulk file-selection setup flow for the eligible marked torrents

#### Scenario: Ineligible marked torrents are skipped
- **WHEN** the user is in batch mode and has marked a mix of eligible and ineligible torrents
- **AND** the user presses `s`
- **THEN** the application starts setup for the eligible marked torrents
- **AND** the application records the ineligible marked torrents as skipped
- **AND** the ineligible marked torrents are not prompted and are not submitted to Real-Debrid for file selection

#### Scenario: No eligible marked torrents cannot start setup
- **WHEN** the user is in batch mode and every marked torrent is ineligible for file selection
- **AND** the user presses `s`
- **THEN** the application does not start bulk file-selection setup
- **AND** the application explains that at least one marked torrent must need file selection

#### Scenario: Bulk file selection respects filtered visible order
- **WHEN** the user has an active torrent filter and marks torrents from the filtered list
- **AND** the user starts bulk file selection
- **THEN** the application prompts eligible marked torrents in their current filtered table order
- **AND** marked torrents hidden by the active filter are not included in that bulk file-selection operation

#### Scenario: Each eligible torrent gets a confirmed file-selection prompt
- **WHEN** the bulk file-selection setup flow reaches an eligible torrent
- **THEN** the application shows that torrent's files in a centered file-selection popup
- **AND** the largest file is preselected by default
- **AND** the popup supports toggling files, selecting all files, clearing all files, confirming, and cancelling

#### Scenario: Empty file selection is rejected
- **WHEN** the user confirms a bulk file-selection prompt with no files selected
- **THEN** the application keeps the prompt open
- **AND** the application explains that at least one file must be selected

#### Scenario: Cancelling setup has no side effects
- **WHEN** the user cancels any bulk file-selection setup prompt before submissions begin
- **THEN** the application returns to batch mode
- **AND** no Real-Debrid file selections are submitted
- **AND** the marked torrent selection remains unchanged

#### Scenario: Confirmed bulk file selections submit sequentially
- **WHEN** the user confirms file selections for every eligible prompted torrent
- **THEN** the application submits one `SelectFiles` request at a time in the confirmed prompt order
- **AND** the application refreshes the torrent list after submission completes

#### Scenario: Bulk file selection continues after failures
- **WHEN** a confirmed bulk file-selection submission fails for one torrent
- **THEN** the application records that torrent as failed
- **AND** the application continues submitting the remaining confirmed file selections

#### Scenario: Bulk file-selection result reports outcomes
- **WHEN** the bulk file-selection operation finishes
- **THEN** the application reports successful, failed, and skipped torrent counts to the user
