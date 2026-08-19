# PicFetch — Architecture

This document is for AI agents so they can find their way through the project.

PicFetch is a Fyne desktop app for viewing dropped/opened images: one binary, split from a single flat `package main`
into `internal/...`
packages. This doc is a navigation map of the current structure — start here to find anything.

## Package map

### `github.com/frathe/picfetch` (package main)

The entry point, and nothing else: `main.go` builds the `fyne.App`, loads the embedded translation bundles, converts
command-line paths to URIs (`argsToURIs`), and hands over to `ui.Run`. It stays at the module root so
`go build .`, the Makefile, and `fyne package` keep working unchanged.

### `internal/ui`

The application. Its unexported, package-local `appState` is the model boundary for the current file set: raw scan/drop
order, displayed order, current index, sort mode, and merge mode. `viewer` is that model's façade and the UI
orchestration hub: it owns Fyne widgets, rendering, and the operations that turn user events into state transitions.
Everything that could own state independently of that core is a subpackage, listed after this table.

This is deliberately not a general controller extraction. Async scan/load/sort/vector work remains with `viewer`, but
its generation/cancellation mechanics are shared through `lifecycle.go`'s package-local request contract. Window
geometry, menu enablement, and the widget-facing display/cache state those jobs coordinate remain here too. Native
file-picker/save dialogs are likewise outside this boundary: `openfiles.go` and `export.go` remain small viewer glue
over `internal/filepicker`. Feature packages keep their existing narrow consumer-side `Host` interfaces; `appState` is
not exported or passed to them as a broad controller.

Feature construction is centralized without becoming a registry:
`features.go`'s one explicit, ordered `registerFeatures` function assigns the eight feature modules directly to
`viewer`. The order stays visible because it is load-bearing; there is no generic feature interface, mutable registry,
or second controller layer.

`Run` is the package's only exported symbol. The `viewer` type never leaves the package; what the subpackages see of it
is a set of exported methods on an unexported type (the "vocabulary" in `viewer.go`), each subpackage binding only to
the few it declares in its own `Host`
interface.

#### Its own files

| File(s)          | Responsibility                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
|------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `run.go`         | `Run` — the only exported symbol and the explicit high-level runtime lifecycle: build the restored startup viewer, start runtime side effects with `favstore.DefaultDir`, register shutdown, register the CLI drop for `OnStarted`, then enter Fyne's event loop. Private `startViewerRuntime` initializes favorites from its caller-provided directory and starts main-window position polling; the directory parameter lets focused tests use temporary storage. Polling therefore cannot observe a nil slideshow or overwrite restored geometry. The shutdown save stops all three position pollers first (the main window's, and the two secondary windows' if they're still open) and builds its `preferences.State` in `currentPreferences`, split out purely so a test can read it back — `SetOnStopped` itself only ever runs inside a live event loop. Also the app-level constants (`appTitle`, the drop-zone size floor `startW`/`startH` — the window-size *ceiling* is `defaultMaxWindowWidth`/`defaultMaxWindowHeight` in `load.go`, next to the code that enforces it) |
| `build.go`       | `buildViewer` — top-level window composition from an already loaded `startupState`; it neither reads persistence, restores geometry, starts runtime polling, nor initializes disk-backed favorites storage. It composes the app-owned widgets from `components.go`/`toast.go` with the modules assigned by `registerFeatures`. The explicit typed-key and typed-rune callbacks remain here beside the window assembly, while one call delegates ordered application-wide shortcut registration to `shortcuts.go`. The root overlay stack stays here too, and the tail of its order is load-bearing: the grid's backdrop is opaque, so the delete confirmation and the toast are stacked *above* it or they render where nobody can see them |
| `startup.go`     | Startup inputs and restoration: private `startupState`, `loadStartupState` (the only UI-layer calls to `session.Load` and `preferences.Load`), cap-default normalization, and `restoreStartupGeometry` for the main and remembered secondary windows. `buildStartupViewer` is the single load → build → restore path shared by `Run` and test setup, so restoration always follows construction of the settings and EXIF windows. Normalization fills only the six positive caps; slideshow zero, size zeroes, position-set flags, and secondary geometry remain untouched because their zero values carry distinct “not chosen/saved” semantics |
| `components.go`  | App-owned widget construction: `fixedHeightLayout` and the dropzone, scan, sort, and info-overlay structs/constructors that `buildViewer` composes. Each constructor returns the same small widget cluster whose fields land in `viewer`; the self-dismissing toast remains in `toast.go` with its lifecycle                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `features.go`    | The one explicit feature-construction sequence, `registerFeatures`: help, EXIF, zoom, grid, deletion, slideshow, settings, then favorites. Grid is assigned before its thumbnail-cache budget is applied; slideshow is assigned before `startViewerRuntime` can start the position poller whose skip callback reads it. It assigns concrete viewer fields directly rather than introducing a generic feature interface or registry                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| `shortcuts.go`   | Application-wide modified-key shortcut registration: the narrow `shortcutAdder` test seam, one ordered `wireGlobalShortcuts` composition point, and the individual open, favorite, clipboard, delete, select-all, and save wiring functions that focused tests exercise directly. The Fyne driver-specific distinctions between built-in shortcuts and `desktop.CustomShortcut` live beside those registrations                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `windowtrack.go` | The app's two window-geometry bindings, one line each now that both mechanisms are shared with the secondary windows that remember their own geometry: `windowSizeTracker` (over `widgets.NewSizeTracker` — records the window's size on every layout pass) and `startWindowPosPolling` (over `winpos.Poll` — keeps `viewer.winPos` current, skipped while the slideshow is full-screen; returns a stop func `Run` calls at shutdown). Plus `widgetGeometry`/`prefGeometry`, the translation between `preferences.WindowGeometry` and `widgets.Geometry` — the same four values, owned separately so neither package has to import the other                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        |
| `testdata/`      | Golden-master screenshots for the e2e suite (moved here with the code that reads them, since a relative path can't reach a parent directory)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `state.go`       | The unexported, package-local `appState`: the current files, raw scan/drop order, current index, sort mode, and merge mode. Its mutation helpers copy replacement lists, reset/clamp the index, and remove one corresponding raw-order duplicate, so the model cannot split its displayed and unsorted lists. It is intentionally a state model, not a feature-facing controller: only `viewer` accesses it                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| `lifecycle.go`   | The package-local async contract: zero-value `revision` and `requestLifecycle` plus immutable `requestToken`. Beginning or invalidating a request advances its revision and cancels the previous context; background work checks the token both before expensive work and before applying through `fyne.Do`. Load, scan, sort, and vector each own an instance rather than sharing invalidation accidentally                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                 |
| `viewer.go`      | The `viewer` façade and orchestration hub: Fyne/UI state, navigation, image cache, and the small methods that apply `appState` changes to the screen. It owns title handling, `clearToDropzone`/`reset`/`showWelcomeState`, `closeFiles` (the File menu's "Close Files" item — like `reset` but never closes the window), `undoGridMaximize` (undoes a grid-triggered `winpos.Maximize` — see `grid/`'s `ConsumeMaximized` — before a resize elsewhere tries to shrink the window back down), merge-mode toggle/get/set, `showFileIfPresent`, and the exported vocabulary the feature packages' narrow `Host` interfaces bind to (`CurrentFile`, `RemoveFile`, `RemoveFiles`, `ShowImage`, `ShowToast`, `ShowEmptyStateError`, `ForceRepaint`, `FileCount`, `FileAt`, `OpenFiles`, `CurrentIndex`, `Generation`, `Unfocus`, `Modifiers`, `Advance`). `Generation` is the dedicated index-to-URI file-set revision consumed by grid and deletion; navigation does not move it. `OpenFiles` sends a favorite's stored list through the existing drop/scan path; `RemoveFiles` is the batch form `internal/ui/deletion` binds to - descending, duplicate-tolerant, and the one place the grid is reconciled after the file set shrinks (`grid.FilesChanged`, plus `grid.Close` once nothing is left); plain `RemoveFile` stays for `load.go`'s failed-decode retry |
| `keys.go`        | `handleKeyEvent` — the single keyboard dispatcher every unmodified key press runs through — plus `handleTypedRune`, its typed-*character* twin (wired to `SetOnTypedRune` alongside it in `build.go`). The grid's filename search is the only consumer of actual characters rather than key names, so runes are delivered only while the grid is up and dropped everywhere else                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `menu.go`        | `buildMainMenu` — the window's menu bar: a File menu (Open Files…/Save Changes/Export as PNG…/Export as JPEG…/Set as Wallpaper/Close Files/Settings…, built here) composed with the Favorites and Help feature menus. The one place that decides how those menus compose, per the cross-feature-composition rule below                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          |
| `drop.go`        | Drop handling and the recursive folder scan: `handleDrop`, its shared completion step `applyScanResult`, `applyScannedFiles` (merges or replaces the file set, then reorders and displays it via `sort.go`'s `startSort` instead of sorting inline on the UI goroutine), `cancelScan`, `realPathOf`, the `maxScan` cap and its `MaxScan`/`SetMaxScan` get/set (the settings window's binding). Scans own `viewer.scanLifecycle`, independent of navigation; a new drop or explicit cancellation supersedes them |
| `memlimits.go`   | The app's memory budget: the three limits bounding how much decoded-image memory can be held, and the settings window's getter/setter pairs for them (`MaxImageCacheMB`/`SetMaxImageCacheMB`, `MaxThumbCacheMB`/`SetMaxThumbCacheMB`, `MaxFileSizeMB`/`SetMaxFileSizeMB`), plus `bytesPerMB` and the shipped defaults derived from `internal/imaging`'s own. Grouped in one file rather than each sitting beside its consumer the way `MaxScan` (drop.go) and `MaxWindowWidth` (load.go) do, because they have no single consumer — the image cache is read in `load.go`, the thumbnail cache lives in `internal/ui/grid`, and the encoded-input ceiling is process-wide state in `internal/imaging` — while together they are one coherent thing. Each setter reaches through to what actually enforces the limit (`imgCache.SetBudget`, `grid.SetCacheBytes`, `imaging.SetMaxEncodedBytes`); `SetMaxImageCacheMB` additionally retunes the SVG re-render ceiling through `vectorRasterPixelsFor` + `imaging.SetMaxVectorRasterPixels`, since that raster is deliberately never charged to the cache and the derivation is how the user's one memory setting still bounds it |
| `load.go`        | Loading and displaying images: `ShowImage`/`attemptLoad`/`finishLoad`/`retryAfterLoadFailure`, neighbor preloading (which bails on the *header* when a neighbor's `imaging.EstimateDecodedBytes` exceeds half the cache budget, and writes with `AddIfFits` rather than `Add`, so a speculative decode can never displace the image on screen), the GIF `animate` loop, `resizeToImage` (takes its `maxW`/`maxH` cap as parameters rather than reading a package constant, so each call site passes the viewer's own `maxWinW`/`maxWinH`) plus `defaultMaxWindowWidth`/`defaultMaxWindowHeight` and the `MaxWindowWidth`/`SetMaxWindowWidth`/`MaxWindowHeight`/`SetMaxWindowHeight` get/set pairs (the settings window's binding). One `loadLifecycle` token spans a decode's retry chain, preloads, and animation; cancellation wakes semaphore/frame-delay waits promptly |
| `toast.go`       | The `toast` component - owns its widgets and a cancellable auto-hide lifecycle (atomic generation, per-show stop/done channels, injected duration) - plus the viewer's `showToast` wrapper                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
| `info.go`        | The persistent info overlay (I key): toggle/sync/update, `formatFileSize`. `syncInfoOverlayVisibility` also settles the "Show EXIF data" link, shown only when the file on screen actually has metadata (`viewer.currentHasEXIF`, carried on `imaging.LoadedImage` the same way `FileSize` is) — it sits there rather than in `updateInfoOverlay` because that one also runs on every zoom change, and a zoom can't add or remove a file's Exif. Also home to `displayedDimensions`, the one answer to "how big is this image": exactly `img.Image.Bounds()` for a raster format, but the rotation-aware *logical* size for a vector, whose on-screen raster gets denser as the user zooms — the shared rule behind both what the overlay reports and what `rotate.go`'s `applyRotationLayout` sizes the window to, each of which shipped a bug fix for reading the raw bounds                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                     |
| `sort.go`        | `toggleSort` (the `S` key) and `SetSortMode`/`SortMode` (the settings window's binding — jumps directly to a mode rather than cycling, safe to call before any files are loaded), plus `startSort`/`finishSort`: the shared background-reorder mechanism (own spinner/label, `sortLifecycle`, completion callback) used by both `SetSortMode` and `drop.go`'s `applyScannedFiles`, so neither freezes the UI on a large stat/Exif-heavy sort. Only a current token may hide progress, clear `sorting`, or invoke the callback; the orderings themselves live in `internal/filesort` |
| `rotate.go`      | View-only 90°-step image rotation (`rotateBy`/`resetRotation`/`redrawRotatedFrame`), composed with EXIF orientation at render time - not written to disk until the File > Save Changes action (see `save.go`) explicitly persists it. Stays here rather than becoming a package: it writes `img.Image` on the core load/animation path, which is the app's own side of the contract `zoom/` is written against. `applyRotationLayout` also hands `zoom` a local, axis-swapped *copy* of `v.vectorLogical` on a 90/270° turn - never the field itself, which stays unrotated since the re-render target in `vector.go` is built from it - since a vector's fit scale would otherwise be computed against the wrong axis. Both `rotateBy` and `resetRotation` call `updateFileMenuState` *before* `applyRotationLayout`, not after: the layout call's own resize can synchronously spawn a `vector.go` re-render goroutine (a real, single-goroutine-serialized non-issue in production, since `fyne.Do` there marshals onto the UI goroutine - but the fake test driver runs a `fyne.Do` callback inline instead, so `updateFileMenuState`'s read of `v.img.Image` afterward could otherwise race that goroutine's write under `-race`); ordering it first removes the race outright rather than narrowing it, since `updateFileMenuState` needs nothing `applyRotationLayout` computes |
| `vector.go`      | The debounced SVG re-render: how an image stays sharp as the display scale moves, once `internal/imaging` owns the parsing and rasterizing. `requestVectorRender` is `zoom`'s `onScaleChanged` handler - fired for a key, a scroll, or a fit-driven resize - and decides whether the new density is worth a fresh raster via `vectorNeedsRender`'s hysteresis band (`vectorSharpenRatio`/`vectorReleaseRatio`: over 1.05x denser to sharpen, under 0.5x to release memory on the way back out), so a slow scroll or a round-trip zoom doesn't re-render every frame. `rasterizeVector` runs off the UI goroutine under `vectorLifecycle`, whose cancellation coalesces a burst by waking superseded debounce waits; it checks its token before rasterizing and again on the `fyne.Do` hand-off. `clearVector` invalidates that lifecycle on every image change. The aliasing rule `finishLoad` also observes: a vector's displayed frame is replaced in place by every re-render, so it is given its own one-element `displayFrames` slice rather than sharing the cached `LoadedImage`'s backing array - writing through that would mutate the cache entry and invalidate the byte weight `ByteCache` already computed for it |
| `save.go`        | `canSaveRotation`/`saveRotation`/`updateFileMenuState`: the File menu's "Save Changes" item (also Cmd/Ctrl+S, see `shortcuts.go`'s `wireSaveShortcut`) that persists `rotate.go`'s view-only rotation back to the file it came from, via `internal/imaging`'s `SaveRotated`/`CanEncode`. Disabled except when there's a loaded, non-animated, encodable-format image with a pending rotation and no load in flight - see `canSaveRotation`'s own doc comment for why each of those matters; a successful save folds the just-written pixels into `displayFrames` and resets `rotation` to 0 so nothing visibly changes. `updateFileMenuState` lives here but drives all four image-dependent File-menu items - `export.go`'s two and `wallpaper.go`'s one included - since every site that calls it can move both conditions at once                                                                                                                                                                                                                                                                            |
| `export.go`      | `canExport`/`exportAs`/`runExport`/`suggestedExportPath`/`exportDestination`: the File menu's "Export as PNG…"/"Export as JPEG…" items, which write the frame on screen to a **new** file via `internal/filepicker`'s `ChooseSave` and `internal/imaging`'s `Export`. `canExport` is deliberately far weaker than `save.go`'s `canSaveRotation` - no encodable source format, no single-frame requirement, no pending rotation - because an export picks the *destination's* format, which is how pixels get out of a WebP/HEIC (decode-only here) or out of one frame of an animation; only the `!v.loading.Load()` guard is shared, and for the same reason. `exportDestination` holds the rule that keeps a file's bytes matching its name: an extension the user typed that this module can encode wins over the menu item, otherwise the menu item's extension is appended. Reuses `chooserDone` rather than a channel of its own - both panels are app-modal, so the open chooser and the save panel are never in flight at once |
| `wallpaper.go`   | `canSetWallpaper`/`setAsWallpaper`/`applyWallpaper`/`writeWallpaperFile`/`sweepWallpapers`/`defaultWallpaperDir`: the File menu's "Set as Wallpaper" item, over `internal/wallpaper`. `canSetWallpaper` is `canExport` verbatim, for the same reasons - what this writes is a PNG of the frame on screen, so neither the source format nor an animation nor a pending rotation matters, while a load in flight still does. It writes that PNG into `viewer.wallpaperDir` (`os.UserCacheDir()/picfetch/wallpapers`, a `t.TempDir()` under test) rather than pointing the OS at the user's own file, because every platform in `internal/wallpaper` stores a *reference*: the user's file is one Shift+Delete away from leaving the desktop with a broken wallpaper, and the copy also carries the rotation, one frame of an animation, and any decode-only format. The name carries a timestamp because macOS caches the desktop picture by path; `sweepWallpapers` is what keeps that from accumulating a file per invocation, and it deliberately runs only after `wallpaper.Set` succeeds - a failed set removes just the file it wrote, since the previous one may still be the live wallpaper |
| `slideshow.go`   | `togglePictureFrameMode` — the five-line glue that closes the grid before handing over to `slideshow.Toggle`, i.e. the one thing the slideshow package must not know — plus `toggleSlideshowShuffle` and the `SlideShuffle`/`SetSlideShuffle`/`SlideInterval`/`SetSlideInterval` get/set pairs (the settings window's binding)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `batch.go`       | `requestDelete`/`deleteGridSelection`/`copySelection`/`copyGridSelection`/`selectAllInGrid`/`reportFileCopyError` — what Shift+Delete and Cmd/Ctrl+C mean while the grid overview is up. **The only file in the module that knows both sides exist**: `internal/ui/grid` owns a selection and will say what is in it, `internal/ui/deletion` moves a set of files to the Trash, and neither imports the other. Each shortcut routes on `grid.Visible()` — the grid's `Targets()` (its selection, or the highlighted cell alone) while it is showing, the file on screen otherwise. The batch copy goes through `internal/clipboard`'s `CopyFiles` rather than `CopyImage`: a dozen selected images can't meaningfully become one clipboard image, so what goes on the clipboard is file *references* |
| `session.go`     | `restoreSession` — thin `*viewer` glue over `internal/session`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                |
| `clipboard.go`   | `copyPathToClipboard`, `copyImageToClipboard`, `reportClipboardError` — thin `*viewer` glue over `internal/clipboard`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| `openfiles.go`   | `openFileDialog`/`runFileChooser`/`reportChooserError` — thin `*viewer` glue over `internal/filepicker` — plus `chooserErrorDetail`, shared with `clipboard.go`                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               |

The last four are deliberately files rather than packages: each is a handful of lines binding one `internal/...` package
to viewer state, with nothing of its own to own.

#### Feature packages (`internal/ui/...`)

Each owns its state and its widgets, and declares the interface it needs from the app rather than being handed a shared
one. Sizes below are that interface, which is the honest measure of how coupled each still is.

| Package        | Responsibility                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | Reaches back via                                                                                                                                                                                                                                                                                                                                                                                   |
|----------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `zoom/`        | The zoom/pan view of the displayed image (0/1/+/-, click-drag pan, scroll-to-zoom anchored at the pointer), and the widget that lays it out in place of the image itself. Measures against a *logical* size rather than `img.Image.Bounds()` directly — `SetLogicalSize`/unexported `native`, with `LogicalSize` as a test-only reader, the same role `Fitting` plays — because an SVG's raster is re-rendered at a different pixel count as the display scale moves, so the pixel count is no longer the size the image should be drawn at; a raster format never calls `SetLogicalSize` and behaves exactly as before                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                        | Three callbacks now, not zero: `onChanged` (a zoom/pan change, to redraw the info overlay), `modifiers` (an accessor for which keys are held, for Shift+scroll pan), and `onScaleChanged` (the effective display scale moved — from a key, a scroll, or a fit-driven resize — which `internal/ui/vector.go`'s `requestVectorRender` uses to re-rasterize a vector). The single-writer contract itself is unchanged: it still shares one `*canvas.Image` where the app owns `img.Image` (the pixels) and this package owns only its size and position. What changed is that this package stopped assuming the pixel count *is* the image's size, not who writes what — zoom still never writes a pixel                                                                                                                                                                       |
| `grid/`        | The full-window thumbnail overview (`G` key): a virtualized `widget.GridWrap` over every loaded image, owning its own thumbnail cache (a separate *byte* budget from `imgCache`, so neither evicts the other) and the bounded worker pool that fills it. `SetCacheBytes` is its one setter, retuning that budget while the app runs — the same shape `slideshow.Controller.SetInterval` already has. Also owns the filename search (`/`, fed by `HandleRune`): a grid-local filter that renumbers the cells it draws while leaving the app's file set untouched — `matches` maps display index → host index, and everything crossing that boundary (`ShowImage`, `FileAt`) goes through `fileIndex`. `filterGen` is the staleness guard it adds alongside the host generation and cell recycling, for the one thing neither can see: a keystroke changes neither the file set nor a cell's id, only what that id *means*. Also owns the **multi-select** (`selection.go`): Cmd/Ctrl+click toggles a cell, Shift+click extends a range, Space and Cmd/Ctrl+A do the same from the keyboard, and `Targets()` is what a batch action acts on — the selection, or the highlighted cell alone when there isn't one. The set holds *host* indices, not the display indices actually clicked, so it survives a filter change; `displayIndex` is `fileIndex`'s inverse, needed to walk a Shift+click's range in display space. Escape stages (selection → search → grid), and both selection gestures call `Host.Unfocus` themselves, since GridWrap grabs canvas focus on every tap and only `Close` used to hand it back. `FilesChanged` is how the app resyncs it after a batch delete shrinks the file set under it. The ring and GridWrap's *own* keyboard cursor are two positions, and GridWrap moves the latter only for the arrow keys it handles itself — so every move of the ring goes through `setHighlight`, which drives both; `OnHighlighted` (fired by hover *and* by arrow keys) delegates to it behind an `id == g.highlight` guard, which is also what stops `wrap.Highlight`'s re-entry through that callback from recursing | 8-method `Host` — the search added none of them and multi-select added exactly one (`Modifiers`, since a Fyne tap carries no modifier state); knows nothing about the slideshow, and nothing about deletion or the clipboard — see below                                                                                                                                                                                                                                                                                                                                     |
| `deletion/`    | The Shift+Delete confirmation flow: its own card and button selection state, followed by a recoverable `trash.Move`. Takes a **set** of `Target`s (`RequestFiles`), not just the file on screen — `Request` is now the one-target wrapper over it, and worded identically for one file so the existing golden masters still hold. A batch's moves run one after another on the single background goroutine, collecting failures rather than aborting, so one file the OS refuses to move costs neither the rest of the batch nor the truth of the toast (`moved 9 of 12 files…`); only what actually moved is removed from the file set                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                          | 7-method `Host` (`CurrentFile`, `RemoveFiles`, `ShowImage`, `ShowEmptyStateError`, `ShowToast`, `ForceRepaint`, `Generation`) — the first of the consumer-side interfaces the split is built on. `RemoveFiles` takes a slice because removing a batch one index at a time would shift every later index out from under the list already captured                                                                                                                                                                                                     |
| `slideshow/`   | Picture-frame mode (`P` key): the full-screen switch, the auto-advance goroutine, the interval `Up`/`Down` tunes, and the `winpos.Tracker` capture/restore that puts the window back where the user left it                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           | 2-method `Host` (`FileCount`, `Advance`) — the smallest in the split; knows nothing about the grid — see below                                                                                                                                                                                                                                                                                     |
| `exifwin/`     | The EXIF metadata panel (`E` key and the info overlay's "Show EXIF data" link) over `internal/imaging`'s `ReadMetadata`. Below the tag list sits a collapsible **Location** section (`fyne.io/x/fyne`'s `widget.Map` over OpenStreetMap tiles, a marker on the capture position), hidden outright for a photo with no GPS tags and collapsed on every fresh open — which is what keeps it from fetching a tile unasked. The disclosure is hand-rolled (a button plus a hidden body) rather than a `widget.Accordion` precisely because expanding is the moment tiles start downloading and `Accordion` offers no way to be told. `tiles.go` is why the map doesn't freeze the app: the widget fetches tiles *inside* its raster draw, on the UI goroutine, so `tileFetcher` is installed as its `http.Client` transport and answers a cache miss instantly with `errTilePending` while downloading in the background — the widget skips that tile for the frame (and, importantly, does not cache the failure), and a redraw follows once the batch lands. Expanding first runs `Warm`, a 5×5 prefetch around the position behind a spinner, with the map hidden until it completes so the first frame is whole rather than a grid of holes. `quietPendingTiles` keeps that same design from filling the log: the widget reports every tile it doesn't get as an error, once per tile per frame, so a zoom or pan onto tiles still downloading would write dozens of blocks a second about this package's normal operation - `tileLogFilter` drops exactly the blocks caused by `errTilePending` and passes everything else, including a `tile fetch error` from any other cause. The panel's own content is a `Border` — metadata label on top, Location section filling the rest — so the map grows with the window rather than sitting at a fixed height; a transparent spacer behind it keeps a floor. A geotagged photo's latitude and longitude also appear as ordinary lines in the tag list. Remembers where and how large it was left (`RestoreGeometry`/`Geometry`/`StopTracking`, three lines over `widgets.Singleton`) and floats above the image window (`widgets.Singleton.KeepOnTop`), so navigating back to the photo doesn't bury the panel describing it                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                               | One `func() (fyne.URI, bool)` accessor — a single function is a smaller, more honest dependency than a one-method interface                                                                                                                                                                                                                                                                        |
| `help/`        | The manual, the About box, and the Help menu, plus the embedded `manual.md`/`manual_de.md` (`currentManual` picks by system locale - German for `de*`, English otherwise). `Menu()` returns the Help `*fyne.Menu` on its own (not a whole `*fyne.MainMenu`) so `internal/ui`'s `menu.go` can compose it with the File menu                                                                                                                                                                                                                                                                                                                                                                            | **Nothing at all** — no interface, no callbacks: everything it draws comes from `New(app, title, art)`                                                                                                                                                                                                                                                                                             |
| `settingswin/` | The Settings window (File > Settings…): one `widget.Form` (sort order, picture-frame interval, folder-scan cap, max window width, max window height, max image cache MB, max thumbnail cache MB, max file size MB) plus two `widget.Check`s (merge mode, picture-frame shuffle) below it. Every control seeds from its `Host` getter and pushes a change straight back through the matching setter on its own `OnChanged` — no Save/Apply step, no draft state of its own. The three numeric entries validate via `fyne.io/fyne/v2/data/validation.NewRegexp` (positive whole numbers only) on top of each `Host` setter's own domain clamp (e.g. `SetMaxWindowWidth` flooring at the drop-zone size). Remembers its own geometry exactly as `exifwin/` does, through the same three `widgets.Singleton` methods | `Host`: a getter/setter pair per preference (`SortMode`/`SetSortMode`, `MergeMode`/`SetMergeMode`, `SlideShuffle`/`SetSlideShuffle`, `SlideInterval`/`SetSlideInterval`, `MaxScan`/`SetMaxScan`, `MaxWindowWidth`/`SetMaxWindowWidth`, `MaxWindowHeight`/`SetMaxWindowHeight`, `MaxImageCacheMB`/`SetMaxImageCacheMB`, `MaxThumbCacheMB`/`SetMaxThumbCacheMB`, `MaxFileSizeMB`/`SetMaxFileSizeMB`) |
| `favorites/`   | The Favorites menu and its add, overwrite, manage, and remove dialogs. `New` performs no disk access; production calls `SetDir` from `Run`, while tests opt into a temporary directory. The first ten case-insensitively sorted entries carry Cmd/Ctrl+1–9,0 accelerators; `Open` resolves those permanently registered slots against the latest refresh. Removal runs off the UI goroutine because the OS trash implementation may shell out. | 4-method `Host` (`FileCount`, `FileAt`, `OpenFiles`, `ShowToast`) |
| `widgets/`     | Viewer-free UI mechanics shared across the packages above: `TappableArea` (the drop zone's tap target), `Singleton` (the raise-or-build lifecycle behind every secondary window, plus the opt-in geometry memory `Remember`/`Geometry`/`StopTracking` turn on — a `winpos.Tracker` and its poller for the position, a `SizeTracker` wrapped around the content for the size, both outliving the window itself so the app can save them at shutdown), the opt-in `KeepOnTop` (a `driver/desktop.Window` `RequestAlwaysOnTop` made before the window goes up, since that is the only moment the glfw driver reads it; a no-op on a backend with no native window, the test driver included), `NewSizeTracker` (the size half of that, also used by `internal/ui` for the main window), and the app's hardcoded style values (`CardRadius`, the dropzone/toast/scrim colors, `NewFocusRing`)                                                                                                                                                                                                                                                                                                                | Nothing from the app — a leaf apart from `internal/winpos`, which the geometry memory reads positions through                                                                                                                                                                                                                                                                                                                                                                             |
| `assets/`      | `WelcomeWebP`/`PlaceholderWebP`, the two images the UI embeds. They live beside the code that draws them because `//go:embed` cannot reach a parent directory; the root `assets/` keeps the icon and README artwork, which the build consumes rather than the program                                                                                                                                                                                                                                                                                                                                                                                                                                 | Nothing — it is a leaf                                                                                                                                                                                                                                                                                                                                                                             |

Concurrency invariant (established when the suite first became
`-race`-clean, phase 2 stage 2): the viewer has **no mutable package state** - test seams that used to be package vars
(`toastDuration`,
`maxScannedFiles`, `currentKeyModifiers`) are per-viewer fields now - and every background goroutine has both a
staleness guard (`requestToken`, or a feature-local generation where its semantics are richer)
and an explicit stop/done signal: the load token's context plus `animStopped` (GIF playback), the slideshow's `Exit`/`Settle` pair, the
poller's stop func, the toast's per-show `stop`/`done` pair, `clipboardDone`, `chooserDone`, `wallpaperDone`, the
request lifecycles' cancellable contexts, and the grid's and viewer's
`pending`/`preloadPending`/`vectorPending` WaitGroups for thumbnail, neighbor-preload, and vector-rasterization decodes. Under Fyne's test driver `fyne.Do`
runs a goroutine's callback inline rather than marshaling it to a UI thread, so the test suite leans on those signals
(`settleToast`/`settleThumbs`/`settleSlideshow`/`settleChooser` and the
`waitFor*`/`dropAndWait` channel helpers in `library_test.go`) instead of ever letting a background goroutine's UI work
overlap the test goroutine's own. `newTestUI`'s `drain` cleanup waits out all of them at the end of every test, so work
a test deliberately abandoned can't run on into the next one — if you add a background operation, give it a signal and
add it there.

The two full-window modes — the grid and the slideshow — must not overlap, and neither package imports the other: the
guards live in this package's dispatcher (`handleKeyEvent`'s `G` case checks `slides.Active()`) and in
`togglePictureFrameMode`, which closes the grid on the way in. That is the general rule for cross-feature interaction
after the split: features expose state and actions, and `internal/ui` decides how they compose.

`appState` is the accepted model boundary inside this package, while `viewer` deliberately remains the façade that
orchestrates it with widgets and features. The split stops before async scan/load/sort lifecycle, geometry restoration,
menu enablement, native file-picker/save-dialog glue, display/cache state, and rendering: moving those would either
mix Fyne lifecycle into the model or widen feature dependencies. Every feature that could own its own state still does,
through its existing narrow consumer-side interface rather than a shared controller.

### `internal/imaging`

Read → probe → decode → EXIF-orient → cache pipeline for image files (JPEG/PNG/GIF including animated, WebP, BMP, TIFF,
ICO, XPM, HEIC, AVIF, SVG), plus a narrower encode-and-write-back path (`save.go`) for the subset of those formats this
module can also re-encode. HEIC/AVIF decode via
`github.com/gen2brain/{heic,avif}` (WASM/wazero, no cgo) and apply their own orientation/transform internally, so
`readEXIFOrientation` deliberately leaves them alone. SVG is the odd one out: the only vector format, so unlike every
raster format its pixels are not fixed at load — `svg.go`/`vector.go` below are what let it be rasterized again as the
display scale changes. Zero dependency on `viewer`.

| File             | Responsibility                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                    |
|------------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `bytecache.go`   | `ByteCache[V]` — the goroutine-safe LRU **bounded by estimated bytes rather than entry count** that both caches below are built on, plus the weight functions (`imageBytes`, type-switched per concrete `image.Image` so a JPEG's `*image.YCbCr` isn't charged 4 B/px; `loadedImageBytes`, which also charges a retained SVG `Vector`'s parse tree — proportional to its encoded source length, since that is already bounded by `MaxEncodedBytes` and walking the tree itself to measure it would cost more than the charge is worth) and `EstimateDecodedBytes`. Its one load-bearing rule: eviction never removes the *most recently added* entry, so the image being displayed stays resident even under a budget smaller than itself — which is what lets `Add` (the current image) and `AddIfFits` (a speculative neighbor) mean different things                                                                                                                                                                       |
| `loader.go`      | `LoadedImage`, `NewImgCache`, `ReadAndProbe`, `DecodeLoaded`, `LoadImage`, `IsSupportedImage`, `InvalidDimensionsError`; plus the encoded-input ceiling — `DefaultMaxEncodedBytes`/`MaxEncodedBytes`/`SetMaxEncodedBytes` and `InputTooLargeError`, which `readRawBytes` enforces with an `io.LimitReader` of limit+1 (the extra byte is what tells "ended at the limit" from "truncated there"). That limit is a package-level atomic rather than a parameter because it is genuinely process-wide decode policy — see its own comment. `LoadedImage`'s `FileSize` and `HasEXIF` are both filled in by the *caller* (`internal/ui/load.go`, at the two sites that decode into the image cache) rather than by `DecodeLoaded`: the thumbnail path decodes through here too and needs neither                                                                                                                                                                                                                                           |
| `svg.go`         | SVG format detection and size arithmetic. `isSVGData`'s root-element scan (`svgRootAttrs`) is the real format test, not whether `oksvg` parses the file: `oksvg` accepts a JSON document without complaint and reports a 0×0 viewBox for it, so "did it parse" can't mean "is it an SVG". `MinVectorWidth`/`MinVectorHeight` (520×340) are the floor a smaller SVG's logical size is raised to — deliberately equal to `internal/ui`'s `startW`/`startH`, the app's smallest window, so an icon-sized SVG opens filling that window instead of as a tiny stamp in its corner. `internal/imaging` can't import `internal/ui` to enforce that equality itself, so `internal/ui/vector_test.go`'s `TestVectorFloorMatchesStartWindowSize` pins the two constants together instead. `MaxVectorRasterPixels` caps a single rasterization — no longer a hard constant but derived from the user's image-cache setting (a quarter of the budget's bytes at 4 B/px, clamped to an 8 MP usability floor and the 32 MP `DefaultMaxVectorRasterPixels` ceiling; at the shipped 512 MB default that lands exactly on the old 32,000,000 constant). It is process-wide atomic state in the mold of `MaxEncodedBytes`, seeded by `buildViewer` and retuned by `internal/ui`'s `SetMaxImageCacheMB` (`memlimits.go`'s `vectorRasterPixelsFor` holds the derivation), and enforced by the exported `ClampVectorRaster` — exported because `internal/ui` must clamp its own re-render target through the same function *before* comparing it against the raster already on screen, or a target the cap would shrink anyway would look like a permanently unmet demand for a sharper image, and re-render forever. `svgSizeFrom` also repairs the 0×0 viewBox `oksvg` silently produces for `width="100%"` documents: `oksvg` abandons root-element parsing on the first attribute it can't read, which for most web-exported SVGs is that percentage width, before it ever reaches the viewBox that follows |
| `vector.go`      | `Vector`/`ParseVector`/`RasterAt`: a parsed SVG kept alive on `LoadedImage` so it can be rasterized again at a different pixel size whenever the zoom level or window size changes, instead of being decoded once like a raster format. `RasterAt`'s mutex guards the whole `SetTarget`-then-`Draw` sequence (`SetTarget` writes the icon's transform, `Draw` reads it) because two of `internal/ui/vector.go`'s `rasterizeVector` goroutines can be inside it on the same `*Vector` at once: one that already passed its own staleness check keeps running while a fresher scale change spawns another and bumps the generation - `TestRasterAtIsSafeForConcurrentUse` covers exactly this. (The grid's thumbnails are not a second party here: `LoadThumbnail` always decodes its own, ephemeral `*Vector` through a separate cache, and discards it after one raster.) Its `recover` sits inside that same lock: `oksvg` panics outright on some inputs (an extreme viewBox raises a slice-bounds panic), and letting that escape would both crash the app and leave the transform half-written under a still-held mutex |
| `exif.go`        | EXIF orientation-tag parsing (unexported: `readEXIFOrientation`, `parseExifOrientation`) and the general-purpose tag reader `ReadMetadata`/`Metadata` (camera make/model, lens, exposure, aperture, ISO, focal length, capture date, and the GPS position the EXIF window's map view centers on — the `0x8825` pointer in IFD0 leads to a sub-IFD this reader now follows, and `Metadata.HasGPS` is what tells a photo at (0, 0) from one with no location tags); falls back to `heic`/`avif`'s own `DecodeExif` for HEIC/AVIF files, which aren't JPEG-APP1-boxed                                                                                                                                                                                                                                                                                                                                                                                                              |
| `orientation.go` | Pixel-level rotate/flip transforms (`ApplyOrientation`, `RotateSteps`)                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                            |
| `gif.go`         | Animated GIF frame decoding/compositing (unexported: `decodeAnimatedGIF`), under a cumulative byte budget: every frame is retained as a full composited RGBA canvas, so cost is canvas size × frame count. The check runs *before* `gif.DecodeAll`, against a frame count and canvas size that unexported `probeGIF`/`skipGIFExtension`/`skipGIFSubBlocks` walk out of the raw GIF block structure without decoding a pixel — so an over-budget animation allocates nothing at all, and the *transient* paletted decode is bounded too: `image/gif` rejects any frame larger than the logical screen, so `DecodeAll`'s own peak is at most a quarter of the budget just cleared. An over-budget GIF then takes the same nil-slice path a non-animated one does — `image.Decode` yields frame 0 — and reports `truncated` so the UI can say why it isn't moving. A zero budget means "never composite", which is what the thumbnail path passes. `probeGIF` mirrors `image/gif`'s own `readExtension` block for block and is deliberately *more lenient* than it, never stricter: accepting a file the decoder rejects merely lands on the static fallback, while rejecting one it accepts would stop a readable GIF animating. `FuzzProbeGIFAgreesWithDecodeAll` pins that agreement, and found the one case where a plain sub-block walk got it wrong                                                          |
| `thumbnail.go`   | `NewThumbCache`, `LoadThumbnail`, unexported `scaleToFit`/`fitEdge` — probes and decodes (`ReadAndProbe` + `DecodeLoaded`, the same pipeline `LoadImage` wraps) then downsamples (`golang.org/x/image/draw`, `ApproxBiLinear`) for `internal/ui/grid`. Passes a **zero animation budget**, so a long GIF no longer composites every frame here just to keep frame 0. An SVG never rasterizes at full logical size here: its branch aims `RasterAt` straight at the `fitEdge` target (~200 px), which is near-free, and discards the ephemeral `Vector` after that one raster                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| `save.go`        | `SaveRotated`, `Export`, `CanEncode`/`CanEncodeExt`, an unexported extension→encoder table (JPEG at quality 95 rather than the stdlib's lossier default 75, PNG, single-frame GIF, BMP, TIFF, AVIF - all already linked in for decode). Both writers go through the unexported `writeEncoded`, which writes to a temp file in the target's own directory and `os.Rename`s it over the destination, so a failed or interrupted encode can never corrupt what was there; it does not preserve the original file's Exif metadata, since it's a plain re-encode rather than a patch of the existing bytes. WebP/HEIC have no Encode in the libraries this module depends on, and ICO/XPM aren't meaningful save targets, so `CanEncode` reports false for all four - `internal/ui/save.go`'s `canSaveRotation` checks it before ever offering the File > Save Changes action. `Export` is the way around all four: it re-encodes into whatever format the *destination* names, so a WebP or HEIC - or one frame of an animation - leaves through File > "Export as…" (`internal/ui/export.go`) even though it can never be written back in place. It picks its encoder from the destination extension with no symlink resolution, which is what `CanEncodeExt` exists for: `CanEncode` resolves a symlink first because `SaveRotated` writes through one, while an export destination is a name the user just typed and may not exist yet |

Extracted from `library.go` + the root `orientation.go`/`exif.go`/`gif.go` on 2026-08-13.

### `internal/favstore`

Persists named file lists under the user's config directory. It takes the base
directory explicitly for every operation, uses atomic temp-file-and-rename
writes, sorts favorite names case-insensitively, reconstructs JSON index keys
numerically, and delegates removal to `internal/trash`. It has no UI or mutable
package state; `DefaultDir` is the production-path helper.

### `internal/session`

Persists and restores the set of files that were open when the window last closed, via Fyne's app-scoped cache. Zero
dependency on `viewer`.

| File         | Responsibility                                |
|--------------|-----------------------------------------------|
| `session.go` | `Save`, `Load`, unexported `state`/`cacheKey` |

Extracted from the root `session.go` on 2026-08-13. The root `session.go` now holds only `viewer.restoreSession()`, the
thin glue that hands `v.savedSession` to `handleDrop`.

### `internal/preferences`

Persists and restores standing UI preferences (sort order, merge mode, the picture-frame slideshow interval and shuffle
order, the folder-scan cap, the window-size cap, window size and position) across launches, via Fyne's app-scoped
`Preferences` store (`fyne.App.Preferences()` — distinct from
`internal/session`'s app *cache*, which is the transient file-set store). Zero dependency on `viewer`.

| File             | Responsibility                                                                                          |
|------------------|---------------------------------------------------------------------------------------------------------|
| `preferences.go` | `Save`, `Load`, `State`, `WindowGeometry`, unexported preference keys and `saveGeometry`/`loadGeometry` |

Added 2026-08-14, mirroring `internal/session`'s shape. `internal/ui/startup.go`'s
`buildStartupViewer` loads the saved `State`, normalizes only unset caps, hands it to `buildViewer` to seed standing
feature preferences, and then restores main and secondary window geometry. `Run` starts runtime position polling only
after that helper returns, and its `SetOnStopped` saves the current values back alongside the existing `session.Save` call.
`WindowPosX`/`WindowPosY`/`WindowPositionSet`
were added 2026-08-14 for manual-window-move persistence — see
`internal/winpos` for why reading a position back needs a whole package of its own where reading a size back didn't.
`SortMode` (added 2026-08-14 for the multi-criteria sort feature) is a string, not an enum: it was originally a string
because the enum lived in `package main` and couldn't be imported here without a cycle. `internal/filesort` (stage 5)
removed that constraint, but the string stays on purpose — it's the on-disk format, and keeping it decoupled from the
enum's declaration order means reordering or renaming a mode can't silently reinterpret a saved preference.
`filesort.FromPref`/`Mode.PrefValue` are the translation. `MaxScanFiles`
and `MaxWindowWidth`/`MaxWindowHeight` were added alongside the Settings window (`internal/ui/settingswin`) so those
standing preferences persist across launches the same way the others already did; all three use the same
zero-means-unset sentinel `WindowSize` does, since the viewer itself never accepts a zero cap for any of them (see
`internal/ui/drop.go`'s
`defaultMaxScannedFiles` and `internal/ui/load.go`'s
`defaultMaxWindowWidth`/`defaultMaxWindowHeight` fallbacks in
`startup.go`). `MaxImageCacheMB`/`MaxThumbCacheMB`/`MaxFileSizeMB` were added 2026-08-16 with the byte-bounded image
memory work and follow the identical pattern (`internal/ui/memlimits.go` holds their defaults and setters). They are
stored in megabytes rather than bytes on purpose: that is the unit the user typed into the Settings window, so it is the
unit that round-trips, and the conversion to the byte budgets `internal/imaging` enforces happens in the setters.
`SettingsWindow`/`ExifWindow` were added 2026-08-17 so the two secondary windows reappear where and how large the user
left them, the same as the main window: a `WindowGeometry` each (position, a `PositionSet` flag, size) over five keys
apiece, written by the shared `saveGeometry`/`loadGeometry` rather than another ten statements in `Save`/`Load`. They are
grouped into a struct where the main window's own geometry is flat, because there are two of them and any further
`widgets.Singleton` window that wants remembering adds a third — `internal/ui/windowtrack.go`'s
`widgetGeometry`/`prefGeometry` translate to and from `widgets.Geometry`, keeping this package free of any UI import.

### `internal/winpos`

Reads and writes a Fyne desktop window's on-screen position. Fyne's public
`Window` has no position getter and no "window moved" event at all — only
`driver/desktop.Window.RequestPosition`, which is write-only — so `Get`
reaches past that into the raw native window handle (`driver.NativeWindow.RunNative` hands out an `NSWindow`/`HWND`/X11
handle depending on platform) and asks the OS directly, mirroring exactly what Fyne's own glfw driver does internally
for its own position bookkeeping so a value round-tripped through `Get` then `Set` lands back where it started. Zero
dependency on `viewer`.

| File         | Responsibility                                                                                                                                                                                                                                                                                                                                                                                                                        |
|--------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `winpos.go`  | `Get`, `Set`, `Maximize`, `Unmaximize` — the platform-agnostic dispatch over `driver.NativeWindow`/`desktop.Window`                                                                                                                                                                                                                                                                                                                   |
| `poll.go`    | `Poll` and `PollInterval` — the background sampler that keeps a `Tracker` current, and the one place the reasons for polling at all (no move event, no position getter) and for hopping each reading through `fyne.DoAndWait` (AppKit's main-thread rule) are written down. Shared by the main window (`internal/ui/windowtrack.go`) and every `widgets.Singleton` window that remembers its position; `skip` is the caller's own "not right now" rule, which only the main window has one of |
| `tracker.go` | `Tracker` — the last *good* reading, kept in atomics (`Store`/`Get`/`Capture`/`Restore`). `Get` above answers "where is this window right now", which is unavailable or wrong at exactly the moments the position is needed: while full-screen it reports the screen corner, and at shutdown the event loop the read hops through is winding down. A `Tracker` is what the poller, the slideshow, and the shutdown save share instead |
| `darwin.go`  | `platformPosition` — cgo/AppKit `NSWindow` frame read, converted out of Cocoa's bottom-left-origin coordinate space. `platformMaximize`/`platformUnmaximize` — `-zoom:`, AppKit's own toggle between the standard and user frame, each guarded to only fire when the window isn't/is already zoomed                                                                                                                                   |
| `windows.go` | `platformPosition` — `ClientToScreen` via `syscall`, matching glfw's own Win32 position query (not `GetWindowRect`, which would include the non-client frame). `platformMaximize`/`platformUnmaximize` — `ShowWindow(SW_MAXIMIZE)`/`ShowWindow(SW_RESTORE)`                                                                                                                                                                           |
| `linux.go`   | `platformPosition` — cgo/Xlib `XTranslateCoordinates`; reports `ok=false` on Wayland (no such handle exists there), matching `RequestPosition`'s own documented Wayland limitation. `platformMaximize`/`platformUnmaximize` — an EWMH `_NET_WM_STATE` `ClientMessage` adding/removing both maximized states, X11-only for the same reason                                                                                             |
| `other.go`   | `platformPosition` — always `ok=false`, for BSD/mobile/wasm/anything else. `platformMaximize`/`platformUnmaximize` — no-ops                                                                                                                                                                                                                                                                                                           |

Added 2026-08-14 for the "restore the window where the user manually left it" feature: `internal/ui`'s
`buildStartupViewer` uses `restoreStartupGeometry` to seed `viewer.winPos` from the saved preference and apply it to the
window, then `Run` calls `startViewerRuntime`, which starts `startWindowPosPolling` (`windowtrack.go`), a background
goroutine that keeps the tracker current via `Capture` since —
unlike a resize — a pure window-drag triggers no layout pass for `windowSizeTracker` to piggyback on. The poller only
ever runs against a real `driver.NativeWindow` (checked once up front), so the fyne test driver's windows — every test
in `internal/ui`, including the focused runtime-start test — receive a no-op stop callback instead of a poller
goroutine. `internal/ui/slideshow` captures
and restores the same tracker around full-screen, so leaving picture-frame mode puts the window back at the
manually-placed position instead of wherever the OS chose to un-full-screen it to. The `Tracker` (added 2026-08-14,
stage 8) is what gives those three consumers one place to share that state, rather than a set of atomics on the viewer
that each of them reached into.

`Poll` (added 2026-08-17) is that background goroutine itself, moved down here out of `startWindowPosPolling` when the
Settings and EXIF windows started remembering their own positions too: nothing about the loop was app-specific, and a
second and third copy of the AppKit-main-thread reasoning is exactly what a shared package exists to prevent. The
`internal/ui` side kept only its skip rule (no reading while the slideshow is full-screen).

### `internal/clipboard`

Puts PNG-encoded image data onto the system clipboard as real image data, via a per-OS shell-out (AppleScript on macOS,
xclip/wl-copy on Linux, PowerShell on Windows). Zero dependency on `viewer`.

| File                           | Responsibility                                                                                                                                 |
|--------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------|
| `clipboard.go`                 | `CopyImage` (exported dispatcher var), unexported per-platform impls (`copyImageDarwin`, `copyImageLinux`, `copyImageWindows`), `writeTempPNG` |
| `copyfiles.go`                 | `CopyFiles` (exported dispatcher var) — the file-*reference* twin of `CopyImage`, for the grid's batch copy: what a file manager's own Copy produces, so a Paste there creates copies of the files. Unexported `copyFilesLinux` (a `text/uri-list` over the same xclip-then-wl-copy pair), `copyFilesWindows` (PowerShell `Set-Clipboard -LiteralPath`, i.e. CF_HDROP — the very thing `copyImageWindows` avoids), `uriList`, `writeTempList`. An empty list is a deliberate no-op: writing one would clear whatever the clipboard already held |
| `darwin.go` / `other.go`       | `copyFilesDarwin` (build-tag pair, real cgo/AppKit impl / non-darwin stub) — `NSPasteboard.writeObjects:` over an `NSURL` array, mirroring `internal/trash`'s own darwin.go. Not an osascript shell-out like `copyImageDarwin`: AppleScript has no reliable form for a *list* of files on the clipboard, and scripting Finder to do it would trigger the Automation permission prompt `internal/trash` exists to avoid |
| `windows.go` / `notwindows.go` | `hideConsoleWindow` (build-tag pair, real impl / no-op twin)                                                                                   |

Extracted from the root `clipboard.go` on 2026-08-13. The root `clipboard.go` now holds only the `*viewer` glue
(`copyPathToClipboard`, `copyImageToClipboard`, `reportClipboardError`) that encodes the current frame and calls
`clipboard.CopyImage`.

### `internal/filepicker`

Opens the current OS's own file browser and returns the paths picked, and (via `ChooseSave`) its save panel for File > "Export as…":
zenity on Linux, a WinForms dialog via PowerShell on Windows, in-process cgo/AppKit `NSOpenPanel` on macOS. Linux and
macOS can pick folders too; Windows is files only (its shell dialog has no mode that combines folder and multi-file
selection - folders there go through drag-and-drop instead). Zero dependency on `viewer`.

| File                           | Responsibility                                                                                                                                                                     |
|--------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `filepicker.go`                | `Choose`/`ChooseSave` (exported dispatcher vars), `ParseFileList` (exported, shared by both), unexported per-platform impls (`chooseFilesLinux`/`chooseSaveLinux`, `chooseFilesWindows`/`chooseSaveWindows`, `buildPowerShellCmd`/`buildPowerShellSaveCmd`, `powerShellEscape`) |
| `darwin.go` / `other.go`       | `chooseFilesDarwin` (`NSOpenPanel`) and `chooseSaveDarwin` (`NSSavePanel`) (build-tag pair, real cgo/AppKit impls / non-darwin stubs)                                             |
| `windows.go` / `notwindows.go` | `hideConsoleWindow` (build-tag pair, real impl / no-op twin)                                                                                                                       |

Extracted from the former platform-specific open-file implementations on 2026-08-13. `internal/ui/openfiles.go` now
holds only the `*viewer` glue (`openFileDialog`, `runFileChooser`, `reportChooserError`) that calls
`filepicker.Choose`/`filepicker.ParseFileList`; the shared tappable widget lives in `internal/ui/widgets`.

Note: `hideConsoleWindow` now exists as four separate build-tag-pair copies — here, in `internal/clipboard`, in
`internal/trash`, and nowhere else (the root `openfiles_windows.go`/`openfiles_notwindows.go` that used to hold another
copy were deleted, since `main` no longer calls it directly). Each copy is ~10 lines and unexported, so duplicating it
per package beat introducing a shared package for one tiny OS-quirk helper.

### `internal/trash`

Moves a file to the current OS's trash/recycle bin rather than deleting it outright, for `internal/ui/deletion`'s
Shift+Delete flow. Each platform needs its own approach: `gio trash` (falling back to `trash-cli`'s
`trash-put`) on Linux, both of which already implement the freedesktop.org trash spec correctly;
`Microsoft.VisualBasic.FileIO`'s recycle-bin delete via PowerShell on Windows; in-process cgo/AppKit
`NSWorkspace.recycleURLs:completionHandler:` on macOS — deliberately not an AppleScript
`tell application "Finder" to delete` shell-out, since scripting another app that way triggers a one-time Automation
permission prompt that a direct framework call avoids. Zero dependency on `viewer`.

| File                           | Responsibility                                                                                                       |
|--------------------------------|----------------------------------------------------------------------------------------------------------------------|
| `trash.go`                     | `Move` (exported dispatcher var), unexported per-platform impls (`moveLinux`, `moveWindows`, `escapePowerShellPath`) |
| `darwin.go` / `other.go`       | `moveDarwin` (build-tag pair, real cgo/AppKit impl / non-darwin stub)                                                |
| `windows.go` / `notwindows.go` | `hideConsoleWindow` (build-tag pair, real impl / no-op twin)                                                         |

Added 2026-08-16. `internal/ui/deletion`'s `performDelete` calls
`trash.Move` in place of the `os.Remove` it used before, and
`internal/uitest`'s `StubTrashMove` swaps it out the same way
`StubClipboardCopy` does for `clipboard.CopyImage`, so tests never invoke the real per-OS mover.

### `internal/wallpaper`

Makes an image file the desktop wallpaper, for `internal/ui/wallpaper.go`'s File > "Set as Wallpaper" action. Same
per-OS-dispatch shape as `internal/trash`: in-process cgo/AppKit
`NSWorkspace.setDesktopImageURL:forScreen:options:error:` on macOS — deliberately not an AppleScript
`tell application "System Events" to set picture of every desktop`, for exactly the reason `internal/trash` avoids
scripting Finder, and it reads each screen's existing options back so only the picture changes, not the user's
Fill/Fit choice; `SystemParametersInfo` through a PowerShell `Add-Type` P/Invoke on Windows (PowerShell has no
wallpaper cmdlet, and `SPIF_UPDATEINIFILE|SPIF_SENDWININICHANGE` is what makes the change outlive the session);
`gsettings`, or `plasma-apply-wallpaperimage` on KDE, on Linux. Zero dependency on `viewer`.

| File                           | Responsibility                                                                                                                   |
|--------------------------------|------------------------------------------------------------------------------------------------------------------------------------|
| `wallpaper.go`                 | `Set` (exported dispatcher var), unexported per-platform impls (`setLinux`, `setWindows`), `isKDE`, `fileURI`, `escapePowerShellPath` |
| `darwin.go` / `other.go`       | `setDarwin` (build-tag pair, real cgo/AppKit impl / non-darwin stub)                                                              |
| `windows.go` / `notwindows.go` | `hideConsoleWindow` (build-tag pair, real impl / no-op twin) — the fourth copy of it, see `internal/filepicker`'s note           |

Added 2026-08-16. Two things carry the weight on Linux: the KDE check comes *before* the binary lookup, because glib
is usually installed there too — a lookup-order fallback would find `gsettings`, write the GNOME schema successfully,
and leave the desktop unchanged while the app reported success, which is the one failure mode worse than an error
message. And `picture-uri-dark` is written alongside `picture-uri`, since GNOME 42 split the background into a
light/dark pair and a user in dark mode sees nothing at all change otherwise; its failure is ignored rather than
reported, because what makes it fail is an older GNOME that has no such key — where the light write was already the
whole job. `fileURI` goes through `net/url` rather than string concatenation so a `#` in a file name can't truncate
the URI into a path that isn't there.

The path handed to `Set` must stay readable for as long as it is the wallpaper: every platform here stores a
reference to the file, not a copy of its pixels. `internal/ui/wallpaper.go` is what guarantees that, by writing its
own PNG into the user's cache directory instead of pointing any of this at a file the app itself can trash.

### `internal/filesort`

The five orderings the `S` key cycles through — natural (numeric-aware)
file name, Exif capture date falling back to mtime, mtime, size, and raw scan/drop order — plus the window-title label
for each and the translation to and from `internal/preferences`'s string constants. Zero dependency on
`viewer`: it takes `fyne.URI` values as plain data and touches only the filesystem and the Exif reader.

| File          | Responsibility                                                                                                                                                  |
|---------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `filesort.go` | `Mode` + its constants, `Next` (the S-key cycle), `Order`, `Label`, `FromPref`/`PrefValue`; unexported: the natural-sort comparison and the stat/Exif sort keys |

Extracted from the root `sort.go` on 2026-08-14. It sits beside
`internal/imaging` rather than under `internal/ui` because it draws nothing and knows about no widget — which is also
what resolves the cycle note in
`internal/preferences` above. `internal/ui/sort.go` keeps only what needs the viewer's own state: the `toggleSort`/
`SetSortMode` entry points and the
`startSort`/`finishSort` background-reorder mechanism that calls `Order` off the UI goroutine. The one thing here that
isn't pure data is `Label`, which returns display text and so goes through `lang.L` — see Translations below.

### `internal/selection`

The multi-select model behind the grid overview's batch actions: a set of file indices plus the anchor a range
extension measures from. Deliberately just integers — what an index means (a position in the app's file set, not a
position in the grid) and which gesture calls which method are `internal/ui/grid`'s business. Zero dependency on
`viewer`, and no fyne import at all, which is why it sits here beside `internal/filesort` rather than under
`internal/ui`: same rule, it draws nothing and knows about no widget.

| File           | Responsibility                                                                                                                                                                                                                                                                       |
|----------------|--------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `selection.go` | `Set` + `New`/`Toggle`/`Add`/`Contains`/`Len`/`Clear`/`Replace`/`Indices`/`Anchor`, and the free function `Range`. `Toggle` moves the anchor and `Add` deliberately doesn't — that is what lets a Shift+click re-extend from a fixed end. `Indices` returns a fresh, sorted slice, since a batch action holds its target list across a background trash move |

Added 2026-08-16 with the grid's multi-select. It has **no** `Reindex`-after-delete operation on purpose: every path
that shrinks the file set clears the selection (`Overview.FilesChanged`), so there is no shifted-index case to carry.

### `internal/uitest`

Test fixtures shared across the module's test suites: synthetic images in every format the viewer reads, the temp files
and URIs to hand them over by, and swap-in stubs for the OS-level seams. Imported only from `_test.go`
files, so it never reaches a production binary. Zero dependency on
`viewer`.

| File        | Responsibility                                                                                                                                              |
|-------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `uitest.go` | `TempJPEGURI`, `WriteTempFile`, `EncodeJPEG`/`EncodePNG`/`EncodeGIF`/`EncodeAnimatedGIF`, `SVGBytes`/`TempSVGURI` (a synthetic SVG with a given viewBox, so a rasterization of it has visibly non-zero pixels), `CaptureDateJPEG`, `GPSJPEG`/`TempGPSJPEGURI` (a JPEG carrying an Exif GPS sub-IFD, for the EXIF window's map), `TruncatedPNGHeader`, `FakeURI`, `ApproxEqual` |
| `stubs.go`  | `StubChooser`, `StubSaveChooser`, `StubClipboardCopy`, `StubClipboardCopyFiles`, `StubTrashMove`, `StubWallpaperSet` — swap `filepicker.Choose`/`filepicker.ChooseSave`/`clipboard.CopyImage`/`clipboard.CopyFiles`/`trash.Move`/`wallpaper.Set` for the duration of a test. `internal/ui`'s `newTestUI` also redirects `viewer.wallpaperDir` to a `t.TempDir()`, the way it neutralizes the toast's duration: the wallpaper copy is the one file this suite produces that is meant to outlive the process |

Added 2026-08-14, replacing per-package copies of the same helpers — Go can't share unexported test helpers across
packages, and the per-feature package split needs one shared source. What deliberately did **not** move:
the wait helpers (`waitUntilLoaded`/`waitForScan`/`settleToast`/
`settleSlideshow`/`dropAndWait`) live in `internal/ui`'s own test files, because they synchronize on unexported `viewer`
channels and WaitGroups — keeping them there is what stops those sync primitives from becoming exported API. (A
`settleThumbs` used to be listed here too; it was removed 2026-08-16 as dead code — `drain`, run via
`t.Cleanup` in every test, already calls `v.grid.Settle()` at teardown, so nothing was left calling `settleThumbs`
directly.)

## Translations

Every user-visible string goes through Fyne's `lang.L`, whose argument **is** the English text and doubles as the lookup
key — so `en.json` is an identity mapping and a new string means adding the same line to every bundle in
`translations/`.

`main.go` owns the `//go:embed translations/*.json` and the one
`lang.AddTranslationsFS` call, because that loads into Fyne's process-wide bundle: every `lang.L` anywhere in the module
reads from it, wherever that call site lives. So the strings themselves live with the code that draws them —
`internal/ui`, each feature package under it, and `filesort.Label` — and only the loading is app setup.

Two tests in `main_test.go` guard the part that rots silently: that every locale covers exactly the English key set (a
new string added to `en.json`
and nowhere else is otherwise invisible until a German user meets an English word), and that `en.json` really is an
identity mapping. Both read the embedded FS, so they check what actually ships.

## Error handling

There is no logging package anywhere in this module (`grep -rl '"log"\|log/slog'` turns up nothing) — the only
error-reporting mechanism is Fyne's own `fyne.LogError`, and it is used exclusively at the app/UI-facing layer:
`main.go`, `internal/ui`'s own files (`clipboard.go`, `openfiles.go`, `save.go`), and `internal/session/session.go`
(the one lower-level package that already imports `fyne` for `fyne.App`/`storage`). Lower-level, `viewer`-independent
packages (`internal/clipboard`, `internal/imaging`, `internal/uitest`) do not import `fyne` for this and never will just
to report an error — an error a caller genuinely cannot or need not act on (a best-effort temp-file cleanup, a read
handle's `Close` after the read already succeeded) is ignored explicitly instead: `defer func() { _ =
os.Remove(path) }()`, matching the pattern already used for `_ = tmp.Close()` in `internal/imaging/save.go`. Never leave
the call bare (`defer os.Remove(path)`) — an explicit `_ =`/`_, _ =` is what tells a reader (and the IDE's
unhandled-error inspection) that the omission is deliberate, not an oversight.

This applies even to calls that are provably infallible: `bytes.Buffer`/`strings.Builder`'s `Write`/`WriteString`,
`hash.Hash.Write`, and `fmt.Fprintf`/`Fprint`/`Fprintln` into either never return a non-nil error (this is also why
`errcheck`'s own default exclude list — `github.com/kisielk/errcheck/errcheck/excludes.go` — omits them). Several test
helpers that build synthetic file headers by hand (`internal/imaging/loader_test.go`'s `encodeXPM`/
`truncatedPNGHeader`, `internal/uitest/uitest.go`'s `TruncatedPNGHeader`) still mark every such call `_, _ = ...`, with
a one-line comment stating why, purely so the code reads as an intentional choice rather than an unhandled error an IDE
inspection flags.

Use `errcheck` command to check for unhadled errors.

## Where to look for X

- "How is an image loaded/decoded/cached?" → `internal/imaging/loader.go`
- "How is image memory bounded, and where are the limits set?" → `internal/imaging/bytecache.go` (the byte-weighted LRU
  both caches use, its never-evict-the-newest rule, and the per-type weight estimates) +
  `internal/ui/memlimits.go` (the three user-facing limits and their setters) + `internal/imaging/gif.go` (the
  cumulative animation budget, and `probeGIF`, the block walk that lets it be applied before `gif.DecodeAll` rather
  than after) + `internal/imaging/loader.go`'s `MaxEncodedBytes` (the ceiling on a file's *encoded* size, enforced
  before anything is decoded)
- "Where does the EXIF panel live?" → `internal/ui/exifwin`
- "Why is the log not full of `tile fetch error`?" → `internal/ui/exifwin/tiles.go`'s `quietPendingTiles`/`tileLogFilter`
- "Why doesn't the EXIF window's map freeze the app while it loads?" → `internal/ui/exifwin/tiles.go` (a
  non-blocking, byte-bounded caching transport under the map widget, whose own fetching happens inside its raster
  draw) + `exifwin.go`'s `startWarm`/`syncLoading` (the prefetch and its spinner)
- "Why is the info overlay's 'Show EXIF data' link missing?" → `internal/ui/info.go`'s `syncInfoOverlayVisibility` (it
  is only offered for a file that has metadata) + `viewer.currentHasEXIF` + `imaging.LoadedImage`'s `HasEXIF`, filled in
  by `load.go`'s `attemptLoad`/`preloadOne`. The `E` key is deliberately *not* conditional: it still opens the panel,
  which says so itself when there's nothing to show
- "Where is a photo's GPS position read, and where is it shown?" → `internal/imaging/exif.go`'s `parseGPSIFD` +
  `internal/ui/exifwin`'s collapsible Location section and `formatExifMetadata`'s latitude/longitude lines
- "How is EXIF orientation handled?" → `internal/imaging/exif.go` + `orientation.go`
- "How does drag-and-drop / folder scanning work?" → `drop.go`'s `handleDrop`
- "How is an image shown/preloaded/animated once loaded?" → `load.go`
- "Which keys do what?" → `keys.go`'s `handleKeyEvent` (key names) and `handleTypedRune` (typed characters, grid search only) + `shortcuts.go` (application-wide modified-key shortcuts)
- "How do I find one file by name in a big drop?" → `internal/ui/grid`'s `HandleRune`/`applyFilter`/`fileIndex` (the `/`
  search, the display→host index mapping it needs, and the search bar) + `keys.go`'s `handleTypedRune` (how a typed
  character reaches it)
- "How do I act on several images at once?" → `internal/selection` (the set and its anchor) +
  `internal/ui/grid/selection.go` (the click/key gestures, the tint, `Targets`) + `internal/ui/batch.go` (the only
  place that joins the grid to `deletion` and the clipboard) + `internal/ui/deletion`'s `RequestFiles` (the batch
  prompt and the partial-failure reporting) + `internal/clipboard`'s `CopyFiles` (per-OS file references)
- "How does zoom/pan work?" → `internal/ui/zoom` (the state, the geometry, and the widget); the keys that drive it are
  in `keys.go`
- "How does an SVG stay sharp when I zoom?" → `internal/imaging/vector.go` (the retained parse
  tree and `RasterAt`) + `internal/imaging/svg.go` (the logical-size floor and the raster
  ceiling) + `internal/ui/zoom`'s `SetLogicalSize`/`native`/`onScaleChanged` (why a denser
  raster still lays out at the right scale) + `internal/ui/vector.go` (the debounced
  re-render). The logical size is what the window, the title and the info overlay are built
  on; the raster behind it changes with the zoom level
- "How does rotation work, and how is it saved to disk?" → `internal/ui/rotate.go` (the view-only R/Shift+R rotation) +
  `internal/ui/save.go` (the File > Save Changes action/Cmd+S that persists it) + `internal/imaging/save.go` (the
  per-format encoders and the atomic temp-file-then-rename write)
- "How do I write an image out in a different format?" → `internal/ui/export.go` (the File > "Export as PNG…/JPEG…"
  actions, the suggested name, and the extension rule) + `internal/filepicker`'s `ChooseSave` (the per-OS save panel) +
  `internal/imaging/save.go`'s `Export` (encode by destination extension). This is the only path out for a format
  `CanEncode` reports false for, and the only one that works on an animation
- "How does 'Set as Wallpaper' work?" → `internal/ui/wallpaper.go` (the action, the PNG copy it writes into the app's
  cache directory, and why it is a copy at all) + `internal/wallpaper/` (per-OS dispatch: AppKit, PowerShell,
  gsettings/plasma-apply-wallpaperimage)
- "How does the slideshow / picture-frame mode work?" → `internal/ui/slideshow` (the mode itself) + `slideshow.go` (the
  grid guard around it)
- "How does delete work?" → `internal/ui/deletion` (the flow, single and batch) + `internal/trash` (per-OS
  move-to-Trash) + `shortcuts.go`'s `wireDeleteShortcut` and `batch.go`'s `requestDelete` (how Shift+Delete reaches it,
  and how it picks between the file on screen and the grid's selection)
- "How are native file dialogs implemented?" → `internal/filepicker/` (per-OS open chooser and save panel) +
  `openfiles.go` and `export.go` (the `*viewer` glue for each)
- "How is the last session saved/restored?" → `internal/session/session.go` (persistence) + `session.go`
  (`restoreSession` glue)
- "How do Favorites work?" → `internal/favstore` (named-list persistence) + `internal/ui/favorites` (menu and dialogs) +
  `internal/ui/run.go` (production directory initialization) + `viewer.go`'s `OpenFiles` (existing drop/scan path)
- "Where is the File menu / Settings window?" → `internal/ui/menu.go`'s `buildMainMenu` (Open Files…/Save
  Changes/Export as…/Close
  Files/Settings…, composed with `help.Menu()`) + `internal/ui/settingswin` (the Settings window itself) + `viewer.go`'s
  `closeFiles` (what "Close Files" runs)
- "How are preferences (sort order, merge mode, slideshow interval/shuffle, folder-scan cap, window-size cap, window
  size/position) persisted?" → `internal/preferences/preferences.go` (persistence) + `internal/ui/startup.go`
  (load/default normalization/geometry restoration) + `features.go` (applying loaded feature preferences) +
  `windowtrack.go` (the size/position trackers) + `run.go` (`currentPreferences`, the shutdown save)
- "How do the Settings and EXIF windows come back where I left them?" → `internal/ui/widgets`'s `Singleton.Remember`/
  `Geometry`/`StopTracking` and `NewSizeTracker` (the mechanism, shared by both windows) + `winpos.Poll` (the position
  half) + `preferences.WindowGeometry` (the storage) + `windowtrack.go`'s `widgetGeometry`/`prefGeometry` (the
  translation), seeded by `restoreStartupGeometry` and read back in `currentPreferences`. A `Singleton` nobody calls `Remember` on
  — the manual, the About box — is unaffected and keeps no geometry at all
- "How is the window's on-screen position read back, since Fyne has no getter for it?" → `internal/winpos/` (per-OS
  native handle read + the `Tracker` that remembers the last good one + `Poll`, the background sampler that keeps a
  Tracker current) + `internal/ui/windowtrack.go`'s `startWindowPosPolling` + `internal/ui/slideshow`'s capture-restore
  around full-screen
- "How does copy-image-to-clipboard work?" → `internal/clipboard/clipboard.go` (per-OS shell-out) + `clipboard.go`
  (`*viewer` glue). For copying the *files* rather than the pixels — the grid's batch copy —
  `internal/clipboard/copyfiles.go` + `darwin.go`, with `batch.go`'s `copySelection` deciding which of the two a
  Cmd/Ctrl+C means
- "How does the grid overview / thumbnail generation work?" → `internal/imaging/thumbnail.go` (decode + downsample) +
  `internal/ui/grid` (`widget.GridWrap` wiring, bounded-concurrency requests, generation/cell-recycling guards)
- "How do I write a test that needs an image / a viewer?" → `internal/uitest` for the fixtures, `newTestViewer(t)`/
  `newTestUI(t)` + `dropAndWait` in `library_test.go` for the viewer and its wait discipline
- "How do I add or translate a user-visible string?" → wrap it in `lang.L` where it's drawn, then add the same key to
  every bundle in `translations/` — see Translations above
- "Why isn't feature X its own package?" → the ownership and cross-feature-composition rules in the `internal/ui`
  section above
- "How/where are errors reported, and when is it OK to ignore one?" → Error handling above

## Keeping this doc current

Update this file whenever the package structure changes: a new
`internal/...` package is created, files move between packages, or a package is renamed/merged/removed. Update it in the
same change, not as a follow-up — that rule is what kept it accurate across the two refactorings through the package
split, and it is the only thing that will keep it accurate as the structure continues to evolve.
