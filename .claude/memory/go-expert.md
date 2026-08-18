# go-expert memory

Persistent notes for the go-expert agent on this repo. Local-only (gitignored) — not shared via git.

## Architecture decisions

- (2026-08-19) Favorites are split into pure `internal/favstore` persistence and a Host-based `internal/ui/favorites` feature. Production storage is initialized explicitly from `ui.Run`, not during viewer construction, so tests never scan the user's real config directory. The first implementation stores only named file lists; disk-backed thumbnail caching remains a separate follow-up.
- (2026-08-19) Favorite accelerators are positional over the current case-insensitively sorted list: Cmd/Ctrl+1–9 open entries 1–9 and Cmd/Ctrl+0 opens entry 10. Register all ten handlers once in `buildViewer`; `favorites.Feature.Open` resolves a slot against names captured by the latest menu refresh, so add/remove never requires shortcut re-registration.
- (2026-08-15) A feature that reorders/replaces `v.files`/`v.unsortedFiles` off the UI goroutine gets its *own* staleness generation counter (e.g. `sortGen`), not `v.gen`. `v.gen` is specifically the load/decode/animation generation - every bump site pairs it with `stopAnimation()`, and reusing it for an unrelated async op would spuriously kill a playing GIF or in-flight preload. Precedent: `toast.gen`. When adding a new async op, grep for every existing writer of the state it will eventually overwrite (`grep -rn "v\.files\s*=\|v\.unsortedFiles\s*="` etc.) and bump the new op's own gen at each of those sites too, so a stale background result can't clobber state a *different* feature changed in the meantime - not just a newer instance of the same op.
- (2026-08-15) Async UI operations in this codebase (scan, sort, ...) all follow one shape: synchronous snapshot of inputs on the UI goroutine -> bump a dedicated gen counter -> show spinner/label + `ForceRepaint()` -> `go func() { compute; fyne.Do(applyResult) }()` -> `applyResult` does `defer close(doneChan)` first, unconditionally resets the spinner/loading flag, *then* checks the gen for staleness before touching shared state. Follow this shape for any future long-running operation instead of inventing a new one. Reference implementations: `sort.go`'s `startSort`/`finishSort` (the generic form, with an `onDone` callback - used by both `SetSortMode` and `drop.go`'s `applyScannedFiles`) and `drop.go`'s `handleDrop`/`applyScanResult`.
- (2026-08-15) A snapshot handed to one of those background goroutines must be a *defensive copy* when it comes from `v.files`/`v.unsortedFiles`, never a bare slice-header assignment: `RemoveFile` shifts those slices in place (`append(s[:j], s[j+1:]...)`), so an aliased snapshot is a genuine data race with whatever the goroutine reads. Same for merges - copy first, then append, so a spare-capacity append doesn't write into the shared array either. Confirmed reachable under `-race`, not theoretical; guarded by `TestSetSortMode_SnapshotDoesNotAliasUnsortedFiles`.
- (2026-08-15) When a background op eventually writes *both* `v.unsortedFiles` and `v.files`, write them together in the completion callback and never one early: `RemoveFile` documents them as holding the same set, and a half-applied update leaves `v.index` able to point past the end of a `v.files` some later-landing reorder replaced. Accepted consequence: a `SetSortMode` fired during a large drop's reorder window supersedes it, so that drop silently doesn't take (no crash, no corruption, user re-drops). Deliberately not solved with a coalescing/retry queue.

## Conventions

- Dedicated spinner/label widget pairs per async feature (not a shared one), even though visually similar - two concurrent async ops (e.g. a merge-mode scan still running when a sort-mode change is requested) must not fight over one pair of widgets. Build each pair via its own `newXUI()` in `build.go` next to `newScanUI`/`newSortUI`, wire into the content `Stack` alongside `scanContainer`.
- `*_test.go`: every background op gets a `waitForX(t, v)` helper next to `waitForScan`/`waitUntilLoaded` in `library_test.go`, and a `{"name", v.xDone}` entry in `drain`'s wait list, so tests can synchronize deterministically instead of polling widget state (which races the fyne test driver's synchronous `fyne.Do` under `-race`).

## Conventions (testing)

- `-race` regression tests for aliasing/ordering bugs are worth keeping even
  though they only fail probabilistically: they can never fail spuriously
  (the detector is the only assertion), so the worst case is a silent pass.
  Size them so the background copy is still in flight when the mutation
  starts (4000 `uitest.FakeURI`s + 200 `RemoveFile` calls reproduced 3/3
  fresh binaries; the detector dedupes repeats within one binary, so
  `-count=N` only ever reports once).

## Open questions

- (2026-08-15) Escape during the post-drop background reorder hits
  `keys.go`'s `len(v.files) == 0` branch (the scan is done, `v.files` not
  set yet) and closes the window instead of cancelling. Logged in
  `todos.md`; needs a "sort in flight" state Escape can see, which means
  deciding whether that state belongs on the viewer or inside `startSort`.
