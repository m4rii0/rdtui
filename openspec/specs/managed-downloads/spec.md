## Requirements

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

#### Scenario: Existing local file requires confirmation
- **WHEN** the resolved filename already exists in the configured download directory
- **THEN** the application asks whether to download again and shows the current local size plus the size difference when the remote filesize is known

#### Scenario: Active managed session is reopened
- **WHEN** a managed download is already active and the user requests another managed download
- **THEN** the application reopens the active managed download session instead of starting a second one

### Requirement: Managed download progress is visible in the TUI
The application SHALL provide a dedicated download view that shows the active managed download's filename, status, progress, transferred bytes, total bytes, speed, ETA when available, and completion or failure state. The download view's footer and full help SHALL be generated from the current download-view actions. Completion-only actions SHALL remain visible in a dimmed style while the download is still active, and switch to active when the download completes.

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
- **THEN** the footer and full-help overlay keep completion-only actions visible in a dimmed style

#### Scenario: Download completion exposes follow-up actions
- **WHEN** aria2 reports the active managed download as `complete`
- **THEN** the footer and full-help overlay include actions to open the file, reveal its directory, or delete the source torrent

### Requirement: Full shortcut help overlay is available for the managed download view
The application SHALL provide a contextual full-help overlay opened with `?` in the managed download view. The overlay SHALL list the current download-view shortcuts and SHALL render completion-only actions dimmed until they become valid.

#### Scenario: User opens full help during an active download
- **WHEN** the user presses `?` in the managed download view while a download is in progress
- **THEN** the application opens a help overlay showing the currently available in-progress download shortcuts

#### Scenario: User opens full help after download completion
- **WHEN** the user presses `?` in the managed download view after the download completes
- **THEN** the application opens a help overlay showing the completion-only actions in addition to the base download-view shortcuts

#### Scenario: User closes download help and stays in the same download state
- **WHEN** the user dismisses the help overlay from the managed download view
- **THEN** the application returns to the same underlying download view and state

### Requirement: Managed download configuration supports aria2 or direct mode
The application SHALL use `default_download_dir` and SHALL allow the user to choose between an `aria2` backend and a built-in `direct` backend. When `aria2` is selected, the application MAY allow an optional `aria2c` binary path override and SHALL keep RPC port, secret, and process lifecycle under application control.

#### Scenario: Default aria2 binary path is used
- **WHEN** `download_backend` is `aria2` and no aria2 binary path override is configured
- **THEN** the application looks up `aria2c` on `PATH` when starting the managed downloader

#### Scenario: Configured aria2 binary path is used
- **WHEN** `download_backend` is `aria2` and the user configures an aria2 binary path override
- **THEN** the application starts that binary instead of the `PATH` default

#### Scenario: Direct backend is selected
- **WHEN** `download_backend` is `direct`
- **THEN** the application downloads the resolved URL with its built-in HTTP downloader instead of starting `aria2c`
