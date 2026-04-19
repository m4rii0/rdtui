## ADDED Requirements

### Requirement: User can generate a direct download URL from a ready torrent
The application SHALL allow a user to choose a downloadable target from a ready torrent and SHALL convert the selected Real-Debrid torrent link into a direct download URL before any local handoff action occurs. This action SHALL be available from both the list view and the detail view.

#### Scenario: Ready torrent produces a direct URL
- **WHEN** the user chooses a downloadable target from a torrent whose status is `downloaded`
- **THEN** the application calls Real-Debrid link unrestriction and returns a direct download URL for that target

#### Scenario: Torrent is not ready for handoff
- **WHEN** the user attempts to download from a torrent that is not in a ready state
- **THEN** the application refuses the handoff action and explains that the torrent is not yet downloadable

### Requirement: User can choose how the direct URL is handed off
After generating a direct download URL, the application SHALL allow the user to either view or copy the URL or launch a configured external downloader with that URL.

#### Scenario: User chooses copy or show behavior
- **WHEN** the user requests a direct URL without launching a downloader
- **THEN** the application presents the direct URL to the user and, when supported, copies it to the clipboard

#### Scenario: User launches an external downloader
- **WHEN** the user chooses the configured downloader action after a direct URL is generated
- **THEN** the application launches the configured external command with the resolved arguments for that URL

### Requirement: External downloader handoff is configurable and non-monitoring
The application SHALL support a configurable external downloader command template and SHALL return control to the torrent UI after launch without attaching to or monitoring the local transfer.

#### Scenario: Configured downloader command succeeds
- **WHEN** a valid external downloader command template is configured and launch succeeds
- **THEN** the application reports the handoff success and returns to the torrent UI without showing local transfer progress

#### Scenario: No downloader is configured
- **WHEN** the user requests a local download handoff but no external downloader command is configured
- **THEN** the application does not attempt a launch and offers URL-based handoff instead

#### Scenario: Downloader launch fails
- **WHEN** the configured external downloader command cannot be executed successfully
- **THEN** the application reports the launch failure and keeps the user in the torrent UI
