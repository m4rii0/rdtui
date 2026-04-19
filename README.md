# rdtui

`rdtui` is a Go terminal UI for managing Real-Debrid torrents.

This first iteration focuses on:
- authenticating with Real-Debrid
- browsing current torrents
- adding magnet links and `.torrent` files
- selecting files for waiting torrents
- generating direct download URLs from ready torrents
- handing those URLs off to your clipboard
- downloading resolved links locally through either the built-in direct downloader or an app-managed `aria2c` session
- deleting torrents

## Requirements

- Go 1.26+
- A Real-Debrid account
- `aria2c` installed on `PATH` only if you want to use the optional `aria2` download backend

## Build

```bash
go build ./cmd/rdtui
```

## Run

```bash
go run ./cmd/rdtui
```

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

Key bindings:
- `j` / `k`: move between torrents
- `r`: refresh torrents
- `m`: add a magnet link
- `u`: add a remote `.torrent` URL
- `i`: browse the filesystem and import one or more local `.torrent` files
- `s`: select files for a torrent in `waiting_files_selection`
- `y`: resolve a ready torrent target to a direct URL and show or copy it
- `d`: resolve a ready torrent target and start a local download using the configured `download_backend`
- `x`: delete the selected torrent
- `q`: quit

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

## Notes

- Real-Debrid rate-limits the API, so the UI uses conservative polling.
- For multi-file torrents, Real-Debrid's file-to-link mapping can be ambiguous. The UI keeps the chosen target explicit and defaults to the safest flow available.
