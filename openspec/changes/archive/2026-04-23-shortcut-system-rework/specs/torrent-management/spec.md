## MODIFIED Requirements

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

## ADDED Requirements

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
