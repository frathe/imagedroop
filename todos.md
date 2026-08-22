# PicFetch — TODOs

## Done

## 3. Extract the directory-scan walker out of `handleDrop`

The recursive folder scan — symlink-cycle guard, per-scan dedupe, the
`maxScan` cap, the throttled progress callback — is now `internal/filescan`'s
`Images(ctx, uris, max, progress)`, tested with just `test.NewApp()` in
`TestMain` instead of a full viewer. `handleDrop` (`internal/ui/drop.go`) is
UI glue: snapshot merge mode and the cap, show the spinner, call `Images`,
apply the result. Both drop paths now share the one walker, so the `maxScan`
cap applies to loose-file drops too, not just recursive folder scans.

## ACTIVE DEVELOPMENT

## TODO

## not deemed worth implementing (edge cases)

- There is a bug in the Windows Version: WHen in Gridview, multiselect via
  the space key works, but when trying it with mouse and Ctrl key, it does not.
  Holding the Ctrl key down and clicking on an image does not select it but
  instead opens it. Observation, when pushing the Ctrl key at exactly the same time
  as clicking on the image, it actually works, and the image is selected.
  (this seems to be a bug in fyne, created an issue, sorry Windows users)

