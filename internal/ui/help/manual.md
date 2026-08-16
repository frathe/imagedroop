# Image Drop — User Manual

Version 0.1.3

Image Drop is a small, fast image viewer for macOS, Windows, and Linux. There
is no toolbar and no built-in file browser: you drop images onto the window
and look at them. That's the whole idea — though if you'd rather not
drag-and-drop, clicking the window or pressing `Cmd`/`Ctrl+O` brings up your
system's own file picker instead (see below).

---

## 1. Getting started

1. Start **Image Drop**. A small empty window (about 520 × 340) appears with a
   rounded border and the text **"Drop images here"**.
2. Drag one or more image files from your file manager (Finder, Explorer,
   Nautilus, …) onto the window and release.
3. The first image is displayed and the window resizes itself to fit it.

You can drop anywhere on the window — the bordered area is a visual hint, not
a target you have to hit precisely.

**No mouse handy for drag-and-drop, or just prefer a file picker?** Click
anywhere on the drop zone, or press `Cmd`/`Ctrl+O` (`Cmd`/`Ctrl+Shift+O` does
the same thing — it's a second binding for the same picker, not a different
one), to bring up your system's own file browser. On macOS and Linux it lets
you select any mix of files and folders at once, the same as a drag-and-drop;
folders are scanned the same way too (see "Scanning folders" below). **On
Windows the picker is files only** — Windows' own file dialog has no mode
that combines folder and multi-file selection — so drag-and-drop is the way
to add a folder there. This works at any time, not just on the empty drop
screen — opening more images while one is already showing replaces the
current set, or adds to it if merge mode (`M`) is on, exactly like dropping
new files would.

---

## 2. Opening this manual

You are reading it, so you have probably already found it, but for the record:

- Press **`F1`** at any time, or
- choose **Help -> Manual** from the menu.

The manual opens in its own scrollable window. Press **`Esc`** or close the
window to dismiss it; the image you were viewing is untouched. Pressing `F1`
again brings the existing manual window back to the front instead of opening a
second copy.

---

## 3. Supported file types

- **JPEG** — `.jpg`, `.jpeg`, `.jpe`, `.jfif` (EXIF rotation applied)
- **PNG** — `.png` (transparency supported)
- **GIF** — `.gif` (animated GIFs play back)
- **WebP** — `.webp` (static images)
- **BMP** — `.bmp`
- **TIFF** — `.tif`, `.tiff`
- **ICO** — `.ico` (Windows icon; the largest embedded image is shown)
- **XPM** — `.xpm` (X Pixmap)
- **HEIC/HEIF** — `.heic`, `.heif` (iPhone photos; EXIF rotation applied)
- **AVIF** — `.avif` (built-in rotation/mirroring applied)

A file is also accepted if your system reports it as `image/jpeg`,
`image/png`, `image/gif`, `image/webp`, `image/bmp`, `image/tiff`,
`image/x-icon`, `image/vnd.microsoft.icon`, `image/x-xpixmap`, `image/heic`,
`image/heif`, or `image/avif`, even when the extension is missing or
unusual.

Everything else — PDFs, RAW camera files, SVG, videos — is **not** supported.

---

## 4. Viewing images

### Automatic window sizing

Each time an image is shown, the window resizes to match it:

- **Large images** are scaled down to fit within **1500 × 950** pixels, keeping
  the aspect ratio.
- **Small images** never shrink the window below the starting size
  (520 × 340). A tiny thumbnail is centred with empty space around it rather
  than producing a window too small to grab.
- You can always resize the window yourself afterwards by dragging its edge.
  The image is scaled to fit and is never cropped or distorted.

### The window title

The title bar tells you what you are looking at, for example:

`sunset.jpg — 4032 x 3024  (2/7)`

- **File name** of the current image
- **Pixel dimensions** of the image (after any rotation correction)
- **`(animated)`** if it is an animated GIF
- **`(2/7)`** — position in the current set, shown only when you dropped more
  than one image
- **`[merge]`** at the very front, only while merge mode (`M`) is on — see
  "Browsing multiple images" below

### Photo rotation

Photos taken with a phone or camera held sideways carry an EXIF orientation
tag. Image Drop reads that tag and rotates or flips the photo automatically,
so portrait shots appear upright instead of lying on their side. All eight
EXIF orientations are handled, and the dimensions in the title reflect the
corrected image.

If a photo still isn't the way up you want — EXIF-corrected but sideways for
some other reason, or you just want to look at it rotated — press **`R`** to
rotate it a further 90° clockwise, or **`Shift+R`** for counter-clockwise.
This is a viewing option by default: it resets back to upright with **`0`**
(along with zoom) and — like navigating to a different image — starts over at
upright on the next picture you view. To keep it, use **File -> Save
Changes** (`Cmd/Ctrl+S`) to write it back into the file, or **File -> Export
as PNG…/JPEG…** to write a rotated copy elsewhere; see "Menu" below.
Rotating swaps the window between landscape and portrait to fit the image's
new orientation, the same as loading a different image would.

### Animated GIFs

Animated GIFs start playing as soon as they are shown:

- Frames run at the speed stored in the file.
- Frames that only update part of the picture are composited correctly, so you
  won't see leftover pixels or flickering blanks.
- Frames with no delay (or a zero delay) are shown for 0.1 s so playback stays
  watchable.
- The animation loops until you navigate away or drop new files, at which
  point it stops on its own.

---

## 5. Zoom and pan

By default an image is always scaled to **fit the window**, exactly as
described above. Four keys switch to a manual zoom level:

- **`+`** — zoom in
- **`-`** — zoom out
- **`1`** — jump straight to **100%** (one image pixel per screen pixel)
- **`0`** — back to fit-to-window

The first `+` or `-` press zooms in or out from whatever fit is currently
showing, rather than jumping to 100% first, so zooming feels continuous.
Repeated presses keep scaling up or down, clamped between 5% and 1600%.

**Scrolling** the mouse wheel or trackpad over the image zooms too, and
unlike the keyboard shortcuts it zooms around the point under your cursor
rather than the image centre, so whatever you're pointed at stays put on
screen as you zoom in or out.

Once the image is zoomed in far enough that it no longer fits in the
window, the cursor changes to a hand over it to show it can be dragged;
**click and drag** to pan around it — the drag is clamped so you can't
pull the image away from the window and leave empty space behind it.
While fitted to the window, or zoomed to a level that still fits inside
it, panning does nothing and the cursor stays the plain arrow.

Holding **Shift** while scrolling pans instead of zooming, in whichever
direction you scroll — handy for a trackpad two-finger swipe once you're
zoomed in, without having to click and drag.

Zoom and pan are per-image: navigating to another picture (with the arrow
keys, `Home`/`End`, or a fresh drop) always starts that image back at
fit-to-window. View rotation (see "Photo rotation" above) resets the same
way, and rotating also resets zoom back to fit — a manual zoom level chosen
before the turn rarely still makes sense once the image's landscape/portrait
axes have swapped.

---

## 6. Info overlay

Press **`I`** to show a small card in the top-left corner of the window with
everything about the current image at a glance:

- the **file name**, and its position in the set (e.g. `3 / 47`) if you
  dropped more than one image
- its **pixel dimensions** (e.g. `1920 x 1080`)
- its **file size** on disk
- the current **zoom level**, as a percentage

It updates live as you navigate or change zoom, and — unlike a toast message
— stays up on screen until you press `I` again to turn it off. It's a
standing preference like sort order or merge mode: once on, it stays on
across navigation and further drops, reappearing as soon as the next image
loads even if you briefly return to the empty drop screen in between.

Below that summary, a **"Show EXIF data"** link opens a separate window with
the current image's Exif metadata — camera make and model, lens, exposure
time, aperture, ISO, focal length, and capture date, one line per tag that's
actually present in the file. `E` opens the same window directly, without
needing the info overlay open first. Location data (GPS coordinates) is
deliberately never read or shown. The window updates if you navigate to a
different image while it's still open, and — like the manual and About
windows — `Esc` closes just that window, and pressing `E` again while it's
already open brings it back to the front instead of opening a second copy.
Files with no Exif data (most PNGs, GIFs, and WebPs, and any JPEG without a
camera-written Exif segment) show a "no metadata found" message instead of
an empty window.

---

## 7. Browsing multiple images

Drop several files at once and step through them with the keyboard:

- **`→`** or **`↓`** — next image
- **`←`** or **`↑`** — previous image
- **`Home`** — jump to the first image
- **`End`** — jump to the last image

Navigation **wraps around**: pressing `→` on the last image returns to the
first, and `←` on the first goes to the last.

Notes:

- The arrow keys only do something when you dropped **two or more** images.
- By default the set is **naturally sorted** by file name, so `IMG_2.jpg`
  comes before `IMG_10.jpg` even though a plain text sort would put them the
  other way round. Press **`S`** to cycle through four more orderings, and
  back to name:
  - **Capture date** — the photo's Exif date/time (the same value the Exif
    window shows as "Date taken"); a file with no Exif capture date - a
    screenshot, most PNGs/GIFs/WebPs, or a JPEG a camera never tagged -
    falls back to its filesystem modification time instead of clumping at
    the very start of the list.
  - **Modified** — filesystem modification time.
  - **Size** — file size, smallest first.
  - **Unsorted** — the raw order the files were handed over by your file
    manager ("stupid sort" — no sorting at all).

  The title bar shows which mode is active (`[sort: date]`,
  `[sort: modified]`, `[sort: size]`, `[unsorted]`) - nothing is shown for
  the default name sort. The image you're currently looking at stays on
  screen across every switch. Sorting never removes duplicate files, and the
  preference carries over to the next drop, and across restarts, until you
  change it again.

  Capture date, modified, and size all have to read every file once to sort
  by it - a stat for modified/size, a raw file read for capture date -
  which can pause noticeably on a very large recursive drop, with no
  progress indicator or way to cancel it once started.
- Dropping new files **replaces** the current set and starts again at the
  first image just dropped, unless **merge mode** is on. Press **`M`** to
  turn merge mode on or off; while it's on, the title bar starts with
  **`[merge]`** so you can always tell which mode you're in. With merge
  mode on, a new drop **adds** its files to the current set instead of
  replacing it — display jumps to the first file just added, sorting still
  applies, and nothing is deduplicated, so dropping the same file twice
  adds it twice. If a merge-mode drop contains nothing supported, the
  existing set is left exactly as it was and you just get an error toast,
  not a wipe. Merge mode is a standing preference, like sort order — it
  stays on (or off) across drops until you press `M` again, so you don't
  have to hold anything down mid-drag.

---

## 8. Grid overview

Press **`G`** to switch to a full-window grid of thumbnails for the current
set — handy for finding one particular image in a large drop by sight
instead of arrowing through them one at a time.

- Click any thumbnail to jump straight to it and return to the normal view,
  or use the keyboard: the arrow keys move a highlighted ring around the
  grid (starting on whichever image was on screen when you opened it), and
  **`Return`** opens whichever thumbnail is currently highlighted.
- Press **`G`** again, or **`Esc`**, to leave the grid without picking
  anything.
- Press **`/`** to search by file name: a bar appears across the top, and
  what you type filters the grid down to the names containing it as you go.
  Matching ignores upper and lower case, and the bar keeps a count of how
  much of the set is left (`3 of 847`). Backspace deletes a character, the
  arrow keys and `Return` work on the matches exactly as they do on the
  full grid, and **`Esc`** clears the search so every image is shown
  again — a second `Esc` then leaves the grid as usual.
- **Select several at once** to act on them together:
  **`Cmd/Ctrl+click`** a thumbnail to add it to the selection (or click it
  again to take it back out), **`Shift+click`** to select everything
  between it and the last one you clicked, **`Space`** to pick whichever
  thumbnail is highlighted, and **`Cmd/Ctrl+A`** to select the lot.
  Selected thumbnails are washed in the accent colour, and the top bar
  counts them (`12 selected`). A plain click still just opens an image, so
  nothing changes until you hold a modifier.
- With a selection made, **`Shift+Delete`** moves all of it to the Trash
  (after the usual confirmation, which names the count rather than every
  file), and **`Cmd/Ctrl+C`** copies the files themselves to the clipboard
  — paste in Finder, Explorer or your file manager to get copies of them.
  Both work on the highlighted thumbnail alone if you haven't selected
  anything, and the grid stays open afterwards so you keep your place.
- Because the selection is a set of *files*, narrowing with `/` and then
  pressing `Cmd/Ctrl+A` selects exactly the matches — `/holiday`,
  `Cmd/Ctrl+A`, `Shift+Delete` clears every holiday photo out of a folder
  of thousands. Clearing the search afterwards leaves the selection intact.
- **`Esc`** undoes one thing at a time: the selection first, then the
  search, then the grid itself. `G` closes the grid as usual, but goes
  quiet while a selection or a search is still up, so it can't throw away
  work in progress.
- Apart from those, every other key is ignored while the grid is up - zoom,
  `S`/`M`/`P`/`I` all do nothing until you either pick a thumbnail (click
  or `Return`) or back out with `G`/`Esc`. While a search is open, the
  letter keys are characters you are typing, so `G` no longer closes the
  grid - `Esc` does.
- The search only narrows what the grid shows. It changes nothing about the
  set itself: pick an image and the arrow keys still walk through every
  file you dropped, and the next time you open the grid it starts
  unfiltered, with nothing selected.
- Thumbnails are generated in the background as they scroll into view, a
  few at a time, so opening the grid on a folder with thousands of images
  doesn't stall the window waiting for all of them to decode up front.
- The grid needs at least one image loaded, and doesn't combine with
  picture-frame mode - opening one closes the other.

---

## 9. Picture-frame mode

Press **`P`** to turn the current image set into a full-screen slideshow —
handy for letting Image Drop just sit there and cycle through a folder of
photos like a digital picture frame.

- The window goes **full-screen**. The image is scaled to fit the screen,
  keeping its aspect ratio — never stretched or cropped, the same
  scale-to-fit behavior as the normal window.
- Every **10 seconds** (by default) the view **automatically advances** to
  the next image, wrapping around at the end just like manual navigation.
  Each transition **crossfades** — the outgoing image fades to nothing, the
  incoming one fades in — instead of the instant swap ordinary browsing
  uses. Manual navigation (`←`/`→`/`Home`/`End`) crossfades the same way
  while picture-frame mode is on.
- **`↑`** increases the interval by one second, **`↓`** decreases it (down to
  a floor of one second). While picture-frame mode is on, `↑`/`↓` control the
  timer instead of navigating — use **`←`**/**`→`** (or `Home`/`End`) to
  navigate manually, which still works as usual and restarts the countdown
  from the new image.
- **`Shift+P`** turns **shuffle** on or off: with it on, auto-advance picks a
  random other image each time instead of the next one in order (never the
  one already on screen), and the title bar starts with **`[shuffle]`**.
  Manual navigation with `←`/`→`/`Home`/`End` is unaffected either way — it
  always steps through the set in order. Shuffle works as a standing
  preference like merge mode: `Shift+P` works even before you ever turn
  picture-frame mode on, and outside it too.
- **Animated GIFs are always let finish.** If a GIF's full loop takes longer
  than the current interval, picture-frame mode waits for it to play through
  at least once before moving on, rather than cutting it off partway.
- Your chosen interval and shuffle setting are remembered the next time you
  turn picture-frame mode on — and carry over the next time you launch
  Image Drop, too.
- Press **`P`** again, or **`Esc`**, to leave picture-frame mode and return to
  the normal window. `Esc` only leaves picture-frame mode here — it doesn't
  also clear the loaded images; press it again afterwards for that.
- Picture-frame mode needs at least one image loaded — pressing `P` on the
  empty drop screen does nothing.

---

## 10. Deleting a file

Press **`Shift+Delete`** to move the file currently on screen to the OS
trash/recycle bin. A confirmation card appears with two buttons:

- **Cancel** — selected by default
- **Move to Trash** (in red)

Whichever button is currently selected is outlined, so what `Return` will
press is always visible before you press it.

You can respond either way:

- **With the mouse**: click either button directly.
- **With the keyboard**: press **`→`** to move the selection onto
  "Move to Trash" (**`←`** moves it back to Cancel) — the outline moves
  with it — then **`Return`** to go with whichever one is currently
  selected. **`Esc`** cancels immediately from either selection.

While the card is up, every other key is ignored — navigation, zoom, `S`/`M`/
`P`/`I`/`G` all do nothing until you respond to the prompt one way or the
other.

Deleting the current file removes it from the set and shows whatever now
takes its place, wrapping around the same way normal navigation does; if it
was the only file left, you're returned to the empty drop screen. If the
file turns out to already be gone, or can't be deleted for some other reason
(permissions, for example), a toast explains what went wrong and the file
stays in the set.

Pressed while the grid overview is showing, `Shift+Delete` asks about
everything selected there instead (see above). The card appears over the
grid, and the grid stays open once you've answered. Anything the system
refuses to move stays both on disk and in the set, and the toast says how
many of them actually went.

---

## 11. Keyboard shortcuts

- **`F1`** — open this manual
- **`Cmd`/`Ctrl+O`** / **`Cmd`/`Ctrl+Shift+O`** — open the system file picker
  (same as clicking the drop zone; both bindings do the same thing; files and
  folders on macOS/Linux, files only on Windows — see above)
- **`→`** / **`↓`** — next image
- **`←`** / **`↑`** — previous image
- **`Home`** / **`End`** — first / last image
- **`S`** — cycle sort order: name -> capture date -> modified -> size ->
  unsorted -> back to name
- **`M`** — toggle merge mode (next drop adds to the set instead of
  replacing it); shown in the title bar as a **`[merge]`** prefix
- **`G`** — toggle the grid overview (see above); arrow keys move the
  highlight, `Return` or a click opens it, `G`/`Esc` backs out
- **`/`** — (grid only) search the grid by file name; `Esc` clears the
  search, a second `Esc` leaves the grid
- **`Space`** — (grid only) add the highlighted thumbnail to the selection,
  or take it back out
- **`Cmd`/`Ctrl+A`** — (grid only) select every thumbnail the grid is
  currently showing (just the matches, while a search narrows it)
- **`Cmd`/`Ctrl+click`** / **`Shift+click`** — (grid only) add one
  thumbnail to the selection / select the whole range up to it
- **`+`** / **`-`** — zoom in / out
- **`1`** — zoom to 100%; **`0`** — back to fit-to-window (and un-rotates, see
  below)
- scroll (mouse wheel or trackpad) — zoom in/out, anchored at the cursor
- **Shift** + scroll — pan instead of zoom
- click and drag — pan around a zoomed-in image
- **`R`** / **`Shift+R`** — rotate the displayed image 90° clockwise /
  counter-clockwise (view-only; resets on `0` or the next image)
- **`I`** — toggle the info overlay (file name, position, dimensions, file
  size, zoom level)
- **`E`** — open the EXIF data window for the current image (camera
  make/model, lens, exposure, aperture, ISO, focal length, capture date);
  also reachable via the **"Show EXIF data"** link in the info overlay
- **`Cmd`/`Ctrl+C`** — copy the current image to the system clipboard, as
  image data you can paste into another app (not a file). In the grid
  overview it copies the selected *files* instead, so a paste in your file
  manager creates copies of them
- **`Cmd`/`Ctrl+Shift+C`** — copy the current image's file path to the
  clipboard
- **`Shift+Delete`** — move the current file to the Trash, after confirming
  (see "Deleting a file" above); in the grid overview, everything selected
  there
- **`P`** — toggle picture-frame mode (full-screen slideshow with a
  crossfade between images, see above)
- **`Shift+P`** — toggle shuffle order for picture-frame mode's
  auto-advance; shown in the title bar as a **`[shuffle]`** prefix
- **`↑`** / **`↓`** *(while in picture-frame mode)* — increase / decrease the
  auto-advance interval by one second
- **`Esc`** — clear the current images and return to the initial drop screen;
  quits the app if there's nothing loaded to clear (in the manual window it
  only closes the manual; in picture-frame mode it leaves picture-frame mode
  first); while a folder scan is still running, it cancels the scan instead
  (see "Scanning folders" below)

**Linux clipboard support.** Copying the image itself (`Ctrl+C`) shells out to
an external tool, since Linux has no single built-in way to put image data on
the clipboard: `xclip`, or `wl-copy` (from the `wl-clipboard` package) as a
fallback for a Wayland session with no XWayland. Most distros don't install
either by default — add one with your package manager, e.g.
`sudo apt install xclip` or `sudo apt install wl-clipboard` on Debian/Ubuntu.
Without one, `Ctrl+C` shows an error toast instead of copying. Copying the
file path (`Ctrl+Shift+C`) is plain text and always works, no extra tool
needed. macOS and Windows need nothing extra either way.

---

## 12. Menu

- **File -> Open Files…** — opens the system file browser, same as
  `Cmd/Ctrl+O`
- **File -> Save Changes** (`Cmd/Ctrl+S`) — writes a rotation you made with
  `R`/`Shift+R` back into the file it came from, in that file's own format.
  Greyed out unless there is a rotation to save; unavailable for animations
  and for formats Image Drop can read but not write (WebP, HEIC, ICO, XPM).
  This replaces the original file, and re-encodes rather than patching it,
  so EXIF metadata is not carried over
- **File -> Export as PNG… / Export as JPEG…** — saves the image as it
  currently appears, rotation included, to a new file you name. Unlike Save
  Changes it works for every image the app can display, including WebP and
  HEIC files and a single frame of an animated GIF, and it never touches the
  original. If the name you type already ends in a format Image Drop can
  write, that format wins over the menu item you picked
- **File -> Close Files** — returns to the drop zone without quitting
- **File -> Settings…** — opens the settings window
- **Help -> Manual** — opens this manual, same as `F1`

---

## 13. Loading feedback

Decoding happens in the background, so the window stays responsive even with
very large files.

- An animated **progress bar** appears along the top edge of the window while
  an image is being loaded, and disappears when it is ready. It is drawn over
  the picture and never shifts anything on screen.
- On the very first drop, the hint text changes to **"Loading…"**.
- On later navigations the previous image stays on screen until the new one is
  ready, so there is no blank flash between pictures.
- Key presses made while an image is still loading are ignored. Holding down
  an arrow key therefore won't queue up a backlog of decodes for images you
  have already skipped past.
- If a slow image finishes decoding after you have already moved on, its
  result is discarded — you always see the image you selected last.

**Scanning folders.** If your drop includes any folders, Image Drop first
walks them (and every subfolder) to collect supported images before showing
anything:

- A spinner appears together with a running count, e.g. **"Scanning… 42
  images"**, updated as more images are found.
- Once the scan finishes, the spinner disappears and the first image found is
  shown, just like a normal drop.
- Drops with no folders skip this step entirely and load immediately.
- Press **`Esc`** at any point while the spinner is up to cancel the scan. If
  this was the very first drop, you're returned to the initial drop screen
  exactly as if nothing had been dropped; if you were merging into an
  already-loaded set, the images you had before the scan started are left
  untouched.

---

## 14. Messages and error handling

Image Drop shows a dialog box in these cases. Dismiss it with its **OK**
button.

- **One unsupported file dropped** — *"…" is not a supported image file*
- **Several files, none of them supported** — *none of the N dropped files is a
  supported image*
- **File cannot be read or decoded** — *could not read "…"*
- **File decodes to a zero-size image** — *invalid image dimensions for "…"*

**Mixed drops are handled quietly.** If you drop a batch of photos plus a few
text files, the supported images are shown and the rest are skipped without
any dialog. Only a drop containing *no* usable image produces an error.

**A file that fails to decode is dropped from the set.** If a file turns
out to be unreadable or corrupt only once you navigate to it (an
extension-based check at drop time can't catch everything), it's removed
from the set and the next one is shown automatically, wrapping around if it
was the last file — you're never left staring at a broken file with the
title bar and position count out of sync with what's actually on screen.
You just get a toast naming the file that was skipped. If every file in the
set turns out to be broken, you land back on the initial drop screen.

**Dropped folders are expanded.** Dropping a folder scans it and every
subfolder inside it for supported images; you can drop any mix of loose image
files and folders in one go. See "Scanning folders" below for what you see
while a folder is being scanned.

---

## 15. Language

The interface text (the drop hint, "Loading…", the menu) can be translated.
Image Drop ships with English and follows your system language when a matching
translation is available; otherwise it falls back to English. This manual
follows the same rule: a German system locale opens the German edition, every
other locale opens this English one.

---

## 16. Quitting

Press `Esc` in the image window when nothing is loaded, or close it the usual
way for your platform (the red close button on macOS, the ✕ on Windows and
Linux). If images are loaded, `Esc` clears them and returns to the initial
drop screen instead — press it again (now that the set is cleared) to quit.
Merge mode, sort order, the picture-frame interval and shuffle setting, and
the window size carry over to the next launch (see their sections above);
nothing else does — zoom, rotation, and which image you were looking at all
reset.

---

## 17. Current limitations

Things Image Drop deliberately does not do (yet):

- No mouse-wheel or trackpad-scroll navigation; browsing between images is
  keyboard-only (arrow keys, `Home`/`End`, see above)
- No real trackpad pinch-to-zoom gesture; Shift+scroll is the closest
  stand-in for two-finger pan (see "Zoom and pan" above)
- No zoom control within picture-frame mode itself, and no per-image timing
  override — every image gets the same interval (animated GIFs aside)
- No image editing beyond rotation: no cropping, no colour or exposure
  adjustment, no resizing. What you can write to disk is a rotation
  (**File -> Save Changes**) and a copy in another format
  (**File -> Export as PNG…/JPEG…**), both described in "Menu" below
- No support for RAW, SVG, or PDF
- No playback controls (pause, step, restart) for animated GIFs
- No EXIF GPS/location display — deliberately left out of the EXIF data
  window (see "Info overlay" above) for privacy

---

## 18. Quick reference

- **Load** — drag image files onto the window (replaces the current set)
- **Open** — click the drop zone, or press `Cmd`/`Ctrl+O` (or
  `Cmd`/`Ctrl+Shift+O`, same thing), for the system file picker (files and
  folders on macOS/Linux, files only on Windows)
- **Merge mode** — `M` toggles it on/off; while on, drops add to the set
  instead of replacing it, and the title bar shows `[merge]`
- **Next / previous** — `→` `↓` / `←` `↑` (wraps around)
- **First / last** — `Home` / `End`
- **Sort order** — `S` cycles name → capture date → modified → size →
  unsorted → back to name
- **Grid overview** — `G` toggles a full-window thumbnail grid; arrow keys
  move the highlight, `Return` or a click opens it, `G`/`Esc` backs out
  without picking one
- **Search by name** — `/` inside the grid filters it to the file names
  containing what you type; `Esc` clears the filter, a second `Esc` leaves
  the grid. The filter survives a selection and vice versa, so `/`, then
  `Cmd`/`Ctrl+A`, acts on exactly the matches
- **Zoom** — `+`/`-` zoom in/out, `1` for 100%, `0` for fit-to-window, or
  scroll to zoom at the cursor; drag, or Shift+scroll, to pan once zoomed in
- **Rotate** — `R`/`Shift+R` rotate 90° clockwise/counter-clockwise,
  view-only; `0` resets it along with zoom
- **Info overlay** — `I` toggles a card with the file name, position,
  dimensions, file size, and zoom level
- **EXIF data window** — `E`, or the info overlay's "Show EXIF data" link,
  opens camera make/model, lens, exposure, aperture, ISO, focal length, and
  capture date for the current image (no GPS/location)
- **Picture-frame mode** — `P` toggles a full-screen slideshow with a
  crossfade between images; `↑`/`↓` tune the (default 10s) auto-advance
  interval while it's on; `Shift+P` toggles shuffle order (`[shuffle]` in
  the title bar)
- **Copy** — `Cmd`/`Ctrl+C` copies the current image, `Cmd`/`Ctrl+Shift+C`
  copies its file path; in the grid, `Cmd`/`Ctrl+C` copies the selected
  files themselves
- **Delete** — `Shift+Delete` opens a confirmation card (`←`/`→` to choose,
  `Return` to go, `Esc` to cancel); moves the file to the Trash, or the
  grid's whole selection
- **Select in the grid** — `Cmd`/`Ctrl+click` or `Space` to pick one,
  `Shift+click` for a range, `Cmd`/`Ctrl+A` for all of them (or all the
  search matches); `Esc` clears the selection
- **Manual** — `F1`, or Help -> Manual
- **Clear / Quit** — `Esc` (clears the loaded images first, then quits;
  cancels a folder scan still in progress instead, if one is running)
- **Formats** — JPEG, PNG, GIF (incl. animated), WebP, BMP, TIFF, ICO, XPM,
  HEIC/HEIF, AVIF
- **Max window size** — 1500 × 950
