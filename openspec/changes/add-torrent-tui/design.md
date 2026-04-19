## Context

This repository is currently a greenfield OpenSpec project with no application code yet. The first usable slice is a Go terminal UI for Real-Debrid that focuses on torrents rather than the broader hoster-link feature set.

The application needs to cover three cross-cutting concerns from the start:
- authentication against Real-Debrid
- a torrent-centric terminal workflow with k9s-inspired stack navigation
- local download handoff without becoming a full download manager

Real-Debrid's torrent flow has a few important constraints that shape the design:
- torrents may pause in `waiting_files_selection` before any generated links exist
- generated torrent links still need `unrestrict/link` to become a direct download URL in many cases
- the API rate limits requests, so polling behavior must stay conservative
- the API docs do not clearly define a durable mapping contract between returned `files` and `links`, so the UI must avoid hiding ambiguity when multiple files are selected

## Goals / Non-Goals

**Goals:**
- Provide a k9s-inspired terminal UI with stack navigation: a full-width torrent table as the main view, Enter to drill into a full-screen detail view, ESC to go back.
- Support both Real-Debrid device auth and private API token auth.
- Let users add torrents from magnet links, one or more local `.torrent` files selected from a filesystem browser, and remote `.torrent` URLs.
- Let users resolve `waiting_files_selection` torrents with a largest-file default and explicit confirmation.
- Let users turn a ready torrent link into a direct URL, then copy it or hand it off to a configured external downloader.
- Provide direct keyboard sorting for all columns via Shift+letter shortcuts with toggle direction.
- Show a context-sensitive footer with shortcut hints at all times.
- Allow torrent actions (select files, copy URL, launch downloader, delete) from both the list and detail views.
- Keep the initial architecture small and testable so the first implementation can land without excess framework code.

**Non-Goals:**
- Monitoring local file transfers after the handoff to an external downloader.
- Supporting non-torrent Real-Debrid hoster workflows in this change.
- Implementing queue management, pause/resume, history, or notifications.
- Solving every multi-file torrent mapping edge case beyond surfacing it clearly and letting the user choose conservatively.

## Decisions

### 1. Build a k9s-inspired stack-navigation Bubble Tea TUI

The application will use a full-width torrent table as the main view. Pressing Enter on a row opens a full-screen detail view for that torrent. Pressing ESC returns to the list. This replaces the previous master-detail two-pane layout.

Why this decision:
- it matches the k9s terminal UI pattern that power users are familiar with
- a full-width table shows more information per row and uses screen real estate better
- the detail view gets the full screen to show files, links, and actions clearly
- stack navigation (push/pop) is simpler to reason about than concurrent two-pane state
- it separates the "browse" and "act" workflows into distinct, focused views

Alternatives considered:
- master-detail two-pane layout: was the previous design; wastes horizontal space on small terminals and splits attention
- screen-per-task navigation: simpler internally, but slower for browsing and repeated actions
- prompt-driven command UI: faster to build, but weaker for ongoing torrent inspection and less coherent as a TUI

### 2. Use direct Shift+letter sorting shortcuts

Each table column is sortable by pressing Shift plus the column's key letter: `S` Status, `P` Progress, `Z` Size, `D` Date, `N` Name. Pressing the same shortcut again toggles the sort direction. There is no column selection cursor — sorting is always a single keypress.

Why this decision:
- it eliminates the need for h/l column navigation and Enter-to-sort, which is now repurposed to open the detail view
- direct shortcuts are faster than a two-step column-select-then-sort workflow
- each column gets a unique, memorable key

Alternatives considered:
- column header selection with arrow keys then Enter: two steps, conflicts with Enter-for-detail
- clicking column headers: not applicable in a terminal

### 3. Show a context-sensitive footer at all times

The footer displays keyboard shortcut hints that change based on the current view (list vs detail vs modal). It is always visible at the bottom of the screen.

Why this decision:
- k9s uses this pattern and it works well for discoverability
- users should never have to guess which keys are available
- the footer takes only one line and provides high value

### 4. Allow torrent actions from both list and detail views

Torrent actions (select files, copy URL, launch downloader, delete) are available from both the main list view and the detail view. The `x` key always triggers delete with confirmation.

Why this decision:
- users should be able to act on a torrent without drilling into detail if they already know what they want
- the detail view is the natural place for actions that require seeing file-level information
- `x` for delete is memorable and consistent across both views

### 5. Separate orchestration from API and UI concerns

The codebase will be split into small packages with clear responsibilities:
- `internal/auth` for device flow, token validation, refresh, and auth selection
- `internal/realdebrid` for typed API client methods
- `internal/download` for copy/show and external command rendering/launch
- `internal/tui` for Bubble Tea state, rendering, and interaction handling
- `internal/app` for coordinating services and translating user actions into state transitions
- `internal/config` for config and credential persistence

Why this decision:
- auth, torrent state, and download handoff each have different failure modes
- it keeps API formatting logic out of the TUI model
- it makes the non-UI behavior testable with mocks and local fixtures

Alternative considered:
- placing all logic inside the TUI model: smaller initially, but harder to test and more likely to tangle HTTP, persistence, and UI concerns

### 6. Support both auth modes with explicit precedence

The app will support:
- stored user-bound device-auth credentials
- an explicit private API token

Credential precedence will be:
1. explicit configured private token
2. stored device-auth credentials
3. first-run auth prompt

Why this decision:
- the user explicitly wants both
- private tokens are convenient for advanced users and scripts
- device auth is the safer default for an open-source client

Alternative considered:
- device auth only: cleaner, but rejects a valid user preference and makes manual setup harder

### 7. Stop v1 at download handoff

The application will generate a direct URL and either present it to the user or launch a configured external downloader. It will not attach to or monitor the local download.

Why this decision:
- it keeps the first version centered on the Real-Debrid torrent lifecycle
- it avoids coupling the app to aria2-specific process and progress handling too early
- it still produces a useful outcome for users immediately

Alternatives considered:
- embed aria2 progress monitoring: attractive, but it doubles the product surface in the first iteration
- copy-only flow: simpler, but too limited for a daily-use tool

### 8. Use the largest file as the default file-selection heuristic

When a torrent enters `waiting_files_selection`, the app will preselect the largest file and open a confirmation flow that allows adjustment before submission.

Why this decision:
- it matches the desired default behavior discussed during exploration
- it is a practical heuristic for media-heavy torrents
- it keeps the UI useful even before richer filtering exists

Alternative considered:
- select all automatically: simpler, but too blunt and often wrong for mixed-content torrents

### 9. Use conservative polling with manual refresh

The TUI will poll the torrent list and selected torrent detail periodically, while also allowing manual refresh. Polling intervals should stay conservative and back off on rate-limit responses.

Why this decision:
- Real-Debrid state changes asynchronously, so a static UI would feel stale
- conservative polling avoids wasting rate-limit budget

Alternative considered:
- aggressive short-interval polling: more responsive, but unnecessary risk against API limits

### 10. Use a filesystem browser for local `.torrent` import and support batch upload

The local `.torrent` add flow will open a filesystem browser inside the TUI so the user can navigate directories and choose one or more `.torrent` files in a single import action.

Selected files will be uploaded to Real-Debrid as a batch, processed sequentially, and reported back with per-file success or failure feedback.

Why this decision:
- the user explicitly wants local browsing instead of a path-only workflow
- bulk import is a natural fit for users with folders full of `.torrent` files
- sequential processing keeps the UI and error handling simple while still enabling multi-file import

Alternatives considered:
- raw path entry only: simpler, but weaker and less discoverable in a TUI
- parallel upload of selected files: faster in theory, but adds unnecessary complexity around rate limits and partial failures in the first iteration

## Risks / Trade-offs

- [File/link ambiguity for multi-file torrents] -> Prefer single-file default selection, make the chosen target explicit in the UI, and avoid claiming certainty when multiple generated links exist.
- [Auth storage complexity from supporting two modes] -> Keep precedence explicit, validate credentials early, and separate stored OAuth credentials from token-based config.
- [External command quoting and portability issues] -> Use argv-style command templates instead of shell strings and keep placeholder substitution minimal and explicit.
- [Polling may hit rate limits] -> Use modest intervals, manual refresh, and backoff behavior after 429 responses.
- [Remote `.torrent` URLs add another failure mode] -> Treat remote fetch failure as a first-class user-facing error and keep the fetch/upload flow isolated in the Real-Debrid client layer.
- [Batch local import may partially succeed] -> Process selected files sequentially and show a per-file result summary so successful uploads are preserved when later files fail.
- [Stack navigation may confuse users accustomed to master-detail] -> The context-sensitive footer always shows available actions and ESC is universally available to go back.

## Migration Plan

There is no existing application to migrate. The change can land as a new Go application with local config and credential files created on first use.

Rollout steps:
1. add the Go module dependencies and application skeleton
2. implement auth and the Real-Debrid client
3. build the k9s-style full-width torrent table and detail view with stack navigation
4. add direct Shift+letter sorting shortcuts and context-sensitive footer
5. add file-selection, add-torrent, batch local import, handoff, and delete flows
6. document configuration and supported downloader templates

Rollback strategy:
- remove the new application code and ignore any local config files created in user directories
- no server-side schema or data migration is involved

## Open Questions

- Which binary name should the application use in the first release?
- Should the app ship with a default copy-to-clipboard behavior when no external downloader is configured, or require the user to choose explicitly each time?
- How should the UI present multi-link torrents when Real-Debrid returns several generated links but the mapping to selected files is only best-effort?
