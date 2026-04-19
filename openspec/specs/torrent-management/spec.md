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
The application SHALL detect torrents in `waiting_files_selection`, SHALL preselect the largest file by default, and SHALL require user confirmation before submitting the selected file IDs to Real-Debrid.

#### Scenario: Largest file is preselected by default
- **WHEN** the user opens a torrent that is waiting for file selection and the torrent contains multiple files
- **THEN** the application highlights the largest file as the default selection before the user confirms or edits the choice

#### Scenario: User confirms file selection
- **WHEN** the user confirms a set of selected files for a waiting torrent
- **THEN** the application submits exactly those file IDs to Real-Debrid and refreshes the torrent state

#### Scenario: User cancels file selection
- **WHEN** the user dismisses the file-selection prompt without confirming
- **THEN** the application leaves the Real-Debrid torrent unchanged

### Requirement: User can start a managed download from the torrent workbench
The application SHALL let the user press `d` for a ready torrent in either the list view or the detail view to begin the managed download target-selection flow.

#### Scenario: User starts managed download from list view
- **WHEN** the selected torrent is `downloaded` and the user presses `d` in the list view
- **THEN** the application opens target selection for managed download

#### Scenario: User starts managed download from detail view
- **WHEN** the selected torrent is `downloaded` and the user presses `d` in the detail view
- **THEN** the application opens target selection for managed download

### Requirement: Torrent actions respect torrent status
The application SHALL enable or disable torrent actions based on the selected torrent's Real-Debrid status so that only valid operations are offered to the user. Torrent actions SHALL be available from both the list view and the detail view, including direct URL handoff and managed download for ready torrents.

#### Scenario: Ready-only actions stay disabled for unfinished torrents
- **WHEN** the selected torrent is `queued`, `downloading`, `compressing`, or `uploading`
- **THEN** the application does not offer direct URL handoff or managed download actions for that torrent

#### Scenario: File-selection action appears for waiting torrents
- **WHEN** the selected torrent is `waiting_files_selection`
- **THEN** the application offers file-selection actions instead of direct URL handoff or managed download actions

#### Scenario: Ready torrent exposes handoff and download actions
- **WHEN** the selected torrent is `downloaded`
- **THEN** the application offers both direct URL handoff and managed download actions for that torrent

### Requirement: User can delete a torrent with confirmation using the x key
The application SHALL allow a user to delete a Real-Debrid torrent by pressing `x`, always requiring explicit confirmation before deletion. The `x` key SHALL work from both the list view and the detail view.

#### Scenario: Confirmed torrent deletion succeeds
- **WHEN** the user confirms deletion for a torrent
- **THEN** the application deletes that torrent from Real-Debrid and refreshes the torrent list

#### Scenario: User cancels deletion
- **WHEN** the user dismisses the deletion confirmation
- **THEN** the application leaves the torrent in place and returns to the previous view

### Requirement: Context-sensitive footer is always visible
The application SHALL display a footer with keyboard shortcut hints at the bottom of the screen at all times. The footer content SHALL change based on the current view to show only the relevant shortcuts, including managed download when that action is available.

#### Scenario: Footer shows list view shortcuts
- **WHEN** the user is in the torrent list view
- **THEN** the footer shows shortcuts for navigation, sorting, refresh, add-torrent, direct URL handoff, managed download, delete, and quit

#### Scenario: Footer shows detail view shortcuts
- **WHEN** the user is in the torrent detail view
- **THEN** the footer shows shortcuts for going back, select files, direct URL handoff, managed download, delete, and refresh
