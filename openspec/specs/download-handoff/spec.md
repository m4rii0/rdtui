## Requirements

### Requirement: User can generate a direct download URL from a ready torrent
The application SHALL allow a user to choose a downloadable target from a ready torrent and SHALL convert the selected Real-Debrid torrent link into a direct download URL before any local handoff action occurs. This action SHALL be available from both the list view and the detail view.

#### Scenario: Ready torrent produces a direct URL
- **WHEN** the user chooses a downloadable target from a torrent whose status is `downloaded`
- **THEN** the application calls Real-Debrid link unrestriction and returns a direct download URL for that target

#### Scenario: Torrent is not ready for handoff
- **WHEN** the user attempts to download from a torrent that is not in a ready state
- **THEN** the application refuses the handoff action and explains that the torrent is not yet downloadable

### Requirement: User can choose how the direct URL is handed off
After generating a direct download URL, the application SHALL allow the user to view or copy that URL without starting a managed local download.

#### Scenario: User chooses copy or show behavior
- **WHEN** the user requests a direct URL handoff for a ready torrent target
- **THEN** the application presents the direct URL to the user and, when supported, copies it to the clipboard

#### Scenario: Direct URL handoff stays separate from managed download
- **WHEN** the user chooses direct URL handoff instead of managed download
- **THEN** the application completes the URL display and copy flow without starting `aria2c`
