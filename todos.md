# PicFetch — TODOs

## Done

 - Trane mascot rebrand
   The app icon (`assets/appIcon.png`, packaging + website favicon source),
   README/website header, and social-preview image now use the Trane
   artwork prepared under `assets/trane/`; the previous mascot is kept for
   reference under `assets/old_mascott/`. `docs/index.html` gained proper
   `<link rel="icon">`/`apple-touch-icon`/`shortcut icon` tags pointing at
   the prepared multi-size favicon set (its generic auto-generated
   `manifest.json` was left unwired — no existing PWA setup to hook it
   into). In-app: `internal/ui/assets/welcome.webp` and `placeholder.webp`
   are now resized Trane art (`internal/ui/assets/assets.go`'s
   `WelcomeWebP`/`PlaceholderWebP`), and a new `DiggingWebP` appears above
   the folder-scan spinner (`newScanUI` in `internal/ui/components.go`,
   `widgets.ScanArtSize`) — shown/hidden alongside `scanSpinner`/
   `scanLabel` at all three call sites in `drop.go`. Golden screenshots
   regenerated via `make golden`.

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

 - split `internal/ui/library_test.go` into per-feature test files
   The 2,428-line monolith's 69 tests now sit beside the code they exercise —
   `drop_test.go`, `sort_test.go`, `load_test.go`, `imgcache_test.go`,
   `animate_test.go`, `info_test.go`, `filestate_test.go`,
   `memlimits_test.go`, `windowsize_test.go`, `windowtrack_test.go` and
   `reset_test.go` — each opening with a header saying what it owns and what
   deliberately lives elsewhere. What remained is `harness_test.go`, reached
   by `git mv` rather than a fresh file so `git blame` survives on the 20
   helpers that 21 of the package's test files depend on. Pure motion: every
   declaration's code is byte-identical, verified by comparing comment-free
   per-declaration hashes across the whole package before and after.

## ACTIVE DEVELOPMENT

## TODO

## Group the `viewer` struct's field clusters into sub-structs

`internal/ui/viewer.go`'s `viewer` has ~70 fields and 111 methods across
the package. ARCHITECTURE.md is explicit that a general controller
extraction is not wanted — and this isn't that. Several field clusters are
already de-facto modules with their own files and single-writer contracts,
just flattened into one namespace:

- **Vector view** (`vector`, `vectorLogical`, `vectorRaster`,
  `vectorLifecycle`, `vectorPending`, `vectorDebounce`, `vectorRasterize`,
  `vectorAfter` — 8 fields, all consumed by `vector.go`): fold into a
  `vectorView` struct field. The write-once test seams travel with it.
- **Scan UI and sort UI are the same shape** (`scanLifecycle`/`scanning`/
  `scanDone`/`scanSpinner`/`scanLabel` vs `sortLifecycle`/`sorting`/
  `sortDone`/`sortSpinner`/`sortLabel`): one `asyncOpUI` type
  {lifecycle, active flag, done channel, spinner, label} used twice, with
  begin/finish/cancel methods that keep flag-vs-token bookkeeping in one
  place instead of spread across drop.go, sort.go and keys.go.
- **Settings-backed limits** (`maxScan`, `maxWinW`/`maxWinH`,
  `imgCacheMB`/`thumbCacheMB`/`maxFileMB`, `favPreviewCache`): a `limits`
  struct, so the settings window's Host surface reads as one concern.

Each cluster can move independently — three small, separately verifiable
commits rather than one big one.

## not deemed worth implementing (edge cases)

- There is a bug in the Windows Version: WHen in Gridview, multiselect via
  the space key works, but when trying it with mouse and Ctrl key, it does not.
  Holding the Ctrl key down and clicking on an image does not select it but
  instead opens it. Observation, when pushing the Ctrl key at exactly the same time
  as clicking on the image, it actually works, and the image is selected.
  (this seems to be a bug in fyne, created an issue, sorry Windows users)