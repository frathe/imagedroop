// Package grid is the full-window thumbnail overview (the G key): a
// virtualized grid of every loaded image, for jumping around a large drop
// by sight instead of arrowing through it one file at a time.
//
// It owns the thumbnail cache and the bounded worker pool that fills it,
// and reaches back into the app through Host. It knows nothing about the
// app's other full-window mode (the slideshow): the two don't compose, but
// that guard lives in the app's key dispatcher, not here.
package grid

import (
	"fmt"
	"image"
	"strings"
	"sync"
	"sync/atomic"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/picfetch/internal/imaging"
	"github.com/frathe/picfetch/internal/selection"
	"github.com/frathe/picfetch/internal/ui/widgets"
	"github.com/frathe/picfetch/internal/winpos"
)

// thumbConcurrency bounds how many thumbnail decodes run at once - a small
// worker-pool semaphore rather than one goroutine per request.
// widget.GridWrap is virtualized (it only ever builds/updates cells for the
// currently visible rows, unlike container.NewGridWrap), which already
// keeps the *number* of thumbnails requested at once bounded to roughly a
// screenful even for a several-thousand-file folder; this bounds how many
// of those run in parallel too, since a photo can still be tens of
// megapixels before scaling shrinks it down.
const thumbConcurrency = 4

// cellSize is the fixed width/height, in canvas points, each grid cell is
// laid out at.
const cellSize = 120

// Host is what the overview needs from the application: the file set to
// draw, the generation counter that tells a finished decode whether its
// file set is still current, and the display actions a selection triggers.
type Host interface {
	// FileCount is how many files are loaded.
	FileCount() int

	// FileAt returns the file at index i.
	FileAt(i int) fyne.URI

	// CurrentIndex is the file currently on screen - where the highlight
	// starts when the grid opens.
	CurrentIndex() int

	// Generation is the app's index-to-URI file-set revision. A decode
	// captures it when it starts and discards its result if it no longer
	// matches, so replacement, reorder, or removal cannot paint a stale
	// thumbnail. Navigation alone leaves it unchanged.
	Generation() uint64

	// ShowImage displays the file at index i.
	ShowImage(i int)

	// HighlightChanged reports which file the ring is on while the grid is
	// up, so the app can name it in the window title - the only thing that
	// identifies a thumbnail once the image view is hidden. i is -1
	// whenever no file is under the ring: the grid closing, or a search
	// that matches nothing.
	HighlightChanged(i int)

	// ForceRepaint redraws the window after a visibility change.
	ForceRepaint()

	// Unfocus releases canvas focus - see Close for why that matters.
	Unfocus()

	// Modifiers is which keyboard modifiers are held right now. A Fyne tap
	// carries none of its own, so the multi-select gestures (Cmd/Ctrl+click
	// to toggle a cell, Shift+click to extend a range) have to ask at the
	// moment the tap arrives - the same accessor internal/ui/zoom already
	// uses for its Shift+scroll pan, and stubbable per-viewer for the same
	// reason: Fyne's test driver implements no desktop.Driver to read them
	// from.
	Modifiers() fyne.KeyModifier
}

// Overview is the grid overlay and the state behind it.
type Overview struct {
	host Host
	win  fyne.Window

	visible bool
	wrap    *widget.GridWrap
	overlay *fyne.Container

	// The bar across the top of the overlay, hidden until there is either a
	// search or a selection to report: what was typed on the left, how much
	// of the set still matches and how much of it is picked on the right.
	// empty is the notice drawn over the grid in the one state that has no
	// cells at all to explain itself.
	searchBar   *fyne.Container
	searchLabel *widget.Label
	countLabel  *widget.Label
	selLabel    *widget.Label
	empty       *widget.Label

	// maximized is true from the moment Toggle grows the window via
	// winpos.Maximize until ConsumeMaximized is next called. Left true
	// across a plain Close - see its own doc comment: closing the grid
	// deliberately leaves the window maximized - so a later resize the app
	// makes for a reason of its own (not just picking an image straight out
	// of the grid, but navigating on afterward too) still knows to undo it
	// first.
	maximized bool

	// highlight is which cell's ring is currently drawn, moved by the
	// arrow keys while the grid is up and committed with Return. Reset to
	// the host's current index every time the grid opens, so it starts on
	// whichever image was already on screen.
	highlight int

	// searching is whether the search bar is up, opened by typing '/' and
	// left by Escape. Distinct from "a filter is active" (matches != nil):
	// an open search with nothing typed yet still shows every file.
	searching bool

	// query is the filter text typed so far, matched case-insensitively
	// against each file's base name.
	query string

	// sel is the multi-select: which files are picked, and the anchor a
	// Shift+click extends from. Holds host file indices rather than the
	// display indices actually clicked, so it survives a filter change -
	// see selection.go.
	sel *selection.Set

	// matches maps a display index - a cell's position in the grid - to
	// the host's own file index, while a filter is active. nil means no
	// filter, and is what every index below means "identity" by: the grid
	// renumbers its cells from zero when filtered, but ShowImage,
	// FileAt and CurrentIndex all speak the host's numbering, so
	// everything crossing that boundary goes through fileIndex.
	matches []int

	// filterGen counts changes to matches, so a thumbnail decode already
	// in flight can tell that the cell it was started for has been
	// renumbered under it. The host's own generation can't see this: the
	// file set is unchanged by a keystroke, and so is the cell's id - only
	// what that id *means* moved. Atomic because applyFilter writes it on
	// the UI goroutine while a decode worker reads it.
	filterGen atomic.Uint64

	// thumbs holds small, already-downsampled thumbnails keyed by URI
	// string - a separate cache and byte budget from the app's full-size
	// image cache (imaging.NewThumbCache vs NewImgCache), since reusing
	// one for both would evict full-size decodes the normal viewing path
	// still needs, and vice versa. SetCacheBytes retunes the budget while
	// the app runs; see its own comment.
	thumbs *imaging.ByteCache[image.Image]

	// sem bounds concurrent decodes; pending counts them, so Settle can
	// wait for every spawned decode to finish.
	sem     chan struct{}
	pending sync.WaitGroup

	// cellIDs tracks which file id each recycled cell is currently
	// showing: GridWrap reuses a small, fixed pool of cell widgets as the
	// user scrolls rather than creating one per file, so an async decode
	// kicked off against an earlier id has to check, once it completes,
	// that its cell hasn't since been recycled to show a different file. A
	// *sync.Map, not a plain map: the update callback writes it on the UI
	// goroutine while a decode's fyne.Do callback reads it - under the
	// real GLFW driver both run on the same marshaled UI goroutine so a
	// plain map would never actually race, but the test driver runs
	// fyne.Do synchronously on the calling (background) goroutine instead,
	// which turned a plain map into a genuine, test-reproducible
	// concurrent read/write.
	cellIDs sync.Map

	// inflight records, per recycled cell, which id a spawned decode is
	// currently working toward - the claim/release pair around each decode
	// goroutine, so repeated update passes over a still-decoding cell
	// don't stack duplicate goroutines behind sem. A sync.Map for the same
	// reason as cellIDs.
	inflight sync.Map
}

// New builds the overview (hidden) around host. win is maximized (see
// Toggle) each time the overview opens - a bigger window means bigger, more
// legible thumbnails - the same reason slideshow.Controller is handed win
// directly rather than reaching for it some other way.
//
// Each cell is a small stack of the thumbnail image plus a highlight ring
// (mirroring the delete confirmation's own selection rings), rather than
// relying on GridWrap's own built-in highlight rendering - that ties into
// real Fyne canvas focus, which this app deliberately never hands to
// GridWrap (see Close's comment on why).
func New(host Host, win fyne.Window) *Overview {
	g := &Overview{
		host:   host,
		win:    win,
		sel:    selection.New(),
		thumbs: imaging.NewThumbCache(imaging.DefaultThumbCacheBytes),
		sem:    make(chan struct{}, thumbConcurrency),
	}

	g.wrap = widget.NewGridWrap(
		g.count,
		func() fyne.CanvasObject {
			img := canvas.NewImageFromImage(nil)
			img.FillMode = canvas.ImageFillContain
			img.ScaleMode = canvas.ImageScaleFastest
			img.SetMinSize(fyne.NewSize(cellSize, cellSize))

			// The selection tint sits under the highlight ring so the ring's
			// stroke stays crisp over it: the two mark different things and
			// routinely land on the same cell.
			tint := widgets.NewSelectionTint()
			tint.Hide()

			ring := widgets.NewFocusRing(widgets.GridRingWidth, widgets.RingRadius)
			ring.Hide()

			return container.NewStack(img, tint, ring)
		},
		func(id widget.GridWrapItemID, o fyne.CanvasObject) {
			cell := o.(*fyne.Container)
			img := cell.Objects[0].(*canvas.Image)
			tint := cell.Objects[1].(*canvas.Rectangle)
			ring := cell.Objects[2].(*canvas.Rectangle)

			g.cellIDs.Store(cell, id)
			setCellHighlighted(ring, id == g.highlight)
			setCellSelected(tint, g.isSelected(id))

			// Refresh reaches this callback whether or not the overlay is
			// actually open: every ForceRepaint refreshes the whole widget
			// tree, hidden GridWrap included. Requesting thumbnails from
			// those refreshes would kick off background decodes for an
			// invisible grid on every navigation step - and, under the
			// fyne test driver (where fyne.Do runs a goroutine's callback
			// inline on the caller instead of marshaling it to the UI
			// goroutine), would let such a decode's completion paint race
			// a later refresh's own cell reset. Skip while hidden: Toggle
			// sets visible before its own refresh, so opening the grid
			// still populates every visible cell.
			if !g.visible {
				img.Image = nil
				img.Refresh()
				return
			}

			// While visible, blanking is requestThumbnail's job, and only
			// on a cache miss - clearing unconditionally here made every
			// scroll tick repaint every already-cached cell twice, with an
			// empty flash in between.
			g.requestThumbnail(cell, img, id, g.host.Generation())
		},
	)

	// A tap on a cell: which of the three things it means depends on what is
	// held down at the time, which the event itself doesn't say - see
	// Host.Modifiers.
	//
	// Both selection gestures have to hand the keyboard back themselves.
	// Fyne's GridWrap grabs canvas focus on every tap, and this app
	// dispatches every key from the *unfocused* canvas handler; Close is
	// what normally undoes that, so a tap that deliberately leaves the grid
	// open would otherwise leave a focused GridWrap swallowing the arrow
	// keys, '/' and everything after it.
	g.wrap.OnSelected = func(id widget.GridWrapItemID) {
		defer g.wrap.UnselectAll()

		switch toggle, extend := pickModifier(g.host.Modifiers()); {
		case toggle:
			g.toggleAt(id)
			g.host.Unfocus()
		case extend:
			g.extendTo(id)
			g.host.Unfocus()
		default:
			// Resolved before Close, not after: closing clears the filter,
			// and an id resolved past that point would map to itself rather
			// than to the file this cell was actually showing.
			i := g.fileIndex(id)

			g.Close()
			if i >= 0 {
				g.host.ShowImage(i)
			}
		}
	}

	// Fired both by keyboard highlight movement (HandleKey forwards the
	// arrow keys to wrap.TypedKey, see below) and by mouse hover (GridWrap
	// wires its own onHovered to the same callback) - either way, move the
	// ring to match.
	//
	// The guard is what stops setHighlight's own re-entry through here from
	// recursing: it re-enters with g.highlight already equal to id.
	g.wrap.OnHighlighted = func(id widget.GridWrapItemID) {
		if id == g.highlight {
			return
		}
		g.setHighlight(id)
	}

	g.searchLabel = widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{Bold: true})
	g.countLabel = widget.NewLabel("")
	g.selLabel = widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{Bold: true})
	g.searchBar = container.NewBorder(nil, nil, nil,
		container.NewHBox(g.selLabel, g.countLabel), g.searchLabel)
	g.searchBar.Hide()

	g.empty = widget.NewLabelWithStyle(lang.L("No file names match"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	g.empty.Hide()

	// An opaque backdrop, not a translucent scrim like the delete
	// confirmation's: the grid replaces the image view entirely rather
	// than dimming it behind a centered card, so it needs to fully hide
	// whatever's underneath.
	backdrop := canvas.NewRectangle(theme.Color(theme.ColorNameBackground))
	body := container.NewStack(container.NewPadded(g.wrap), container.NewCenter(g.empty))
	g.overlay = container.NewStack(backdrop, container.NewBorder(g.searchBar, nil, nil, nil, body))
	g.overlay.Hide()

	return g
}

// Overlay is the full-window grid, for the app to place in its window
// stack.
func (g *Overview) Overlay() fyne.CanvasObject {
	return g.overlay
}

// Visible reports whether the grid is up - the app's key dispatcher checks
// this before its own handling.
func (g *Overview) Visible() bool {
	return g.visible
}

// Highlight is the currently ringed cell.
func (g *Overview) Highlight() int {
	return g.highlight
}

// setHighlight moves the ring to display index id and keeps GridWrap's own
// keyboard cursor on the same cell.
//
// The two are separate positions: GridWrap advances its cursor only for the
// arrow keys it handles itself, so a mouse hover - or the grid opening on
// the file currently on screen - used to move the ring without it. The next
// arrow key then resumed from wherever the keyboard had last been, jumping
// the ring away from the cell the user was pointing at.
//
// wrap.Highlight re-enters OnHighlighted, which returns immediately because
// g.highlight is already set. GridWrap's own TypedKey does call RefreshItem
// on the old and new positions before this runs, but at that point
// g.highlight still holds the *old* value, so those calls redraw both cells
// as "still old" - the two RefreshItem calls here are the ones that actually
// apply the moved ring.
func (g *Overview) setHighlight(id int) {
	old := g.highlight
	g.highlight = id
	// Highlight is a no-op on an empty grid, which would leave the cursor
	// pointing into the set the filter just emptied.
	if g.count() > 0 {
		g.wrap.Highlight(id)
	}
	g.wrap.RefreshItem(old)
	g.wrap.RefreshItem(id)

	// Only while the grid owns the screen: with it closed the title belongs
	// to the image view, and setHighlight still runs from Toggle (which has
	// already set visible) and from a filter change.
	if g.visible {
		if g.count() == 0 {
			g.host.HighlightChanged(-1)
		} else {
			g.host.HighlightChanged(g.fileIndex(id))
		}
	}
}

// ConsumeMaximized reports whether the window is still sitting maximized
// from an earlier Toggle and hasn't been undone since, clearing the flag
// either way - a one-shot check for whoever is about to resize the window
// for a reason of their own to know whether it first needs to undo the
// grid's maximize (see winpos.Unmaximize), without ever being told twice.
func (g *Overview) ConsumeMaximized() bool {
	m := g.maximized
	g.maximized = false
	return m
}

// Toggle flips the grid on or off. A no-op with nothing loaded. The
// caller is responsible for not opening it while another full-window mode
// owns the screen - see the app's key dispatcher.
func (g *Overview) Toggle() {
	if g.visible {
		g.Close()
		return
	}
	if g.host.FileCount() == 0 {
		return
	}

	g.visible = true

	// Maximize, not full-screen (see winpos.Maximize) - more room for more,
	// bigger thumbnails at once, without picture-frame mode's chrome-free
	// look. Deliberately one-way: closing the grid does not shrink the
	// window back down, the same way clicking a real maximize button
	// doesn't un-maximize when you switch to another app and back. A no-op
	// wherever winpos can't reach a native window (the fyne test driver,
	// Wayland, mobile, wasm), so the grid still opens there, just without
	// the resize.
	winpos.Maximize(g.win)
	g.maximized = true

	// Start the highlight on whichever image is currently on screen, and
	// scroll it into view - setHighlight also refreshes the grid, which is
	// what actually paints the ring.
	g.setHighlight(g.host.CurrentIndex())
	g.overlay.Show()
	g.host.ForceRepaint()
}

// Close dismisses the grid, restoring the normal image view. A no-op when
// it isn't showing, so the app can call it defensively (on every drop, and
// when entering its other full-window mode) without checking Visible
// first.
//
// Unfocuses the canvas on the way out: tapping a thumbnail is a real Fyne
// widget tap, and Fyne's own GridWrap unconditionally grabs canvas focus
// on tap before calling OnSelected. This app otherwise never uses Fyne's
// widget-focus system - every other key binding is dispatched manually
// from the canvas's default (unfocused) key handler - so a focused
// GridWrap left behind after closing would silently swallow every key
// press afterward (arrow keys included) until something else happened to
// steal focus back.
func (g *Overview) Close() {
	if !g.visible {
		return
	}

	g.visible = false
	// The filter and the selection are both ways of working with the grid,
	// not standing settings: each open starts on the whole set with nothing
	// picked. This also covers the app's defensive Close on every drop,
	// where a query or a selection left over from the previous file set
	// would otherwise be applied to - or, worse, acted on against - the new
	// one.
	g.sel.Clear()
	g.clearSearch()
	// Explicitly, because clearSearch returns early with no search open and
	// would otherwise leave the bar showing a selection count that no longer
	// applies the next time the grid opens.
	g.syncTopBar()
	g.overlay.Hide()
	g.host.HighlightChanged(-1)
	g.host.Unfocus()
	g.host.ForceRepaint()
}

// HandleKey handles a key press while the grid is up: Escape and G back out
// of it, Space picks the highlighted cell, Return commits it, arrow keys move
// the highlight, and Page Up/Page Down move it by one visible page. Every
// other key is deliberately swallowed by the caller.
//
// While a search is open the letter keys stop meaning anything here, since
// each of them is also arriving at HandleRune as a query character - G
// most visibly, which would otherwise close the grid on its way into the
// query. Space is left out of the search branch for exactly the same reason:
// a space typed into a query must not also toggle a cell.
//
// Escape stages rather than closing outright - see escape.
func (g *Overview) HandleKey(ev *fyne.KeyEvent) {
	if g.searching {
		switch ev.Name {
		case fyne.KeyEscape:
			g.escape()
		case fyne.KeyBackspace:
			g.backspace()
		case fyne.KeyReturn, fyne.KeyEnter:
			g.wrap.Select(g.highlight)
		case fyne.KeyPageUp:
			g.movePage(-1)
		case fyne.KeyPageDown:
			g.movePage(1)
		case fyne.KeyUp, fyne.KeyDown, fyne.KeyLeft, fyne.KeyRight,
			fyne.KeyHome, fyne.KeyEnd:
			// Listed rather than left to the default branch below: every
			// other key is a character being typed, and must not reach
			// GridWrap at all.
			g.wrap.TypedKey(ev)
		}

		return
	}

	switch ev.Name {
	case fyne.KeyEscape:
		g.escape()
	case fyne.KeyG:
		// Inert while a selection is pending, the same way it goes inert
		// while a search is open: closing the grid discards the selection,
		// and a user part-way through assembling one is far more likely to
		// have meant Escape's first stage. Escape is the way out either way.
		if g.sel.Len() == 0 {
			g.Close()
		}
	case fyne.KeySpace:
		g.toggleAt(g.highlight)
	case fyne.KeyReturn, fyne.KeyEnter:
		g.wrap.Select(g.highlight)
	case fyne.KeyPageUp:
		g.movePage(-1)
	case fyne.KeyPageDown:
		g.movePage(1)
	default:
		// GridWrap already knows how to move its own highlight across
		// rows and columns, including the row arithmetic - forward the
		// event rather than reimplementing it here.
		g.wrap.TypedKey(ev)
	}
}

// movePage moves the ring by one rendered grid page, clamped at either end.
// GridWrap handles arrows itself but deliberately has no Page Up/Page Down
// behavior, so keep this movement on the same setHighlight path that keeps
// its keyboard cursor, the ring, scrolling, and the host notification in
// sync. A grid that has not yet been laid out still advances by one row.
// Row count must mirror GridWrap.ColumnCount's own arithmetic (pitch is
// itemMin+padding, not itemMin) or the two disagree on where a row ends,
// and Page Down scrolls a partially visible edge row clean out of view.
func (g *Overview) movePage(direction int) {
	if g.count() == 0 {
		return
	}

	pad := g.wrap.Theme().Size(theme.SizeNamePadding)
	rows := max(1, int((g.wrap.Size().Height+pad)/(cellSize+pad)))
	step := g.wrap.ColumnCount() * rows
	target := g.highlight + direction*step
	target = max(0, min(target, g.count()-1))
	if target != g.highlight {
		g.setHighlight(target)
	}
}

// escape undoes one layer per press, smallest first: the selection, then the
// search, then the grid itself. Each of those took the user effort to build,
// so a single keystroke never throws away more than the one thing they were
// most likely aiming at.
func (g *Overview) escape() {
	switch {
	case g.sel.Len() > 0:
		g.ClearSelection()
	case g.searching:
		g.clearSearch()
	default:
		g.Close()
	}
}

// backspace drops the last character of the query. Rune-wise, not
// byte-wise: the query holds whatever the user typed, and a German file
// name's umlaut would otherwise be cut in half into invalid UTF-8.
func (g *Overview) backspace() {
	if g.query == "" {
		return
	}

	r := []rune(g.query)
	g.query = string(r[:len(r)-1])
	g.applyFilter()
}

// clearSearch closes the search bar and restores the unfiltered grid.
func (g *Overview) clearSearch() {
	if !g.searching {
		return
	}

	g.searching = false
	g.query = ""
	g.applyFilter()
}

// Searching reports whether the search bar is up.
func (g *Overview) Searching() bool {
	return g.searching
}

// Query is the filter text typed so far.
func (g *Overview) Query() string {
	return g.query
}

// HandleRune handles a character typed while the grid is up: '/' opens the
// search bar, and every character after that extends the query.
//
// Runes rather than the key names HandleKey sees, because a fyne.KeyEvent
// carries neither case nor the punctuation filenames are full of - there is
// no key name for '_' at all. Taking the canvas's typed-rune callback also
// keeps Fyne's widget focus out of it, so the arrow keys, Return and Escape
// still reach HandleKey exactly as they do with no search open (an
// approach a focused widget.Entry would have taken away).
//
// Search is opened from the rune rather than from HandleKey's KeySlash
// because a key press delivers both callbacks: activating on the key event
// would open the bar and then immediately type the '/' into it.
func (g *Overview) HandleRune(r rune) {
	if !g.searching {
		if r == '/' {
			g.searching = true
			g.applyFilter()
		}

		return
	}

	g.query += string(r)
	g.applyFilter()
}

// count is how many cells the grid shows - the filtered subset while a
// search narrows it, the whole file set otherwise. This is GridWrap's own
// length function.
func (g *Overview) count() int {
	if g.matches != nil {
		return len(g.matches)
	}

	return g.host.FileCount()
}

// fileIndex maps a display index to the host's file index, or -1 when the
// display index addresses no cell. The two numberings differ only while a
// filter is active; the bounds check is the one OnSelected and
// requestThumbnail did against FileCount before filtering existed.
func (g *Overview) fileIndex(id int) int {
	if id < 0 || id >= g.count() {
		return -1
	}
	if g.matches == nil {
		return id
	}

	return g.matches[id]
}

// applyFilter recomputes the visible subset from the current query and
// redraws the grid around it. An empty query - which is what an
// just-opened search bar has - matches everything, so opening search
// changes nothing on screen until a character is typed.
//
// The whole set is rescanned per keystroke rather than narrowed from the
// previous result: Backspace widens the match set again, and a
// strings.Contains over a few thousand names is not worth a cache.
func (g *Overview) applyFilter() {
	g.matches = nil

	if g.searching && g.query != "" {
		needle := strings.ToLower(g.query)
		g.matches = make([]int, 0, g.host.FileCount())

		for i := range g.host.FileCount() {
			if strings.Contains(strings.ToLower(g.host.FileAt(i).Name()), needle) {
				g.matches = append(g.matches, i)
			}
		}
	}

	g.filterGen.Add(1)

	g.wrap.Refresh()
	// The highlight is a display index, so a filter that shortens the grid
	// under it would leave it pointing past the last cell. After the
	// refresh, so GridWrap's cursor is moved against the new length.
	g.setHighlight(0)
	if g.count() > 0 {
		g.wrap.ScrollTo(0)
	}

	g.syncTopBar()
}

// syncTopBar redraws the bar from the current query, match count and
// selection size. The bar earns its space whenever either of the two is
// active, and each half appears on its own: a selection built without ever
// opening the search shows only its count, and vice versa.
func (g *Overview) syncTopBar() {
	if g.searching {
		g.searchLabel.SetText(fmt.Sprintf(lang.L("Search: %s"), g.query))
		g.countLabel.SetText(fmt.Sprintf(lang.L("%d of %d"), g.count(), g.host.FileCount()))
		g.searchLabel.Show()
		g.countLabel.Show()
	} else {
		g.searchLabel.Hide()
		g.countLabel.Hide()
	}

	if n := g.sel.Len(); n > 0 {
		g.selLabel.SetText(fmt.Sprintf(lang.L("%d selected"), n))
		g.selLabel.Show()
	} else {
		g.selLabel.Hide()
	}

	if !g.searching && g.sel.Len() == 0 {
		g.searchBar.Hide()
		g.empty.Hide()

		return
	}

	g.searchBar.Show()

	// Only when the query itself emptied the grid: with no files loaded at
	// all there is no search to be in (Toggle refuses to open), so this
	// can't misfire on an empty set.
	if g.searching && g.count() == 0 {
		g.empty.Show()
	} else {
		g.empty.Hide()
	}
}

// Warm decodes thumbnails for the host's current file set into the cache,
// synchronously, so a subsequent open paints every cell from the cache
// instead of spawning background decodes.
//
// The app itself decodes lazily, as cells scroll into view; this exists
// for callers that need the cache populated up front. In practice that is
// the test suite: under the fyne test driver a decode's completion runs
// inline on the decoding goroutine, so a lazily-filled grid can paint a
// cell while the update pass that spawned the decode is still walking
// cells - a race no amount of waiting afterwards can undo, only avoided by
// having nothing to spawn.
func (g *Overview) Warm() error {
	for i := 0; i < g.host.FileCount(); i++ {
		u := g.host.FileAt(i)
		if _, ok := g.thumbs.Get(u.String()); ok {
			continue
		}

		thumb, err := imaging.LoadThumbnail(u)
		if err != nil {
			return err
		}
		g.thumbs.Add(u.String(), thumb)
	}

	return nil
}

// Settle waits for every thumbnail decode spawned so far to finish -
// including its completion paint, which runs before the wait returns. The
// app never needs this; tests do, to keep a decode goroutine from touching
// widgets after the test that started it has moved on.
func (g *Overview) Settle() {
	g.pending.Wait()
}

// Cached reports whether u's thumbnail is in the cache. Contains rather
// than Get, so asking the question doesn't reorder the cache's own idea of
// what was used least recently.
func (g *Overview) Cached(u fyne.URI) bool {
	return g.thumbs.Contains(u.String())
}

// SetCacheBytes retunes the thumbnail cache's byte budget and evicts down
// to it right away - the settings window's binding, reached through
// internal/ui's SetMaxThumbCacheMB. A setter rather than a New parameter
// for the same reason slideshow.Controller.SetInterval is one: the value
// changes while the app runs, and the overview is built once and lives in
// the window's content stack for the process's lifetime.
func (g *Overview) SetCacheBytes(n int64) {
	g.thumbs.SetBudget(n)
}

// setCellHighlighted shows or hides a cell's highlight ring.
func setCellHighlighted(ring *canvas.Rectangle, highlighted bool) {
	if highlighted {
		ring.Show()
	} else {
		ring.Hide()
	}
}

// requestThumbnail fills img with the thumbnail for the file at id, from
// the cache if present (painted synchronously) or freshly decoded and
// scaled otherwise. key identifies which cell this request was made for
// (see the cellIDs field) - it's the stable per-slot container, not img
// itself, only because that's what cellIDs and inflight are keyed by; img
// is where the result actually gets painted. gen is the host's generation
// at request time: if a new drop supersedes the current file set before
// the decode finishes, the result must not be painted - a now-meaningless
// index into a cell. cellIDs[key] guards the paint for the same reason at
// a finer grain - the file set can still be current while this particular
// cell has scrolled on to show a different id in the meantime.
func (g *Overview) requestThumbnail(key *fyne.Container, img *canvas.Image, id int, gen uint64) {
	i := g.fileIndex(id)
	if i < 0 {
		return
	}

	// Captured here, on the UI goroutine, for the same reason gen is
	// passed in: it pins which query this request's id was resolved
	// under, so the completion can tell whether that is still the query
	// on screen.
	fgen := g.filterGen.Load()

	u := g.host.FileAt(i)
	cacheKey := u.String()

	if thumb, ok := g.thumbs.Get(cacheKey); ok {
		img.Image = thumb
		img.Refresh()

		return
	}

	// Nothing to show while the decode is in flight; whatever the recycled
	// cell held last belongs to a different file. Skipped when already
	// blank so the repaints that arrive during a slow decode don't each
	// redraw an empty cell.
	if img.Image != nil {
		img.Image = nil
		img.Refresh()
	}

	if !g.claim(key, id) {
		return
	}

	// pending lets Settle wait for every spawned decode to fully finish -
	// Done fires only after the completion fyne.Do below has returned, so
	// a Wait that comes back guarantees no decode goroutine will touch a
	// widget afterwards.
	g.pending.Go(func() {

		g.sem <- struct{}{}
		defer func() { <-g.sem }()

		// Bail *before* decoding, not just after: during a fast scroll
		// through a large set, most queued requests are for cells recycled
		// long ago to other files, and this predicate is exactly what the
		// completion below would discard their results with anyway.
		// Checking it here (safe off the UI goroutine: a sync.Map load and
		// an atomic generation read) drains that dead backlog at lookup
		// speed - without it, the workers grind through a full decode per
		// scrolled-past cell while the cells actually on screen sit blank
		// at the back of the queue.
		if !g.stillWanted(key, id, gen, fgen) {
			g.release(key, id)

			// That check raced the UI goroutine's cell updates in one
			// narrow window: the cell scrolled away and back to id between
			// the update pass (which saw the old claim and didn't spawn)
			// and here. Re-check on the UI goroutine, where updates are
			// serialized, and re-request rather than leave the cell blank
			// until something else happens to refresh it.
			fyne.Do(func() {
				if g.stillWanted(key, id, gen, fgen) {
					g.requestThumbnail(key, img, id, gen)
				}
			})

			return
		}

		// A second look at the cache: merge mode can load one path at two
		// indices, so a peer worker may have finished this exact file
		// while this request sat behind sem.
		thumb, ok := g.thumbs.Get(cacheKey)
		if !ok {
			var err error
			if thumb, err = imaging.LoadThumbnail(u); err != nil {
				// No retry here: release lets the cell's next update pass
				// claim and try again, and the normal viewing path is
				// where the file's actual error surfaces to the user.
				g.release(key, id)
				return
			}

			// Cached unconditionally, not gated on stillWanted like the
			// paint below: the thumbnail is keyed by URI, not index, so it
			// stays valid however far the cell has scrolled on. Discarding
			// it would mean decoding the same file again the moment the
			// user scrolls back.
			g.thumbs.Add(cacheKey, thumb)
		}

		g.release(key, id)

		fyne.Do(func() {
			if g.stillWanted(key, id, gen, fgen) {
				img.Image = thumb
				img.Refresh()
			}
		})
	})
}

// claim records that a decode goroutine is being spawned to fill the cell
// key with id's thumbnail - false means an identical decode is already in
// flight, so don't spawn another. Every repaint re-runs GridWrap's update
// callback for every visible cell, and one multi-megapixel decode easily
// outlives several repaints; without this gate each of those passes would
// stack another goroutine behind sem for work already underway. A claim
// for a different id overwrites the old entry - the cell scrolled on, and
// the superseded decode's release is compare-and-delete precisely so its
// late finish can't clobber the newer claim.
func (g *Overview) claim(key *fyne.Container, id int) bool {
	if cur, ok := g.inflight.Load(key); ok && cur == id {
		return false
	}
	g.inflight.Store(key, id)

	return true
}

// release clears key's claim, but only if it still belongs to id - see
// claim on why a finished decode must not drop a newer claim made over it.
func (g *Overview) release(key *fyne.Container, id int) {
	g.inflight.CompareAndDelete(key, id)
}

// stillWanted reports whether a decode for id (kicked off at generation
// gen, under filter generation fgen) is still worth anything to the cell
// identified by key - checked by the worker before it decodes and by the
// completion before it paints, and split out so the generation and
// cell-recycling logic can be driven directly and synchronously from a test
// instead of racing a real decode goroutine. Safe from any goroutine
// (cellIDs is a sync.Map, Generation and filterGen atomic reads).
//
// False whenever a newer drop superseded the file set gen was captured
// against, this cell has since been recycled to show a different id, or a
// keystroke has since renumbered the cells under it - the three ways the
// file this decode is carrying can stop being the file this cell shows.
func (g *Overview) stillWanted(key *fyne.Container, id int, gen, fgen uint64) bool {
	current, ok := g.cellIDs.Load(key)

	return ok && gen == g.host.Generation() && fgen == g.filterGen.Load() && current == id
}
