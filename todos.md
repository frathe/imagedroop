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

 - fixed a `-race` failure in `TestShow_TracksAnimatedGIFLoopDuration`
   The test loaded a GIF with 50ms/100ms frame delays and then called
   `ShowImage` on the test goroutine, so `finishLoad`'s writes to
   `displayFrames`/`displayFrameIdx` raced the still-cycling `animate`
   goroutine's own write and `redrawRotatedFrame`'s read of the same two
   fields. Test-driver-only: the real driver marshals every `fyne.Do` onto
   the one UI goroutine, which serializes `animate`'s frame write against
   `finishLoad` (and against `slideshow`'s `Advance`, which is itself always
   inside a `fyne.Do`) — the fyne test driver runs the callback inline on
   the calling goroutine instead, so nothing serialized them. Fixed the way
   `TestCanSaveRotation_FalseForAnimatedImage` and
   `TestCanExport_TrueForAnimatedImage` already do it: multi-second frame
   delays park `animate` in its frame-delay `select` for the whole test, so
   its goroutine never touches those fields. Nothing here depended on the
   animation actually playing, only on the delays summing.

 - fixed the same `-race` failure in `TestViewerShow_NavigatingAwayStopsAnimation`
   The one CI was actually failing on (7 `DATA RACE` blocks, all tracing to
   `animate_test.go`'s `v.ShowImage(1)`). Same two fields, same cause as the
   entry above. This one could not just be skipped past: superseding a live
   animation by navigating is its subject. Parking the animation keeps that
   subject intact — `ShowImage` still bumps the lifecycle, and the parked
   goroutine wakes on `token.context().Done()` inside its frame-delay
   `select` and returns *without* entering its `fyne.Do`, so it never
   touches `displayFrames`/`displayFrameIdx` at all. What the test no longer
   claims is that the animation was mid-cycle at the instant of navigation;
   the test comment says so, and the frame-clock seam below is what would
   let it claim that again.

 - grouped the `viewer` struct's field clusters into sub-structs
   `viewer`'s ~70 flat fields lost three clusters that were already de-facto
   modules — each with its own file and its own single-writer contract — so
   the sub-struct is the type system catching up to a boundary the code
   already had. Not a controller extraction: nothing moved between packages,
   no `Host` interface changed, no feature gained or lost an owner.
   `vectorView` (`vector.go`) took the eight `vector*` fields, and
   `clearVector` split into `vectorView.clear` (its own state) plus the
   `zoom.SetLogicalSize` call the app makes about it. `asyncOpUI`
   (`asyncop.go`) turned out to fit twice: the folder scan and the background
   reorder are the identical shape — one cancellable lifecycle, an active
   flag, a per-request done channel, progress widgets — so they are two
   instances (`scanOp`, `sortOp`) rather than two near-duplicate types, with
   `begin`/`show`/`finish`/`invalidate`/`cancel` holding the flag-versus-token
   bookkeeping that used to be spread across `drop.go`, `sort.go` and
   `keys.go`. The type is deliberately viewer-independent: what a cancellation
   *means* — restore the drop zone, repaint, toast — stays at the call sites.
   `settings` (`memlimits.go`) took the seven fields the Settings window's
   `Host` surface reads and writes, which also freed that name by renaming the
   window field to `settingsWin`; storage only, every getter/setter stays
   beside its consumer.

   `vector`/`scanOp`/`sortOp` are value fields that must never be copied —
   they hold a `WaitGroup` and a lifecycle mutex, and `go vet`'s `copylocks`
   enforces it — following `winPos`'s precedent. `settings` carries no lock.
   Behavior-preserving throughout, with one dead-store removal: `cancelSort`
   used to hide its spinner and label a second time after `invalidateSort`
   already had. Two things were deliberately left alone rather than silently
   fixed: `clearToDropzone`'s bare `scanOp.lifecycle.invalidate()` (see the
   entry above it) and `favThumbLifecycle`/`favThumbDone`, which are the same
   lifecycle-plus-done-channel shape but have no progress UI to group with.

## ACTIVE DEVELOPMENT

## TODO

## `clearToDropzone` leaves a running scan's flag and widgets behind

`viewer.clearToDropzone` calls `v.scanLifecycle.invalidate()` but never
clears `v.scanning` and never hides the scan art/spinner/label — unlike
`cancelScan`, which does all three. Reachable through File ▸ Close Files
while a recursive folder scan is running: `keys.go`'s Escape branch checks
`v.scanning` *before* the reset branch, so the keyboard path can't get there,
but the menu item has no such guard. The scan's own completion closure won't
clean up either — it finds its token stale and returns early, and the comment
on `scanning` explains why that is normally fine ("the newer scan already
owns the flag by the time the stale one's closure would run") — but here
there is no newer scan. Needs checking against what `clearToDropzone`
repaints afterward before deciding whether anything is actually left visible.

## Give `animate` a frame-clock seam so animation timing is deterministic

Two tests have now had to be fixed for the same race, and the fix both
times was to park `animate` in a multi-second frame delay so its goroutine
never runs during the test. That works, but it means no test can exercise
an animation that is *actively cycling* — including the one case worth
covering, a navigation superseding a live animation.

The root of it: `animate` (load.go) sleeps on a bare
`time.After(delays[idx])`, so a test cannot step it. Every other piece of
background work in this package already has a per-viewer seam for exactly
this reason (`vector.after` is the closest precedent, and `vector.debounce`
alongside it). A `func(time.Duration) <-chan time.Time` field on `viewer`,
defaulting to `time.After`, would let a test release frames one at a time
and assert against a known frame index instead of racing or parking.

Worth doing before a third test hits this. Note the seam must be
write-once/pre-first-drop like the vector ones, per the concurrency
invariant.

## 4. RAW support via embedded preview extraction — L

Camera RAW files (CR2/CR3, NEF, ARW, DNG…) all embed full-size JPEG
previews. Extracting those is pure-Go-feasible (TIFF/IFD walking —
`internal/imaging/exif.go` already has the walker) and turns "drop the
memory card folder" into a supported workflow without shipping a demosaic
engine. Scope guard: viewer-only — no RAW decode, no editing; the info
overlay/title marks it as "(preview)". Extends `imaging.IsSupportedImage`
and the loader; EXIF window works as-is since the metadata lives in the
same IFDs.

## not deemed worth implementing (edge cases)

- There is a bug in the Windows Version: WHen in Gridview, multiselect via
  the space key works, but when trying it with mouse and Ctrl key, it does not.
  Holding the Ctrl key down and clicking on an image does not select it but
  instead opens it. Observation, when pushing the Ctrl key at exactly the same time
  as clicking on the image, it actually works, and the image is selected.
  (this seems to be a bug in fyne, created an issue, sorry Windows users)

