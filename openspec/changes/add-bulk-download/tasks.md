## 1. Batch Entry And State

- [ ] 1.1 Add bulk download setup, queue, result, and cleanup state to the TUI model.
- [ ] 1.2 Add a batch-mode `d` shortcut that is visible but dimmed unless at least two marked torrents are `downloaded`.
- [ ] 1.3 Start bulk setup from marked torrent IDs in current visible table order.

## 2. Setup Popups

- [ ] 2.1 Add a bulk order popup with navigation, reordering, confirm, and cancel behavior.
- [ ] 2.2 Load torrent details for ordered torrents and derive downloadable targets from existing download-target logic.
- [ ] 2.3 Auto-select single-target torrents and show one file-choice popup per multi-target torrent.
- [ ] 2.4 Reject empty file-choice confirmation with an inline error.
- [ ] 2.5 Add a final confirmation popup summarizing torrent count, file count, and order before downloads start.

## 3. Queue Execution

- [ ] 3.1 Start exactly one managed download at a time from the confirmed bulk plan.
- [ ] 3.2 Reuse existing direct-link resolution, existing-file prompt, and managed download start flow for each queued file.
- [ ] 3.3 Advance to the next queued file after success, failure, or skipped existing-file confirmation.
- [ ] 3.4 Record per-file outcomes and aggregate per-torrent complete, partial, failed, and skipped status.
- [ ] 3.5 Render a bulk download progress and finished summary view.

## 4. Cleanup

- [ ] 4.1 Enable `x` from the finished bulk summary to open source-torrent cleanup.
- [ ] 4.2 Preselect fully successful torrents and leave failed, partial, skipped, or cancelled torrents unselected.
- [ ] 4.3 Allow every cleanup row to be manually toggled and show a warning when incomplete torrents are selected.
- [ ] 4.4 Delete selected source torrents with confirmation and report cleanup successes and failures.

## 5. Verification And Documentation

- [ ] 5.1 Add model tests for bulk eligibility, ordering, file-choice flow, final confirmation, queue continuation, and cleanup defaults.
- [ ] 5.2 Add rendering tests for new popups, summary state, dimmed shortcuts, and cleanup warning text.
- [ ] 5.3 Update README usage docs for batch-mode bulk download and cleanup behavior.
- [ ] 5.4 Run `go test ./...` and fix failures.
