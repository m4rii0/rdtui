## Context

The TUI has a batch mode entered with `b`. Today it supports marking torrents, selecting all visible rows, clearing marks, deleting marked torrents with confirmation, copying URLs for marked torrents, and bulk downloading marked completed torrents.

Single-torrent actions outside batch mode include file selection with `s`, URL copy with `y`, managed download with `d`, delete with `x`, detail view with `enter`, and import/add flows. Local `.torrent` import already supports selecting multiple files in the browser, so it is already bulk-capable in its own context.

The largest gap in the torrent-list bulk operation set is `s`: users can select files for one `waiting_files_selection` torrent, but cannot apply that flow across several marked torrents. This change broadens the proposal to "bulk operations" while keeping implementation focused on the next missing operation, bulk file selection.

## Goals / Non-Goals

**Goals:**
- Make batch mode feel like the consistent home for torrent bulk operations.
- Enable batch-mode `s` for marked torrents that need file selection.
- Respect the current visible table order, including active search filters, when deciding prompt order.
- Skip marked torrents that are not in a file-selection status instead of blocking the whole operation.
- Reuse the single-torrent file-selection defaults and controls for each eligible torrent.
- Submit selections sequentially and report successful, failed, and skipped torrents.
- Allow cancellation during setup without submitting any file selections.

**Non-Goals:**
- Parallel `SelectFiles` requests.
- Persisting bulk operation queues across restarts.
- Changing Real-Debrid file-selection semantics or the service interface.
- Automatically selecting files for every torrent without user confirmation.
- Reworking all existing batch operations in this change.
- Adding bulk magnet or remote URL entry in this change.

## Bulk Operations Inventory

| Operation | Current Status | Notes |
| --- | --- | --- |
| Mark/unmark torrents | Exists | `space`, `ctrl+a`, `ctrl+d`; selection uses visible filtered table. |
| Delete marked torrents | Exists | `x`; destructive confirmation already supports multiple IDs. |
| Copy URLs for marked torrents | Exists | `y`; skips not-ready torrents during copy. |
| Download marked completed torrents | Exists | `d`; full setup, queue, summary, and cleanup flow. |
| Select files for marked waiting torrents | Missing | This is the target operation for the change. |
| Import multiple local `.torrent` files | Exists outside batch mode | File browser already supports multi-select and batch import. |
| Add multiple magnet links or remote URLs | Missing but lower priority | Could be separate input/import capability; not tied to marked torrents. |
| Refresh marked torrents only | Missing but low value | Current refresh is global and simpler. |
| Bulk open/reveal completed local files | Not applicable now | Managed download state tracks one active download, not a persisted library. |

## Decisions

### Keep implementation focused on bulk file selection

The broadened change name frames the long-term concept as bulk operations, but implementation should add only the missing torrent-list operation that is ready and well-scoped: batch-mode `s`.

Rationale: delete, copy, and download already exist. Bulk file selection fills the practical gap for users importing or adding several torrents that pause at `waiting_files_selection`.

Alternative considered: introduce a general bulk-operations framework first. That risks overbuilding before there are enough divergent operations to justify a framework.

### Add a separate bulk file-selection state

Bulk file selection will use dedicated TUI state instead of reusing `bulkDownloadState` directly.

Rationale: bulk download plans are based on downloadable targets for already-downloaded torrents, while bulk file selection plans are based on source torrent files and submit file IDs to Real-Debrid. Sharing state would mix two different concepts and make cleanup/result handling harder to reason about.

Alternative considered: generalize `bulkDownloadState` into a shared bulk wizard. That would add abstraction before there is a second fully aligned use case and risks making the existing download flow harder to maintain.

### Skip ineligible marked torrents

When batch-mode `s` starts, the TUI will partition marked visible torrents into eligible and skipped sets. Eligible torrents are those whose current list status is `waiting_files_selection` or `magnet_conversion`; all others are skipped and included in the final result summary.

Rationale: this matches the requested behavior and supports mixed selections after filtering or selecting all visible rows. It avoids forcing users to carefully unmark completed/downloading torrents before selecting files for the torrents that still need attention.

Alternative considered: require every marked torrent to be eligible. That is simpler but too brittle for mixed bulk selections.

### Collect selections before submitting

The setup flow will load details for eligible torrents, show one file-selection prompt per eligible torrent, and only start `SelectFiles` submissions after all prompts have been confirmed.

Rationale: cancellation during setup should have no side effects. This mirrors the safety property of the bulk download setup flow while keeping the per-torrent popup familiar.

Alternative considered: submit each torrent immediately after its popup is confirmed. That would feel faster but makes cancellation ambiguous and can leave a half-applied operation before the user finishes reviewing the batch.

### Continue after per-torrent failures

The execution step will call `SelectFiles` sequentially for each planned torrent and continue after individual failures.

Rationale: a transient or torrent-specific failure should not waste the rest of the confirmed batch. This matches existing batch operation behavior.

Alternative considered: stop on first failure. That is simpler but makes large batches less useful and gives users less control over partial success.

## Risks / Trade-offs

- Broad change name can imply more scope than intended -> keep proposal, specs, and tasks explicit that implementation adds bulk file selection as the next missing bulk operation.
- Bulk setup may feel long for many multi-file torrents -> keep prompts one torrent at a time and preserve visible order so progress is predictable.
- List status can be stale between marking and execution -> rely on `SelectFiles` errors for stale/ineligible torrents loaded as eligible, record failures, and refresh afterward.
- Skipping ineligible torrents could hide a mistaken selection -> include skipped counts and skipped torrent names in the result feedback where space allows.
- Separate bulk state increases model size -> keep it narrowly focused on torrent ID, name, files, selected file IDs, cursor, and results.
