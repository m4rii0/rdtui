## Why

Real-Debrid exposes the torrent workflow needed for a useful terminal client, but this repository does not yet provide any way to browse, manage, and download from torrents without using the website. A first iteration focused on torrents creates a practical daily-use tool while keeping the initial scope narrow enough to implement cleanly.

## What Changes

- Add a Go terminal UI for Real-Debrid torrents with a master-detail workflow for browsing torrents and inspecting the selected torrent.
- Support authenticating with either Real-Debrid device auth or a pasted private API token.
- Allow users to add new torrents from magnet links, one or more local `.torrent` files selected from a filesystem browser, and remote `.torrent` URLs.
- Allow users to select files for torrents that are waiting for file selection, with the largest file preselected by default and confirmation before submission.
- Allow users to generate a direct download URL from a ready torrent, then either copy/show that URL or launch a configured external downloader.
- Allow users to delete a torrent from Real-Debrid with confirmation.

## Capabilities

### New Capabilities
- `real-debrid-authentication`: Sign in with either device auth or a private API token and reuse valid stored credentials across sessions.
- `torrent-management`: List, inspect, add, select files for, and delete Real-Debrid torrents from the terminal UI.
- `download-handoff`: Convert ready torrent links into direct URLs and hand them off to the user or an external downloader without tracking the local transfer.

### Modified Capabilities

None.

## Impact

- Adds a new Go terminal application and initial package layout.
- Introduces dependencies for terminal UI, configuration, clipboard support, and process launching.
- Integrates with Real-Debrid authentication, torrents, and unrestrict endpoints.
- Adds local credential/config storage, filesystem-browsing support for local torrent import, and external downloader configuration.
