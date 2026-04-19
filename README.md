# rdtui

`rdtui` is a Go terminal UI for managing Real-Debrid torrents.

This first iteration focuses on:
- authenticating with Real-Debrid
- browsing current torrents
- adding magnet links and `.torrent` files
- selecting files for waiting torrents
- generating direct download URLs from ready torrents
- handing those URLs off to your clipboard or an external downloader
- deleting torrents

## Requirements

- Go 1.26+
- A Real-Debrid account

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
  "external_command": [
    "aria2c",
    "--dir",
    "{{dir}}",
    "--out",
    "{{filename}}",
    "{{url}}"
  ]
}
```

Supported downloader placeholders:
- `{{url}}`
- `{{dir}}`
- `{{filename}}`

If no `external_command` is configured, `rdtui` will still resolve and show direct URLs and will try to copy them to the clipboard when possible.

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
- `x`: resolve a ready torrent target and launch the configured downloader
- `d`: delete the selected torrent
- `q`: quit

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
