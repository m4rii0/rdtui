## MODIFIED Requirements

### Requirement: Managed download progress is visible in the TUI
The application SHALL provide a dedicated download view that shows the active managed download's filename, status, progress, transferred bytes, total bytes, speed, ETA when available, and completion or failure state. The download view's footer and full help SHALL be generated from the currently available download-view actions.

#### Scenario: Download is in progress
- **WHEN** aria2 reports the active managed download as `active`, `waiting`, or `paused`
- **THEN** the application refreshes and displays the latest download metrics in the managed download view

#### Scenario: Download completes
- **WHEN** aria2 reports the active managed download as `complete`
- **THEN** the application keeps the completed state visible and offers actions to open the file, reveal its directory, or delete the source torrent

#### Scenario: Download fails
- **WHEN** aria2 reports the active managed download as `error` or the managed aria2 process exits unexpectedly
- **THEN** the application shows the failure and lets the user return to the torrent workflow

#### Scenario: Download view hides completion-only actions while active
- **WHEN** aria2 reports the active managed download as `active`, `waiting`, or `paused`
- **THEN** the footer and full-help overlay show only the shortcuts valid for an in-progress download view

#### Scenario: Download completion exposes follow-up actions
- **WHEN** aria2 reports the active managed download as `complete`
- **THEN** the footer and full-help overlay include actions to open the file, reveal its directory, or delete the source torrent

## ADDED Requirements

### Requirement: Full shortcut help overlay is available for the managed download view
The application SHALL provide a contextual full-help overlay opened with `?` in the managed download view. The overlay SHALL list all currently available shortcuts for that download state and SHALL hide actions that are not currently valid.

#### Scenario: User opens full help during an active download
- **WHEN** the user presses `?` in the managed download view while a download is in progress
- **THEN** the application opens a help overlay showing the currently available in-progress download shortcuts

#### Scenario: User opens full help after download completion
- **WHEN** the user presses `?` in the managed download view after the download completes
- **THEN** the application opens a help overlay showing the completion-only actions in addition to the base download-view shortcuts

#### Scenario: User closes download help and stays in the same download state
- **WHEN** the user dismisses the help overlay from the managed download view
- **THEN** the application returns to the same underlying download view and state
