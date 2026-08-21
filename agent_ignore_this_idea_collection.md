# PicFetch — Feature Ideas (next 10)

Suggestions from a codebase review (2026-08-21), ranked by user value per
effort. They follow the app's ethos: a fast, keyboard-first *viewer* (not
an editor), cross-platform, pure Go / no cgo. Effort: S (a day-ish),
M (a few days), L (a week+).

## 1. File-into-folder culling keys (move/copy to target folders) — M

The classic photo-culling loop: step through a shoot, press a key, and the
current file is moved (or copied) into a chosen folder. Suggest `F` →
one-time folder picker per session (via `internal/filepicker`), then every
further press files silently; `Shift+F` re-picks. Batch version from the
grid's existing multi-select. Moving a file reuses the removal path
(`RemoveFile`/`RemoveFiles`) so navigation, grid, and caches stay
consistent; a toast confirms each move. This is the single biggest gap
between "viewer" and "the tool you sort a camera card with".

## 2. Pick / reject flags with grid filter and batch actions — M

`,` = pick, `.` = reject, shown as a small corner badge on the image and on
grid thumbnails. The grid's search bar grows two filter toggles (picks
only / hide rejects) on top of its existing display→host index mapping.
Batch actions close the loop: "delete rejects" hands the reject set to
`internal/ui/deletion`'s existing confirmer; "favorite picks" hands it to
`favstore`. Flags are per-session state on `appState` (persisting them is a
separate, later decision). Pairs naturally with #1.

## 3. Zoom & pan lock across navigation — S

A toggle (`L`, `[zoom-lock]` title prefix like `[merge]`) that keeps the
current zoom level and pan offset when stepping between images instead of
resetting to fit. This is how you compare sharpness across a burst: zoom
to 100% on the eyes, arrow through the set. Implementation is small:
`finishLoad` currently always calls `v.zoom.ResetToFit()`; the toggle
skips that (and the window auto-resize) and re-applies the stored
zoom/offset via the existing `internal/ui/zoom` API.

## 4. RAW support via embedded preview extraction — L

Camera RAW files (CR2/CR3, NEF, ARW, DNG…) all embed full-size JPEG
previews. Extracting those is pure-Go-feasible (TIFF/IFD walking —
`internal/imaging/exif.go` already has the walker) and turns "drop the
memory card folder" into a supported workflow without shipping a demosaic
engine. Scope guard: viewer-only — no RAW decode, no editing; the info
overlay/title marks it as "(preview)". Extends `imaging.IsSupportedImage`
and the loader; EXIF window works as-is since the metadata lives in the
same IFDs.

## 5. Transparency checkerboard + viewer background choice — S

PNG/SVG/WebP with alpha currently composite over the theme background,
which hides white-on-transparent content entirely. Offer checkerboard /
black / white / theme, cycled with `B` and remembered in preferences.
Fyne-side it's a layer in `build.go`'s content stack behind `v.img`
(a `canvas.Raster` for the checkerboard), shown only while an image is up.
Small, visible polish that every icon/asset designer notices immediately.

## 6. "Reveal in file manager" and "Copy path" — S

`Cmd/Ctrl+R` menu item + info-overlay link: open Finder / Explorer /
xdg file manager with the current file selected, plus a "Copy full path"
sibling. Shell-out per platform (`open -R`, `explorer /select,`,
`dbus/xdg-open` fallback) following the pattern `internal/wallpaper` and
`internal/clipboard` already use for OS integration, including their
test-seam/`Done`-channel discipline. Tiny feature, used constantly.

## 7. Export options: resize + strip metadata — S/M

The export prompt (`Cmd/Ctrl+E`, `widgets.ChoiceCard`) currently chooses
format only. Add two options: "max dimension" (e.g. 1600/2400/original,
reusing `imaging`'s scaling) and "strip metadata" (encode without EXIF —
for JPEG this is re-encoding minus the APP1 segment, which
`internal/imaging/save.go` already gets close to). Turns export into
"prepare this photo for the web/mail" — a privacy feature and a
convenience feature in one.

## 8. Watch-folder auto-refresh — M

An opt-in per-session toggle: when the dropped set came from a folder,
watch it (fsnotify) and merge newly appearing images into the set through
the existing scan path (`OpenFiles` → `handleDrop` in merge mode), with
the usual dedupe. Picture-frame mode is the killer use: a frame pointed at
a synced/camera-upload folder shows new photos as they arrive. Needs care
with scan lifecycles (debounce bursts; a watch event must not supersede a
user drop), which the `requestLifecycle` machinery already models well.

## 9. Startup flags for scripting and kiosk use — S

The binary already accepts paths; add `--slideshow` (start in
picture-frame mode), `--shuffle`, `--interval=8s`, `--sort=date`,
`--merge`, `--recursive-limit=n`. All of them map to existing viewer
setters that the settings window already drives, so `main.go`'s
`argsToURIs` grows a small flag-parsing sibling and `ui.Run` gets an
options struct. Makes PicFetch usable as a photo-frame appliance
(autostart on a Pi) and in shell workflows — cheap surface area for a
whole new deployment story.

## 10. Duplicate finder (perceptual hash) in the grid — L

A grid mode that computes a perceptual hash (dHash/aHash — ~30 lines of
pure Go over the already-decoded thumbnails) per file in the background
via the existing bounded thumbnail worker pool, then groups visually
identical shots and badges duplicates. Combined with #1/#2, this answers
"which of these 400 photos are the same shot twice?" — a job no quick
viewer does well today. Hash work rides the thumbnail pipeline's
claim/semaphore machinery, so it inherits cancellation and memory bounds
for free.

## Honorable mentions (not in the 10)

- ICC color-management (convert embedded profiles to sRGB on decode) —
  correct but invisible to most; revisit if wide-gamut screenshots become
  a complaint.
- Undo last delete (restore from `internal/trash`) — valuable, but
  platform trash-restore APIs are uneven; needs a design pass.
- Luminance/RGB histogram in the EXIF window — nice companion to #3 for
  exposure checks.
- Filmstrip strip along the window bottom — the grid (`G`) already covers
  most of this with less screen cost.
