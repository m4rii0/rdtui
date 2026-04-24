## ADDED Requirements

### Requirement: User can check for stable updates explicitly
The application SHALL provide a `check-update` command that compares the current embedded `rdtui` version with the latest stable GitHub release without launching the TUI.

#### Scenario: Newer stable release is available
- **WHEN** the user runs `rdtui check-update` from a released version older than the latest stable GitHub release
- **THEN** the application reports the current version, the latest version, and that an update is available

#### Scenario: Current version is already latest
- **WHEN** the user runs `rdtui check-update` from the latest stable released version
- **THEN** the application reports that `rdtui` is up to date

#### Scenario: Current version cannot be compared
- **WHEN** the user runs `rdtui check-update` from a `dev`, empty, or invalid embedded version
- **THEN** the application refuses the check and explains that only released semver versions can be checked for self-update

### Requirement: User can install a stable update explicitly
The application SHALL provide an `update` command that downloads and installs a newer stable GitHub release only when the user explicitly invokes it.

#### Scenario: Newer stable release installs on Unix-like platforms
- **WHEN** the user runs `rdtui update` on a supported Unix-like platform from an older released version
- **THEN** the application downloads the matching release asset, verifies its SHA256 checksum, replaces the current executable, and reports the installed version

#### Scenario: No update is available
- **WHEN** the user runs `rdtui update` from the latest stable released version
- **THEN** the application reports that no update is available and leaves the current executable unchanged

#### Scenario: Dry run does not install
- **WHEN** the user runs `rdtui update --dry-run`
- **THEN** the application reports what update action would occur without replacing or writing over the current executable

#### Scenario: Updater flags are parsed as options
- **WHEN** the user runs an updater command with supported flags
- **THEN** the application parses those flags into command options without requiring a TUI startup path

### Requirement: Updates use stable GitHub release assets
The updater SHALL use the latest published non-draft, non-prerelease GitHub release as the update source and SHALL select the release asset matching the current operating system and architecture.

#### Scenario: Platform asset exists
- **WHEN** the latest stable release contains the asset matching the current operating system and architecture
- **THEN** the updater selects that asset for download

#### Scenario: Platform asset is missing
- **WHEN** the latest stable release does not contain an asset matching the current operating system and architecture
- **THEN** the updater fails before installation and reports the expected asset name

#### Scenario: Unsupported platform
- **WHEN** the user runs an updater command on an unsupported operating system or architecture
- **THEN** the updater fails before download and reports that self-update is unsupported for that platform

### Requirement: Downloaded update assets are verified before install
The updater SHALL verify the selected release asset against the release `checksums.txt` SHA256 digest before installing or instructing the user to install it.

#### Scenario: Checksum matches
- **WHEN** the updater downloads the selected release asset and `checksums.txt` contains a matching SHA256 digest for that asset
- **THEN** the updater allows the install or manual replacement flow to continue

#### Scenario: Checksum is missing
- **WHEN** `checksums.txt` does not contain a digest for the selected release asset
- **THEN** the updater fails and leaves the current executable unchanged

#### Scenario: Checksum does not match
- **WHEN** the downloaded asset's SHA256 digest differs from the digest in `checksums.txt`
- **THEN** the updater fails and leaves the current executable unchanged

### Requirement: Windows updates provide verified manual replacement instructions
On Windows, the updater SHALL download and verify the matching release asset but SHALL NOT attempt to replace the running executable in the first pass.

#### Scenario: Windows update asset is verified
- **WHEN** the user runs `rdtui update` on Windows and a newer stable release is available
- **THEN** the updater downloads and verifies the matching `.exe` asset, reports the verified file path, and explains how to manually replace the current executable

#### Scenario: Windows verification fails
- **WHEN** the user runs `rdtui update` on Windows and verification fails
- **THEN** the updater reports the verification failure and does not instruct the user to replace the executable

### Requirement: Normal TUI startup does not check for updates
The application SHALL NOT perform update checks, downloads, or installs during normal TUI startup.

#### Scenario: User starts TUI normally
- **WHEN** the user runs `rdtui` without an updater command
- **THEN** the application starts the TUI without contacting GitHub for update information
