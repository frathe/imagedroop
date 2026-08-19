# (DONE) Refactoring Plan 2: Consolidate Async Lifecycle Management

## Objective
Centralize the app’s lifecycle rules for stale work, cancellation, and async completion so load/sort/scan/vector jobs follow one consistent contract.

## Why this is second
The project already has repeated patterns for stale-result protection: `gen`, `sortGen`, `vectorGen`, and multiple cancel functions like `scanCancel`, `loadCancel`, and `sortCancel`. That is a sign that lifecycle logic is duplicated in several places and is hard to reason about uniformly.

## Target design
Create a small lifecycle/orchestration helper that standardizes:
- generation or revision tracking
- stale-request rejection
- cancellation context ownership
- completion-handling semantics
- irreversible invalidation when a newer request supersedes an older one

The helper is package-local to `internal/ui` and has two layers:
- a zero-value revision primitive for capturing and comparing monotonically
  increasing revisions
- a request lifecycle that starts one current request, cancels the previous
  request's context, and returns a token combining that context with the
  captured revision

Load, scan, sort, and vector rendering each own a separate request lifecycle.
Decode retries, neighbor preloads, and GIF animation are descendants of one
load token rather than independent requests. A separate file-set revision is
exposed through `viewer.Generation` for grid thumbnail and deletion guards;
ordinary image navigation must not invalidate work whose indices still refer
to the same file set.

## Async inventory and invalidation matrix

| Owner | Work covered by one request | Superseded by | Must not be superseded by |
|---|---|---|---|
| Load | probe, decode, broken-file retry chain, neighbor preloads, GIF animation | newer navigation, a new drop, clearing files, shutdown | sort-only changes before they land; scan cancellation |
| Scan | direct-drop filtering or recursive directory walk and progress updates | newer drop, explicit scan cancellation, clearing files, shutdown | navigation within the existing set |
| Sort | `filesort.Order` and its state-writing callback | newer sort, file removal, clearing/replacing files, explicit sort cancellation, shutdown | navigation or vector rendering |
| Vector | debounce, SVG rasterization, and UI hand-off | newer render request, image/vector change, clearing files, shutdown | unrelated scan or sort work |
| File set | index-to-URI identity consumed by grid and deletion | replace/merge landing, reorder landing, removal, clear | navigation, decode retry before it removes a file, scan/sort merely starting |

Toast, slideshow, grid filtering/cell recycling, thumbnail workers, chooser,
clipboard, deletion, favorites, wallpaper, and window-position polling retain
their existing feature-local lifecycle contracts. They have additional
semantics that do not fit a single-current-request abstraction and are outside
this refactoring.

## Completion contract

1. Every background operation captures a request token and checks it before
   expensive work when practical and again immediately before applying a
   result through `fyne.Do`.
2. Starting or invalidating a lifecycle irreversibly advances its revision and
   cancels the previous token's context. Cancellation stops cooperative work;
   revision comparison remains the final stale-result guard.
3. A stale completion may only settle resources owned by that invocation. It
   must not hide a newer request's spinner, clear a newer in-flight flag, write
   model/widget state, or invoke a state-writing callback.
4. Per-invocation done channels are closed exactly once on success, error,
   cancellation, and stale completion. Existing WaitGroups call `Done` on
   every return path, including cancellation during vector debounce and while
   a preload is queued behind its semaphore.
5. Load descendants share the parent token and context. Finishing the visible
   decode does not invalidate that token because its preloads and animation
   remain legitimate until the next load invalidation.
6. UI-owned booleans such as `scanning` and `sorting` remain local presentation
   state. Only the current token's completion may finalize them; explicit
   cancellation finalizes them synchronously on the UI goroutine.

## Delegation stages

1. Introduce and unit-test the lifecycle and revision primitives.
2. Migrate load and scan together because they currently share `viewer.gen`;
   split their invalidation while preserving the load descendant chain.
3. Migrate sort independently, including stale spinner ownership.
4. Migrate vector rendering and replace its shutdown-only stop channel with
   lifecycle cancellation.
5. Remove legacy viewer fields, update test synchronization and
   `ARCHITECTURE.md`, then run normal and race-enabled suites.

## Specific steps
1. [x] Inventory all async jobs and their invalidation rules.
2. [x] Extract a common “request lifecycle” abstraction used by decode, sort, scan, and vector render.
3. [x] Replace ad hoc generation checks with a shared controller contract.
4. [x] Keep cancellation responsibilities explicit and local to the owning job.
5. [x] Ensure no work can finish into a superseded state without first checking validity.
6. [x] Add targeted tests for stale requests and out-of-order completion.

## Implemented result

- `internal/ui/lifecycle.go` owns the zero-value `revision`,
  `requestLifecycle`, and `requestToken` primitives.
- `viewer` owns separate load, scan, sort, and vector lifecycle instances plus
  a dedicated file-set revision returned by `Generation`.
- Load retries, preloads, and GIF animation share one token; cancellation wakes
  animation delays and preloads queued behind the semaphore.
- Scan cancellation no longer interrupts navigation through an existing set.
- Only the current sort token may finalize progress UI or invoke its callback;
  explicit invalidation finalizes its own UI synchronously.
- Vector invalidation replaces the shutdown-only stop channel and wakes
  superseded debounce waits.
- Targeted lifecycle regressions, `make test`, and `go test -race ./...` pass.

## Risks to watch
- Subtle races between UI goroutine and background decode goroutines.
- Cancellation semantics that silently skip legitimate finalization.
- Inconsistent stale-result checks across different features.

## Best suited agent
`go-expert`

## Success criteria
- One consistent invalidation model across all async loops.
- Fewer custom generation counters and cancellation wrappers.
- Less risk of stale UI updates after a newer file set or sort order is active.
