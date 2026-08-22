# PicFetch — TODOs

## Done

## ACTIVE DEVELOPMENT

## TODO

## 3. Extract the directory-scan walker out of `handleDrop` (`internal/ui/drop.go:94-271`)

The recursive folder scan — symlink-cycle guard (`visitedDirs`), per-scan
dedupe (`seenFiles`), the `maxScan` cap, the throttled progress callback —
is an anonymous-closure state machine inside a goroutine inside a 180-line
viewer method. Its logic is pure (URIs in, image URIs + truncated flag
out) but its tests (`TestHandleDrop_SymlinkCycleDoesNotHang`,
`_DedupesOverlappingDirectories`, `_CapsFileCountForLargeTrees`, …) each
need a full viewer, a Fyne test app, and the drain machinery.

Extract a `scanImages(ctx, uris, max, progress func(n int)) ([]fyne.URI,
bool)` — as a package-local file or a small `internal/scan` package —
leaving `handleDrop` as UI glue: snapshot merge mode, show spinner, call
walker, apply result. This also removes the duplication between the
no-directories fast path and the goroutine path, which today each carry
their own `seen` map + `IsSupportedImage` + `realPathOf` dedupe loop.

## not deemed worth implementing (edge cases)

- There is a bug in the Windows Version: WHen in Gridview, multiselect via
  the space key works, but when trying it with mouse and Ctrl key, it does not.
  Holding the Ctrl key down and clicking on an image does not select it but
  instead opens it. Observation, when pushing the Ctrl key at exactly the same time
  as clicking on the image, it actually works, and the image is selected.
  (this seems to be a bug in fyne, created an issue, sorry Windows users)

