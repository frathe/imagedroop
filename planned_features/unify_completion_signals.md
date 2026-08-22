# Unify the test-synchronization channels behind `internal/completion`

> **For agentic workers:** REQUIRED SUB-SKILL: use
> `superpowers:subagent-driven-development` to implement this plan stage by
> stage. Steps use checkbox (`- [ ]`) syntax for tracking.

Implementation plan for item 5 in `todos.md` — "Unify the
test-synchronization channels behind one small type". The deliverable is
one audited `completion.Signal` type replacing nine hand-rolled copies of
the same replace-on-start / close-on-finish / wait-in-test contract, with
the general type owning the mechanism (the channel, its replacement, its
idempotent close) and each call site keeping its own domain-specific
staleness rules on top of it.

**Goal:** Collapse nine `chan struct{}` fields, nine field-comment
restatements of the same discipline, and eleven hand-rolled waiters into
one package with two types and two test helpers.

**Architecture:** A new viewer-independent `internal/completion` package
exports `Signal`, a mutex-guarded, replaceable one-shot completion channel.
`Begin()` installs a fresh generation and returns the `func()` that closes
*that* generation — so a superseded request still closes its own channel
without touching the field a newer one now owns, which is exactly the rule
currently enforced only by prose. `Wait(ctx)`, `Begun()` and `Current()`
serve the test suite, the last returning a `Handle` that keeps naming one
generation across a supersession. Migration is incremental: `drain`
carries two lists (raw channels not yet migrated, Signals already
migrated) so every stage compiles and passes on its own.

**Tech Stack:** Go (module targets 1.26.7; local toolchain go1.27.0), Fyne v2 test driver, `go test -race`.

**Precedent:** `internal/decodepool` (`todos.md`, item 4's stretch goal) —
one audited type replacing N hand-rolled copies of one contract, the
general type owning the mechanism, callers keeping their domain rules.
`internal/completion` is deliberately the same move and should read like a
sibling of it.

---

## Global Constraints

Copied from `AGENTS.md`; every stage's requirements implicitly include
these.

- **Do not run `git commit`.** Each stage ends with a *suggested* commit
  message for the user. This overrides the "Commit" step the
  writing-plans skill would normally end a task with.
- Do not add `TODO`/`FIXME` comments to source. Open work belongs in
  `todos.md`.
- Update `ARCHITECTURE.md` in the same change when packages are added —
  `internal/completion` is a new package, so this is mandatory (Stage 8).
- Every goroutine needs cancellation/staleness handling plus an observable
  stop/done signal, and any new background work must be added to
  `newTestUI`'s `drain` cleanup in `internal/ui/harness_test.go`.
- No mutable package-level test seams. Runtime/test-configurable values
  belong on `viewer` or the owning feature.
- Report UI-boundary failures with `fyne.LogError`; viewer-independent
  packages return errors. `internal/completion` is viewer-independent: no
  Fyne types, no `fyne.Do`, no UI marshaling.
- Mark intentionally ignored errors explicitly (`_ =`).
- Verification per stage, from the repository root:
  `gofmt -l .` (must print nothing), `go vet ./...`, `go build ./...`,
  `go test -race ./...`.

---

## Baseline (measured 2026-08-22, before any change)

| fact | value |
| --- | --- |
| one-shot done channels on `viewer`/its components | 9 |
| `internal/ui/viewer.go` | 772 lines |
| `internal/ui/harness_test.go` | 462 lines |
| distinct `close(done)` sites | 12 (4 of them for `loadDone` alone) |
| producers writing a *shared* channel field | 2 pairs — `clipboardDone` (`clipboard.go:48`, `batch.go:109`), `chooserDone` (`openfiles.go:31`, `export.go:129`) |
| hand-rolled waiters in `harness_test.go` | 7 (`drain`'s table + `waitUntilLoaded`, `waitForScan`, `waitForSort`, `waitForAnimStopped`, `settleChooser`, `settleToast`) |
| inline `select`-on-channel waiters in feature tests | 4 (`clipboard_test.go:106`, `batch_test.go:48`, `favthumbs_test.go:30`, `wallpaper_test.go:37`) |
| tests asserting a channel field is `nil`/non-`nil` | 4 (`wallpaper_test.go:208`, `favthumbs_test.go:95`, `export_test.go:662`, `imgcache_test.go:207`) |
| fields ever reset to `nil` after use | 0 — verified by grep; `Begun()`'s monotonic semantics therefore match today's exactly |

### The nine signals, and the three shapes they come in

| # | today | producer(s) | closed at | shape |
| --- | --- | --- | --- | --- |
| 1 | `scanOp.done` | `asyncop.go:37` `begin()` | `drop.go:158` | returned-from-`begin`, closed by `defer` in a completion method |
| 2 | `sortOp.done` | `asyncop.go:37` `begin()` | `sort.go:104` | same |
| 3 | `loadDone` | `load.go:62` | `load.go:97`, `:141`, `:308`, `:428` | **threaded as a `done chan struct{}` parameter** through `attemptLoad` → `finishLoad` / `retryAfterLoadFailure`, closed at four points |
| 4 | `clipboardDone` | `clipboard.go:48`, `batch.go:109` | `clipboard.go:51`, `batch.go:112` | plain `make` + `defer close(done)` |
| 5 | `chooserDone` | `openfiles.go:31`, `export.go:129` | `openfiles.go:34`, `export.go:132` | same |
| 6 | `wallpaperDone` | `wallpaper.go:64` | `wallpaper.go:67` | same |
| 7 | `favThumbDone` | `favthumbs.go:61` | `favthumbs.go:64` | same |
| 8 | `animStopped` | `load.go:294` | `load.go:448` (`defer`) | captured and passed to `animate` as a parameter |
| 9 | `toast.done` | `toast.go:85` | `toast.go:94` (`defer`) | plain, paired with a separate `toast.stop` cancel channel |

`animFrame` (`viewer.go:245`) stays an `atomic.Uint64`. It is a different
contract — an N-event counter that tests poll, not a one-shot close — and
`waitForAnimFrame` stays exactly as it is. Same for `vector.pending`
(a `sync.WaitGroup`), `preloads` (`decodepool.Pool`), and `grid.Settle` /
`slides.Settle`: those are N-goroutine waits, not one-shot completions.
`toast.stop` also stays: it is a *cancel* signal, not a completion.

---

## Design

### The type

```go
type Signal struct {
	mu   sync.Mutex
	done chan struct{}
}

func (s *Signal) Begin() (done func())
func (s *Signal) Wait(ctx context.Context) error
func (s *Signal) Begun() bool
func (s *Signal) Current() Handle

type Handle struct{ ch chan struct{} }

func (h Handle) Wait(ctx context.Context) error
```

Five decisions, and why:

**`Begin` returns the closer, and never the channel.** The one rule this
type exists to make unbreakable is "a stale generation must still close its
own channel". Handing back a `func()` that has closed over *this*
generation's channel makes that automatic: a superseded producer literally
cannot reach the field a newer one now owns. This is also why the load
case works unchanged — `load.go` already threads its `done chan struct{}`
through `attemptLoad`/`finishLoad`/`retryAfterLoadFailure` as a parameter,
and a `func()` threads identically.

**The closer is idempotent (`sync.Once`).** Today a double close panics.
For a primitive whose whole job is to be hard to misuse, idempotence is
the right call — it is what `context.CancelFunc` does. No current site
closes twice; this removes the class.

**`Wait` on a never-begun Signal returns `nil` immediately.** That
reproduces `drain`'s current `if c.ch == nil { continue }` for free. Call
sites that *require* the operation to have started (`settleChooser`)
assert `Begun()` explicitly first, exactly as they assert `!= nil` today.

**`Wait` snapshots the channel under the mutex, then selects.** Reading
the field once and then waiting is precisely what `case <-v.loadDone` does
today. The mutex is the one behavior change: it removes the hazard
documented at `openfiles.go:42`, where `runFileChooser` was split out of
`openFileDialog` *only* because a background goroutine writing
`v.chooserDone` races a test reading it. That split stays (it is still
useful for driving the path on the test goroutine), but the race it dodges
stops existing.

**`Current()` returns a `Handle` naming one generation, so a caller can
wait for the generation it was looking at even after a newer one has
superseded it.** This is not speculative API — it is required by
`drop_test.go:527`
(`TestHandleDrop_SupersededScanGoroutineExits`), which captures
`scanDoneA := v.scanOp.done`, then drops a *second* folder (replacing the
generation) and waits on the first to prove the superseded scan's
goroutine exited. Without `Handle` that test cannot be expressed and would
have to be rewritten into something that waits on the current generation —
which is already finished by then, making the test silently vacuous while
still passing. `Signal.Wait(ctx)` is then just
`s.Current().Wait(ctx)`, and the zero `Handle` waits on nothing and
returns `nil`, matching `Signal.Wait`'s never-begun case.

### Migration shape

`drain` carries two lists while the migration is in flight:

```go
for _, c := range []struct {
	name string
	ch   chan struct{}
}{
	// entries move out of here, one stage at a time, until empty
} { ... }

for _, c := range []struct {
	name string
	sig  *completion.Signal
}{
	// ...and into here
} { waitFor(t, c.name, c.sig) }
```

Stage 8 deletes the first loop once it is empty. Every stage in between
compiles and passes `go test -race ./...` on its own, which is what makes
this safe to hand to a fresh sub-agent per stage.

### Field naming after migration

The `Done` suffix goes away — `v.clipboard.Wait(ctx)` reads better than
`v.clipboardDone.Wait(ctx)`, and the type already says "completion".

| today | after |
| --- | --- |
| `v.loadDone` | `v.load` |
| `v.clipboardDone` | `v.clipboard` |
| `v.chooserDone` | `v.chooser` |
| `v.wallpaperDone` | `v.wallpaper` |
| `v.favThumbDone` | `v.favThumb` |
| `v.animStopped` | `v.anim` |
| `v.scanOp.done` | `v.scanOp.done` (unchanged name, new type) |
| `v.sortOp.done` | `v.sortOp.done` (unchanged name, new type) |
| `t.done` (toast) | `t.hidden` |

Check for collisions before renaming: `viewer` already has a `load`-ish
surface. Stage 6 verifies with `grep -n "v\.load\b" internal/ui/` before
committing to the name and falls back to `loadOp` if `load` collides.

---

## Stage sequencing and model assignment

Simple sites first, so the type is proven by five call sites before the
hard one lands. `loadDone` (Stage 6) is the only stage that changes
function signatures across a retry chain; `asyncOpUI` (Stage 7) changes a
return type.

| stage | work | model | why |
| --- | --- | --- | --- |
| 1 | the `internal/completion` package + its tests | **sonnet** | self-contained new package, TDD, no callers |
| 2 | `clipboardDone` + `chooserDone` + harness scaffolding | **sonnet** | mechanical; 2 producers each |
| 3 | `wallpaperDone` + `favThumbDone` | **sonnet** | mechanical; single producer each |
| 4 | `animStopped` | **sonnet** | one producer, one param, one nil-assertion |
| 5 | `toast.done` | **sonnet** | one file, one helper |
| 6 | `loadDone` | **fable** | 4 close sites, signature changes through a retry chain, the subtlest correctness argument in the package |
| 7 | `scanOp.done` + `sortOp.done` via `asyncOpUI` | **sonnet** | mechanical once the pattern is established |
| 8 | cleanup + docs | **sonnet** | deletion and prose |

I review every stage against `gofmt -l .`, `go vet ./...`,
`go build ./...` and `go test -race ./...` myself — running the commands,
not reading the agent's report — and fix up the code before dispatching
the next stage.

---

## Stage 1: the `internal/completion` package

> **Pre-verified.** The `completion.go` and `completion_test.go` code in
> this stage was extracted verbatim into a throwaway module and run before
> this plan was handed over: `gofmt -l` clean, `go vet` clean, and all eight
> tests pass under `-race`. Type it in as written; if it does not build,
> something was transcribed wrong.

**Files:**
- Create: `internal/completion/completion.go`
- Create: `internal/completion/completion_test.go`

**Interfaces:**
- Consumes: nothing.
- Produces: `completion.Signal` with `Begin() (done func())`,
  `Wait(ctx context.Context) error`, `Begun() bool`, `Current() Handle`;
  and `completion.Handle` with `Wait(ctx context.Context) error`. Every
  later stage depends on exactly these signatures.

- [ ] **Step 1: Write the failing tests**

Create `internal/completion/completion_test.go`:

```go
package completion_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/frathe/picfetch/internal/completion"
)

// waitTimeout is the deadline every wait in this file gets. A passing test
// returns as soon as its channel closes and never waits this long.
const waitTimeout = 2 * time.Second

func waitCtx(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	t.Cleanup(cancel)

	return ctx
}

// A Signal nobody has begun has nothing to wait for, so Wait returns
// immediately - the behavior drain's "skip the nil channel" relies on.
func TestSignal_WaitBeforeBeginReturnsImmediately(t *testing.T) {
	var s completion.Signal

	if err := s.Wait(waitCtx(t)); err != nil {
		t.Fatalf("Wait on a never-begun Signal = %v, want nil", err)
	}
	if s.Begun() {
		t.Error("Begun on a never-begun Signal = true, want false")
	}
}

func TestSignal_WaitBlocksUntilDone(t *testing.T) {
	var s completion.Signal

	done := s.Begin()
	if !s.Begun() {
		t.Fatal("Begun after Begin = false, want true")
	}

	released := make(chan error, 1)
	go func() { released <- s.Wait(waitCtx(t)) }()

	select {
	case err := <-released:
		t.Fatalf("Wait returned %v before done was called, want it to block", err)
	case <-time.After(20 * time.Millisecond):
	}

	done()

	select {
	case err := <-released:
		if err != nil {
			t.Fatalf("Wait after done = %v, want nil", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Wait never returned after done was called")
	}
}

func TestSignal_WaitRespectsContextCancellation(t *testing.T) {
	var s completion.Signal

	s.Begin() // deliberately never called: the operation never finishes

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.Wait(ctx); err == nil {
		t.Fatal("Wait with a cancelled context = nil, want the context's error")
	}
}

// The rule the whole type exists for: a superseded generation still closes
// its own channel, and doing so must not release a waiter on the newer one.
func TestSignal_SupersededGenerationClosesItsOwnChannel(t *testing.T) {
	var s completion.Signal

	stale := s.Begin()
	fresh := s.Begin()

	released := make(chan error, 1)
	go func() { released <- s.Wait(waitCtx(t)) }()

	stale()

	select {
	case err := <-released:
		t.Fatalf("the stale generation's done released a waiter on the fresh one (err = %v)", err)
	case <-time.After(20 * time.Millisecond):
	}

	fresh()

	select {
	case err := <-released:
		if err != nil {
			t.Fatalf("Wait after the fresh done = %v, want nil", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Wait never returned after the fresh generation finished")
	}
}

// A Handle keeps naming the generation it was taken from, so a caller can
// still wait out a request that has since been superseded. This is what
// drop_test.go's TestHandleDrop_SupersededScanGoroutineExits needs: it
// waits for the *first* scan's goroutine to exit after a second drop has
// already replaced the generation.
func TestHandle_WaitsItsOwnGenerationAfterSupersession(t *testing.T) {
	var s completion.Signal

	stale := s.Begin()
	staleHandle := s.Current()

	fresh := s.Begin()

	released := make(chan error, 1)
	go func() { released <- staleHandle.Wait(waitCtx(t)) }()

	// Finishing the *newer* generation must not release a waiter holding a
	// handle on the older one.
	fresh()

	select {
	case err := <-released:
		t.Fatalf("the fresh generation released a waiter on the stale handle (err = %v)", err)
	case <-time.After(20 * time.Millisecond):
	}

	stale()

	select {
	case err := <-released:
		if err != nil {
			t.Fatalf("Handle.Wait after its own done = %v, want nil", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Handle.Wait never returned after its own generation finished")
	}
}

// The zero Handle - taken from a Signal nobody has begun - has nothing to
// wait for, matching Signal.Wait's own never-begun case.
func TestHandle_ZeroValueWaitsForNothing(t *testing.T) {
	var s completion.Signal

	if err := s.Current().Wait(waitCtx(t)); err != nil {
		t.Fatalf("Wait on a never-begun Signal's handle = %v, want nil", err)
	}
}

// Idempotent, so a retry chain that can reach its finish twice reports
// completion instead of panicking on a second close.
func TestSignal_DoneIsIdempotent(t *testing.T) {
	var s completion.Signal

	done := s.Begin()
	done()
	done()
	done()

	if err := s.Wait(waitCtx(t)); err != nil {
		t.Fatalf("Wait after a repeated done = %v, want nil", err)
	}
}

// Begin/Wait/Begun from several goroutines at once must be race-free: this
// is the hazard openfiles.go:42 documents, and the reason Signal is
// mutex-guarded rather than a plain field.
func TestSignal_ConcurrentBeginWaitAndBegun(t *testing.T) {
	var s completion.Signal

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			done := s.Begin()
			_ = s.Begun()
			_ = s.Wait(waitCtx(t))
			done()
		})
	}
	wg.Wait()

	// Whichever generation ended up current, its done was called by the
	// goroutine that began it, so a final Wait must not block.
	if err := s.Wait(waitCtx(t)); err != nil {
		t.Fatalf("Wait after the concurrent burst = %v, want nil", err)
	}
}
```

Note for the implementer: `wg.Go` (no explicit `wg.Add(1)`) is Go 1.25+
and is already the style used in `internal/decodepool/decodepool.go:81`.
`for range 8` is Go 1.22+ range-over-int, already used at
`drop_test.go:515`. Both are fine on this repo's `go 1.26.7`.

- [ ] **Step 2: Run the tests to verify they fail**

Run: `go test ./internal/completion/...`
Expected: FAIL — the package `internal/completion` does not exist yet
(`no required module provides package .../internal/completion`).

- [ ] **Step 3: Write the implementation**

Create `internal/completion/completion.go`:

```go
// Package completion is the one-shot "this background operation has
// finished" signal that internal/ui grew nine hand-rolled copies of: a
// channel replaced at the start of each request and closed when that
// request finishes, which the test suite waits on instead of polling
// widget state a producer goroutine may still be writing.
//
// The rule it makes unbreakable is the one those nine copies could only
// state in prose: a request that has been superseded must still close its
// own channel, without touching the field a newer request now owns. Begin
// hands back a func closed over this generation's channel, so a stale
// producer cannot reach the newer one even by accident.
//
// It is deliberately viewer-independent: no Fyne types, no fyne.Do, no UI
// marshaling. The caller decides what counts as stale and what finishing
// means; Signal answers only "has the generation I am looking at finished
// yet".
package completion

import (
	"context"
	"sync"
)

// Signal is a replaceable one-shot completion channel. The zero Signal is
// ready to use and reports Begun() == false.
//
// Safe for concurrent use. That matters more than it looks: the fields
// this type replaces were written by background goroutines and read by the
// test goroutine with nothing synchronizing the two, which is the hazard
// openfiles.go's runFileChooser was split out to dodge.
type Signal struct {
	mu   sync.Mutex
	done chan struct{}
}

// Begin supersedes any generation already in flight and returns the
// function that finishes *this* one. Call it exactly where the old code
// did `defer close(done)`.
//
// The returned func is idempotent: calling it twice is a no-op rather than
// the panic a repeated close(chan) would be, so a retry chain that can
// reach its finish along two paths stays correct.
//
// Deliberately no way to get the channel itself: the whole point is that a
// superseded producer holds a closer over its own generation and nothing
// else.
func (s *Signal) Begin() (done func()) {
	ch := make(chan struct{})

	s.mu.Lock()
	s.done = ch
	s.mu.Unlock()

	var once sync.Once

	return func() { once.Do(func() { close(ch) }) }
}

// Current returns a Handle naming the generation in flight right now, so a
// caller can wait out *that* request even after a newer one has superseded
// it. Taken from a Signal that has never begun, the zero Handle waits for
// nothing.
//
// This exists for one real case: a test that starts a request, starts a
// second one that replaces it, and then wants to prove the first one's
// goroutine actually exited. Waiting on the Signal itself would wait on
// the second generation and prove nothing.
func (s *Signal) Current() Handle {
	s.mu.Lock()
	defer s.mu.Unlock()

	return Handle{ch: s.done}
}

// Handle names one generation of a Signal. It keeps naming that generation
// for good, which is what separates it from Signal.Wait: the Signal moves
// on, a Handle does not. The zero Handle has nothing to wait for.
type Handle struct {
	ch chan struct{}
}

// Wait blocks until this handle's generation finishes or ctx is done,
// returning ctx.Err() only in the latter case. The zero Handle returns nil
// immediately.
func (h Handle) Wait(ctx context.Context) error {
	if h.ch == nil {
		return nil
	}

	select {
	case <-h.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Wait blocks until the generation current *at the moment Wait is called*
// has finished, or until ctx is done, whichever happens first. It returns
// ctx.Err() only in the latter case.
//
// A Signal that has never begun has nothing to wait for and returns nil
// immediately - the "a viewer that never scanned has nothing to drain"
// case, which callers would otherwise each have to special-case.
//
// The channel is snapshotted under the lock and waited on outside it, so a
// waiter never blocks a producer's Begin. A generation that starts after
// this snapshot is not waited for, exactly as reading the old channel
// field once and selecting on it behaved. A caller that needs to keep
// waiting on a specific generation across a supersession takes a Handle
// instead.
//
// The application never needs this; tests do.
func (s *Signal) Wait(ctx context.Context) error {
	return s.Current().Wait(ctx)
}

// Begun reports whether Begin has ever been called - "did this operation
// ever start", not "is it still running". It replaces the `!= nil` checks
// tests used to make against the raw channel fields, and is monotonic for
// the same reason those were: nothing ever puts a Signal back to its zero
// state.
func (s *Signal) Begun() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.done != nil
}
```

- [ ] **Step 4: Run the tests to verify they pass**

Run: `go test -race ./internal/completion/...`
Expected: PASS, all eight tests.

- [ ] **Step 5: Verify the whole repo is still green**

Run, from the repository root:

```bash
gofmt -l .
go vet ./...
go build ./...
go test -race ./...
```

Expected: `gofmt -l .` prints nothing; the rest pass. Nothing else in the
repo references the new package yet, so this stage cannot break anything —
if it does, the failure is pre-existing and must be reported, not fixed
here.

- [ ] **Step 6: Report, do not commit**

Per `AGENTS.md`, do not run `git commit`. End with this suggested message:

```
Add internal/completion: one audited replace-on-start/close-on-finish signal

internal/ui grew nine hand-rolled copies of the same one-shot done-channel
contract. This is the type they collapse into, following internal/decodepool's
precedent: the general type owns the mechanism, callers keep their own
staleness rules. Begin returns a closer over its own generation, so a
superseded request cannot touch the field a newer one owns.
```

---

## Stage 2: `clipboardDone` and `chooserDone`, plus the harness scaffolding

This stage also introduces the shared `waitFor` helper and `drain`'s
second list, because it is the first migration. Later stages just add rows.

**Files:**
- Modify: `internal/ui/viewer.go:370-377` (the field block)
- Modify: `internal/ui/clipboard.go:43-52`
- Modify: `internal/ui/batch.go:106-113`
- Modify: `internal/ui/openfiles.go:21-36`
- Modify: `internal/ui/export.go:124-133`
- Modify: `internal/ui/harness_test.go` (`drain`, `settleChooser`, and the new `waitFor`)
- Modify: `internal/ui/clipboard_test.go:100-110`
- Modify: `internal/ui/batch_test.go:42-52`
- Modify: `internal/ui/export_test.go:662`

**Interfaces:**
- Consumes: `completion.Signal` / `Begin` / `Wait` / `Begun` from Stage 1.
- Produces: `v.clipboard` and `v.chooser` as `completion.Signal` value
  fields on `viewer`; `waitFor(t *testing.T, name string, s *completion.Signal)`
  in `harness_test.go`, which every later stage reuses.

- [ ] **Step 1: Replace the two field declarations**

In `internal/ui/viewer.go`, replace the `clipboardDone`/`chooserDone`/
`wallpaperDone` block at lines 370-377 with the version below. Leave
`wallpaperDone` alone for now — Stage 3 takes it — and note the comment
shrinks because the discipline it used to restate now lives on the type.

```go
	// clipboard is begun by copyImageToClipboard (clipboard.go) and
	// copyGridSelection (batch.go) and finished once that goroutine has
	// fully run, error reporting included. chooser is the same for the
	// native file dialog, shared by openFileDialog (openfiles.go) and
	// exportAs (export.go) - they mean "the native dialog goroutine" and
	// are never in flight at once, since both panels are app-modal.
	// See internal/completion for the contract all of these keep.
	//
	// Value fields, never copied: each holds a mutex.
	clipboard completion.Signal
	chooser   completion.Signal

	wallpaperDone chan struct{}
```

Add the import: `"github.com/frathe/picfetch/internal/completion"`.

- [ ] **Step 2: Convert the four producers**

`internal/ui/clipboard.go` — replace lines 43-52's channel plumbing:

```go
	// clipboard is finished once this copy's goroutine has fully run,
	// error reporting included, so a test can wait for the whole
	// operation instead of polling widget state the goroutine may still
	// be writing.
	done := v.clipboard.Begin()

	go func() {
		defer done()

		if err := clipboard.CopyImage(data); err != nil {
			v.reportClipboardError(err)
		}
	}()
```

`internal/ui/batch.go` — same shape at lines 106-113:

```go
	done := v.clipboard.Begin()

	go func() {
		defer done()

		if err := clipboard.CopyFiles(paths); err != nil {
			v.reportFileCopyError(err)

			return
		}

		fyne.Do(func() {
```

(leave the rest of that goroutine body exactly as it is)

`internal/ui/openfiles.go` — at lines 29-36. Keep the long comment about
why waiting matters, drop only the sentence restating the channel
discipline:

```go
	done := v.chooser.Begin()

	go func() {
		defer done()

		v.runFileChooser()
	}()
```

`internal/ui/export.go` — at lines 124-133:

```go
	// chooser is shared with openFileDialog's own goroutine rather than
	// given a twin of its own: it means "the native file dialog
	// goroutine", and these two are never in flight at once - both panels
	// are app-modal, so neither can be reached while the other is up.
	done := v.chooser.Begin()

	go func() {
		defer done()

		v.runExport(src, img, ext)
	}()
```

- [ ] **Step 3: Add `waitFor` and the second drain list**

In `internal/ui/harness_test.go`, add the helper next to `testTimeout`
(after line 202):

```go
// waitFor blocks until s's current operation finishes, failing the test on
// timeout. One helper for every completion.Signal on the viewer, so the
// testTimeout deadline lives in exactly one place instead of being
// restated by a hand-rolled select per operation.
func waitFor(t *testing.T, name string, s *completion.Signal) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := s.Wait(ctx); err != nil {
		t.Fatalf("timed out waiting for %s", name)
	}
}
```

Add imports `"context"` and
`"github.com/frathe/picfetch/internal/completion"`.

In `drain`, remove the `{"clipboard copy", v.clipboardDone}` and
`{"file chooser", v.chooserDone}` rows from the existing table, and add
the second loop directly after it:

```go
	for _, c := range []struct {
		name string
		sig  *completion.Signal
	}{
		{"clipboard copy", &v.clipboard},
		{"file chooser", &v.chooser},
	} {
		waitFor(t, c.name, c.sig)
	}
```

Note there is no `nil` guard on this loop and none is needed: `Wait` on a
never-begun Signal returns immediately. Update `drain`'s doc comment
(lines 118-121) to say so — the current text explains the nil-skip, which
no longer exists for migrated entries.

- [ ] **Step 4: Convert `settleChooser` and the inline test waiters**

`harness_test.go`'s `settleChooser` (lines 366-380):

```go
func settleChooser(t *testing.T, v *viewer) {
	t.Helper()

	if !v.chooser.Begun() {
		t.Fatal("no file-chooser goroutine pending to settle")
	}

	waitFor(t, "the file-chooser goroutine", &v.chooser)
}
```

`clipboard_test.go` at lines ~100-110 and `batch_test.go` at ~42-52: each
has a `select { case <-v.clipboardDone: case <-time.After(testTimeout): ... }`.
Replace the whole select with:

```go
	waitFor(t, "the clipboard copy", &v.clipboard)
```

Keep the surrounding explanatory comments; only the select goes. Remove
the now-unused `"time"` import from either file **only if** nothing else
in it uses `time`.

`export_test.go:662` asserts `v.wallpaperDone != nil` — that is Stage 3's
field, leave it. Search `export_test.go` for `chooserDone`; if any
assertion there reads the chooser channel, convert it to
`v.chooser.Begun()`.

- [ ] **Step 5: Verify**

```bash
gofmt -l .
go vet ./...
go build ./...
go test -race ./internal/ui/...
go test -race ./...
```

Expected: all pass. Run the focused suite first — it is where any breakage
will be.

- [ ] **Step 6: Report, do not commit**

Suggested message:

```
Move the clipboard and file-chooser done channels onto completion.Signal

First two of nine. Adds harness_test.go's waitFor helper and drain's
Signal list, which the remaining migrations reuse.
```

---

## Stage 3: `wallpaperDone` and `favThumbDone`

**Files:**
- Modify: `internal/ui/viewer.go` (the `wallpaperDone` and `favThumbDone` fields)
- Modify: `internal/ui/wallpaper.go:60-69`
- Modify: `internal/ui/favthumbs.go:58-66`
- Modify: `internal/ui/harness_test.go` (`drain`'s two lists)
- Modify: `internal/ui/wallpaper_test.go:27-40` and `:208`
- Modify: `internal/ui/favthumbs_test.go:19-32` and `:95`
- Modify: `internal/ui/export_test.go:662`

**Interfaces:**
- Consumes: `completion.Signal`, and `waitFor` from Stage 2.
- Produces: `v.wallpaper`, `v.favThumb` as `completion.Signal` fields.

- [ ] **Step 1: Replace the field declarations**

In `internal/ui/viewer.go`, the `wallpaperDone chan struct{}` line left by
Stage 2 and the `favThumbDone` block become:

```go
	// wallpaper is begun by setAsWallpaper (wallpaper.go) and finished
	// once the change has fully landed, toast included. favThumb is the
	// same for SyncFavoritePreviews' pass over a favorite's previews
	// (favthumbs.go); it is begun on every pass, so a test waits on it
	// after triggering one rather than holding it across two.
	// See internal/completion for the contract.
	//
	// Value fields, never copied: each holds a mutex.
	wallpaper completion.Signal
	favThumb  completion.Signal
```

- [ ] **Step 2: Convert the two producers**

`internal/ui/wallpaper.go` at lines 60-69:

```go
	// wallpaper is finished once this change has fully landed, toast
	// included, so a test can read widget state without racing the
	// goroutine that writes it.
	done := v.wallpaper.Begin()

	go func() {
		defer done()

		v.applyWallpaper(src, img)
	}()
```

`internal/ui/favthumbs.go` at lines 58-66 — note `Begin()` goes *after*
`v.favThumbLifecycle.begin()`, keeping the existing order:

```go
	token := v.favThumbLifecycle.begin()

	done := v.favThumb.Begin()

	go func() {
		defer done()

		if err := favthumbs.Sync(token.context(), favDir, files, gridSink{v.grid}); err != nil {
```

(leave the error handling below it untouched)

- [ ] **Step 3: Move the two drain rows**

In `harness_test.go`'s `drain`, delete `{"wallpaper", v.wallpaperDone}`
and `{"favorite previews", v.favThumbDone}` from the channel table and add
to the Signal loop:

```go
		{"wallpaper", &v.wallpaper},
		{"favorite previews", &v.favThumb},
```

- [ ] **Step 4: Convert the tests**

`wallpaper_test.go` lines ~27-40 — the helper that checks non-nil then
selects. It becomes:

```go
	if !v.wallpaper.Begun() {
		t.Fatal("no wallpaper goroutine pending to settle")
	}

	waitFor(t, "the wallpaper change", &v.wallpaper)
```

`wallpaper_test.go:208` — `if v.wallpaperDone != nil {` becomes
`if v.wallpaper.Begun() {`. Read the surrounding assertion message first
and keep it: it asserts the operation was never *started*, which is
exactly what `Begun()` reports.

`export_test.go:662` — the same `v.wallpaperDone != nil` check, same
conversion to `v.wallpaper.Begun()`.

`favthumbs_test.go` lines ~19-32:

```go
	if !v.favThumb.Begun() {
		t.Fatal("no favorite-preview pass pending to settle")
	}

	waitFor(t, "the favorite-preview pass", &v.favThumb)
```

`favthumbs_test.go:95` — `if v.favThumbDone != nil {` becomes
`if v.favThumb.Begun() {`.

Drop any now-unused `"time"` imports.

- [ ] **Step 5: Verify**

```bash
gofmt -l .
go vet ./...
go build ./...
go test -race ./internal/ui/...
go test -race ./...
```

Pay attention to `TestSetAsWallpaper*` and `TestSyncFavoritePreviews*`
specifically — they are the tests whose nil-assertions changed meaning.
Run them verbosely if anything is red:
`go test -race -run 'Wallpaper|FavoritePreview' -v ./internal/ui/`

- [ ] **Step 6: Report, do not commit**

```
Move the wallpaper and favorite-preview done channels onto completion.Signal

Four of nine. The `!= nil` assertions that meant "this never started"
become Begun(), which reports exactly that.
```

---

## Stage 4: `animStopped`

**Files:**
- Modify: `internal/ui/viewer.go:232-246` (the `animFrame`/`animStopped` comment block)
- Modify: `internal/ui/load.go:292-296` (arming) and `:436-448` (`animate`)
- Modify: `internal/ui/harness_test.go:303-313` (`waitForAnimStopped`)
- Modify: `internal/ui/animate_test.go:146-155` and `:179-181`
- Modify: `internal/ui/imgcache_test.go:207-209`

**Interfaces:**
- Consumes: `completion.Signal`, `waitFor`.
- Produces: `v.anim` as a `completion.Signal`; `animate`'s signature
  becomes `func (v *viewer) animate(token requestToken, frames []image.Image, delays []time.Duration, stopped func())`.

- [ ] **Step 1: Replace the field**

`internal/ui/viewer.go` — keep `animFrame` exactly as it is (it is an
`atomic.Uint64` counter, a different contract, and stays). Replace only
`animStopped chan struct{}` with `anim completion.Signal`, and trim the
comment's channel-discipline sentence:

```go
	// animFrame counts every write to v.img.Image - attemptLoad's initial
	// frame plus each one animate cycles to afterwards - and anim is
	// finished by animate once its load token is cancelled or stale and
	// it returns. Both exist so tests can synchronize on frame changes
	// and animation shutdown via an atomic and a completion.Signal
	// instead of reading v.img.Image directly from another goroutine,
	// which would race with attemptLoad's/animate's writes under the fyne
	// test driver: it runs fyne.Do synchronously on the calling goroutine
	// rather than marshaling onto a single UI thread, so even a read
	// sequenced after the load signal finishes has no happens-before edge
	// against a concurrently running animate call - only observing
	// animFrame's new value does. Each animate call gets its own captured
	// finisher (see attemptLoad), so a superseded request's completion
	// can't be mistaken for a newer one's.
	//
	// anim is a value field, never copied: it holds a mutex.
	animFrame atomic.Uint64
	anim      completion.Signal
```

- [ ] **Step 2: Convert the producer and `animate`**

`internal/ui/load.go` at lines 292-296:

```go
	if len(loaded.Frames) > 1 {
		stopped := v.anim.Begin()
		go v.animate(token, loaded.Frames, loaded.Delays, stopped)
	}
```

`internal/ui/load.go` at line 447 — the signature and its `defer`:

```go
func (v *viewer) animate(token requestToken, frames []image.Image, delays []time.Duration, stopped func()) {
	defer stopped()
```

Everything else in `animate` is unchanged. Update the doc comment's
"stopped is closed right before it returns" to "stopped is called right
before it returns", and its "see the animFrame/animStopped comment" to
"see the animFrame/anim comment".

- [ ] **Step 3: Convert `waitForAnimStopped`**

`harness_test.go` lines 303-313:

```go
// waitForAnimStopped waits for the current animate call to finish v.anim,
// which it does right before returning once it notices its generation is
// stale.
func waitForAnimStopped(t *testing.T, v *viewer) {
	t.Helper()

	waitFor(t, "the animation to stop", &v.anim)
}
```

Add `{"animation", &v.anim}` to `drain`'s Signal loop — it is not in
`drain` today, and adding it is the right call: `AGENTS.md` requires every
background goroutine to be in the drain cleanup, and this one qualifies.
If that makes any test hang, that is a real finding — report it rather
than removing the row.

- [ ] **Step 4: Convert the two nil-assertions**

`animate_test.go:179-181`:

```go
	if !v.anim.Begun() {
		t.Fatal("loading an animated GIF should arm the animation signal")
	}
```

`imgcache_test.go:207-209`:

```go
	if v.anim.Begun() {
		t.Error("the animation signal is armed, want no animation goroutine for a refused animation")
	}
```

`animate_test.go:146-155` is the one site that needs judgement rather than
substitution. It currently reads:

```go
	oldAnimStopped := v.animStopped

	v.ShowImage(1)
	waitUntilLoaded(t, v)

	select {
	case <-oldAnimStopped:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for the superseded animation to stop")
	}
```

`Handle` expresses this directly, preserving the capture's exact meaning
regardless of whether image 1 turns out to re-arm the animation. Replace
the capture and the select with:

```go
	oldAnim := v.anim.Current()

	v.ShowImage(1)
	waitUntilLoaded(t, v)

	waitHandle(t, "the superseded animation to stop", oldAnim)
```

Add the `Handle` twin of `waitFor` to `harness_test.go`, next to it:

```go
// waitHandle is waitFor for a generation captured before a newer request
// superseded it - see completion.Signal.Current.
func waitHandle(t *testing.T, name string, h completion.Handle) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), testTimeout)
	defer cancel()

	if err := h.Wait(ctx); err != nil {
		t.Fatalf("timed out waiting for %s", name)
	}
}
```

Do not "simplify" this to `waitForAnimStopped(t, v)`. Even where the
current generation happens to be the same one, waiting on the Signal makes
the test assert something weaker than it does today, and the next person
to change the fixture would not notice.

- [ ] **Step 5: Verify**

```bash
gofmt -l .
go vet ./...
go build ./...
go test -race -run 'Anim|Animate|GIF|ImgCache' -v ./internal/ui/
go test -race ./...
```

- [ ] **Step 6: Report, do not commit**

```
Move animStopped onto completion.Signal, and into drain

Five of nine. animate now takes a stopped func() rather than a channel it
closes. Adds the animation to drain's cleanup, which AGENTS.md requires
of every background goroutine and which it was missing.
```

---

## Stage 5: `toast.done`

**Files:**
- Modify: `internal/ui/toast.go:44-49` (fields), `:83-104` (`show`)
- Modify: `internal/ui/harness_test.go:335-360` (`settleToast`)

**Interfaces:**
- Consumes: `completion.Signal`, `waitFor`.
- Produces: `toast.hidden` as a `completion.Signal`. `toast.stop` is
  unchanged — it is a cancel signal, not a completion, and keeps its
  `chan struct{}` and its `cancelAutoHide` nil-out.

- [ ] **Step 1: Replace the field**

`internal/ui/toast.go` lines 44-49:

```go
	// stop cancels the pending auto-hide goroutine without hiding
	// anything (closed by the next show call, or by a test's
	// settleToast). It stays a raw channel: it is a cancel signal, not a
	// completion, and cancelAutoHide's nil-out is what makes "is one
	// pending" answerable. hidden is finished when that goroutine exits,
	// whichever way it went - see internal/completion.
	//
	// stop is per-show and only ever swapped on the UI goroutine.
	stop   chan struct{}
	hidden completion.Signal
```

Add the `completion` import. Note `toast` is already a pointer type
(`newToast` returns `*toast`), so the mutex inside `hidden` is safe.

- [ ] **Step 2: Convert `show`**

`internal/ui/toast.go` lines 83-104:

```go
	stop := make(chan struct{})
	t.stop = stop

	done := t.hidden.Begin()

	t.text.Text = msg
	t.text.Refresh()
	t.card.Show()
	t.repaint()

	go func() {
		defer done()

		select {
		case <-time.After(t.duration):
		case <-stop:
			return
		}

		fyne.Do(func() {
			t.autoHide(gen)
		})
	}()
```

- [ ] **Step 3: Convert `settleToast`**

`harness_test.go` lines ~349-360 — the `v.toast.stop == nil` guard stays
(it is the "was a toast actually shown?" check and `stop` is still a raw
channel); only the select changes:

```go
	v.toast.cancelAutoHide()

	waitFor(t, "the toast's auto-hide goroutine", &v.toast.hidden)

	v.toast.autoHide(v.toast.gen.Load())
```

- [ ] **Step 4: Verify**

```bash
gofmt -l .
go vet ./...
go build ./...
go test -race -run 'Toast' -v ./internal/ui/
go test -race ./...
```

Every test that shows a toast calls `settleToast`, so breakage here shows
up broadly rather than in one file — run the full `internal/ui` suite, not
just the `Toast` subset.

- [ ] **Step 5: Report, do not commit**

```
Move the toast auto-hide done channel onto completion.Signal

Six of nine. toast.stop stays a raw channel: it cancels, it does not
report completion, and cancelAutoHide's nil-out is load-bearing.
```

---

## Stage 6: `loadDone` — the retry chain

**Model: fable.** This is the only stage with a real correctness argument
rather than a substitution. Four close sites, a `done` threaded as a
parameter through three functions, and an ordering constraint
(`preloadNeighbors` must run and finish reading `v.state.files` *before*
the signal finishes) that is documented in prose at `load.go:298-306` and
must survive.

**Files:**
- Modify: `internal/ui/viewer.go:205-208` (the `loadDone` field)
- Modify: `internal/ui/load.go:60-64` (`ShowImage`), `:84` (`attemptLoad` signature), `:95-99`, `:139-143`, `:192` (`finishLoad` signature), `:306-309`, `:423-434` (`retryAfterLoadFailure`)
- Modify: `internal/ui/harness_test.go:231-257` (`waitUntilLoaded`), `drain`
- Check: `internal/ui/openfiles_test.go:30` and `internal/ui/openfiles.go:42` (comments naming `v.loadDone`)

**Interfaces:**
- Consumes: `completion.Signal`, `waitFor`.
- Produces: `v.load` (or `v.loadOp`, see step 1) as a `completion.Signal`.
  `attemptLoad`, `finishLoad` and `retryAfterLoadFailure` take
  `done func()` instead of `done chan struct{}`.

- [ ] **Step 1: Pick the field name**

Run `grep -n 'v\.load\b' internal/ui/*.go` and
`grep -n '\bload\b' internal/ui/viewer.go`. If `load` is free as a field
name on `viewer`, use `v.load`. If it collides with an existing field or
method, use `v.loadOp` and use that name consistently for the rest of this
stage. Report which one you chose.

The rest of this stage is written as if `v.load` won.

- [ ] **Step 2: Replace the field**

`internal/ui/viewer.go` lines 205-208:

```go
	// load is begun by ShowImage and finished by whichever step of that
	// call's decode/retry chain ends it - see load.go. The whole chain
	// shares one generation rather than beginning a new one per retry, so
	// a waiter sees the chain as finished only once it truly settles
	// instead of racing whichever retry finishes first.
	// See internal/completion for the contract.
	//
	// A value field, never copied: it holds a mutex.
	load completion.Signal
```

Also update the two other places in `viewer.go` that name `loadDone` in
prose: line 229 (`scanOp.done/loadDone so tests can wait...`) and line 240
(`even a read sequenced after done/loadDone`). Rewrite them to name
`v.load`.

- [ ] **Step 3: Convert `ShowImage`**

`internal/ui/load.go` lines 60-64:

```go
	token := v.loadLifecycle.begin()

	done := v.load.Begin()

	v.attemptLoad(token, i, done)
```

- [ ] **Step 4: Convert the three signatures and four finish sites**

Change the parameter type in all three declarations from
`done chan struct{}` to `done func()`:

- `load.go:84`: `func (v *viewer) attemptLoad(token requestToken, i int, done func()) {`
- `load.go:192`: `func (v *viewer) finishLoad(token requestToken, _ int, u fyne.URI, loaded *imaging.LoadedImage, done func()) {`
- `load.go:423`: `func (v *viewer) retryAfterLoadFailure(token requestToken, msg string, i int, done func()) {`

Then replace each `close(done)` with `done()`. All four:

- `load.go:97` — the cache-hit-but-stale path:
  ```go
	if loaded, ok := v.imgCache.Get(u.String()); ok {
		if !token.current() {
			done()
			return
		}
		v.finishLoad(token, i, u, loaded, done)
		return
	}
  ```
- `load.go:141` — the decoded-but-stale path:
  ```go
		fyne.Do(func() {
			if !token.current() {
				done() // user already navigated elsewhere
				return
			}
  ```
- `load.go:308` — the success path. **The ordering comment above it at
  lines 298-306 must survive**, updated only where it says "done closes" /
  "closing done first". `preloadNeighbors` still has to run before this:
  ```go
	// Must run - and finish reading v.state.files/v.state.index - before the
	// load signal finishes below: that finish is what a waiter (a test's
	// waitUntilLoaded, or a future navigation) synchronizes on to know
	// this call is done touching viewer state. Under the fyne test
	// driver, this whole function already runs on whatever goroutine
	// called fyne.Do rather than a dedicated UI goroutine (see
	// attemptLoad's token comment), so finishing the signal first would
	// let a waiter go on to mutate v.state.files - via reset() or a fresh
	// drop - concurrently with this read.
	v.preloadNeighbors(token)

	done()
  ```
- `load.go:428` — the retry-exhausted path:
  ```go
	if len(v.state.files) == 0 {
		v.ShowEmptyStateError(msg)
		done()
		return
	}
  ```

Also update `retryAfterLoadFailure`'s doc comment (lines 417-422): "and
finalizes done" still reads correctly, but "why token and done are threaded
through unchanged" should now say the chain shares one generation.

- [ ] **Step 5: Convert `waitUntilLoaded` and `drain`**

`harness_test.go` lines 231-257 — the first select becomes `waitFor`; the
preload settle below it is untouched (it waits a `decodepool.Pool`, not a
Signal):

```go
func waitUntilLoaded(t *testing.T, v *viewer) {
	t.Helper()

	waitFor(t, "the image to finish loading", &v.load)

	// Also wait out the neighbor preloads finishLoad kicked off (they're
	// registered with preloads before the load signal finishes): a preload
	// goroutine that outlives its test keeps reading files - and shared
	// library state like the MIME map - under whatever test runs next,
	// which -race rightly reports. "Loaded" here deliberately means
	// "loaded, and everything that load spawned has settled".
	settled := make(chan struct{})
	go func() {
		v.preloads.Wait()
		close(settled)
	}()
	select {
	case <-settled:
	case <-time.After(testTimeout):
		t.Fatal("timed out waiting for neighbor preloads to settle")
	}
}
```

In `drain`, move `{"load", v.loadDone}` out of the channel table and add
`{"load", &v.load}` to the Signal loop.

- [ ] **Step 6: Fix the prose that names `loadDone`**

`grep -rn 'loadDone' internal/` must come back empty. Two known comment
sites to rewrite rather than delete, because they explain a real hazard:
`openfiles.go:42` ("avoiding a data race on `v.scanOp.done/v.loadDone`")
and `openfiles_test.go:30`. Rewrite them to name `v.scanOp.done`/`v.load`,
and add that `completion.Signal` is now internally synchronized, so the
split remains useful for driving the path on the test goroutine but no
longer dodges a race. `harness_test.go:23` and `:318` also name
`loadDone`; update them too.

- [ ] **Step 7: Verify**

```bash
gofmt -l .
go vet ./...
go build ./...
go test -race -run 'Load|Show|Retry|Preload|Navigate' -v ./internal/ui/
go test -race ./...
go test -race -count=5 ./internal/ui/
```

The `-count=5` run matters here specifically: this is the stage where an
ordering mistake shows up as a flake rather than a hard failure.

- [ ] **Step 8: Report, do not commit**

```
Move loadDone onto completion.Signal

Seven of nine, and the subtle one: the decode/retry chain finishes its
generation at four different points and threads the finisher through
attemptLoad/finishLoad/retryAfterLoadFailure. preloadNeighbors still runs
before the signal finishes - that ordering is what keeps a waiter from
mutating v.state.files under finishLoad's read.
```

---

## Stage 7: `scanOp.done` and `sortOp.done` via `asyncOpUI`

**Files:**
- Modify: `internal/ui/asyncop.go:19-46` (the struct and `begin`)
- Modify: `internal/ui/drop.go:91`, `:123`, `:144`, `:152-158`
- Modify: `internal/ui/sort.go:82`, `:93`, `:100-104`
- Modify: `internal/ui/viewer.go:196-203` and `:227-229` (prose)
- Modify: `internal/ui/harness_test.go` (`waitForScan`, `waitForSort`, `drain`)
- Modify: `internal/ui/drop_test.go:527-536`

**Interfaces:**
- Consumes: `completion.Signal`, `waitFor`.
- Produces: `asyncOpUI.done` becomes a `completion.Signal`;
  `asyncOpUI.begin()` returns `(requestToken, func())`.
  `applyScanResult` and `finishSort` take `scanDone func()` /
  `sortDone func()`.

- [ ] **Step 1: Convert the type**

`internal/ui/asyncop.go` — the field at line 27 and `begin` at 37-46:

```go
	done      completion.Signal
```

```go
// begin supersedes any request already in flight, marks the operation
// active, and begins a fresh completion generation. The finisher is
// returned so the caller can capture it: a superseded request must still
// finish its own generation without touching the one a newer request now
// owns - see internal/completion.
func (o *asyncOpUI) begin() (requestToken, func()) {
	token := o.lifecycle.begin()
	o.active = true

	return token, o.done.Begin()
}
```

Update the struct's doc comment (line 19-23): "a per-request done channel
the test suite waits on" becomes "a per-request completion signal the test
suite waits on".

`asyncOpUI` is already documented as "A value field on viewer, never
copied: it holds a lifecycle mutex" — extend that to "a lifecycle mutex and
a completion mutex".

- [ ] **Step 2: Convert `drop.go`**

Line 91 is unchanged in shape (`token, scanDone := v.scanOp.begin()`) —
only `scanDone`'s type changes. Lines 123 and 144 pass it through
unchanged. Line 157's signature and 158's defer:

```go
func (v *viewer) applyScanResult(token requestToken, merging bool, uris, images []fyne.URI, truncated bool, maxScan int, scanDone func()) {
	defer scanDone()
```

Update line 152's comment: "always closes scanDone" → "always finishes
scanDone".

- [ ] **Step 3: Convert `sort.go`**

Line 82 unchanged in shape. Line 103's signature and 104's defer:

```go
func (v *viewer) finishSort(token requestToken, ordered []fyne.URI, sortDone func(), onDone func([]fyne.URI)) {
	defer sortDone()
```

Update line 100's comment: "always closes sortDone" → "always finishes
sortDone".

- [ ] **Step 4: Convert the harness**

`waitForScan` and `waitForSort`:

```go
func waitForScan(t *testing.T, v *viewer) {
	t.Helper()

	waitFor(t, "the scan", &v.scanOp.done)
}

func waitForSort(t *testing.T, v *viewer) {
	t.Helper()

	waitFor(t, "the sort", &v.sortOp.done)
}
```

In `drain`, move the last two rows out of the channel table into the
Signal loop as `{"scan", &v.scanOp.done}` and `{"sort", &v.sortOp.done}`.
The channel table is now empty.

- [ ] **Step 5: Convert `drop_test.go:527-536`**

`TestHandleDrop_SupersededScanGoroutineExits` captures
`scanDoneA := v.scanOp.done` after a folder drop, then calls
`dropAndWait(t, v, jpegB)` — which begins a *second* scan generation — and
waits on the first to prove the superseded scan's goroutine exited. This
capture is load-bearing: waiting on the current generation instead would
wait on B, which `dropAndWait` has already waited out, leaving the test
passing while proving nothing.

`Handle` (Stage 1) exists for exactly this. Use `waitHandle` from
Stage 4:

```go
	v.handleDrop([]fyne.URI{storage.NewFileURI(rootA)})
	scanA := v.scanOp.done.Current()

	jpegB := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, jpegB)

	waitHandle(t, "the superseded scan's goroutine to exit", scanA)
```

The `t.Fatal` message the old select carried ("superseded scan's goroutine
never exited - scanOp.done was never closed") is now `waitHandle`'s
formatted message; the assertion below it on `v.state.files` is unchanged.

Drop the `"time"` import from `drop_test.go` only if nothing else in the
file uses it.

- [ ] **Step 6: Verify**

```bash
gofmt -l .
go vet ./...
go build ./...
go test -race -run 'Drop|Scan|Sort' -v ./internal/ui/
go test -race ./...
go test -race -count=5 ./internal/ui/
```

- [ ] **Step 7: Report, do not commit**

```
Move the scan and sort done channels onto completion.Signal

Nine of nine. asyncOpUI.begin now returns (requestToken, func()); the
finisher threads through applyScanResult and finishSort exactly as the
channel did.
```

---

## Stage 8: cleanup and docs

**Files:**
- Modify: `internal/ui/harness_test.go` (delete the empty channel table, retune the file header)
- Modify: `ARCHITECTURE.md`
- Modify: `todos.md`
- Modify: `needs_refactoring.md`

- [ ] **Step 1: Delete the empty channel loop**

`drain`'s first `for` loop has no rows left. Delete it, along with the
`"time"`-based select it contained, and collapse the doc comment (lines
118-121) which currently explains a nil-skip that no longer exists:

```go
// drain waits out every background operation this viewer may still have in
// flight. Each wait is individually optional - Wait on a completion.Signal
// that never began returns immediately - but the set is exhaustive on
// purpose: it is the backstop that keeps one test's goroutines out of the
// next one, whatever that test happened to exercise.
```

- [ ] **Step 2: Retune the harness file header**

`harness_test.go` lines 21-30 describe "closing loadDone/scanOp.done as
the last thing their completion block does". Rewrite to name
`completion.Signal` and point at `internal/completion` for the contract,
keeping the happens-before explanation intact — that paragraph is the
reason the whole suite is race-free and must not be lost.

- [ ] **Step 3: Confirm the migration is total**

```bash
grep -rn 'Done chan struct{}\|animStopped\|loadDone\|clipboardDone\|chooserDone\|wallpaperDone\|favThumbDone' internal/
```

Expected: no matches. `toast.stop` (a raw `chan struct{}`) and
`internal/ui/*`'s other channels are fine and out of scope — this grep is
scoped to the nine names.

- [ ] **Step 4: Update `ARCHITECTURE.md`**

`AGENTS.md` requires this in the same change as a package addition. Add
`internal/completion` to the package map next to `internal/decodepool`,
described as the one-shot completion signal shared by the viewer's
background operations. Follow the existing entry format exactly — read
`internal/decodepool`'s entry first and mirror its depth. Also update any
`ARCHITECTURE.md` prose that names the old channel fields; find it with
`grep -n 'loadDone\|animStopped\|scanDone\|clipboardDone' ARCHITECTURE.md`.

- [ ] **Step 5: Update `todos.md` and `needs_refactoring.md`**

Move item 5 out of `## TODO` into `## Done` in `todos.md`, written in the
same voice as the existing Done entries (they describe what changed and
why, and what deliberately stayed behind — say that `animFrame` stayed an
atomic counter, `toast.stop` stayed a raw cancel channel, and
`vector.pending`/`preloads`/`Settle` stayed N-goroutine waits). Delete
item 5 from `needs_refactoring.md`.

- [ ] **Step 6: Final verification**

```bash
gofmt -l .
go vet ./...
go build ./...
go test -race ./...
go test -race -count=5 -timeout 20m ./internal/ui/
```

The `-timeout 20m` on the `-count=5` run is not optional: the `internal/ui`
race suite takes ~230s per run, so five runs is ~1140s and `go test`'s
*default* timeout is 600s. Without the flag the run is killed at 600s and
the resulting goroutine dump — mostly `(*toast).show.func1` parked in its
`select` — looks like a hang but is just arithmetic. Every `-count>1`
invocation in this plan needs a timeout set accordingly.

- [ ] **Step 7: Report, do not commit**

```
Finish the completion.Signal migration: drop drain's channel list, update docs

Nine hand-rolled done channels, nine field-comment restatements of the same
discipline and eleven hand-rolled waiters are now one type, three methods
and one test helper. ARCHITECTURE.md gains internal/completion; todos.md
item 5 moves to Done.
```

---

## Self-review

**Spec coverage.** Every one of the nine signals in the baseline table has
a stage: 4+5 → Stage 2, 6+7 → Stage 3, 8 → Stage 4, 9 → Stage 5, 3 →
Stage 6, 1+2 → Stage 7. All four nil-assertions are converted (Stage 3
takes three of them, Stage 4 the fourth). All four inline test waiters are
converted (Stage 2 takes two, Stage 3 takes two). All seven harness
waiters are converted. The four decisions from the design questions —
new package, +`toast.done`, mutex-guarded, `Wait(ctx) error` — are each
realized in Stage 1's code and used unchanged thereafter.

**Type consistency.** `Begin() (done func())`, `Wait(ctx context.Context) error`,
`Begun() bool` are defined in Stage 1 and used with those exact
signatures in Stages 2-7. `waitFor(t, name, *completion.Signal)` is
defined in Stage 2 and used with that exact signature in Stages 3-7.
`asyncOpUI.begin() (requestToken, func())` is defined in Stage 7 and its
two callers are converted in the same stage.

**The one risk found during planning, and how it is closed.** Two tests
capture a done channel and wait on it *after* a newer request has
superseded it: `animate_test.go:146` (`animStopped`) and
`drop_test.go:527` (`scanOp.done`). I checked both rather than assuming.

`drop_test.go:527` is load-bearing — `TestHandleDrop_SupersededScanGoroutineExits`
calls `dropAndWait` between the capture and the wait, which begins a second
scan generation and waits it out. Rewriting that test to wait on the
current generation would have left it passing while proving nothing. That
finding is why `Signal.Current() Handle` is in the design at all; without
it the migration would have quietly weakened a test. Stage 4 adds the
`waitHandle` helper, Stage 7 uses it, and Stage 1 covers the semantics
with `TestHandle_WaitsItsOwnGenerationAfterSupersession`.

`animate_test.go:146` is probably defensive rather than load-bearing (the
second image looks static, so the generation is likely not replaced), but
Stage 4 uses `Handle` there too instead of relying on that reading. It
costs one line and removes the need to be right about the fixture.

**Deliberately not in scope:** `animFrame` (atomic counter, N events),
`toast.stop` (cancel, not completion), `vector.pending` (`WaitGroup`),
`preloads` (`decodepool.Pool`), `grid.Settle` / `slides.Settle`
(N-goroutine waits), and folding `requestLifecycle` into `Signal` — three
of the nine sites have no lifecycle at all, so pairing them would force
one on code that does not want it.
