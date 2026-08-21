package ui

import (
	"testing"

	"github.com/frathe/picfetch/internal/imaging"
)

// memlimits.go groups three separately enforced megabyte figures - the
// decoded-image cache budget, the thumbnail cache budget and the
// encoded-input ceiling - because no one of them has a consumer to sit
// beside (they are read in load.go, internal/ui/grid and internal/imaging
// respectively) while together they are the one memory budget the user
// sees. What this file tests is that grouping as the settings
// window sees it: the getters report the current figures, the setters retune
// the thing each limit actually enforces and floor at 1 rather than accept a
// "no limit" this code was never written to understand.
//
// The behavior those budgets produce - an image bigger than the whole cache
// budget still displaying, a too-large neighbor skipped by the preloader, an
// oversized file refused outright - is tested in imgcache_test.go instead;
// this file stops at the binding surface.

func TestMemoryLimitGettersAndSetters(t *testing.T) {
	v := newTestViewer(t)
	t.Cleanup(func() { imaging.SetMaxEncodedBytes(0) }) // process-wide - see memlimits.go

	if got, want := v.MaxImageCacheMB(), defaultMaxImageCacheMB; got != want {
		t.Errorf("MaxImageCacheMB() = %d, want the shipped default %d", got, want)
	}
	if got, want := v.MaxThumbCacheMB(), defaultMaxThumbCacheMB; got != want {
		t.Errorf("MaxThumbCacheMB() = %d, want the shipped default %d", got, want)
	}
	if got, want := v.MaxFileSizeMB(), defaultMaxFileSizeMB; got != want {
		t.Errorf("MaxFileSizeMB() = %d, want the shipped default %d", got, want)
	}

	// Each setter has to reach past the viewer's own bookkeeping field to
	// the thing that actually enforces the limit.
	v.SetMaxImageCacheMB(64)
	if got, want := v.MaxImageCacheMB(), 64; got != want {
		t.Errorf("MaxImageCacheMB() = %d, want %d", got, want)
	}
	if got, want := v.imgCache.Budget(), int64(64*bytesPerMB); got != want {
		t.Errorf("imgCache.Budget() = %d, want %d", got, want)
	}

	v.SetMaxThumbCacheMB(32)
	if got, want := v.MaxThumbCacheMB(), 32; got != want {
		t.Errorf("MaxThumbCacheMB() = %d, want %d", got, want)
	}

	v.SetMaxFileSizeMB(16)
	if got, want := v.MaxFileSizeMB(), 16; got != want {
		t.Errorf("MaxFileSizeMB() = %d, want %d", got, want)
	}
	if got, want := imaging.MaxEncodedBytes(), int64(16*bytesPerMB); got != want {
		t.Errorf("imaging.MaxEncodedBytes() = %d, want %d", got, want)
	}
}

// A zero or negative megabyte figure isn't a "no limit" any of this is
// written to understand, so every setter floors at 1 - the same guard
// SetMaxScan makes for the scan cap.
func TestMemoryLimitSetters_FloorAtOne(t *testing.T) {
	v := newTestViewer(t)
	t.Cleanup(func() { imaging.SetMaxEncodedBytes(0) })

	for _, n := range []int{0, -5} {
		v.SetMaxImageCacheMB(n)
		v.SetMaxThumbCacheMB(n)
		v.SetMaxFileSizeMB(n)

		if got := v.MaxImageCacheMB(); got != 1 {
			t.Errorf("MaxImageCacheMB() = %d after SetMaxImageCacheMB(%d), want 1", got, n)
		}
		if got := v.MaxThumbCacheMB(); got != 1 {
			t.Errorf("MaxThumbCacheMB() = %d after SetMaxThumbCacheMB(%d), want 1", got, n)
		}
		if got := v.MaxFileSizeMB(); got != 1 {
			t.Errorf("MaxFileSizeMB() = %d after SetMaxFileSizeMB(%d), want 1", got, n)
		}
	}
}

// The SVG re-render raster is deliberately never charged to imgCache (it is
// live display state, not a cache entry), so honoring the user's memory
// setting means deriving its ceiling from the budget instead: a quarter of
// the budget's bytes at 4 B per RGBA pixel, clamped by imaging's own
// floor and default ceiling.
func TestSetMaxImageCacheMBRetunesTheVectorRasterCeiling(t *testing.T) {
	v := newTestViewer(t)
	t.Cleanup(func() { imaging.SetMaxVectorRasterPixels(imaging.DefaultMaxVectorRasterPixels) })

	// 256 MB / 4 (a quarter of the budget) / 4 B per RGBA px = 16,777,216.
	v.SetMaxImageCacheMB(256)
	if got := imaging.MaxVectorRasterPixels(); got != 16_777_216 {
		t.Errorf("after 256 MB: ceiling = %d, want 16777216", got)
	}

	// A tiny budget lands on the floor rather than making SVGs unusable...
	v.SetMaxImageCacheMB(64)
	if got := imaging.MaxVectorRasterPixels(); got != 8_000_000 {
		t.Errorf("after 64 MB: ceiling = %d, want the 8000000 floor", got)
	}

	// ...and a huge one never exceeds the shipped 32 MP behavior.
	v.SetMaxImageCacheMB(4096)
	if got := imaging.MaxVectorRasterPixels(); got != imaging.DefaultMaxVectorRasterPixels {
		t.Errorf("after 4096 MB: ceiling = %d, want the %d default ceiling", got, int64(imaging.DefaultMaxVectorRasterPixels))
	}
}
