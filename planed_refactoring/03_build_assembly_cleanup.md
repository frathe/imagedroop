# Refactoring Plan 3: Separate App Assembly from App Behavior

## Objective
Reduce the size and cohesion of `internal/ui/build.go` by separating widget construction and app wiring from the actual behavior logic of the application.

## Why this is third
`build.go` is still doing a lot of assembly work in one place, which makes the app harder to evolve without touching a central wiring file. The project already has several feature packages; the next step is to make the assembly layer more declarative and less monolithic.

## Target design
- Keep `build.go` focused on composing the window and feature modules.
- Introduce a small assembly layer or feature registry that wires together:
  - app controller
  - feature windows
  - host interfaces
  - global shortcuts
  - shared UI overlays
- Move behavior-specific logic out of the central builder where it is not strictly infrastructure.

## Specific steps
1. Identify the pure assembly responsibilities in `build.go`.
2. Separate feature wiring from behavior logic.
3. Define a narrow “feature registration” pattern for options such as grid, slideshow, favorites, settings, help, and EXIF windows.
4. Reduce cross-feature coupling in construction time.
5. Keep startup and shutdown behavior explicit rather than buried inside widget setup.
6. Validate that the app still builds and renders the same way via the existing e2e test path.

## Risks to watch
- Over-abstracting the assembly layer and adding indirection with no gain.
- Reintroducing hidden dependencies between features when they are assembled.
- Creating a second source of initialization logic that drifts from runtime behavior.

## Best suited agent
`general-purpose`

## Success criteria
- `build.go` is smaller and easier to follow.
- New features can be added by composing modules rather than editing one large builder.
- Behavior and construction responsibilities are cleaner and less entangled.
