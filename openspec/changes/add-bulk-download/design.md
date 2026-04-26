## Context

The TUI already supports batch mode for marking torrents and running sequential batch delete/copy operations. Managed local downloads are currently single-active-download oriented: the service tracks one active managed download and reopens it if another download is requested while it is still active. The bulk download feature should reuse that constraint instead of introducing parallel downloads or a downloader-level queue.

Bulk download is primarily a TUI orchestration feature. It needs to collect user intent, resolve each selected torrent/file into direct URLs, run one managed download at a time, record per-file results, and expose safe cleanup once the queue finishes.

## Goals / Non-Goals

**Goals:**
- Enable batch-mode `d` for multiple marked torrents whose status is `downloaded`.
- Preserve current table order as the default queue order while allowing the user to reorder before starting.
- Ask for file choices one torrent at a time when a ready torrent has multiple downloadable files.
- Require a final confirmation popup before queue execution starts.
- Continue after per-file failures and show a summary when the queue finishes.
- Provide a cleanup popup where successful source torrents are preselected, failed or partial torrents are unselected, and every row remains manually toggleable.

**Non-Goals:**
- Parallel downloads.
- A persistent download queue across application restarts.
- New download backend behavior or external dependencies.
- Automatic deletion of source torrents without explicit confirmation.

## Decisions

### TUI-owned sequential queue

The TUI will own the bulk queue state and advance it one item at a time after the current managed download reaches a terminal state.

Rationale: this matches the existing single-active-download service model and avoids changing the `downloadManager` interface or backend implementations. It also keeps ordering, prompts, and cleanup tied to user-visible state.

Alternative considered: add a multi-download queue to `Service` or the download backends. This would be heavier, would complicate both aria2 and direct implementations, and is unnecessary for sequential behavior.

### Separate setup, execution, and cleanup states

Bulk download will use distinct model state for setup prompts, active queue execution, finished summary, and cleanup confirmation.

Rationale: setup is interactive and can be cancelled without side effects; execution is asynchronous and must survive per-item failures; cleanup is destructive and needs its own confirmation.

### Default order follows the current view

The initial queue order will come from the current visible torrent order, which already reflects sorting and filtering.

Rationale: users expect marked rows to process in the same order they are seeing. This also matches existing `batchSelectedIDs()` ordering behavior.

### File choices are requested per torrent

For each selected torrent, the setup flow will inspect available download targets. Single-target torrents can be accepted automatically. Multi-target torrents will show a file-choice popup before the final confirmation.

Rationale: asking one torrent at a time keeps the popup understandable and avoids a large mixed list where file names from different torrents are hard to reason about.

### Continue after failures

Failures resolving a target or completing a managed download will be recorded on the corresponding file result, then the queue will advance to the next file.

Rationale: one bad link or transient download failure should not waste the rest of a prepared batch.

### Cleanup defaults are conservative but manually overrideable

The finished queue will enable `x` to open a cleanup popup. Torrents whose requested files all completed will be preselected. Failed, partial, skipped, or cancelled torrents will be unselected by default, but users can manually toggle them.

Rationale: this prevents accidental deletion of source torrents with missing files while keeping power-user control available.

## Risks / Trade-offs

- Queue state becomes larger in the TUI model -> Keep bulk structs focused on setup selections, active item, and compact results.
- Multiple popups in setup can feel long for large batches -> Auto-select single-target torrents and only prompt when a torrent has multiple file targets.
- Existing-file prompts during a queue can interrupt execution -> Treat each prompt as part of the current item and continue the queue after the user decides.
- Source torrent deletion is dangerous after partial failure -> Preselect only fully successful torrents and clearly label failed or partial rows in cleanup.
- Downloads are sequential and slower than parallel -> This is intentional for predictable resource use and to align with the existing single-active-download model.
