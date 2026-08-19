# Refactoring Plan 2: Consolidate Async Lifecycle Management

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

## Specific steps
1. Inventory all async jobs and their invalidation rules.
2. Extract a common “request lifecycle” abstraction used by decode, sort, scan, and vector render.
3. Replace ad hoc generation checks with a shared controller contract.
4. Keep cancellation responsibilities explicit and local to the owning job.
5. Ensure no work can finish into a superseded state without first checking validity.
6. Add targeted tests for stale requests and out-of-order completion.

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
