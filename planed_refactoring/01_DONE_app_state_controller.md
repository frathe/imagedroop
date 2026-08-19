# (DONE) Refactoring Plan 1: Extract an App-State Controller

## Objective
Separate the root application state from the concrete Fyne widgets so the app has a clear controller layer instead of one large `viewer` object owning both UI and domain state.

## Why this is first
`internal/ui/viewer.go` still owns too many responsibilities at once: the current file set, index, loading state, merge/sort preference state, window geometry, menu state, cached image references, and feature interactions. That makes it difficult to reason about lifecycle and makes new features more fragile.

## Accepted boundary
The working implementation establishes an unexported, package-local `appState` in `internal/ui`, not an exported
application-wide controller. It owns the loaded/displayed file lists, current index, sort mode, and merge mode.
`viewer` remains its façade and orchestration hub: it translates events into model changes and renders their effects
through the Fyne widgets.

This boundary intentionally excludes asynchronous scan/load/sort lifecycle (including generation and cancellation),
window geometry/session wiring, menu enablement/visibility, image/display cache state, and rendering. Those concerns
remain on `viewer` because they coordinate Fyne widgets or asynchronous work rather than describing the current file
model. Native file-picker and save-dialog glue is also out of scope; it remains in the existing `viewer` wrappers over
`internal/filepicker`.

Feature packages continue to declare their own narrow consumer-side `Host` interfaces (and `exifwin` its one callback).
No broad `Controller` interface is introduced, and `appState` is never handed to feature packages.

## Specific steps
1. Identify the true ownership boundary for the “viewer state” versus UI-only widgets.
2. Extract a small state struct for the app’s current model.
3. Move file/index behavior and mode toggles out of `viewer` and into the controller.
4. Keep `viewer` as a composition hub that renders the state and forwards events.
5. [x] Verify the existing narrow Host interfaces for `grid`, `settingswin`, `exifwin`, favorites, slideshow, and
   deletion remain the accepted consumer boundaries; no broad controller interface is required by the completed
   package-local state extraction.
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
