## Why

Current popup/dialog system is inconsistent: file selection and delete confirmation render as inline bordered boxes appended below content (not overlaid), while the file browser uses a properly centered overlay. This creates a disjointed UX — some popups feel "stuck to the bottom," file selection lacks size/context info, and delete confirmation is a bare `y/n` with no safeguards for batch operations. The reference repo (k9s) demonstrates a cleaner factory-based dialog pattern with consistent centered rendering, confirm-with-ack for dangerous ops, and reusable components.

## What Changes

- Replace inline modal rendering (`renderModal()` in `view.go`) with centered overlay popups using `lipgloss.Place(center, center)`, matching the existing file browser pattern
- Rework file-selection popup: centered overlay with file sizes, select-all/deselect-all/toggle-range controls, scroll indicators, and footer with keybinding hints
- Rework delete confirmation: centered overlay with clear danger styling; batch deletes show count + require explicit `y` confirmation; add optional confirm-with-ack pattern (type torrent name) for single destructive deletes
- Introduce reusable popup component system: factory functions for confirm, select, and info popups that handle centering, border styling, focus, and dismissal uniformly
- Standardize popup overlay rendering: all popups render background view first, then overlay centered popup on top
- Add flash/toast-style status messages for transient feedback (success, error, info) that auto-dismiss

## Capabilities

### New Capabilities
- `popup-system`: Reusable centered overlay popup component system with factory functions (confirm, multi-select, info), consistent border/styling, footer hints, and dismiss handling
- `flash-messages`: Transient status message overlay for success/error/info feedback that auto-dismisses after a timeout

### Modified Capabilities
- `torrent-management`: File selection and delete confirmation rendering changes from inline modal to centered overlay popup; enhanced file selection with sizes and bulk controls; enhanced delete with danger styling and confirm-with-ack

## Impact

- `internal/tui/view.go`: Remove `renderModal()`, add centered popup renderers
- `internal/tui/model.go`: Refactor mode handling for popup system; add flash message state
- `internal/tui/` (new files): `popup.go` for reusable popup components, `flash.go` for flash message system
- `openspec/specs/torrent-management/spec.md`: Updated requirements for file selection and delete confirmation UX
- `openspec/specs/import-popup-browser/spec.md`: Minor alignment with new popup system (may use shared overlay renderer)
