# Image Drop — Architecture

This documents is for AI agents so they find their way through the project.

Image Drop is a Fyne desktop app for viewing dropped/opened images: one
binary, split from a single flat `package main` into `internal/...`
packages. This doc is a navigation map of the current structure — start
here to find anything.

## Package map

### `github.com/frathe/imagedrop` (package main)

The entry point, and nothing else: `main.go` builds the `fyne.App`, loads
the embedded translation bundles, converts command-line paths to URIs
(`argsToURIs`), and hands over to `ui.Run`. It stays at the module root so
`go build .`, the Makefile, and `fyne package` keep working unchanged.

### `internal/ui`

The application. `viewer` (unexported) holds the core state — the file
set, the current index, the image and its decode cache — and the files
below are its own code: construction, the key dispatcher, drop, load,
display. Everything that could own state independently of that core is a
subpackage, listed after this table.

`Run` is the package's only exported symbol. The `viewer` type never
leaves the package; what the subpackages see of it is a set of exported
methods on an unexported type (the "vocabulary" in `viewer.go`), each
subpackage binding only to the few it declares in its own `Host`
interface.

#### Its own files

| File(s) | Responsibility |
|---|---|
| `run.go` | `Run` — the only exported symbol: builds the window, wires the startup drop (CLI arguments) and the shutdown save of session + preferences, then hands control to Fyne's event loop. Also the app-level constants (`appTitle`, the drop-zone size floor `startW`/`startH` — the window-size *ceiling* is `defaultMaxWindowWidth`/`defaultMaxWindowHeight` in `load.go`, next to the code that enforces it) |
| `build.go` | `buildViewer` — constructs and wires the whole UI, composed from per-feature widget constructors (`newDropzoneUI`/`newScanUI`/`newInfoOverlayUI` here, `newToast` in toast.go, each returning a small widget-cluster struct/component); also the keyboard-shortcut wiring (`wireOpenShortcuts`/`wireClipboardShortcuts`/`wireDeleteShortcut`) |
| `windowtrack.go` | The two window-geometry trackers: `windowSizeTracker` (a layout that records the window's current size on every pass) and `startWindowPosPolling` (a background poller keeping `viewer.winPos` current via `internal/winpos`, skipped while the slideshow is full-screen; returns a stop func `Run` calls at shutdown) |
| `testdata/` | Golden-master screenshots for the e2e suite (moved here with the code that reads them, since a relative path can't reach a parent directory) |
| `viewer.go` | The `viewer` struct (UI state, navigation, image cache) — the core of the app — plus its small core-state methods: title handling, `clearToDropzone`/`reset`/`showWelcomeState`, `closeFiles` (the File menu's "Close Files" item — like `reset` but never closes the window), `undoGridMaximize` (undoes a grid-triggered `winpos.Maximize` — see `grid/`'s `ConsumeMaximized` — before a resize elsewhere tries to shrink the window back down), merge-mode toggle/get/set, `showFileIfPresent`, and the exported vocabulary the feature packages' `Host` interfaces bind to (`CurrentFile`, `RemoveFile`, `ShowImage`, `ShowToast`, `ShowEmptyStateError`, `ForceRepaint`, `FileCount`, `FileAt`, `CurrentIndex`, `Generation`, `Unfocus`, `Advance`) |
| `keys.go` | `handleKeyEvent` — the single keyboard dispatcher every unmodified key press runs through |
| `menu.go` | `buildMainMenu` — the window's menu bar: a File menu (Open Files…/Close Files/Settings…, built here) composed with `help.Menu()`'s Help menu. The one place that decides how the two compose, per the cross-feature-composition rule below |
| `drop.go` | Drop handling and the recursive folder scan: `handleDrop`, its shared completion step `applyScanResult`, `applyScannedFiles` (merges or replaces the file set, then reorders and displays it via `sort.go`'s `startSort` instead of sorting inline on the UI goroutine), `cancelScan`, `realPathOf`, the `maxScan` cap and its `MaxScan`/`SetMaxScan` get/set (the settings window's binding) |
| `load.go` | Loading and displaying images: `ShowImage`/`attemptLoad`/`finishLoad`/`retryAfterLoadFailure`, neighbor preloading, the GIF `animate` loop, `resizeToImage` (takes its `maxW`/`maxH` cap as parameters rather than reading a package constant, so each call site passes the viewer's own `maxWinW`/`maxWinH`) plus `defaultMaxWindowWidth`/`defaultMaxWindowHeight` and the `MaxWindowWidth`/`SetMaxWindowWidth`/`MaxWindowHeight`/`SetMaxWindowHeight` get/set pairs (the settings window's binding) |
| `toast.go` | The `toast` component - owns its widgets and a cancellable auto-hide lifecycle (atomic generation, per-show stop/done channels, injected duration) - plus the viewer's `showToast` wrapper |
| `info.go` | The persistent info overlay (I key): toggle/sync/update, `formatFileSize` |
| `sort.go` | `toggleSort` (the `S` key) and `SetSortMode`/`SortMode` (the settings window's binding — jumps directly to a mode rather than cycling, safe to call before any files are loaded), plus `startSort`/`finishSort`: the shared background-reorder mechanism (own spinner/label, staleness generation `sortGen`, completion callback) used by both `SetSortMode` and `drop.go`'s `applyScannedFiles`, so neither freezes the UI on a large stat/Exif-heavy sort — the orderings themselves live in `internal/filesort` |
| `rotate.go` | View-only 90°-step image rotation (`rotateBy`/`resetRotation`/`redrawRotatedFrame`), composed with EXIF orientation at render time - never written to disk. Stays here rather than becoming a package: it writes `img.Image` on the core load/animation path, which is the app's own side of the contract `zoom/` is written against |
| `slideshow.go` | `togglePictureFrameMode` — the five-line glue that closes the grid before handing over to `slideshow.Toggle`, i.e. the one thing the slideshow package must not know — plus `toggleSlideshowShuffle` and the `SlideShuffle`/`SetSlideShuffle`/`SlideInterval`/`SetSlideInterval` get/set pairs (the settings window's binding) |
| `session.go` | `restoreSession` — thin `*viewer` glue over `internal/session` |
| `clipboard.go` | `copyPathToClipboard`, `copyImageToClipboard`, `reportClipboardError` — thin `*viewer` glue over `internal/clipboard` |
| `openfiles.go` | `openFileDialog`/`runFileChooser`/`reportChooserError` — thin `*viewer` glue over `internal/filepicker` — plus `chooserErrorDetail`, shared with `clipboard.go` |

The last four are deliberately files rather than packages: each is a
handful of lines binding one `internal/...` package to viewer state, with
nothing of its own to own.

#### Feature packages (`internal/ui/...`)

Each owns its state and its widgets, and declares the interface it needs
from the app rather than being handed a shared one. Sizes below are that
interface, which is the honest measure of how coupled each still is.

| Package | Responsibility | Reaches back via |
|---|---|---|
| `zoom/` | The zoom/pan view of the displayed image (0/1/+/-, click-drag pan, scroll-to-zoom anchored at the pointer), and the widget that lays it out in place of the image itself | **Nothing.** It shares one `*canvas.Image` on a single-writer-per-field contract — the app owns `img.Image` (the pixels), this package owns the image's size and position — plus an `onChanged` callback and a `modifiers` accessor |
| `grid/` | The full-window thumbnail overview (`G` key): a virtualized `widget.GridWrap` over every loaded image, owning its own thumbnail cache (a separate LRU budget from `imgCache`, so neither evicts the other) and the bounded worker pool that fills it | 7-method `Host`; knows nothing about the slideshow — see below |
| `deletion/` | The Shift+Delete confirmation flow: its own card, selection state, and the `os.Remove` that follows | 6-method `Host` (`CurrentFile`, `RemoveFile`, `ShowImage`, `ShowEmptyStateError`, `ShowToast`, `ForceRepaint`) — the first of the consumer-side interfaces the split is built on |
| `slideshow/` | Picture-frame mode (`P` key): the full-screen switch, the auto-advance goroutine, the interval `Up`/`Down` tunes, and the `winpos.Tracker` capture/restore that puts the window back where the user left it | 2-method `Host` (`FileCount`, `Advance`) — the smallest in the split; knows nothing about the grid — see below |
| `exifwin/` | The EXIF metadata panel (`E` key and the info overlay's "Show EXIF data" link) over `internal/imaging`'s `ReadMetadata` | One `func() (fyne.URI, bool)` accessor — a single function is a smaller, more honest dependency than a one-method interface |
| `help/` | The manual, the About box, and the Help menu, plus the embedded `manual.md`/`manual_de.md` (`currentManual` picks by system locale - German for `de*`, English otherwise). `Menu()` returns the Help `*fyne.Menu` on its own (not a whole `*fyne.MainMenu`) so `internal/ui`'s `menu.go` can compose it with the File menu | **Nothing at all** — no interface, no callbacks: everything it draws comes from `New(app, title, art)` |
| `settingswin/` | The Settings window (File > Settings…): one `widget.Form` (sort order, picture-frame interval, folder-scan cap, max window width, max window height) plus two `widget.Check`s (merge mode, picture-frame shuffle) below it. Every control seeds from its `Host` getter and pushes a change straight back through the matching setter on its own `OnChanged` — no Save/Apply step, no draft state of its own. The three numeric entries validate via `fyne.io/fyne/v2/data/validation.NewRegexp` (positive whole numbers only) on top of each `Host` setter's own domain clamp (e.g. `SetMaxWindowWidth` flooring at the drop-zone size) | `Host`: a getter/setter pair per preference (`SortMode`/`SetSortMode`, `MergeMode`/`SetMergeMode`, `SlideShuffle`/`SetSlideShuffle`, `SlideInterval`/`SetSlideInterval`, `MaxScan`/`SetMaxScan`, `MaxWindowWidth`/`SetMaxWindowWidth`, `MaxWindowHeight`/`SetMaxWindowHeight`) |
| `widgets/` | Viewer-free UI mechanics shared across the packages above: `TappableArea` (the drop zone's tap target), `Singleton` (the raise-or-build lifecycle behind every secondary window), and the app's hardcoded style values (`CardRadius`, the dropzone/toast/scrim colors, `NewFocusRing`) | Nothing — it is a leaf |
| `assets/` | `WelcomeWebP`/`PlaceholderWebP`, the two images the UI embeds. They live beside the code that draws them because `//go:embed` cannot reach a parent directory; the root `assets/` keeps the icon and README artwork, which the build consumes rather than the program | Nothing — it is a leaf |

Concurrency invariant (established when the suite first became
`-race`-clean, phase 2 stage 2): the viewer has **no mutable package
state** - test seams that used to be package vars (`toastDuration`,
`maxScannedFiles`, `currentKeyModifiers`) are per-viewer fields now - and
every background goroutine has both a staleness guard (generation counter)
and an explicit stop/done signal: `animStop`/`animStopped` (GIF playback),
the slideshow's `Exit`/`Settle` pair, the poller's stop func, the toast's
per-show `stop`/`done` pair, `clipboardDone`, `chooserDone`, and the
grid's and viewer's `pending`/`preloadPending` WaitGroups for thumbnail and
neighbor-preload decodes. Under Fyne's test driver `fyne.Do` runs a
goroutine's callback inline rather than marshaling it to a UI thread, so
the test suite leans on those signals
(`settleToast`/`settleThumbs`/`settleSlideshow`/`settleChooser` and the
`waitFor*`/`dropAndWait` channel helpers in `library_test.go`) instead of
ever letting a background goroutine's UI work overlap the test goroutine's
own. `newTestUI`'s `drain` cleanup waits out all of them at the end of
every test, so work a test deliberately abandoned can't run on into the
next one — if you add a background operation, give it a signal and add it
there.

The two full-window modes — the grid and the slideshow — must not overlap,
and neither package imports the other: the guards live in this package's
dispatcher (`handleKeyEvent`'s `G` case checks `slides.Active()`) and in
`togglePictureFrameMode`, which closes the grid on the way in. That is the
general rule for cross-feature interaction after the split: features expose
state and actions, and `internal/ui` decides how they compose.

What is still a method on `*viewer` (defined in `viewer.go`) is the core the
split deliberately stopped at: drop, scan, load, display, navigation,
rotation, and the thin glue files above. Every feature that could own its
own state now does — the structural extractions are finished.

### `internal/imaging`

Read → probe → decode → EXIF-orient → cache pipeline for image files
(JPEG/PNG/GIF including animated, WebP, BMP, TIFF, ICO, XPM, HEIC, AVIF).
HEIC/AVIF decode via `github.com/gen2brain/{heic,avif}` (WASM/wazero, no
cgo) and apply their own orientation/transform internally, so
`readEXIFOrientation` deliberately leaves them alone. Zero dependency on
`viewer`.

| File | Responsibility |
|---|---|
| `loader.go` | `LoadedImage`, `NewImgCache`, `ReadAndProbe`, `DecodeLoaded`, `LoadImage`, `IsSupportedImage`, `InvalidDimensionsError` |
| `exif.go` | EXIF orientation-tag parsing (unexported: `readEXIFOrientation`, `parseExifOrientation`) and the general-purpose tag reader `ReadMetadata`/`Metadata` (camera make/model, lens, exposure, aperture, ISO, focal length, capture date — GPS deliberately never read); falls back to `heic`/`avif`'s own `DecodeExif` for HEIC/AVIF files, which aren't JPEG-APP1-boxed |
| `orientation.go` | Pixel-level rotate/flip transforms (`ApplyOrientation`, `RotateSteps`) |
| `gif.go` | Animated GIF frame decoding/compositing (unexported: `decodeAnimatedGIF`) |
| `thumbnail.go` | `NewThumbCache`, `LoadThumbnail`, unexported `scaleToFit` — decodes via `LoadImage` then downsamples (`golang.org/x/image/draw`, `ApproxBiLinear`) for `internal/ui/grid` |

Extracted from `library.go` + the root `orientation.go`/`exif.go`/`gif.go` on
2026-08-13 — see `legacy/2026-08-13_refactoring.md`.

### `internal/session`

Persists and restores the set of files that were open when the window last
closed, via Fyne's app-scoped cache. Zero dependency on `viewer`.

| File | Responsibility |
|---|---|
| `session.go` | `Save`, `Load`, unexported `state`/`cacheKey` |

Extracted from the root `session.go` on 2026-08-13 — see `legacy/2026-08-13_refactoring.md`.
The root `session.go` now holds only `viewer.restoreSession()`, the thin
glue that hands `v.savedSession` to `handleDrop`.

### `internal/preferences`

Persists and restores standing UI preferences (sort order, merge mode, the
picture-frame slideshow interval and shuffle order, the folder-scan cap,
the window-size cap, window size and position) across launches, via Fyne's
app-scoped `Preferences` store (`fyne.App.Preferences()` — distinct from
`internal/session`'s app *cache*, which is the transient file-set store).
Zero dependency on `viewer`.

| File | Responsibility |
|---|---|
| `preferences.go` | `Save`, `Load`, `State`, unexported preference keys |

Added 2026-08-14, mirroring `internal/session`'s shape. `internal/ui`'s
`buildViewer` loads the saved `State` to seed `sortMode`/`mergeMode`/the
slideshow interval and the initial window size and position, and `Run`'s
`SetOnStopped` saves the current values back, alongside the existing
`session.Save` call. `WindowPosX`/`WindowPosY`/`WindowPositionSet`
were added 2026-08-14 for manual-window-move persistence — see
`internal/winpos` for why reading a position back needs a whole package of
its own where reading a size back didn't. `SortMode` (added 2026-08-14 for
the multi-criteria sort feature) is a string, not an enum: it was
originally a string because the enum lived in `package main` and couldn't
be imported here without a cycle. `internal/filesort` (stage 5) removed
that constraint, but the string stays on purpose — it's the on-disk format,
and keeping it decoupled from the enum's declaration order means reordering
or renaming a mode can't silently reinterpret a saved preference.
`filesort.FromPref`/`Mode.PrefValue` are the translation. `MaxScanFiles`
and `MaxWindowWidth`/`MaxWindowHeight` were added alongside the Settings
window (`internal/ui/settingswin`) so those standing preferences persist
across launches the same way the others already did; all three use the
same zero-means-unset sentinel `WindowSize` does, since the viewer itself
never accepts a zero cap for any of them (see `internal/ui/drop.go`'s
`defaultMaxScannedFiles` and `internal/ui/load.go`'s
`defaultMaxWindowWidth`/`defaultMaxWindowHeight` fallbacks in
`buildViewer`).

### `internal/winpos`

Reads and writes a Fyne desktop window's on-screen position. Fyne's public
`Window` has no position getter and no "window moved" event at all — only
`driver/desktop.Window.RequestPosition`, which is write-only — so `Get`
reaches past that into the raw native window handle
(`driver.NativeWindow.RunNative` hands out an `NSWindow`/`HWND`/X11 handle
depending on platform) and asks the OS directly, mirroring exactly what
Fyne's own glfw driver does internally for its own position bookkeeping so
a value round-tripped through `Get` then `Set` lands back where it started.
Zero dependency on `viewer`.

| File | Responsibility |
|---|---|
| `winpos.go` | `Get`, `Set`, `Maximize`, `Unmaximize` — the platform-agnostic dispatch over `driver.NativeWindow`/`desktop.Window` |
| `tracker.go` | `Tracker` — the last *good* reading, kept in atomics (`Store`/`Get`/`Capture`/`Restore`). `Get` above answers "where is this window right now", which is unavailable or wrong at exactly the moments the position is needed: while full-screen it reports the screen corner, and at shutdown the event loop the read hops through is winding down. A `Tracker` is what the poller, the slideshow, and the shutdown save share instead |
| `darwin.go` | `platformPosition` — cgo/AppKit `NSWindow` frame read, converted out of Cocoa's bottom-left-origin coordinate space. `platformMaximize`/`platformUnmaximize` — `-zoom:`, AppKit's own toggle between the standard and user frame, each guarded to only fire when the window isn't/is already zoomed |
| `windows.go` | `platformPosition` — `ClientToScreen` via `syscall`, matching glfw's own Win32 position query (not `GetWindowRect`, which would include the non-client frame). `platformMaximize`/`platformUnmaximize` — `ShowWindow(SW_MAXIMIZE)`/`ShowWindow(SW_RESTORE)` |
| `linux.go` | `platformPosition` — cgo/Xlib `XTranslateCoordinates`; reports `ok=false` on Wayland (no such handle exists there), matching `RequestPosition`'s own documented Wayland limitation. `platformMaximize`/`platformUnmaximize` — an EWMH `_NET_WM_STATE` `ClientMessage` adding/removing both maximized states, X11-only for the same reason |
| `other.go` | `platformPosition` — always `ok=false`, for BSD/mobile/wasm/anything else. `platformMaximize`/`platformUnmaximize` — no-ops |

Added 2026-08-14 for the "restore the window where the user manually left
it" feature: `internal/ui`'s `buildViewer` seeds `viewer.winPos` from the
saved preference and applies it to the window, and starts
`startWindowPosPolling` (`windowtrack.go`), a background goroutine that
keeps the tracker current via `Capture` since — unlike a resize — a pure
window-drag triggers no layout pass for `windowSizeTracker` to piggyback
on. The poller only ever runs against a real `driver.NativeWindow` (checked
once up front), so the fyne test driver's windows — every test in
`internal/ui`, via `buildViewer`/`newTestViewer` — never get a poller
goroutine at all. `internal/ui/slideshow` captures and restores the same
tracker around full-screen, so leaving picture-frame mode puts the window
back at the manually-placed position instead of wherever the OS chose to
un-full-screen it to. The `Tracker` (added 2026-08-14, stage 8) is what
gives those three consumers one place to share that state, rather than a
set of atomics on the viewer that each of them reached into.

### `internal/clipboard`

Puts PNG-encoded image data onto the system clipboard as real image data,
via a per-OS shell-out (AppleScript on macOS, xclip/wl-copy on Linux,
PowerShell on Windows). Zero dependency on `viewer`.

| File | Responsibility |
|---|---|
| `clipboard.go` | `CopyImage` (exported dispatcher var), unexported per-platform impls (`copyImageDarwin`, `copyImageLinux`, `copyImageWindows`), `writeTempPNG` |
| `windows.go` / `notwindows.go` | `hideConsoleWindow` (build-tag pair, real impl / no-op twin) |

Extracted from the root `clipboard.go` on 2026-08-13 — see `legacy/2026-08-13_refactoring.md`.
The root `clipboard.go` now holds only the `*viewer` glue
(`copyPathToClipboard`, `copyImageToClipboard`, `reportClipboardError`) that
encodes the current frame and calls `clipboard.CopyImage`.

### `internal/filepicker`

Opens the current OS's own file browser and returns the paths picked:
zenity on Linux, a WinForms dialog via PowerShell on Windows, in-process
cgo/AppKit `NSOpenPanel` on macOS. Linux and macOS can pick folders too;
Windows is files only (its shell dialog has no mode that combines folder
and multi-file selection - folders there go through drag-and-drop instead).
Zero dependency on `viewer`.

| File | Responsibility |
|---|---|
| `filepicker.go` | `Choose` (exported dispatcher var), `ParseFileList` (exported), unexported per-platform impls (`chooseFilesLinux`, `chooseFilesWindows`, `buildPowerShellCmd`, `powerShellEscape`) |
| `darwin.go` / `other.go` | `chooseFilesDarwin` (build-tag pair, real cgo/AppKit impl / non-darwin stub) |
| `windows.go` / `notwindows.go` | `hideConsoleWindow` (build-tag pair, real impl / no-op twin) |

Extracted from the root `openfiles.go` + `openfiles_{darwin,windows,other,
notwindows}.go` (all four deleted from root) on 2026-08-13 — see
`legacy/2026-08-13_refactoring.md`. The root `openfiles.go` now holds only the `tappableArea`
widget and the `*viewer` glue (`openFileDialog`, `runFileChooser`,
`reportChooserError`) that calls `filepicker.Choose`/`filepicker.ParseFileList`.

Note: `hideConsoleWindow` now exists as three separate build-tag-pair
copies — here, in `internal/clipboard`, and nowhere else (the root
`openfiles_windows.go`/`openfiles_notwindows.go` that used to hold a third
copy were deleted, since `main` no longer calls it directly). Each copy is
~10 lines and unexported, so duplicating it per package beat introducing a
shared package for one tiny OS-quirk helper.

### `internal/filesort`

The five orderings the `S` key cycles through — natural (numeric-aware)
file name, Exif capture date falling back to mtime, mtime, size, and raw
scan/drop order — plus the window-title label for each and the translation
to and from `internal/preferences`'s string constants. Zero dependency on
`viewer`: it takes `fyne.URI` values as plain data and touches only the
filesystem and the Exif reader.

| File | Responsibility |
|---|---|
| `filesort.go` | `Mode` + its constants, `Next` (the S-key cycle), `Order`, `Label`, `FromPref`/`PrefValue`; unexported: the natural-sort comparison and the stat/Exif sort keys |

Extracted from the root `sort.go` on 2026-08-14. It sits beside
`internal/imaging` rather than under `internal/ui` because it draws nothing
and knows about no widget — which is also what resolves the cycle note in
`internal/preferences` above. `internal/ui/sort.go` keeps only what needs the
viewer's own state: the `toggleSort`/`SetSortMode` entry points and the
`startSort`/`finishSort` background-reorder mechanism that calls `Order` off
the UI goroutine. The one thing here that isn't pure data is `Label`, which returns
display text and so goes through `lang.L` — see Translations below.

### `internal/uitest`

Test fixtures shared across the module's test suites: synthetic images in
every format the viewer reads, the temp files and URIs to hand them over
by, and swap-in stubs for the OS-level seams. Imported only from `_test.go`
files, so it never reaches a production binary. Zero dependency on
`viewer`.

| File | Responsibility |
|---|---|
| `uitest.go` | `TempJPEGURI`, `WriteTempFile`, `EncodeJPEG`/`EncodePNG`/`EncodeGIF`/`EncodeAnimatedGIF`, `CaptureDateJPEG`, `TruncatedPNGHeader`, `FakeURI`, `ApproxEqual` |
| `stubs.go` | `StubChooser`, `StubClipboardCopy` — swap `filepicker.Choose`/`clipboard.CopyImage` for the duration of a test |

Added 2026-08-14, replacing per-package copies of the same helpers — Go
can't share unexported test helpers across packages, and the per-feature
package split needs one shared source. What deliberately did **not** move:
the wait helpers (`waitUntilLoaded`/`waitForScan`/`settleToast`/
`settleThumbs`/`settleSlideshow`/`dropAndWait`) live in `internal/ui`'s own
test files, because they synchronize on unexported `viewer` channels and
WaitGroups — keeping them there is what stops those sync primitives from
becoming exported API.

## Translations

Every user-visible string goes through Fyne's `lang.L`, whose argument
**is** the English text and doubles as the lookup key — so `en.json` is an
identity mapping and a new string means adding the same line to every
bundle in `translations/`.

`main.go` owns the `//go:embed translations/*.json` and the one
`lang.AddTranslationsFS` call, because that loads into Fyne's process-wide
bundle: every `lang.L` anywhere in the module reads from it, wherever that
call site lives. So the strings themselves live with the code that draws
them — `internal/ui`, each feature package under it, and `filesort.Label` —
and only the loading is app setup.

Two tests in `main_test.go` guard the part that rots silently: that every
locale covers exactly the English key set (a new string added to `en.json`
and nowhere else is otherwise invisible until a German user meets an
English word), and that `en.json` really is an identity mapping. Both read
the embedded FS, so they check what actually ships.

## Where to look for X

- "How is an image loaded/decoded/cached?" → `internal/imaging/loader.go`
- "Where does the EXIF panel live?" → `internal/ui/exifwin`
- "How is EXIF orientation handled?" → `internal/imaging/exif.go` + `orientation.go`
- "How does drag-and-drop / folder scanning work?" → `drop.go`'s `handleDrop`
- "How is an image shown/preloaded/animated once loaded?" → `load.go`
- "Which keys do what?" → `keys.go`'s `handleKeyEvent`
- "How does zoom/pan work?" → `internal/ui/zoom` (the state, the geometry, and the widget); the keys that drive it are in `keys.go`
- "How does the slideshow / picture-frame mode work?" → `internal/ui/slideshow` (the mode itself) + `slideshow.go` (the grid guard around it)
- "How does delete work?" → `internal/ui/deletion` (the flow) + `build.go`'s wireDeleteShortcut (how Shift+Delete reaches it)
- "How are native file-open dialogs implemented?" → `internal/filepicker/` (per-OS chooser) + `openfiles.go` (`*viewer` glue)
- "How is the last session saved/restored?" → `internal/session/session.go` (persistence) + `session.go` (`restoreSession` glue)
- "Where is the File menu / Settings window?" → `internal/ui/menu.go`'s `buildMainMenu` (Open Files…/Close Files/Settings…, composed with `help.Menu()`) + `internal/ui/settingswin` (the Settings window itself) + `viewer.go`'s `closeFiles` (what "Close Files" runs)
- "How are preferences (sort order, merge mode, slideshow interval/shuffle, folder-scan cap, window-size cap, window size/position) persisted?" → `internal/preferences/preferences.go` (persistence) + `internal/ui`'s `build.go` (`buildViewer`) and `windowtrack.go` (the size/position trackers) and `run.go` (the shutdown save)
- "How is the window's on-screen position read back, since Fyne has no getter for it?" → `internal/winpos/` (per-OS native handle read + the `Tracker` that remembers the last good one) + `internal/ui/windowtrack.go`'s `startWindowPosPolling` + `internal/ui/slideshow`'s capture-restore around full-screen
- "How does copy-image-to-clipboard work?" → `internal/clipboard/clipboard.go` (per-OS shell-out) + `clipboard.go` (`*viewer` glue)
- "How does the grid overview / thumbnail generation work?" → `internal/imaging/thumbnail.go` (decode + downsample) + `internal/ui/grid` (`widget.GridWrap` wiring, bounded-concurrency requests, generation/cell-recycling guards)
- "How do I write a test that needs an image / a viewer?" → `internal/uitest` for the fixtures, `newTestViewer(t)`/`newTestUI(t)` + `dropAndWait` in `library_test.go` for the viewer and its wait discipline
- "How do I add or translate a user-visible string?" → wrap it in `lang.L` where it's drawn, then add the same key to every bundle in `translations/` — see Translations above
- "Why isn't feature X its own package?" → `legacy/2026-08-14_refactoring.md`, which records what was deliberately left in `internal/ui` and why

## Keeping this doc current

Update this file whenever the package structure changes: a new
`internal/...` package is created, files move between packages, or a
package is renamed/merged/removed. Update it in the same change, not as a
follow-up — that rule is what kept it accurate across the two refactorings
in `legacy/`, and it is the only thing that will keep it accurate now that
they are finished and nothing else is tracking the structure.
