# PicFetch — notes for Claude

- **Start with `ARCHITECTURE.md`** — accurate package map and "where to look for X" index.
  Update it in the same change whenever the package structure changes (its own rule).
- Open work lives in `todos.md`; do not add TODO/FIXME comments to code.
- Build/test via the Makefile: `make test` (or `go test -race ./...` from this directory),
  `make run`, `make build`. Golden-image e2e tests live in `testdata/` and are
  machine/Fyne-version specific — failures land in `testdata/failed/` for comparison.
- User-visible strings go through `lang.L`; add the key to every bundle in
  `translations/` (`main_test.go` fails if a locale drifts).
- Never run `git commit` — print a suggested commit message; the user commits themselves.
