# PicFetch Agent Guide

## Start Here

- Read `ARCHITECTURE.md` before code: it is the authoritative package map and “where to look for X” index.
- Update `ARCHITECTURE.md` in the same change when packages are added, removed, renamed, or files move between packages.
- Open work belongs in `todos.md`; do not add `TODO`/`FIXME` comments to source.
- Do not run `git commit`. End with a suggested commit message for the user.

## Architecture and Data Flow

- `main.go` only creates the Fyne app, embeds translations, converts CLI paths, and calls `internal/ui.Run`; keep package `main` thin.
- `internal/ui/appState` owns the current/unsorted file lists, index, sort mode, and merge mode. The unexported `viewer` is its Fyne-facing façade; `ui.Run` is the package’s only exported entry point.
- Feature packages such as `internal/ui/grid`, `deletion`, and `slideshow` own their widgets/state and declare narrow consumer-side `Host` interfaces. Do not pass them `appState` or invent a shared controller/registry.
- Cross-feature decisions stay in `internal/ui`: for example, `batch.go` joins grid selection to deletion/clipboard, and `togglePictureFrameMode` prevents grid/slideshow overlap.
- Feature construction order in `internal/ui/features.go` and overlay order in `build.go` are load-bearing; preserve explicit composition rather than auto-registration.
- Input flows through CLI/open/drop → `handleDrop` scan → `filesort.Order` → `ShowImage` → `internal/imaging` probe/decode/orient/cache → Fyne display. Reuse this path rather than creating parallel open/load logic.
- `internal/imaging` is viewer-independent. Full images and grid thumbnails use separate byte-budgeted caches; preserve `ByteCache.Add` (displayed image) versus `AddIfFits` (speculative preload) semantics.
- Session file sets use `internal/session`/Fyne cache; standing settings and geometry use `internal/preferences`/Fyne preferences. Startup wiring is in `internal/ui/startup.go`, shutdown persistence in `run.go`.
- OS integrations live behind dispatcher vars in `internal/{clipboard,filepicker,trash,wallpaper}` with build-tagged platform files. Tests must replace them via `internal/uitest` stubs, never touch the real desktop.
- Keep `appID` synchronized across `main.go`, `FyneApp.toml`, and `Makefile`; changing it disconnects existing preferences/session data.

## Concurrency and Fyne

- Scan, load, sort, and vector work each own a `requestLifecycle`; capture its token, check staleness before expensive work and before applying results, and marshal background UI updates through `fyne.Do`.
- Do not add mutable package-level test seams. Runtime/test-configurable values belong on `viewer` or the owning feature.
- Every goroutine needs cancellation/staleness handling plus an observable stop/done signal. If adding background work, add it to `newTestUI`’s `drain` cleanup in `internal/ui/harness_test.go`.
- Fyne’s test driver runs `fyne.Do` inline. Use `dropAndWait`, `waitFor*`, feature `Settle`, and existing completion channels before assertions; never sleep to guess completion.

## Project Conventions

- Every user-visible string is `lang.L("English text")`; add that exact key to every `translations/*.json` bundle. English is an identity map and `main_test.go` enforces locale parity.
- Report UI-boundary failures with `fyne.LogError`; viewer-independent packages return errors. Mark intentionally ignored errors explicitly (`_ =` or `_, _ =`) so IDE/`errcheck` inspections see intent.
- Use `internal/uitest` for synthetic image formats, temp URIs, approximate comparisons, and OS seam stubs. UI tests should build through `newTestUI`/`newTestViewer`, which mirror production startup.
- Keep platform-specific behavior in existing build-tag pairs and preserve no-cgo HEIC/AVIF decoding through `gen2brain` WASM; Fyne itself still requires a C/OpenGL toolchain.

## Build and Verification

- Use the Makefile: `make run`, `make build`, `make test`, `make fmt`, `make vet`; `make help` lists packaging/security targets.
- Match CI before handoff: formatting, `go vet ./...`, `go build ./...`, then `go test -race ./...` from the repository root.
- Run focused tests while iterating, e.g. `go test -run TestE2E -v ./internal/ui/...`; the complete suite remains the final check.
- Golden screenshots are under `internal/ui/testdata/`. Regenerate only with `make golden` (Docker linux/amd64), inspect `internal/ui/testdata/failed/*.png`, and never commit failed renders.
- Packaging uses Fyne/Fyne-cross; macOS is native, while Windows/Linux cross-builds require Docker. `fyne package` may bump `FyneApp.toml`’s build number.

