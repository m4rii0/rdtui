## 1. Managed Aria2 Foundation

- [x] 1.1 Add the aria2 RPC dependency and create an `internal/aria2` package for RPC calls, status mapping, and managed session data.
- [x] 1.2 Implement `aria2c` process startup with app-owned RPC flags, generated secret, loopback port selection, readiness probing, and graceful shutdown fallback.
- [x] 1.3 Add focused tests for missing-binary handling, launch argument construction, RPC readiness, shutdown, and status translation.

## 2. Config and App Orchestration

- [x] 2.1 Add download backend configuration, keep `default_download_dir` as the managed download destination, and support an optional `aria2c` binary path when aria2 is selected.
- [x] 2.2 Treat legacy `external_command` config as ignored compatibility data and remove the launcher-oriented service path as the supported workflow.
- [x] 2.3 Add shared models and app service methods for starting, reopening, and polling a single active managed download session.

## 3. TUI Managed Download Flow

- [x] 3.1 Add `d` key handling in list and detail views and reuse the target picker to start managed downloads from ready torrents.
- [x] 3.2 Add a dedicated managed download mode with periodic aria2 status refresh, active/completed/error rendering, and reopen behavior for an already-active session.
- [x] 3.3 Implement completion actions to open the downloaded file, reveal its directory, or delete the source torrent and return cleanly to the torrent workflow.
- [x] 3.4 Update footer text, modal labels, and status messages so `y` remains direct URL handoff, `d` is managed download, and stale launcher wording is removed.

## 4. Documentation and Verification

- [x] 4.1 Update README configuration and usage docs for managed aria2 startup, `aria2c` binary requirements, and the difference between URL handoff and managed download.
- [x] 4.2 Add tests for key bindings, single-active-download behavior, managed download state transitions, completion actions, and legacy config tolerance.
- [ ] 4.3 Manually verify: resolve a ready torrent target, start a managed download, observe progress, complete the download, use open and reveal actions, and confirm direct URL copy and show still work.
