package favthumbs

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"fyne.io/fyne/v2/storage"
)

func TestEntryNameStableAcrossCalls(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "a.jpg")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := storage.NewFileURI(path)

	first, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName(%q) reported false", path)
	}
	second, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName(%q) reported false on second call", path)
	}
	if first != second {
		t.Errorf("EntryName not stable: first %q, second %q", first, second)
	}
}

func TestEntryNameDiffersForDifferentPaths(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	pathA := filepath.Join(dir, "a.jpg")
	pathB := filepath.Join(dir, "b.jpg")
	if err := os.WriteFile(pathA, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(pathB, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}

	nameA, ok := EntryName(storage.NewFileURI(pathA))
	if !ok {
		t.Fatalf("EntryName(%q) reported false", pathA)
	}
	nameB, ok := EntryName(storage.NewFileURI(pathB))
	if !ok {
		t.Fatalf("EntryName(%q) reported false", pathB)
	}
	if nameA == nameB {
		t.Errorf("EntryName did not differ for distinct paths: both %q", nameA)
	}
}

func TestEntryNameChangesWhenModTimeChanges(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "a.jpg")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := storage.NewFileURI(path)

	before, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName(%q) reported false", path)
	}

	newTime := time.Now().Add(2 * time.Hour)
	if err := os.Chtimes(path, newTime, newTime); err != nil {
		t.Fatal(err)
	}

	after, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName(%q) reported false after Chtimes", path)
	}
	if before == after {
		t.Errorf("EntryName unchanged after mod time change: %q", before)
	}
}

func TestEntryNameChangesWhenSizeChanges(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "a.jpg")
	if err := os.WriteFile(path, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	src := storage.NewFileURI(path)

	before, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName(%q) reported false", path)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(" world"); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	after, ok := EntryName(src)
	if !ok {
		t.Fatalf("EntryName(%q) reported false after append", path)
	}
	if before == after {
		t.Errorf("EntryName unchanged after size change: %q", before)
	}
}

func TestEntryNameMissingFileReportsFalse(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "missing.jpg")
	name, ok := EntryName(storage.NewFileURI(path))
	if ok {
		t.Fatalf("EntryName(%q) = %q, true; want false", path, name)
	}
	if name != "" {
		t.Errorf("EntryName(%q) name = %q, want \"\"", path, name)
	}
}

func TestEntryNameNilURIReportsFalse(t *testing.T) {
	t.Parallel()

	name, ok := EntryName(nil)
	if ok {
		t.Fatalf("EntryName(nil) = %q, true; want false", name)
	}
	if name != "" {
		t.Errorf("EntryName(nil) name = %q, want \"\"", name)
	}
}

func TestDirJoinsSubDirOntoFavoriteDirectory(t *testing.T) {
	t.Parallel()

	favDir := filepath.Join(t.TempDir(), "Trip")
	want := filepath.Join(favDir, SubDir)
	if got := Dir(favDir); got != want {
		t.Errorf("Dir(%q) = %q, want %q", favDir, got, want)
	}
}
