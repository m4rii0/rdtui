## Requirements

### Requirement: All popups render as centered overlay
The application SHALL render all popup modes (file selection, delete confirmation, target picker, overwrite confirmation, URL display) as centered overlay popups on top of the background view. The background view (main list, detail, or download) SHALL be rendered first, then the popup SHALL be overlaid using `lipgloss.Place(center, center)`.

#### Scenario: File selection popup centered overlay
- **WHEN** the user opens file selection for a waiting torrent
- **THEN** the background view (main list or detail) SHALL be rendered first
- **AND** the file selection popup SHALL be overlaid centered on top with a rounded border

#### Scenario: Delete confirmation popup centered overlay
- **WHEN** the user presses `x` on a torrent
- **THEN** the background view SHALL be rendered first
- **AND** the delete confirmation popup SHALL be overlaid centered on top with a rounded border

#### Scenario: Small terminal fallback
- **WHEN** the terminal is smaller than 40 columns or 12 rows
- **THEN** the popup SHALL render at full width with no centering padding

### Requirement: Reusable popup helper functions
The application SHALL provide shared popup helper functions in a dedicated file (`popup.go`): a centering function that places content in the center of the terminal, a boxing function that wraps content in a bordered container with a title, a footer renderer for keybinding hints, and a dimension calculator for popup sizing.

#### Scenario: Popup helpers used by all popup renderers
- **WHEN** any popup mode is rendered
- **THEN** it SHALL use the shared helpers for centering, boxing, footer, and sizing
- **AND** no popup renderer SHALL duplicate centering or border logic

### Requirement: Popup footer with keybinding hints
Every popup SHALL display a footer line at the bottom showing all available keybindings for that popup. The footer SHALL be visible at all times while the popup is open.

#### Scenario: File selection popup footer
- **WHEN** the file selection popup is displayed
- **THEN** a footer SHALL show keybindings: `space=toggle  ctrl+a=all  ctrl+d=clear  enter=confirm  esc=cancel`

#### Scenario: Delete popup footer
- **WHEN** the delete confirmation popup is displayed
- **THEN** a footer SHALL show keybindings: `y/enter=delete  n/esc=cancel`

#### Scenario: Target picker popup footer
- **WHEN** the target picker popup is displayed
- **THEN** a footer SHALL show keybindings: `↑↓=navigate  enter=confirm  esc=cancel`

### Requirement: Per-popup dedicated render functions
The application SHALL replace the monolithic `renderModal()` switch statement with dedicated render functions for each popup type: `renderSelectFilesPopup()`, `renderDeletePopup()`, `renderOverwritePopup()`, `renderTargetPickerPopup()`, `renderShowURLPopup()`. Each SHALL handle its own content, sizing, and footer.

#### Scenario: No renderModal function remains
- **WHEN** the application is compiled
- **THEN** `renderModal()` SHALL NOT exist in the codebase
- **AND** each popup type SHALL have its own render function

### Requirement: Consistent view routing for popups
The `renderView()` function SHALL determine whether the current mode is a popup mode and, if so, render the background view first then overlay the popup on top. The file browser SHALL no longer be a special case — it SHALL use the same overlay routing as all other popups.

#### Scenario: File browser uses standard popup routing
- **WHEN** the user opens the file browser popup
- **THEN** it SHALL go through the same popup overlay path as all other popup modes
- **AND** `renderView()` SHALL NOT have a special early-return for `modeFileBrowser`

#### Scenario: Popup mode with detail background
- **WHEN** a popup is opened from the detail view
- **THEN** the detail view SHALL be rendered as the background behind the popup
