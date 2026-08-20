# Favorites Disk Thumbnail Cache

Status: planned. Source TODO: `todos.md` → "favorites disk thumbnail cache".

## Goal

Persist grid previews for each favorite under that favorite's own directory,
so re-opening a favorite paints its grid from ~8 KB JPEGs instead of decoding
multi-megapixel sources. Previews are generated in the background when a
favorite is saved, and topped up (plus swept) when one is opened.

## Decisions

| Question | Decision |
|---|---|
| Layout | Per-favorite: `<favoritesDir>/<name>/thumbs/` |
| Trigger | On favorite save (generate all) and on favorite open (fill missing) |
| Staleness | Source `mtime` + `size` encoded in the preview's filename |
| Format | JPEG q85; PNG when the thumbnail has real transparency |
| Pruning | Sweep on open: delete any preview the current list does not map to |
| Grid wiring | Pre-warm the existing in-memory `ByteCache` via three new accessors on `grid.Overview` |
| User control | One settings checkbox, default on; failures are `fyne.LogError` only |

### On-disk shape

```
~/Library/Application Support/picfetch/favorites/
  Trip/
    file-list.json
    thumbs/
      3f2ac41d9b7e5a08-1755590400-4823919.jpg
      9c17be4e2210f6d3-1755501234-2201883.png
```

Filename is `<pathhash>-<mtime>-<size><ext>`:

- `pathhash` — first 8 bytes of SHA-256 over the source's absolute path, hex.
- `mtime` — source mod time, Unix seconds.
- `size` — source size in bytes.
- `ext` — `.jpg`, or `.png` when the generated thumbnail has any pixel with
  alpha < 0xFF.

A changed source yields a different filename, so a stale preview is never
read; it is deleted by the next sweep. Reading probes `.jpg` then `.png`.

### Known limitation (accepted)

Pre-warm is bounded by the thumbnail cache's byte budget — about 1,600
previews at the 256 MB default. A favorite larger than that gets a warm head
and a cold tail; the tail decodes from source exactly as it does today.
Previews are still written to disk for every file.

Getting that bound right needs `grid.ThumbCacheFull`, not just `AddIfFits`.
`AddIfFits` refuses only an entry that outweighs the *whole* budget by
itself; once the cache is merely full it evicts least-recently-used entries
and stores anyway. A pre-warm that ignored this would evict its own earliest
entries as it walked the list and finish holding only the *last* N
thumbnails — while the grid opens at the *first* file. So the Stage 7 sink
stops offering once `ThumbCacheFull` reports true.

A lazy per-cell disk provider inside `grid` would remove the ceiling and is
the natural follow-up, but is deliberately out of scope here.

## Architecture

New viewer-independent package `internal/favthumbs` (returns errors, no Fyne
widgets, depends on `internal/imaging` and `internal/favstore`).

```go
package favthumbs

const SubDir = "thumbs"

// Sink is the consumer-side view of the caller's in-memory thumbnail cache.
type Sink interface {
    Cached(src fyne.URI) (image.Image, bool) // skip decode when already warm
    Store(src fyne.URI, thumb image.Image)   // speculative add (AddIfFits)
}

func EntryName(src fyne.URI) (string, bool)                  // stat-derived base name
func Read(favDir string, src fyne.URI) (image.Image, bool)   // current preview, if any
func Write(favDir string, src fyne.URI, thumb image.Image) error
func Sweep(favDir string, files []fyne.URI) error
func Sync(ctx context.Context, favDir string, files []fyne.URI, sink Sink) error
```

`Sync` is the whole background pass, and serves both triggers:

```
for each file (bounded concurrency, ctx-checked):
    sink.Cached(u)  hit  -> Write if no current preview on disk; next
                            (no Store: it came from the sink already)
    Read(favDir, u) hit  -> Store; next
    otherwise            -> imaging.LoadThumbnail(u); Write; Store
finally: Sweep(favDir, files)
```

Cross-feature wiring stays in `internal/ui` (new file `favthumbs.go`), the
same way `batch.go` joins grid selection to deletion/clipboard:

- `favorites.Host` gains one method, `SyncFavoritePreviews(favDir string, files []fyne.URI)`.
- `internal/ui/favorites` calls it after a successful `writeFavorite` and
  after `openFavorite` loads a list, passing `favstore.Dir(f.dir, name)`.
- `viewer.SyncFavoritePreviews` owns a `requestLifecycle` + done channel,
  checks the preference, and adapts `v.grid` to `favthumbs.Sink`.
- `internal/ui/grid` grows `CachedThumb`/`StoreThumb`/`ThumbCacheFull`
  accessors over its existing `thumbs` cache.

## Stages

Each stage is TDD: write the test, run it, watch it fail for the right
reason, then write the minimal code to pass. Every stage ends green with
`go vet ./... && go build ./... && go test -race ./<pkg>/...`, and the full
suite runs before handoff. Stages are ordered by dependency; 1-4 are all
inside the new package and touch nothing existing.

### Stage 1 — `favthumbs`: paths and entry names (sonnet)

Add `internal/favthumbs/name.go` plus `favstore.Dir(dir, name) string`.

- `favstore.Dir` returns `filepath.Join(dir, name)`; empty string for an
  invalid name.
- `EntryName(src)` stats `src` and returns `<pathhash>-<mtime>-<size>`,
  `false` when the stat fails.
- `Dir(favDir)` returns `filepath.Join(favDir, SubDir)`.

Tests: same path is stable across calls; two paths differ; touching the file
changes the name; growing the file changes the name; missing file returns
false; `favstore.Dir` rejects `../escape`.

### Stage 2 — `favthumbs`: encode, write, read one preview (sonnet)

Add `internal/favthumbs/store.go`.

- `hasAlpha(image.Image) bool` — type-switch fast path (`*image.Gray`,
  `*image.YCbCr`, `*image.CMYK` are opaque), otherwise scan with early exit.
- `Write` creates the `thumbs` dir, encodes JPEG q85 or PNG per `hasAlpha`,
  and writes atomically (temp file + `Rename`, mirroring `favstore.Save`).
- `Read` probes `.jpg` then `.png` for the current entry name and decodes.

Tests: round trip returns equivalent pixels; an opaque thumbnail lands on
`.jpg` and a transparent one on `.png` (build fixtures with
`uitest.EncodePNG` using a transparent color); `Read` misses when the source's
mtime changes; misses when the directory does not exist; a partial write
leaves no readable file.

### Stage 3 — `favthumbs`: sweep (sonnet)

Add `Sweep` to `internal/favthumbs/store.go`.

- Build the expected base-name set from `files`.
- Delete any regular file in `thumbs/` whose base is not expected.
- Guard: for a source that fails to stat, retain files whose `<pathhash>-`
  prefix matches, so an offline volume does not discard good previews.
- A missing `thumbs/` directory is not an error.

Tests: stale entry for a changed file is deleted; an entry for a removed file
is deleted; current entries survive; an unreadable source's previews survive;
a non-preview file in the directory is left alone; missing dir is a no-op.

### Stage 4 — `favthumbs`: `Sync` orchestration (opus — concurrency)

Add `internal/favthumbs/sync.go` with `Sink` and `Sync`.

- Bounded worker pool; check `ctx.Err()` before each expensive step.
- One file's failure is logged by the caller, not fatal to the pass.
- `Sweep` runs once, after the file loop, and is skipped when `ctx` is done.
- Nil `sink` is legal.

Tests: generates only missing previews (pre-seed one, assert one decode);
`sink.Cached` hits skip `imaging.LoadThumbnail` entirely; every file reaches
`sink.Store`; a cancelled context returns early and leaves the sweep undone;
an undecodable file does not abort its peers; running twice writes no second
copy.

### Stage 5 — `grid`: cache accessors (sonnet)

Add to `internal/ui/grid/grid.go`, beside the existing `Cached`:

- `CachedThumb(u fyne.URI) (image.Image, bool)` — `thumbs.Get`.
- `StoreThumb(u fyne.URI, thumb image.Image) bool` — `thumbs.AddIfFits`.
- `ThumbCacheFull() bool` — `thumbs.Bytes() >= thumbs.Budget()`, the bound
  the pre-warm actually needs (see Known limitation above).

Tests in `grid_test.go`: a stored thumbnail is returned; `CachedThumb` misses
for an unknown URI; a thumbnail outweighing the whole budget is refused and
stays uncached; `ThumbCacheFull` flips at the budget; and — pinning the
reason `ThumbCacheFull` exists — two stores under a one-entry budget leave
the *second* cached and evict the first.

### Stage 6a — `preferences`: the toggle's persistence (sonnet)

- Add `FavoritePreviewCache bool` to `preferences.State`, key
  `favoritePreviewCache`.
- Default is **on**, so load must use `BoolWithFallback(key, true)` — plain
  `Bool` would make a fresh install read `false`.

Tests: default load is true; save/load round trips false; round trips true.

### Stage 6b — settings window row (sonnet)

- `settingswin.Host` gains `FavoritePreviewCache() bool` /
  `SetFavoritePreviewCache(bool)`.
- A `widget.Check` form row labelled `lang.L("Cache favorite previews on disk")`
  with hint `lang.L("Reuse saved previews instead of decoding originals")`.
- Add both keys to `translations/en.json` and `translations/de.json`
  (`main_test.go` enforces parity).
- `viewer` field + getter/setter alongside the `memlimits.go` pattern;
  restore in `startup.go`, persist in `run.go`'s `currentPreferences`.

**Do not skip the `currentPreferences` half.** Stage 6a made `Save` write
this key unconditionally — correct, since `false` is a real user choice and
gating the write would make "off" impossible to persist. But that means any
`preferences.State` built without the field carries its zero value, `false`,
straight to disk. `currentPreferences` is exactly such a literal today, so
until it sets `FavoritePreviewCache` from the viewer, **every app shutdown
writes the setting off** and the feature silently disables itself on the
second launch. Harmless right now only because nothing reads the key yet.
Stage 6b must include a test asserting a default-constructed viewer's
`currentPreferences()` reports `FavoritePreviewCache` true.

Tests: toggling the check calls the host setter; the row reflects the host's
current value; `main_test.go` locale parity stays green.

### Stage 7 — `internal/ui`: the cross-feature join (opus — concurrency)

- `internal/ui/favthumbs.go`: `viewer.SyncFavoritePreviews`, a
  `favThumbLifecycle requestLifecycle`, a `favThumbDone chan struct{}`, and
  the `favthumbs.Sink` adapter over `v.grid`. Returns immediately when the
  preference is off. Errors go to `fyne.LogError`.
- `favorites.Host` gains `SyncFavoritePreviews`; call it from
  `writeFavorite` (after a successful save) and `openFavorite` (after load).
- Update every `favorites.Host` implementation in tests.
- Add `favThumbDone` to `drain` in `internal/ui/library_test.go`, per the
  AGENTS.md rule that every new background goroutine joins the drain.

Tests: opening a favorite writes previews under its `thumbs/`; a second open
writes nothing new; the toggle off writes nothing; a superseded open cancels
the previous pass; the drain returns rather than timing out.

### Stage 8 — docs and TODO (sonnet)

- `ARCHITECTURE.md`: add `internal/favthumbs` to the package map, extend the
  `favorites/` and `grid/` rows, and add a "How are favorite previews cached?"
  entry to the where-to-look index.
- `README.md`: one line under settings for the new checkbox.
- `todos.md`: move the item to Done.

## Verification before handoff

```
make fmt
go vet ./...
go build ./...
go test -race ./...
```

Golden screenshots are untouched by this work; do not run `make golden`.
No `git commit` — end with a suggested commit message.
