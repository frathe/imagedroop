# Planned feature: "Export image" prompt + export/wallpaper shortcuts

Covers the first two `todos.md` bullets, which are one File-menu change:

1. The two `Export as PNG…` / `Export as JPEG…` items become a single
   **Export image** item that asks which format, with a keyboard-driven
   prompt modelled on the delete confirmation, plus `Cmd/Ctrl+E`.
2. **Set as Wallpaper** gains and displays `Cmd/Ctrl+Shift+E`.

Decisions already made by the user:

- Both bullets ship together.
- The prompt is built on a **generalized** version of the delete
  confirmation card, extracted into `internal/ui/widgets` and used by both
  `internal/ui/deletion` and the export flow. No new feature package.
- Work runs as four sequential stages, one agent per stage.

## Ground rules for every stage

- Read `AGENTS.md` and the relevant part of `ARCHITECTURE.md` first.
- Every user-visible string is `lang.L("English text")` and needs the exact
  key in **both** `translations/en.json` (identity mapping) and
  `translations/de.json`; `main_test.go` enforces key-set parity.
- Do not add `TODO`/`FIXME` comments; open work belongs in `todos.md`.
- Do not run `git commit`. End with a suggested commit message.
- Match this codebase's comment style: comments explain *why* a thing is the
  way it is (see `deletion.go`, `shortcuts.go`), not what the line does.
- Verify with `gofmt -l .`, `go vet ./...`, `go build ./...`, then
  `go test -race ./...` from the repository root.

## Stage 1 — extract the modal choice card into `internal/ui/widgets`

Goal: one reusable scrim+card component with manual Left/Right selection
rings and Return/Escape handling, with `deletion.Confirmer` rebuilt on top
of it and **behaviourally and visually unchanged**.

New file `internal/ui/widgets/choicecard.go`, roughly:

```go
type Choice struct {
    Label      string
    Importance widget.Importance   // widget.MediumImportance for a plain button
    OnChosen   func()
}

func NewChoiceCard(repaint func(), choices ...Choice) *ChoiceCard
func (c *ChoiceCard) Overlay() fyne.CanvasObject
func (c *ChoiceCard) Visible() bool
func (c *ChoiceCard) Show(message string)          // resets selection to index 0
func (c *ChoiceCard) Hide()                        // hides + repaint
func (c *ChoiceCard) HandleKey(ev *fyne.KeyEvent)  // Left/Right/Return/Escape
func (c *ChoiceCard) SetOnCancel(func())           // Escape / index-0 semantics, if needed
func (c *ChoiceCard) Selected() int                // test seam
```

Semantics to preserve exactly from `deletion.Confirmer`:

- Index 0 is leftmost and the default selection; Left/Right **clamp**, they
  do not wrap.
- `Return`/`Enter` runs the selected choice; `Escape` cancels.
- The card hides *before* the chosen action runs (see `performDelete`).
- Exactly one focus ring is visible at a time (`widgets.NewFocusRing`,
  `ButtonRingWidth`, `RingRadius`); rings sit *behind* a padded button in a
  `container.NewStack`, per `ringed()`.

Layout must stay **pixel-identical for the two-button case**: same
`container.NewGridWithColumns(2, …)`, same `cardBG` with `CardRadius` and
`theme.ColorNameOverlayBackground`, same
`container.NewStack(scrim, container.NewCenter(card))`, same centered
word-wrapped `widget.Label`. The golden screenshots
`internal/ui/testdata/delete_confirm_cancel.png` and
`delete_confirm_danger.png` must still match; regenerating them is **not**
an acceptable outcome for this stage.

Then rewrite `internal/ui/deletion/deletion.go` to hold a `*ChoiceCard`
instead of its own `overlay`/`message`/`cancelRing`/`dangerRing`/`visible`/
`dangerSelected` fields. `deletion.Confirmer`'s exported API
(`New`, `Overlay`, `Visible`, `Request`, `RequestFiles`, `Cancel`,
`HandleKey`, `Settle`, `ShortcutHandler`) and its `Host` interface must not
change — `internal/ui` and every existing test call it as it is today.

Tests: `internal/ui/widgets/choicecard_test.go` covering selection clamping
at both ends, `Show` resetting selection, Return running the selected
choice, Escape cancelling, `Visible` transitions, and the repaint callback
firing. Existing `deletion` and `internal/ui` tests must pass untouched — if
one needs editing, that is a signal the extraction changed behaviour.

## Stage 2 — the export prompt, the menu item, and the shortcuts

- `internal/ui/export.go`: add `promptExport()` — no-op unless `canExport()`
  — that shows the `ChoiceCard` asking which format, with PNG and JPEG
  choices calling the existing `v.exportAs(exportPNGExt/exportJPEGExt)`.
  PNG is index 0 (the default selection). Escape cancels without a panel.
- `viewer` (`viewer.go`): replace `exportPNGItem`/`exportJPEGItem` with a
  single `exportItem *fyne.MenuItem`, and add the prompt field
  (`exportPrompt *widgets.ChoiceCard`).
- `features.go` or `build.go`: construct the prompt where `deletion` is
  constructed, and add `view.exportPrompt.Overlay()` to the window stack in
  `build.go` directly after `view.deletion.Overlay()` — it must paint above
  the grid backdrop for the same reason the delete card does.
- `menu.go`: one `lang.L("Export image")` item running `promptExport`,
  disabled by default, with a display-only
  `desktop.CustomShortcut{KeyE, KeyModifierShortcutDefault}`. Give
  `Set as Wallpaper` a display-only
  `{KeyE, KeyModifierShortcutDefault | KeyModifierShift}`. Update the
  file's opening comment, which currently explains why the export items
  carry no accelerator.
- `save.go` `updateFileMenuState`: one `exportItem.Disabled = !canExport()`
  in place of the two, and update its doc comment ("five file-dependent
  items").
- `shortcuts.go`: `wireExportShortcuts` binding `Cmd/Ctrl+E` to
  `promptExport` and `Cmd/Ctrl+Shift+E` to `setAsWallpaper`, registered from
  `wireGlobalShortcuts`. `E` is *not* one of the glfw driver's specially
  cased bare combos (only Z/Y/V/C/Insert/X/A are), so plain
  `desktop.CustomShortcut`s reach it the way `Cmd/Ctrl+S` does — say so in
  the doc comment.
- `keys.go`: the prompt owns the keyboard while it is up, exactly as the
  delete card does — add the check to `handleKeyEvent` (dispatch to
  `HandleKey`, then return) and to `handleTypedRune` (swallow). Order:
  deletion first, then the export prompt, then the grid. Note that plain
  `E` still opens the EXIF panel; only the modified combo is new.
- Translations for every new string in `en.json` and `de.json`
  (`"Export image"` already exists in both; the prompt's question and the
  `PNG`/`JPEG` button labels do not).

## Stage 3 — tests

- `internal/ui/export_test.go`: `promptExport` is a no-op when `canExport()`
  is false; it opens the card when true; choosing PNG/JPEG reaches
  `exportAs` with the right extension (drive it through the existing
  `filepicker` stubs in `internal/uitest`, the way the current export tests
  do); Escape closes it and starts no save panel.
- Keyboard: while the prompt is up, navigation/zoom/`S`/`M` keys are
  swallowed; `Escape` dismisses the prompt rather than resetting the
  session.
- `shortcuts` tests in the existing style (drive a bare
  `*fyne.ShortcutHandler` through `wireGlobalShortcuts`): `Cmd/Ctrl+E`
  opens the prompt, `Cmd/Ctrl+Shift+E` runs the wallpaper path.
- `menu_test.go` / `save_test.go`: the single export item's enabled state
  tracks `canExport`, and both new accelerators are displayed.
- Full suite: `go test -race ./...`. Report golden-screenshot results
  honestly; they need Docker (`make golden`) to regenerate and must not be
  regenerated here.

## Stage 4 — documentation

- `internal/ui/help/manual.md` and `manual_de.md`: the menu section (around
  the current `Export as PNG… / Export as JPEG…` and `Set as Wallpaper`
  entries), the rotation section that points at Export, the shortcut
  mentions, and the feature summary near the end. Document `Cmd/Ctrl+E` and
  `Cmd/Ctrl+Shift+E` and the prompt's Left/Right/Return/Escape keys.
- `ARCHITECTURE.md`: the new `widgets.ChoiceCard`, `deletion`'s use of it,
  the export prompt, and the File-menu/shortcut changes.
- `todos.md`: move the two bullets to `## Done`.
- Leftover `"Export as PNG…"` / `"Export as JPEG…"` translation keys: drop
  them from both bundles if nothing references them any more (parity is
  checked between bundles, not against the source, so they must be removed
  from **both** or neither).
- Suggested commit message for the whole change.
