## 1. Project Skeleton

- [x] 1.1 Create the Go application entrypoint and base package layout for auth, config, Real-Debrid client, download handoff, app orchestration, and TUI code.
- [x] 1.2 Add the initial dependencies for Bubble Tea, styling, configuration, and any clipboard/process-launch helpers needed for the terminal app.
- [x] 1.3 Define shared models for authentication state, torrents, torrent files, and download handoff results.

## 2. Configuration and Authentication

- [x] 2.1 Implement config and data path resolution for persisted settings and stored credentials.
- [x] 2.2 Implement private API token loading, validation, and precedence behavior.
- [x] 2.3 Implement Real-Debrid device auth start, polling, credential persistence, and token refresh behavior.
- [x] 2.4 Build the first-run authentication flow and authenticated session bootstrap logic.

## 3. Real-Debrid Client

- [x] 3.1 Implement typed client methods for user validation, torrent listing, and torrent detail retrieval.
- [x] 3.2 Implement add-torrent methods for magnet links, local `.torrent` file uploads, batched local `.torrent` uploads, and remote `.torrent` URL ingestion.
- [x] 3.3 Implement file-selection submission, torrent deletion, and link unrestriction methods with the correct request formats.
- [x] 3.4 Add client tests with mocked Real-Debrid responses for success, auth failure, and rate-limit/error cases.

## 4. Torrent Workbench UI

- [x] 4.1 Build the master-detail TUI layout for torrent list and selected torrent detail.
- [x] 4.2 Render torrent status, progress, metadata, files, and inline error states in the appropriate panes.
- [x] 4.3 Add periodic refresh plus manual refresh behavior with conservative polling defaults.
- [x] 4.4 Gate visible actions by torrent status so waiting and ready torrents expose the correct workflows.

## 5. Add and Select Torrent Content

- [x] 5.1 Add the UI flow for creating a torrent from a magnet link.
- [x] 5.2 Add a filesystem-browser UI flow for creating torrents from one or more local `.torrent` files.
- [x] 5.3 Add the UI flow for creating a torrent from a remote `.torrent` URL.
- [x] 5.4 Implement batch local-import result handling for full success, partial failure, and validation errors.
- [x] 5.5 Implement the waiting-files-selection flow with largest-file default selection, confirmation, and manual adjustment.

## 6. Download Handoff and Delete Flows

- [x] 6.1 Implement the ready-torrent handoff flow for choosing a downloadable target and resolving a direct URL.
- [x] 6.2 Implement URL presentation and clipboard-oriented handoff behavior when the user does not launch a downloader.
- [x] 6.3 Implement configurable external downloader command rendering and process launch without local transfer monitoring.
- [x] 6.4 Implement delete confirmation, deletion execution, and post-delete refresh behavior.

## 7. Documentation and Verification

- [x] 7.1 Add focused tests for selection heuristics, auth precedence, and downloader command templating.
- [x] 7.2 Verify the full happy path manually: authenticate, add torrent, select files, hand off a download, and delete a torrent.
- [x] 7.3 Document configuration, authentication choices, and the torrent-first workflow in the project README.

## 8. UI Rework: k9s-style Navigation

- [ ] 8.1 Replace the two-pane master-detail layout with a single full-width torrent table in the main view.
- [ ] 8.2 Add a new `modeDetail` mode with a full-screen torrent detail view. Enter from the list opens this view; ESC returns to the list.
- [ ] 8.3 Rebind Enter in the main view to open the detail view instead of sorting by column.
- [ ] 8.4 Remove column selection (h/l navigation, `selectedColumn` field). Remove the `moveColumnSelection` method.
- [ ] 8.5 Implement direct Shift+letter sorting shortcuts: `S` Status, `P` Progress, `Z` Size, `D` Date, `N` Name. Each toggles direction when pressed again.
- [ ] 8.6 Add a context-sensitive footer that is always visible. List view footer: `↑↓ j/k enter=view S/P/Z/D/N=sort r=refresh m/u/i y/x q=quit`. Detail view footer: `esc=back s=select y=copy x=delete r=refresh`.
- [ ] 8.7 Make torrent actions (s=select files, y=copy URL, x=delete) available from both the list view and the detail view. Rebind delete to `x` (was `d`).
- [ ] 8.8 Update `renderTorrentList` to use full terminal width. Remove `torrentListPaneWidth` and two-pane layout helpers.
- [ ] 8.9 Update `renderTableHeader` to remove column selection highlighting. Keep sort direction indicators (↑/↓).
- [ ] 8.10 Update tests for new key bindings, mode transitions (Enter→detail, ESC→list), and direct sort shortcuts.
