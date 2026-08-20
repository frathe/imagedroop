package spiral

import (
	"fmt"
	"image"
	"image/color"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
)

// ─── Content overlays ────────────────────────────────────────────────────────
//
// "Content" overlays are the plain multi-line text panels (status, help) as
// opposed to the performance overlay, which uses its own FPS-colored
// backdrop and single-line layout. They all share the same backdrop look via
// newContentOverlay/setOverlayContentText below.

// contentOverlayBackdropColor is the shared translucent black backdrop used
// by every content overlay.
var contentOverlayBackdropColor = color.NRGBA{0, 0, 0, 191}

// contentOverlayPadding is the space, in points, between a content
// overlay's backdrop edge and the text drawn on top of it.
const contentOverlayPadding = 10

// contentOverlayWidth is the shared fixed backdrop width for all content
// overlays (status, help).
const contentOverlayWidth = 300

// newContentOverlay creates the hidden container used for a multi-line
// content overlay (status, help), with a backdrop rectangle of
// contentOverlayBackdropColor positioned at (contentOverlayPadding,
// contentOverlayPadding). width is fixed; the backdrop's height grows to
// fit whatever text is later passed to setOverlayContentText.
//
// This must stay a plain *fyne.Container (not a named wrapper type):
// Fyne's internal render-tree walker type-switches on the exact concrete
// type `*fyne.Container` to find its children, so a wrapper struct that only
// embeds one would never have its children discovered or drawn.
func newContentOverlay(width float32) *fyne.Container {
	o := container.NewWithoutLayout()

	bg := canvas.NewRectangle(contentOverlayBackdropColor)
	bg.Move(fyne.NewPos(contentOverlayPadding, contentOverlayPadding))
	bg.Resize(fyne.NewSize(width, contentOverlayPadding*2))
	o.Objects = []fyne.CanvasObject{bg}

	o.Hide()
	return o
}

// createTextOverlay populates a container with multi-line text using
// absolute positioning.
func createTextOverlay(o *fyne.Container, text string, x, y float32, lineHeight float32, size float32) {
	lines := strings.Split(text, "\n")
	o.Objects = nil

	for i, line := range lines {
		t := canvas.NewText(line, image.White)
		t.TextSize = size
		t.Move(fyne.NewPos(x, y+float32(i)*lineHeight))
		o.Objects = append(o.Objects, t)
	}
	o.Refresh()
}

// setOverlayTextWithBackdrop rebuilds o's text lines via createTextOverlay
// (which resets o.Objects) and re-prepends bg so it stays the container's
// first, bottommost object. Shared by every overlay that draws text over a
// backdrop rectangle - content overlays (via setOverlayContentText below)
// and the performance overlay (updateFPS), which keeps its own dynamic
// FPS-colored bg but rebuilds its text the same way.
func setOverlayTextWithBackdrop(o *fyne.Container, bg fyne.CanvasObject, text string, x, y, lineHeight, textSize float32) {
	createTextOverlay(o, text, x, y, lineHeight, textSize)
	o.Objects = append([]fyne.CanvasObject{bg}, o.Objects...)
	o.Refresh()
}

// setOverlayContentText replaces a content overlay's text (the overlay must
// have been built by newContentOverlay, so Objects[0] is its backdrop),
// growing the backdrop to fit the new line count.
func setOverlayContentText(o *fyne.Container, text string, lineHeight, textSize float32) {
	bg := o.Objects[0].(*canvas.Rectangle)
	lineCount := len(strings.Split(text, "\n"))
	bg.Resize(fyne.NewSize(bg.Size().Width, contentOverlayPadding*2+float32(lineCount)*lineHeight))

	setOverlayTextWithBackdrop(o, bg, text, contentOverlayPadding*2, contentOverlayPadding*2, lineHeight, textSize)
}

// ─── Status overlay ──────────────────────────────────────────────────────────

// statusLineHeight is the vertical spacing between stacked lines in the
// status overlay.
const statusLineHeight = 18

func newStatusOverlay() *fyne.Container {
	return newContentOverlay(contentOverlayWidth)
}

// setStatusText replaces the overlay's contents, one line of text per "\n"-
// separated line in text.
func setStatusText(o *fyne.Container, text string) {
	setOverlayContentText(o, text, statusLineHeight, 14)
}

// updateStatus rebuilds the status overlay from w's current monitor info and
// st's current pattern/speed settings. Each line goes through its own
// lang.L call rather than one call for the whole block, so a translator
// sees eight small, self-contained strings instead of one multi-line blob
// that would have to be retranslated whole every time a single line
// changed.
func updateStatus(w fyne.Window, st *state, statusText *fyne.Container) {
	mi := getMonitorInfo(w)

	lines := []string{
		fmt.Sprintf(lang.L("Monitor: %s"), mi.name()),
		fmt.Sprintf(lang.L("Resolution: %d×%d px"), mi.width, mi.height),
		fmt.Sprintf(lang.L("Scale: %sx"), f32toStr(mi.scale)),
		fmt.Sprintf(lang.L("Logical: %s×%s pts"), f32toStr(mi.logicalW), f32toStr(mi.logicalH)),
		fmt.Sprintf(lang.L("Pattern: %s (N)"), st.presetName()),
		fmt.Sprintf(lang.L("Turn speed: %s (←/→)"), strconv.FormatFloat(st.speed(), 'f', 2, 64)),
		fmt.Sprintf(lang.L("Colour speed: %s (↑/↓)"), strconv.FormatFloat(st.hueSpeed(), 'f', 2, 64)),
		lang.L("Press R to hide"),
	}

	setStatusText(statusText, strings.Join(lines, "\n"))
}

// refreshStatus updates the overlay text only if it's currently visible, so
// adjusting speed while the overlay is hidden doesn't pop it open.
func refreshStatus(w fyne.Window, st *state, statusText *fyne.Container) {
	if statusText.Visible() {
		updateStatus(w, st, statusText)
	}
}

// helpLineHeight is the vertical spacing between stacked lines in the
// help overlay.
const helpLineHeight = 22

func newHelpOverlay() *fyne.Container {
	return newContentOverlay(contentOverlayWidth)
}

func newFPSOverlay() *fyne.Container {
	o := container.NewWithoutLayout()
	o.Hide()
	return o
}

// updateFPS rebuilds the performance overlay from a per-frame delta time,
// coloring its backdrop green/yellow/red as a quick visual read of frame
// health without needing to read the number. dt == 0 (the very first frame,
// or a stalled clock) reports 0 fps rather than dividing by zero.
func updateFPS(w fyne.Window, o *fyne.Container, dt float64) {
	fps := 0.0
	if dt > 0 {
		fps = 1.0 / dt
	}
	text := fmt.Sprintf(lang.L("FPS: %.0f"), fps)

	var bgColor color.NRGBA
	if fps > 60 {
		bgColor = color.NRGBA{0, 120, 0, 180} // Dark Green
	} else if fps >= 40 {
		bgColor = color.NRGBA{120, 120, 0, 180} // Dark Yellow
	} else {
		bgColor = color.NRGBA{150, 0, 0, 180} // Red
	}

	// Position top-right: Canvas width minus some padding
	size := w.Canvas().Size()
	x := size.Width - 100
	y := float32(20)

	bg := canvas.NewRectangle(bgColor)
	bg.Resize(fyne.NewSize(70, 22))
	bg.Move(fyne.NewPos(x-10, y-2))

	setOverlayTextWithBackdrop(o, bg, text, x, y, 20, 14)
}

// updateHelpText rebuilds the help overlay's key list, one lang.L call per
// line. The separator is plain punctuation with no words in it, so it's the
// one line here that isn't run through lang.L - there is nothing in it for
// a translator to translate.
//
// Unlike the donor demo, ESC here closes the spiral window rather than
// quitting the app: Escape must never quit picfetch.
func updateHelpText(o *fyne.Container) {
	lines := []string{
		lang.L("Keyboard Commands:"),
		"-------------------",
		lang.L("ESC: Close help / Close window"),
		lang.L("F1:  Open help overlay"),
		lang.L("F:   Toggle follow mode"),
		lang.L("N:   Switch spiral pattern"),
		lang.L("P:   Toggle FPS counter"),
		lang.L("R:   Toggle resolution info"),
		lang.L("←/→: Adjust turn speed"),
		lang.L("↑/↓: Adjust colour speed"),
	}

	setOverlayContentText(o, strings.Join(lines, "\n"), helpLineHeight, 16)
}
