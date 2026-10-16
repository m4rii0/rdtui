# TUI UX: visibility, stability and navigation

## Intent

Improve everyday use without redesigning the application or extending its download
backends. The user selected the first proposed batch: persistent download status,
stable layout, adaptive columns and consistent navigation. Implementation will be
inline on branch `ux`, with a validated commit for each implementation task.

## Constraints and non-goals

- Go 1.27.2; retain the versions in go.mod (Bubble Tea v2, Bubbles v2, Lip Gloss v2).
- No new dependencies, backend capabilities, config fields or API polling frequency.
- Keep UI copy in English, matching the existing application.
- Preserve request generations, download references, queue identities and runtime ownership.
- No preview pane, command palette, cancellation/pause controls, persistent jobs,
  file-tree redesign or new filtering language.
- Do not replay mutations or start transfers when navigating or rendering.
- Retain existing letter shortcuts unless explicitly changed below.

## 1. Persistent activity strip

The authenticated torrent list (including search), torrent detail, download and
bulk-download screens have a one-row activity strip. Popup backgrounds retain
their underlying strip; authentication and the filesystem browser are unchanged.
Idle state reads `No active download` so the strip never changes layout height.

Use existing observed state, not calls from View. An admitted active bulk queue
takes precedence over an individual transfer; a queue still starting also counts.
Show queue position and total, current filename if known, status, percentage and
speed when available. Distinguish URL resolution, waiting/overwrite, transfer and
completion/failure; do not invent a percentage before a transfer exists. A stale
transfer from an earlier item must never appear as the current queue item.
Keep the latest terminal result visible until replaced by new activity, with its
terminal status, not a misleading active indicator. Prefer the most recent
standalone transfer over a finished old queue. Do not include direct URLs/tokens.

`F6` reopens the current queue or standalone download without resolving, starting
or selecting anything. It is available in the five base screens above, including
search, but not through a modal/help overlay or a text-entry form. Idle F6 is a
no-op. Returning with Esc restores the previous base screen, including a search
query/focus if applicable. An existing overwrite decision must keep its current
identity and existing modal routing; this feature does not defer or auto-answer it.

Preserve current observation schedules outside transfer screens. At most one poll
per existing request boundary may be in flight. Poll errors retain the last known
snapshot and remain distinguishable from terminal transfer failures.

## 2. Stable list geometry and feedback

For terminal sizes at least 40 columns by 12 rows, the main/search screen reserves
fixed rows for header, table, search/filter, activity, feedback, divider and footer.
Search/filter and feedback reserve a row even when empty. Loading, success, errors,
flashes and activity updates must not change the table's height or move the footer.
The same geometry helper supplies the table row count and page-navigation distance.
Use six chrome rows (header, search, activity, feedback, divider, footer), no blank
header spacer, plus the existing two outer padding rows. Table height is height
minus eight; its title/header/divider consume three rows, leaving at least one
data row at 40x12. Outer horizontal padding consumes four cells.

Feedback precedence is error, flash, then status. Render one physical row, truncate
ANSI-aware by terminal cell width, and normalize embedded newlines. Feedback must
not overwrite the activity strip. Full error text remains inspectable via `?`:
the help overlay includes a wrapped, scrollable "Current error" section when an
error exists (up/down and PgUp/PgDn scroll; Esc closes). This is presentation only,
not an error history store. Help must remain bounded to the viewport.

Prioritize F6/activity access, help and navigation over secondary footer hints if
space is limited. For terminals smaller than 40x12, use bounded compact rendering:
keep the selected name/status and essential help/navigation hints where space
permits; no negative dimensions, panic or lines wider than the viewport. Full
detail/file-list scrolling outside the help overlay is not part of this batch.

## 3. Adaptive torrent columns

Compute breakpoints from the table's usable cell width after marking/scrollbar
space, not from raw terminal width. Preserve existing column indices for sorting
and fuzzy-match highlighting.

- At 80 usable columns or more: Status, %, Size, Name, Added.
- At 60–79 usable columns: Status, %, Size, Name.
- At 40–59 usable columns: Status, %, Name.
- Below 40 usable columns: Status and Name, with compact status labels.

Name receives all remaining width, with a target minimum of 16 cells whenever
possible; squeeze fixed columns before violating the viewport. Unicode names and
ANSI styles count by rendered width, not bytes. Hidden columns remain sortable.
Existing fuzzy matching still searches the full torrent representation; highlighting
must map to the correct visible field even when an intervening column is hidden.
The detail screen remains the existing way to inspect a full name.

## 4. Consistent navigation

### Moving through the torrent list

In main/search modes, Home/End move to the first/last visible torrent and
PgUp/PgDn move by the visible table data-row count, clamped at the edges. Empty
lists are no-ops. Preserve filter and batch marks. Use the normal detail-request
generation path once per actual selection change. No gg prefix state is added.
Search continues to accept ordinary characters, including q and o.

### Escape and quit

Esc closes the topmost modal/help first. In main, it exits batch mode before
clearing a kept filter; clearing preserves the selected torrent by ID where
possible. Search Esc cancels the search and restores the same torrent in the
unfiltered list. Existing Esc behavior for detail/download/bulk remains intact.

In main/detail/download/bulk base screens, q means quit, never clear filter. If a
nonterminal download or an admitted/starting bulk queue exists, show a confirmation
that exiting cancels active work. `y` confirms; `n`, Enter and Esc cancel. Canceling
restores the exact screen and preserves filters, marks, return destinations and
overwrite state; use a separate quit-dialog context, not the existing returnMode.
Do not stop the runtime before confirmation. With no active work, q quits directly.
Ctrl+C remains the existing immediate exit and SIGTERM is unchanged. In text-entry
forms and search, q remains input. Do not intercept keys through another modal.

### Discoverable ordering

`o` in the main list opens a sort picker with Status, Progress, Size, Date and Name.
Show the active key/direction. Arrow keys/j/k move, Enter applies using existing
sortByColumn semantics (same column toggles direction), Esc cancels. Preserve the
selected torrent by ID. Existing S/P/Z/D/N shortcuts remain available outside the
picker. The picker and quit dialog appear in the centralized shortcut/help system.

## Acceptance and validation

Automated tests cover render dimensions and stable row positions; breakpoints on
both sides of 40/60/80 usable columns; ANSI/Unicode names; marked rows/scrollbars;
hidden-column sort/highlighting; navigation at boundaries and with filters; literal
search input; active/terminal/stale queue and standalone activity; F6 without start
calls; quit cancellation/confirmation and unchanged Ctrl+C; help/error scrolling.
Use deterministic fake scheduling/services, never live Real-Debrid credentials.

Each task follows red/green tests, full `go test ./internal/tui -count=1 -timeout=45s`,
diff review and a commit. Final gates: `make verify` and `make test-race` with
`GOFLAGS='-timeout=45s -count=1'`, then `make build-all`. Also smoke-test the demo
in a PTY at 120x30, 80x24 and 40x12 if available; report any unperformed manual
validation rather than treating unit tests as visual proof.

## Author decisions requiring review

F6 for transfer access, o for ordering, the 40/60/80 cell breakpoints, Enter as a
safe cancel in the quit dialog, an error section in help rather than another
message-history screen, and no gg/G bindings are proposed implementation choices,
not preferences previously specified by the user.
