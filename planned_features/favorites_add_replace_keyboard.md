# Planned feature: keyboard-driven Add-to-Favorites and Replace-favorite prompts

The last two Favorites dialogs that the keyboard cannot answer get the
treatment the removal confirmation already has:

1. **Add Current List to Favorites…** becomes a two-stop keyboard story — the
   name field holds the keyboard on open, `↓` moves to a ringed
   `Cancel`/`Add` pair, `↑` comes back, `Return` in the field saves, `Esc`
   cancels from either stop.
2. **Replace Favorite** stops being a `dialog.NewConfirm` and becomes the same
   focusable `widgets.ChoicePanel` prompt the removal confirmation is, with
   `Cancel` reopening the Add dialog rather than throwing the typed name away.

## Why this is worse than `todos.md` records

`todos.md` says neither prompt is dangerous any more and only `Return` is
missing. Reading the code, the Add dialog is in a worse state than that:

- `dialog.NewForm` focuses nothing inside itself (`dialog/form.go` in Fyne
  v2.8.0 never calls `Canvas.Focus`), so `Canvas.Focused()` is nil while it
  is up.
- `keys.go`'s overlay guard now drops every key while any overlay is up.

Together those mean **the Add dialog is keyboard-dead until the user clicks
into the field**. Typing does nothing, `Return` does nothing (`FormDialog`
never wires `Entry.OnSubmitted`), and `Escape` does nothing
(`widget.Entry.TypedKey` has no `KeyEscape` case). Auto-focusing the field is
therefore part of this work, not a nicety.

## Decisions already made by the user

- **The Add dialog is rebuilt**, not patched: `dialog.NewCustomWithoutButtons`
  whose content is the name entry plus a `widgets.ChoicePanel` offering
  `Cancel`/`Add` — the exact shape `removeFavorite` already uses. The name
  field **auto-focuses** when the dialog opens.
- **Two stops move on `↓`/`↑`**, never `Tab`. `←`/`→` keep meaning
  text-cursor movement inside the field and ring movement on the buttons.
  `Return` in the field submits directly, so the common path never visits the
  second stop.
- **Invalid names keep `Add` disabled**, as Fyne's `FormDialog` does today —
  neither a click nor `Return` can submit a name containing `/ \ : * ? " < > |`
  or an empty one.
- **Cancelling the Replace prompt reopens the Add dialog** with the typed name
  still in the field, so a name clash costs a keystroke rather than the name.

### Why `Tab` is not the mechanism

Fyne's glfw driver intercepts `Tab` before the focused widget sees it
(`internal/driver/glfw/window.go`'s `capturesTab`) and hands it to
`FocusManager.FocusNext`, which walks *into* a focusable widget's children
(`internal/app/focus_manager.go`'s `nextWithWalker` returns `false` after
recording a focusable, so traversal continues downward). A `Tab` chain would
therefore run entry → `ChoicePanel` → its `Cancel` button → its `Add` button,
landing focus on a bare `widget.Button` that answers `Space` and never
`Return`. The arrow keys are the app's own model already (`managePanel`'s
two-axis ring) and are not intercepted by anything.

## Decisions this plan makes, open to override at review

- **`↓` hands the keyboard to the panel without changing its selection**, so
  it lands on `Cancel` (index 0), the panel's default. Rationale: `Return` in
  the field is already the fast path to `Add`, and index 0 stays the default
  selection everywhere in this app. A user who wants `Add` from the panel
  presses `↓` `→` `Return`.
- **`Replace` keeps plain importance**, not `widget.DangerImportance` — that
  matches how the prompt looks today. `Cancel` is still index 0 and so the
  default selection, which is what keeps `Return` from replacing by itself.
- **The Add dialog's two choices reuse `cancelChoice`/`confirmChoice`** rather
  than inventing `addChoice`: index 0 cancels, index 1 confirms, in every
  prompt this package raises.

## Ground rules for every stage

- Read `AGENTS.md` and the relevant part of `ARCHITECTURE.md` first.
- TDD: write the failing test first, watch it fail for the right reason, then
  make it pass.
- Every user-visible string is `lang.L("English text")` and needs the exact key
  in **both** `translations/en.json` (identity mapping) and
  `translations/de.json`; `main_test.go` enforces key-set parity. This feature
  is expected to need **no new keys** — `Add to Favorites`, `Add`, `Cancel`,
  `Name`, `Replace Favorite` and `Replace` are all already in both bundles.
  If a stage does introduce one, add it to both bundles in that same stage.
- Do not add `TODO`/`FIXME` comments; open work belongs in `todos.md`.
- Do not run `git commit`. End with a suggested commit message.
- Match this codebase's comment style: comments explain *why* a thing is the
  way it is (see `choicepanel.go`, `manage.go`), not what the line does.
- Verify with `gofmt -l .`, `go vet ./...`, `go build ./...`, then
  `go test -race ./...` from the repository root.
- No golden screenshots cover any Favorites dialog (`internal/ui/testdata/`
  holds only the drop zone, the successful drop and the delete card), so
  `make golden` is **not** part of this work.

## Process

Six sequential stages, one sub-agent per stage, **sonnet** for all six. The
work is reviewed and fixed up after each stage before the next one starts —
no stage is dispatched until its predecessor is reviewed. Stage 4 is the one
with real Fyne focus mechanics in it; if a sonnet agent stalls there, re-run
that stage on opus rather than letting it guess.

Stage order is load-bearing: Stage 5 (Replace) calls into Stage 4's
`showAdd`, so the Add dialog has to exist first, and Stage 4's choices are
indexed by constants Stage 2 creates.

---

## Stage 1 — `widgets.ChoicePanel`: `SetOnBack` and disabled choices

Goal: the shared panel gains the two capabilities the Add dialog needs, with
no behavior change for any existing caller.

In `internal/ui/widgets/choicepanel.go`:

```go
// SetOnBack registers what the Up key runs: the panel is one stop in a
// larger keyboard story and Up is how the user leaves it upwards
// (internal/ui/favorites' Add dialog, whose name field is the stop above).
// Optional - nil leaves Up ignored, which is what every panel inside a
// ChoiceCard wants, since the app dispatcher feeds those keys and Up means
// something else out there.
func (p *ChoicePanel) SetOnBack(onBack func())

// SetChoiceEnabled enables or disables choice i. A disabled choice runs
// nothing and dismisses nothing, whether it is clicked or confirmed from the
// keyboard, and renders greyed - the same deal a disabled Fyne button in a
// dialog.FormDialog offers, which is what the Add dialog replaces.
// Out-of-range indices are a no-op.
func (p *ChoicePanel) SetChoiceEnabled(i int, enabled bool)

// ChoiceEnabled reports whether choice i can be run. False for an
// out-of-range index.
func (p *ChoicePanel) ChoiceEnabled(i int) bool
```

Implementation notes:

- Add an `onBack func()` field beside `onCancel`, and a
  `case fyne.KeyUp:` arm in `TypedKey` that runs it only when non-nil. Every
  other key arm stays exactly as it is.
- `SetChoiceEnabled`/`ChoiceEnabled` go straight through
  `p.buttons[i].Enable()`/`Disable()`/`Disabled()`. Do **not** add a parallel
  `[]bool` — one source of truth means the greyed rendering and the guard can
  never disagree.
- Guard `runChoice`: return early **before** `onDismiss` when the button is
  disabled. A disabled choice must not take the prompt down either. `Confirm`
  goes through `runChoice`, so this covers `Return` as well; `test.Tap` is
  already covered because `widget.Button.Tapped` checks `Disabled()` itself,
  but the guard has to hold for the keyboard path regardless.
- `Select` deliberately still **moves onto** a disabled choice rather than
  skipping it — a greyed button under the ring is the app telling the user why
  `Return` did nothing, and `FormDialog`'s disabled Submit was focusable too.
  Say that in a comment; it is the kind of thing a later reader would
  "fix" otherwise.

Tests in `internal/ui/widgets/choicepanel_test.go`:

1. `Up` with no `onBack` set is inert — selection unchanged, no panic.
2. `Up` runs `onBack` once when set, and does not change the selection.
3. `SetChoiceEnabled(i, false)` greys the button and `ChoiceEnabled(i)`
   reports `false`; re-enabling restores both.
4. `Confirm()` on a disabled selected choice runs neither `OnChosen` nor
   `onDismiss`.
5. `test.Tap` on a disabled choice runs neither.
6. Out-of-range indices: `SetChoiceEnabled` is a no-op, `ChoiceEnabled`
   reports `false`.
7. A regression guard that the existing `ChoiceCard`/deletion behavior is
   untouched: a panel built the old way still confirms and dismisses.

---

## Stage 2 — `showConfirm`: one shared two-choice confirmation

Goal: the "message + focusable `Cancel`/action panel + buttonless dialog"
shape lives in one place, so the Replace prompt is a caller rather than a
copy. Pure refactor — no behavior change, and the existing `manage_test.go`
suite is the safety net.

New file `internal/ui/favorites/confirm.go`:

```go
// cancelChoice and confirmChoice are the two button indices every
// confirmation this package raises uses: Cancel first/left and so the
// default selection, the confirming action second/right. A prompt never
// opens, or reopens, with the action already under Return.
const (
	cancelChoice  = 0
	confirmChoice = 1
)

// confirmation describes one keyboard-driven two-choice prompt.
type confirmation struct {
	title      string
	message    string
	action     string            // the confirming button's label
	importance widget.Importance // widget.DangerImportance for a destructive action
	onConfirm  func()
	onCancel   func() // the Cancel choice and Escape both; nil for "just close"
	onClosed   func() // whichever way the dialog goes; nil for nothing
}

// showConfirm raises c and hands it the keyboard, returning the dialog for a
// caller that needs to hold it.
func (f *Feature) showConfirm(c confirmation) dialog.Dialog
```

- The body is today's `removeFavorite` body with the four strings and the two
  callbacks lifted out: the unwrapped centered `widget.Label`, a
  `widgets.NewChoicePanel(nil, …)`, `SetOnDismiss`/`SetOnCancel`,
  `dialog.NewCustomWithoutButtons`, `SetOnClosed`, `Show()`, then
  `f.win.Canvas().Focus(panel)` **after** `Show`.
- Move the long "why not `dialog.NewConfirm`" comment from `removeFavorite`
  to `showConfirm` — it is now the shared rule — and leave a one-line pointer
  at `removeFavorite`. Keep the "unwrapped label" comment with the label; it
  explains a `dialog.NewCustom*` limitation and belongs here too.
- **Document the callback ordering** in `showConfirm`'s comment, because
  Stage 5 depends on it: `ChoicePanel` dismisses *before* running a choice
  (`runChoice`) and *before* `onCancel` (`TypedKey`'s Escape arm), so
  `onClosed` always fires **before** `onConfirm`/`onCancel`. A callback that
  raises something of its own therefore cannot be undone by the outgoing
  dialog's `onClosed`.

In `internal/ui/favorites/manage.go`:

- Delete the `cancelChoice`/`removeChoice` block (now in `confirm.go`). Keep
  `openCol`/`removeCol`/`columnCount` — those are the panel's own axes.
- Rewrite `removeFavorite` as a `showConfirm` call: title
  `lang.L("Remove Favorite")`, the existing message, action
  `lang.L("Remove")`, `widget.DangerImportance`, `onConfirm`
  `func() { f.performRemove(name) }`, `onClosed` `f.focusManage`.

Mechanical rename across `manage.go` and `manage_test.go`: `removeChoice` →
`confirmChoice` (roughly six sites).

Tests:

- Every existing `manage_test.go` test must pass unchanged apart from that
  rename. Do not weaken any of them.
- Add one new test for the API surface `removeFavorite` does not exercise:
  `onCancel` runs on the Cancel choice **and** on `Escape`, and in both cases
  after the dialog is already down.

---

## Stage 3 — `nameEntry`: the field as a keyboard stop

Goal: a name field that can be left. Widget only — no dialog, no favorites
logic.

New file `internal/ui/favorites/add.go`:

```go
// nameEntry is the Add to Favorites dialog's name field, and the first of
// its two keyboard stops. A plain widget.Entry cannot be one: it has no
// KeyEscape case at all, so Escape aimed at the dialog dies in the field,
// and its Down key moves a cursor row a single-line entry does not have -
// leaving no key that means "I am done typing, take me to the buttons".
type nameEntry struct {
	widget.Entry

	onEscape func()
	onDown   func()
}

func newNameEntry(onEscape, onDown func()) *nameEntry
func (e *nameEntry) TypedKey(ev *fyne.KeyEvent)
```

Rules:

- `fyne.KeyEscape` runs `onEscape` when set and is consumed either way — it
  must never reach `Entry.TypedKey`.
- `fyne.KeyDown` runs `onDown` when set and is consumed either way.
- Everything else delegates to `e.Entry.TypedKey(ev)`, so `←`/`→`, `Home`,
  `End`, `Backspace`, `Delete` and selection all keep working.
- `Return`/`Enter` is deliberately **not** intercepted: a single-line
  `widget.Entry` already calls `OnSubmitted` for it (`typedKeyReturn` in
  Fyne's `widget/entry.go`), which is where Stage 4 hangs submission.
- The constructor must call `e.ExtendBaseWidget(e)`. Without it Fyne focuses
  and renders the embedded `widget.Entry` and this override is never called —
  worth a comment, it is a silent failure.

Tests in `internal/ui/favorites/add_test.go`:

1. `Escape` runs `onEscape` and leaves the text untouched.
2. `Down` runs `onDown`.
3. Both are inert with nil hooks — no panic, text untouched.
4. Other keys still reach the embedded entry: text `"abc"`, cursor at the
   end, `Backspace` leaves `"ab"`.
5. Typed runes still land in the text.
6. `OnSubmitted` fires on `Return` with the current text.

Fixture: `test.NewApp()`, a window holding the entry, focused, keys sent
through the canvas's focused object. **Reuse** `manage_test.go`'s existing
`typeKey` helper — `add_test.go` is in the same `favorites` package, so
declaring a second `typeKey` is a redeclaration compile error, not a
harmless copy. The same goes for `newFeature` and `fakeHost` in
`favorites_test.go`, which Stage 4 needs.

---

## Stage 4 — the Add to Favorites dialog

Goal: the rebuilt dialog, both stops wired, live validation, auto-focus, and
the single-dialog guard.

Still in `internal/ui/favorites/add.go`:

```go
// addPanel is the Add to Favorites dialog's content and both of its keyboard
// stops: the name field, which holds the keyboard on open, and the
// widgets.ChoicePanel below it, which Down hands the keyboard to and Up
// hands it back from.
type addPanel struct {
	entry   *nameEntry
	choices *widgets.ChoicePanel
	content fyne.CanvasObject
}
```

`Feature` gains two fields beside `manageDialog`/`managePanel`, and for the
same reason — a non-nil `addDialog` doubles as the guard against stacking a
second one:

```go
addDialog dialog.Dialog
addPanel  *addPanel
```

```go
// showAdd raises the Add to Favorites dialog with initial already in its
// name field. A no-op while one is already up: the menu bar stays live while
// a Fyne dialog is up (they are canvas overlays, not OS-modal), so the menu
// item can be chosen twice - the guard ShowManage makes for the same reason.
func (f *Feature) showAdd(initial string)

func (f *Feature) newAddDialog(initial string) (dialog.Dialog, *addPanel)
```

Wiring, in `newAddDialog`:

- Entry: `newNameEntry(...)`, `SetText(initial)`, and today's `Validator`
  unchanged (the regexp plus the `favstore.ValidName` check on the trimmed
  name) — it still renders Fyne's own inline validation feedback, which is
  what tells the user *why* `Add` is greyed.
- Choices: `widgets.NewChoicePanel(nil, Cancel, Add)` — index 0
  `lang.L("Cancel")` plain, index 1 `lang.L("Add")` plain. `repaint` is nil,
  as in `showConfirm`: Fyne draws a dialog's content itself.
- Live validation — **not** `SetOnValidationChanged`, which cannot do this
  job. `widget.Entry.setValidationError` (`widget/entry_validation.go:81-83`)
  returns early, without calling `onValidationChanged`, for any transition
  *into* an error state while the entry is focused:
  ```go
  gone := err == nil
  if !gone && (e.focused || (!e.hasFocused && e.Text == "")) {
      return false
  }
  ```
  This field holds focus for its whole useful life (`Down` moves the
  keyboard, it does not blur), so wiring it that way leaves `Add` stuck
  enabled the moment a user edits a valid name into an invalid one — and
  `Return` then sails past `OnSubmitted`'s guard, moving the ring onto `Add`
  and closing the dialog into `saveFavorite`'s toast-only rejection. Use
  `entry.OnChanged` and read `entry.Validate()`'s **return value**, which is
  computed unconditionally and so is not subject to that suppression:
  ```go
  entry.OnChanged = func(string) {
      choices.SetChoiceEnabled(confirmChoice, entry.Validate() == nil)
  }
  ```
  Then seed once with the same expression: `OnChanged` fires only on a text
  *change*, so it never runs for the dialog's own construction — neither for
  a fresh empty field (`Add` must start disabled) nor for `SetText(initial)`,
  which runs before `Validator` exists. Fyne's own inline error text still
  goes through `SetValidationError` and keeps its normal not-mid-word
  suppression.
- `Return` in the field:
  ```go
  entry.OnSubmitted = func(string) {
      if !choices.ChoiceEnabled(confirmChoice) {
          return
      }
      choices.Select(confirmChoice)
      choices.Confirm()
  }
  ```
  Going through the panel's own path rather than calling `saveFavorite`
  directly keeps one dismiss-then-run ordering for both ways of saying Add.
  The `ChoiceEnabled` check comes first so an invalid `Return` changes
  nothing at all, not even the ring.
- `Add`'s `OnChosen`: `func() { f.saveFavorite(entry.Text) }` —
  `saveFavorite` already trims and re-validates.
- `entry.onEscape`: hide the dialog. `entry.onDown`:
  `f.win.Canvas().Focus(choices)`. `choices.SetOnBack(func() { f.win.Canvas().Focus(entry) })`.
  `choices.SetOnDismiss` hides the dialog. No `SetOnCancel` — index 0 already
  does nothing beyond dismissing.
- Content: `container.NewVBox(widget.NewLabel(lang.L("Name")), entry, choices)`,
  stacked behind a transparent `canvas.Rectangle` whose `SetMinSize` is
  `fyne.NewSize(addDialogWidth, 0)` with `addDialogWidth = 360` as a named
  constant. Reason for the constant, in a comment: an entry's own minimum
  width is a few characters, so a buttonless custom dialog sized to its
  content would open as a sliver — `managePanel` sets a floor on its scroll
  for the same reason.
- `dialog.NewCustomWithoutButtons(lang.L("Add to Favorites"), content, f.win)`.
- `SetOnClosed`: the superseded-dialog guard `ShowManage` uses
  (`if f.addDialog != d { return }`), then clear both fields and
  `f.win.Canvas().Unfocus()` — a focus left behind swallows every key
  binding afterwards, since this app dispatches from the canvas's *unfocused*
  handler.
- `Show()`, then `f.win.Canvas().Focus(entry)` — the auto-focus, after
  `Show` because Fyne can only focus an object already part of an overlay it
  can walk to.

`addToFavorites` collapses to `f.showAdd("")`.

Tests in `add_test.go` (the two existing `favorites_test.go` add tests move
here and are rewritten onto the new API — the `newFeature` fixture is
reusable as it stands):

1. Opening the dialog focuses the field: `win.Canvas().Focused()` is the
   `*nameEntry`.
2. `↓` moves the keyboard to the `*widgets.ChoicePanel`; `↑` moves it back
   to the field.
3. `Add` starts disabled on an empty field, goes live on a valid name, and
   greys again on `a/b`.
4. `Return` in the field with a valid name saves the favorite and closes the
   dialog.
5. `Return` in the field with an invalid name saves nothing, leaves the
   dialog up, and leaves the ring on `Cancel`.
6. `↓` `→` `Return` saves; `↓` `Return` (on `Cancel`) closes without saving.
7. `Escape` closes without saving from **both** stops.
8. The name is trimmed: `"  Trip  "` saves as `Trip`.
9. A second `showAdd` while one is up does not stack a second overlay.
10. Closing releases the keyboard — `Canvas().Focused()` is nil afterwards.
11. `showAdd("Holiday")` opens with the field already holding `Holiday`
    (Stage 5 depends on this).

---

## Stage 5 — the Replace-favorite confirmation

Goal: the last `dialog.NewConfirm` in the package goes, and a name clash
costs a keystroke instead of the typed name.

In `internal/ui/favorites/favorites.go`, `saveFavorite`'s `Exists` branch
becomes a `showConfirm` call:

- title `lang.L("Replace Favorite")`, the existing message, action
  `lang.L("Replace")`, plain importance (see the decisions above).
- `onConfirm`: `func() { f.writeFavorite(name) }`.
- `onCancel`: `func() { f.showAdd(name) }` — both the `Cancel` choice and
  `Escape`, since `showConfirm` runs it for both.
- `onClosed`: `func() { f.win.Canvas().Unfocus() }`.

The reopen leans on the ordering Stage 2 documented: the panel hides the
confirmation (firing `onClosed`, which unfocuses) **before** running
`onCancel`, so `showAdd` puts the keyboard on the reopened field last and
keeps it. Pin that with a test rather than trusting it.

`dialog` and `validation` imports in `favorites.go` shrink to whatever is
still used; `widget` may drop out entirely once `newAddDialog` has moved to
`add.go`.

Tests (extend `favorites_test.go`, or a new `confirm_test.go` if the
confirmation tests read better together):

1. Saving a name that already exists raises the confirmation; the canvas
   reports a `*widgets.ChoicePanel` as focused, and the ring is on `Cancel`.
2. `→` `Return` replaces: the stored list is the new one and
   `SyncFavoritePreviews` was reported for it.
3. `Return` on `Cancel` closes the confirmation and reopens the Add dialog
   with the name still in the field, with the field focused.
4. `Escape` behaves exactly as `Cancel` — same reopen, same field contents.
5. Neither cancel path writes anything.
6. Exactly one overlay is up after the reopen: the confirmation is gone, not
   stacked under the Add dialog.

---

## Stage 6 — documentation and full verification

- `ARCHITECTURE.md`, `favorites/` row: the add, replace and remove prompts are
  now one shape — `confirm.go`'s `showConfirm` over a focusable
  `widgets.ChoicePanel` — and the Add dialog is the package's third focused
  widget, with its two stops and their keys. Name the new files (`add.go`,
  `confirm.go`) and say what each holds. The row currently claims
  `managePanel` and `ChoicePanel` are "the two `fyne.Focusable` widgets this
  app ever focuses" — that count is now three; fix it here and anywhere else
  it appears (`manage.go`'s type comment says it too).
- `ARCHITECTURE.md`, `widgets/` row: `ChoicePanel` gains `SetOnBack` and
  `SetChoiceEnabled`/`ChoiceEnabled`, and what each is for.
- `internal/ui/help/manual.md` **and** `manual_de.md`: the Favorites menu
  section (around the `Add Current List to Favorites…` and
  `Manage Favorites…` bullets) gains the Add dialog's keys and the Replace
  prompt's, in the same voice the removal confirmation's paragraph already
  uses. Keep the two manuals structurally parallel — `manual_test.go` checks
  embedding and the no-tables rule, but parity of content is on us.
- `README.md`, the Favorites bullet around line 50: one clause that the add
  and replace prompts are keyboard-driven too.
- `internal/ui/keys.go`'s `handleKeyEvent` comment names "favorites'
  Manage/Add/Replace/removal dialogs" as dialogs that may focus nothing —
  that is no longer true of any of them. Reword so it describes the guard's
  remaining reasons (the file picker, native menus) without losing the
  history of why the guard exists.
- `todos.md`: move the item out of `TODO` into `Done`, written the way the
  entries above it are — what changed, and where it lives.
- `translations/en.json` / `de.json`: only if a stage introduced a new
  `lang.L` key. Expected: none. `go test ./...` at the root proves it either
  way through `main_test.go`.
- Full verification from the repository root: `gofmt -l .`, `go vet ./...`,
  `go build ./...`, `go test -race ./...`.
- End with a suggested commit message; do not commit.

---

## Files this touches

| File | Stage | What |
| --- | --- | --- |
| `internal/ui/widgets/choicepanel.go` | 1 | `SetOnBack`, `SetChoiceEnabled`, `ChoiceEnabled`, disabled guard in `runChoice` |
| `internal/ui/widgets/choicepanel_test.go` | 1 | tests for both |
| `internal/ui/favorites/confirm.go` | 2 (new) | `cancelChoice`/`confirmChoice`, `confirmation`, `showConfirm` |
| `internal/ui/favorites/manage.go` | 2 | `removeFavorite` over `showConfirm`; constants move out |
| `internal/ui/favorites/manage_test.go` | 2 | `removeChoice` → `confirmChoice`; one new `onCancel` test |
| `internal/ui/favorites/add.go` | 3, 4 (new) | `nameEntry`, `addPanel`, `newAddDialog`, `showAdd`, `addDialogWidth` |
| `internal/ui/favorites/add_test.go` | 3, 4 (new) | entry key rules, then the whole dialog |
| `internal/ui/favorites/favorites.go` | 4, 5 | `addToFavorites` → `showAdd("")`; `saveFavorite` over `showConfirm`; `addDialog`/`addPanel` fields |
| `internal/ui/favorites/favorites_test.go` | 4, 5 | the two add tests move out; replace-prompt tests land |
| `internal/ui/keys.go` | 6 | comment only |
| `ARCHITECTURE.md`, manuals, `README.md`, `todos.md` | 6 | documentation |

Untouched, and verified so: nothing in `internal/ui` drives the Add or
Replace dialogs (`menu_test.go` and `keys_test.go` only reach Manage
Favorites), and no golden screenshot contains any Favorites dialog.
