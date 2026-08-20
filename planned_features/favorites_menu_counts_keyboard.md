# Planned feature: favorite item counts + keyboard-driven Manage Favorites

Two changes that make the Favorites menu carry more information and make its
management dialog fully usable without a mouse:

1. Every favorite shows **how many files it stores**, in the Favorites menu
   and in the Manage Favorites rows: `Holiday 2024 (128)`.
2. **Manage Favorites…** becomes keyboard-driven — a focus ring moves over the
   rows and over each row's `Open`/`Remove` buttons, `Return` activates the
   ringed button, `Escape` closes — and gains `Cmd`/`Ctrl+Shift+F`.

Decisions already made by the user:

- The count is the number of **stored entries** read from the favorite's
  `file-list.json`. Not an existence-checked count: no `os.Stat` per file, no
  background pass, no "N missing" marker. The number always matches what
  opening the favorite will try to load.
- Manage Favorites stays a **Fyne dialog** (`dialog.NewCustom`). It is not
  reshaped into a `widgets.ChoiceCard`-style window overlay. Keyboard support
  comes from making the dialog's *content* a focusable widget.
- Navigation is **two-dimensional**: `Up`/`Down` move between rows,
  `Left`/`Right` move between that row's `Open` and `Remove` buttons, `Return`
  activates whichever is ringed. This is the `ChoiceCard` model extended to a
  second axis, so the two prompts feel like one app.
- The dialog is reachable via `Cmd`/`Ctrl+Shift+F`, displayed on the menu item.
- Work runs as five sequential stages, one agent per stage.

## Ground rules for every stage

- Read `AGENTS.md` and the relevant part of `ARCHITECTURE.md` first.
- TDD: write the failing test first, watch it fail for the right reason, then
  make it pass.
- Every user-visible string is `lang.L("English text")` and needs the exact key
  in **both** `translations/en.json` (identity mapping) and `translations/de.json`;
  `main_test.go` enforces key-set parity.
- Do not add `TODO`/`FIXME` comments; open work belongs in `todos.md`.
- Do not run `git commit`. End with a suggested commit message.
- Match this codebase's comment style: comments explain *why* a thing is the
  way it is (see `deletion.go`, `choicecard.go`), not what the line does.
- Verify with `gofmt -l .`, `go vet ./...`, `go build ./...`, then
  `go test -race ./...` from the repository root.

## Stage 1 — `favstore.Count`

Goal: the store can answer "how many files does this favorite hold?" without
the caller building `fyne.URI` values it will throw away.

In `internal/favstore/favstore.go`:

```go
// Count returns how many files the favorite named name stores.
func Count(dir, name string) (int, error)
```

- Extract the "read `file-list.json` and decode it into `map[string]string`"
  half of `Load` into an unexported helper, and build both `Load` and `Count`
  on it, so the two can never disagree about what a stored list is.
- `Count` rejects an invalid name exactly as `Load` does, and propagates a
  missing directory / unreadable file / malformed JSON as an error rather than
  reporting `0`: zero is a real, distinguishable answer (a favorite saved from
  an empty list is refused by `writeFavorite`, but a hand-edited `{}` is not).
- `Load`'s behaviour and error text stay exactly as they are.

Tests in `internal/favstore/favstore_test.go`: a saved list of N files counts
N; a name that was never saved is an error; a favorite whose `file-list.json`
is malformed is an error; an invalid name is an error; `Load` still round-trips
after the refactor.

## Stage 2 — counts in the Favorites menu

Goal: `refreshMenu` labels each favorite with its stored count.

In `internal/ui/favorites/favorites.go`:

- Label is `fmt.Sprintf(lang.L("%s (%d)"), name, count)`. A punctuation-only
  key has precedent in this bundle (`"%d of %d"`, `"Zoom: %d%%"`).
- A favorite whose count cannot be read falls back to the **bare name**. It
  must still be listed, still be openable, and still hold its accelerator
  slot — an unreadable list is a reason to show less, never to hide a
  favorite the user saved. Do not `reportError` per favorite here: a menu
  refresh that raised a toast per unreadable entry would bury the user in
  toasts on every refresh.
- `f.names` keeps holding **names**, never labels: `Open(index)` and the
  `Cmd`/`Ctrl+1`–`0` slots resolve through it.

Tests in `internal/ui/favorites/favorites_test.go`: menu labels carry counts
for saved favorites; a favorite with a corrupt `file-list.json` shows its bare
name and still opens; the digit slots still map to the right favorite after
labels change.

New translation keys: `"%s (%d)"`.

## Stage 3 — the keyboard-driven Manage Favorites panel

Goal: the Manage Favorites dialog is fully usable from the keyboard, and every
mouse interaction it has today still works.

New file `internal/ui/favorites/manage.go` holding `managePanel`, a focusable
widget built the way `widgets.TappableArea` is (`widget.BaseWidget` +
`widget.NewSimpleRenderer`), plus the dialog assembly moved out of
`favorites.go`.

Shape:

```go
type managePanel struct {
    widget.BaseWidget
    // rows, ring position, callbacks, the scroll it lives in
}

func (p *managePanel) FocusGained()
func (p *managePanel) FocusLost()
func (p *managePanel) TypedRune(rune)          // ignored
func (p *managePanel) TypedKey(*fyne.KeyEvent)  // Up/Down/Left/Right/Return/Escape
```

Behaviour:

- One row per favorite: `Name (count)` label, then an `Open` button and a
  `Remove` button. Each button sits behind a
  `widgets.NewFocusRing(widgets.ButtonRingWidth, widgets.RingRadius)`,
  inset the way `choicecard.go`'s `ringed` insets its buttons — a ring stacked
  at the same size as the button it marks is invisible.
- The ring is a `(row, col)` position with `col` in `{open, remove}`, named
  constants in the `cancelChoice`/`dangerChoice` spirit. `Up`/`Down` move
  rows, `Left`/`Right` move columns, **both clamp rather than wrap** — the
  rule `ChoiceCard.Select` already sets.
- `Return`/`Enter` runs the ringed button's action. `Escape` closes the dialog.
- A **click** runs that button's action regardless of where the ring currently
  sits, matching `ChoiceCard.runChoice`. Clicking should also move the ring
  there, so the two never disagree about what `Return` would do next.
- `Remove` renders with `widget.DangerImportance`, like the delete card's
  destructive button.
- Moving the ring outside the `container.VScroll` viewport scrolls it back
  into view; a ring the user cannot see is worse than no ring.
- Focus: the panel is focused via `f.win.Canvas().Focus(panel)` right after
  the dialog is shown, and the canvas is unfocused when it closes (the same
  release `grid.Overview.Close` performs). Fyne 2.8's `Canvas.Focus` resolves
  through the **top overlay's** focus manager, which is the dialog's — that is
  why a focusable content widget receives keys here at all, and why the app's
  own `SetOnTypedKey` dispatcher in `keys.go` never sees them and needs no new
  guard.
- `Open` hides the dialog and then runs the existing `openFavorite`, so
  `SyncFavoritePreviews` + `Host.OpenFiles` stay the single open path.
- `Remove` raises the existing `dialog.NewConfirm` and `performRemove`
  unchanged. That confirm is a second overlay, so it takes the keyboard while
  it is up; focus must return to the panel when it closes, whichever way it
  closes. After a successful removal, the rebuilt panel keeps the ring on the
  same row index, clamped to the new last row (and no ring at all once the
  list is empty).
- Empty state keeps the existing `"No favorites yet"` label, and the panel is
  still focused so `Escape` closes the dialog.
- Re-entry: `ShowManage` while the dialog is already open is a no-op rather
  than stacking a second dialog — the guard `deletion.RequestFiles` and
  `promptExport` both make for themselves.

Tests in `internal/ui/favorites/`: each direction clamping at its edge;
`Return` on `Open` reaching `Host.OpenFiles` with that favorite's files;
`Return` on `Remove` raising the confirmation; ring position preserved across a
removal and clamped when the removed row was last; a click running its own
button while the ring is elsewhere; the panel focused on show; `Escape`
closing; the empty state accepting `Escape` with no rows.

New translation keys: `"Open"` (if not already present — check the bundle
before adding).

## Stage 4 — `Cmd`/`Ctrl+Shift+F`

Goal: the dialog is reachable without the menu.

- `Feature.ShowManage` becomes exported; `manageItem.Shortcut` is set to
  `&desktop.CustomShortcut{KeyName: fyne.KeyF, Modifier: fyne.KeyModifierShortcutDefault | fyne.KeyModifierShift}`
  so the menu item displays the accelerator.
- New `wireManageFavoritesShortcut` in `internal/ui/shortcuts.go`, called from
  `wireGlobalShortcuts` in its existing order, bound to a viewer method that
  refuses while `v.deletion.Visible()` or `v.exportPrompt.Visible()` — the
  exact guard `promptExport` makes, and for the same reason: a shortcut
  bypasses `handleKeyEvent`, so without it the dialog would open over a card
  that still believes it owns the keyboard.
- `Cmd`/`Ctrl+Shift+E` (wallpaper) is the precedent for the modifier pair; `F`
  is unused in both the bare-key dispatcher and the shortcut table.

Tests: the shortcut registers through the existing `shortcutAdder` seam with
the right key and modifiers; firing it while the delete confirmation is up
does not open the dialog.

## Stage 5 — documentation and full verification

- `ARCHITECTURE.md`: update the `favorites/` row (the panel, the counts, the
  new shortcut) and the `shortcuts.go` row; keep the `Host` method count
  accurate — this feature adds **no** `Host` methods.
- `internal/ui/help/manual.md` and `manual_de.md`: the counts, the keyboard
  navigation inside Manage Favorites, and `Cmd`/`Ctrl+Shift+F` in both the
  shortcut list and the Favorites menu section.
- `README.md`: mention counts and the shortcut where favorites are described.
- `todos.md`: move this feature's entry to **Done** with a short summary.
- Full check from the repository root: `gofmt -l .`, `go vet ./...`,
  `go build ./...`, `go test -race ./...`.
