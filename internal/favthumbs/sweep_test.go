package favthumbs

import (
	"image/color"
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2"
)

func TestSweepDeletesStalePreviewForChangedSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	thumb := newOpaqueThumb(2, 2, color.RGBA{R: 5, G: 6, B: 7, A: 255})
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}

	oldName, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName reported false")
	}
	oldPath := filepath.Join(Dir(favDir), oldName+".jpg")

	newTime := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(src.Path(), newTime, newTime); err != nil {
		t.Fatal(err)
	}

	if err := Sweep(favDir, []fyne.URI{src}); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if fileExists(oldPath) {
		t.Errorf("expected stale preview at %q to be deleted", oldPath)
	}
}

func TestSweepDeletesPreviewForRemovedSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	thumb := newOpaqueThumb(2, 2, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}
	name, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName reported false")
	}
	previewPath := filepath.Join(Dir(favDir), name+".jpg")

	// src is no longer part of the favorite's file list at all - not just
	// changed, gone from the set - so nothing should keep its preview.
	if err := Sweep(favDir, []fyne.URI{}); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if fileExists(previewPath) {
		t.Errorf("expected preview at %q for a removed source to be deleted", previewPath)
	}
}

func TestSweepKeepsCurrentPreview(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	thumb := newOpaqueThumb(2, 2, color.RGBA{R: 4, G: 5, B: 6, A: 255})
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}
	name, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName reported false")
	}
	previewPath := filepath.Join(Dir(favDir), name+".jpg")

	if err := Sweep(favDir, []fyne.URI{src}); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if !fileExists(previewPath) {
		t.Errorf("expected current preview at %q to survive Sweep", previewPath)
	}
}

// TestSweepKeepsPreviewsForUnstatableSource pins down the offline-volume
// guard: a source that cannot be stat-ed right now (network share
// unmounted, removable media unplugged) has an unknown current version, so
// Sweep must not treat every preview that could belong to it as stale. The
// preview's own embedded mtime/size no longer matching anything is exactly
// the situation the guard exists for.
func TestSweepKeepsPreviewsForUnstatableSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	thumb := newOpaqueThumb(2, 2, color.RGBA{R: 7, G: 8, B: 9, A: 255})
	if err := Write(favDir, src, thumb); err != nil {
		t.Fatalf("Write: %v", err)
	}
	name, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName reported false")
	}
	previewPath := filepath.Join(Dir(favDir), name+".jpg")

	if err := os.Remove(src.Path()); err != nil {
		t.Fatal(err)
	}

	// src is still listed among the favorite's files, but can no longer be
	// stat-ed - the offline-volume case.
	if err := Sweep(favDir, []fyne.URI{src}); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if !fileExists(previewPath) {
		t.Errorf("expected preview at %q for an unstat-able source to survive Sweep", previewPath)
	}
}

func TestSweepLeavesNonPreviewFileAlone(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	favDir := filepath.Join(dir, "Trip")
	if err := os.MkdirAll(Dir(favDir), 0o755); err != nil {
		t.Fatal(err)
	}
	notesPath := filepath.Join(Dir(favDir), "notes.txt")
	if err := os.WriteFile(notesPath, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Sweep(favDir, []fyne.URI{}); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if !fileExists(notesPath) {
		t.Errorf("expected non-preview file at %q to survive Sweep", notesPath)
	}
}

func TestSweepMissingThumbsDirIsNoOp(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	favDir := filepath.Join(dir, "Trip")

	if _, err := os.Stat(Dir(favDir)); !os.IsNotExist(err) {
		t.Fatalf("thumbs dir already exists before Sweep: %v", err)
	}

	if err := Sweep(favDir, []fyne.URI{}); err != nil {
		t.Fatalf("Sweep on missing thumbs dir = %v, want nil", err)
	}
}

// TestSweepDeletesLeftoverTempFile pins down that a temp file abandoned by
// an interrupted Write (os.CreateTemp(dir, ".favthumb-*"+ext) that never
// reached its Rename) is not special-cased: its base name is never in the
// expected set for any real source, and its extension is a preview
// extension, so plain candidate rules already catch it.
func TestSweepDeletesLeftoverTempFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	favDir := filepath.Join(dir, "Trip")
	if err := os.MkdirAll(Dir(favDir), 0o755); err != nil {
		t.Fatal(err)
	}
	tempPath := filepath.Join(Dir(favDir), ".favthumb-abc123.jpg")
	if err := os.WriteFile(tempPath, []byte("partial"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := Sweep(favDir, []fyne.URI{}); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if fileExists(tempPath) {
		t.Errorf("expected leftover temp file at %q to be swept", tempPath)
	}
}

func TestSweepWithEmptyFilesDeletesEveryPreview(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	favDir := filepath.Join(dir, "Trip")

	srcA := newSourceFile(t, dir, "a.jpg")
	srcB := newSourceFile(t, dir, "b.jpg")
	if err := Write(favDir, srcA, newOpaqueThumb(2, 2, color.RGBA{R: 1, G: 1, B: 1, A: 255})); err != nil {
		t.Fatalf("Write a: %v", err)
	}
	if err := Write(favDir, srcB, newOpaqueThumb(2, 2, color.RGBA{R: 2, G: 2, B: 2, A: 255})); err != nil {
		t.Fatalf("Write b: %v", err)
	}

	// The favorite's file list is now empty - every preview under it should
	// go, not just the ones for sources that were merely removed from a
	// still-nonempty list.
	if err := Sweep(favDir, []fyne.URI{}); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	entries, err := os.ReadDir(Dir(favDir))
	if err != nil {
		t.Fatalf("ReadDir(thumbs): %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, len(entries))
		for i, e := range entries {
			names[i] = e.Name()
		}
		t.Errorf("thumbs dir has %d entries after Sweep with empty files, want 0: %v", len(entries), names)
	}
}

func TestSweepDeletesPNGPreviewForRemovedSource(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	src := newSourceFile(t, dir, "a.jpg")
	favDir := filepath.Join(dir, "Trip")

	// newTransparentThumb forces Write onto the .png path.
	if err := Write(favDir, src, newTransparentThumb(2, 2)); err != nil {
		t.Fatalf("Write: %v", err)
	}
	name, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName reported false")
	}
	previewPath := filepath.Join(Dir(favDir), name+".png")
	if !fileExists(previewPath) {
		t.Fatalf("expected .png preview at %q right after Write", previewPath)
	}

	if err := Sweep(favDir, []fyne.URI{}); err != nil {
		t.Fatalf("Sweep: %v", err)
	}

	if fileExists(previewPath) {
		t.Errorf("expected .png preview at %q for a removed source to be deleted", previewPath)
	}
}
