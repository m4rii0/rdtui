## ADDED Requirements

### Requirement: User can start a bulk download from batch mode
The application SHALL allow the user to start a bulk managed download from batch mode only when at least two marked torrents are ready with Real-Debrid status `downloaded`.

#### Scenario: Eligible batch selection starts bulk setup
- **WHEN** the user has marked multiple torrents in batch mode and each marked torrent has status `downloaded`
- **THEN** the `d` action is enabled and starts the bulk download setup flow

#### Scenario: Ineligible batch selection cannot start bulk setup
- **WHEN** the user has marked fewer than two torrents or any marked torrent is not `downloaded`
- **THEN** the `d` action remains unavailable and the application does not start bulk download setup

#### Scenario: Bulk setup can be cancelled
- **WHEN** the user cancels any bulk download setup popup before final confirmation
- **THEN** the application returns to batch mode without resolving links, starting downloads, or changing marked torrents

### Requirement: User can choose bulk download order
The application SHALL show a bulk order popup before file selection and SHALL default the queue to the same order as the current torrent view.

#### Scenario: Bulk order defaults to current view order
- **WHEN** the bulk order popup opens for marked ready torrents
- **THEN** the marked torrents are listed in the same order they appear in the current filtered and sorted torrent view

#### Scenario: User reorders bulk downloads
- **WHEN** the user moves items in the bulk order popup and confirms the popup
- **THEN** the application uses the confirmed order for later file-selection prompts and sequential queue execution

### Requirement: User chooses files one torrent at a time
The application SHALL inspect each ordered torrent during setup and SHALL ask which file targets to download whenever a torrent has multiple downloadable targets.

#### Scenario: Single-target torrent is selected automatically
- **WHEN** a selected torrent exposes exactly one downloadable target
- **THEN** the application adds that target to the pending bulk download plan without showing a file-choice popup for that torrent

#### Scenario: Multi-target torrent prompts for file choices
- **WHEN** a selected torrent exposes multiple downloadable targets
- **THEN** the application shows a popup for that torrent listing its downloadable files and lets the user select one or more files to download

#### Scenario: Empty file choice is rejected
- **WHEN** the user confirms a multi-target file-choice popup with no files selected
- **THEN** the application keeps the popup open and explains that at least one file must be selected

#### Scenario: File-choice prompts follow confirmed order
- **WHEN** multiple selected torrents need file-choice prompts
- **THEN** the application shows those prompts one by one in the confirmed bulk order

### Requirement: Bulk queue requires final confirmation
The application SHALL show a final confirmation popup after order and file choices are complete and before starting any managed download.

#### Scenario: Final confirmation summarizes pending work
- **WHEN** all required file choices have been collected
- **THEN** the application shows a confirmation popup summarizing the number of torrents, number of files, and download order that will be processed

#### Scenario: User confirms bulk queue
- **WHEN** the user confirms the final bulk download popup
- **THEN** the application starts the sequential bulk download queue

#### Scenario: User cancels final confirmation
- **WHEN** the user cancels the final bulk download popup
- **THEN** the application returns to batch mode without starting any downloads

### Requirement: Bulk downloads run sequentially and continue after failures
The application SHALL process bulk download files one at a time using the configured managed download backend and SHALL continue to the next file after a per-file failure.

#### Scenario: Queue starts first selected file
- **WHEN** the user confirms a bulk download plan containing one or more files
- **THEN** the application resolves the first file target and starts exactly one managed download

#### Scenario: Queue advances after completion
- **WHEN** the active bulk download file reaches a complete terminal state
- **THEN** the application records that file as successful and starts the next queued file if one remains

#### Scenario: Queue continues after failed download
- **WHEN** resolving or downloading a queued file fails
- **THEN** the application records the failure for that file and continues with the next queued file if one remains

#### Scenario: Existing file prompt is handled per queued file
- **WHEN** a queued file resolves to a filename that already exists in the configured download directory
- **THEN** the application asks whether to download that file again before starting that file's managed download

#### Scenario: Existing file cancellation skips one file
- **WHEN** the user declines to download an existing file again during bulk queue execution
- **THEN** the application records that queued file as skipped and continues with the next queued file if one remains

### Requirement: Bulk download summary reports all outcomes
The application SHALL show a finished bulk download summary that reports successful, failed, partial, and skipped outcomes after the queue has no remaining files.

#### Scenario: Queue completion shows summary
- **WHEN** the bulk download queue has processed every queued file
- **THEN** the application shows a summary containing total files, successful files, failed files, skipped files, and per-torrent outcome labels

#### Scenario: Partial torrent is identified
- **WHEN** at least one requested file for a torrent succeeds and at least one requested file for the same torrent fails or is skipped
- **THEN** the summary labels that torrent as partial

#### Scenario: Failed torrent is identified
- **WHEN** every requested file for a torrent fails or is skipped
- **THEN** the summary labels that torrent as failed or skipped according to its recorded file outcomes

### Requirement: User can clean up source torrents after bulk download
The application SHALL enable a cleanup action after bulk download completion and SHALL require confirmation before deleting any source torrents.

#### Scenario: Cleanup action opens delete selection popup
- **WHEN** a bulk download summary is visible after queue completion and the user presses `x`
- **THEN** the application opens a cleanup popup listing source torrents from the bulk download plan

#### Scenario: Successful torrents are preselected for cleanup
- **WHEN** the cleanup popup opens
- **THEN** torrents whose requested files all completed successfully are selected by default

#### Scenario: Failed and partial torrents are unselected by default
- **WHEN** the cleanup popup opens
- **THEN** torrents with failed, partial, skipped, or cancelled results are unselected by default and include an explanation of their outcome

#### Scenario: Cleanup rows are manually toggleable
- **WHEN** the cleanup popup is open
- **THEN** the user can manually toggle any listed source torrent, including failed and partial torrents

#### Scenario: Cleanup warns about risky manual selections
- **WHEN** the user selects one or more failed, partial, skipped, or cancelled torrents in the cleanup popup
- **THEN** the popup shows a warning that selected torrents include incomplete bulk downloads

#### Scenario: Confirmed cleanup deletes selected source torrents
- **WHEN** the user confirms the cleanup popup with one or more source torrents selected
- **THEN** the application deletes the selected source torrents and reports the cleanup result

#### Scenario: Cleanup cancellation preserves source torrents
- **WHEN** the user cancels the cleanup popup
- **THEN** the application leaves all source torrents unchanged and returns to the bulk download summary
