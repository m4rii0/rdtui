## 1. Shortcut Architecture

- [x] 1.1 Define shared shortcut/binding types for semantic actions, help labels, grouping, footer eligibility, ordering, and visibility predicates.
- [x] 1.2 Add centralized capability predicates for action availability so dispatch and help use the same state checks.
- [x] 1.3 Implement active shortcut collection by context layer: overlay, subcontext, surface, and global.

## 2. Per-Context Shortcut Modules

- [x] 2.1 Extract main-list shortcuts into a dedicated module, including navigation, sort, add/import actions, and quit behavior.
- [x] 2.2 Extract detail and managed-download shortcuts into dedicated modules with state-aware action visibility.
- [x] 2.3 Extract popup shortcut modules for file selection, delete confirmation, target picker, overwrite confirmation, and URL display.
- [x] 2.4 Extract file-browser shortcuts into dedicated modules, including normal navigation, visual mode, and path-editing subcontext.

## 3. Footer and Full Help

- [x] 3.1 Replace handwritten footer strings with generated compact footer output from the active visible shortcuts.
- [x] 3.2 Add a contextual full-help overlay opened with `?` and closed with `?` or `esc`.
- [x] 3.3 Group full-help output by section and hide unavailable actions automatically.

## 4. Text Input and Search Behavior

- [x] 4.1 Update search-mode shortcut handling so printable keys remain owned by the search input.
- [x] 4.2 Update browser path-editing behavior so printable keys remain owned by the path input while editing.
- [x] 4.3 Keep full-help unavailable or non-conflicting in text-entry contexts.

## 5. Verification

- [x] 5.1 Add tests ensuring footer and full help are generated from the same active bindings.
- [x] 5.2 Add tests ensuring unavailable actions are hidden in list, detail, download, and popup contexts.
- [x] 5.3 Add tests for printable-key ownership in search and browser path-edit states.
- [x] 5.4 Add tests for help overlay open/close behavior and context preservation.

## 6. Follow-up: Dimmed Unavailable Shortcuts

- [x] 6.1 Update shortcut specs so state-gated current-screen actions stay visible but dimmed instead of hidden.
- [x] 6.2 Update footer and full-help rendering to show unavailable current-context actions in a dimmed style while keeping unrelated shortcuts hidden.
- [x] 6.3 Keep disabled actions non-launchable and preserve inline error feedback when pressed.
- [x] 6.4 Add tests for dimmed shortcut rendering in footer and full help across list, browser, and download contexts.
