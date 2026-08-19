# Refactoring Plan 1: Extract an App-State Controller

## Objective
Separate the root application state from the concrete Fyne widgets so the app has a clear controller layer instead of one large `viewer` object owning both UI and domain state.

## Why this is first
`internal/ui/viewer.go` still owns too many responsibilities at once: the current file set, index, loading state, merge/sort preference state, window geometry, menu state, cached image references, and feature interactions. That makes it difficult to reason about lifecycle and makes new features more fragile.

## Target design
- Keep Fyne widgets as a thin presentation layer.
- Introduce a dedicated app-state/controller that owns:
  - loaded file list and current index
  - merge/sort/display mode state
  - load generation and cancellation state
  - window geometry and restore/session state
  - menu enablement and visibility decisions
- Let feature code talk to a narrow interface rather than reaching directly into the entire viewer object.

## Specific steps
1. Identify the true ownership boundary for the “viewer state” versus UI-only widgets.
2. Extract a small state struct for the app’s current model.
3. Move file/index behavior and mode toggles out of `viewer` and into the controller.
4. Keep `viewer` as a composition hub that renders the state and forwards events.
5. Narrow the Host interfaces for `grid`, `settingswin`, `exifwin`, favorites, slideshow, and others.
6. Ensure menu enablement and other derived UI state come from controller state instead of ad hoc viewer fields.

## Risks to watch
- Accidental split-brain state if widget code and controller code both mutate the same fields.
- Hidden feature behaviors that rely on direct access to viewer internals.
- Conflicting lifecycle assumptions between asynchronous loads and immediate UI updates.

## Best suited agent
`refactor-planner`

## Success criteria
- `viewer.go` is no longer the single source of truth for everything.
- Feature packages rely on explicit interfaces instead of a giant object.
- State transitions are easier to test in isolation.
