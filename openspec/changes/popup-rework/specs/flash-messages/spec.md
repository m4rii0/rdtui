## ADDED Requirements

### Requirement: Transient flash messages for action feedback
The application SHALL display transient status messages (flash messages) below the main content and above the footer. Flash messages SHALL auto-dismiss after 3 seconds. Each flash message SHALL have a level: success (green), error (red), warning (yellow), or info (blue).

#### Scenario: Successful action shows flash
- **WHEN** the user completes an action (e.g., deletes a torrent, selects files, copies a URL)
- **THEN** a green flash message SHALL appear below the main content with a description of the completed action

#### Scenario: Error shows flash
- **WHEN** an async operation fails
- **THEN** a red flash message SHALL appear with the error description

#### Scenario: Flash auto-dismisses after 3 seconds
- **WHEN** a flash message is displayed
- **THEN** it SHALL automatically disappear after 3 seconds without user interaction

#### Scenario: Flash cleared on mode change
- **WHEN** the user changes mode (e.g., enters a popup, navigates to detail)
- **THEN** any active flash message SHALL be cleared immediately

### Requirement: Flash message state in model
The `Model` struct SHALL include a `flash` field holding the current flash message text, level, and creation time. The flash SHALL be set via a `setFlash(level, msg)` method and cleared via a `clearFlash()` method or automatically via a `tea.Tick` command.

#### Scenario: Model tracks flash state
- **WHEN** a flash message is set
- **THEN** `Model.flash.message` SHALL contain the text, `Model.flash.level` SHALL contain the level, and `Model.flash.setAt` SHALL contain the timestamp

#### Scenario: Flash message tick clears expired flash
- **WHEN** a `tea.Tick` fires after the flash duration
- **THEN** the flash SHALL be cleared and the view SHALL re-render without the flash

### Requirement: Flash rendering styled by level
Flash messages SHALL be rendered with level-specific styling: success messages in green bold, error messages in red bold, warning messages in yellow bold, and info messages in blue bold. The flash SHALL render as a single line.

#### Scenario: Success flash styling
- **WHEN** a success flash is active
- **THEN** it SHALL render as a green bold text line

#### Scenario: Error flash styling
- **WHEN** an error flash is active
- **THEN** it SHALL render as a red bold text line
