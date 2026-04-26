## 1. Command Parsing

- [x] 1.1 Add a Cobra command tree that recognizes `--version`, `check-update`, `update`, and `update --dry-run` before TUI startup.
- [x] 1.2 Bind updater flags into an options set so multiple future flags can be parsed together without changing the command handler shape.
- [x] 1.3 Return clear usage errors for unknown updater commands or unsupported updater flags.
- [x] 1.4 Add parser tests covering normal TUI startup, version output, update check, update, dry run, multiple-flag parsing shape, and invalid arguments.

## 2. Release Discovery

- [x] 2.1 Add an updater package for GitHub latest-release lookup using `https://api.github.com/repos/m4rii0/rdtui/releases/latest`.
- [x] 2.2 Add version validation and comparison for `v`-prefixed SemVer versions, refusing `dev`, empty, prerelease, unsupported build metadata, or invalid versions while allowing tagged `+dirty` local builds.
- [x] 2.3 Add platform asset selection for supported `GOOS/GOARCH` combinations and clear failures for unsupported or missing assets.
- [x] 2.4 Add tests for release response parsing, semver comparison, invalid current versions, and platform asset mapping.

## 3. Download Verification

- [x] 3.1 Download the selected release asset and `checksums.txt` to temporary files or memory as appropriate.
- [x] 3.2 Parse `checksums.txt` and locate the expected SHA256 digest for the selected asset name.
- [x] 3.3 Verify the downloaded asset digest before install or manual replacement instructions.
- [x] 3.4 Add tests for checksum parsing, missing checksum entries, matching digests, and mismatched digests.

## 4. Install Flow

- [x] 4.1 Implement Unix-like install by writing a verified executable, setting executable permissions, backing up the current executable, renaming the verified binary into place, and validating `--version` after replacement.
- [x] 4.2 Ensure Unix-like install failures leave the current executable unchanged or report the backup path when rollback cannot be completed.
- [x] 4.3 Implement Windows behavior that writes the verified replacement and prints manual replacement instructions without replacing the running `.exe`.
- [x] 4.4 Add install tests using temporary executable paths for success, non-writable or invalid targets, backup behavior, and Windows manual fallback where feasible.

## 5. User Output And Documentation

- [x] 5.1 Implement `check-update` output for update available, already current, invalid current version, network failure, and missing platform asset cases.
- [x] 5.2 Implement `update` and `update --dry-run` output for installed update, no update, verification failure, unsupported platform, and Windows manual replacement cases.
- [x] 5.3 Document `rdtui check-update`, `rdtui update`, and `rdtui update --dry-run` in the README.

## 6. Verification

- [x] 6.1 Run `go test ./...`.
- [x] 6.2 Run `make verify` or document any environment-specific blocker.
