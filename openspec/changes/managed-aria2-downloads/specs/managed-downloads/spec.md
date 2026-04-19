## ADDED Requirements

### Requirement: rdtui manages its own aria2 process for local downloads
The application SHALL start and own an `aria2c` process when the user begins a managed download, SHALL expose RPC only on loopback with an app-generated secret, and SHALL shut the process down when the application exits or no managed session remains.

#### Scenario: Managed downloader starts successfully
- **WHEN** the user starts a managed download and the configured `aria2c` binary is available
- **THEN** the application starts `aria2c` with RPC enabled on loopback and proceeds to queue the resolved URL

#### Scenario: aria2c is unavailable
- **WHEN** the user starts a managed download and the configured `aria2c` binary cannot be found or started
- **THEN** the application shows a clear error and does not leave the torrent workflow in a partial state

### Requirement: User can start a managed local download from a resolved torrent target
The application SHALL resolve the selected Real-Debrid target to a direct download URL and SHALL submit that URL to the managed aria2 session using the configured download directory and resolved filename.

#### Scenario: Ready torrent starts managed download
- **WHEN** the user chooses a downloadable target from a torrent whose status is `downloaded` and confirms the managed download action
- **THEN** the application resolves the direct URL and submits it to the managed aria2 session as a local download

#### Scenario: Active managed session is reopened
- **WHEN** a managed download is already active and the user requests another managed download
- **THEN** the application reopens the active managed download session instead of starting a second one

### Requirement: Managed download progress is visible in the TUI
The application SHALL provide a dedicated download view that shows the active managed download's filename, status, progress, transferred bytes, total bytes, speed, ETA when available, and completion or failure state.

#### Scenario: Download is in progress
- **WHEN** aria2 reports the active managed download as `active`, `waiting`, or `paused`
- **THEN** the application refreshes and displays the latest download metrics in the managed download view

#### Scenario: Download completes
- **WHEN** aria2 reports the active managed download as `complete`
- **THEN** the application keeps the completed state visible and offers actions to open the file or reveal its directory

#### Scenario: Download fails
- **WHEN** aria2 reports the active managed download as `error` or the managed aria2 process exits unexpectedly
- **THEN** the application shows the failure and lets the user return to the torrent workflow

### Requirement: Managed download configuration stays minimal and app-owned
The application SHALL use `default_download_dir` and MAY allow an optional `aria2c` binary path override, but SHALL keep RPC port, secret, and process lifecycle under application control.

#### Scenario: Default aria2 binary path is used
- **WHEN** no aria2 binary path override is configured
- **THEN** the application looks up `aria2c` on `PATH` when starting the managed downloader

#### Scenario: Configured aria2 binary path is used
- **WHEN** the user configures an aria2 binary path override
- **THEN** the application starts that binary instead of the `PATH` default
