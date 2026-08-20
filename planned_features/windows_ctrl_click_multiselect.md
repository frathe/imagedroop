# Fix: Ctrl+click multi-select does nothing in grid view on Windows

Status: planned, not started.
TODO entry this closes: "There is a bug in the windows Version: When in Gridview,
multiselect via space key works, but when trying it with mouse and Ctrl key it does not."

---

## 1. Root cause

Not a picfetch logic bug. `internal/ui/grid` asks for the modifier state at tap
time and gets a **stale answer** on Windows.

The chain:

```
grid.Overview.OnSelected            internal/ui/grid/grid.go:278
  -> Host.Modifiers()               internal/ui/grid/grid.go:88
  -> viewer.Modifiers()             internal/ui/viewer.go:752
  -> viewer.keyModifiers()          internal/ui/viewer.go:406
  -> defaultKeyModifiers()          internal/ui/keys.go:22
  -> desktop.Driver.CurrentKeyModifiers()
```

`CurrentKeyModifiers()` is not a live query. It returns `gLDriver.currentKeyModifiers`,
a **latched field written only from the glfw key callback**
(`internal/driver/glfw/window_desktop.go:683`):

```go
w.driver.currentKeyModifiers = desktopModifierCorrected(mods, key, action)
```

and `desktopModifierCorrected` (`window_desktop.go:707`) is:

```go
if action == glfw.Press {
    mods |= glfwKeyToModifier(key)   // X11 workaround: glfw/glfw#1630
} else {
    mods &= ^glfwKeyToModifier(key)  // <-- glfw.Repeat lands HERE
}
```

`glfw.Repeat` is neither `Press` nor `Release`, so it takes the `else` branch and
**strips the modifier's own bit**.

Windows auto-repeats held keys as further `WM_KEYDOWN` messages
(`glfw/src/win32_window.c:706`), and GLFW converts a second press of an
already-pressed key into `GLFW_REPEAT` (`glfw/src/input.c:287`):

```c
if (action == GLFW_PRESS && window->keys[key] == GLFW_PRESS)
    repeated = GLFW_TRUE;
...
if (repeated) action = GLFW_REPEAT;
```

So on Windows, roughly one auto-repeat delay (~500 ms) after Ctrl goes down,
`currentKeyModifiers` **zeroes itself while Ctrl is still physically held**. The
next tap reads `0`, `pickModifier` reports neither gesture, and the default
branch opens the image.

macOS is immune because modifier keys arrive through `flagsChanged:`
(`glfw/src/cocoa_window.m:572`), which only ever emits `GLFW_PRESS`/`GLFW_RELEASE` —
never a repeat. That is exactly why Cmd+click works there and Ctrl+click does not
on Windows. Linux/X11 auto-repeats too and is very likely affected as well.

**This matches the reported symptom exactly:** pressing Ctrl at the same instant as
the click lands *inside* the auto-repeat delay and works; holding Ctrl first does not.

### The fix in one line

Stop asking the stale latch what is held at tap time. `desktop.MouseEvent.Modifier`
is filled directly from the glfw mouse callback's own `mods`
(`internal/driver/glfw/window.go:493`) and is correct on every platform.

### Second consumer, same bug

`internal/ui/zoom/widget.go:63` (Shift+scroll to pan) reads the same accessor.
`fyne.ScrollEvent` carries no modifiers, so it cannot be fixed the same way —
Part B below fixes the accessor itself.

Key-triggered readers are **not** affected (`keys.go:147`, `keys.go:208` — Shift+R
rotate): a key press writes the latch from its own event, so it is correct at the
moment it is read.

---

## 2. Two traps that will break a naive implementation

### Trap 1 — on a mouse event the shortcut modifier is `Control` on *every* platform

`fyne.KeyModifierShortcutDefault` is `KeyModifierSuper` on darwin and
`KeyModifierControl` elsewhere (`key_darwin.go:6`, `key_other.go:8`). But
`convertMouseButton` (`window_desktop.go:465`) **rewrites the modifiers on darwin
before they reach the event**:

```go
if runtime.GOOS == "darwin" {
    if modifier&fyne.KeyModifierControl != 0 { rightClick = true; modifier &^= fyne.KeyModifierControl }
    if modifier&fyne.KeyModifierSuper   != 0 { modifier |= fyne.KeyModifierControl; modifier &^= fyne.KeyModifierSuper }
}
```

So a Cmd+click on macOS arrives as `KeyModifierControl`, and a physical Ctrl+click
becomes a *secondary* tap (which never reaches `Tapped` at all).

Testing `mods & fyne.KeyModifierShortcutDefault` against a `desktop.MouseEvent`
would therefore silently break macOS. The mouse path needs its own predicate that
checks `fyne.KeyModifierControl` unconditionally.

### Trap 2 — a `desktop.Mouseable` cell takes the tap away from `GridWrap`

Fyne hit-tests with `driver.FindObjectAtPositionMatching` (`internal/driver/util.go:32`),
which overwrites `found` on every match as it walks down, so the **deepest**
matching object wins. Its matcher accepts
`fyne.Tappable, fyne.SecondaryTappable, fyne.DoubleTappable, fyne.Focusable, desktop.Mouseable`.

`gridWrapItem` (`widget/gridwrap.go:459`) is the parent and is `fyne.Tappable`.
The moment our cell content implements `desktop.Mouseable`, the cell becomes the
hit-test winner and `gridWrapItem.Tapped` — and therefore `GridWrap.OnSelected` —
**stops firing from the mouse entirely**. There is no way to observe the modifier
without also taking over the tap. So the cell must implement *both*
`desktop.Mouseable` and `fyne.Tappable`, and drive the gesture itself.

Consequences, all of them fine or actively good:

- `gridWrapItem.onTapped` (`gridwrap.go:645`) no longer runs, so GridWrap no longer
  calls `canvas.Focus(...)` on every tap. The `Host.Unfocus()` calls stay as
  belt-and-braces (the keyboard `wrap.Select` path still reaches it).
- GridWrap's own selection highlight no longer flashes on tap. The current code
  already undoes it immediately with `defer g.wrap.UnselectAll()`.
- `l.gw.currentHighlight` is no longer moved by a tap. Already covered: you must
  hover a cell to click it, and hover fires `OnHighlighted` -> `setHighlight` ->
  `wrap.Highlight`, which keeps GridWrap's cursor in step.
- **The cell must NOT implement `desktop.Hoverable`**, or it would shadow
  `gridWrapItem` for hover too and kill `OnHighlighted`. Do not add `Cursorable`
  either.

### Verification limit (important)

Fyne's **test driver does no hit-testing** — `test.Tap` calls `Tapped` directly.
So Trap 2 (the cell actually winning the hit test) **cannot be covered by a unit
test**. It has to be verified by running the real app. Unit tests cover the
gesture logic and the "we no longer consult the stale latch" contract.

---

## 3. What gets built

### Part A — grid taps read modifiers off the event

**New `internal/ui/grid/cell.go`** — unexported widget replacing the bare
`container.NewStack(img, tint, ring)` the GridWrap template returns today
(`grid.go:218-231`):

```go
type cell struct {
    widget.BaseWidget

    img     *canvas.Image
    tint    *canvas.Rectangle
    ring    *canvas.Rectangle
    content *fyne.Container

    // mods is the modifier state the pending tap was pressed under, captured
    // from the event because the driver's latched CurrentKeyModifiers cannot
    // be trusted at tap time - see cell.go's header comment.
    mods fyne.KeyModifier

    onTapped func(c *cell, mods fyne.KeyModifier)
}
```

- `CreateRenderer` -> `widget.NewSimpleRenderer(c.content)` (identical layout and
  `MinSize` to today's stack, so golden screenshots must not move).
- `MouseDown(ev *desktop.MouseEvent)` -> `c.mods = ev.Modifier`
- `MouseUp(ev *desktop.MouseEvent)` -> `c.mods = ev.Modifier`
  **This is required, not redundant**: `processMouseClicked` calls
  `mouseClickedHandleMouseable` (MouseUp) at `window.go:497` and only reaches
  `mouseClickedHandleTapDoubleTap` (Tapped) at `window.go:568`. MouseUp runs
  *before* Tapped, so a MouseUp that cleared `mods` would erase the answer.
  Re-recording is also marginally more accurate (state at release).
- `Tapped(*fyne.PointEvent)` -> read `c.mods`, reset it to 0, call `onTapped`.
- No `MouseIn`/`MouseMoved`/`MouseOut`, no `Cursor()`.

**New `pickMouseModifier` in `internal/ui/grid/selection.go`** — the mouse twin of
`pickModifier`, checking `fyne.KeyModifierControl` (Trap 1) with a comment
explaining why it is not `KeyModifierShortcutDefault`.

**`internal/ui/grid/grid.go`:**
- Extract the three-way switch at `grid.go:278-295` into
  `func (g *Overview) activate(id int, toggle, extend bool)`.
- `wrap.OnSelected` becomes the **keyboard-only** path (Return/Enter via
  `wrap.Select`, `grid.go:490` and `grid.go:520`) and keeps reading
  `g.host.Modifiers()` — correct there, since a key press writes the latch from
  its own event. Keeps `defer g.wrap.UnselectAll()`.
- The template returns `newCell(...)`; the update callback casts `o.(*cell)`.
- `cellIDs` and `inflight` are re-keyed from `*fyne.Container` to `*cell`;
  `requestThumbnail`'s `key` parameter changes type to match.

### Part B — the modifier latch (fixes zoom's Shift+scroll too)

**New `internal/ui/modifiers.go`:**

```go
// modifierLatch tracks which modifier keys are physically held, to repair the
// one thing desktop.Driver.CurrentKeyModifiers gets wrong: a held modifier's
// own auto-repeat clears its bit (see the header comment for the full chain).
type modifierLatch struct {
    mu   sync.Mutex
    held fyne.KeyModifier
}

func (l *modifierLatch) down(ev *fyne.KeyEvent)   // sets the bit for this key
func (l *modifierLatch) up(ev *fyne.KeyEvent)     // clears it
func (l *modifierLatch) clear()                   // drops everything
func (l *modifierLatch) combine(driver fyne.KeyModifier) fyne.KeyModifier
```

Name -> bit, from `fyne.io/fyne/v2/driver/desktop/key.go`:
`KeyShiftLeft`/`KeyShiftRight` -> Shift, `KeyControlLeft`/`KeyControlRight` -> Control,
`KeyAltLeft`/`KeyAltRight` -> Alt, `KeySuperLeft`/`KeySuperRight` -> Super.

**Why `combine` returns `driver | held`, not `held` alone.** The driver value is
never *falsely positive* — a release always clears the bit correctly — it is only
falsely negative, and only because of the repeat bug. The latch is the opposite:
it can get stuck "held" if a KeyUp is missed. OR-ing keeps every correct answer
the driver gives and adds back only the bit the repeat bug dropped.

**Why the latch survives auto-repeat.** `processKeyPressed` (`window.go:683`)
calls `canvas.onKeyDown` only under `case press:`; a repeat falls to `default:`
and skips it. So repeats never touch the latch.

**Stuck-bit mitigations:**
- Every KeyUp clears its own bit.
- `application.Lifecycle().SetOnExitedForeground(...)` -> `clear()`, covering
  Alt+Tab away with a modifier held.
- Known residual: `canvas.onKeyDown`/`onKeyUp` only fire while nothing holds Fyne
  widget focus. This app deliberately keeps canvas focus empty, and Part A removes
  the one thing that routinely grabbed it (GridWrap on tap), so this is narrow.
  If it ever misbehaves in practice, the follow-up is to resync the latch from
  `desktop.MouseEvent.Modifier`, which is authoritative and arrives constantly.

**Wiring:**
- `viewer` gets a `modLatch modifierLatch` field (an owned field, **not** a
  package-level var — AGENTS.md forbids mutable package-level test seams).
- `build.go:83` currently sets `keyModifiers: defaultKeyModifiers` inside the
  struct literal. Replace with an assignment after the literal:
  `view.keyModifiers = view.currentModifiers`, where `currentModifiers()` returns
  `view.modLatch.combine(defaultKeyModifiers())`.
- `window.Canvas().SetOnKeyDown(...)` / `SetOnKeyUp(...)` next to the existing
  `SetOnTypedKey`/`SetOnTypedRune` at `build.go:146-156`. Both hooks are currently
  **unused on this canvas** — verified, nothing to clobber.
- The lifecycle hook goes wherever the `fyne.App` handle already is (`run.go:39`
  and `run.go:72` already use `SetOnStarted`/`SetOnStopped`). `SetOnExitedForeground`
  is unused — do not disturb the other two.
- `stubKeyModifiers` (`rotate_test.go:20`) replaces `v.keyModifiers` wholesale and
  keeps working untouched.

---

## 4. Task breakdown for sub-agents

Each task is one agent, one review checkpoint. I review and fix up after every
step before dispatching the next. Tasks 1-4 are strictly sequential (each edits
what the previous one produced). Task 5 is independent of 1-4 and could run in
parallel, but sequential is simpler to review.

Every task ends with `go build ./... && go test -race ./internal/ui/...` green
unless stated otherwise, and none of them may add `TODO`/`FIXME` comments
(AGENTS.md) or invent new user-visible strings (no `translations/` churn — this
change adds none).

---

### Task 1 — the cell widget (sonnet)

**Files:** new `internal/ui/grid/cell.go`, new `internal/ui/grid/cell_test.go`.
**Must not touch** `grid.go` or `selection.go`. The package must still compile and
every existing test must still pass, untouched.

Build the `cell` widget exactly as specified in Part A, with a header comment
carrying the root cause (Section 1) and the "no Hoverable" rule (Trap 2).

Tests:
- `MouseDown` then `Tapped` forwards the modifier from the event.
- `MouseDown`, `MouseUp`, then `Tapped` still forwards it (the ordering trap).
- `Tapped` with no preceding mouse event forwards `0`.
- Two taps in a row do not leak the first tap's modifier into the second.
- A compile-time assertion block that `*cell` satisfies `fyne.Tappable` and
  `desktop.Mouseable` but **not** `desktop.Hoverable`.
- `newCell(...).MinSize()` equals the current template's
  `container.NewStack(img, tint, ring)` MinSize, so grid layout cannot shift.

---

### Task 2 — `pickMouseModifier` + extract `activate` (sonnet)

**Files:** `internal/ui/grid/selection.go`, `internal/ui/grid/grid.go`,
`internal/ui/grid/selection_test.go`.

Pure refactor plus one new function. No cell wiring yet, no behaviour change:
every existing test must pass **without being modified**.

1. Add `pickMouseModifier` beside `pickModifier`, with the Trap 1 comment.
2. Extract `grid.go:278-295`'s switch into
   `func (g *Overview) activate(id int, toggle, extend bool)`, moving the existing
   comments about `Unfocus` and about resolving `fileIndex` before `Close` with it.
3. `OnSelected` becomes
   `toggle, extend := pickModifier(g.host.Modifiers()); g.activate(id, toggle, extend)`.
4. Add a table test for `pickMouseModifier` covering Control, Shift, both
   (Control wins, same precedence as `pickModifier`), neither, and — the point of
   the whole function — `fyne.KeyModifierSuper` alone reporting **no** gesture.

---

### Task 3 — wire cells into the GridWrap (sonnet; escalate to Opus if it fights back)

**Files:** `internal/ui/grid/grid.go`, `internal/ui/grid/grid_test.go`.

The riskiest step. Precise scope:

1. The `New` template func returns `newCell(g.onCellTapped)` instead of the bare
   stack. Prefer extracting the template into a named unexported constructor so
   Task 1's `MinSize` test can address it directly.
2. The update callback takes `o.(*cell)` and reaches `c.img`, `c.tint`, `c.ring`
   instead of indexing `cell.Objects[0..2]`.
3. `cellIDs` and `inflight` are keyed by `*cell`; `requestThumbnail(key *cell, ...)`
   and its `cellIDs.Load` / `inflight` call sites follow. Update the field doc
   comments at `grid.go:170` and `grid.go:184` and `requestThumbnail`'s comment at
   `grid.go:843` ("the stable per-slot container" is no longer a container).
4. `func (g *Overview) onCellTapped(c *cell, mods fyne.KeyModifier)`: look the id up
   in `cellIDs` (bail if absent — a recycled cell mid-refresh), then
   `toggle, extend := pickMouseModifier(mods); g.activate(id, toggle, extend)`.
5. Leave `OnSelected` in place for the keyboard path. Add a comment on it saying
   it is now keyboard-only and why.

Existing tests keep driving `g.wrap.Select(...)`, which still exercises
`OnSelected` -> `activate`, so they should stay green as-is. **If any existing test
needs changing, stop and report rather than editing it** — that is a behaviour
change I need to see.

---

### Task 4 — regression tests that encode the Windows bug (sonnet)

**Files:** `internal/ui/grid/selection_test.go` (or a new `cellinput_test.go`),
`internal/ui/grid/grid_test.go`.

These are the guards that would have caught the bug, and they run on any platform
because the bug is "we asked the wrong source":

1. **The bug itself.** `host.mods = 0` (the zeroed latch a Windows auto-repeat
   leaves behind) while the tap event carries `fyne.KeyModifierControl`: the cell
   toggles, the grid stays open, `ShowImage` is never called.
2. **The mirror.** `host.mods = fyne.KeyModifierShortcutDefault` but the tap event
   carries `0`: a plain open. Proves the mouse path no longer consults the host at all.
3. Shift+click extends from the event's modifier, same shape.
4. A wiring test: build an `Overview`, pull a `*cell` out of the template, drive
   `MouseDown`+`Tapped`, and assert it reaches `activate`.
5. Add a `clickCell(g, id, mods)` helper next to the existing `click` helper, and
   update `click`'s doc comment — it now documents the *keyboard* path, and the
   "a Fyne tap carries none" line in it and in `fakeHost.mods` is no longer true
   of taps.

State plainly in a comment on test 1 that hit-test shadowing (Trap 2) is **not**
covered here because the Fyne test driver does no hit-testing.

---

### Task 5 — the modifier latch (Opus — global accessor, focus-dependent)

**Files:** new `internal/ui/modifiers.go`, new `internal/ui/modifiers_test.go`,
`internal/ui/viewer.go`, `internal/ui/build.go`, `internal/ui/run.go`.

Build Part B exactly as specified, including the reasoning comments for
`driver | held` and for why repeats never reach `onKeyDown`.

Tests (pure unit tests on `modifierLatch`, no window needed):
- `down(LeftControl)` then `combine(0)` reports Control — the Windows repeat case.
- `down` then `up` reports nothing.
- Left and right variants of each modifier both map to the same bit.
- A non-modifier key (`fyne.KeyA`) changes nothing.
- `combine` never *drops* a bit the driver reports that the latch missed.
- `clear()` empties it.
- Two modifiers held at once accumulate, and releasing one keeps the other.

Then one `internal/ui` test proving `buildViewer` actually wires
`SetOnKeyDown`/`SetOnKeyUp` — drive the canvas hooks and read `v.keyModifiers()`.

**Do not** change `stubKeyModifiers` or any test that uses it.

---

### Task 6 — docs and TODO (sonnet)

**Files:** `ARCHITECTURE.md`, `todos.md`, plus comment touch-ups.

1. `ARCHITECTURE.md` grid row (line 86): the Cmd/Ctrl+click gesture now reads its
   modifier off the tap event via the `cell` widget, which owns the tap outright;
   `Host.Modifiers` is the keyboard path only. Note that `cell` deliberately is not
   `Hoverable` so `OnHighlighted` still comes from `gridWrapItem`.
2. `ARCHITECTURE.md` `keys.go` row (line 56) and the `viewer.go` row (line 55):
   the modifier latch and what it repairs.
3. The "where to look for X" index (around line 477): add an entry for
   "Why doesn't Ctrl+click select on Windows?" pointing at `grid/cell.go` and
   `modifiers.go`.
4. `internal/ui/grid/grid.go:78-88` — `Host.Unfocus` and `Host.Modifiers` doc
   comments; `Modifiers` is keyboard-only now.
5. `internal/ui/keys.go:12-28` — `defaultKeyModifiers`' comment currently asserts
   the driver value "is kept in sync by the glfw driver on every key event". That
   is precisely the false claim this bug disproves. Rewrite it.
6. `todos.md` — move the entry to Done with a short description in the style of the
   favorites-thumbnail-cache entry. Add a new TODO **only** if Task 5 was skipped.

---

### Task 7 — verification (me, then you)

1. `make fmt`, `go vet ./...`, `go build ./...`, `go test -race ./...` from the repo root.
2. Golden screenshots: layout must be unchanged (Task 1's MinSize test is the
   early warning). If `internal/ui/testdata/failed/*.png` appears, that is a real
   regression, not a golden to regenerate.
3. `make run` on macOS — the platform that works today, so this is the regression
   check: Cmd+click toggles, Cmd+click again deselects, Shift+click extends, a
   plain click still opens, hover still moves the ring, arrow keys still work after
   a Cmd+click, and Ctrl+click still behaves as a right-click (macOS convention).
4. **You build for Windows and confirm**: hold Ctrl for a second or two, then click
   — the cell selects instead of opening. Also worth a look while you are there:
   Shift+scroll to pan a zoomed image, which Task 5 should have fixed on the same
   root cause.
5. Suggested commit message at the end. No `git commit` from me (AGENTS.md).

---

## 5. Open risks

| Risk | Handling |
|---|---|
| The cell does not actually win the hit test | Only provable by running the app. Task 7 step 3 on macOS catches it immediately — Cmd+click would stop working entirely. |
| Windows auto-repeat of modifier keys is not the mechanism | The fix does not depend on it. Reading modifiers off the mouse event is correct regardless of *which* quirk staled the latch, and the timing symptom is only explicable by something time-based in this path. |
| Latch sticks "held" after a missed KeyUp | Cleared on KeyUp and on `SetOnExitedForeground`. OR-ing with the driver means the latch can only ever add a bit, never remove a correct one. Follow-up if seen: resync from `desktop.MouseEvent.Modifier`. |
| Golden screenshots shift | `widget.NewSimpleRenderer` passes `MinSize` straight through; Task 1 asserts it explicitly. |
| Linux | Same root cause, same fix, no extra work. Untested unless you have a Linux box. |
