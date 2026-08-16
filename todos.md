# Image Drop — TODOs

## Done

- **Filename search in the grid** — `/` opens a search bar in the grid
   overview (`internal/ui/grid`) and what follows filters the cells to the
   file names containing it, case-insensitively; `Esc` clears the filter,
   a second `Esc` leaves the grid as before. The filter is grid-local: it
   renumbers only the cells drawn, so `matches`/`fileIndex` map a display
   index back to the host's file index and `Host` needed no new method —
   navigation outside the grid still walks the whole set. Input arrives via
   a new `Canvas.SetOnTypedRune` dispatch (`keys.go`'s `handleTypedRune`)
   rather than key names, since a `fyne.KeyEvent` carries neither case nor
   `_`; that also keeps Fyne's widget focus out of it, so arrows/Return/Esc
   still reach the grid as they always did. `G` had to become inert while
   searching — a letter key delivers both a rune and a key event, so the
   close shortcut would otherwise fire on its way into the query. A new
   `filterGen` guard discards a thumbnail decode whose cell was renumbered
   under it: the file set and the cell's id are both still current after a
   keystroke, so neither existing guard could see it.

- **Save As / convert image format** — File > "Export as PNG…"/"Export as
   JPEG…" (`internal/ui/export.go`) writes the frame on screen to a new file
   in a format chosen by the menu item rather than by the source, via
   `internal/filepicker`'s new `ChooseSave` (per-OS save panel: `NSSavePanel`
   on macOS, `zenity --save` on Linux, `SaveFileDialog` on Windows) and
   `internal/imaging`'s new `Export`. Deliberately available wherever "Save
   Changes" isn't: WebP/HEIC/ICO/XPM sources and animated GIFs both export
   fine, since the destination's format is what's re-encoded. `SaveRotated`
   and `Export` now share one atomic temp-file-then-rename writer. The name
   the user types wins when it already carries an encodable extension,
   otherwise the menu item's is appended — so a file's bytes always match
   its extension.

- **Bound a GIF's transient paletted decode** — the animation budget is now
   applied *before* `gif.DecodeAll` instead of after it. `internal/imaging`'s
   new `probeGIF` walks the raw GIF block structure (skipping every block by
   the length fields the format already carries, decoding no pixels) to count
   image descriptors and read the logical screen size, so an over-budget
   animation allocates nothing at all. Because `image/gif` rejects any frame
   larger than the logical screen, clearing the gate also caps `DecodeAll`'s
   own paletted peak at a quarter of the budget — the transient cost that was
   previously bounded only by `MaxEncodedBytes` and LZW expansion. Side
   benefit: a single-frame GIF is no longer decoded twice. A fuzz target pins
   `probeGIF` against `gif.DecodeAll`; it caught a real divergence on
   application extensions within a second.

- **Byte-bounded image memory** — both image caches are now bounded by an
   estimated byte budget instead of an entry count (`internal/imaging`'s new
   `ByteCache`), with the budgets, plus a ceiling on a file's encoded size,
   exposed in File > Settings… (`internal/ui/memlimits.go`). Animated GIFs
   get a cumulative budget checked before any frame is composited, falling
   back to a static first frame with a toast rather than refusing the file;
   thumbnails pass a zero animation budget, so a long GIF no longer
   composites every frame just to keep frame 0. Neighbor preloading bails on
   the header when a decode would evict the image on screen. Worst-case
   retained image memory drops from roughly 12 GB to the configured
   budgets.

- **Move to Trash instead of permanent delete** — `internal/ui/deletion`'s
   Shift+Delete flow now routes through the new per-OS `internal/trash`
   package instead of calling `os.Remove` directly.

- **Save rotation to disk** — a File > "Save Changes" menu item (also
   Cmd/Ctrl+S), grayed out except when there's a pending rotation to save,
   writes `rotate.go`'s view-only rotation back to the file via the new
   `internal/imaging/save.go` (`SaveRotated`/`CanEncode`: JPEG/PNG/GIF/
   BMP/TIFF/AVIF, atomic temp-file-then-rename write). Animated images and
   formats with no encoder (WebP/HEIC/ICO/XPM) are excluded, not converted.

- **Fix golden masters for linux/amd64 CI**, add make golden
   delete_confirm_{cancel,danger}.png were regenerated on darwin/arm64,
   which Fyne's test harness compares leniently but CI (linux/amd64)
   does not - CI failed with a byte-exact mismatch. Re-rendered both
   under linux/amd64 to match CI, and added a `make golden` Docker
   target (+ CONTRIBUTING/README updates) so future regenerations are
   never machine-dependent.

## TODO

- **Multi-select + batch ops in grid view** — Shift/Cmd-click to select
   multiple thumbnails in `internal/ui/grid`, then batch-delete (through
   `internal/ui/deletion`) or batch-copy the selection.

- **Set current image as desktop wallpaper** — a per-OS action
   (AppleScript on macOS, PowerShell on Windows, gsettings on Linux)
   mirroring the per-OS dispatch pattern already used by
   `internal/clipboard`/`internal/filepicker`/`internal/winpos`.

## not deemed worth implementing (edgecases)

- **Downsample very large stills before caching** — the premise doesn't hold.
   Downsampling can only run *after* `image.Decode` returns, so it shrinks
   what is retained, never the transient full-resolution decode that was the
   reason to want it: none of the decoders this module links (`image/jpeg`,
   `image/png`, `x/image/{webp,bmp,tiff}`, `gen2brain/{avif,heic}`) expose a
   reduced-resolution decode mode. The peak stays bounded by
   `MaxEncodedBytes` and `maxImagePixels`, as it already is. What was left —
   fitting more images in the cache budget, cheaper `RotateSteps`/redraw/
   clipboard-encode — didn't justify a `LoadedImage` interface change plus a
   full-resolution reload path for File > Save Changes, clipboard copy, and
   zoom past the downsample factor, each of which would otherwise silently
   operate on degraded pixels.
