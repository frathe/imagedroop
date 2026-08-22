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

## 4. Split `internal/ui/grid/grid.go` into four files

`internal/ui/grid/grid.go` (995 lines, one file holding four separable
concerns) is now four files in the same package, with no API change:
`grid.go` keeps `Host`, `Overview`, construction, and the
Toggle/Close/Overlay lifecycle; `nav.go` took the highlight ring and its key
dispatch; `search.go` took the `/` filename filter and the display→host
index mapping; `thumbs.go` took the thumbnail cache and its bounded decode
pipeline. `grid_test.go` split the same four ways plus a new
`harness_test.go` for the shared fake `Host` and helpers (`openGrid`,
`typeQuery`) that `selection_test.go` also uses. Pure motion: every
declaration moved byte-identical, nothing renamed, no visibility change, no
exported API change.

## ACTIVE DEVELOPMENT

## Shared bounded-decode-pool type (item 4's stretch goal)

The grid's thumbnail decode pool (`thumbs.go`) and the viewer's preload pool
(`preloadSem`/`preloading`/`preloadPending`) still duplicate the same
semaphore/in-flight-claim/WaitGroup trio. Collapsing them onto one shared
type in a new `internal/decodepool` package is in progress — see
`planned_features/split_grid.md` (stages 9–12).

## TODO

## 5. Unify the test-synchronization channels behind one small type

The viewer carries nine ad-hoc `chan struct{}` fields with the same
replace-on-start / close-on-finish / wait-in-test contract — `scanDone`,
`loadDone`, `sortDone`, `clipboardDone`, `chooserDone`, `wallpaperDone`,
`favThumbDone`, `animStopped` (plus `animFrame`'s atomic counter) — each
re-documenting the same discipline in its field comment, each with its own
hand-rolled waiter in `harness_test.go`. A tiny `completion` type (e.g.
`begin() (done func())` + `wait(ctx)`) would collapse nine fields and
seven waiter helpers into one audited implementation, and make it
impossible for a new async feature to get the "stale generation must still
close its own channel" rule subtly wrong — a rule currently enforced only
by prose. Pairs naturally with the `asyncOpUI` grouping in item 2.

## not deemed worth implementing (edge cases)

- There is a bug in the Windows Version: WHen in Gridview, multiselect via
  the space key works, but when trying it with mouse and Ctrl key, it does not.
  Holding the Ctrl key down and clicking on an image does not select it but
  instead opens it. Observation, when pushing the Ctrl key at exactly the same time
  as clicking on the image, it actually works, and the image is selected.
  (this seems to be a bug in fyne, created an issue, sorry Windows users)

