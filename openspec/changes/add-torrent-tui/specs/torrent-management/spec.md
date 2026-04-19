## ADDED Requirements

### Requirement: User can browse current Real-Debrid torrents
The application SHALL display the user's current Real-Debrid torrents in a terminal UI and SHALL show the selected torrent's details, including status, progress, and files when available.

#### Scenario: Torrent list loads successfully
- **WHEN** the authenticated application fetches the user's torrents successfully
- **THEN** the UI shows a torrent list and a detail view for the selected torrent

#### Scenario: Torrent detail is unavailable temporarily
- **WHEN** loading the selected torrent detail fails but the list remains available
- **THEN** the application keeps the torrent list visible and shows an inline detail error instead of exiting

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

### Requirement: Torrent actions respect torrent status
The application SHALL enable or disable torrent actions based on the selected torrent's Real-Debrid status so that only valid operations are offered to the user.

#### Scenario: Ready-only actions stay disabled for unfinished torrents
- **WHEN** the selected torrent is `queued`, `downloading`, `compressing`, or `uploading`
- **THEN** the application does not offer direct-download handoff actions for that torrent

#### Scenario: File-selection action appears for waiting torrents
- **WHEN** the selected torrent is `waiting_files_selection`
- **THEN** the application offers file-selection actions instead of a direct-download handoff action

### Requirement: User can delete a torrent with confirmation
The application SHALL allow a user to delete a Real-Debrid torrent only after explicit confirmation.

#### Scenario: Confirmed torrent deletion succeeds
- **WHEN** the user confirms deletion for a torrent
- **THEN** the application deletes that torrent from Real-Debrid and refreshes the torrent list

#### Scenario: User cancels deletion
- **WHEN** the user dismisses the deletion confirmation
- **THEN** the application leaves the torrent in place and returns to the previous view
