## What does this change do, and why?

<!-- A short description of the change and the motivation behind it. -->

## How was this tested?

<!-- e.g. `make test`, manual testing steps, new/updated tests added. -->

## Checklist

- [ ] `gofmt -l .` is clean, `go vet ./...` and `go test -race ./...` pass
- [ ] User-visible strings go through `lang.L`, with the key added to every
      bundle in `translations/`
- [ ] `internal/ui/help/manual.md` and `manual_de.md` updated, if this
      changes documented behavior
- [ ] `ARCHITECTURE.md` updated, if this changes the package structure
- [ ] No new TODO/FIXME comments — open items go in `todos.md` instead
