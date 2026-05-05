## Requirements

### Requirement: User can browse current Real-Debrid torrents in a full-width table
The application SHALL display the user's current Real-Debrid torrents in a full-width terminal table as the main view. Each row SHALL show the torrent's status, progress, size, date added, and name. The table SHALL display a sort direction indicator (↑/↓) on the currently sorted column header.

#### Scenario: Torrent list loads successfully
- **WHEN** the authenticated application fetches the user's torrents successfully
- **THEN** the UI shows a full-width torrent table with sortable columns

#### Scenario: Torrent list is empty
- **WHEN** the authenticated application fetches the user's torrents and the list is empty
- **THEN** the UI shows an empty-state message instead of the table

### Requirement: User can drill into a full-screen torrent detail view
The application SHALL allow the user to press Enter on a selected torrent row to open a full-screen detail view showing the torrent's name, status, progress, size, added date, files (with selection markers), and generated link count. Pressing ESC SHALL return the user to the torrent list.

#### Scenario: User opens torrent detail from list
- **WHEN** the user presses Enter on a selected torrent in the list view
- **THEN** the application fetches that torrent's detailed info and displays it in a full-screen detail view

#### Scenario: User returns to list from detail
- **WHEN** the user presses ESC while in the detail view
- **THEN** the application returns to the full-width torrent list view

#### Scenario: Torrent detail is unavailable temporarily
- **WHEN** loading the selected torrent detail fails but the list remains available
- **THEN** the application keeps the torrent list visible and shows an inline error instead of exiting

### Requirement: User can sort torrents by any column with a single keypress
The application SHALL provide direct keyboard sorting shortcuts for each table column: `S` for Status, `P` for Progress, `Z` for Size, `D` for Date, and `N` for Name. Pressing the same shortcut again SHALL toggle the sort direction. The current sort column and direction SHALL be indicated in the table header.

#### Scenario: User sorts by a new column
- **WHEN** the user presses a sort shortcut (S, P, Z, D, or N) and the table is not currently sorted by that column
- **THEN** the application sorts the torrent list by that column in descending order

#### Scenario: User toggles sort direction
- **WHEN** the user presses a sort shortcut for the column that is already the active sort column
- **THEN** the application toggles the sort direction between ascending and descending

### Requirement: User can add torrents from supported inputs
The application SHALL allow users to add new Real-Debrid torrents from magnet links, one or more local `.torrent` files selected through a filesystem browser, and remote `.torrent` URLs.

#### Scenario: User adds a magnet link
- **WHEN** the user submits a valid magnet link
- **THEN** the application creates a new Real-Debrid torrent and refreshes the torrent list

#### Scenario: User adds a local torrent file
- **WHEN** the user selects a valid local `.torrent` file
- **THEN** the application uploads that torrent file to Real-Debrid and refreshes the torrent list

#### Scenario: User browses a directory and selects multiple local torrent files
- **WHEN** the user opens the local torrent browser, navigates the filesystem, and confirms multiple valid `.torrent` files
- **THEN** the application uploads each selected torrent file to Real-Debrid and reports the batch results to the user

#### Scenario: Batch local import partially fails
- **WHEN** the user confirms multiple local `.torrent` files and one upload fails after earlier files have already succeeded
- **THEN** the application preserves the successful uploads, reports which files failed, and refreshes the torrent list with the torrents that were created successfully

#### Scenario: User adds a remote torrent URL
- **WHEN** the user submits a reachable `.torrent` URL
- **THEN** the application fetches the torrent file contents, uploads them to Real-Debrid, and refreshes the torrent list

### Requirement: Waiting torrents require confirmed file selection
The application SHALL detect torrents in `waiting_files_selection`, SHALL preselect the largest file by default, and SHALL require user confirmation before submitting the selected file IDs to Real-Debrid. The file selection popup SHALL render as a centered overlay with file sizes, select-all/deselect-all controls, scroll indicators for large file lists, and a footer with keybinding hints.

#### Scenario: Largest file is preselected by default
- **WHEN** the user opens a torrent that is waiting for file selection and the torrent contains multiple files
- **THEN** the application highlights the largest file as the default selection before the user confirms or edits the choice

#### Scenario: User confirms file selection
- **WHEN** the user confirms a set of selected files for a waiting torrent
- **THEN** the application submits exactly those file IDs to Real-Debrid and refreshes the torrent state

#### Scenario: User cancels file selection
- **WHEN** the user dismisses the file-selection prompt without confirming
- **THEN** the application leaves the Real-Debrid torrent unchanged

#### Scenario: File selection popup shows file sizes
- **WHEN** the file selection popup is displayed
- **THEN** each file entry SHALL show the file path and file size in human-readable format

#### Scenario: File selection popup shows select-all shortcut
- **WHEN** the user presses `Ctrl+A` in the file selection popup
- **THEN** all files SHALL be selected

#### Scenario: File selection popup shows deselect-all shortcut
- **WHEN** the user presses `Ctrl+D` in the file selection popup
- **THEN** all file selections SHALL be cleared

#### Scenario: File selection popup shows scroll indicator
- **WHEN** the file list exceeds the visible popup area
- **THEN** the popup header SHALL show a scroll position indicator (e.g., showing current position and total count)

#### Scenario: File selection popup shows footer with keybindings
- **WHEN** the file selection popup is displayed
- **THEN** a footer SHALL show: `space=toggle  ctrl+a=all  ctrl+d=clear  enter=confirm  esc=cancel`

#### Scenario: File selection popup renders as centered overlay
- **WHEN** the file selection popup is opened from any view
- **THEN** the popup SHALL render as a centered overlay on top of the background view

### Requirement: User can start a managed download from the torrent workbench
The application SHALL let the user press `d` for a ready torrent in either the list view or the detail view to begin the managed download target-selection flow.

#### Scenario: User starts managed download from list view
- **WHEN** the selected torrent is `downloaded` and the user presses `d` in the list view
- **THEN** the application opens target selection for managed download

#### Scenario: User starts managed download from detail view
- **WHEN** the selected torrent is `downloaded` and the user presses `d` in the detail view
- **THEN** the application opens target selection for managed download

### Requirement: Torrent actions respect torrent status
The application SHALL enable or disable torrent actions based on the selected torrent's Real-Debrid status so that only valid operations are offered to the user. Torrent actions SHALL be available from both the list view and the detail view, including direct URL handoff and managed download for ready torrents. Actions that are not currently valid for the selected torrent SHALL remain visible in the current screen's shortcut help, but SHALL render dimmed to indicate that they cannot currently be launched.

#### Scenario: Ready-only actions stay dimmed for unfinished torrents
- **WHEN** the selected torrent is `queued`, `downloading`, `compressing`, or `uploading`
- **THEN** the application does not offer direct URL handoff or managed download actions for that torrent
- **AND** those actions remain visible in the footer and full-help overlay in a dimmed style

#### Scenario: File-selection action appears only for waiting torrents
- **WHEN** the selected torrent is `waiting_files_selection`
- **THEN** the application offers file-selection actions instead of direct URL handoff or managed download actions
- **AND** unavailable current-screen actions remain visible in the footer and full-help overlay in a dimmed style

#### Scenario: Ready torrent exposes handoff and download actions
- **WHEN** the selected torrent is `downloaded`
- **THEN** the application offers both direct URL handoff and managed download actions for that torrent
- **AND** both actions appear in the footer and full-help overlay

### Requirement: User can delete a torrent with confirmation using the x key
The application SHALL allow a user to delete a Real-Debrid torrent by pressing `x`, always requiring explicit confirmation before deletion. The `x` key SHALL work from both the list view and the detail view. The delete confirmation popup SHALL render as a centered overlay with danger styling (red-tinted header), show the torrent name for single deletes or the count plus first N filenames for batch deletes, and include a footer with keybinding hints.

#### Scenario: Confirmed torrent deletion succeeds
- **WHEN** the user confirms deletion for a torrent
- **THEN** the application deletes that torrent from Real-Debrid and refreshes the torrent list
- **AND** a success flash message SHALL appear confirming the deletion

#### Scenario: User cancels deletion
- **WHEN** the user dismisses the deletion confirmation
- **THEN** the application leaves the torrent in place and returns to the previous view

#### Scenario: Single delete popup shows torrent name
- **WHEN** the user presses `x` on a single torrent
- **THEN** the delete confirmation popup SHALL show the torrent filename

#### Scenario: Batch delete popup shows count and filenames
- **WHEN** the user presses `x` during batch mode with multiple selected torrents
- **THEN** the delete confirmation popup SHALL show the count of torrents to delete
- **AND** SHALL list the first 5 filenames

#### Scenario: Delete popup has danger styling
- **WHEN** the delete confirmation popup is displayed
- **THEN** the popup header or title SHALL use red-tinted styling to indicate a destructive action

#### Scenario: Delete popup renders as centered overlay
- **WHEN** the delete confirmation popup is opened
- **THEN** the popup SHALL render as a centered overlay on top of the background view

#### Scenario: Delete popup shows footer with keybindings
- **WHEN** the delete confirmation popup is displayed
- **THEN** a footer SHALL show: `y/enter=delete  n/esc=cancel`

### Requirement: Context-sensitive footer is always visible
The application SHALL display a footer with keyboard shortcut hints at the bottom of the screen at all times. The footer content SHALL be generated from the currently active visible shortcuts for the current screen or popup. State-gated actions for the current screen SHALL remain visible in the footer in a dimmed style when unavailable, while unrelated shortcuts from other contexts SHALL remain hidden.

#### Scenario: Footer shows relevant list-view shortcuts with dimmed unavailable actions
- **WHEN** the user is in the torrent list view
- **THEN** the footer shows the current list-view shortcuts
- **AND** any list-view action that is unavailable for the selected torrent is rendered dimmed instead of being removed
- **AND** unrelated shortcuts from other contexts remain hidden

#### Scenario: Footer updates when selected torrent status changes
- **WHEN** the selected torrent changes from an unfinished torrent to a ready torrent
- **THEN** the footer updates so the ready-only actions change from dimmed to active for that selection

### Requirement: Full shortcut help overlay is available for rich torrent views
The application SHALL provide a contextual full-help overlay opened with `?` on rich torrent views. The overlay SHALL list the current screen's shortcuts for the active context, grouped by purpose. State-gated current-screen actions that are not currently valid SHALL remain visible in a dimmed style, while unrelated shortcuts from other contexts SHALL remain hidden.

#### Scenario: User opens full help from list view
- **WHEN** the user presses `?` in the torrent list view
- **THEN** the application opens a help overlay showing the currently available list-view shortcuts grouped by purpose

#### Scenario: User opens full help from detail view
- **WHEN** the user presses `?` in the torrent detail view
- **THEN** the application opens a help overlay showing the currently available detail-view shortcuts grouped by purpose

#### Scenario: Help reflects selected torrent status
- **WHEN** the selected torrent is not ready for direct URL or managed download
- **THEN** those actions remain visible in the help overlay in a dimmed style

#### Scenario: User closes full help without changing view
- **WHEN** the user dismisses the help overlay
- **THEN** the application returns to the same underlying view and selection state

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
