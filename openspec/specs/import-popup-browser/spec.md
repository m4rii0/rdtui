## ADDED Requirements

### Requirement: Browser opens in current working directory
When the user activates the import file browser, it SHALL display the contents of the current working directory (where `rdtui` was launched), not the user's home directory.

#### Scenario: User presses i from main view
- **WHEN** user presses `i` in `modeMain` or `modeDetail`
- **THEN** the file browser opens with `CurrentDir` set to the current working directory

### Requirement: Browser renders as a centered popup overlay
The file browser SHALL render as a bordered, centered popup overlay on top of the background view (main torrent list or detail view). The popup SHALL use a rounded border and display the current directory path as a title.

#### Scenario: File browser popup display
- **WHEN** the file browser is active (`modeFileBrowser`)
- **THEN** the background view (main or detail) SHALL be rendered first, and the file browser SHALL be overlaid as a centered popup on top of it
- **AND** the popup SHALL have a rounded border with the current directory path in the title
- **AND** the popup SHALL occupy approximately 70% of the terminal width and a reasonable height

#### Scenario: Small terminal fallback
- **WHEN** the terminal is smaller than 40 columns or 12 rows
- **THEN** the popup SHALL render at full width with no centering padding

### Requirement: Checkbox-style selection markers
Selected files SHALL display `[x]` and unselected files SHALL display `[ ]` instead of `*` markers.

#### Scenario: File listing shows checkboxes
- **WHEN** the file browser displays `.torrent` files
- **THEN** selected files SHALL show `[x]` prefix and unselected files SHALL show `[ ]` prefix

### Requirement: Visual mode for range selection
The file browser SHALL support a visual mode activated by pressing `V`. In visual mode, moving the cursor with `j`/`k` SHALL automatically select all `.torrent` files between the anchor point (where `V` was pressed) and the current cursor position. Pressing `V` again SHALL exit visual mode and preserve selections.

#### Scenario: Enter visual mode and select range
- **WHEN** user presses `V` on a `.torrent` file at index 2
- **AND** moves cursor down to index 5 with `j`
- **THEN** all `.torrent` files between index 2 and index 5 SHALL be selected
- **AND** the popup footer SHALL indicate visual mode is active

#### Scenario: Exit visual mode preserves selections
- **WHEN** user presses `V` again after selecting a range
- **THEN** visual mode SHALL be deactivated
- **AND** all files selected during visual mode SHALL remain selected

#### Scenario: Visual mode ignores directories
- **WHEN** visual mode is active and cursor passes over directory entries
- **THEN** directories SHALL NOT be added to the selection (only `.torrent` files are selected)

### Requirement: Select all .torrent files
Pressing `Ctrl+A` SHALL select all `.torrent` files in the current directory. If all `.torrent` files are already selected, pressing `Ctrl+A` SHALL deselect all of them.

#### Scenario: Select all with Ctrl+A
- **WHEN** user presses `Ctrl+A` in the file browser
- **THEN** all `.torrent` files in the current directory SHALL be selected

#### Scenario: Deselect all with Ctrl+A when all selected
- **WHEN** all `.torrent` files in the current directory are already selected
- **AND** user presses `Ctrl+A`
- **THEN** all `.torrent` files SHALL be deselected

### Requirement: Clear all selections
Pressing `Ctrl+D` SHALL deselect all files, clearing the entire selection map.

#### Scenario: Clear selections with Ctrl+D
- **WHEN** user has 3 files selected and presses `Ctrl+D`
- **THEN** all 3 files SHALL be deselected
- **AND** the selected count SHALL show 0

### Requirement: Toggle hidden file visibility
Hidden files and directories (names starting with `.`) SHALL be hidden by default. Pressing `H` SHALL toggle their visibility and refresh the listing.

#### Scenario: Hidden files not shown by default
- **WHEN** the current directory contains `.hidden.torrent` and `normal.torrent`
- **THEN** only `normal.torrent` SHALL appear in the listing

#### Scenario: Toggle hidden files on
- **WHEN** user presses `H`
- **THEN** hidden files and directories SHALL become visible in the listing
- **AND** pressing `H` again SHALL hide them

### Requirement: File size display
Each `.torrent` file entry in the browser SHALL display its file size.

#### Scenario: Torrent file shows size
- **WHEN** the browser lists a `.torrent` file that is 1.2 MB
- **THEN** the entry SHALL display the filename followed by the size (e.g., `(1.2 MB)`)

### Requirement: Popup footer with keybinding hints and selection count
The popup SHALL ALWAYS display a footer line at the bottom showing the current selection count and ALL available keybindings. This footer SHALL be visible at all times while the file browser popup is open, regardless of directory content, error state, or selection state.

#### Scenario: Footer always visible
- **WHEN** the file browser popup is displayed in any state (empty directory, error, normal listing)
- **THEN** a footer line SHALL always be rendered at the bottom of the popup showing keybindings: `enter=open/toggle  space=toggle  V=visual  ctrl+a=all  ctrl+d=clear  H=hidden  ctrl+s=import  esc=cancel`
- **AND** the footer SHALL show the count of selected files (e.g., `Selected: 3`)

#### Scenario: Footer visible on error
- **WHEN** the current directory cannot be read and an error is displayed
- **THEN** the footer with keybinding hints SHALL still be rendered below the error message
