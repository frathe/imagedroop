# PicFetch — Next Refactorings

Findings from a full-codebase review (2026-08-21). The codebase is in good
shape overall — the Phase-2 feature-package split left `internal/ui`'s
subpackages clean and narrow, there are no TODO/FIXME markers, and the doc
comments are unusually thorough. What remains is structural: the leftovers
that the previous refactoring rounds deliberately kept in the core, plus a
few files that have outgrown their single-file shape. Ranked by
payoff-per-risk, best first.

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

## 4. Split `internal/ui/grid/grid.go` (995 lines, one file)

The grid package is healthy but is one file holding four separable
concerns; its test file (1,154 lines) mirrors the problem. Split within
the package, no API change:

- `grid.go` — `Host`, `Overview`, construction, Toggle/Close/Overlay
  lifecycle.
- `nav.go` — `HandleKey`, `movePage`, highlight/selection movement,
  `escape`/`backspace`.
- `search.go` — `HandleRune`, `Query`, `applyFilter`, `syncTopBar`, the
  display→host index mapping (`fileIndex`, `count`).
- `thumbs.go` — the decode pipeline: `requestThumbnail`,
  `claim`/`release`/`stillWanted`, `Cached`/`CachedThumb`/`StoreThumb`/
  `ThumbCacheFull`/`SetCacheBytes`, `Warm`/`Settle`.

Stretch goal, only if a real need appears: `viewer.go` notes the preload
semaphore is "the same small-worker-pool shape internal/ui/grid gives
thumbnails" — the claim/semaphore/pending trio in `thumbs.go` and
`preloadSem`/`preloading`/`preloadPending` in the viewer could share one
bounded-decode-pool type.

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

## Noted but not in the top 5

- `finishLoad` (`internal/ui/load.go:192-305`) is a 114-line
  do-everything pipeline (vector setup, fade, overlay, zoom, resize,
  title, animation, preload). It is linear and well-commented; decompose
  into named steps only if it needs to change anyway.
- `internal/imaging/exif.go` (687 lines) holds two parsers plus IFD
  walking plus display formatting. Cohesive and well-tested; a
  parse/format file split is cosmetic.
- `ARCHITECTURE.md` is ~66 KB and duplicates much per-field/function doc
  commentary; consider trimming it to the navigation map it says it is,
  so it stops drifting from the code.
