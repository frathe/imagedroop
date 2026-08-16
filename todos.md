# Image Drop — TODOs

## Done

- **Move to Trash instead of permanent delete** — `internal/ui/deletion`'s
   Shift+Delete flow now routes through the new per-OS `internal/trash`
   package instead of calling `os.Remove` directly.

## TODO

- **Save rotation to disk** — `rotate.go`'s 90°-step rotation
   (`rotateBy`/`resetRotation`) is view-only; add an explicit save action
   that writes the rotated pixels back to the file via `internal/imaging`'s
   orientation transforms (`ApplyOrientation`/`RotateSteps`).

- **Save As / convert image format** — `internal/imaging` decodes many
   formats (HEIC/AVIF/BMP/TIFF/WebP/...) but the app never writes any of
   them back out; add an export/"save as" action to convert the current
   image to a common format (PNG/JPEG).

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
