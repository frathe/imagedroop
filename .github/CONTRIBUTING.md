# Contributing to Image Drop

Thanks for taking the time to contribute! This document covers everything
you need to get set up and send a useful pull request.

## Code of Conduct

This project follows a [Code of Conduct](CODE_OF_CONDUCT.md). By
participating, you're expected to uphold it.

## Getting set up

- Go 1.26.6 or newer (see the `go` directive in [go.mod](../go.mod))
- A C toolchain for cgo (Fyne's OpenGL bindings require it) — Xcode Command
  Line Tools on macOS, `gcc` + `libgl1-mesa-dev`/`xorg-dev` on Linux
- See the [README](../README.md#requirements) for the full list, including
  the optional tools needed for cross-compiling and security checks

```sh
git clone https://github.com/frathe/imagedrop.git
cd imagedrop
make run
```

## Before you start

- **Start with [ARCHITECTURE.md](../ARCHITECTURE.md)** — it's an accurate
  package map and a "where to look for X" index. Update it in the same
  change whenever the package structure changes.
- Open work is tracked in [todos.md](../todos.md); please don't add
  TODO/FIXME comments to the code itself.
- For anything beyond a small fix, opening an issue first to discuss the
  approach is welcome but not required.

## Making a change

1. Fork the repo and create a branch off `main`.
2. Make your change.
3. Run the checks the CI pipeline runs:

   ```sh
   gofmt -l .          # should print nothing
   go vet ./...
   go test -race ./...
   ```

   Or via the [Makefile](../Makefile): `make fmt`, `make vet`, `make test`.
4. If you touched any user-visible string, route it through `lang.L` and add
   the key to **every** bundle in [translations/](../translations/) —
   `main_test.go` fails if a locale drifts out of sync.
5. If you touched behavior described in the built-in manual
   ([internal/ui/help/manual.md](../internal/ui/help/manual.md) and its `_de`
   counterpart), update both. Fyne's markdown renderer has no table
   extension, so keep the manual table-free.
6. If a golden-image e2e test fails because of a legitimate visual change,
   regenerate the master with `make golden` rather than a plain `go test` —
   it renders inside a `linux/amd64` container matching CI exactly (needs
   Docker). Fyne's software rasterizer renders slightly different
   anti-aliased pixels depending on CPU architecture, so a master captured
   by running `go test` directly on, say, an Apple Silicon Mac can pass
   there and still fail in CI. `make golden` writes the new render to
   `internal/ui/testdata/failed/<name>.png` — inspect it, and if it looks
   right, copy it over `internal/ui/testdata/<name>.png` to accept it as the
   new baseline. See the [README](../README.md#end-to-end-suite-internaluie2e_testgo)
   for more on the e2e suite.
7. Commit with a clear message describing *why*, not just *what*.

## Submitting a pull request

- Open the PR against `main` and fill in the pull request template.
- Keep the change focused — unrelated cleanup makes review harder and is
  easier to land as its own PR.
- CI (`gofmt`, `go vet`, `go build`, `go test -race`) must pass.
- A maintainer will review and may ask for changes before merging.

## Reporting bugs and requesting features

Please use the issue templates — they ask for the information needed to
reproduce a bug or evaluate a feature request.

## Security issues

Please do **not** open a public issue for a security vulnerability — see
[SECURITY.md](SECURITY.md) instead.
