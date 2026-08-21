# PicFetch — TODOs

## Done

 - favorites’ disk thumbnail cache
   Favorite grid previews are now generated in the background on save and
   topped up on open, cached under each favorite's own `thumbs/` folder keyed
   by source mtime+size, and swept for stale/removed entries afterward
   (`internal/favthumbs`, wired through `internal/ui/favthumbs.go`).

 - favorite item counts + keyboard-driven Manage Favorites
   Every favorite now shows how many files it stores, in both the Favorites
   menu and the Manage Favorites rows (`favstore.Count`, `internal/ui/favorites`'
   `menuLabel`). Manage Favorites is fully keyboard-driven — a focus ring
   moves over the rows and over each row's Open/Remove buttons, `Return`
   activates the ringed one, `Escape` closes (`internal/ui/favorites/manage.go`'s
   `managePanel`) — and gains its own shortcut, `Cmd`/`Ctrl+Shift+F`
   (`internal/ui/shortcuts.go`'s `wireManageFavoritesShortcut`).

 - keys aimed at a Fyne dialog no longer reach the image view
   The favorites removal confirmation is now a custom dialog whose content is
   a focusable `widgets.ChoicePanel` (Cancel/Remove, `←`/`→`, `Return`,
   `Esc`), so it holds the keyboard instead of leaving `Canvas.Focused()` nil.
   `handleKeyEvent`/`handleTypedRune` additionally ignore every key while a
   canvas overlay is up, which stops the same class for every other dialog —
   `Escape` used to reset the whole session from behind one.

 - keyboard-driven Add-to-Favorites and Replace-favorite prompts
   The Add dialog (`internal/ui/favorites/add.go`'s `nameEntry`/`addPanel`/
   `newAddDialog`/`showAdd`) opens with its name field auto-focused, `↓`
   hands the keyboard to a `Cancel`/`Add` `widgets.ChoicePanel` without
   moving its ring, `↑` hands it back, `Return` in the field saves once the
   name validates, and `Esc` cancels from either stop; `Add` stays greyed
   while the name is empty or invalid. The Replace-favorite confirmation
   (raised from `saveFavorite` on a name clash) is the same focusable
   two-choice shape, with `Cancel` and `Esc` alike reopening the Add dialog
   with the clashing name still typed rather than throwing it away. Both
   share the removal confirmation's shape through a new
   `internal/ui/favorites/confirm.go`'s `showConfirm`, so `dialog.NewConfirm`
   is now gone from the package entirely. `widgets.ChoicePanel` gained
   `SetOnBack` (what `Up` runs) and `SetChoiceEnabled`/`ChoiceEnabled` (a
   disabled choice runs and dismisses nothing, from a click or the
   keyboard) to support this.

## ACTIVE DEVELOPMENT

## TODO

## not deemed worth implementing (edge cases)

- There is a bug in the Windows Version: WHen in Gridview, multiselect via
  the space key works, but when trying it with mouse and Ctrl key, it does not.
  Holding the Ctrl key down and clicking on an image does not select it but
  instead opens it. Observation, when pushing the Ctrl key at exactly the same time
  as clicking on the image, it actually works, and the image is selected.
  (this seems to be a bug in fyne, created an issue, sorry Windows users)