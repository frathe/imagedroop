# Extract the folder scan into `internal/filescan`

Implementation plan for item 3 in `todos.md` — "Extract the directory-scan
walker out of `handleDrop`". The deliverable is a viewer-independent walker
whose semantics (symlink-cycle guard, per-scan dedupe, the `maxScan` cap,
drop order) are provable in fast pure tests, leaving `handleDrop` as UI glue.

## Baseline (measured 2026-08-22, before any change)

| fact | value |
| --- | --- |
| `internal/ui/drop.go` | 366 lines |
| `handleDrop` itself | 173 lines (`drop.go:88-260`) |
| of which the scan closure | ~110 lines, an anonymous state machine inside a goroutine inside a viewer method |
| `internal/ui/drop_test.go` | 623 lines, 24 tests |
| tests that need a full viewer + Fyne test app + `drain` to assert pure walker logic | 5 |
| duplicate dedupe loops (`seen` map + `IsSupportedImage` + `realPathOf`) | 2 — the no-directories fast path and the goroutine |

## Probe result that shapes the plan

`storage.CanList` / `storage.List` fail with `no repository registered for
scheme 'file'` in a bare test binary. Fyne registers the file repository in
`test/driver.go`, so **`test.NewApp()` in `TestMain` is the entire
prerequisite** — no viewer, no window, no `drain`, no golden machinery.
Verified in a throwaway package, since deleted. `internal/filesort` and
`internal/imaging` already carry exactly this `TestMain` for the same reason,
with a comment noting that omitting it once went unnoticed until a Linux CI
run. The new package copies that precedent verbatim.

This is the whole payoff of the todo, and it is confirmed rather than assumed.

## Decisions taken

1. **Package is `internal/filescan`** — sibling of `internal/filesort` and
   `internal/imaging`, not a package-local file in `internal/ui`. The name
   pairs with `filesort`; the placement structurally prevents the walker's
   tests from ever regrowing a viewer dependency.
2. **The `maxScan` cap applies to both drop paths.** Today it caps only the
   recursive walk; a drop of 300 000 loose files bypasses it. One walker means
   one rule. This is a user-visible behavior change on a path nothing
   currently tests.
3. **Cancellation is `context.Context`, not `requestToken`** — mirroring
   `filesort.Order(ctx, mode, raw)`. `requestLifecycle.begin` already cancels
   the previous token's context, and `asyncOpUI.invalidate` does too, so ctx
   alone covers every supersession the current `token.current()` polling
   covers.
4. **`defaultMaxScannedFiles` moves too**, becoming `filescan.DefaultMax`.
   `startup.go` and two test files follow it.
5. **Walker logic moves down; UI-only assertions stay up.** Three
   viewer-based tests are deleted as redundant, one is trimmed to its toast
   assertion, and one folder-drop end-to-end is deliberately kept.
6. **No commits.** `AGENTS.md` forbids agents running `git commit`; a
   suggested message is at the end of this document.

## Two invariants that must survive

**Invariant A — result order is load-bearing.** The walker processes the
dropped URIs in order first, then pops directories off a **LIFO stack**
(`d := dirs[len(dirs)-1]; dirs = dirs[:len(dirs)-1]`). That produces a
specific traversal order which `filesort.Order` preserves verbatim under
`ByDropOrder`. Changing the stack to a queue would silently reorder every
user's "stupid sort". A test pins this.

**Invariant B — `IsSupportedImage` is never called on a directory.**
`storage.CanList` is checked *first* precisely because directories have no
extension and would otherwise fall through to `MimeType()`'s
open-and-sniff fallback. `imaging.IsSupportedImage`'s own doc comment records
that checking mime first "turned a recursive folder scan into thousands of
needless file opens". The `.DS_Store`-clutter assertion in the recursion test
is what guards this; it moves down with the test.

## Target API

```go
// Package filescan gathers displayable image files from a dropped or opened
// set of paths, recursing into directories.
package filescan

// DefaultMax caps how many images a single recursive folder scan will
// gather. (Was internal/ui's defaultMaxScannedFiles.)
const DefaultMax = 200_000

// Images walks uris and returns every supported image found, recursing into
// directories. truncated reports that the walk stopped at max rather than
// exhausting the tree.
func Images(ctx context.Context, uris []fyne.URI, max int, progress func(n int)) (images []fyne.URI, truncated bool)
```

Semantics the implementation must preserve exactly:

| aspect | rule |
| --- | --- |
| `max < 1` | floored to 1 inside `Images` (mirrors `SetMaxScan`'s floor; a 0 cap is not "unlimited") |
| truncation | set when the running count reaches `max` *after* appending, so `len(images) == max` exactly |
| dedupe | per call, keyed on `realPathOf` (symlink-resolved, falling back to `u.Path()`) |
| directory cycle guard | `visitedDirs` keyed the same way; a real path is descended into once |
| directory detection | `storage.CanList(u)` **before** `imaging.IsSupportedImage` (Invariant B) |
| order | dropped URIs first in order, then a LIFO directory stack (Invariant A) |
| `progress` | called with the running count, throttled `n == 1 \|\| n%10 == 0 \|\| truncated`; called on the walking goroutine, so marshaling to the UI is the caller's job; `nil` means no calls |
| `ctx` | checked per candidate and at the top of each directory pop; on cancellation returns whatever it has so far (the caller discards it) |
| errors | a `storage.List` error skips that directory and continues, as today |

## Target `handleDrop`

```go
maxScan := v.settings.maxScan   // snapshotted once, used by both paths

if !hasDirs {
    images, truncated := filescan.Images(token.context(), uris, maxScan, nil)
    fyne.Do(func() { v.applyScanResult(token, merging, uris, images, truncated, maxScan, scanDone) })
    return
}

go func() {
    images, truncated := filescan.Images(token.context(), uris, maxScan, func(n int) {
        fyne.Do(func() {
            if !token.current() { return }
            v.scanOp.label.SetText(fmt.Sprintf(lang.L("Scanning... %d images"), n))
        })
    })
    fyne.Do(func() { v.applyScanResult(token, merging, uris, images, truncated, maxScan, scanDone) })
}()
```

Three consequences, all intended:

- **The fast path passes `nil` progress.** It is synchronous and
  instantaneous; there is nothing to show, and this sidesteps calling
  `fyne.Do` from the UI goroutine entirely. Behavior identical to today.
- **The mid-walk early return disappears.** `drop.go:240-244` currently does
  `token.cancelContext(); close(scanDone); return`, skipping `fyne.Do`. Its
  own comment says this exists only to stop wasted I/O sooner, since the
  trailing `fyne.Do` "would discard the result anyway". The walker's `ctx`
  check now stops the I/O, so the goroutine always completes through
  `applyScanResult`, which already closes `scanDone` and bails on a stale
  token. One fewer hand-rolled path through the channel contract.
- **`maxScan` is snapshotted, fixing a latent race.** Today the background
  goroutine reads `v.settings.maxScan` live while the settings window can
  write it, and `SetMaxScan`'s doc comment already *claims* "one already in
  flight keeps running under whatever cap it started with" — which is
  currently false. Snapshotting makes the comment true and removes the race.
  `applyScanResult` takes the snapshot as a parameter so its truncation toast
  reports the cap the scan actually used.

## Test migration map

**New pure tests in `internal/filescan/filescan_test.go`** (`TestMain` =
`test.NewApp()`; helpers from `internal/uitest`, which imports nothing from
`internal/`, so no cycle):

| test | source |
| --- | --- |
| `TestImages_EmptyInput` | new |
| `TestImages_FiltersUnsupportedFiles` | from `TestHandleDrop_FiltersUnsupportedFiles` |
| `TestImages_RecursesIntoNestedDirectories` | from `TestHandleDrop_RecursesIntoNestedDirectories`, **including the `.DS_Store` clutter** (Invariant B) |
| `TestImages_SymlinkCycleDoesNotHang` | from `TestHandleDrop_SymlinkCycleDoesNotHang` |
| `TestImages_DedupesOverlappingDirectories` | from `TestHandleDrop_DedupesOverlappingDirectories` |
| `TestImages_DedupesDuplicateURIsInDirectDrop` | from `TestHandleDrop_DedupesDuplicateURIsInDirectDrop` |
| `TestImages_CapsAtMax` | from `TestHandleDrop_CapsFileCountForLargeTrees` (the count half) |
| `TestImages_CapAppliesToDirectFileDrop` | new — pins decision 2 |
| `TestImages_MaxFlooredAtOne` | new — `max` of 0 and -5 both yield 1 image |
| `TestImages_PreservesDropOrder` | new — pins Invariant A |
| `TestImages_ProgressThrottle` | new — records every `n`; asserts 1, every 10th, and a final call on truncation; and that `nil` is safe |
| `TestImages_ContextCancellationStopsWalk` | new — pre-cancelled ctx returns without walking |

**Deleted from `internal/ui/drop_test.go`** (fully covered above; the viewer
adds nothing to the assertion):

- `TestHandleDrop_SymlinkCycleDoesNotHang`
- `TestHandleDrop_DedupesOverlappingDirectories`
- `TestHandleDrop_DedupesDuplicateURIsInDirectDrop`

**Trimmed in `internal/ui/drop_test.go`:**

- `TestHandleDrop_CapsFileCountForLargeTrees` — keeps the folder drop and the
  truncation-toast assertions (toast visible, text mentions the cap); its
  doc comment now points at `filescan` for the cap semantics themselves.

**Deliberately kept as-is:**

- `TestHandleDrop_RecursesIntoNestedDirectories` — the only test proving the
  *async* path reaches the UI at all (3 files loaded, dropzone hidden). Its
  comment gains a pointer to the filescan test that owns the walker half.
- Everything else: empty drop, no supported images, error-after-images,
  format acceptance, merge/replace, `cancelScan` ×3, `clearToDropzone`,
  superseded goroutine, navigation, `MaxScan`/`SetMaxScan` getters.

## Execution steps

Each step is one subagent, one verification gate. I review the diff after
every step and fix it up before dispatching the next. Steps are strictly
sequential — 2 depends on 1, 3 on 2, 4 on 3.

---

### Step 1 — create `internal/filescan` (Sonnet)

**Additive only. No file outside `internal/filescan/` may be touched.**

Write `internal/filescan/filescan.go` and `internal/filescan/filescan_test.go`
implementing the Target API above. TDD: write the test file first, watch it
fail, then implement.

Lift the walker body from `internal/ui/drop.go:151-254` and `realPathOf`
from `drop.go:40-50`. Move the existing doc comments with the code they
describe — they are the most valuable thing being moved. Rewrite only the
sentences the move itself invalidates (references to `v.settings.maxScan`,
`token.current()`, `v.state.files`, `handleDrop`).

`TestMain` copies `internal/filesort/filesort_test.go:29-32` verbatim.

**Verify:** `go test -count=1 ./internal/filescan/` passes;
`go build ./...` and `go vet ./...` still clean; `git status --short` shows
only new files under `internal/filescan/`.

---

### Step 2 — rewire `handleDrop` (Sonnet)

From `internal/ui/drop.go`, delete the scan goroutine's closure state machine,
the fast path's duplicate dedupe loop, `realPathOf`, and the
`defaultMaxScannedFiles` const. Rewire both paths to `filescan.Images` per
the Target `handleDrop` above, including the `maxScan` snapshot and the new
`applyScanResult` parameter.

Update the two remaining references to the moved const: `startup.go:44` and
`drop.go`'s `MaxScan`/`SetMaxScan` doc comments (`SetMaxScan` keeps its own
floor — the walker's is defence in depth, not a replacement).

**No test file may be edited in this step.** The suite must stay green
unchanged; no existing test drops more than `maxScan` loose files, so
decision 2 is invisible here. If a test does fail, stop and report rather
than adjusting it — that is a signal the extraction changed behavior.

**Verify:**
`go test -count=1 -run 'TestHandleDrop|TestCancelScan|TestClearToDropzone|TestNavigation|TestMaxScan|TestSetMaxScan|TestToggleMergeMode|TestMergeMode' ./internal/ui/`
then the full `go test -count=1 ./internal/ui/`. Confirm
`grep -rn 'realPathOf\|defaultMaxScannedFiles' internal/ --include='*.go'`
returns only `_test.go` hits (step 3 clears those).

---

### Step 3 — prune `internal/ui/drop_test.go` (Sonnet)

Apply the "Deleted / Trimmed / Kept" columns of the test migration map, and
update the file's 20-line header comment: the three guards it describes
(symlink cycle, overlapping dedupe, `maxScan`) now live in
`internal/filescan`, and this file covers the path from a drop to a loaded
file set.

Update the two other test files that referenced the moved const:
`drop_test.go:365` and `preferences_wiring_test.go:173,201,237` →
`filescan.DefaultMax`.

**Verify:** `go test -count=1 ./internal/ui/` passes; `drop_test.go` is
21 tests; `grep -rn 'defaultMaxScannedFiles' internal/` is empty.

---

### Step 4 — documentation (Sonnet)

`ARCHITECTURE.md`, at these anchors:

- **line 59**, the `drop.go` row — the recursive walk and `realPathOf` are
  gone; say the walk lives in `internal/filescan` and that `drop.go` keeps
  `handleDrop` (UI glue), `applyScanResult`, `applyScannedFiles`,
  `cancelScan`, and the `MaxScan`/`SetMaxScan` binding.
- **a new `### internal/filescan` section**, placed immediately before
  `### internal/filesort` (line 393) so the two viewer-independent
  file-set packages sit together. Include the one-file responsibility table
  the other sections use, and record the `test.NewApp()` `TestMain`
  requirement.
- **line ~229**, `internal/preferences`'s note pointing at
  `internal/ui/drop.go`'s `defaultMaxScannedFiles` → `filescan.DefaultMax`.
- **line 512**, the index entry: "How does drag-and-drop / folder scanning
  work?" → `drop.go`'s `handleDrop` **plus** `internal/filescan`'s `Images`.
- **line ~417**, `internal/selection`'s "sits here beside `internal/filesort`"
  aside — extend to name `filescan` if it reads naturally; skip if forced.

`todos.md`: move item 3 from `## TODO` to `## Done` with a one-line summary.

`needs_refactoring.md`: no change — items 4 and 5 are untouched by this work.

**Verify:** `grep -n 'realPathOf\|defaultMaxScannedFiles' ARCHITECTURE.md`
is empty; `grep -c 'internal/filescan' ARCHITECTURE.md` ≥ 4.

---

### Step 5 — full verification (me, no subagent)

Match CI exactly, per `AGENTS.md`:

```sh
make fmt && go vet ./... && go build ./... && go test -race ./...
```

Plus a manual read of the complete diff against the invariants above, and
`make run` with a folder drop to confirm the spinner counter still ticks.

## Risks

| risk | mitigation |
| --- | --- |
| Traversal order changes, silently altering `ByDropOrder` | Invariant A pinned by `TestImages_PreservesDropOrder` in step 1, before any ui code moves |
| `IsSupportedImage` starts being called on directories, re-introducing thousands of file opens | Invariant B; the `.DS_Store` clutter assertion moves down in step 1 |
| A subagent "fixes" a failing ui test in step 2 instead of reporting the behavior change | Step 2's brief forbids editing test files at all |
| Removing the mid-walk early return changes the superseded-scan contract | `TestHandleDrop_SupersededScanGoroutineExits` is kept unchanged and still waits on `scanOp.done`; the normal path already proves `fyne.Do` → `applyScanResult` closes it |
| Doc drift between `ARCHITECTURE.md` and the code | Step 4's greps are mechanical, not judgement calls |

## Suggested commit message

```
Extract the folder scan into internal/filescan

handleDrop's recursive walk was a ~110-line anonymous state machine inside
a goroutine inside a viewer method, with a second copy of its dedupe loop
in the no-directories fast path. Its guards - the symlink-cycle visited
set, the per-scan dedupe, the maxScan cap - could only be tested through a
full viewer, a Fyne test app and the drain machinery.

filescan.Images(ctx, uris, max, progress) now owns the walk as plain data
in, images + truncated out, beside internal/filesort. handleDrop is UI
glue: snapshot merge mode and the cap, show the spinner, call the walker,
apply the result. Both drop paths share the one walker, so the maxScan cap
now also bounds a drop of loose files; the cap is snapshotted per scan,
making SetMaxScan's "one already in flight keeps running under whatever cap
it started with" true rather than racy.

Five walker tests no longer need a viewer.
```
