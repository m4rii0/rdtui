## Context

`rdtui` is distributed as versioned GitHub release binaries built by `.github/workflows/release.yml`. The release workflow embeds `GITHUB_REF_NAME` into `internal/version.Version`, publishes platform-specific assets under stable names, and publishes `checksums.txt` plus a Sigstore bundle for those checksums.

The current CLI path only handles `--version` before starting the Bubble Tea TUI. Self-update behavior should happen before any TUI initialization because it is a command-line maintenance action, not part of torrent management. The first version should avoid background network access and avoid mutating the running executable unless the user explicitly invokes an updater command.

## Goals / Non-Goals

**Goals:**
- Add explicit updater commands that can check for and install newer stable releases.
- Use Cobra for command and flag parsing while keeping updater behavior explicit and testable.
- Use GitHub Releases as the update source and the existing release asset naming scheme as the platform contract.
- Verify downloaded release assets against `checksums.txt` before any install action.
- Replace the executable on Unix-like platforms and provide a verified manual replacement path on Windows.
- Fail safely with actionable messages when update preconditions are not met.

**Non-Goals:**
- No background update checks during normal TUI startup.
- No silent or automatic updates.
- No prerelease channel support in the first pass.
- No embedded Sigstore verification in the first pass.
- No Windows helper process for replacing a running `.exe` in the first pass.
- No package-manager integration or downgrade support.

## Decisions

### Use Cobra subcommands before TUI startup

Add a Cobra root command in `cmd/rdtui` that handles maintenance commands before constructing the app service or Bubble Tea program.

Initial command surface:
- `rdtui --version`
- `rdtui check-update`
- `rdtui update`
- `rdtui update --dry-run`

Rationale: updater commands should not require Real-Debrid config, auth bootstrap, or TUI state. Cobra provides standard subcommand and flag behavior, clear usage errors, and testable command execution with `SetArgs`, `SetOut`, and `SetErr`. Updater flags should still be bound into an options struct rather than handled as one-off positional checks so later combinations like `rdtui update --dry-run --prerelease` do not require reworking the command model.

Alternative considered: keep a custom parser. Rejected because the updater is intentionally growing command and flag surface area, and Cobra gives a more maintainable path for future flags.

### Use GitHub latest release as the stable update source

The updater will query `GET https://api.github.com/repos/m4rii0/rdtui/releases/latest`. GitHub defines this endpoint as the latest published non-draft, non-prerelease release, matching the desired stable-only first pass.

Rationale: the existing release workflow already publishes all required assets to GitHub Releases, so no separate update manifest or registry is needed.

Alternative considered: add a custom update manifest. Rejected for v1 because it would duplicate information already available from GitHub and introduce another release artifact to maintain.

### Compare released semver tags only

The updater will require the current embedded version and latest release tag to be valid Go-style semver strings with a `v` prefix. `dev`, empty, dirty, or otherwise invalid versions will not self-update.

Rationale: self-update needs deterministic ordering. Builds produced by `go run`, local `make build` on untagged commits, or dirty working trees may not correspond to a published release asset.

Alternative considered: allow any current version and update if a latest release exists. Rejected because it can surprise users running local builds and obscures downgrade/sidegrade behavior.

### Treat release asset names as the platform contract

The updater will map runtime platform to the existing release asset names:

| Platform | Asset |
| --- | --- |
| `darwin/amd64` | `rdtui-darwin-amd64` |
| `darwin/arm64` | `rdtui-darwin-arm64` |
| `linux/amd64` | `rdtui-linux-amd64` |
| `linux/arm64` | `rdtui-linux-arm64` |
| `windows/amd64` | `rdtui-windows-amd64.exe` |
| `windows/arm64` | `rdtui-windows-arm64.exe` |

Rationale: this matches the release workflow and keeps platform support explicit. Unsupported platforms fail before download.

Alternative considered: infer assets by substring matching without a fixed map. Rejected because it can select the wrong asset if release assets change or include auxiliary files.

### Verify SHA256 before install

The updater will download `checksums.txt`, parse the expected digest for the selected asset, compute the downloaded asset's SHA256, and fail closed on any mismatch or missing checksum.

Rationale: this catches corrupted or incomplete downloads and prevents installation of an asset that does not match the published release checksum. It also aligns with the current release verification instructions.

Alternative considered: verify only GitHub HTTPS transport. Rejected because HTTPS does not detect a mismatched or accidentally uploaded asset.

Alternative considered: require Sigstore verification immediately. Deferred because it is more complex to embed and validate correctly; the release workflow already produces the bundle, so the design leaves a clean hardening path.

### Install atomically where practical

On Unix-like platforms, the updater will download to a temporary file, verify it, mark it executable, create a backup of the current executable, rename the verified binary into place, run the new binary with `--version`, and remove the backup after a healthy install.

On Windows, the updater will download and verify the replacement but will not replace the running `.exe` in v1. It will print the verified replacement path and manual instructions.

Rationale: Unix rename semantics make replacement practical. Windows replacement of a running executable requires a helper process or installer flow, which is explicitly deferred.

Alternative considered: always ask users to manually replace the binary. Rejected for Unix-like platforms because the automatic path is simple and useful.

## Risks / Trade-offs

- GitHub API/network failure -> Report the failure and leave the installed binary unchanged.
- GitHub unauthenticated rate limits -> Use one latest-release request per explicit command and keep error messages clear; conditional requests can be added later if needed.
- Invalid current version from local builds -> Refuse self-update and explain that only released versions can self-update.
- Missing or renamed release asset -> Fail before download and identify the expected asset name.
- Missing checksum or checksum mismatch -> Fail closed and leave the installed binary unchanged.
- SHA256 checksums do not prove release authenticity if both binary and checksum assets are tampered with -> Document the distinction and keep embedded Sigstore verification as a follow-up hardening change.
- Install path is not writable -> Leave the verified download in a temporary or user-visible location and explain how to install manually.
- Symlinked executable path -> Resolve conservatively or fail with manual instructions if the actual replacement target is ambiguous.
- Partial Unix replacement failure -> Keep a backup when possible and restore or report the backup path if rollback fails.
- Windows manual replacement is less convenient -> Acceptable for v1; a helper process can be proposed later.

## Migration Plan

No data migration is required. Existing installs continue to work. Users who want self-update behavior invoke `rdtui check-update` or `rdtui update` explicitly.

Rollback is manual: users can reinstall the previous GitHub release asset or use the backup path if an update fails after backup creation.

## Open Questions

- Should the verified Windows replacement be written next to the current executable, to the user's downloads directory, or to a temp directory?
- Should `update --dry-run` perform only release comparison, or also verify that the matching asset and checksum are present?
