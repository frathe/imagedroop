package widgets

import (
	"image/color"
	"testing"

	"fyne.io/fyne/v2/test"
)

// The style constructors read the current theme, so they need an app to read
// it from - see NewFocusRing/NewSelectionTint.
func TestMain(m *testing.M) {
	test.NewApp()
	m.Run()
}

// TestNewSelectionTint_IsTranslucent is the whole point of the tint: it
// marks a grid cell as picked while leaving the thumbnail underneath
// recognisable. An opaque fill would hide the very thing the user is
// selecting by sight.
func TestNewSelectionTint_IsTranslucent(t *testing.T) {
	tint := NewSelectionTint()

	c, ok := color.NRGBAModel.Convert(tint.FillColor).(color.NRGBA)
	if !ok {
		t.Fatalf("FillColor = %T, want something convertible to color.NRGBA", tint.FillColor)
	}

	if c.A == 0 {
		t.Error("tint is fully transparent, so a selected cell would look unselected")
	}
	if c.A == 255 {
		t.Error("tint is fully opaque, so a selected cell would hide its thumbnail")
	}
}
