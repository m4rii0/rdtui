## 1. Shortcut Architecture

- [ ] 1.1 Define shared shortcut/binding types for semantic actions, help labels, grouping, footer eligibility, ordering, and visibility predicates.
- [ ] 1.2 Add centralized capability predicates for action availability so dispatch and help use the same state checks.
- [ ] 1.3 Implement active shortcut collection by context layer: overlay, subcontext, surface, and global.

## 2. Per-Context Shortcut Modules

- [ ] 2.1 Extract main-list shortcuts into a dedicated module, including navigation, sort, add/import actions, and quit behavior.
- [ ] 2.2 Extract detail and managed-download shortcuts into dedicated modules with state-aware action visibility.
- [ ] 2.3 Extract popup shortcut modules for file selection, delete confirmation, target picker, overwrite confirmation, and URL display.
- [ ] 2.4 Extract file-browser shortcuts into dedicated modules, including normal navigation, visual mode, and path-editing subcontext.

## 3. Footer and Full Help

- [ ] 3.1 Replace handwritten footer strings with generated compact footer output from the active visible shortcuts.
- [ ] 3.2 Add a contextual full-help overlay opened with `?` and closed with `?` or `esc`.
- [ ] 3.3 Group full-help output by section and hide unavailable actions automatically.

## 4. Text Input and Search Behavior

- [ ] 4.1 Update search-mode shortcut handling so printable keys remain owned by the search input.
- [ ] 4.2 Update browser path-editing behavior so printable keys remain owned by the path input while editing.
- [ ] 4.3 Keep full-help unavailable or non-conflicting in text-entry contexts.

## 5. Verification

- [ ] 5.1 Add tests ensuring footer and full help are generated from the same active bindings.
- [ ] 5.2 Add tests ensuring unavailable actions are hidden in list, detail, download, and popup contexts.
- [ ] 5.3 Add tests for printable-key ownership in search and browser path-edit states.
- [ ] 5.4 Add tests for help overlay open/close behavior and context preservation.
