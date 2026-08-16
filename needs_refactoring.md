# Refactoring needed: byte-bounded image memory

## Finding

Image memory is bounded by entry counts, not by decoded size. This makes the
current limits unreliable for large photographs and animations:

- `maxImagePixels` permits a 200-megapixel image, whose RGBA pixels alone can
  require about 800 MB.
- `imgCacheSize` retains 16 decoded images. At the permitted maximum that is
  theoretically more than 12 GB; even ordinary 24-megapixel photos can put the
  cache around 1.5 GB.
- `thumbCacheSize` retains 4096 thumbnails. At the 200-by-200 maximum, RGBA
  storage can approach 625 MB before cache and image-object overhead.
- Animated GIFs retain a complete composited RGBA canvas for every frame, with
  no cumulative decoded-pixel budget. The per-frame dimension check does not
  protect against a file containing many frames.
- `readRawBytes` uses `io.ReadAll`, so encoded input also has no byte limit and
  contributes to peak memory while decoding.

This is a resource-exhaustion risk for unusually large or adversarial files and
can also cause avoidable memory pressure during normal browsing. Changing one
entry-count constant would only move the threshold; it would not make memory
usage predictable.

## Recommended refactor

1. Replace both entry-count LRUs with byte-weighted caches. Record an estimated
   decoded cost for each static image, animation, and thumbnail, and evict until
   a configurable byte budget is met. Always allow the currently displayed
   image even when it alone exceeds the cache budget, but do not retain
   oversized neighbors.
2. Add a cumulative decoded-pixel/byte limit for animations before creating the
   composited RGBA frame copies. Ideally decode or composite animation frames
   incrementally instead of retaining every full canvas indefinitely.
3. Add an encoded-input limit around `readRawBytes`, returning a typed error that
   the UI can explain. Pick the limit from real supported camera files rather
   than from an arbitrary small default.
4. Downsample very large still images toward the actual display/zoom need before
   caching them. Preserve a deliberate path for full-resolution operations such
   as saving.
5. Add tests that assert cache eviction by byte weight, cumulative animation
   rejection, oversized encoded-input rejection, and successful display of one
   image larger than the cache budget.

## Completion criteria

- Cache memory has explicit byte budgets and tests proving those budgets.
- A many-frame animation cannot allocate unbounded full-canvas copies.
- Oversized input fails with a user-facing, non-crashing error.
- Neighbor preloading cannot multiply one oversized image into several retained
  oversized decodes.

