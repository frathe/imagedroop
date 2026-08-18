package ui

import (
	"github.com/frathe/picfetch/internal/imaging"
)

// The three limits that bound how much memory images are allowed to occupy,
// and the settings window's getter/setter pairs for them. Grouped in one
// file rather than sitting beside their consumers the way MaxScan (drop.go)
// and MaxWindowWidth (load.go) do, because they have no single consumer to
// sit beside - the image cache is read in load.go, the thumbnail cache lives
// in internal/ui/grid, and the encoded-input ceiling is process-wide state
// in internal/imaging - while together they are one coherent thing: the
// app's memory budget.
//
// All three are expressed in megabytes rather than bytes because that is the
// unit the user types into the settings window and the unit
// internal/preferences round-trips; the conversion to the byte budgets
// internal/imaging actually enforces happens in the setters below.

// bytesPerMB converts the megabyte figures above into the byte budgets
// imaging.ByteCache and imaging.SetMaxEncodedBytes take.
const bytesPerMB = 1 << 20

// The shipped defaults, derived from internal/imaging's own so there is
// exactly one place each number is chosen - see DefaultImgCacheBytes,
// DefaultThumbCacheBytes and DefaultMaxEncodedBytes for why each is what it
// is. Used by buildViewer when nothing was ever saved, the same
// zero-means-unset fallback maxScan and maxWinW/maxWinH already use.
const (
	defaultMaxImageCacheMB = imaging.DefaultImgCacheBytes / bytesPerMB
	defaultMaxThumbCacheMB = imaging.DefaultThumbCacheBytes / bytesPerMB
	defaultMaxFileSizeMB   = imaging.DefaultMaxEncodedBytes / bytesPerMB
)

// MaxImageCacheMB/MaxThumbCacheMB/MaxFileSizeMB report the current limits -
// the settings window's getters.
func (v *viewer) MaxImageCacheMB() int { return v.imgCacheMB }
func (v *viewer) MaxThumbCacheMB() int { return v.thumbCacheMB }
func (v *viewer) MaxFileSizeMB() int   { return v.maxFileMB }

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

	v.imgCacheMB = n
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

	v.thumbCacheMB = n
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

	v.maxFileMB = n
	imaging.SetMaxEncodedBytes(int64(n) * bytesPerMB)
}
