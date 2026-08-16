# Image Drop — TODOs

## Done

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

- **Bound a GIF's transient paletted decode** — `decodeAnimatedGIF`'s budget
   stops the 4-bytes-per-pixel composited copies, but `gif.DecodeAll` has
   already decoded every frame as paletted (~1 B/px) before the check can
   run, because the standard library exposes no way to learn a GIF's frame
   count without decoding it. Retained memory is bounded; that transient
   peak is bounded only by `MaxEncodedBytes` and LZW expansion. Closing it
   means counting image descriptors in the raw GIF blocks before decoding.

- **Save As / convert image format** — `internal/imaging/save.go` can only
   overwrite a file in its own original format (see "Save rotation to disk"
   above), and has no encoder at all for WebP/HEIC/ICO/XPM; add an
   export/"save as" action to convert the current image to a common format
   (PNG/JPEG) regardless of its source format.

- **Filename search / jump-to-file** — a type-ahead search (e.g. bound to
   `/`) that filters or jumps to a file by name within the current file
   set, most useful from `internal/ui/grid`.

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
