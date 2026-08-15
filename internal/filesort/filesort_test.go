package filesort

import (
	"image/color"
	"os"
	"slices"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/imagedrop/internal/uitest"
)

// These exercise the orderings directly, without a viewer: the S-key
// cycling and the window-title prefix stay in internal/ui, which is what
// actually owns those behaviours.

// TestModes_MatchesNextsCycleOrder guards the settings window's sort-order
// dropdown: Modes must list every mode in exactly the order Next steps
// through, starting from ByName, so the dropdown's options line up with
// what pressing S repeatedly would produce.
func TestModes_MatchesNextsCycleOrder(t *testing.T) {
	modes := Modes()

	if len(modes) != int(modeCount) {
		t.Fatalf("len(Modes()) = %d, want %d (modeCount)", len(modes), modeCount)
	}

	m := ByName
	for i, want := range modes {
		if m != want {
			t.Errorf("Modes()[%d] = %v, want %v to match Next's cycle order", i, want, m)
		}
		m = m.Next()
	}
	if m != ByName {
		t.Errorf("cycling Next() len(Modes()) times landed on %v, want back to ByName", m)
	}
}

// TestDisplayName_EveryModeHasADistinctNonEmptyName guards the settings
// window's dropdown against two modes silently sharing a label (which
// would make them indistinguishable in the picker) - unlike Label, which
// deliberately returns "" for the default mode, every DisplayName must be
// non-empty since the dropdown has no "no prefix" equivalent of its own.
func TestDisplayName_EveryModeHasADistinctNonEmptyName(t *testing.T) {
	seen := make(map[string]Mode)

	for _, m := range Modes() {
		name := DisplayName(m)
		if name == "" {
			t.Errorf("DisplayName(%v) = \"\", want a non-empty name", m)
		}
		if other, ok := seen[name]; ok {
			t.Errorf("DisplayName(%v) and DisplayName(%v) both = %q, want distinct names", m, other, name)
		}
		seen[name] = m
	}
}

func TestNaturalLess(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"IMG_2.jpg", "IMG_10.jpg", true},
		{"IMG_10.jpg", "IMG_2.jpg", false},
		{"IMG_2.jpg", "IMG_2.jpg", false},
		{"a.jpg", "B.jpg", true},          // case-insensitive
		{"file9.png", "file10.png", true}, // numeric run, not lexical
		{"file09.png", "file9.png", false},
		{"file09.png", "file010.png", true},
		{"img2.jpg", "img.jpg", false}, // longer/digit-suffixed sorts after
		{"img.jpg", "img2.jpg", true},
	}

	for _, c := range cases {
		if got := naturalLess(c.a, c.b); got != c.want {
			t.Errorf("naturalLess(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}

// TestOrderedFiles_SortsByCaptureDate uses controlled Exif dates (rather
// than relying on tie-breaking, like the cycling test above) to check the
// actual comparator: an earlier DateTimeOriginal sorts first, and a file
// with no Exif data at all falls back to its mtime instead of clumping at
// the zero time.
func TestOrderedFiles_SortsByCaptureDate(t *testing.T) {
	mode := ByCaptureDate

	older := storage.NewFileURI(uitest.WriteTempFile(t, "older.jpg", uitest.CaptureDateJPEG(t, 4, 4, "2020:01:01 00:00:00")))
	newer := storage.NewFileURI(uitest.WriteTempFile(t, "newer.jpg", uitest.CaptureDateJPEG(t, 4, 4, "2023:06:15 12:00:00")))
	noExif := uitest.TempJPEGURI(t, "no_exif.jpg", 4, 4, color.White)

	// noExif has no Exif capture date, so it falls back to its mtime - push
	// that later than both Exif dates above so the expected order doesn't
	// depend on when the test happens to run.
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(noExif.Path(), future, future); err != nil {
		t.Fatalf("os.Chtimes: %v", err)
	}

	got := Order(mode, []fyne.URI{newer, noExif, older})

	var names []string
	for _, u := range got {
		names = append(names, u.Name())
	}
	if want := []string{"older.jpg", "newer.jpg", "no_exif.jpg"}; !slices.Equal(names, want) {
		t.Errorf("Order() = %v, want %v", names, want)
	}
}

// TestOrderedFiles_SortsByModTime uses explicit, well-separated os.Chtimes
// values so the expected order doesn't depend on filesystem mtime
// resolution or how fast the test happens to run.
func TestOrderedFiles_SortsByModTime(t *testing.T) {
	mode := ByModTime

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	c := uitest.TempJPEGURI(t, "c.jpg", 4, 4, color.White)

	base := time.Now().Truncate(time.Second)
	for i, u := range []fyne.URI{a, b, c} {
		mt := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(u.Path(), mt, mt); err != nil {
			t.Fatalf("os.Chtimes: %v", err)
		}
	}

	// Handed to orderedFiles newest-touched first, oldest last - it should
	// still come back oldest (a) to newest (c).
	got := Order(mode, []fyne.URI{c, a, b})

	var names []string
	for _, u := range got {
		names = append(names, u.Name())
	}
	if want := []string{"a.jpg", "b.jpg", "c.jpg"}; !slices.Equal(names, want) {
		t.Errorf("Order() = %v, want %v", names, want)
	}
}

// TestOrderedFiles_SortsBySize checks the size comparator directly against
// files of deliberately distinct byte counts, smallest first.
func TestOrderedFiles_SortsBySize(t *testing.T) {
	mode := BySize

	small := storage.NewFileURI(uitest.WriteTempFile(t, "small.dat", make([]byte, 10)))
	medium := storage.NewFileURI(uitest.WriteTempFile(t, "medium.dat", make([]byte, 100)))
	large := storage.NewFileURI(uitest.WriteTempFile(t, "large.dat", make([]byte, 1000)))

	got := Order(mode, []fyne.URI{large, small, medium})

	var names []string
	for _, u := range got {
		names = append(names, u.Name())
	}
	if want := []string{"small.dat", "medium.dat", "large.dat"}; !slices.Equal(names, want) {
		t.Errorf("Order() = %v, want %v", names, want)
	}
}
