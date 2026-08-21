package ui

import (
	"github.com/frathe/picfetch/internal/imaging"
)

// settings is every value the Settings window's Host surface reads and
// writes, and nothing else: the app's memory budget, the two geometry caps,
// the folder-scan cap, and the favorite-preview-cache toggle. Grouped so
// that surface reads as the single concern it is, and so run.go's
// currentPreferences copy is a flat field-for-field one.
//
// The three memory limits (imgCacheMB/thumbCacheMB/maxFileMB) are why this
// file exists at all: they have no single consumer to sit beside - the image
// cache is read in load.go, the thumbnail cache lives in internal/ui/grid,
// and the encoded-input ceiling is process-wide state in internal/imaging -
// while together they are one coherent thing, the app's memory budget. The
// other four fields do have a natural home each (maxScan in drop.go,
// maxWinW/maxWinH in load.go, favPreviewCache in favthumbs.go), and their
// getter/setter pairs stay there exactly as before; only the storage moved
// here so the whole settings-backed state is declared in one struct instead
// of flattened across viewer's ~70 fields.
//
// All the megabyte figures are megabytes rather than bytes because that is
// the unit the user types into the settings window and the unit
// internal/preferences round-trips; the conversion to the byte budgets
// internal/imaging actually enforces happens in the setters, which stay
// where their consumers are.
type settings struct {
	// maxScan caps how many images a single recursive folder scan will
	// gather - see handleDrop (drop.go). A field rather than the package
	// var it used to be, so tests shrink it per-viewer instead of
	// mutating a global.
	maxScan int

	// maxWinW/maxWinH cap how large the window is ever allowed to
	// auto-grow to fit a loaded image - see resizeToImage (load.go),
	// which never resizes past them. Fields rather than the constants
	// they used to be, so the settings window can change them per-viewer
	// and tests can shrink/grow them without touching a global.
	maxWinW, maxWinH float32

	// imgCacheMB/thumbCacheMB/maxFileMB are the app's memory budget, in
	// the megabytes the settings window shows - see memlimits.go, which
	// holds their getter/setter pairs and converts each to the byte budget
	// its consumer actually enforces.
	imgCacheMB, thumbCacheMB, maxFileMB int

	// favPreviewCache is the settings window's "Cache favorite previews on
	// disk" checkbox - see favthumbs.go for its getter/setter pair. Restored
	// from preferences.State.FavoritePreviewCache in features.go and read
	// back into it by currentPreferences (run.go).
	favPreviewCache bool
}

// bytesPerMB converts the megabyte figures above into the byte budgets
// imaging.ByteCache and imaging.SetMaxEncodedBytes take.
const bytesPerMB = 1 << 20

// The shipped defaults, derived from internal/imaging's own so there is
// exactly one place each number is chosen - see DefaultImgCacheBytes,
// DefaultThumbCacheBytes and DefaultMaxEncodedBytes for why each is what it
// is. Used by startup preference normalization when nothing was ever saved,
// the same zero-means-unset fallback maxScan and maxWinW/maxWinH use.
const (
	defaultMaxImageCacheMB = imaging.DefaultImgCacheBytes / bytesPerMB
	defaultMaxThumbCacheMB = imaging.DefaultThumbCacheBytes / bytesPerMB
	defaultMaxFileSizeMB   = imaging.DefaultMaxEncodedBytes / bytesPerMB
)

// MaxImageCacheMB/MaxThumbCacheMB/MaxFileSizeMB report the current limits -
// the settings window's getters.
func (v *viewer) MaxImageCacheMB() int { return v.settings.imgCacheMB }
func (v *viewer) MaxThumbCacheMB() int { return v.settings.thumbCacheMB }
func (v *viewer) MaxFileSizeMB() int   { return v.settings.maxFileMB }

// SetMaxImageCacheMB retunes the decoded-image cache's byte budget, evicting
// down to it immediately - the settings window's binding. Floored at 1 MB
// rather than 0 for the reason SetMaxScan floors at 1: a zero budget isn't a
// "no limit" any of this is written to understand. A budget too small to
// hold even one photo is still perfectly well-defined, because ByteCache
// never evicts its most recently added entry - the image on screen stays
// resident, and only its neighbors stop being kept.
func (v *viewer) SetMaxImageCacheMB(n int) {
	if n < 1 {
		n = 1
	}

	v.settings.imgCacheMB = n
	v.imgCache.SetBudget(int64(n) * bytesPerMB)
	imaging.SetMaxVectorRasterPixels(vectorRasterPixelsFor(n))
}

// vectorRasterPixelsFor derives the SVG re-render ceiling from the image
// cache budget: a quarter of the budget's bytes, at 4 bytes per RGBA
// pixel. The re-render raster is live display state rather than a cache
// entry - deliberately never charged to imgCache - so this derivation is
// how the one memory setting the user sees still bounds it.
// imaging.SetMaxVectorRasterPixels applies the floor and ceiling.
func vectorRasterPixelsFor(cacheMB int) int64 {
	return int64(cacheMB) * bytesPerMB / 4 / 4
}

// SetMaxThumbCacheMB retunes the grid's thumbnail cache the same way, through
// the one setter internal/ui/grid exposes. Lowering it while the grid is open
// is safe: a cell whose thumbnail gets evicted just decodes it again the next
// time it scrolls into view.
func (v *viewer) SetMaxThumbCacheMB(n int) {
	if n < 1 {
		n = 1
	}

	v.settings.thumbCacheMB = n
	v.grid.SetCacheBytes(int64(n) * bytesPerMB)
}

// SetMaxFileSizeMB changes the ceiling on a file's encoded size. Unlike the
// two above, the value it writes is process-wide rather than per-viewer (see
// imaging.SetMaxEncodedBytes for why), so the viewer's own field exists only
// to answer the settings window's getter.
func (v *viewer) SetMaxFileSizeMB(n int) {
	if n < 1 {
		n = 1
	}

	v.settings.maxFileMB = n
	imaging.SetMaxEncodedBytes(int64(n) * bytesPerMB)
}
