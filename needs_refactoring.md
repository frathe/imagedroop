# PicFetch — Next Refactorings

Findings from a full-codebase review (2026-08-21). The codebase is in good shape overall — the Phase-2 feature-package
split left `internal/ui`'s subpackages clean and narrow, there are no TODO/FIXME markers, and the doc comments are
unusually thorough. What remains is structural: the leftovers that the previous refactoring rounds deliberately kept in
the core, plus a few files that have outgrown their single-file shape. Ranked by payoff-per-risk, best first.

I can't write to the file — this side-question instance has no tools. Here's the text to paste, in the file's existing
voice and wrap width.

Note on numbering: Stage 8 deleted item 5 from `needs_refactoring.md`, and that file's numbering tracked `todos.md`
(which has 3, 4, 5 in Done). So 6 and 7 continue the sequence; renumber to 1 and 2 if you'd rather restart now that the
list is empty.

## Two new numbered items

## 7. Restore the "never started" canary to the harness's wait helpers

Before the `completion.Signal` migration, waiting on an operation that had never begun blocked on a nil channel until
the test timed out with a named message. `Signal.Wait` on a never-begun signal returns nil immediately - which is
exactly what lets `drain` drop its nil-guard, but also means a helper that used to fail loudly now returns silently.

The guard went back in unevenly, for a defensible reason with an uneven result. Helpers that carried an *explicit*
`== nil` check kept it as
`Begun()`: `settleChooser`, `settleWallpaper`, `settleFavoritePreviews`, and `settleToast` - the last being the best of
the four, since its
`stop == nil` answers "pending *now*" rather than "ever begun". Helpers that relied on the *implicit* nil-channel block
lost theirs silently:
`waitUntilLoaded`, `waitForScan`, `waitForSort`, `waitForAnimStopped`,
`waitForClipboard`. Five of ten waiters have a canary; five do not.

No call site is vacuous today - each has a positive assertion after the wait - so what this costs is diagnostic
sharpness, not coverage. The fix is a mechanical `if !v.load.Begun() { t.Fatal(...) }` per helper, but it needs a full
`-race` run to prove no test legitimately waits on a signal that never began.

## Noted but not in the top 5

- `internal/ui/toast.go` merges its `fyne.io/...` and
  `github.com/frathe/picfetch/...` imports into one block, where ~20 other `internal/ui` files use three
  blank-line-separated groups. Present since that file's first commit (`9eda18c`). `gofmt` does not enforce grouping by
  path prefix and `make verify` runs no
  `goimports -local`, so nothing catches it. Worth a one-line fix the next time that file's imports change for another
  reason.
- `finishLoad` (`internal/ui/load.go:192-305`) is a 114-line do-everything pipeline (vector setup, fade, overlay, zoom,
  resize, title, animation, preload). It is linear and well-commented; decompose into named steps only if it needs to
  change anyway.
- `internal/imaging/exif.go` (687 lines) holds two parsers plus IFD walking plus display formatting. Cohesive and
  well-tested; a parse/format file split is cosmetic.
- `ARCHITECTURE.md` is ~66 KB and duplicates much per-field/function doc commentary; consider trimming it to the
  navigation map it says it is, so it stops drifting from the code.

Unify nine hand-rolled completion channels behind internal/completion 
internal/ui had grown nine copies of one contract: a chan struct{} replaced                                                                                                                                                            
at the start of each background request and closed when it finished, which                                                                                                                                                             
the test suite waited on instead of polling widget state a producer goroutine                                                                                                                                                          
might still be writing. Nine fields, nine field comments restating the same                                                                                                                                                            
discipline, eleven hand-rolled waiters. 
The rule those copies could only state in prose - a superseded request must                                                                                                                                                            
still close its own channel without touching the field a newer request now                                                                                                                                                             
owns - is now enforced by the type: Begin() returns a closure over its own                                                                                                                                                             
generation, so a stale producer has no way to reach the newer one.                                                                                                                                                                     
Current()/Handle covers the one case a bare Wait can't: proving a specific,                                                                                                                                                            
since-superseded generation's goroutine actually exited. 
drain's two loops merge into one, ordered causally rather than by migration                                                                                                                                                            
order - chooser, scan, sort, load, animation, then preloads. Each producer's                                                                                                                                                           
finisher fires after the next stage's Begin, so a row that can still start the                                                                                                                                                         
work a later row waits on must come first. 
animFrame, toast.stop, vector.pending, preloads and the two Settle calls stay                                                                                                                                                          
as they are: an N-event counter, a cancel whose nil-out is load-bearing, and                                                                                                                                                           
N-goroutine WaitGroup waits are all different contracts from a one-shot                                                                                                                                                                
completion.                                                          