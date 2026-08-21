# Group the `viewer` struct's field clusters into sub-structs

Implementation plan for the `todos.md` entry of the same name.

## What this is, and what it deliberately is not

`internal/ui/viewer.go`'s `viewer` carries ~70 fields, and the package hangs
111 methods off it. ARCHITECTURE.md is explicit that a general controller
extraction is **not** wanted, and this is not that. Nothing moves between
packages, no `Host` interface changes, no feature gains or loses an owner,
and no behavior changes at all. Three field clusters that are already
de-facto modules — each with its own file, its own single-writer contract,
and its own lifecycle — stop being flattened into one namespace and become
named sub-structs of `viewer`.

The clusters, and how big each one actually is:

| Cluster | Fields | Prod refs | Test refs | Hot files |
|---|---|---|---|---|
| Vector view | 8 | 32 | 49 | `vector.go` (18), `vector_test.go` (46) |
| Scan UI | 6 | 25 | 26 | `drop.go` (19), `drop_test.go` (16) |
| Sort UI | 5 | 22 | 28 | `sort.go` (19), `sort_test.go` (17) |
| Settings-backed state | 7 | 22 | 5 | `load.go`, `run.go`, `memlimits.go`, `drop.go` |

## Decisions taken before planning

Answered by the repo owner, and binding on every stage below:

1. **`asyncOpUI` depth: group + mechanics methods.** The type owns the
   lifecycle, the active flag, the done channel, and the three widgets, and
   gets the methods for *that* bookkeeping only. Viewer-level policy —
   `showWelcomeState`, `ForceRepaint`, the per-op toast text — stays at its
   call sites in `drop.go`/`sort.go`. The type stays viewer-independent.
2. **Settings struct: named `settings`, all seven fields**, including
   `favPreviewCache`, because the set is exactly the `settingswin.Host`
   getter/setter surface and nothing else.
3. **Types live beside their consumers**: `vectorView` in `vector.go`,
   `asyncOpUI` in a new `asyncop.go` (it has two consumers, so it belongs to
   neither), `settings` in `memlimits.go`, whose file doc comment widens from
   "the three memory limits" to "the settings-backed state".
4. **ARCHITECTURE.md: full rewrite of the affected rows**, not a
   find-and-replace of stale field names.
5. **Name clash resolved by renaming the window.** `viewer.settings` is
   already `*settingswin.Window`. It becomes `settingsWin` (10 mechanical
   sites) so `settings` is free for the new state struct.

## Non-negotiable constraints for every stage

These are the things that break quietly if a stage gets them wrong.

- **Behavior-preserving. Zero semantic change.** Every observable behavior,
  every ordering, every staleness check stays exactly as it is. If a stage
  finds something that looks like a bug, it writes it down and leaves it
  alone. (One such thing is already known — see *Flagged, not fixed* below.)
- **`vectorView` and `asyncOpUI` contain synchronization primitives**
  (`requestLifecycle` holds a `sync.Mutex`; `vectorView` holds a
  `sync.WaitGroup`). They must be **value fields on `viewer`, initialized in
  place**. No constructor may return one by value, they may never be copied,
  assigned wholesale, ranged over, or passed by value — `go vet`'s
  `copylocks` will reject it, and rightly. `winPos winpos.Tracker` is the
  precedent already in the struct, comment included: *"A value field, never
  copied."*
- **The doc comments are the documentation.** This package's field comments
  are unusually dense and carry the real reasoning. A field's comment travels
  with the field into its new struct, edited only where a name it mentions
  changed. Nothing gets summarized away, and no comment is dropped because
  the struct it now sits in "makes it obvious".
- **`AGENTS.md` rules apply**: no `TODO`/`FIXME` comments in source; do not
  run `git commit`; mark intentionally ignored errors explicitly. No new
  user-visible strings are introduced by any stage, so no `translations/*.json`
  changes are needed — and none should appear.
- **Verification per stage** (from the repository root):
  `gofmt -l .` (empty) → `go vet ./...` → `go build ./...` →
  `go test -race ./...`. Not just `./internal/ui/...` — the full suite,
  because `main_test.go` enforces locale parity and the settings window's
  `Host` surface is asserted from `internal/ui/settingswin`. **See the
  Baseline section: the suite is red at HEAD**, and how each stage is gated
  depends on which resolution the owner picks there.
- **Golden screenshots**: this refactor changes no pixels. Whatever the
  golden tests do on the runner (they are Docker linux/amd64 via
  `make golden`), they must pass unchanged. Nothing regenerates them.
- **No test *behavior* changes.** Test files change only where a field path
  changed. If a stage feels the need to add, delete, restructure or re-name a
  test, it stops and reports instead.

## Baseline — the suite is red before we start

Captured on `main` at `b38b2ae`, working tree clean apart from an unrelated
staged asset deletion:

- `go build ./...` — clean
- `go vet ./...` — clean
- `go test -race ./...` — **FAILS**, in `internal/ui`, pre-existing and
  unrelated to this work:

```
--- FAIL: TestShow_TracksAnimatedGIFLoopDuration (0.88s)
    testing.go:1712: race detected during execution of test
```

Reproduces 4 runs out of 5 with `-count=5 -run TestShow_TracksAnimatedGIFLoopDuration`.

**The race**, on `viewer.displayFrames` and `viewer.displayFrameIdx`:

| Goroutine | What it does |
|---|---|
| test goroutine | `slideshow_test.go:229`'s `v.ShowImage(index+1)` → `attemptLoad` → `finishLoad` **writes** `displayFrames` (`load.go:195`) and `displayFrameIdx` (`load.go:211`) |
| the *previous* load's `animate` | still cycling the GIF: **reads** `displayFrames` via `redrawRotatedFrame` (`rotate.go:69`) and **writes** `displayFrameIdx` (`load.go:465`) |

The test drops an animated GIF, then calls `ShowImage` for the next file
without waiting out the GIF's `animate` goroutine. `invalidateLoad` cancels
that goroutine's token, but under Fyne's test driver `fyne.Do` runs the
callback *inline on the animate goroutine* rather than marshaling it onto a
UI thread, so cancellation and the goroutine actually noticing are not the
same instant — which is precisely why `animStopped` exists. `waitUntilLoaded`
does not wait on the outgoing load's `animStopped`, only the incoming one's.

Under the real driver `fyne.Do` serializes both onto the UI goroutine, so
this is a test-synchronization gap rather than a product bug — the same class
the concurrency invariant in ARCHITECTURE.md is written about, and the same
class `animFrame`/`animStopped` were added to close.

**This blocks the review gate as written.** Stage 1 edits `finishLoad`
(the vector block at `load.go:205-208` sits inside it) and stage 3 edits its
`resizeToImage` calls — so a red test inside the very function being changed
makes "did the agent break something?" unanswerable. `.github/workflows/ci.yml`
runs the same `go test -race ./...`.

Resolution is a decision for the repo owner — see *Stage 0* below.

### Stage 0 — fix the pre-existing race first (recommended, not yet approved)

Make the gate mean something before any refactor stage runs. Almost certainly
a test-side fix: have `waitUntilLoaded` (or the test itself) wait out the
outgoing load's `animStopped` before asserting, the way `drain` already waits
out every other background signal at teardown. Small, but it is a debugging
task with its own investigation, so it gets its own approval and its own
commit, ahead of stage 1 — and its own `todos.md` entry.

**The alternative**, if the owner would rather not touch it now: record the
failure as a known-red baseline and gate every stage on *no new failures*
(`go test -race ./... -skip TestShow_TracksAnimatedGIFLoopDuration` green,
plus that one test failing identically to the baseline). Workable, but it
leaves the weakest verification exactly where the code changes are densest.

### Update: stage 0 done, and a second race behind it

`TestShow_TracksAnimatedGIFLoopDuration` is fixed — multi-second frame delays
park `animate` for the test's duration, the idiom
`TestCanSaveRotation_FalseForAnimatedImage` and
`TestCanExport_TrueForAnimatedImage` already use for the same reason. 10/10
clean, was 4/5 failing.

A full `-race` run afterwards then surfaced **a second, independent race of
the same shape**, in `TestViewerShow_NavigatingAwayStopsAnimation`
(`animate_test.go`): a 20ms-per-frame GIF, then `v.ShowImage(1)` on the test
goroutine while `animate` is still cycling. It captures `oldAnimStopped`
before navigating and waits on it after, but the race happens *during*
`ShowImage`, before that wait.

It cannot be fixed the same way. Parking the animation would destroy the
test's premise — a live animation superseded by a navigation is exactly what
it exists to prove — so it needs an actual decision about how to exercise
that safely under a driver that runs `fyne.Do` inline. Logged in `todos.md`,
deliberately not fixed inside this refactor.

**Resolved.** It was fixed in commit `b39d7ad`, once CI made the decision for
us — a plain `go test -race ./...` was the only thing standing between the
Stage 1 commit and a green pipeline. Parking the animation preserved the
test's subject after all: `ShowImage` still supersedes the goroutine, which
wakes on `token.context().Done()` inside its frame-delay `select` and returns
*without* entering its `fyne.Do`, so it never touches the two contended
fields. What the test gave up is the claim that the animation was mid-cycle
at the instant of navigation.

**The gate from stage 2a onward is therefore the plain, unskipped**

```
go test -race ./...
```

fully green. No `-skip`, no known-bad test, no permitted failures. A failure
a stage sees is that stage's own.

Both fixes had to park the animation because `animate` sleeps on a bare
`time.After` with no seam, so no test can step it. That is now its own
`todos.md` entry (a frame-clock seam, in the mould of `vector.after`) and is
deliberately outside this refactor.

## Stages

Five stages. Stages 1–4 each go to a sub-agent; after each one I read the
whole diff, verify it against the checks below, fix up whatever needs fixing,
and only then release the next stage. Stage 5 is mine.

Stages 1, 2a+2b, and 3 are the three separately-committable units the TODO
asks for. Stage 4 is the doc consolidation that can only be written once all
three landed.

---

### Stage 1 — `vectorView`

**Agent:** `go-expert` · **Model:** sonnet

**New type, in `internal/ui/vector.go`, above `requestVectorRender`:**

```go
// vectorView is the whole state of the SVG re-render: the parsed document,
// the two sizes the policy compares, the lifecycle its rasterization runs
// under, and the three write-once seams tests replace. A value field on
// viewer, never copied - it holds a WaitGroup and a lifecycle mutex.
type vectorView struct {
	svg       *imaging.Vector
	logical   fyne.Size
	raster    image.Point
	lifecycle requestLifecycle
	pending   sync.WaitGroup
	debounce  time.Duration
	rasterize func(vec *imaging.Vector, w, h int) (image.Image, error)
	after     func(time.Duration) <-chan time.Time
}
```

**Field mapping — exhaustive:**

| Was | Becomes |
|---|---|
| `v.vector` | `v.vector.svg` |
| `v.vectorLogical` | `v.vector.logical` |
| `v.vectorRaster` | `v.vector.raster` |
| `v.vectorLifecycle` | `v.vector.lifecycle` |
| `v.vectorPending` | `v.vector.pending` |
| `v.vectorDebounce` | `v.vector.debounce` |
| `v.vectorRasterize` | `v.vector.rasterize` |
| `v.vectorAfter` | `v.vector.after` |

The `viewer` field is `vector vectorView`. The eight old fields and their
comments leave `viewer.go` entirely; `viewer.go` keeps a short comment on the
new `vector` field pointing at `vector.go`, and the eight detailed comments
move onto the new struct's fields verbatim apart from the names they cite.

**One method moves onto the type.** `clearVector` splits along the line
between "the vector view's own state" and "what the app does about it":

```go
// clear drops the vector state and abandons any re-render in flight, so a
// rasterization started for the previous image can never land on the next.
func (vv *vectorView) clear() {
	vv.svg = nil
	vv.logical = fyne.Size{}
	vv.raster = image.Point{}
	vv.lifecycle.invalidate()
}
```

and `viewer.clearVector()` becomes `v.vector.clear()` followed by the
existing `v.zoom.SetLogicalSize(fyne.Size{})`. `clearVector` keeps its name
and its doc comment — every one of its call sites is unchanged.

**Construction.** `build.go` currently sets the three seams in three
statements after the struct literal. They become:

```go
view.vector.debounce = defaultVectorDebounce
view.vector.rasterize = func(vec *imaging.Vector, w, h int) (image.Image, error) { return vec.RasterAt(w, h) }
view.vector.after = time.After
```

Assignment in place — **not** a `newVectorView()` returning a value.

**Files touched:** `vector.go`, `viewer.go`, `build.go`, `load.go` (4),
`info.go` (4), `rotate.go` (2), `run.go` (1), `vector_test.go` (46),
`harness_test.go` (3).

**Definition of done:**
- Each of these returns zero: `grep -rn "v\.vectorLogical\|v\.vectorRaster\|v\.vectorLifecycle\|v\.vectorPending\|v\.vectorDebounce\|v\.vectorRasterize\|v\.vectorAfter\|view\.vector[A-Z]" --include="*.go" internal/`
- `grep -n "vector" internal/ui/viewer.go` shows only the one new field and its
  pointer-comment.
- `ARCHITECTURE.md`'s `vector.go` and `rotate.go` rows rewritten around the new
  names (`rotate.go`'s row cites `v.vectorLogical` by name today).
- Full verification chain passes.

---

### Stage 2a — `asyncOpUI`, proved on the sort UI

**Agent:** `go-expert` · **Model:** sonnet

Split from stage 2b deliberately: 2a designs the type and converts the
*simpler* of its two users, 2b then applies a type that has already been
proved. Neither half needs a bigger model than sonnet on its own.

**New file `internal/ui/asyncop.go`:**

```go
// asyncOpUI is the shape the folder scan (drop.go) and the background
// reorder (sort.go) share: one cancellable lifecycle, a flag saying whether
// that lifecycle's request is still meaningfully pending, a per-request done
// channel the test suite waits on, and the progress widgets shown for as
// long as it runs. Two instances of one type rather than two parallel sets
// of fields, so the flag-versus-token bookkeeping lives in one place instead
// of spread across drop.go, sort.go and keys.go.
//
// Deliberately viewer-independent: what to *do* about a cancelled operation
// - put the drop zone back, repaint, toast - differs between the two and
// stays at the call sites.
//
// A value field on viewer, never copied: it holds a lifecycle mutex.
type asyncOpUI struct {
	lifecycle requestLifecycle
	active    bool
	done      chan struct{}
	art       *canvas.Image // the scan's Trane-digging art; nil for the sort
	spinner   *widget.ProgressBarInfinite
	label     *widget.Label
}

// begin supersedes any request already in flight, marks the operation
// active, and installs a fresh done channel. The channel is returned as well
// as stored so the caller can capture it: a superseded request must still
// close its own channel without touching the field a newer one now owns.
func (o *asyncOpUI) begin() (requestToken, chan struct{})

// show reveals the progress widgets. Separate from begin because the scan
// sets its label's text first.
func (o *asyncOpUI) show()

// finish clears the active flag and hides the progress widgets. Called by
// the completion step of whichever token is still current - never by a
// stale one, which must not report "nothing in flight" while a newer
// request is still running.
func (o *asyncOpUI) finish()

// invalidate supersedes and cancels the current request, finishing the UI
// only if this operation was actually active. Returns the new revision.
func (o *asyncOpUI) invalidate() uint64

// cancel is invalidate guarded by the flag, reporting whether there was
// anything to cancel. The caller decides what the cancellation means.
func (o *asyncOpUI) cancel() bool
```

**Bare-lifecycle call sites are not a mistake — leave them bare.** Two places
invalidate the lifecycle *without* touching the flag or the widgets, and both
must keep doing exactly that:

- `run.go`'s `SetOnStopped` — shutdown; nothing is going to be repainted.
- `viewer.go`'s `clearToDropzone` — see *Flagged, not fixed* below.

Both become `v.sortOp.lifecycle.invalidate()` / `v.scanOp.lifecycle.invalidate()`,
i.e. reaching through to the field, not the new method. The plan calls this
out so a later reader doesn't "tidy" it into `invalidate()` and change
behavior.

**Sort conversion — field mapping:**

| Was | Becomes |
|---|---|
| `v.sortLifecycle` | `v.sortOp.lifecycle` |
| `v.sorting` | `v.sortOp.active` |
| `v.sortDone` | `v.sortOp.done` |
| `v.sortSpinner` | `v.sortOp.spinner` |
| `v.sortLabel` | `v.sortOp.label` |

`viewer.invalidateSort()` **keeps its name, its signature and its doc
comment** and becomes `return v.sortOp.invalidate()`. Its four callers are
untouched. `startSort` uses `begin()`/`show()`, `finishSort` uses `finish()`,
`cancelSort` becomes:

```go
func (v *viewer) cancelSort() {
	if !v.sortOp.cancel() {
		return
	}
	if len(v.state.files) == 0 {
		v.showWelcomeState()
		v.dropzone.Show()
	}
	v.ForceRepaint()
	v.ShowToast(lang.L("cancelled sorting"))
}
```

Note `cancelSort` today calls `v.invalidateSort()` and *then* hides the two
widgets again redundantly — `invalidateSort` already hid them. The redundant
pair disappears into `cancel()`; that is a dead-store removal, not a
behavior change, and the stage should say so in its report.

**Construction.** `newSortUI()` in `components.go` stays as it is — it returns
only widget pointers, no locks, so returning it by value is fine. `build.go`
stops assigning `sortSpinner`/`sortLabel` in the struct literal and instead
assigns after it:

```go
view.sortOp.spinner = sortUIC.spinner
view.sortOp.label = sortUIC.label
```

**Files touched:** new `asyncop.go`, plus `sort.go` (19), `viewer.go`,
`build.go`, `keys.go` (1 of its 2), `run.go` (1), `sort_test.go` (17),
`filestate_test.go` (7), `harness_test.go` (4).

**Definition of done:**
- Zero hits for `grep -rn "v\.sortLifecycle\|v\.sorting\b\|v\.sortDone\|v\.sortSpinner\|v\.sortLabel\|view\.sort[A-Z]" --include="*.go" internal/`
- `asyncop.go` has no import of anything viewer-specific and no reference to
  `viewer`.
- `ARCHITECTURE.md`'s `sort.go` row rewritten.
- Full verification chain passes.

---

### Stage 2b — the scan UI onto `asyncOpUI`

**Agent:** `go-expert` · **Model:** sonnet

Applies the type stage 2a proved. This half is where the `art` field earns
its nil guard.

**Field mapping:**

| Was | Becomes |
|---|---|
| `v.scanLifecycle` | `v.scanOp.lifecycle` |
| `v.scanning` | `v.scanOp.active` |
| `v.scanDone` | `v.scanOp.done` |
| `v.scanArt` | `v.scanOp.art` |
| `v.scanSpinner` | `v.scanOp.spinner` |
| `v.scanLabel` | `v.scanOp.label` |

`show()`/`finish()` must guard `art` for nil, since the sort instance has
none — and the guard's comment should say which instance that is.

**Call sites:**
- `handleDrop`: `v.invalidateLoad()` stays where it is, then
  `token, scanDone := v.scanOp.begin()`, then the existing
  `v.scanOp.label.SetText(...)`, then `v.scanOp.show()`, then the four
  existing `Hide()` calls on the dropzone widgets, then `v.ForceRepaint()`.
  Ordering unchanged.
- `applyScanResult`: the `scanning = false` + three `Hide()` calls become
  `v.scanOp.finish()`, staying **inside** the `token.current()` guard.
- `cancelScan`: same shape as `cancelSort` above, keeping its own comment
  about why it never touches `v.state.files`, and its own toast string.
- `keys.go`: `v.scanning` → `v.scanOp.active`.
- `clearToDropzone`, `run.go`: bare `.lifecycle.invalidate()` — see 2a.

**Construction:** `newScanUI()` unchanged; `build.go` assigns
`view.scanOp.art/spinner/label` after the struct literal, and the
`scanContainer` composition keeps reading `scan.art`/`scan.spinner`/`scan.label`
from the local, as it does today.

**Files touched:** `drop.go` (19), `viewer.go` (3), `build.go`, `keys.go`,
`openfiles.go` (1), `components.go` (1, a comment citing `v.scanning`),
`run.go` (1), `drop_test.go` (16), `e2e_test.go` (7), `menu_test.go` (6),
`harness_test.go` (3), `filestate_test.go` (2), `sort_test.go` (1),
`openfiles_test.go` (1).

**Definition of done:**
- Zero hits for
  `grep -rnE "v\.scan(Lifecycle|Done|Spinner|Label|Art)\b|v\.scanning\b|view\.scan(Lifecycle|Done|Spinner|Label|Art)\b" --include="*.go" internal/`

  Note the shape: a naive `view\.scan[A-Z]` also matches `view.scanOp`, which
  this stage's own construction and bare-lifecycle lines legitimately
  contain — stage 2a hit exactly that false positive with `view.sortOp` and
  had to disambiguate by inspection. Enumerate the old suffixes instead.
- The two `asyncOpUI` instances are the type's only users; no third appears.
- `ARCHITECTURE.md`'s `drop.go` and `components.go` rows rewritten, new
  `asyncop.go` row added to the file table, placed beside `drop.go`/`sort.go`
  as the table's existing grouping-by-concern ordering implies.
- Full verification chain passes.

---

### Stage 3 — `settings`, and the `settingsWin` rename

**Agent:** `go-expert` · **Model:** sonnet

**Do the rename first, as its own self-contained pass**, then add the struct.
Two mechanical steps in one stage, in that order, so the compiler catches any
confusion between them.

**Step 3.1 — rename the window field.** `viewer.settings *settingswin.Window`
→ `settingsWin`. Ten sites: `startup.go:70`, `run.go:78`, `run.go:122`,
`features.go:83`, `menu.go:57`, `features_test.go:18`, `menu_test.go:156`,
`menu_test.go:162`, `preferences_wiring_test.go:116`,
`preferences_wiring_test.go:310`. The field's doc comment gets a sentence
saying why it is `settingsWin` and not `settings` — the state struct below it
has that name.

**Step 3.2 — the struct, in `memlimits.go`:**

```go
// settings is every value the Settings window's Host surface reads and
// writes, and nothing else: the app's memory budget, the two geometry caps,
// the folder-scan cap, and the favorite-preview-cache toggle. Grouped so
// that surface reads as the single concern it is, and so run.go's
// currentPreferences copy is a flat field-for-field one.
//
// All the megabyte figures are megabytes rather than bytes because that is
// the unit the user types and the unit internal/preferences round-trips; the
// conversion to the byte budgets internal/imaging enforces happens in the
// setters, which stay where their consumers are.
type settings struct {
	maxScan          int
	maxWinW, maxWinH float32
	imgCacheMB       int
	thumbCacheMB     int
	maxFileMB        int
	favPreviewCache  bool
}
```

No locks, so a plain value field `settings settings` on `viewer`. Every
existing field comment moves onto the struct's fields.

**`memlimits.go`'s file doc comment widens.** Today it opens "The three
limits that bound how much memory images are allowed to occupy…" and explains
why those three are grouped away from their consumers. That reasoning stays
and gains the new frame: the file now declares the whole settings-backed
state, while the getter/setter pairs stay beside their consumers
(`MaxScan` in `drop.go`, `MaxWindowWidth`/`MaxWindowHeight` in `load.go`,
`FavoritePreviewCache` in `favthumbs.go`) exactly as before. **No accessor
moves files in this stage.**

**Field mapping:**

| Was | Becomes |
|---|---|
| `v.maxScan` | `v.settings.maxScan` |
| `v.maxWinW` / `v.maxWinH` | `v.settings.maxWinW` / `v.settings.maxWinH` |
| `v.imgCacheMB` | `v.settings.imgCacheMB` |
| `v.thumbCacheMB` | `v.settings.thumbCacheMB` |
| `v.maxFileMB` | `v.settings.maxFileMB` |
| `v.favPreviewCache` | `v.settings.favPreviewCache` |

**Construction stays exactly as split as it is today**: `build.go`'s struct
literal seeds `maxScan`, `maxWinW`, `maxWinH` and `imgCacheMB` directly from
`prefs` (now as a nested `settings: settings{...}` literal), while
`features.go:51-54` continues to seed `thumbCacheMB`, `maxFileMB` and
`favPreviewCache` *through their setters*, because those setters are what
push the value into `grid`/`imaging`. Do not "unify" the two — the split is
load-bearing and `build.go` already carries a comment saying so.

**Files touched:** `memlimits.go` (6), `load.go` (8), `run.go` (7 + 2 rename),
`drop.go` (5), `favthumbs.go` (3), `rotate.go` (1), `startup.go`,
`features.go`, `menu.go`, `viewer.go`, `build.go`, `drop_test.go` (3),
`zoom_test.go` (1), `rotate_test.go` (1), `features_test.go`, `menu_test.go`,
`preferences_wiring_test.go`.

**Explicitly out of scope:** `internal/ui/settingswin/settingswin_test.go` has
fields named `maxScan`, `maxWinW`, `imgCacheMB` on its own `fakeHost`. They
are a different type in a different package. **Do not touch that file.**

**Definition of done:**
- Zero hits for `grep -rn "v\.maxScan\|v\.maxWinW\|v\.maxWinH\|v\.imgCacheMB\|v\.thumbCacheMB\|v\.maxFileMB\|v\.favPreviewCache\|view\.maxScan\|view\.maxWin" --include="*.go" internal/`
- `internal/ui/settingswin/` is byte-identical to before the stage.
- `git diff --stat` shows no change to `translations/`.
- `ARCHITECTURE.md`'s `memlimits.go`, `load.go`, `drop.go` and `settingswin/`
  rows rewritten.
- Full verification chain passes.

---

### Stage 4 — ARCHITECTURE.md consolidation

**Agent:** `go-expert` · **Model:** sonnet · **Docs only — no `.go` file may change.**

Stages 1–3 each rewrote the rows they owned. This stage writes the parts that
could only be written once all three landed, and sweeps for anything stale.

1. **The `viewer.go` row** — rewritten around the new shape. It should say
   what the sub-structs are, and, more usefully, *why these three and not a
   fourth*: each was already a module with its own file and single-writer
   contract, and the grouping is the namespace catching up to a boundary that
   already existed. It should also restate that this is not a controller
   extraction, since that is the thing a reader will suspect.
2. **The concurrency-invariant paragraph** (~line 101) names `vectorPending`,
   `scanLifecycle`, `sortLifecycle` and the `preloadPending` WaitGroup among
   the stop/done signals `newTestUI`'s `drain` waits out. Rewrite it with the
   new paths, and keep its closing instruction — *"if you add a background
   operation, give it a signal and add it there"* — intact.
3. **The "where to look for X" index** (~line 503 onward) — check every entry
   that names one of the moved fields.
4. **Full-file sweep for stale names.** Every one of these must return zero
   hits in `ARCHITECTURE.md`, `README.md` and `AGENTS.md`:
   `vectorLogical`, `vectorRaster`, `vectorLifecycle`, `vectorPending`,
   `vectorDebounce`, `vectorRasterize`, `vectorAfter`, `scanLifecycle`,
   `scanSpinner`, `scanLabel`, `scanArt`, `sortLifecycle`, `sortSpinner`,
   `sortLabel`, `maxWinW`, `maxWinH`, `imgCacheMB`, `thumbCacheMB`,
   `maxFileMB`, `favPreviewCache`.
   `v.scanning`/`v.sorting`/`v.maxScan`/`v.vector` need reading in context
   rather than grepping, since the bare words appear in ordinary prose.
5. Add the `asyncop.go` row if stage 2b did not, and verify the file table
   lists every `internal/ui/*.go` file exactly once.

**Definition of done:** `git diff --name-only` for this stage lists only
`ARCHITECTURE.md`. The stale-name greps return zero.

---

### Stage 5 — close out (mine, not delegated)

- `todos.md`: move the entry from `## TODO` into `## Done` with the
  description this repo's Done entries carry — what the sub-structs are, where
  each type lives, what `asyncOpUI`'s methods do and what deliberately stayed
  at the call sites, and the `settings`/`settingsWin` naming.
- Final full verification chain from a clean tree.
- `git diff` read end to end one last time.
- Three suggested commit messages handed over — stage 1, stages 2a+2b, stage 3
  — with the stage 4 doc consolidation folded into whichever the owner
  prefers, or offered as a fourth.

---

## Flagged, not fixed

`viewer.clearToDropzone` calls `v.scanLifecycle.invalidate()` but never clears
`v.scanning` and never hides the scan spinner, art or label. Reachable via
File ▸ Close Files while a folder scan is running: `keys.go`'s Escape branch
checks `v.scanning` *before* the reset branch, so the keyboard path can't get
there, but the menu item has no such guard. The scan's own completion closure
then finds its token stale and returns without clearing the flag either — the
comment on `scanning` says as much: *"A scan superseded by a newer drop (stale
gen) never touches it, since the newer scan already owns the flag."* There is
no newer scan here.

Whether that leaves visible stuck widgets depends on what `clearToDropzone`
repaints afterward, which is why this is flagged rather than asserted. It is a
pre-existing condition, it is **not** in scope for a behavior-preserving
refactor, and stage 2b must reproduce it exactly. Worth its own `todos.md`
entry afterward.

## Review gate between every stage

Not a formality — it is why this is five stages instead of one. After each
sub-agent reports, before the next is released:

1. Read the full `git diff`, not the agent's summary of it.
2. Check the stage's own "definition of done" greps myself.
3. Confirm the diff is *only* renames plus the new type: no reordered
   statements, no removed comments, no "while I was in there" changes, no new
   or altered tests.
4. Run the full verification chain myself.
5. Fix up whatever needs fixing before releasing the next stage — the next
   stage builds on this one's shape.

## Why sonnet everywhere

Every stage is a mechanical transformation against an exhaustive field map,
with the design decisions already made here and the tricky semantics
(bare-lifecycle call sites, the stale-token guards, the `copylocks`
constraint, the construction split) spelled out as constraints rather than
left to be rediscovered. The one stage with real design content — inventing
`asyncOpUI` and reconciling two call-site families — is split into 2a and 2b
precisely so that neither half needs more than sonnet. If a stage's agent
reports that the shape does not fit, that is a signal to stop and re-plan, not
to escalate the model.
