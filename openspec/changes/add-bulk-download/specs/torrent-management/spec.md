## MODIFIED Requirements

### Requirement: User can start a managed download from the torrent workbench
The application SHALL let the user press `d` for a ready torrent in either the list view or the detail view to begin the managed download target-selection flow. The application SHALL also let the user press `d` in batch mode to begin the bulk download setup flow when at least two marked torrents are ready with status `downloaded`.

#### Scenario: User starts managed download from list view
- **WHEN** the selected torrent is `downloaded` and the user presses `d` in the list view
- **THEN** the application opens target selection for managed download

#### Scenario: User starts managed download from detail view
- **WHEN** the selected torrent is `downloaded` and the user presses `d` in the detail view
- **THEN** the application opens target selection for managed download

#### Scenario: User starts bulk managed download from batch mode
- **WHEN** at least two marked torrents are `downloaded` and the user presses `d` in batch mode
- **THEN** the application begins the bulk download setup flow for the marked torrents

### Requirement: Torrent actions respect torrent status
The application SHALL enable or disable torrent actions based on the selected torrent's Real-Debrid status so that only valid operations are offered to the user. Torrent actions SHALL be available from both the list view and the detail view, including direct URL handoff and managed download for ready torrents. In batch mode, the bulk managed download action SHALL be available only when the marked set contains at least two torrents and every marked torrent has status `downloaded`. Actions that are not currently valid for the selected torrent or marked batch selection SHALL remain visible in the current screen's shortcut help, but SHALL render dimmed to indicate that they cannot currently be launched.

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

#### Scenario: Batch bulk download stays dimmed for incomplete marked sets
- **WHEN** the user is in batch mode and the marked set has fewer than two torrents or includes any torrent whose status is not `downloaded`
- **THEN** the bulk managed download action remains visible in the footer and full-help overlay in a dimmed style

#### Scenario: Batch bulk download is active for ready marked sets
- **WHEN** the user is in batch mode and the marked set contains at least two torrents whose statuses are all `downloaded`
- **THEN** the bulk managed download action appears active in the footer and full-help overlay
