## ADDED Requirements

### Requirement: User can start a managed download from the torrent workbench
The application SHALL let the user press `d` for a ready torrent in either the list view or the detail view to begin the managed download target-selection flow.

#### Scenario: User starts managed download from list view
- **WHEN** the selected torrent is `downloaded` and the user presses `d` in the list view
- **THEN** the application opens target selection for managed download

#### Scenario: User starts managed download from detail view
- **WHEN** the selected torrent is `downloaded` and the user presses `d` in the detail view
- **THEN** the application opens target selection for managed download

## MODIFIED Requirements

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

### Requirement: Context-sensitive footer is always visible
The application SHALL display a footer with keyboard shortcut hints at the bottom of the screen at all times. The footer content SHALL change based on the current view to show only the relevant shortcuts, including managed download when that action is available.

#### Scenario: Footer shows list view shortcuts
- **WHEN** the user is in the torrent list view
- **THEN** the footer shows shortcuts for navigation, sorting, refresh, add-torrent, direct URL handoff, managed download, delete, and quit

#### Scenario: Footer shows detail view shortcuts
- **WHEN** the user is in the torrent detail view
- **THEN** the footer shows shortcuts for going back, select files, direct URL handoff, managed download, delete, and refresh
