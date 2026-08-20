package spiral

import (
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
)

// sliderRowSliders returns the *widget.Slider objects found directly in
// box, in the order they were added - i.e. the order addSliderRow's calls
// wired up the panel's rows.
func sliderRowSliders(box *fyne.Container) []*widget.Slider {
	var sliders []*widget.Slider
	for _, obj := range box.Objects {
		if s, ok := obj.(*widget.Slider); ok {
			sliders = append(sliders, s)
		}
	}
	return sliders
}

// TestAddSliderRowOnChangedUpdatesTargetShaderUniformAndActivity exercises
// addSliderRow in isolation from the rest of newSettingsPanel's layout, so
// this test only fails when the wiring itself - target pointer, shader
// uniform, activity timer - breaks, not when unrelated layout changes.
func TestAddSliderRowOnChangedUpdatesTargetShaderUniformAndActivity(t *testing.T) {
	test.NewApp()
	st := newState()
	shader := newShader(st)

	p := &settingsPanel{box: container.NewWithoutLayout()}
	target := 0.0
	p.addSliderRow(0, "Test", 0, 10, 3, 1, "arms", &target, shader)

	sliders := sliderRowSliders(p.box)
	if len(sliders) != 1 {
		t.Fatalf("box has %d sliders after one addSliderRow call; want 1", len(sliders))
	}
	s := sliders[0]

	p.lastMove.Store(0) // zero out so a nonzero value after SetValue is unambiguous
	s.SetValue(7)

	if target != s.Value {
		t.Errorf("target = %f; want slider's post-clamp value %f", target, s.Value)
	}
	if got := shader.Uniforms["arms"]; got != float32(s.Value) {
		t.Errorf(`Uniforms["arms"] = %f; want %f`, got, s.Value)
	}
	if p.lastMove.Load() == 0 {
		t.Error("lastMove still 0 after OnChanged fired; want it to mark activity")
	}
}

func TestPanelVisible(t *testing.T) {
	const timeout = 5 * time.Second
	now := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)

	if !panelVisible(now.Add(-timeout), now, timeout) {
		t.Error("panelVisible at exactly the timeout boundary = false; want true (just inside)")
	}
	if panelVisible(now.Add(-timeout-time.Millisecond), now, timeout) {
		t.Error("panelVisible just past the timeout = true; want false")
	}
}

func TestPanelAnchor(t *testing.T) {
	got := panelAnchor(fyne.NewSize(800, 600))
	want := fyne.NewPos(800-settingsPanelWidth-settingsPanelMargin, settingsPanelMargin)
	if got != want {
		t.Errorf("panelAnchor(800x600) = %v; want %v", got, want)
	}
}

// TestNewSettingsPanelOverlayHoldsTrackerAndBoxAsSeparateChildren is a
// regression guard for the hit-testing trap the donor's comment on
// newSettingsPanel describes: Fyne's hit-testing stops descending into a
// CanvasObject's children the moment that object reports Visible() ==
// false, so a mouse tracker nested inside the (auto-hiding) box would go
// deaf whenever the box hides, and nothing would be left listening for the
// movement that's supposed to bring it back. The tracker must live in the
// never-hidden overlay as a sibling of the box, not inside it.
func TestNewSettingsPanelOverlayHoldsTrackerAndBoxAsSeparateChildren(t *testing.T) {
	test.NewApp()
	st := newState()
	shader := newShader(st)
	p := newSettingsPanel(st, shader)

	if len(p.overlay.Objects) != 2 {
		t.Fatalf("overlay has %d children; want 2 (the mouse tracker and the box)", len(p.overlay.Objects))
	}
	if _, ok := p.overlay.Objects[0].(*hoverRect); !ok {
		t.Errorf("overlay.Objects[0] = %T; want *hoverRect (the mouse tracker)", p.overlay.Objects[0])
	}
	if p.overlay.Objects[1] != fyne.CanvasObject(p.box) {
		t.Error("overlay.Objects[1] is not p.box; the box must be a direct sibling of the tracker in the overlay, not nested inside it")
	}
	for _, obj := range p.box.Objects {
		if _, ok := obj.(*hoverRect); ok {
			t.Error("found the mouse tracker nested inside p.box; it must live in the overlay instead so hiding the box doesn't silence it")
		}
	}
}

func TestNewSettingsPanelStartsVisibleAndTickHidesAfterIdleTimeout(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("")
	defer w.Close()
	w.Resize(fyne.NewSize(640, 480))

	st := newState()
	shader := newShader(st)
	p := newSettingsPanel(st, shader)

	if !p.box.Visible() {
		t.Fatal("box.Visible() = false immediately after newSettingsPanel; want true - a freshly built panel must start visible")
	}

	// Drive the idle timer by writing a stale timestamp directly, rather
	// than sleeping past settingsPanelIdleTimeout.
	stale := time.Now().Add(-settingsPanelIdleTimeout - time.Second).UnixMilli()
	p.lastMove.Store(stale)

	p.tick(w)

	if p.box.Visible() {
		t.Error("box.Visible() = true after tick() saw a stale lastMove; want false (auto-hidden)")
	}
}

func TestTickAnchorsBoxToWindowRightEdge(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("")
	defer w.Close()
	w.Resize(fyne.NewSize(640, 480))

	st := newState()
	shader := newShader(st)
	p := newSettingsPanel(st, shader)

	p.tick(w)

	want := panelAnchor(w.Canvas().Size())
	if got := p.box.Position(); got != want {
		t.Errorf("box.Position() after tick() = %v; want %v (panelAnchor of the window's canvas size)", got, want)
	}
}

// TestAddSliderRowStepSmallerThanRange guards the reasoning in addSliderRow's
// doc comment: widget.NewSlider defaults Step to 1, and Fyne's snapping
// rounds every drag to a multiple of Step that can fall outside [Min, Max]
// when Step exceeds the slider's range, sticking the handle. Every row
// newSettingsPanel wires up must keep Step below its own range.
func TestAddSliderRowStepSmallerThanRange(t *testing.T) {
	test.NewApp()
	st := newState()
	shader := newShader(st)
	p := newSettingsPanel(st, shader)

	sliders := sliderRowSliders(p.box)
	if len(sliders) != 3 {
		t.Fatalf("box has %d sliders; want 3 (Arms, Twists, Pixel Density)", len(sliders))
	}
	for i, s := range sliders {
		if rng := s.Max - s.Min; s.Step >= rng {
			t.Errorf("slider %d: Min=%f Max=%f Step=%f; want Step < range %f", i, s.Min, s.Max, s.Step, rng)
		}
	}
}
