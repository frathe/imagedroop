// Package filesort orders a set of image files for display: by natural
// (numeric-aware) file name, by Exif capture date, by modification time, by
// size, or not at all.
//
// It sits beside internal/imaging rather than under internal/ui because
// it draws nothing and knows about no widget - it takes fyne.URI values as
// plain data and touches only the filesystem and the Exif reader. The one
// exception is Label, which is display text and so is translated. That
// placement also
// resolves a knot noted in ARCHITECTURE.md: internal/preferences stores the
// sort mode as one of its own string constants because it could not import
// the enum back when the enum lived in package main, and FromPref/PrefValue
// here are the translation.
package filesort

import (
	"os"
	"sort"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/imagedrop/internal/imaging"
	"github.com/frathe/imagedrop/internal/preferences"
)

// Mode selects how a file set is ordered relative to the raw scan/drop
// order. The S key cycles through the values below in declaration order
// (see Next), wrapping back to ByName; the mode persists across drops and
// is saved/restored via internal/preferences (see FromPref/PrefValue, which
// translate to and from that package's own string constants rather than
// this type directly, so a saved preference stays readable even if this
// enum's order or members ever change).
type Mode int

const (
	ByName        Mode = iota // natural, numeric-aware file-name order (the default)
	ByCaptureDate             // Exif capture date, falling back to mtime when absent
	ByModTime                 // file modification time
	BySize                    // file size, smallest first
	ByDropOrder               // raw scan/drop order, unmodified ("stupid" sort)

	// modeCount is the number of modes above - a sentinel, not an
	// actual mode, so it must stay declared last.
	modeCount
)

// Next returns the mode after m, wrapping back to ByName - the cycle the S
// key steps through.
func (m Mode) Next() Mode {
	return (m + 1) % modeCount
}

// Modes returns every mode in Next's cycle order, for a UI picker like the
// settings window's sort-order dropdown.
func Modes() []Mode {
	return []Mode{ByName, ByCaptureDate, ByModTime, BySize, ByDropOrder}
}

// DisplayName returns m's full, human-readable name, for the settings
// window's sort-order dropdown - unlike Label below, which is a short
// window-title prefix left empty for the default mode, and so unsuited to a
// picker where every mode needs a visible option.
func DisplayName(m Mode) string {
	switch m {
	case ByCaptureDate:
		return lang.L("Capture date")
	case ByModTime:
		return lang.L("Modified date")
	case BySize:
		return lang.L("File size")
	case ByDropOrder:
		return lang.L("Drop order")
	default:
		return lang.L("Name")
	}
}

// Order returns raw arranged according to m. It always returns a fresh
// slice, so callers never alias the input.
//
// The capture-date, modified, and size modes each stat or read every file
// once up front (see sortByInt64Key) rather than rescanning on every
// comparison, but that's still one disk touch per file with no progress
// indicator or way to cancel - fine for a normal drop, but a large
// recursive folder scan will visibly pause here. The same trade-off is
// already accepted, and left unaddressed, for the "Cancel pending decodes"
// case (see todos.md's "not deemed worth implementing" section) - this
// mode just adds three more paths that stat every file instead of one.
func Order(m Mode, raw []fyne.URI) []fyne.URI {
	ordered := append([]fyne.URI(nil), raw...)

	switch m {
	case ByDropOrder:
		// Already in raw order - nothing to do.
	case ByCaptureDate:
		sortByInt64Key(ordered, func(u fyne.URI) int64 { return captureOrModTime(u).UnixNano() })
	case ByModTime:
		sortByInt64Key(ordered, func(u fyne.URI) int64 { return modTimeOf(u).UnixNano() })
	case BySize:
		sortByInt64Key(ordered, fileSizeOf)
	default: // ByName
		sort.SliceStable(ordered, func(i, j int) bool {
			return naturalLess(ordered[i].Name(), ordered[j].Name())
		})
	}

	return ordered
}

// sortByInt64Key stable-sorts files ascending by key(file), computing each
// file's key exactly once up front. Calling key lazily from inside
// sort.Slice's own Less closure would instead invoke it O(n log n) times,
// which matters here since every key func in this file costs a stat or a
// raw file read, not a cheap in-memory comparison.
func sortByInt64Key(files []fyne.URI, key func(fyne.URI) int64) {
	type item struct {
		u fyne.URI
		k int64
	}

	items := make([]item, len(files))
	for i, u := range files {
		items[i] = item{u, key(u)}
	}

	sort.SliceStable(items, func(i, j int) bool {
		return items[i].k < items[j].k
	})

	for i, it := range items {
		files[i] = it.u
	}
}

// statFile stats u's filesystem path, reporting ok=false for anything that
// can't be stat'd (a broken symlink, a permissions error, a file removed
// since the scan) so the mtime/size sort keys below have a well-defined
// fallback instead of erroring the whole sort out.
func statFile(u fyne.URI) (os.FileInfo, bool) {
	info, err := os.Stat(u.Path())
	if err != nil {
		return nil, false
	}
	return info, true
}

// modTimeOf returns u's filesystem modification time, or the zero time if
// it can't be stat'd - which sorts first, same as an unreadable capture
// date or size does for their own modes.
func modTimeOf(u fyne.URI) time.Time {
	if info, ok := statFile(u); ok {
		return info.ModTime()
	}
	return time.Time{}
}

// fileSizeOf returns u's size in bytes, or 0 if it can't be stat'd.
func fileSizeOf(u fyne.URI) int64 {
	if info, ok := statFile(u); ok {
		return info.Size()
	}
	return 0
}

// captureOrModTime returns u's Exif capture date (see
// imaging.CaptureDate), falling back to its filesystem modification time
// for anything with no readable capture date - a screenshot, a PNG, or a
// JPEG that simply carries no DateTimeOriginal/DateTime tag - so the
// capture-date sort mode still produces a sensible, total order instead of
// clumping every such file at the same zero-time position.
func captureOrModTime(u fyne.URI) time.Time {
	if t, ok := imaging.CaptureDate(u); ok {
		return t
	}
	return modTimeOf(u)
}

// Label returns the window-title prefix for m, or "" for the
// default (natural by-name) sort so the common case leaves the title
// exactly as it read before this feature existed. Mirrors the "[merge]"
// prefix mergeMode already uses - see applyTitle, which combines both.
//
// This is the one function here that produces text for a human to read
// rather than data, so it's the one that translates - the English strings
// below are the translation keys (see translations/). It stays with the
// enum it describes rather than moving to the UI: a new mode should need
// one file changed, not two. With no bundle loaded, lang.L returns the key
// itself, which is why the tests read English.
func Label(m Mode) string {
	switch m {
	case ByCaptureDate:
		return lang.L("[sort: date]")
	case ByModTime:
		return lang.L("[sort: modified]")
	case BySize:
		return lang.L("[sort: size]")
	case ByDropOrder:
		return lang.L("[unsorted]")
	default:
		return ""
	}
}

// FromPref maps a saved internal/preferences.State.SortMode string
// back to this package's Mode enum, defaulting to ByName (the
// shipped default) for an empty or unrecognized value - covering both
// "nothing saved yet" and a value from a future version this build doesn't
// know about.
func FromPref(s string) Mode {
	switch s {
	case preferences.SortByCaptureDate:
		return ByCaptureDate
	case preferences.SortByModTime:
		return ByModTime
	case preferences.SortBySize:
		return BySize
	case preferences.SortByDropOrder:
		return ByDropOrder
	default:
		return ByName
	}
}

// PrefValue is FromPref's inverse, for run.go's SetOnStopped save.
func (m Mode) PrefValue() string {
	switch m {
	case ByCaptureDate:
		return preferences.SortByCaptureDate
	case ByModTime:
		return preferences.SortByModTime
	case BySize:
		return preferences.SortBySize
	case ByDropOrder:
		return preferences.SortByDropOrder
	default:
		return preferences.SortByName
	}
}

// naturalLess reports whether a should sort before b using natural order,
// so "IMG_2.jpg" sorts before "IMG_10.jpg": runs of ASCII digits compare
// numerically (ignoring leading zeros), everything else compares
// case-insensitively one byte at a time. A run is only consumed where both
// strings have a digit at the current position, so "img.jpg" and
// "img2.jpg" - which disagree on where the digit run starts - still
// compare byte for byte instead of misaligning on run boundaries.
func naturalLess(a, b string) bool {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		if isDigit(a[i]) && isDigit(b[j]) {
			da, _ := splitRun(a[i:], isDigit)
			db, _ := splitRun(b[j:], isDigit)

			ta := strings.TrimLeft(da, "0")
			tb := strings.TrimLeft(db, "0")
			if len(ta) != len(tb) {
				return len(ta) < len(tb)
			}
			if ta != tb {
				return ta < tb
			}

			i += len(da)
			j += len(db)
			continue
		}

		if la, lb := toLower(a[i]), toLower(b[j]); la != lb {
			return la < lb
		}
		i++
		j++
	}
	return len(a)-i < len(b)-j
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func toLower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + ('a' - 'A')
	}
	return c
}

// splitRun splits off the leading run of s for which keep holds true.
func splitRun(s string, keep func(byte) bool) (run, rest string) {
	i := 0
	for i < len(s) && keep(s[i]) {
		i++
	}
	return s[:i], s[i:]
}
