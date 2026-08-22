package grid

import (
	"image/color"
	"os"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/uitest"
)

func TestMain(m *testing.M) {
	// The overlay is built from real widgets, so these need an app for the
	// theme and driver. Each test still gets its own window (see
	// newOverview) - New takes one to maximize on open - but the fyne test
	// driver's windows are not driver.NativeWindow, so winpos.Maximize
	// degrades to a no-op there, the same as every other winpos call this
	// app makes under test.
	test.NewApp()
	os.Exit(m.Run())
}

// fakeHost stands in for the viewer. It records the display actions the
// grid asks for, so a selection or a close can be observed without a real
// window behind it.
type fakeHost struct {
	files []fyne.URI
	index int
	gen   uint64

	// mods is what Modifiers reports - the keyboard state a tap is read
	// against, which a Fyne tap event carries none of. Set around a
	// wrap.Select call to stand in for holding a key while clicking (see
	// the click helper in selection_test.go).
	mods fyne.KeyModifier

	shown     []int
	repaints  int
	unfocused int

	// highlighted records every file index the grid reported for the ring
	// (-1 for "none"), so the window-title notification can be asserted in
	// order without a real title bar.
	highlighted []int
}

func (f *fakeHost) FileCount() int         { return len(f.files) }
func (f *fakeHost) FileAt(i int) fyne.URI  { return f.files[i] }
func (f *fakeHost) CurrentIndex() int      { return f.index }
func (f *fakeHost) Generation() uint64     { return f.gen }
func (f *fakeHost) ShowImage(i int)        { f.shown = append(f.shown, i) }
func (f *fakeHost) ForceRepaint()          { f.repaints++ }
func (f *fakeHost) Unfocus()               { f.unfocused++ }
func (f *fakeHost) HighlightChanged(i int) { f.highlighted = append(f.highlighted, i) }

func (f *fakeHost) Modifiers() fyne.KeyModifier { return f.mods }

// hostWith returns a host holding n small real JPEGs - real files because
// the decode path under test actually reads them.
func hostWith(t *testing.T, names ...string) *fakeHost {
	t.Helper()

	uris := make([]fyne.URI, 0, len(names))
	for _, name := range names {
		uris = append(uris, uitest.TempJPEGURI(t, name, 8, 8, color.White))
	}

	return &fakeHost{files: uris}
}

// newOverview builds an Overview over host and a real (test-driver)
// window, closing the window when the test ends - the fixture behind every
// New call in this file, now that New needs a window to maximize on open.
func newOverview(t *testing.T, host Host) *Overview {
	t.Helper()

	win := test.NewWindow(nil)
	t.Cleanup(win.Close)

	return New(host, win)
}

// last is the most recent index the grid reported for the ring, or -2 when
// it never reported anything - distinct from the -1 that means "nothing
// highlighted".
func (f *fakeHost) last() int {
	if len(f.highlighted) == 0 {
		return -2
	}

	return f.highlighted[len(f.highlighted)-1]
}

// openGrid builds an overview over the named files, warms every thumbnail
// (so no cell spawns a background decode, see Warm) and opens it - the
// starting state for every search test below.
func openGrid(t *testing.T, names ...string) (*Overview, *fakeHost) {
	t.Helper()

	host := hostWith(t, names...)
	g := newOverview(t, host)
	if err := g.Warm(); err != nil {
		t.Fatalf("Warm: %v", err)
	}
	g.Toggle()

	return g, host
}

// typeQuery opens search and types q into it, one rune at a time - the way
// the canvas's typed-rune callback delivers them.
func typeQuery(g *Overview, q string) {
	g.HandleRune('/')
	for _, r := range q {
		g.HandleRune(r)
	}
}
