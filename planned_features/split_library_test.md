# Split `internal/ui/library_test.go`

Implementation plan for the open item in `todos.md`. Pure test-file motion:
no production file is touched, no test is added, removed, renamed or
rewritten. The deliverable is navigability — today "where is the test for X?"
has two answers, `x_test.go` and "somewhere in library_test.go".

## Baseline (measured 2026-08-21, before any change)

| fact | value |
| --- | --- |
| `internal/ui/library_test.go` | 2,428 lines, 90 top-level declarations |
| tests in it | 69 (plus `TestMain`) |
| shared helpers in it | 20 (plus `TestMain`) |
| other test files in the package | 23, of which **21 depend on those helpers** |
| decls across all `internal/ui/*_test.go` (imports excluded) | 342 |
| `go test -count=1 ./internal/ui/` | ~10 s |
| `go test -race -count=1 ./internal/ui/` | ~2 m 27 s |

Helper reach, for a sense of the blast radius: `newTestViewer` is used in 17
files, `dropAndWait` in 16, `waitUntilLoaded` in 15, `settleToast` in 9.
`testApp`, `drain`, `waitForCached`, `waitForAnimFrame`, `waitForAnimStopped`,
`assertEquivalentFileSlices`, `assertValidFileIndex` and `namesOfURIs` are
used only from inside `library_test.go` today; per the todo they all move to
the harness anyway, so the harness stays the one place a new shared helper
lands.

## Decisions taken

1. **Granularity — fine.** 11 new test files plus the renamed harness.
2. **Motion is verbatim, with one narrow allowance.** Declaration bodies move
   byte-identical. Comments move with their declaration. The only comment
   edits permitted are cross-references the move itself invalidated. New
   content is limited to each file's header comment and its section dividers.
3. **The five viewer-level file-state invariants get their own
   `filestate_test.go`.** `state_test.go` stays what it is: pure `appState`
   unit tests with no viewer and no Fyne app.
4. **Verification is fast per step, race once at the end.**
5. **`library_test.go` is renamed, not deleted** — see "Why the harness moves
   last" below.
6. **No commits.** `AGENTS.md` forbids agents running `git commit`; a
   suggested message is at the end of this document for the user to run.

## Two rules that do most of the work

**Rule A — preserve original relative order.** Within each new file,
declarations appear in the same order they appear in `library_test.go` today.
This is not cosmetic. Six doc comments say "tested directly above", "already
covered above", "the note on X below". All six were checked against the
target mapping: every one of them refers to a declaration landing in the
*same* file, so preserving order keeps all six accurate and reduces the
permitted comment edits to approximately zero.

**Rule B — the harness moves last, by `git mv`.** Rather than lifting the
harness out first, steps 1–11 move the *tests* out. What remains in
`library_test.go` after step 11 is exactly the harness, and step 12 renames
that remainder with `git mv library_test.go harness_test.go`. This matters
for history: a 2,428→450-line extraction sits far below git's 50 % rename
threshold and would break `git blame` on the most-depended-on code in the
package, whereas renaming a file that is already 100 % its final content is
detected as a rename. It also inverts the risk profile — the step touching
the code 21 files depend on becomes a rename with zero content change,
instead of the riskiest step in the plan.

## Verification harness

Correctness here is not "the tests still pass" — a subagent could silently
weaken a test and the suite would stay green. The invariant is that **no
declaration's code changes**, and that is checked mechanically.

`declhash` parses every `*_test.go` in the package with `go/parser` *without*
`parser.ParseComments`, so comments are never attached and `go/printer`
renders code only. It emits `<decl name> <sha256 of comment-free source>`,
sorted, with import blocks skipped (those legitimately differ per file).

Three properties were proven against this repo before writing this plan:

| probe | result |
| --- | --- |
| A comment-only edit to a test file | **no hash drift** — comment-insensitive |
| Moving a test to a different file in the package (gofmt + goimports applied) | **no hash drift** — move-insensitive |
| Changing `index: 4` to `index: 5` in one test body | **exactly that one decl's hash changed** |

So the diff is empty for a correct move, and non-empty in precisely the
cases we care about. The working tree was restored after each probe.

`$SCRATCH` throughout this document means the session scratchpad directory:

```sh
SCRATCH=/private/tmp/claude-502/-Users-ronin-Projects-picfetch/2c62330d-0868-4353-8cc5-283c45ffc5cb/scratchpad
```

The tool lives there, not in the repo — it is review
tooling for this refactor, not something the project ships:

```
$SCRATCH/declhash.go        # the tool (source below)
$SCRATCH/baseline.txt       # 342 lines, captured in step 0
```

<details>
<summary><code>declhash.go</code></summary>

```go
// Command declhash prints one line per top-level declaration in a package's
// _test.go files: <decl name> <TAB> <sha256 of its comment-free source>.
package main

import (
	"crypto/sha256"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	dir := os.Args[1]
	files, err := filepath.Glob(filepath.Join(dir, "*_test.go"))
	if err != nil {
		panic(err)
	}
	var out []string
	fset := token.NewFileSet()
	for _, f := range files {
		// No parser.ParseComments: comments are never attached, so the
		// printer renders code only.
		af, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			panic(err)
		}
		for _, d := range af.Decls {
			if g, ok := d.(*ast.GenDecl); ok && g.Tok == token.IMPORT {
				continue // import blocks legitimately differ per file
			}
			var buf strings.Builder
			if err := printer.Fprint(&buf, fset, d); err != nil {
				panic(err)
			}
			out = append(out, fmt.Sprintf("%-64s %x", declName(d), sha256.Sum256([]byte(buf.String()))))
		}
	}
	sort.Strings(out)
	fmt.Println(strings.Join(out, "\n"))
}

func declName(d ast.Decl) string {
	switch t := d.(type) {
	case *ast.FuncDecl:
		if t.Recv != nil && len(t.Recv.List) > 0 {
			return "method:" + t.Name.Name
		}
		return "func:" + t.Name.Name
	case *ast.GenDecl:
		var names []string
		for _, s := range t.Specs {
			switch sp := s.(type) {
			case *ast.ValueSpec:
				for _, n := range sp.Names {
					names = append(names, n.Name)
				}
			case *ast.TypeSpec:
				names = append(names, sp.Name.Name)
			}
		}
		return t.Tok.String() + ":" + strings.Join(names, ",")
	}
	return "?"
}
```

</details>

### The check every step must pass

```sh
gofmt -l internal/ui                             # must print nothing
goimports -l internal/ui                         # must print nothing
go vet ./internal/ui/                            # must be clean
go run $SCRATCH/declhash.go internal/ui > $SCRATCH/after.txt
diff $SCRATCH/baseline.txt $SCRATCH/after.txt    # MUST BE EMPTY
go test -count=1 ./internal/ui/                  # must pass, ~10 s
```

A non-empty `diff` is a hard stop, not a judgement call.

## Step table

Ordered smallest-first so the recipe is validated on a 2-test file before it
is applied to a 22-test one. Every step is **sequential** — they all edit
`library_test.go`, so none of them can run in parallel. Model is Sonnet
throughout: each step is mechanical motion plus one header comment, well
inside Sonnet's range, and the decl-hash check catches any slip regardless of
model.

| # | new file | tests | ~lines | model |
| --- | --- | --- | --- | --- |
| 0 | *(me)* baseline capture | — | — | — |
| 1 | `windowtrack_test.go` | 2 | ~35 | Sonnet |
| 2 | `windowsize_test.go` | 3 | ~75 | Sonnet |
| 3 | `memlimits_test.go` | 3 | ~110 | Sonnet |
| 4 | `animate_test.go` | 3 | ~105 | Sonnet |
| 5 | `reset_test.go` | 1 | ~45 | Sonnet |
| 6 | `info_test.go` | 5 | ~170 | Sonnet |
| 7 | `filestate_test.go` | 5 | ~185 | Sonnet |
| 8 | `imgcache_test.go` | 8 | ~225 | Sonnet |
| 9 | `sort_test.go` | 8 | ~320 | Sonnet |
| 10 | `load_test.go` | 9 | ~260 | Sonnet |
| 11 | `drop_test.go` | 22 | ~500 | Sonnet |
| 12 | `git mv` → `harness_test.go` | 0 | ~450 | Sonnet |
| 13 | *(me)* docs + full verify | — | — | — |

69 tests distributed; 2 + 3 + 3 + 3 + 1 + 5 + 5 + 8 + 8 + 9 + 22 = 69. ✅

## Per-step manifests

Declaration names are exhaustive and authoritative. A step moves exactly
these, each with its leading doc comment, in this order.

### Step 0 — baseline (me, before dispatching anything)

- Confirm the working tree is clean apart from the three known entries
  (`needs_refactoring.md`, `todos.md`, `agent_ignore_this_idea_collection.md`).
- Write `declhash.go` to the scratchpad, run it, save `baseline.txt`
  (expect 342 lines).
- Record `go test -count=1 ./internal/ui/` green at ~10 s.

### Step 1 — `windowtrack_test.go` (mirrors `windowtrack.go`)

- `TestStartWindowPosPolling_TestDriverGetsNoopStop`
- `TestStartWindowPosPolling_PanicsWithoutConstructedSlideshow`

Carries the `// --- stage-2 stop signals ---` divider? **No** — that divider
also covers `TestInvalidateLoad_WakesAnimateImmediately`, which goes to
`animate_test.go` in step 4. Drop the divider; the file header replaces it.

Header must note that `TestWindowSizeTracker_RecordsResizes` — the other
`windowtrack.go` test — deliberately stays in `preferences_wiring_test.go`,
because it is about geometry persistence rather than the poller.

### Step 2 — `windowsize_test.go` (mirrors `load.go`'s resize/cap block)

- `TestResizeToImage`
- `TestMaxWindowSizeGetterSetter`
- `TestSetMaxWindowSize_FloorsAtTheDropZoneSize`

Order is load-bearing: `TestMaxWindowSizeGetterSetter`'s comment says "the cap
`resizeToImage` enforces (tested directly above)".

Header must distinguish this file from `zoom_test.go`, which owns
`syncWindowToZoom` — both resize the window, for different reasons.

### Step 3 — `memlimits_test.go` (mirrors `memlimits.go`)

- `TestMemoryLimitGettersAndSetters`
- `TestMemoryLimitSetters_FloorAtOne`
- `TestSetMaxImageCacheMBRetunesTheVectorRasterCeiling`

Carries the `// --- the memory-limit getter/setter pairs ---` divider content
into the header. Header must say the *behaviour* these limits produce is
tested in `imgcache_test.go`; this file is the settings-window binding
surface only.

### Step 4 — `animate_test.go` (mirrors `load.go`'s `animate`)

- `TestViewerShow_AnimatesGIF`
- `TestViewerShow_NavigatingAwayStopsAnimation`
- `TestInvalidateLoad_WakesAnimateImmediately`

### Step 5 — `reset_test.go` (mirrors `viewer.go`'s `reset`/`clearToDropzone`)

- `TestViewerReset`

Header must point at the three sibling reset tests that stay where they are
and why: `TestViewerReset_ReshowsRestoreLinkWhenSessionUnconsumed` and
`TestViewerReset_DoesNotReshowRestoreLinkOnceConsumed` in `session_test.go`,
`TestClearToDropzone_HidesInfoCardButKeepsThePreference` in `info_test.go`,
`TestClearToDropzone_PurgesTheImageCache` in `imgcache_test.go`. Each is
about what a *feature* does across a reset, not about reset itself.

### Step 6 — `info_test.go` (mirrors `info.go`)

- `TestFormatFileSize`
- `TestToggleInfoOverlay_HiddenUntilAnImageIsLoaded`
- `TestToggleInfoOverlay_ContentAndPersistenceAcrossNavigation`
- `TestToggleInfoOverlay_ZoomLineTracksZoomChanges`
- `TestClearToDropzone_HidesInfoCardButKeepsThePreference`

Carries the `// --- info overlay (I) ---` divider content into the header.

### Step 7 — `filestate_test.go` (viewer-level invariants over `state.go`)

- `TestViewerFileStateSlicesRemainEquivalentAcrossTransitions`
- `TestViewerIndexStaysValidAcrossFileStateTransitions`
- `TestViewerModesApplyBeforeAndAfterLoadingFiles`
- `TestStaleFileStateCompletionsDoNotOverwriteNewerState`
- `TestGenerationTracksFileSetIdentityNotNavigation`

**Do not move** `assertEquivalentFileSlices`, `assertValidFileIndex` or
`namesOfURIs` — they are harness helpers and stay in `library_test.go` for
step 12, even though only these five tests call them today. That is the
todo's explicit instruction and it keeps one home for shared helpers.

Header must draw the line against `state_test.go`: that file tests `appState`
as a plain struct with no viewer and no Fyne app; this one tests the
invariants the viewer must hold across scan/sort/remove transitions.

### Step 8 — `imgcache_test.go` (`load.go`'s preload + byte budget)

- `TestFinishLoad_PreloadsBothNeighbors`
- `TestAttemptLoad_CacheHitServesFileRemovedFromDisk`
- `TestRemoveFile_PurgesCacheEntry`
- `TestAttemptLoad_DisplaysAnImageLargerThanTheWholeCacheBudget`
- `TestPreloadOne_SkipsANeighborTooLargeForTheBudget`
- `TestAttemptLoad_ReportsAFileTooLargeToOpen`
- `TestAttemptLoad_ToastsAndFallsBackToAStaticFrameForAnOversizedAnimation`
- `TestClearToDropzone_PurgesTheImageCache`

Two dividers fold into the header here: `// --- imgCache / speculative
preloading ---` and `// --- byte-bounded image memory ---`. Order is
load-bearing: `TestPreloadOne_SkipsANeighborTooLargeForTheBudget`'s comment
opens "covers the other completion criterion", which only reads correctly
after `TestAttemptLoad_DisplaysAnImageLargerThanTheWholeCacheBudget`.

`waitForCached` stays in `library_test.go` for step 12.

### Step 9 — `sort_test.go` (mirrors `sort.go`)

- `TestHandleDrop_NaturalSortsByDefault`
- `TestToggleSort_CyclesThroughAllModesAndBackToName`
- `TestSetSortMode_JumpsDirectlyRatherThanCycling`
- `TestSetSortMode_SafeWithNoFilesLoaded`
- `TestInvalidateSortCancelsAndFinalizesCurrentProgress`
- `TestSetSortMode_SnapshotDoesNotAliasUnsortedFiles`
- `TestHandleKeyEvent_EscapeDuringFirstDropReorderDoesNotCloseWindow`
- `TestHandleKeyEvent_EscapeDuringResortOfExistingFilesDoesNotClearThem`

Carries the `// --- sorting ---` divider content into the header. Order is
load-bearing for `TestSetSortMode_JumpsDirectlyRatherThanCycling` ("already
covered above").

**The one comment edit the plan actually expects.**
`TestHandleKeyEvent_EscapeDuringFirstDropReorderDoesNotCloseWindow`'s comment
ends "...the same approach `TestCancelScan_CancelsInFlightScanWithNoFilesYet`
uses for the gathering phase." That test lands in `drop_test.go` in step 11.
The reference stays *true* (Go tests are package-scoped), but add the file
hint. This is the entire scope of the "light cleanup" allowance.

### Step 10 — `load_test.go` (mirrors `load.go`'s decode path)

- `TestInvalidateLoad_CancelsPriorLoadContext`
- `TestInvalidateLoad_ZeroValueIsSafe`
- `TestShowImage_StartsLoadLifecycle`
- `TestViewerShow_LoadsAndNavigates`
- `TestViewerShow_DecodeErrorKeepsHint`
- `TestViewerShow_RejectsAbsurdHeaderDimensions`
- `TestViewerShow_AutoAdvancesPastBrokenFileDuringNavigation`
- `TestViewerShow_AutoAdvancesPastBrokenFirstFile`
- `TestViewerShow_AllFilesBrokenFallsBackToEmptyState`

Carries the `// --- invalidateLoad (load-request cancellation) ---` divider.

The block comment under `// --- viewer.ShowImage (async decode path) ---`
(the ten lines explaining why waiting on `loadDone`/`scanDone` gives a
happens-before relationship under the test driver's inline `fyne.Do`) explains
why the *wait helpers* exist. It belongs to the harness — **leave it in
`library_test.go`** for step 12, which folds it into the harness header.

### Step 11 — `drop_test.go` (mirrors `drop.go`)

Largest step. Four groups, all keeping their original relative order.

*Synchronous `handleDrop` behaviour* — carries `// --- viewer.handleDrop
(synchronous behaviour) ---`:
- `TestHandleDrop_EmptyDrop`
- `TestHandleDrop_NoSupportedImages`
- `TestHandleDrop_ErrorAfterImagesClearsDisplay`
- `TestHandleDrop_FiltersUnsupportedFiles`
- `TestHandleDrop_AcceptsPNGAndGIF`

*Merge vs. replace* — carries `// --- handleDrop merge vs. replace (M toggles
merge mode) ---`:
- `TestHandleDrop_SecondDropWithoutMergeModeReplaces`
- `TestHandleDrop_MergeModeMergesIntoExistingSet`
- `TestHandleDrop_MergeModeDropWithNothingSupportedKeepsExistingSet`
- `TestToggleMergeMode_PrefixesTitleAndPersistsAcrossDrops`
- `TestMergeModeGetterSetter`

*Directory recursion and the scan cap*:
- `TestHandleDrop_RecursesIntoNestedDirectories`
- `TestHandleDrop_SymlinkCycleDoesNotHang`
- `TestHandleDrop_CapsFileCountForLargeTrees`
- `TestMaxScanGetterSetter`
- `TestSetMaxScan_FloorsAtOne`

*Scan cancellation and dedupe*:
- `TestCancelScan_NoOpWhenNotScanning`
- `TestCancelScan_CancelsInFlightScanWithNoFilesYet`
- `TestCancelScan_PreservesExistingFilesInMergeMode`
- `TestHandleDrop_SupersededScanGoroutineExits`
- `TestNavigationDoesNotInvalidateScan`
- `TestHandleDrop_DedupesOverlappingDirectories`
- `TestHandleDrop_DedupesDuplicateURIsInDirectDrop`

Note `toggleMergeMode`/`SetMergeMode` live in `viewer.go`, not `drop.go`.
Their tests belong here anyway because the behaviour they assert is entirely
about how a drop composes with the existing set; the header should say so, so
nobody "fixes" it later.

Two order dependencies to preserve: `TestMergeModeGetterSetter` ("already
covered above" → `TestToggleMergeMode_...`), `TestMaxScanGetterSetter` ("above
exercises by writing it directly" → `TestHandleDrop_CapsFileCountForLargeTrees`).
And `TestCancelScan_CancelsInFlightScanWithNoFilesYet` refers to
`TestHandleDrop_SupersededScanGoroutineExits` "below" — satisfied by the group
order above.

### Step 12 — rename the remainder to `harness_test.go`

At this point `library_test.go` should contain **exactly** these 21
declarations and nothing else:

`testApp`, `TestMain`, `newTestUI`, `drain`, `newTestViewer`, `testTimeout`,
`dropAndWait`, `dropAndWaitScan`, `waitUntilLoaded`, `waitForScan`,
`waitForSort`, `settleToast`, `settleChooser`, `settleSlideshow`,
`warmThumbs`, `waitForAnimFrame`, `waitForAnimStopped`, `waitForCached`,
`assertEquivalentFileSlices`, `assertValidFileIndex`, `namesOfURIs` —
20 helpers plus `TestMain`. (90 original decls = 69 tests + `TestMain` +
20 helpers.)

The step:
1. `git mv internal/ui/library_test.go internal/ui/harness_test.go`
2. Group the helpers into ordered sections: construction (`testApp`,
   `TestMain`, `newTestUI`, `drain`, `newTestViewer`), waits
   (`testTimeout`, `dropAndWait*`, `waitUntilLoaded`, `waitForScan`,
   `waitForSort`, `waitForCached`, `waitForAnim*`), settles (`settleToast`,
   `settleChooser`, `settleSlideshow`, `warmThumbs`), assertions
   (`assertEquivalentFileSlices`, `assertValidFileIndex`, `namesOfURIs`).
   Reordering *is* permitted here and only here — there are no `above`/`below`
   references among the helpers except `testTimeout`'s "every wait helper
   below", which the section order above keeps true.
3. Write the file header, folding in the surviving `// --- viewer.ShowImage
   (async decode path) ---` block comment about the happens-before discipline
   and the `// --- test viewer construction ---` divider.
4. Confirm `git status` reports the change as a **rename**, not
   delete + add. If it does not, the file still contains something it
   shouldn't — stop and report.

### Step 13 — docs and full verification (me)

- `AGENTS.md:27` — "add it to `newTestUI`'s `drain` cleanup in
  `internal/ui/library_test.go`" → `internal/ui/harness_test.go`.
- `ARCHITECTURE.md:109` — "`waitFor*`/`dropAndWait` channel helpers in
  `library_test.go`" → `harness_test.go`.
- `ARCHITECTURE.md:579` — "`newTestUI(t)` + `dropAndWait` in
  `library_test.go`" → `harness_test.go`.
- `todos.md` — move the item from **TODO** to **Done** with a short summary
  in the established voice.
- Full check: `gofmt -l .`, `go vet ./...`, `go build ./...`,
  `go test -race -count=1 ./...`, then `make verify`.
- Final `declhash` diff against `baseline.txt` — still empty.

## Subagent brief template

Each step is dispatched with this brief, with the bracketed parts filled in
from the manifest above.

> You are performing one step of a mechanical test-file split in
> `/Users/ronin/Projects/picfetch`. Read `planned_features/split_library_test.md`
> first — you are executing **Step N**.
>
> Move these declarations out of `internal/ui/library_test.go` into a new file
> `internal/ui/[target]_test.go`, in exactly this order: [list].
>
> **Hard rules**
> - Move each declaration together with its leading doc comment, byte for
>   byte. Do not reword, reformat, rename, split, merge or "improve" any test
>   body or doc comment.
> - Preserve the relative order given above — doc comments say "above" and
>   "below" and the order keeps them true.
> - Touch no file other than `internal/ui/library_test.go` and the new file.
>   No production file. No other test file. No docs.
> - Do not run `git commit`.
>
> **What you do add:** a file header doc comment in the style of
> `internal/ui/keys_test.go` and `internal/ui/grid_test.go` — a short
> paragraph saying what belongs in this file and what deliberately lives
> elsewhere. [Per-step header guidance from the manifest.] Section dividers in
> the existing `// --- name ---------` style where the manifest calls for
> them.
>
> **Before you report done, run and paste the output of:**
> ```sh
> gofmt -l internal/ui
> goimports -l internal/ui
> go vet ./internal/ui/
> go run $SCRATCH/declhash.go internal/ui > $SCRATCH/after.txt
> diff $SCRATCH/baseline.txt $SCRATCH/after.txt
> go test -count=1 ./internal/ui/
> ```
> The `diff` **must be empty**. A non-empty diff means you changed code that
> should only have moved — fix it before reporting, and if you cannot, report
> the diff rather than a success.

## My review between every step

Not a rubber stamp — the subagent's own report is evidence, not proof:

1. Re-run the check block myself. I trust my own output, not the paste.
2. `git diff --stat` — expect exactly two files touched, and the line count
   removed from `library_test.go` to match the line count added to the new
   file within the header-comment allowance.
3. `git diff internal/ui/library_test.go` — every hunk must be a pure
   deletion. Any `+` line in that file outside the import block is a red flag.
4. Read the new file's header comment. This is the part no checker can verify
   and the part that carries the actual value of the refactor. Rewrite it
   myself if it is vague, wrong about what lives elsewhere, or off-voice.
5. Check the import block of both files for leftovers `goimports` kept alive
   for the wrong reason.

## Risks

| risk | mitigation |
| --- | --- |
| A subagent "improves" a test while moving it | `declhash` diff catches any code change; a non-empty diff is a hard stop |
| A helper gets copied instead of moved | Duplicate declaration → compile error, immediately |
| A doc comment's "above"/"below" goes stale | Rule A: relative order preserved. All six references verified to land in-file |
| `git blame` breaks on the harness | Rule B: `git mv` at 100 % similarity in step 12 |
| Import blocks drift | `goimports -l` must print nothing at every step |
| A step lands a race the fast check misses | Full `-race` at step 13; steps are pure motion so the risk is near-zero by construction |
| Two steps run in parallel and collide | They are sequential by design; every step edits `library_test.go` |

## Suggested commit message (for the user to run at the end)

```
Split internal/ui/library_test.go into per-feature test files

The package's shared test harness lived in the middle of a 2,428-line
file surrounded by 69 tests that predated the per-feature test files,
so "where is the test for X?" had two answers and every new shared
helper landed in the monolith.

The 69 tests move out to the feature files they belong to -
drop_test.go, sort_test.go, load_test.go, imgcache_test.go,
animate_test.go, info_test.go, filestate_test.go, memlimits_test.go,
windowsize_test.go, windowtrack_test.go and reset_test.go, each with a
header saying what it owns and what deliberately lives elsewhere - and
what remains is renamed to harness_test.go, keeping git history on the
code 21 of the package's test files depend on.

Pure motion: every declaration's code is byte-identical, verified by
comparing comment-free per-declaration hashes before and after.
```
