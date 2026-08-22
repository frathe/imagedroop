# Split `internal/ui/grid/grid.go`

> **For agentic workers:** each stage below is dispatched to a fresh subagent
> by the orchestrating session, which reviews the diff before the next stage
> starts. Steps use checkbox (`- [ ]`) syntax for tracking.

Implementation plan for item 4 in `todos.md` — "Split `internal/ui/grid/grid.go`
(995 lines, one file)" — plus the stretch goal that item names, which this plan
promotes to a real stage (stages 9–12) at the user's request.

**Goal.** Turn one 995-line production file and one 1,154-line test file into
four production files and five test files inside the same package, with **zero
change to behavior and zero change to the exported API**; then collapse the two
hand-rolled bounded-decode pools (grid thumbnails, viewer preload) onto one
shared type.

**Architecture.** Stages 1–8 are pure motion: declarations move byte-identical
between files in package `grid`. Nothing is renamed, nothing changes visibility,
no `Overview` field moves (a struct cannot span files). Stages 9–12 are a real
refactor: a new viewer-independent package `internal/decodepool` holding the
semaphore + in-flight-claim + WaitGroup trio that `internal/ui/grid` and
`internal/ui`'s preload path each grew independently.

**Tech stack.** Go 1.x, Fyne v2, no new third-party dependencies.

**Spec.** `todos.md` item 4 (the file split and its stretch goal). Repo rules
that constrain every stage: `AGENTS.md`. Package map to update: `ARCHITECTURE.md`.

---

## Global Constraints

Copied from `AGENTS.md`; every stage's requirements implicitly include these.

- **No `git commit`.** Agents never commit. Suggested commit messages are at the
  end of this document for the user to run.
- **No `TODO`/`FIXME` comments in source.** Open work belongs in `todos.md`.
- **`ARCHITECTURE.md` is updated in the same change** when packages are added or
  files move. Stage 8 covers the split; stage 12 covers the new package.
- **Verification is the Makefile / CI order:** `make fmt`, `go vet ./...`,
  `go build ./...`, then `go test -race ./...` from the repository root.
- **Every user-visible string stays `lang.L("English text")`** with the key
  present in every `translations/*.json`. No stage here adds or changes a
  string; if one appears in a diff, it is a mistake.
- **No mutable package-level test seams.** Nothing in this plan adds one.
- Branch: work continues on **`extract-filescan`** (user's choice), on top of
  `d51c156`.

---

## Baseline (measured 2026-08-22, before any change)

| fact | value |
| --- | --- |
| `internal/ui/grid/grid.go` | 995 lines, 36 top-level declarations |
| `internal/ui/grid/grid_test.go` | 1,154 lines, 50 tests + `TestMain` + 6 helpers |
| `internal/ui/grid/selection.go` | 204 lines — **not touched by stages 1–8** |
| `internal/ui/grid/selection_test.go` | 383 lines, 24 tests — **not touched** |
| `go test -count=1 ./internal/ui/grid/` | 0.6 s |
| test + subtest result lines | 76 |
| `go doc -all ./internal/ui/grid` | 245 lines |
| grid decode pool | `sem chan struct{}` (cap 4) + `pending sync.WaitGroup` + `inflight sync.Map` |
| viewer preload pool | `preloadSem chan struct{}` (cap 2) + `preloadPending sync.WaitGroup` + `preloading sync.Map` |

Baseline artifacts captured before stage 1, used as the invariant in every
stage's review gate:

```
/private/tmp/claude-502/-Users-ronin-Projects-picfetch/dad3c0dc-0dcf-4675-9fc9-c1299e023aee/scratchpad/grid.doc.baseline     # go doc -all ./internal/ui/grid
/private/tmp/claude-502/-Users-ronin-Projects-picfetch/dad3c0dc-0dcf-4675-9fc9-c1299e023aee/scratchpad/grid.tests.baseline   # sorted --- PASS/FAIL lines, 76 of them
```

Regenerate either from a clean tree at any time:

```bash
go doc -all ./internal/ui/grid
go test -count=1 -v ./internal/ui/grid/ 2>/dev/null | grep -E '^(--- |    --- )' | sed 's/ (.*//' | sort
```

---

## Decisions taken

1. **Motion is verbatim.** Declaration bodies and their doc comments move
   byte-identical. New content is limited to each new file's header comment and
   its `package`/`import` block. Exactly one comment edit is permitted, named in
   stage 1 — see "The one permitted comment edit" below.
2. **The `Overview` struct and `Host` interface stay whole in `grid.go`.** Field
   comments describing search or thumbnail state stay with their fields. Field
   clustering into sub-structs is a *different* refactor
   (`planned_features/viewer_field_clusters.md`'s shape) and is out of scope.
3. **`grid_test.go` splits into five files**, mirroring the production split
   plus `harness_test.go` for the shared fakes and helpers — the same convention
   `internal/ui/harness_test.go` already sets.
4. **Test placement is mechanical: the test lives where the function named in
   its prefix lives.** `TestHandleKey_BackspaceShortensTheQuery` goes to
   `nav_test.go` because `backspace` lives in `nav.go`, even though the behavior
   is search-flavored. `TestClose_ClearsTheSearch` goes to `grid_test.go`
   because `Close` lives in `grid.go`. This rule removes every judgment call;
   the full mapping is tabulated below so no agent has to apply it.
5. **`harness_test.go` is a new file; `grid_test.go` keeps its name and its
   git history.** The precedent in `planned_features/split_library_test.md`
   renamed the source file with `git mv` so the 450-line, 21-file-deep harness
   kept `git blame`. That reasoning does not transfer: this harness is 128 lines
   used by four files in one package, and the remainder splits roughly 50/50
   between harness and lifecycle tests. `git blame -C` recovers it. Rejected
   alternative: extract nav/search/thumbs tests, `git mv grid_test.go
   harness_test.go`, then extract the seven lifecycle tests into a fresh
   `grid_test.go` — three extra steps to move whose blame breaks, not whether.
6. **Stages run sequentially, one subagent each, reviewed before the next.**
   Stages 1–3 all rewrite `grid.go`; stages 5–7 all rewrite `grid_test.go`.
   Parallelism is possible only in the "each agent writes only its own new file,
   orchestrator does one deletion pass" variant, which trades the per-stage
   verbatim gate for speed. Not taken.
7. **Model per stage: Sonnet for stages 1–9, Opus for stages 10–11.** Stages
   1–8 are mechanical motion with an exact line map. Stage 9 writes a new
   package against a fully specified API. Stages 10–11 rewire live concurrency
   under `-race` and get Opus.
8. **The stretch goal is stages 9–12 and is separable.** If stage 8 lands and
   the shared pool later looks like a bad trade, stages 1–8 stand alone and the
   remaining stages can be dropped without rework.
9. **No commits.** Suggested messages at the end.
10. **Production files get a header comment; test files do not.** Corrected
    during stage 7's review. `selection.go` opens with one, so `nav.go`,
    `search.go` and `thumbs.go` do too. But every `_test.go` file in the module
    — `selection_test.go`, `internal/ui/harness_test.go`, `load_test.go`,
    `batch_test.go`, `filescan_test.go`, `filesort_test.go`,
    `deletion/*_test.go` — opens directly with `package X`. The four header
    comments this plan originally specified for the new test files were
    stripped to match; the `// --- section ---` dividers already segment them,
    and the filenames carry the rest. The only test file in the module with a
    header is `deletion/export_test.go`, which is a test-only *seam*, not a
    test file.

---

## The one permitted comment edit

`grid.go:299`, inside `New`, in the comment above `g.wrap.OnHighlighted`:

```go
// Fired both by keyboard highlight movement (HandleKey forwards the
// arrow keys to wrap.TypedKey, see below) and by mouse hover (GridWrap
```

`HandleKey` moves to `nav.go`, so "see below" stops being true. Change **only**
those two words:

```go
// Fired both by keyboard highlight movement (HandleKey forwards the
// arrow keys to wrap.TypedKey, see nav.go) and by mouse hover (GridWrap
```

Every other cross-reference in the file was checked and survives the move:

| line | text | verdict |
| --- | --- | --- |
| 78 | `see Close for why that matters` | by name, valid package-wide |
| 138 | `see selection.go` | file unchanged |
| 143 | `every index below means "identity"` | conceptual, not positional |
| 173, 390 | `earlier id`, `earlier Toggle` | not location references |
| 201 | `see Close's comment on why` | by name; `New` and `Close` both stay in `grid.go` |
| 393, 414 | `see winpos.Unmaximize`, `see winpos.Maximize` | other package |
| 497 | `the default branch below` | inside `HandleKey`, moves with it |
| 779 | `unlike Cached above` | `Cached` and `CachedThumb` both → `thumbs.go`, order preserved |
| 789, 809 | `see AddIfFits's own comment`, `see evict's comment in ...bytecache.go` | other package |
| 794 | `see ThumbCacheFull below` | both → `thumbs.go`, order preserved |
| 843, 887, 898, 937 | `see the cellIDs field`, three `below`s | inside `requestThumbnail`, move with it |

`selection.go:47`'s `see escape` is a by-name reference to a function moving to
`nav.go` — still valid, no edit.

---

## Target file map (production)

Line ranges are **inclusive, against `grid.go` at `d51c156`**, and each range
starts at the declaration's doc comment, not its `func` line. Ranges are
contiguous and exhaustive: 1–995 with no gaps.

### `grid.go` — stays (≈415 lines)

| range | declaration |
| --- | --- |
| 1–30 | package doc comment + `import` block |
| 41–44 | `const cellSize` |
| 45–90 | `type Host interface` |
| 91–191 | `type Overview struct` |
| 192–333 | `func New` |
| 334–339 | `func (g *Overview) Overlay` |
| 340–345 | `func (g *Overview) Visible` |
| 389–399 | `func (g *Overview) ConsumeMaximized` |
| 400–432 | `func (g *Overview) Toggle` |
| 433–469 | `func (g *Overview) Close` |

### `nav.go` — new (≈180 lines)

| range | declaration |
| --- | --- |
| 346–350 | `func (g *Overview) Highlight` |
| 351–388 | `func (g *Overview) setHighlight` |
| 470–532 | `func (g *Overview) HandleKey` |
| 533–555 | `func (g *Overview) movePage` |
| 556–570 | `func (g *Overview) escape` |
| 571–583 | `func (g *Overview) backspace` |
| 831–839 | `func setCellHighlighted` |

`setCellHighlighted` lands here, not in `grid.go`, for symmetry with
`setCellSelected` in `selection.go`: each ring/tint helper sits beside the state
that moves it.

### `search.go` — new (≈165 lines)

| range | declaration |
| --- | --- |
| 584–594 | `func (g *Overview) clearSearch` |
| 595–599 | `func (g *Overview) Searching` |
| 600–604 | `func (g *Overview) Query` |
| 605–631 | `func (g *Overview) HandleRune` |
| 632–642 | `func (g *Overview) count` |
| 643–657 | `func (g *Overview) fileIndex` |
| 658–693 | `func (g *Overview) applyFilter` |
| 694–734 | `func (g *Overview) syncTopBar` |

### `thumbs.go` — new (≈275 lines)

| range | declaration |
| --- | --- |
| 31–40 | `const thumbConcurrency` |
| 735–762 | `func (g *Overview) Warm` |
| 763–770 | `func (g *Overview) Settle` |
| 771–777 | `func (g *Overview) Cached` |
| 778–786 | `func (g *Overview) CachedThumb` |
| 787–800 | `func (g *Overview) StoreThumb` |
| 801–820 | `func (g *Overview) ThumbCacheFull` |
| 821–830 | `func (g *Overview) SetCacheBytes` |
| 840–954 | `func (g *Overview) requestThumbnail` |
| 955–972 | `func (g *Overview) claim` |
| 973–978 | `func (g *Overview) release` |
| 979–995 | `func (g *Overview) stillWanted` |

**Within each new file, declarations appear in the order listed** — which is
their order in today's `grid.go`. This is what keeps `unlike Cached above` and
`see ThumbCacheFull below` accurate without editing them.

### Expected import sets

Guidance, not gospel: `make fmt` plus `go build ./...` is the authority. If a
listed import turns out unused, drop it; if an unlisted one is needed, add it.

- `grid.go`: `image`, `sync`, `sync/atomic`, `fyne.io/fyne/v2`, `canvas`,
  `container`, `lang`, `theme`, `widget`, `internal/imaging`,
  `internal/selection`, `internal/ui/widgets`, `internal/winpos`.
  **Loses** `fmt` and `strings` (both only used by `syncTopBar`/`applyFilter`).
- `nav.go`: `fyne.io/fyne/v2`, `canvas`, `theme` (`movePage`'s padding math).
- `search.go`: `fmt`, `strings`, `lang`.
- `thumbs.go`: `image`, `fyne.io/fyne/v2`, `canvas`, `internal/imaging`.

### File header comments

Each new file opens with one short header in the voice of the existing code —
this is the only prose any stage 1–3 agent writes:

```go
// Keyboard and pointer navigation of the grid: the highlight ring, its
// relationship to GridWrap's own keyboard cursor, and the key dispatch that
// moves it.
```

```go
// The filename search ('/'): the query, the display-index-to-host-index
// mapping a filter creates, and the top bar that reports both it and the
// selection.
```

```go
// The thumbnail decode pipeline: the byte-budgeted cache, its accessors, and
// the bounded worker pool plus the three staleness guards that decide whether
// a finished decode still belongs on the cell that asked for it.
```

---

## Target file map (tests)

Ranges are inclusive, against `grid_test.go` at `d51c156`.

### `harness_test.go` — new (≈128 lines)

| range | declaration |
| --- | --- |
| 1–92 | file header comment, `TestMain`, `type fakeHost`, its nine interface methods, `hostWith`, `newOverview` |
| 171–181 | `func (f *fakeHost) last` |
| 522–537 | `func openGrid` |
| 551–559 | `func typeQuery` |

`last` is a `fakeHost` method used only by `nav_test.go`, but the fake and all
its methods stay in one place. `openGrid` and `typeQuery` are also used by
`selection_test.go` (22 and 7 times), which is why they cannot stay in a
behavior file.

No header comment: every `_test.go` file in the module opens directly with
`package X` — see decision 10.

### `grid_test.go` — keeps its name, keeps only lifecycle (≈112 lines)

`TestToggle_NoFilesIsNoop` (93–102), `TestToggle_OpensAndCloses` (103–119),
`TestToggle_StartsHighlightOnCurrentImage` (120–134),
`TestClose_NoopWhenAlreadyClosed` (135–148), `TestClose_UnfocusesCanvas`
(149–170), `TestToggle_KeyboardCursorStartsOnTheCurrentImage` (423–441),
`TestClose_ClearsTheSearch` (717–731).

### `nav_test.go` — new (≈410 lines)

`TestHighlightChanged_ReportsTheFileUnderTheRing` (182–214),
`TestHighlightChanged_ReportsTheHostIndexOfAFilteredCell` (215–233),
`TestHighlightChanged_ReportsNoneWhenNothingMatches` (234–251),
`TestHighlightChanged_SilentWhileClosed` (252–268),
`TestHandleKey_EscapeAndGClose` (269–286),
`TestHandleKey_ArrowMovesHighlight` (287–308),
`TestHandleKey_PageMovesHighlightByOneVisiblePage` (309–346),
`TestHandleKey_PageMovesHighlightWhileSearching` (347–368),
`func hover` (369–376), `TestHover_MovesTheRingAndTheKeyboardCursor` (377–404),
`TestHover_OnTheHighlightedCellIsANoop` (405–422),
`TestHandleKey_LeftAtStartIsNoop` (487–500),
`TestHandleKey_ReturnOpensHighlightedAndCloses` (501–521),
`TestHandleKey_ReturnOpensHostIndexOfFilteredCell` (585–599),
`TestHandleKey_EscapeClearsSearchBeforeClosingTheGrid` (623–648),
`TestHandleKey_BackspaceShortensTheQuery` (649–666),
`TestHandleKey_BackspaceDeletesAWholeRune` (667–684),
`TestHandleKey_BackspaceOnEmptyQueryStaysInSearch` (685–698),
`TestHandleKey_GDoesNotCloseWhileSearching` (699–716),
`TestHandleKey_ReturnWithNoMatchesOpensNothing` (742–755),
`TestSetCellHighlighted` (1142–1154).

`hover` moves here rather than to the harness: all five of its uses are in this
file.

### `search_test.go` — new (≈250 lines)

`TestHandleRune_FilteringResetsTheKeyboardCursorToo` (442–461),
`TestHandleRune_NoMatchesLeavesTheKeyboardCursorAlone` (462–486),
`TestHandleRune_SlashOpensSearch` (538–550),
`TestHandleRune_QueryFiltersToMatchingNames` (560–574),
`TestHandleRune_MatchingIsCaseInsensitive` (575–584),
`TestHandleRune_FilteringResetsHighlightToFirstMatch` (600–622),
`TestHandleRune_NoMatchesShowsAnEmptyGrid` (732–741),
`TestSearchBar_HiddenUntilSearchOpens` (756–773),
`TestSearchBar_ShowsQueryAndMatchCount` (774–786),
`TestSearchBar_EmptyNoticeOnlyWhenNothingMatches` (787–810).

`TestSearchBar_*` has no method prefix; it maps to `syncTopBar`, which is in
`search.go`.

### `thumbs_test.go` — new (≈331 lines)

The contiguous block **811–1141**: `func newCell`,
`TestRequestThumbnail_CacheHitAppliesSynchronously`,
`TestRequestThumbnail_DecodesInBackgroundAndCaches`,
`TestSetCacheBytes_RetunesTheThumbnailBudget`,
`TestCachedThumb_MissesForUnstoredURI`,
`TestStoreThumb_ThenCachedThumb_ReturnsWhatWasStored`,
`TestStoreThumb_TooBigForBudgetIsRefused`,
`TestThumbCacheFull_FalseUntilBudgetReached`,
`TestStoreThumb_AloneDoesNotProtectTheHeadOfTheList`,
`TestRequestThumbnail_OutOfRangeIDIsNoop`, `TestClaimRelease`,
`TestRequestThumbnail_RecycledBeforeDecodeBailsAndReleases`,
`TestRequestThumbnail_QueryChangeDiscardsInFlightDecode`, `TestStillWanted`.

`newCell` moves here rather than to the harness: all nine of its uses are in
this file.

**Test count check:** 7 + 20 + 10 + 13 = 50 tests, plus `TestMain` in the
harness = 51 `func Test*` declarations, matching the baseline. Counting the two
non-test helpers that move with their tests (`hover`, `newCell`), `nav_test.go`
gains 21 declarations and `thumbs_test.go` 14.

---

## The review gate (run by the orchestrator after every motion stage)

Stages 1–7 are verbatim moves, which makes them mechanically checkable. After
each, before dispatching the next:

```bash
# 1. Nothing but the header/import block is new in the target file, and every
#    line deleted from the source file reappears in it byte-identically.
git diff -U0 HEAD -- internal/ui/grid/grid.go \
  | grep '^-' | grep -v '^---' | sed 's/^-//' | sort > /tmp/removed.txt
sort internal/ui/grid/nav.go > /tmp/added.txt      # target file for this stage
comm -23 /tmp/removed.txt /tmp/added.txt           # MUST be empty except import lines

# 2. Additions to the source file are only import-block changes.
git diff -U0 HEAD -- internal/ui/grid/grid.go | grep '^+' | grep -v '^+++'

# 3. The exported API is byte-identical to the baseline.
go doc -all ./internal/ui/grid | diff - /private/tmp/claude-502/-Users-ronin-Projects-picfetch/dad3c0dc-0dcf-4675-9fc9-c1299e023aee/scratchpad/grid.doc.baseline

# 4. Every test still exists and still passes, by name.
go test -count=1 -v ./internal/ui/grid/ 2>/dev/null \
  | grep -E '^(--- |    --- )' | sed 's/ (.*//' | sort \
  | diff - /private/tmp/claude-502/-Users-ronin-Projects-picfetch/dad3c0dc-0dcf-4675-9fc9-c1299e023aee/scratchpad/grid.tests.baseline
```

Checks 3 and 4 hold for stages 1–8 without exception — that is the whole point
of "verbatim". Check 1's `comm` output being non-empty is a **stop**: it means
a line changed in transit.

For stages 5–7 substitute `grid_test.go` as the source file. Check 3 does not
apply to test-only stages (test files contribute nothing to `go doc`), but run
it anyway — it must still match.

---

## Stage 1 — extract `nav.go`

**Model:** Sonnet.
**Files:** create `internal/ui/grid/nav.go`; modify `internal/ui/grid/grid.go`.

- [ ] **Step 1.** Create `internal/ui/grid/nav.go` with the header comment given
      above under "File header comments", then `package grid`, then an empty
      import block to be filled in step 3.
- [ ] **Step 2.** Move these ranges out of `grid.go` and into `nav.go`, in this
      order, byte-identical including doc comments: 346–350 (`Highlight`),
      351–388 (`setHighlight`), 470–532 (`HandleKey`), 533–555 (`movePage`),
      556–570 (`escape`), 571–583 (`backspace`), 831–839 (`setCellHighlighted`).
      Delete each from `grid.go`. Do not reflow, re-wrap, or reword anything.
- [ ] **Step 3.** Fix both files' imports so the package compiles. Expected:
      `nav.go` needs `fyne.io/fyne/v2`, `fyne.io/fyne/v2/canvas`,
      `fyne.io/fyne/v2/theme`; `grid.go` keeps everything it still uses.
- [ ] **Step 4.** Make the one permitted comment edit in `grid.go` — change
      `see below` to `see nav.go` in the `OnHighlighted` comment (was line 299).
      This is the only comment text any stage-1 step may touch.
- [ ] **Step 5.** Run `make fmt`, then:

```bash
go build ./... && go test -count=1 ./internal/ui/grid/
```

Expected: builds clean, all tests pass in well under 5 s.

- [ ] **Step 6.** Report the new line counts of `grid.go` and `nav.go`, and
      state explicitly that no declaration body or doc comment was altered
      other than the edit in step 4. Do not commit.

---

## Stage 2 — extract `search.go`

**Model:** Sonnet.
**Files:** create `internal/ui/grid/search.go`; modify `internal/ui/grid/grid.go`.

Line numbers below are **the original `d51c156` numbers**; `grid.go` has shifted
under stage 1, so locate each declaration by name, not by offset.

- [ ] **Step 1.** Create `internal/ui/grid/search.go` with the search header
      comment given above, then `package grid`.
- [ ] **Step 2.** Move these declarations out of `grid.go` into `search.go`, in
      this order, byte-identical: `clearSearch`, `Searching`, `Query`,
      `HandleRune`, `count`, `fileIndex`, `applyFilter`, `syncTopBar`
      (original ranges 584–734).
- [ ] **Step 3.** Fix imports. Expected: `search.go` needs `fmt`, `strings`,
      `fyne.io/fyne/v2/lang`; `grid.go` **loses** `fmt` and `strings` and keeps
      `lang` (used by the `No file names match` label in `New`).
- [ ] **Step 4.** Run `make fmt`, then `go build ./... && go test -count=1 ./internal/ui/grid/`.
- [ ] **Step 5.** Report line counts and confirm verbatim motion. No comment
      edits are permitted in this stage. Do not commit.

---

## Stage 3 — extract `thumbs.go`

**Model:** Sonnet.
**Files:** create `internal/ui/grid/thumbs.go`; modify `internal/ui/grid/grid.go`.

- [ ] **Step 1.** Create `internal/ui/grid/thumbs.go` with the thumbnail header
      comment given above, then `package grid`.
- [ ] **Step 2.** Move these out of `grid.go` into `thumbs.go`, in this order,
      byte-identical: `const thumbConcurrency` (with its 8-line comment),
      `Warm`, `Settle`, `Cached`, `CachedThumb`, `StoreThumb`, `ThumbCacheFull`,
      `SetCacheBytes`, `requestThumbnail`, `claim`, `release`, `stillWanted`.
- [ ] **Step 3.** Fix imports. Expected: `thumbs.go` needs `image`,
      `fyne.io/fyne/v2`, `fyne.io/fyne/v2/canvas`,
      `github.com/frathe/picfetch/internal/imaging`. `grid.go` keeps `image`
      (the `thumbs *imaging.ByteCache[image.Image]` field), `imaging` (the
      `NewThumbCache` call in `New`), `sync` and `sync/atomic` (struct fields).
- [ ] **Step 4.** Run `make fmt`, then `go build ./... && go test -count=1 ./internal/ui/grid/`.
- [ ] **Step 5.** `grid.go` should now be ≈415 lines. Report the four files'
      line counts. Do not commit.

---

## Stage 4 — extract `harness_test.go`

**Model:** Sonnet.
**Files:** create `internal/ui/grid/harness_test.go`; modify `internal/ui/grid/grid_test.go`.

- [ ] **Step 1.** Create `internal/ui/grid/harness_test.go` opening directly
      with `package grid` — no header comment, per decision 10.
- [ ] **Step 2.** Move out of `grid_test.go`, byte-identical, in this order:
      `TestMain`, `type fakeHost` and all nine of its interface methods,
      `hostWith`, `newOverview` (original 1–92, minus that file's own header
      comment and import block, which stay with `grid_test.go`); then
      `func (f *fakeHost) last` (171–181); then `func openGrid` (522–537); then
      `func typeQuery` (551–559).
- [ ] **Step 3.** Fix both files' imports. `harness_test.go` needs at least
      `os`, `testing`, `fyne.io/fyne/v2`, `fyne.io/fyne/v2/test`,
      `github.com/frathe/picfetch/internal/uitest`; `grid_test.go` keeps only
      what its remaining tests use.
- [ ] **Step 4.** Run `make fmt`, then:

```bash
go build ./... && go test -count=1 ./internal/ui/grid/
```

Both `grid_test.go` and `selection_test.go` must still compile — `openGrid` and
`typeQuery` are used by the latter 22 and 7 times.

- [ ] **Step 5.** Report line counts. Do not commit.

---

## Stage 5 — extract `nav_test.go`

**Model:** Sonnet.
**Files:** create `internal/ui/grid/nav_test.go`; modify `internal/ui/grid/grid_test.go`.

- [ ] **Step 1.** Create `internal/ui/grid/nav_test.go` opening directly with
      `package grid` — no header comment, per decision 10. The
      `// --- highlight notification ---` and `// --- key handling ---` dividers
      travel with their tests and do the segmenting.
- [ ] **Step 2.** Move the 20 tests and the `hover` helper listed under
      "`nav_test.go`" in the target map above, byte-identical, **in the order
      listed** (which is their current order in `grid_test.go`).
- [ ] **Step 3.** Fix imports in both files.
- [ ] **Step 4.** Run `make fmt`, then `go build ./... && go test -count=1 ./internal/ui/grid/`.
- [ ] **Step 5.** Confirm the moved-test count is 21 declarations (20 `Test*` +
      `hover`). Do not commit.

---

## Stage 6 — extract `search_test.go`

**Model:** Sonnet.
**Files:** create `internal/ui/grid/search_test.go`; modify `internal/ui/grid/grid_test.go`.

- [ ] **Step 1.** Create `internal/ui/grid/search_test.go` opening directly with
      `package grid` — no header comment, per decision 10. Hoist the
      `// --- search ---` divider to sit directly under the import block, the
      way `grid_test.go` and `thumbs_test.go` carry theirs.
- [ ] **Step 2.** Move the 10 tests listed under "`search_test.go`" above,
      byte-identical, in the order listed.
- [ ] **Step 3.** Fix imports in both files.
- [ ] **Step 4.** Run `make fmt`, then `go build ./... && go test -count=1 ./internal/ui/grid/`.
- [ ] **Step 5.** Report counts. Do not commit.

---

## Stage 7 — extract `thumbs_test.go`

**Model:** Sonnet.
**Files:** create `internal/ui/grid/thumbs_test.go`; modify `internal/ui/grid/grid_test.go`.

- [ ] **Step 1.** Create `internal/ui/grid/thumbs_test.go` opening directly with
      `package grid` — no header comment, per decision 10. The
      `// --- thumbnails ---` and `// --- CachedThumb / StoreThumb /
      ThumbCacheFull ---` dividers travel with the block.
- [ ] **Step 2.** Move the contiguous block — original `grid_test.go` lines
      811–1141, i.e. `newCell` plus 13 tests — byte-identical.
- [ ] **Step 3.** Fix imports in both files. `grid_test.go` is now ≈112 lines
      holding seven lifecycle tests and nothing else.
- [ ] **Step 4.** Run `make fmt`, then:

```bash
go build ./... && go test -count=1 ./internal/ui/grid/ && go test -race -count=1 ./internal/ui/grid/
```

The race run belongs here specifically: `thumbs_test.go` holds every test that
drives a real decode goroutine.

- [ ] **Step 5.** Report the final line counts for all nine files in the
      package. Do not commit.

---

## Stage 8 — documentation and full verification for the split

**Model:** Sonnet.
**Files:** modify `ARCHITECTURE.md`, `todos.md`.

- [ ] **Step 1.** In `ARCHITECTURE.md`, find the `grid/` row of the
      `internal/ui` subpackage table (around line 89) and the "where to look for
      X" entries that name grid internals (around lines 554, 558, 622). Add the
      new file names to the existing prose so each concern is findable by file:
      the search entry should name `internal/ui/grid/search.go`, the thumbnail
      entry `internal/ui/grid/thumbs.go`, the navigation/highlight prose
      `internal/ui/grid/nav.go`, alongside the `internal/ui/grid/selection.go`
      reference already there. **Do not rewrite or re-summarize the existing
      text** — this is an insertion of file names into sentences that already
      describe the behavior correctly.
- [ ] **Step 2.** In `todos.md`, move item 4 from `## TODO` to `## Done`,
      rewriting it in the past tense in the same voice as item 3 above it: what
      each new file holds, and that the split is within the package with no API
      change. If stages 9–12 are also being run, note that the stretch goal is
      in progress under `## ACTIVE DEVELOPMENT` rather than marking it done.
- [ ] **Step 3.** Run the full CI-equivalent sequence from the repository root:

```bash
make fmt && go vet ./... && go build ./... && go test -race ./...
```

Expected: clean. Report the total wall time of the race run.

- [ ] **Step 4.** Report a summary: nine files in `internal/ui/grid`, none over
      ~420 lines, exported API unchanged, 74 tests in the package. Do not commit.

**This is the natural stopping point for the split.** Stages 9–12 are the
stretch goal and are independently abandonable.

---

## Stage 9 — new package `internal/decodepool`

**Model:** Sonnet (the API below is fully specified; the judgment was made here).
**Files:** create `internal/decodepool/decodepool.go`, `internal/decodepool/decodepool_test.go`.

### Why this type exists

Two places grew the same trio independently:

| | `internal/ui/grid` | `internal/ui` viewer |
| --- | --- | --- |
| semaphore | `sem`, cap `thumbConcurrency` = 4 | `preloadSem`, cap `preloadConcurrency` = 2 |
| in-flight claim | `inflight sync.Map`, `*fyne.Container` → `int` | `preloading sync.Map`, `string` → `struct{}` |
| completion count | `pending sync.WaitGroup` | `preloadPending sync.WaitGroup` |
| claim rule | present **with the same id** → skip; else store and overwrite | present → skip (`LoadOrStore`) |
| release rule | `CompareAndDelete(key, id)` | `Delete(key)` |
| queued cancellation | none (plain blocking send) | `select` on `token.context().Done()` |

The two claim rules unify exactly: "present with an equal value → refuse;
otherwise store and accept" **is** `LoadOrStore` when the value type is
`struct{}`, because every `struct{}` is equal to every other. Likewise
`CompareAndDelete(key, struct{}{})` is `Delete(key)`. So one generic type covers
both without weakening either.

### The API

```go
// Package decodepool bounds background decode work. It is the semaphore +
// per-key in-flight claim + completion counter that internal/ui/grid's
// thumbnails and internal/ui's speculative preload each grew independently:
// a small worker pool, a gate against spawning a second goroutine for work
// already underway, and a WaitGroup a test can wait out so no goroutine
// outlives the test that started it.
//
// It is deliberately viewer-independent: no Fyne types, no UI marshaling. The
// caller decides what a key is, what staleness means, and when to release.
package decodepool

// Pool bounds concurrent work over keys of type K, each claim carrying a
// value of type V that says what the in-flight work is for. The zero Pool is
// not usable; call New.
type Pool[K, V comparable] struct { /* sem, inflight, pending */ }

// New returns a pool that runs at most limit functions at once. limit must be
// positive.
func New[K, V comparable](limit int) *Pool[K, V]

// Claim records that work is being spawned for key toward v, reporting
// whether the caller should actually spawn it. False means identical work is
// already in flight. A claim for the same key with a *different* v succeeds
// and overwrites: the caller has moved on to different work for that key, and
// the superseded worker's Release will not clobber the new claim (see
// Release).
func (p *Pool[K, V]) Claim(key K, v V) bool

// Release drops key's claim, but only if it still names v - so a superseded
// worker finishing late cannot drop a newer claim made over it.
func (p *Pool[K, V]) Release(key K, v V)

// Go spawns fn on its own goroutine, which first waits for a free slot in the
// pool. acquired is false when ctx was cancelled while fn was still queued;
// fn is called either way, so a caller can always undo its Claim, but must
// not start work when acquired is false. The slot is held for the whole of
// fn and released when it returns.
//
// Wait returns only after every fn spawned so far has returned.
func (p *Pool[K, V]) Go(ctx context.Context, fn func(acquired bool))

// Wait blocks until every function spawned so far has returned. The
// application never needs this; tests do.
func (p *Pool[K, V]) Wait()
```

`Go` calls `fn` even when the context cancelled it out of the queue. That is
what preserves today's viewer behavior: `preloadOne`'s `defer
v.preloading.Delete(key)` runs whether or not the semaphore was ever acquired,
so a preload cancelled while queued still clears its in-flight mark. An API that
silently dropped `fn` would leak that mark forever.

- [ ] **Step 1: write the failing tests.** Create
      `internal/decodepool/decodepool_test.go`. No Fyne, no `TestMain` — this
      package touches no repository or driver.

```go
package decodepool

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestClaim_SecondClaimForSameValueIsRefused(t *testing.T) {
	p := New[string, int](1)
	if !p.Claim("a", 1) {
		t.Fatal("first claim refused")
	}
	if p.Claim("a", 1) {
		t.Fatal("duplicate claim accepted")
	}
}

func TestClaim_DifferentValueOverwrites(t *testing.T) {
	p := New[string, int](1)
	p.Claim("a", 1)
	if !p.Claim("a", 2) {
		t.Fatal("claim for a different value refused")
	}
	// The superseded worker's release must not drop the newer claim.
	p.Release("a", 1)
	if p.Claim("a", 2) {
		t.Fatal("stale release dropped the newer claim")
	}
}

func TestClaim_StructValueBehavesLikeLoadOrStore(t *testing.T) {
	p := New[string, struct{}](1)
	if !p.Claim("a", struct{}{}) {
		t.Fatal("first claim refused")
	}
	if p.Claim("a", struct{}{}) {
		t.Fatal("duplicate claim accepted")
	}
	p.Release("a", struct{}{})
	if !p.Claim("a", struct{}{}) {
		t.Fatal("claim after release refused")
	}
}

func TestGo_LimitBoundsConcurrency(t *testing.T) {
	p := New[int, int](2)
	var inFlight, peak atomic.Int64
	release := make(chan struct{})
	for i := 0; i < 8; i++ {
		p.Go(context.Background(), func(acquired bool) {
			if !acquired {
				t.Error("acquired false with an uncancelled context")
				return
			}
			n := inFlight.Add(1)
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			<-release
			inFlight.Add(-1)
		})
	}
	close(release)
	p.Wait()
	if peak.Load() > 2 {
		t.Fatalf("peak concurrency %d, want <= 2", peak.Load())
	}
}

func TestGo_CancelledWhileQueuedStillCallsFn(t *testing.T) {
	p := New[int, int](1)
	block := make(chan struct{})
	p.Go(context.Background(), func(bool) { <-block })

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var got atomic.Bool
	var sawAcquired atomic.Bool
	p.Go(ctx, func(acquired bool) {
		got.Store(true)
		sawAcquired.Store(acquired)
	})

	close(block)
	p.Wait()
	if !got.Load() {
		t.Fatal("fn was not called for a queue-cancelled request")
	}
	if sawAcquired.Load() {
		t.Fatal("acquired was true for a queue-cancelled request")
	}
}

func TestWait_ReturnsOnlyAfterEveryFnReturns(t *testing.T) {
	p := New[int, int](4)
	var done atomic.Int64
	block := make(chan struct{})
	for i := 0; i < 4; i++ {
		p.Go(context.Background(), func(bool) {
			<-block
			done.Add(1)
		})
	}
	close(block)
	p.Wait()
	if done.Load() != 4 {
		t.Fatalf("Wait returned with %d of 4 done", done.Load())
	}
}
```

- [ ] **Step 2: run them, confirm they fail** with "undefined: New".

```bash
go test ./internal/decodepool/
```

- [ ] **Step 3: implement `internal/decodepool/decodepool.go`** to the API above.
      `Claim` is `Load` + compare + `Store`; `Release` is `CompareAndDelete`;
      `Go` is `pending.Go(func(){ select on sem vs ctx.Done(); defer drain;
      fn(acquired) })`; `Wait` is `pending.Wait()`. Keep the doc comments from
      the API block verbatim — they carry the reasoning.
- [ ] **Step 4: run the tests, plus race.**

```bash
go test -count=1 ./internal/decodepool/ && go test -race -count=1 ./internal/decodepool/
```

- [ ] **Step 5.** Report. Do not commit. Nothing outside `internal/decodepool`
      may be touched in this stage.

---

## Stage 10 — move the grid's thumbnail pool onto `decodepool`

**Model:** Opus. Live concurrency, `-race` coverage, three staleness guards.
**Files:** modify `internal/ui/grid/grid.go`, `internal/ui/grid/thumbs.go`,
`internal/ui/grid/thumbs_test.go`.

- [ ] **Step 1.** In `grid.go`, replace the `sem`/`pending`/`inflight` fields
      with one field:

```go
// decodes bounds concurrent thumbnail decodes and gates duplicate work per
// recycled cell - see internal/decodepool. The key is the cell container
// (the stable per-slot widget, not the image inside it) and the value is the
// file id that cell's in-flight decode is working toward, so a cell recycled
// onto a different file supersedes rather than blocks.
decodes *decodepool.Pool[*fyne.Container, int]
```

      `cellIDs sync.Map` **stays** — it is what `stillWanted` reads, and it is a
      different question from "is a decode in flight". Keep its field comment.
      In `New`, replace the `sem: make(chan struct{}, thumbConcurrency)`
      initializer with `decodes: decodepool.New[*fyne.Container, int](thumbConcurrency)`.
- [ ] **Step 2.** In `thumbs.go`, delete `claim` and `release` and call
      `g.decodes.Claim(key, id)` / `g.decodes.Release(key, id)` at each of their
      four existing call sites. **`stillWanted` stays exactly as it is** — it
      reads `cellIDs`, `Generation()` and `filterGen`, none of which the pool
      knows about.
- [ ] **Step 3.** Rewrite `requestThumbnail`'s spawn as:

```go
g.decodes.Go(context.Background(), func(bool) {
    // ... today's body, minus the `g.sem <- struct{}{}` /
    // `defer func() { <-g.sem }()` pair, which the pool now owns.
})
```

      The grid has no cancellation context today, so `context.Background()` is
      the faithful translation and `acquired` is always true. Preserve the
      **exact order** of today's body: the early `stillWanted` bail with its
      `release` and re-request `fyne.Do`; the second cache look; the
      `LoadThumbnail` error path's `release`; the unconditional `thumbs.Add`;
      the `release` **before** the completion `fyne.Do`, not after. That last
      ordering is deliberate and load-bearing — do not "tidy" it into a
      `defer`.
- [ ] **Step 4.** `Settle` becomes `g.decodes.Wait()`. Keep its doc comment,
      updating only the sentence that names `pending`.
- [ ] **Step 5.** In `thumbs_test.go`, `TestClaimRelease` now drives
      `g.decodes.Claim`/`Release` instead of the deleted methods. Keep the test —
      it pins the supersession rule *as the grid uses it*, which the
      `decodepool` unit test cannot see. Rename it to
      `TestDecodes_ClaimRelease` only if the compiler forces it; otherwise leave
      the name so the baseline test inventory stays comparable.
- [ ] **Step 6.** Verify:

```bash
make fmt && go build ./... && go test -race -count=1 ./internal/ui/grid/
```

      Then the whole suite: `go test -race ./...`. Every one of
      `TestRequestThumbnail_*`, `TestStillWanted`, and the grid tests in
      `internal/ui` must pass unchanged.
- [ ] **Step 7.** Report which behaviors you verified are unchanged, naming the
      tests. Do not commit.

---

## Stage 11 — move the viewer's preload pool onto `decodepool`

**Model:** Opus. Same reasons, plus this path is covered by timing-sensitive
tests in `internal/ui/harness_test.go`.
**Files:** modify `internal/ui/viewer.go`, `internal/ui/build.go`,
`internal/ui/load.go`, `internal/ui/harness_test.go`.

- [ ] **Step 1.** In `viewer.go`, replace the `preloading`/`preloadSem`/
      `preloadPending` trio (lines ~281–293) with:

```go
// preloads bounds how many speculative neighbor decodes run at once and
// stops rapid navigation piling up a second decode of the same
// not-yet-cached neighbor - see internal/decodepool. Keyed by URI string;
// the claim carries no value, since the URI alone says what the work is.
// waitUntilLoaded (harness_test.go) waits it out after every load so a
// preload goroutine never outlives the test whose navigation spawned it.
preloads *decodepool.Pool[string, struct{}]
```

      Note the existing comment says "mirroring thumbPending below" — there is
      no `thumbPending` field anywhere in the module; it is a stale reference to
      `internal/ui/grid`'s `pending`. The replacement comment above drops it.
      That is a doc fix, and it is in scope for this stage precisely because the
      field it misnames is the one being deleted.
- [ ] **Step 2.** In `build.go:73`, replace
      `preloadSem: make(chan struct{}, preloadConcurrency)` with
      `preloads: decodepool.New[string, struct{}](preloadConcurrency)`.
      Leave `const preloadConcurrency = 2` where it is in `load.go`, updating
      only its comment's reference to `preloadSem`.
- [ ] **Step 3.** In `load.go`'s `preloadOne`, replace

```go
if _, inFlight := v.preloading.LoadOrStore(key, struct{}{}); inFlight {
    return
}
v.preloadPending.Go(func() {
    defer v.preloading.Delete(key)
    select {
    case v.preloadSem <- struct{}{}:
    case <-token.context().Done():
        return
    }
    defer func() { <-v.preloadSem }()
    if !token.current() { return }
    // ... decode
})
```

      with

```go
if !v.preloads.Claim(key, struct{}{}) {
    return
}
v.preloads.Go(token.context(), func(acquired bool) {
    defer v.preloads.Release(key, struct{}{})
    if !acquired || !token.current() {
        return
    }
    // ... decode, unchanged
})
```

      The `Claim` moves **outside** the goroutine, matching today's
      `LoadOrStore` placement — the caller must learn "already in flight"
      synchronously, before spawning. Everything after `token.current()` —
      `ReadAndProbe`, the `Budget()` read, the `EstimateDecodedBytes > budget/2`
      bail, `DecodeLoaded` — is untouched.
- [ ] **Step 4.** In `harness_test.go`, replace both `v.preloadPending.Wait()`
      calls (lines ~171 and ~249) with `v.preloads.Wait()`. Keep the surrounding
      comments, including the one at ~242 explaining the registration ordering
      against `loadDone` — that reasoning is unchanged.
- [ ] **Step 5.** Verify. This stage's risk lives in `internal/ui`'s suite:

```bash
make fmt && go vet ./... && go build ./... && go test -race ./internal/ui/
```

      Then the whole suite: `go test -race ./...`. Run `internal/ui` **twice**;
      a preload-ordering regression is the kind that shows up intermittently.
- [ ] **Step 6.** Report, naming the preload tests in `imgcache_test.go` and
      `load_test.go` you confirmed still pass. Do not commit.

---

## Stage 12 — documentation and final verification for the shared pool

**Model:** Sonnet.
**Files:** modify `ARCHITECTURE.md`, `todos.md`, `needs_refactoring.md`.

- [ ] **Step 1.** Add a row for `internal/decodepool` to `ARCHITECTURE.md`'s
      top-level package table, beside its viewer-independent siblings
      (`internal/filescan`, `internal/filesort`, `internal/imaging`). Say what it
      is (semaphore + per-key in-flight claim + completion WaitGroup), who its
      two consumers are, and why `stillWanted` and `cellIDs` deliberately stayed
      in `internal/ui/grid` rather than moving into it.
- [ ] **Step 2.** Update the `grid/` row and the `internal/ui` `load.go` row to
      name `decodepool` where they currently describe the hand-rolled pools.
      Search `ARCHITECTURE.md` for `preloadSem`, `preloadPending`, `preloading`,
      and `thumbPending` and fix every hit — the last of those names a field
      that never existed.
- [ ] **Step 3.** In `todos.md`, move the stretch goal out of
      `## ACTIVE DEVELOPMENT` into `## Done` under item 4, in past tense.
- [ ] **Step 4.** In `needs_refactoring.md`, item 5 (the nine `chan struct{}`
      test-sync fields) is now the sole remaining top item. Add one sentence
      noting that `decodepool` is the precedent for it: one audited type
      replacing N hand-rolled copies of the same contract.
- [ ] **Step 5.** Full CI-equivalent run from the repository root:

```bash
make fmt && go vet ./... && go build ./... && go test -race ./...
```

- [ ] **Step 6.** Report. Do not commit.

---

## What could go wrong

**A moved declaration silently changes.** The stage gate's `comm -23` check
catches it: every line deleted from the source must reappear in the target.
Non-empty output that isn't an import line is a stop.

**An import ends up in the wrong file and the package still compiles.** Harmless
— `make fmt` and `go vet` keep it honest, and an unused import is a compile
error in Go, so this cannot silently rot.

**Stage 10 reorders `release` relative to the completion `fyne.Do`.** This is the
single most likely real regression in the whole plan. Today's code releases the
claim *before* painting, deliberately, so a cell's next update pass can re-claim
during the paint. `TestRequestThumbnail_RecycledBeforeDecodeBailsAndReleases`
and `TestRequestThumbnail_QueryChangeDiscardsInFlightDecode` are the tests that
would catch it; both run under `-race` in stage 10's step 6.

**Stage 11 changes when the in-flight mark clears.** Today it clears via `defer`
at goroutine exit; the plan keeps that (`defer v.preloads.Release`). The trap is
"tidying" it to release before the decode, which would let rapid navigation
stack duplicate full-size decodes. `imgcache_test.go`'s oversized-neighbor tests
are the guard.

**`selection_test.go` breaks in stage 4.** It uses `openGrid` 22 times and
`typeQuery` 7 times. Both move to `harness_test.go`, same package, so it should
be invisible — but it is the one file outside the split that the split can
break, which is why stage 4's step 4 compiles the whole package.

---

## Suggested commit messages

After stage 8 (the split alone):

```
Split internal/ui/grid/grid.go into four files

grid.go kept Host, Overview, New and the Toggle/Close/Overlay lifecycle;
nav.go took the highlight ring and key dispatch; search.go took the '/'
filter and the display-to-host index mapping; thumbs.go took the thumbnail
cache and its bounded decode pool. grid_test.go split the same four ways
plus harness_test.go for the shared fake Host and helpers, which
selection_test.go also uses.

Pure motion: every declaration moved byte-identical, no rename, no
visibility change, no API change. go doc -all over the package is identical
before and after, and all 74 tests pass by the same names.
```

After stage 12 (the shared pool):

```
Extract the bounded decode pool into internal/decodepool

internal/ui/grid's thumbnails and internal/ui's speculative preload had each
grown the same trio: a semaphore, a per-key sync.Map claim against spawning
duplicate work, and a WaitGroup tests wait out. The two claim rules unify
exactly - the grid's "present with an equal value refuses, a different value
supersedes" is LoadOrStore when the value is struct{} - so one generic
Pool[K, V] covers both without weakening either.

The staleness guards stay where they were: stillWanted and cellIDs are the
grid's business, and the pool deliberately knows nothing about generations,
filters or cell recycling.
```

---

## Result

| file | before | after |
| --- | --- | --- |
| `internal/ui/grid/grid.go` | 995 | ~415 |
| `internal/ui/grid/nav.go` | — | ~180 |
| `internal/ui/grid/search.go` | — | ~165 |
| `internal/ui/grid/thumbs.go` | — | ~275 |
| `internal/ui/grid/grid_test.go` | 1,154 | ~112 |
| `internal/ui/grid/harness_test.go` | — | ~128 |
| `internal/ui/grid/nav_test.go` | — | ~410 |
| `internal/ui/grid/search_test.go` | — | ~250 |
| `internal/ui/grid/thumbs_test.go` | — | ~331 |
| `internal/ui/grid/selection.go` | 204 | 204 (untouched) |
| `internal/ui/grid/selection_test.go` | 383 | 383 (untouched) |
| `internal/decodepool/` | — | ~120 + ~130 test (stage 9) |
