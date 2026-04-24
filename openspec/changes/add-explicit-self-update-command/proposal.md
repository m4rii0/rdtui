## Why

`rdtui` already publishes versioned multi-platform GitHub release binaries, but users currently need to discover, download, verify, and install updates manually. An explicit self-update command gives users a safer, repeatable way to stay current without adding background network activity or automatic changes during normal TUI use.

## What Changes

- Add an explicit update-check command that reports whether a newer stable GitHub release is available.
- Add an explicit update command that downloads the current platform's release asset, verifies it against `checksums.txt` using SHA256, and installs it when supported.
- Refuse self-update for `dev` or invalid embedded versions because update comparison requires a valid released semver tag.
- Use GitHub's latest release endpoint, which excludes draft and prerelease releases by default.
- Replace the running executable on Unix-like platforms when safe; on Windows, download and verify the replacement and provide manual replacement instructions for the first pass.
- Prepare command parsing so updater commands and one or more updater flags are handled separately from the normal TUI startup path.

## Capabilities

### New Capabilities
- `self-update`: Explicit release checking and self-update behavior for the `rdtui` executable.

### Modified Capabilities
- None.

## Impact

- `cmd/rdtui/main.go`: command parsing for updater subcommands before launching the TUI.
- New updater internals for GitHub release lookup, semver comparison, platform asset selection, checksum verification, and executable installation.
- New dependency on `github.com/spf13/cobra` for command and flag parsing.
- Release asset naming and `checksums.txt` become part of the updater contract.
- Tests for version comparison, asset selection, checksum parsing/verification, command parsing, and install edge cases.
- Possible new dependency on `golang.org/x/mod/semver` for correct Go-style semver comparison.
