package spiral

import (
	"image/color"
	"sync/atomic"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
)

// settingsPanelWidth / settingsPanelHeight are the fixed dimensions of the
// settings panel, used both to lay it out internally and to anchor it to
// the right edge of the window (see panelAnchor).
const (
	settingsPanelWidth  = 260
	settingsPanelHeight = 250
	settingsPanelMargin = 10

	// settingsPanelIdleTimeout is how long the panel stays visible after
	// the last detected mouse movement before it auto-hides.
	settingsPanelIdleTimeout = 5 * time.Second
)

// panelTitleY / panelTitleSize position and size the headline; sliderRowsTop
// is where the first slider row begins, leaving room for the headline above
// it.
const (
	panelTitleY    = 12
	panelTitleSize = 18
	sliderRowsTop  = 40
)

// Layout constants for the rows added by addSliderRow: each row is
// sliderRowHeight tall, with the label near its top and the slider control
// below it.
const (
	sliderRowHeight     = 60
	sliderX             = 15
	sliderLabelYOffset  = 20
	sliderTrackYOffset  = 40
	sliderControlWidth  = 220
	sliderControlHeight = 30
)

// settingsPanel is the slider panel plus the bookkeeping its auto-hide
// needs. It replaces the donor demo's two package-level globals
// (settingsPanel *fyne.Container and lastMouseMoveTime atomic.Int64) - this
// repo forbids mutable package-level state - so every field that used to be
// a global now lives here instead.
type settingsPanel struct {
	// overlay is never hidden; it is what gets added to the canvas as the
	// single settings overlay. It holds the full-window mouse tracker and
	// box as two separate children - see newSettingsPanel for why they
	// cannot be merged into one container.
	overlay *fyne.Container

	// box is the small visible panel (background + title + sliders). This
	// is what tick Hides()/Shows()/Moves() as the mouse comes and goes.
	box *fyne.Container

	// lastMove is the Unix millisecond timestamp of the last detected
	// activity, updated via markActivity by the mouse tracker and by every
	// slider's OnChanged (see addSliderRow). It exists solely to drive the
	// panel's auto-hide timer.
	lastMove atomic.Int64
}

// markActivity records that user activity happened just now.
func (p *settingsPanel) markActivity() {
	p.lastMove.Store(time.Now().UnixMilli())
}

// addSliderRow adds a label+slider pair to p.box at the given row index
// (0-based, sliderRowHeight apart), wired to update *target, the shader
// uniform named uniformName, and the panel's activity timer on every
// change.
//
// step must be smaller than max-min: widget.NewSlider defaults Step to 1,
// and Fyne's snapping (value mod step) rounds every drag to a multiple of
// step that can fall outside [min, max] when step exceeds the slider's
// range, leaving the handle stuck instead of tracking the cursor.
func (p *settingsPanel) addSliderRow(row int, label string, min, max, initial, step float64, uniformName string, target *float64, shader *canvas.Shader) {
	y := sliderRowsTop + float32(row)*sliderRowHeight

	l := widget.NewLabel(label)
	l.Move(fyne.NewPos(sliderX, y+sliderLabelYOffset))

	s := widget.NewSlider(min, max)
	s.Step = step
	s.Value = initial
	s.Move(fyne.NewPos(sliderX, y+sliderTrackYOffset))
	s.Resize(fyne.NewSize(sliderControlWidth, sliderControlHeight))
	s.OnChanged = func(v float64) {
		*target = v
		shader.Uniforms[uniformName] = float32(v)
		shader.Refresh()
		p.markActivity()
	}

	p.box.Add(l)
	p.box.Add(s)
}

// newSettingsPanel builds the settings overlay for st and shader. The
// returned panel's overlay must be added to the canvas as-is; its box holds
// the visible controls and is repositioned/hidden/shown by tick.
//
// overlay and box must stay different containers: Fyne's hit-testing stops
// descending into a CanvasObject's children the moment that object itself
// reports Visible() == false (see internal/driver.walkObjectTree's
// requireVisible check). If the full-window mouse tracker lived inside box,
// hiding box would silence the tracker along with it, and nothing would be
// left listening for mouse movement to bring the panel back.
func newSettingsPanel(st *state, shader *canvas.Shader) *settingsPanel {
	p := &settingsPanel{
		overlay: container.NewWithoutLayout(),
		box:     container.NewWithoutLayout(),
	}

	p.overlay.Add(newMouseTracker(st, p.markActivity))

	// background; uses the same backdrop as the other content overlays
	// (status/help - see contentOverlayBackdropColor in overlays.go).
	//
	// This must be a literal *canvas.Rectangle, not a Hoverable wrapper
	// like hoverRect: Fyne's GL painter dispatches drawing via a type
	// switch on each object's exact concrete type
	// (internal/painter/gl/draw.go), which only matches *canvas.Rectangle
	// itself - a struct that merely embeds one satisfies the
	// CanvasObject/Hoverable interfaces (so hit-testing still works) but is
	// never actually painted, i.e. invisible. This doesn't need to be
	// separately Hoverable for activity tracking either: the full-window
	// mouse tracker added above is a sibling in the same overlay tree and
	// already catches hover anywhere a slider doesn't claim first (see
	// newMouseTracker's MouseMoved).
	bg := canvas.NewRectangle(contentOverlayBackdropColor)
	bg.Resize(fyne.NewSize(settingsPanelWidth, settingsPanelHeight))
	bg.Move(fyne.NewPos(0, 0))

	title := canvas.NewText(lang.L("Spiral Controls"), color.White)
	title.TextStyle = fyne.TextStyle{Bold: true}
	title.TextSize = panelTitleSize
	title.Move(fyne.NewPos(sliderX, panelTitleY))

	p.box.Add(bg)
	p.box.Add(title)
	p.addSliderRow(0, lang.L("Arms"), 1, 16, st.arms, 1, "arms", &st.arms, shader)
	p.addSliderRow(1, lang.L("Twists"), 5, 100, st.twist, 1, "twistBase", &st.twist, shader)
	p.addSliderRow(2, lang.L("Pixel Density"), 0.25, 1.0, st.density, 0.01, "density", &st.density, shader)

	p.box.Resize(fyne.NewSize(settingsPanelWidth, settingsPanelHeight))
	// Initial position; repositioned to the right edge once the window's
	// real size is known (see tick).
	p.box.Move(fyne.NewPos(settingsPanelMargin, settingsPanelMargin))

	p.overlay.Add(p.box)

	// Seed activity so a freshly built panel starts visible instead of
	// immediately auto-hiding on its first tick.
	p.markActivity()

	return p
}

// panelVisible reports whether the panel should be showing, given when
// activity was last seen.
func panelVisible(lastMove, now time.Time, timeout time.Duration) bool {
	return now.Sub(lastMove) <= timeout
}

// panelAnchor is where the panel sits: pinned to the window's right edge.
func panelAnchor(canvasSize fyne.Size) fyne.Position {
	return fyne.NewPos(canvasSize.Width-settingsPanelWidth-settingsPanelMargin, settingsPanelMargin)
}

// tick auto-hides/shows p's box based on activity and keeps it anchored to
// the right edge of w. It is meant to be called once per frame.
//
// Unlike the donor demo's tickSettingsPanel, which ran on the render
// goroutine and read w.Canvas().Size() there - off Fyne's UI goroutine,
// wrapping only its mutations in fyne.Do - tick itself must NOT call
// fyne.Do: the caller is expected to invoke it from inside a single
// fyne.Do per frame (see AGENTS.md: "marshal background UI updates through
// fyne.Do"), so by the time tick runs it is already on the UI goroutine and
// can read/write Fyne objects directly.
func (p *settingsPanel) tick(w fyne.Window) {
	now := time.Now()
	lastMove := time.UnixMilli(p.lastMove.Load())

	if visible := panelVisible(lastMove, now, settingsPanelIdleTimeout); visible != p.box.Visible() {
		if visible {
			p.box.Show()
		} else {
			p.box.Hide()
		}
	}

	// Anchor the panel to the right edge of the window, recomputed
	// continuously so it stays correct across resizes/monitor changes.
	canvasSize := w.Canvas().Size()
	if canvasSize.Width > 0 {
		if target := panelAnchor(canvasSize); p.box.Position() != target {
			p.box.Move(target)
		}
	}
}
