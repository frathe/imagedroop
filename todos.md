# Image Drop — TODOs

## Done

- **Move to Trash instead of permanent delete** — `internal/ui/deletion`'s
   Shift+Delete flow now routes through the new per-OS `internal/trash`
   package instead of calling `os.Remove` directly.

- **Save rotation to disk** — a File > "Save Changes" menu item (also
   Cmd/Ctrl+S), grayed out except when there's a pending rotation to save,
   writes `rotate.go`'s view-only rotation back to the file via the new
   `internal/imaging/save.go` (`SaveRotated`/`CanEncode`: JPEG/PNG/GIF/
   BMP/TIFF/AVIF, atomic temp-file-then-rename write). Animated images and
   formats with no encoder (WebP/HEIC/ICO/XPM) are excluded, not converted.

## TODO

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
