## Why

Users can already mark multiple torrents for batch delete or URL copy, but downloading still requires handling ready torrents one at a time. Bulk download lets users turn a selected set of completed torrents into a controlled sequential download queue with explicit file choices, confirmation, failure reporting, and safe cleanup.

## What Changes

- Enable the `d` download action in batch mode when multiple marked torrents are ready (`downloaded`).
- Add a bulk download setup flow that preserves current view order by default, lets the user adjust download order, and asks which files to download for torrents with multiple downloadable files.
- Add a final confirmation popup before starting the queue.
- Download selected files sequentially using the existing managed download backend, continuing after per-file failures.
- Show a completion summary with successful, failed, partial, and skipped results.
- After the queue finishes, allow source-torrent cleanup with a popup that preselects fully successful torrents and leaves failed or partial torrents unselected while allowing manual toggles.

## Capabilities

### New Capabilities
- `bulk-downloads`: Batch-mode bulk download setup, sequential queue execution, result summary, and cleanup behavior for multiple ready torrents.

### Modified Capabilities
- `torrent-management`: Batch mode gains a state-gated managed download action for marked ready torrents.

## Impact

- Affects TUI batch shortcuts, popup modes, model state, queue orchestration, managed download status polling, and delete cleanup flow.
- Reuses existing Real-Debrid torrent detail/link resolution APIs and the configured managed download backend.
- No new external dependencies or breaking changes are expected.
