## Why

`rdtui` can currently resolve a ready torrent into a direct download URL, but it does not complete the last mile inside the app. Users still need to leave the TUI or wire up their own downloader, and the existing launch-oriented path is incomplete in practice because it is not exposed as a real user workflow.

## What Changes

- Add a managed local download flow that starts from a resolved Real-Debrid link and runs fully from within `rdtui`.
- Have `rdtui` start and manage its own `aria2c` process instead of requiring a pre-existing aria2 RPC daemon.
- Add a dedicated download action in the torrent UI and a full-screen download progress view with status, speed, progress, ETA, and completion feedback.
- Keep direct URL copy and show behavior for users who still want a handoff-only workflow.
- Update downloader configuration to describe how `rdtui` launches and talks to its managed aria2 instance, including download directory and an optional `aria2c` binary path override.
- Tighten the existing download-handoff behavior so managed downloads and URL handoff each have explicit, reachable UI actions.

## Capabilities

### New Capabilities
- `managed-downloads`: Start, monitor, and complete a local download session from a resolved Real-Debrid link using an `aria2c` process that is started and managed by `rdtui`.

### Modified Capabilities
- `download-handoff`: Clarify how direct URL copy and show behavior coexists with the new managed download flow, and limit external handoff behavior to explicitly supported actions.
- `torrent-management`: Add a download action, update keyboard-driven torrent actions, and describe the new download progress workflow available from list and detail views.

## Impact

- Adds aria2 process lifecycle management and RPC orchestration to the Go application.
- Affects `internal/download`, `internal/app`, `internal/tui`, `internal/config`, shared models, and user documentation.
- Introduces a new dependency for aria2 RPC integration and relies on the `aria2c` binary being available on the local machine.
