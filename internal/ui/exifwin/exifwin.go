// Package exifwin is the EXIF metadata window: a small panel listing the
// current image's camera settings, opened with the E key or the info
// overlay's "Show EXIF data" link. Below the list sits a collapsible
// OpenStreetMap view, shown only for a photo that carries GPS tags and
// collapsed until the user expands it - which is also what keeps the widget
// from fetching any map tiles unasked.
//
// It takes no host interface, only a `current` func: everything it needs
// from the app is "which file is on screen, if any", and one accessor is a
// smaller, more honest dependency than an interface with a single method.
package exifwin

import (
	"context"
	"fmt"
	"image/color"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	xwidget "fyne.io/x/fyne/widget"

	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/ui/widgets"
)

const (
	exifW = 420.0
	exifH = 360.0

	// mapH is the least the map opens at, and mapZoom how far in it
	// starts: close enough to read the streets around the pin, far enough
	// to place it in its town. Beyond mapH the map follows the window.
	mapH    = 240.0
	mapZoom = 15
)

// Window is the EXIF panel. At most one is open at a time (widgets.
// Singleton): a second request raises the existing window rather than
// stacking up duplicates.
type Window struct {
	app fyne.App

	// current reports the file whose metadata should be shown, and whether
	// there is one at all - the app's "currently displayed image", which
	// changes under this window as the user navigates.
	current func() (fyne.URI, bool)

	win widgets.Singleton

	// text is the panel's content label, live only while the window is
	// open (nil otherwise, which is what makes Refresh a no-op then).
	text *widget.Label

	// locationMap and location are the OpenStreetMap view and the
	// collapsible section holding it, both live only while the window is
	// open. The section is hidden entirely for a photo with no GPS tags,
	// and starts collapsed otherwise: no tiles are fetched until the user
	// asks to see them.
	locationMap *xwidget.Map
	location    *fyne.Container
	toggle      *widget.Button
	body        *fyne.Container
	loading     *fyne.Container

	// expanded is whether the user has opened the section; lat/lon/hasPos
	// are the position the current image carries, kept so expanding later
	// knows where to point without re-reading the file.
	expanded bool
	lat, lon float64
	hasPos   bool

	// tiles downloads and caches the map's tiles off the UI goroutine -
	// see tiles.go for why the widget's own fetching can't be left to it.
	// warming and warmGen track the prefetch that fills the first view,
	// warmDone lets tests wait for it.
	tiles    *tileFetcher
	warming  bool
	warmGen  int
	warmDone chan struct{}
}

// New returns the EXIF window for application. current is called on every
// open and refresh to find the file to read.
func New(application fyne.App, current func() (fyne.URI, bool)) *Window {
	return &Window{
		app:     application,
		current: current,
		tiles:   newTileFetcher(osmTiles, nil),
	}
}

// Show opens the panel, or raises it and syncs it to the current image if
// it's already open. A no-op when nothing is displayed, since there's no
// file to read metadata from.
func (w *Window) Show() {
	if _, ok := w.current(); !ok {
		return
	}

	// Raising an already-open window must first sync it to whatever image
	// is now current; Refresh no-ops while the window isn't open yet (text
	// nil), so the fresh-window path below isn't affected.
	w.Refresh()

	w.win.Show(w.app, lang.L("EXIF Data"), fyne.NewSize(exifW, exifH), func() fyne.CanvasObject {
		w.text = widget.NewLabel("")
		w.text.Wrapping = fyne.TextWrapWord

		w.buildLocation()
		w.Refresh()

		// Border, not a scrolled box: the metadata takes the height it
		// needs at the top and the map section gets everything below it,
		// so dragging the window taller makes the map taller with it.
		// Nothing here needs to scroll - the panel's minimum size already
		// covers the longest the metadata gets.
		return container.NewBorder(container.NewPadded(w.text), nil, nil, nil, w.location)
	}, func() {
		w.tiles.SetOnChange(nil)

		// Anything a prefetch still has in flight belongs to a window that
		// no longer exists.
		w.warmGen++

		w.text = nil
		w.locationMap = nil
		w.location = nil
		w.toggle = nil
		w.body = nil
		w.loading = nil
		w.expanded = false
		w.warming = false
	})
}

// Refresh re-reads the current file's raw bytes and updates the panel from
// them. A no-op while the window isn't open. Called both from Show (opening,
// or raising an already-open window onto whatever image is now current) and
// by the app's finishLoad, so navigating to a different image while the
// window is up keeps it in sync instead of showing a stale file's metadata.
//
// Re-reading from disk here rather than keeping the raw bytes from the
// original decode around is a deliberate trade: the image cache only ever
// holds decoded pixels (see its own size comment), and the EXIF window is an
// on-demand, comparatively rare action - not worth doubling every cached
// entry's memory with raw file bytes it usually never needs.
func (w *Window) Refresh() {
	u, ok := w.current()
	if w.text == nil || !ok {
		return
	}

	// context.Background(): this is a quick, on-demand, synchronous re-read
	// for the EXIF panel, not part of the cancellable load/preload chain
	// internal/ui's ShowImage/attemptLoad/preloadOne share a generation's
	// context for.
	data, _, err := imaging.ReadAndProbe(context.Background(), u)
	if err != nil {
		w.text.SetText(lang.L("Could not read this file's metadata."))
		w.showLocation(imaging.Metadata{})
		return
	}

	m := imaging.ReadMetadata(data)

	w.text.SetText(formatExifMetadata(m))
	w.showLocation(m)
}

// buildLocation assembles the collapsible location section: a disclosure
// button, and under it the map with a loading indicator stacked over it.
//
// It is a hand-rolled disclosure rather than a widget.Accordion because
// expanding is the moment the first tiles may be downloaded, and Accordion
// offers no way to be told when that happens - the whole point of this
// section is that nothing is fetched until the user asks for it.
func (w *Window) buildLocation() {
	w.locationMap = xwidget.NewMapWithOptions(
		xwidget.WithOsmTiles(),
		xwidget.WithTileSource(w.tiles.template),
		xwidget.WithHTTPClient(w.tiles.client()),
		xwidget.WithZoomButtons(true),
		xwidget.WithScrollButtons(false),
		xwidget.AtZoomLevel(mapZoom),
	)

	// A tile that arrives after the frame that asked for it only reaches
	// the screen if the map is told to redraw - see tiles.go. Redrawing
	// once the batch is in, rather than per tile, is what keeps a pan
	// across a dozen new tiles from queueing a dozen repaints of a map
	// that is still mostly holes.
	w.tiles.SetOnChange(func(pending int) {
		if pending > 0 {
			return
		}

		fyne.Do(func() {
			if w.locationMap == nil {
				return
			}

			w.syncLoading()
			w.locationMap.Refresh()
		})
	})

	spinner := widget.NewProgressBarInfinite()
	w.loading = container.NewCenter(container.NewVBox(widget.NewLabel(lang.L("Loading map…")), spinner))
	w.loading.Hide()

	// The map's MinSize is a single tile, so a transparent rectangle
	// stacked behind it gives the section a floor to open at; above that
	// the map grows with the window - see the panel's content layout.
	spacer := canvas.NewRectangle(color.Transparent)
	spacer.SetMinSize(fyne.NewSize(0, mapH))

	w.body = container.NewStack(spacer, w.locationMap, w.loading)
	w.body.Hide()

	w.toggle = widget.NewButtonWithIcon(lang.L("Location"), theme.MenuExpandIcon(), w.toggleLocation)
	w.toggle.Alignment = widget.ButtonAlignLeading
	w.toggle.Importance = widget.LowImportance

	w.location = container.NewBorder(w.toggle, nil, nil, nil, w.body)
}

// toggleLocation opens or closes the section. Opening is what starts the
// download of the tiles around the capture position, and what puts the
// loading indicator up until they are all in.
func (w *Window) toggleLocation() {
	w.expanded = !w.expanded

	if w.expanded {
		w.toggle.SetIcon(theme.MenuDropDownIcon())
		w.body.Show()

		// Showing a child doesn't re-run its parent's layout, and a hidden
		// child is given no space at all - without this the map would be
		// revealed at zero height, and so never drawn.
		w.location.Refresh()
		w.startWarm()

		return
	}

	w.toggle.SetIcon(theme.MenuExpandIcon())
	w.body.Hide()
	w.location.Refresh()
}

// startWarm downloads the block of tiles around the current position in
// the background, showing the loading indicator until they land. Its own
// generation counter is what keeps a prefetch for an image the user has
// already navigated away from - or for a window they have since closed -
// from touching anything when it finishes.
func (w *Window) startWarm() {
	if !w.hasPos || w.locationMap == nil {
		return
	}

	w.warmGen++
	gen := w.warmGen
	lat, lon := w.lat, w.lon

	done := make(chan struct{})
	w.warmDone = done

	w.warming = true

	// Drawing the map before its tiles are cached would have it ask for
	// every one of them and get nothing (see tiles.go), logging a failure
	// per tile per frame and showing a grid of holes. Keeping it hidden
	// until the block is in trades that for a spinner and one clean frame.
	w.locationMap.Hide()
	w.syncLoading()

	tiles := w.tiles

	go func() {
		tiles.Warm(lat, lon, mapZoom)

		fyne.Do(func() {
			defer close(done)

			if gen != w.warmGen || w.locationMap == nil {
				return
			}

			w.warming = false
			w.syncLoading()
			w.locationMap.Show()

			// Revealing the map has to re-run the stack's layout for the
			// same reason expanding the section does.
			w.body.Refresh()
		})
	}()
}

// syncLoading shows the indicator while the first view is still being
// prefetched or any tile a pan or zoom asked for is still on its way, and
// hides it once nothing is outstanding.
func (w *Window) syncLoading() {
	if w.loading == nil {
		return
	}

	if w.warming || w.tiles.Pending() > 0 {
		w.loading.Show()
		return
	}

	w.loading.Hide()
}

// showLocation points the map at m's capture position and reveals the
// section holding it, or hides the section entirely when m carries no GPS
// tags - most photos don't, and an empty map of the Atlantic is worse than
// no map at all. The section is left however the user set it: a photo that
// still has a position doesn't re-collapse an expanded map out from under
// them, only a fresh window starts collapsed.
func (w *Window) showLocation(m imaging.Metadata) {
	if w.location == nil {
		return
	}

	w.lat, w.lon, w.hasPos = m.Latitude, m.Longitude, m.HasGPS

	if !m.HasGPS {
		w.location.Hide()
		return
	}

	w.locationMap.SetMarkers([]xwidget.MapMarker{
		xwidget.NewMapMarker(m.Latitude, m.Longitude, lang.L("Photo location")),
	})
	w.locationMap.PanToLatLon(m.Latitude, m.Longitude)
	w.location.Show()

	// An expanded section following the user from image to image needs the
	// new position's tiles, which are usually nowhere near the old ones.
	if w.expanded {
		w.startWarm()
	}
}

// Open reports whether the panel is currently showing.
func (w *Window) Open() bool {
	return w.win.Open()
}

// RestoreGeometry makes the panel remember where and how large it was,
// seeded with what the last run left it at. Called once during internal/ui's
// startup restoration; the app reads the current values back out of
// Geometry at shutdown. Without it the panel opens at exifW x exifH
// wherever the OS puts it, which is what it always did.
func (w *Window) RestoreGeometry(g widgets.Geometry) {
	w.win.Remember(g)
}

// Geometry is where the panel currently is and how large - or where it was
// last, since it outlives the window being closed. What internal/ui hands
// preferences.Save at shutdown.
func (w *Window) Geometry() widgets.Geometry {
	return w.win.Geometry()
}

// StopTracking stops following the panel's position, for a shutdown that
// finds it still open - see widgets.Singleton.StopTracking.
func (w *Window) StopTracking() {
	w.win.StopTracking()
}

// Window returns the open window, or nil when it's closed - the identity
// callers and tests use to tell "raised the same window" from "opened a
// second one".
func (w *Window) Window() fyne.Window {
	return w.win.Window()
}

// Text returns the panel's content label while it's open, or nil - the
// rendered metadata, for callers and tests that need to read it back.
func (w *Window) Text() *widget.Label {
	return w.text
}

// Location returns the collapsible map section while the panel is open, or
// nil - for tests that need to check whether the current image has a
// position to show at all.
func (w *Window) Location() *fyne.Container {
	return w.location
}

// LocationExpanded reports whether the map section is open. False for a
// freshly-opened window, which is what keeps a photo's coordinates off the
// network until the user asks to see them.
func (w *Window) LocationExpanded() bool {
	return w.expanded
}

// ToggleLocation opens or closes the map section, as tapping its header
// does - the entry point for tests, and for any future menu item or key
// that wants to drive it.
func (w *Window) ToggleLocation() {
	if w.toggle == nil {
		return
	}

	w.toggle.OnTapped()
}

// formatExifMetadata renders m as one display line per field that's
// actually set - a file with only some tags (or, for non-JPEG formats,
// none at all) just shows fewer lines rather than a wall of blanks.
func formatExifMetadata(m imaging.Metadata) string {
	// Six decimals is about a tenth of a metre - past anything a camera's
	// GPS resolves, and short enough to read.
	var lat, lon string
	if m.HasGPS {
		lat = fmt.Sprintf("%.6f°", m.Latitude)
		lon = fmt.Sprintf("%.6f°", m.Longitude)
	}

	fields := []struct {
		label, value string
	}{
		{lang.L("Camera"), strings.TrimSpace(m.Make + " " + m.Model)},
		{lang.L("Lens"), m.LensModel},
		{lang.L("Exposure"), m.ExposureTime},
		{lang.L("Aperture"), m.FNumber},
		{lang.L("ISO"), m.ISO},
		{lang.L("Focal length"), m.FocalLength},
		{lang.L("Date taken"), m.DateTaken},
		{lang.L("Latitude"), lat},
		{lang.L("Longitude"), lon},
	}

	var lines []string
	for _, f := range fields {
		if f.value == "" {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s: %s", f.label, f.value))
	}

	if len(lines) == 0 {
		return lang.L("No EXIF metadata found in this file.")
	}

	return strings.Join(lines, "\n")
}
