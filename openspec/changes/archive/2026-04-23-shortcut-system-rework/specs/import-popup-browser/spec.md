## MODIFIED Requirements

### Requirement: Popup footer with keybinding hints and selection count
The popup SHALL ALWAYS display a footer line at the bottom showing the current selection count and the current browser-context keybindings. This footer SHALL be visible at all times while the file browser popup is open, regardless of directory content, error state, or selection state. When the browser enters a distinct subcontext such as path editing, the footer SHALL update to show only the shortcuts relevant to that subcontext. Browser actions that are unavailable within the current browser context SHALL remain visible in a dimmed style when they are part of that context, while unrelated shortcuts remain hidden.

#### Scenario: Footer reflects normal browser mode
- **WHEN** the file browser popup is displayed in its normal navigation mode
- **THEN** the footer shows the currently available browser-navigation and selection shortcuts
- **AND** the footer shows the count of selected files

#### Scenario: Footer reflects path-editing mode
- **WHEN** the file browser is editing a path
- **THEN** the footer shows only path-editing shortcuts such as completion, selection, confirm, and cancel-edit
- **AND** it hides normal browser actions that are not active while editing

#### Scenario: Browser footer dims unavailable current-context actions
- **WHEN** the file browser is in its normal navigation mode and a current-context action cannot currently run
- **THEN** that action remains visible in the footer in a dimmed style
- **AND** unrelated browser-editing shortcuts remain hidden

## ADDED Requirements

### Requirement: Browser editing mode gives printable keys to the path input
When the browser is in path-editing mode, printable keys SHALL be handled by the path input rather than interpreted as browser navigation or global help shortcuts.

#### Scenario: Typing a path does not trigger browser shortcuts
- **WHEN** the user is editing the browser path and types printable characters such as `h`, `j`, `k`, `l`, or `?`
- **THEN** those characters are inserted into the path input
- **AND** the browser does not interpret them as movement, hidden-file toggle, or full-help actions

### Requirement: Browser full help reflects the active browser context
The application SHALL provide contextual full help for the file browser popup when the browser is in its normal interaction state. The help SHALL list the current browser-context shortcuts and SHALL render unavailable current-context actions dimmed while keeping unrelated shortcuts hidden.

#### Scenario: User opens full help from normal browser mode
- **WHEN** the user presses `?` in the file browser popup while not editing the path
- **THEN** the application opens a help overlay showing the available browser shortcuts grouped by purpose

#### Scenario: Full help is unavailable while editing a path
- **WHEN** the browser path input is focused
- **THEN** pressing `?` SHALL be handled as text input rather than opening the full-help overlay
