## MODIFIED Requirements

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
