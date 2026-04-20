## Context

The TUI currently handles shortcuts primarily through a large `msg.String()` switch in the main model update flow, while footer hints are rendered separately in multiple places. This works for the current feature set, but it couples unrelated views together and requires behavior and help text to be maintained in parallel.

The application now has several distinct interaction surfaces:
- full-screen list, detail, and managed-download views
- popup overlays such as delete confirmation, target selection, file selection, overwrite confirmation, and URL display
- subcontexts such as search mode, batch mode, browser visual mode, and browser path editing

These do not all behave the same way, but today they are flattened into one routing path. That is the main design pressure behind the rework.

## Goals / Non-Goals

**Goals:**
- Make shortcuts internally modular so each surface can define and own its own mappings.
- Use a single source of truth for key dispatch, compact footer hints, and full help.
- Hide unavailable actions rather than advertising actions that will only be rejected.
- Add a `?` full-help overlay that reflects the current view or popup context.
- Preserve the existing keyboard-first workflow and current user-facing bindings unless a conflict requires refinement.
- Ensure text-entry states keep ownership of printable keys.

**Non-Goals:**
- User-configurable remapping or persisted keymap customization.
- Replacing the current visual style with the stock Bubbles help component look.
- Re-architecting the entire TUI into many fully independent Bubble Tea models in this change.
- Broad navigation redesign outside the shortcut/help system itself.

## Decisions

### 1. Use structured key bindings as the source of truth

The shortcut layer will use structured key binding definitions rather than raw string comparisons as the primary representation of shortcuts.

Each binding definition will carry:
- actual key matchers
- help label and description
- semantic action identity
- grouping for full-help presentation
- visibility rules
- whether the binding is eligible for compact footer display

Why this decision:
- it keeps dispatch and help text aligned
- it supports width-aware footer rendering and grouped full help
- it makes per-view composition straightforward

### 2. Keep the current `mode` model, but add shortcut contexts on top

This change will not replace the current `mode` enum with a brand-new navigation framework. Instead, it will layer shortcut contexts on top of the existing model structure.

Conceptually, the active shortcut resolution order will be:
1. overlay context
2. subcontext
3. surface context
4. global context

Examples:
- delete popup overrides the underlying list or detail view
- search mode refines main-view behavior
- browser path editing overrides normal browser navigation
- global bindings remain available only where they do not conflict

Why this decision:
- it is the smallest change that still fixes the architecture
- it avoids unnecessary churn in unrelated view code
- it fits the current return-mode and popup overlay design

### 3. Model full help as an orthogonal overlay, not a new navigation mode

The `?` help screen will be treated as an overlay state rather than another primary mode in the navigation stack.

That means:
- opening help does not replace the current view/popup context
- closing help returns to the exact same underlying state
- help can render over list, detail, managed-download, and richer popup contexts

Why this decision:
- it avoids entangling help with existing popup return flows
- it preserves context naturally
- it matches how users think about temporary help overlays

### 4. Hide unavailable actions using the same predicates that guard execution

Shortcut visibility will be determined by capability predicates that reflect current state.

Examples:
- file-selection shortcuts appear only when the selected torrent is waiting for file selection
- direct URL and managed download appear only for ready torrents
- managed-download completion actions appear only when the download is complete
- destructive actions appear only when there is something valid to delete

These same predicates must also guard execution.

Why this decision:
- it prevents help/behavior drift
- it reduces user confusion
- it makes tests more meaningful because availability is defined once

### 5. Use row-based state where possible for list-view shortcut visibility

In the main torrent list, visibility should generally be derived from the selected visible torrent row rather than depending entirely on asynchronously refreshed detail state.

Why this decision:
- list navigation should not make help flicker or lag while detail catches up
- the footer should reflect what the selected row implies immediately
- detail data can still be used for execution where necessary

### 6. Text-entry contexts own printable keys

When a text input is active, printable keys must go to that input before the shortcut system tries to interpret them as commands.

This applies especially to:
- search input
- token/magnet/url entry
- browser path editing

Why this decision:
- it fixes conflicts where letters like `j` and `k` currently act as movement instead of typed characters
- it makes the application behave like a real text UI instead of a command parser with accidental input capture

### 7. Keep custom footer styling and use a generated full-help overlay

The application already has a strong footer visual language. This change will preserve that style, but the content will be generated from active bindings instead of handwritten strings.

The full-help overlay will use the same binding data and current style system, grouped into labeled sections such as:
- Navigation
- Actions
- Selection
- Sort
- Global

Why this decision:
- it keeps the current look and feel
- it avoids re-theming the app around a stock help component
- it gives the app a scalable discoverability mechanism

## Risks / Trade-offs

- [Refactor touches many input paths] -> Minimize risk by changing shortcut dispatch and help rendering first, without redesigning unrelated business logic.
- [Visibility may still drift from execution] -> Centralize action predicates and use them in both help generation and handlers.
- [Footer crowding can return as features grow] -> Use footer eligibility and priority rather than trying to show everything in one line.
- [Help overlay could conflict with tiny confirm popups] -> Allow full help only on contexts where it adds real value; simple yes/no popups can keep just the footer if needed.
- [Search/browser editing regressions] -> Add explicit tests for printable-key ownership and editing-specific help output.

## Migration Plan

This is an internal refactor of the TUI interaction layer.

Rollout steps:
1. introduce shared shortcut definitions and visibility predicates
2. switch compact footer rendering to generated active shortcuts
3. split raw key dispatch into per-context handlers using the new shortcut layer
4. add the contextual `?` full-help overlay
5. update tests to cover dispatch/help alignment and text-entry conflicts

Rollback strategy:
- revert the shortcut-layer refactor and return to direct mode-based key handling
- no config or persisted user data migration is involved

## Open Questions

- Should the full-help overlay be available inside very small confirm popups, or only on richer screens and popups?
- Should the compact footer always reserve space for `? help`, even in narrow terminals where one action hint may need to drop?
