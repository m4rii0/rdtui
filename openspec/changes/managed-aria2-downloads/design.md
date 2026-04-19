## Context

`rdtui` already handles the Real-Debrid torrent lifecycle through authentication, torrent browsing, file selection, and direct URL resolution. The current flow stops at URL handoff: users can copy or show a resolved URL, and there is leftover code for an external-launch path, but there is no complete in-app local download workflow.

The requested change is to let `rdtui` start and manage its own `aria2c` process instead of depending on a pre-running aria2 RPC daemon or a generic external downloader command. The `venaqui` reference demonstrates that aria2 RPC is a good fit for progress polling, completion state, and post-download actions.

There are a few constraints that shape the design:
- the existing torrent-first TUI should remain the primary workflow
- direct URL copy and show behavior should remain available for lightweight handoff
- local configuration already exists, so config changes should stay small and predictable
- the managed downloader must be owned by the app, not exposed as a user-managed RPC service

## Goals / Non-Goals

**Goals:**
- Start a local download directly from a resolved ready-torrent target.
- Have `rdtui` launch and own `aria2c` for the current app session.
- Show download progress, speed, ETA, completion, and failure state in the TUI.
- Keep direct URL copy and show behavior as a separate workflow.
- Keep the implementation small, testable, and compatible with the current torrent workbench.

**Non-Goals:**
- Supporting multiple concurrently monitored downloads in the first change.
- Building a general-purpose download manager with queues, history, pause/resume, or retries UI.
- Requiring users to manually start an aria2 RPC daemon.
- Preserving the current external downloader command workflow as a first-class feature.

## Decisions

### 1. Use a session-owned `aria2c` process

`rdtui` will start `aria2c` lazily when the user begins a managed download, reuse that process for the rest of the app session, and shut it down when the application exits or no longer needs the managed downloader.

Why this decision:
- it removes the setup burden of requiring a pre-existing aria2 RPC daemon
- it keeps downloader state scoped to the app session
- it gives the TUI a stable component to poll for progress and completion

Alternatives considered:
- require a pre-running aria2 RPC daemon: smaller implementation, but worse user experience and more setup friction
- keep a generic external launcher only: does not provide managed progress or completion inside the app

### 2. Launch aria2 with app-owned RPC settings on loopback only

The managed process will be launched without daemon mode and with explicit RPC settings owned by `rdtui`, including `--enable-rpc`, `--rpc-listen-all=false`, a per-session `--rpc-secret`, `--no-conf=true`, and an app-selected unprivileged `--rpc-listen-port`. The app will wait for RPC readiness before submitting downloads and will stop the process with `aria2.shutdown`, falling back to process termination if graceful shutdown fails.

Why this decision:
- aria2's RPC model is the cleanest way to add downloads, poll status, and shut the process down cleanly
- loopback-only RPC and a generated secret keep the managed process private to the current session
- owning the launch flags prevents user config from breaking the RPC contract that the app depends on

Alternatives considered:
- parse `aria2c` stdout: brittle and weaker than RPC for status tracking
- expose RPC port and secret as user config: flexible, but it gives away the invariants the app needs to manage the child process safely

### 3. Wrap process lifecycle and RPC access behind an internal aria2 package

The implementation will introduce a dedicated internal package that owns three concerns together:
- locating and launching the `aria2c` binary
- creating and reusing the session RPC client
- mapping aria2 status into app-facing download session data

This package may use `github.com/siku2/arigo` for JSON-RPC calls, but the rest of the app will depend only on the local wrapper rather than the third-party API directly.

Why this decision:
- it keeps process and RPC details out of the TUI model
- it makes lifecycle and status behavior testable in isolation
- it leaves room to change the underlying RPC client without rewriting UI or service logic

Alternative considered:
- place RPC and process logic directly in `internal/app` or `internal/tui`: smaller at first, but harder to test and easier to tangle with UI state

### 4. Add a dedicated managed download action and view

The torrent workbench will keep `y` for direct URL handoff and `x` for delete. It will add `d` as the explicit managed download action from both the list view and the detail view.

The existing target picker will be reused for managed download. After the user chooses a target, the app will resolve the direct URL, start or reuse the managed aria2 session, submit the download, and enter a dedicated full-screen download view.

The download view will show:
- filename
- aria2 status
- progress percentage
- transferred and total bytes
- download speed
- ETA when available
- completion or error state

On completion, the view will offer open-file, reveal-directory, and delete-torrent actions.

Why this decision:
- it keeps the new behavior explicit and easy to discover
- it avoids overloading URL copy behavior with downloader side effects
- it matches the minimal successful shape from the `venaqui` reference without importing its whole app structure

Alternatives considered:
- replace `y` with download behavior: too surprising and removes a useful lightweight workflow
- show download status inline in the torrent table: weaker focus and more state complexity for the first version

### 5. Support one active managed download session at a time

The first implementation will track at most one active managed download at a time. If the user requests another managed download while one is already active, the app will reopen the active download session instead of starting a second one.

Why this decision:
- it keeps state management small while still delivering the core feature
- it avoids designing a downloads list, queue model, and multi-session navigation in the same change
- it keeps the app-managed `aria2c` lifecycle straightforward in v1

Alternative considered:
- allow multiple active downloads immediately: useful, but it expands the UI and state surface significantly

### 6. Keep config minimal while making aria2 optional

The managed flow will continue using `default_download_dir` and add a small optional override for the `aria2c` binary path, but the actual download backend will be configurable. `rdtui` will support:
- `download_backend = "aria2"` to use the managed `aria2c` process
- `download_backend = "direct"` to fall back to a built-in HTTP download without `aria2c`

The app will no longer treat `external_command` as the supported download workflow.

Existing `external_command` data will be ignored by the new flow and will naturally disappear the next time config is saved without that field.

Why this decision:
- the app still owns downloader startup directly when aria2 is selected, so a generic launcher is no longer the main path
- a small backend switch is enough to make aria2 optional without exposing app-critical RPC settings accidentally
- ignoring the legacy field is sufficient because the config is local and JSON decoding already tolerates unknown fields

Alternative considered:
- keep both managed aria2 downloads and a separate external launcher UI: possible, but it adds another user-facing path without being required for the requested outcome

## Risks / Trade-offs

- [Missing `aria2c` binary] -> Validate availability before or during managed-download startup and show a clear remediation error.
- [Port selection race for the RPC listener] -> Select an available local port and retry startup with a new port if aria2 cannot bind the chosen one.
- [Child process outlives the parent after abnormal exit] -> Attempt graceful RPC shutdown on normal exit and fall back to killing the child process when the app still owns it.
- [Single-download limit may frustrate power users] -> Reopen the active session predictably and leave multi-download support for a later dedicated change.
- [Removing the external launcher changes behavior for users with existing config] -> Keep URL copy and show behavior, document the new managed flow, and ignore legacy launcher config safely.

## Migration Plan

1. Add the managed aria2 package, dependency, and config surface for `aria2c` path resolution.
2. Replace the current launcher-oriented download path with managed download orchestration in `internal/app` and `internal/tui`.
3. Add the dedicated download view, active-session tracking, and completion actions.
4. Update README and footer text to document `d` for managed download and `y` for direct URL handoff.
5. Stop using `external_command` as an active workflow and treat it as ignored legacy config.

Rollback strategy:
- remove the managed aria2 integration and restore the previous handoff-only workflow
- leave existing downloaded files untouched because they are local user data, not app-owned state

## Open Questions

None for this change. The scope is intentionally limited to one managed download session and app-owned aria2 lifecycle.
