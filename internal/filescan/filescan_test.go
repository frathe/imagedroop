package filescan

import (
	"context"
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/uitest"
)

// This file exercises Images directly: no viewer, no Fyne window, and no
// drain machinery - only test.NewApp() (see TestMain below), the same
// minimum internal/filesort's own tests need. Everything that used to
// require a full viewer harness to reach - the symlink-cycle guard, per-call
// dedupe (both within a directory tree and across duplicate dropped URIs),
// the max cap on both the recursive-scan and the loose-file-drop paths, the
// throttled progress callback, and context cancellation - is a plain
// data-in/data-out call here. TestImages_PreservesDropOrder is the one that
// pins this extraction's whole reason for existing: it proves the directory
// stack is genuinely LIFO rather than a queue that happens to look right,
// and that order is what the app's drop-order sort hands to users verbatim.

// TestMain registers the fyne test app so storage.CanList/storage.List's
// "file" scheme is resolvable - without it they fail with "no repository
// registered for scheme 'file'" instead of walking the filesystem.
// internal/filesort and internal/imaging carry this same TestMain for the
// same reason.
func TestMain(m *testing.M) {
	test.NewApp()
	os.Exit(m.Run())
}

func TestImages_EmptyInput(t *testing.T) {
	images, truncated := Images(context.Background(), nil, DefaultMax, nil)
	if len(images) != 0 || truncated {
		t.Fatalf("nil uris: images = %v, truncated = %v, want none and not truncated", images, truncated)
	}

	images, truncated = Images(context.Background(), []fyne.URI{}, DefaultMax, nil)
	if len(images) != 0 || truncated {
		t.Fatalf("empty uris: images = %v, truncated = %v, want none and not truncated", images, truncated)
	}
}

func TestImages_FiltersUnsupportedFiles(t *testing.T) {
	jpegURI := uitest.TempJPEGURI(t, "keep.jpg", 4, 4, color.White)
	pngPath := uitest.WriteTempFile(t, "keep.png", uitest.EncodePNG(t, 4, 4, color.White))

	images, truncated := Images(context.Background(), []fyne.URI{
		jpegURI,
		storage.NewFileURI(pngPath),
		uitest.FakeURI{FileName: "skip.txt", Ext: ".txt"},
		uitest.FakeURI{FileName: "skip.md", Ext: ".md"},
	}, DefaultMax, nil)

	if truncated {
		t.Fatal("truncated = true, want false")
	}
	if len(images) != 2 {
		t.Fatalf("images = %v, want only the jpg and png kept", images)
	}
}

func TestImages_RecursesIntoNestedDirectories(t *testing.T) {
	root := t.TempDir()
	for i := range 3 {
		dir := filepath.Join(root, fmt.Sprintf("sub%d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Non-image clutter that a real photo folder always contains, so
		// the walk is exercised against realistic input rather than a
		// tree of nothing but photos.
		//
		// Note what this does and doesn't prove about Invariant B (the
		// storage.CanList-before-IsSupportedImage ordering): reversing
		// that ordering would still exclude .DS_Store and still descend
		// into the directories, so the assertion below stays green either
		// way. Invariant B is a cost invariant - it exists so a scan
		// doesn't open thousands of files just to learn they aren't
		// images - and cost isn't observable from out here. The guard is
		// the doc comment on that branch in Images, not this test.
		if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("junk"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("photo%d.jpg", i)), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	images, truncated := Images(context.Background(), []fyne.URI{storage.NewFileURI(root)}, DefaultMax, nil)
	if truncated {
		t.Fatal("truncated = true, want false")
	}
	if len(images) != 3 {
		t.Fatalf("images = %v, want the 3 nested photos, none of the .DS_Store junk", images)
	}
	for _, u := range images {
		if u.Name() == ".DS_Store" {
			t.Fatalf(".DS_Store made it into the result: %v", images)
		}
	}
}

// TestImages_SymlinkCycleDoesNotHang guards the visitedDirs check in
// Images: a symlink back to an ancestor directory turns the recursive
// expansion into a cycle (listing root/loop lists root again, including
// root/loop, forever) unless each directory's real, symlink-resolved path
// is tracked and a repeat visit is skipped.
func TestImages_SymlinkCycleDoesNotHang(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skipf("symlinks not supported on this filesystem: %v", err)
	}

	images, truncated := Images(context.Background(), []fyne.URI{storage.NewFileURI(root)}, DefaultMax, nil)
	if len(images) != 1 || truncated {
		t.Fatalf("images = %d, truncated = %v, want the 1 real photo, not one entry per pass through the symlink cycle", len(images), truncated)
	}
}

// TestImages_DedupesOverlappingDirectories drops a folder together with one
// of its own subfolders in the same call - a folder tree reached via two
// different dropped paths - and checks the subfolder's photo isn't counted
// twice in the resulting set.
func TestImages_DedupesOverlappingDirectories(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "top.jpg"), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.jpg"), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
		t.Fatal(err)
	}

	images, truncated := Images(context.Background(), []fyne.URI{storage.NewFileURI(root), storage.NewFileURI(sub)}, DefaultMax, nil)
	if truncated {
		t.Fatal("truncated = true, want false")
	}
	if len(images) != 2 {
		t.Fatalf("images = %v, want top.jpg and nested.jpg once each, not nested.jpg twice from the overlapping drop", images)
	}
}

// TestImages_DedupesDuplicateURIsInDirectDrop covers passing the same file
// twice in one call with no directory anywhere in the input - which an
// os.Args launch or a native chooser's output could in principle produce -
// which should not add it to the result twice.
func TestImages_DedupesDuplicateURIsInDirectDrop(t *testing.T) {
	photo := uitest.TempJPEGURI(t, "photo.jpg", 4, 4, color.White)

	images, truncated := Images(context.Background(), []fyne.URI{photo, photo}, DefaultMax, nil)
	if truncated {
		t.Fatal("truncated = true, want false")
	}
	if len(images) != 1 {
		t.Fatalf("images = %v, want the duplicate URI collapsed to a single entry", images)
	}
}

func TestImages_CapsAtMax(t *testing.T) {
	root := t.TempDir()
	for i := range 5 {
		name := filepath.Join(root, fmt.Sprintf("photo%d.jpg", i))
		if err := os.WriteFile(name, uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	images, truncated := Images(context.Background(), []fyne.URI{storage.NewFileURI(root)}, 3, nil)
	if len(images) != 3 || !truncated {
		t.Fatalf("images = %d, truncated = %v, want exactly 3 and truncated", len(images), truncated)
	}
}

// TestImages_CapAppliesToDirectFileDrop pins a deliberate behavior change
// from the pre-extraction code: handleDrop's old no-directories fast path
// had no cap of its own, only the recursive-scan goroutine did, so a drop
// of loose files could exceed maxScan entirely. One walker now means one
// rule - max bounds a drop of loose files too, with no directory anywhere
// in the input.
func TestImages_CapAppliesToDirectFileDrop(t *testing.T) {
	var uris []fyne.URI
	for i := range 5 {
		uris = append(uris, uitest.TempJPEGURI(t, fmt.Sprintf("photo%d.jpg", i), 4, 4, color.White))
	}

	images, truncated := Images(context.Background(), uris, 3, nil)
	if len(images) != 3 || !truncated {
		t.Fatalf("images = %d, truncated = %v, want exactly 3 and truncated", len(images), truncated)
	}
}

// TestImages_MaxFlooredAtOne guards max < 1 being floored to 1, not treated
// as "unlimited" - a 0 (or negative) cap must still stop the walk after its
// first image, same as SetMaxScan's own floor.
func TestImages_MaxFlooredAtOne(t *testing.T) {
	root := t.TempDir()
	for i := range 5 {
		name := filepath.Join(root, fmt.Sprintf("photo%d.jpg", i))
		if err := os.WriteFile(name, uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	for _, maxFloor := range []int{0, -5} {
		images, truncated := Images(context.Background(), []fyne.URI{storage.NewFileURI(root)}, maxFloor, nil)
		if len(images) != 1 || !truncated {
			t.Errorf("max = %d: images = %d, truncated = %v, want exactly 1 and truncated (0 is not \"unlimited\")", maxFloor, len(images), truncated)
		}
	}
}

// TestImages_PreservesDropOrder pins Invariant A: dirs is a LIFO stack, not
// a queue. This tree is built so a stack and a queue would visit it in a
// different order (see the comment on "want" below) - proving the
// implementation is genuinely a stack, not merely producing an order that
// happens to match by accident. This order is exactly what filesort.Order's
// ByDropOrder mode ("stupid sort") preserves verbatim, so a silent change
// here would silently reorder that mode for every user.
func TestImages_PreservesDropOrder(t *testing.T) {
	root := t.TempDir()
	dirA := filepath.Join(root, "dirA")
	subA := filepath.Join(dirA, "subA")
	dirB := filepath.Join(root, "dirB")
	for _, d := range []string{dirA, subA, dirB} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	write := func(path string) {
		if err := os.WriteFile(path, uitest.EncodeJPEG(t, 2, 2, color.White), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write(filepath.Join(dirA, "a.jpg"))
	write(filepath.Join(subA, "nested.jpg"))
	write(filepath.Join(dirB, "b.jpg"))

	images, truncated := Images(context.Background(), []fyne.URI{storage.NewFileURI(dirA), storage.NewFileURI(dirB)}, DefaultMax, nil)
	if truncated {
		t.Fatal("truncated = true, want false")
	}

	var got []string
	for _, u := range images {
		got = append(got, u.Name())
	}
	// dirA, dirB are pushed in that order; a LIFO pop takes dirB first (its
	// single child, b.jpg, lands before anything from dirA). Popping dirA
	// next visits a.jpg then pushes subA, which is popped (and its
	// nested.jpg visited) before the now-empty stack ends the walk. A FIFO
	// queue would instead produce a.jpg, b.jpg, nested.jpg.
	want := []string{"b.jpg", "a.jpg", "nested.jpg"}
	if !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestImages_ProgressThrottle(t *testing.T) {
	makePhotos := func(t *testing.T, n int) string {
		t.Helper()
		root := t.TempDir()
		for i := range n {
			name := filepath.Join(root, fmt.Sprintf("photo%02d.jpg", i))
			if err := os.WriteFile(name, uitest.EncodeJPEG(t, 2, 2, color.White), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		return root
	}

	t.Run("throttled to the first and every 10th call", func(t *testing.T) {
		root := makePhotos(t, 25)

		var calls []int
		images, truncated := Images(context.Background(), []fyne.URI{storage.NewFileURI(root)}, 1000, func(n int) {
			calls = append(calls, n)
		})

		if len(images) != 25 || truncated {
			t.Fatalf("images = %d, truncated = %v, want 25 images, not truncated", len(images), truncated)
		}
		want := []int{1, 10, 20}
		if !slices.Equal(calls, want) {
			t.Errorf("progress calls = %v, want %v", calls, want)
		}
	})

	t.Run("truncation forces a final call off the every-10th cadence", func(t *testing.T) {
		root := makePhotos(t, 25)

		var calls []int
		images, truncated := Images(context.Background(), []fyne.URI{storage.NewFileURI(root)}, 13, func(n int) {
			calls = append(calls, n)
		})

		if len(images) != 13 || !truncated {
			t.Fatalf("images = %d, truncated = %v, want 13 images, truncated", len(images), truncated)
		}
		want := []int{1, 10, 13}
		if !slices.Equal(calls, want) {
			t.Errorf("progress calls = %v, want %v (final truncation count reported even though 13%%10 != 0)", calls, want)
		}
	})

	t.Run("nil progress is never called and never panics", func(t *testing.T) {
		root := makePhotos(t, 1)

		images, truncated := Images(context.Background(), []fyne.URI{storage.NewFileURI(root)}, DefaultMax, nil)
		if len(images) != 1 || truncated {
			t.Fatalf("images = %d, truncated = %v, want 1 image, not truncated", len(images), truncated)
		}
	})
}

func TestImages_ContextCancellationStopsWalk(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), uitest.EncodeJPEG(t, 2, 2, color.White), 0o644); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	images, truncated := Images(ctx, []fyne.URI{storage.NewFileURI(root)}, DefaultMax, nil)
	if len(images) != 0 || truncated {
		t.Fatalf("images = %d, truncated = %v, want no images gathered from an already-cancelled context", len(images), truncated)
	}
}

// TestImages_ContextCancellationMidWalkStopsWalk covers the ctx check at the
// top of the directory-pop loop, which the already-cancelled case above
// never reaches: that one bails in process before a single directory is
// pushed, so the loop body never runs. This is the check that replaced the
// pre-extraction token.current() test inside the same loop, and it is the
// reason a superseded scan stops walking a large tree instead of racing it
// to completion for a result the caller will discard - so it is worth
// pinning on its own rather than trusting the entry guard to stand in for
// it.
//
// Cancelling from inside the progress callback on the very first image
// makes the stopping point deterministic: every remaining child of the
// directory being listed returns early from process, and the loop then
// returns before popping another directory. One image out of fifty.
func TestImages_ContextCancellationMidWalkStopsWalk(t *testing.T) {
	root := t.TempDir()
	for d := range 5 {
		dir := filepath.Join(root, fmt.Sprintf("sub%d", d))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		for i := range 10 {
			name := filepath.Join(dir, fmt.Sprintf("photo%02d.jpg", i))
			if err := os.WriteFile(name, uitest.EncodeJPEG(t, 2, 2, color.White), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	images, truncated := Images(ctx, []fyne.URI{storage.NewFileURI(root)}, DefaultMax, func(n int) {
		cancel()
	})

	if len(images) != 1 {
		t.Fatalf("images = %d, want the walk to stop at the 1 image gathered before the cancellation, not run on through all 50", len(images))
	}
	if truncated {
		t.Error("truncated = true, want false - the walk was cancelled, not capped")
	}
}
