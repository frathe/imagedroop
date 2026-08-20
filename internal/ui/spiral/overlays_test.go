package spiral

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/test"
)

func TestNewContentOverlayStartsHidden(t *testing.T) {
	o := newContentOverlay(contentOverlayWidth)
	if o.Visible() {
		t.Error("newContentOverlay: Visible() = true; want false, a fresh overlay must start hidden")
	}
}

// TestSetOverlayContentTextKeepsBackdropAtIndexZero guards the invariant
// setOverlayContentText itself depends on: it type-asserts Objects[0] to
// *canvas.Rectangle on every call, so a change that reordered the backdrop
// would panic the next time text is set, not just render wrong.
func TestSetOverlayContentTextKeepsBackdropAtIndexZero(t *testing.T) {
	o := newContentOverlay(contentOverlayWidth)

	setOverlayContentText(o, "one\ntwo\nthree", statusLineHeight, 14)

	if _, ok := o.Objects[0].(*canvas.Rectangle); !ok {
		t.Fatalf("Objects[0] = %T; want *canvas.Rectangle", o.Objects[0])
	}
}

func TestSetOverlayContentTextBackdropGrowsWithLineCount(t *testing.T) {
	o := newContentOverlay(contentOverlayWidth)

	setOverlayContentText(o, "one line", statusLineHeight, 14)
	shortHeight := o.Objects[0].(*canvas.Rectangle).Size().Height

	setOverlayContentText(o, "one\ntwo\nthree\nfour\nfive", statusLineHeight, 14)
	tallHeight := o.Objects[0].(*canvas.Rectangle).Size().Height

	if tallHeight <= shortHeight {
		t.Errorf("backdrop height = %f after 5 lines; want > %f (height after 1 line)", tallHeight, shortHeight)
	}

	wantTall := float32(contentOverlayPadding*2) + 5*float32(statusLineHeight)
	if tallHeight != wantTall {
		t.Errorf("backdrop height = %f after 5 lines; want %f", tallHeight, wantTall)
	}
}

func TestUpdateFPSBackdropColorThresholds(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("")
	defer w.Close()
	w.Resize(fyne.NewSize(640, 480))

	tests := []struct {
		name string
		dt   float64
		want color.NRGBA
	}{
		{"above 60fps is dark green", 1.0 / 61.0, color.NRGBA{0, 120, 0, 180}},
		{"between 40 and 60fps is dark yellow", 1.0 / 50.0, color.NRGBA{120, 120, 0, 180}},
		{"below 40fps is red", 1.0 / 20.0, color.NRGBA{150, 0, 0, 180}},
		{"zero dt guards against divide by zero, reporting 0fps (red)", 0, color.NRGBA{150, 0, 0, 180}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			o := newFPSOverlay()
			updateFPS(w, o, tt.dt)

			bg, ok := o.Objects[0].(*canvas.Rectangle)
			if !ok {
				t.Fatalf("Objects[0] = %T; want *canvas.Rectangle", o.Objects[0])
			}
			got, ok := bg.FillColor.(color.NRGBA)
			if !ok {
				t.Fatalf("FillColor = %T; want color.NRGBA", bg.FillColor)
			}
			if got != tt.want {
				t.Errorf("dt=%f: FillColor = %+v; want %+v", tt.dt, got, tt.want)
			}
		})
	}
}

func overlayTextLines(o *fyne.Container) []string {
	var lines []string
	for _, obj := range o.Objects {
		if t, ok := obj.(*canvas.Text); ok {
			lines = append(lines, t.Text)
		}
	}
	return lines
}

func TestUpdateStatusIncludesResolutionScaleAndPreset(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("")
	defer w.Close()
	w.Resize(fyne.NewSize(640, 480))

	st := newState()
	o := newStatusOverlay()

	updateStatus(w, st, o)

	joined := strings.Join(overlayTextLines(o), "\n")

	mi := getMonitorInfo(w)
	wantResolution := fmt.Sprintf("%d×%d", mi.width, mi.height)
	if !strings.Contains(joined, wantResolution) {
		t.Errorf("status text = %q; want it to contain resolution %q", joined, wantResolution)
	}

	wantScale := f32toStr(mi.scale)
	if !strings.Contains(joined, wantScale) {
		t.Errorf("status text = %q; want it to contain scale %q", joined, wantScale)
	}

	if !strings.Contains(joined, st.presetName()) {
		t.Errorf("status text = %q; want it to contain preset name %q", joined, st.presetName())
	}
}

func TestUpdateStatusIncludesCurrentSpeeds(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("")
	defer w.Close()
	w.Resize(fyne.NewSize(640, 480))

	st := newState()
	st.adjustSpeed(1.0)
	st.adjustHueSpeed(0.01)
	o := newStatusOverlay()

	updateStatus(w, st, o)

	joined := strings.Join(overlayTextLines(o), "\n")

	wantSpeed := strconv.FormatFloat(st.speed(), 'f', 2, 64)
	if !strings.Contains(joined, wantSpeed) {
		t.Errorf("status text = %q; want it to contain turn speed %q", joined, wantSpeed)
	}

	wantHueSpeed := strconv.FormatFloat(st.hueSpeed(), 'f', 2, 64)
	if !strings.Contains(joined, wantHueSpeed) {
		t.Errorf("status text = %q; want it to contain colour speed %q", joined, wantHueSpeed)
	}
}

func TestRefreshStatusOnlyUpdatesWhenVisible(t *testing.T) {
	a := test.NewApp()
	w := a.NewWindow("")
	defer w.Close()
	w.Resize(fyne.NewSize(640, 480))

	st := newState()
	o := newStatusOverlay() // starts hidden

	refreshStatus(w, st, o)
	if len(o.Objects) > 1 {
		t.Errorf("refreshStatus on a hidden overlay populated %d objects; want just the backdrop", len(o.Objects))
	}

	o.Show()
	refreshStatus(w, st, o)
	if len(o.Objects) <= 1 {
		t.Errorf("refreshStatus on a visible overlay left %d objects; want backdrop plus text", len(o.Objects))
	}
}

func TestUpdateHelpTextOneLinePerCommand(t *testing.T) {
	o := newHelpOverlay()

	updateHelpText(o)

	lines := overlayTextLines(o)
	const wantLines = 10 // heading, separator, 8 key bindings
	if len(lines) != wantLines {
		t.Fatalf("len(lines) = %d; want %d: %q", len(lines), wantLines, lines)
	}

	joined := strings.Join(lines, "\n")
	if !strings.Contains(joined, "ESC: Close help / Close window") {
		t.Errorf("help text = %q; want it to contain the Escape line", joined)
	}
}
