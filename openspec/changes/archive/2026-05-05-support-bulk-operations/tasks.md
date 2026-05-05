## 1. Batch Entry And Eligibility

- [x] 1.1 Add batch-mode `s` shortcut visibility and help text for bulk file selection.
- [x] 1.2 Add eligibility logic that enables bulk file selection when at least one marked visible torrent is in a file-selection status.
- [x] 1.3 Build the initial bulk file-selection plan from marked visible torrents in current filtered/sorted table order.
- [x] 1.4 Record marked visible torrents that are not in a file-selection status as skipped.

## 2. Setup State And Prompts

- [x] 2.1 Add dedicated TUI state for bulk file-selection plans, current prompt cursor, selected file IDs, skipped torrents, and results.
- [x] 2.2 Load torrent details for each eligible planned torrent before prompting.
- [x] 2.3 Reuse the existing largest-file default when initializing each torrent's file selection.
- [x] 2.4 Render a bulk file-selection popup that identifies the current torrent and supports move, toggle, select all, clear, confirm, and cancel controls.
- [x] 2.5 Reject confirmation for a torrent when no files are selected.
- [x] 2.6 Return to batch mode with marked torrents unchanged when the user cancels setup before submissions begin.

## 3. Sequential Submission And Results

- [x] 3.1 Submit confirmed file selections one torrent at a time using the existing `SelectFiles` service API.
- [x] 3.2 Continue submitting remaining planned torrents after a per-torrent failure.
- [x] 3.3 Track successful, failed, and skipped torrent outcomes.
- [x] 3.4 Refresh the torrent list after the bulk file-selection operation completes.
- [x] 3.5 Report result counts to the user, including skipped ineligible torrents.
- [x] 3.6 Clear batch mode and marked selection only after submissions complete.

## 4. Bulk Operations UX And Documentation

- [x] 4.1 Update README batch-mode docs to frame batch mode as bulk operations and include `s` bulk file selection.
- [x] 4.2 Document skipped ineligible behavior for mixed marked selections.
- [x] 4.3 Keep the task scope limited to bulk file selection; leave bulk magnet/URL entry and selected-only refresh for future changes.

## 5. Verification

- [x] 5.1 Add model tests for shortcut eligibility, skipped ineligible torrents, filtered visible order, cancellation, empty-selection rejection, sequential submission, and partial failures.
- [x] 5.2 Add rendering tests for the bulk file-selection prompt, footer/help shortcuts, and result messaging.
- [x] 5.3 Run `go test ./...` and fix any failures.
- [x] 5.4 Run GitNexus change detection before commit to confirm the affected scope is expected.
