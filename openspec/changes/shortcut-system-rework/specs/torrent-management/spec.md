## MODIFIED Requirements

### Requirement: Torrent actions respect torrent status
The application SHALL enable or disable torrent actions based on the selected torrent's Real-Debrid status so that only valid operations are offered to the user. Torrent actions SHALL be available from both the list view and the detail view, including direct URL handoff and managed download for ready torrents. Actions that are not currently valid SHALL be hidden from shortcut help instead of being advertised and then rejected.

#### Scenario: Ready-only actions stay hidden for unfinished torrents
- **WHEN** the selected torrent is `queued`, `downloading`, `compressing`, or `uploading`
- **THEN** the application does not offer direct URL handoff or managed download actions for that torrent
- **AND** those actions do not appear in the footer or full-help overlay

#### Scenario: File-selection action appears only for waiting torrents
- **WHEN** the selected torrent is `waiting_files_selection`
- **THEN** the application offers file-selection actions instead of direct URL handoff or managed download actions
- **AND** only the valid actions appear in the footer or full-help overlay

#### Scenario: Ready torrent exposes handoff and download actions
- **WHEN** the selected torrent is `downloaded`
- **THEN** the application offers both direct URL handoff and managed download actions for that torrent
- **AND** both actions appear in the footer and full-help overlay

### Requirement: Context-sensitive footer is always visible
The application SHALL display a footer with keyboard shortcut hints at the bottom of the screen at all times. The footer content SHALL be generated from the currently active visible shortcuts for the current screen or popup and SHALL show only the relevant shortcuts, including managed download when that action is available.

#### Scenario: Footer shows only relevant list-view shortcuts
- **WHEN** the user is in the torrent list view
- **THEN** the footer shows only shortcuts that are valid for the current list-view state and selected torrent

#### Scenario: Footer updates when selected torrent status changes
- **WHEN** the selected torrent changes from an unfinished torrent to a ready torrent
- **THEN** the footer updates to show the ready-only actions for that selection
- **AND** hidden actions from the prior state are removed

## ADDED Requirements

### Requirement: Full shortcut help overlay is available for rich torrent views
The application SHALL provide a contextual full-help overlay opened with `?` on rich torrent views. The overlay SHALL list all currently available shortcuts for the active context, grouped by purpose, and SHALL hide actions that are not currently valid.

#### Scenario: User opens full help from list view
- **WHEN** the user presses `?` in the torrent list view
- **THEN** the application opens a help overlay showing the currently available list-view shortcuts grouped by purpose

#### Scenario: User opens full help from detail view
- **WHEN** the user presses `?` in the torrent detail view
- **THEN** the application opens a help overlay showing the currently available detail-view shortcuts grouped by purpose

#### Scenario: Help reflects selected torrent status
- **WHEN** the selected torrent is not ready for direct URL or managed download
- **THEN** those actions do not appear in the help overlay

#### Scenario: User closes full help without changing view
- **WHEN** the user dismisses the help overlay
- **THEN** the application returns to the same underlying view and selection state
