## Why

Batch mode already supports marking multiple torrents, deleting them, copying URLs, and downloading completed torrents in bulk, but the operation set is uneven. The most immediate missing bulk operation is file selection for torrents waiting on Real-Debrid file choices, which currently requires repeating the same `s` flow one torrent at a time.

## What Changes

- Treat batch mode as the home for bulk torrent operations and document the current operation set.
- Enable the `s` select-files action in batch mode when at least one marked torrent is eligible for Real-Debrid file selection.
- Start a bulk file-selection setup flow from marked torrents in the current visible table order, respecting active filters.
- Skip marked torrents that are not eligible for file selection instead of blocking the operation.
- Show one file-selection prompt per eligible torrent, using the existing largest-file default and select-all/clear controls.
- Submit confirmed file selections sequentially, continue after per-torrent failures, and report successful, failed, and skipped results.

## Capabilities

### New Capabilities

- None.

### Modified Capabilities

- `torrent-management`: Batch mode is expanded as bulk operations mode by adding bulk file selection for marked torrents, with ineligible marked torrents skipped.

## Impact

- Affects TUI batch shortcuts, model state, file-selection popup flow, Real-Debrid `SelectFiles` orchestration, tests, and README usage docs.
- Reuses existing torrent detail loading, default file-selection logic, visible-order batch selection, and `SelectFiles` service API.
- No external dependencies, persistent data changes, or breaking changes are expected.
