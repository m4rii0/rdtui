# rdtui

`rdtui` is a Go terminal UI for managing Real-Debrid torrents.

![rdtui terminal showcase](docs/assets/rdtui-showcase.gif)

## Status

`rdtui` is an early public release. The core torrent management and download flows are implemented, but behavior and configuration may still change between releases.

## Personal Project Disclaimer

This repository is a personal project created and maintained by me in my own capacity. It is not affiliated with, endorsed by, sponsored by, or representative of my employer or any company I work for.

All code, opinions, design decisions, and documentation in this repository are my own and do not reflect the views, policies, or interests of my employer.

## Current Functionality

- Authenticate with Real-Debrid via device auth or private API token
- Browse and manage your active torrent list
- Open a detail view for individual torrents (files, seeders, speed, progress)
- Add torrents via magnet links, remote `.torrent` URLs, or local `.torrent` file imports (including batch imports)
- Select files for torrents awaiting file selection
- Resolve ready torrents to direct download URLs and copy them to the clipboard
- Download resolved links locally using the built-in HTTP downloader or an app-managed `aria2c` session
- Monitor in-progress downloads with a live progress screen, with options to open the completed file, reveal it in its directory, or delete the source torrent
- Fuzzy search and filter torrents
- Sort torrents by status, progress, size, date, or name
- Batch operations: select multiple torrents to delete or copy URLs in bulk
- Delete torrents
- Built-in help overlay listing all available shortcuts

## Requirements

- Go 1.26+
- A Real-Debrid account
- `aria2c` installed on `PATH` only if you want to use the optional `aria2` download backend

## Install

```bash
go install github.com/m4rii0/rdtui/cmd/rdtui@latest
```

Then run:

```bash
rdtui
```

### Verify Release Assets

GitHub release checksums are signed in CI with keyless Sigstore signing. To verify a downloaded release, run these commands from the directory containing the release assets:

```bash
cosign verify-blob \
  --bundle checksums.txt.sigstore.json \
  --certificate-identity-regexp '^https://github\.com/m4rii0/rdtui/\.github/workflows/release\.yml@refs/tags/v.*$' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  checksums.txt

sha256sum --check checksums.txt
```

## Build

```bash
make build
```

This compiles the binary to `bin/rdtui` and embeds the current git version via ldflags.

For a plain build without version info:

```bash
go build ./cmd/rdtui
```

## Run

```bash
go run ./cmd/rdtui
```

Or after building:

```bash
./bin/rdtui
```

To check the embedded version:

```bash
rdtui --version
```

To check whether a newer stable GitHub release is available:

```bash
rdtui check-update
```

To explicitly update a released binary:

```bash
rdtui update
```

Preview the update action without replacing files:

```bash
rdtui update --dry-run
```

Self-update is explicit only: normal TUI startup does not check for or install updates. The updater performs a SHA256 integrity check against `checksums.txt` before install; use the Sigstore commands above when you need release authenticity verification. Windows builds download and verify the replacement executable, then print manual replacement instructions.

On first run the app will ask you to authenticate.

## Authentication

`rdtui` supports two auth modes.

### 1. Device Auth

Recommended for normal use.

Choose device auth in the TUI, open the shown Real-Debrid verification URL, enter the user code, approve the app, then return to the terminal.

The app stores the resulting user-bound credentials locally and reuses them on later runs.

### 2. Private API Token

If you prefer, choose token auth and paste your Real-Debrid private API token.

The token is validated before the torrent UI is enabled.

You can also provide it through:

```bash
RDTUI_PRIVATE_TOKEN=... go run ./cmd/rdtui
```

Credential precedence is:
1. configured or environment-provided private token
2. stored device-auth credentials
3. interactive auth prompt

## Configuration

The app stores local state in your config directory:

- Linux: `~/.config/rdtui/`
- macOS: `~/Library/Application Support/rdtui/`
- Windows: `%AppData%\rdtui\`

### `config.json`

Example:

```json
{
  "private_token": "",
  "default_download_dir": "/home/you/Downloads",
  "download_backend": "aria2",
  "aria2c_path": ""
}
```

- `private_token` is optional. If present, it is used before stored device-auth credentials.
- `default_download_dir` controls where local downloads are saved. If omitted, it defaults to `~/Downloads`.
- `download_backend` supports `aria2` or `direct`. If omitted or invalid, it defaults to `direct`.
- `aria2c_path` is optional and only used when `download_backend` is `aria2`. Leave it empty to use `aria2c` from `PATH`.
- Older `external_command` entries are ignored by the managed download flow and will disappear the next time `rdtui` saves config.

Behavior summary:
- `download_backend = "direct"`: `rdtui` downloads the resolved URL with its built-in HTTP downloader and does not require `aria2c`.
- `download_backend = "aria2"`: `rdtui` starts its own loopback-only `aria2c` RPC session and monitors that download in the TUI.

### `auth.json`

This file is created automatically when device auth succeeds.

## Usage

Once authenticated, the main screen shows a master-detail torrent workbench.

Key bindings (main view):
- `j` / `k`: move between torrents
- `enter`: open torrent detail view
- `/`: search and filter torrents (fuzzy match)
- `r`: refresh torrents
- `m`: add a magnet link
- `u`: add a remote `.torrent` URL
- `i`: browse the filesystem and import one or more local `.torrent` files
- `s`: select files for a torrent in `waiting_files_selection`
- `y`: resolve a ready torrent target to a direct URL and show or copy it
- `d`: resolve a ready torrent target and start a local download using the configured `download_backend`
- `x`: delete the selected torrent
- `b`: enter batch mode (select multiple torrents)
- `S` / `P` / `Z` / `D` / `N`: sort by status / progress / size / date / name
- `?`: toggle help overlay
- `q`: quit (or clear active filter)

Batch mode (`b`):
- `space`: mark/unmark torrent
- `ctrl+a`: select all, `ctrl+d`: clear selection
- `x`: delete marked torrents
- `y`: copy URLs for marked torrents
- `b` / `esc`: exit batch mode

Detail view (`enter`):
- `s`: select files (when applicable)
- `y`: copy URL, `d`: download, `x`: delete, `r`: refresh
- `i`: import a `.torrent` file
- `esc`: back to main view

Download progress view (`d`):
- `r`: refresh progress
- `o`: open completed file
- `s`: reveal file in directory
- `x`: delete source torrent
- `esc`: back to main view

### Managed Download

Press `d` on a ready torrent to resolve the selected target and start a managed local download. `rdtui` shows a dedicated progress screen and lets you refresh progress, open the completed file, reveal it in its directory, or delete the source torrent once the download has completed.

If target file already exists in `default_download_dir`, `rdtui` pauses first and asks whether you want to download it again, showing current local size and size difference when remote size is known.

When `download_backend` is `aria2`, `rdtui` starts its own loopback-only `aria2c` RPC session for the download.

When `download_backend` is `direct`, `rdtui` falls back to a built-in HTTP download and does not require `aria2c`.

If you only want the direct URL, press `y` instead. That flow still shows the URL and copies it to the clipboard when possible without starting `aria2c`.

### File Selection

For torrents in `waiting_files_selection`, the app preselects the largest file by default and lets you adjust the selection before submitting it.

### Local `.torrent` Import

The file browser only shows directories and `.torrent` files. You can select multiple `.torrent` files before importing them as a batch.

Batch imports preserve partial success: if some uploads fail, the successful torrents remain added and the app reports which files failed.

## Testing

```bash
go test ./...
```

## License

This project is licensed under the [Apache License 2.0](LICENSE).

Copyright 2026 Mario (m4rii0).

## Legal Disclaimer

This project does not contain any copyrighted material. It is a client application that solely makes API calls to the [Real-Debrid](https://real-debrid.com) service. All content accessed through this tool is hosted and served by Real-Debrid; this repository contains no media files, torrents, or pirated content of any kind.

This project is not affiliated with, endorsed by, or connected to Real-Debrid in any way.

This software is provided "as is", without warranty of any kind, express or implied. Use it at your own risk.

## Notes

- Real-Debrid rate-limits the API, so the UI uses conservative polling.
- For multi-file torrents, Real-Debrid's file-to-link mapping can be ambiguous. The UI keeps the chosen target explicit and defaults to the safest flow available.
