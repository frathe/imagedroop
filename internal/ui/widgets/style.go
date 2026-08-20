// Package widgets holds the viewer-free UI mechanics shared across the
// app's feature packages: the tappable area behind the drop zone, the
// singleton-window helper the secondary windows share, and the style
// values below.
package widgets

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

// This file gathers the app's shared hardcoded style values - everything
// visual that isn't taken from Fyne's own theme - so one concept can't
// quietly drift apart between call sites. Where two uses of the same concept
// genuinely differ (the delete-confirm rings vs the grid highlight ring),
// the difference is a named parameter here instead of two unrelated
// literals.

const (
	// CardRadius rounds every card-shaped overlay background: the toast,
	// the info overlay, and the delete-confirmation card.
	CardRadius = 8

	// The dropzone's rounded border box.
	DropzoneBorderWidth  = 4
	DropzoneBorderRadius = 14

	// WelcomeArtSize is the square box the welcome and empty-state art
	// share, so both occupy the exact same spot regardless of which one is
	// currently shown.
	WelcomeArtSize = 180

	// Focus rings (see NewFocusRing): the delete-confirmation buttons use a
	// thinner ring than the grid's cell highlight, whose stroke has to stay
	// visible against a busy thumbnail behind it.
	ButtonRingWidth = 2
	GridRingWidth   = 3
	RingRadius      = 6

	// SelectionTintAlpha is how opaque the grid's multi-select wash is (see
	// NewSelectionTint). Enough to read as "picked" across both themes and
	// over any thumbnail, little enough to still see which image it is.
	SelectionTintAlpha = 90
)

var (
	DropzoneBorderColor = color.NRGBA{R: 100, G: 100, B: 135, A: 200}
	DropzoneHoverColor  = color.NRGBA{R: 150, G: 150, B: 205, A: 255}

	// The toast's fixed, deliberately loud warning colors: dark text for
	// contrast against the pastel-orange background. Not theme-derived on
	// purpose - the toast should look the same, and stand out the same, in
	// both light and dark themes (contrast the info overlay, which uses the
	// theme's own overlay-background/foreground pairing instead).
	ToastBGColor   = color.NRGBA{R: 255, G: 179, B: 128, A: 235}
	ToastTextColor = color.NRGBA{R: 51, G: 26, B: 0, A: 255}

	// ScrimColor dims the image view behind the delete-confirmation card.
	ScrimColor = color.NRGBA{R: 0, G: 0, B: 0, A: 140}
)

// NewFocusRing returns one of the manually drawn selection rings used where
// this app tracks its own selection instead of Fyne's widget-focus system
// (see grid.Overview.Close's comment for why it never uses that): a
// transparent rectangle with a colored stroke. Returned visible; callers
// hide it when the ringed item starts unselected.
//
// The stroke is the theme's *primary* color, not ColorNameFocus: Fyne's
// focus color is that same hue at 16% alpha (0x2a), a wash meant to be
// painted across a widget's whole area, and drawn instead as a hairline it
// all but disappears - against the delete card's red button, and against a
// grid thumbnail. Selection here has to be unmissable, so it takes the
// opaque color.
func NewFocusRing(strokeWidth, cornerRadius float32) *canvas.Rectangle {
	ring := canvas.NewRectangle(color.Transparent)
	ring.StrokeColor = theme.Color(theme.ColorNamePrimary)
	ring.StrokeWidth = strokeWidth
	ring.CornerRadius = cornerRadius

	return ring
}

// Ringed pairs a button with its selection ring: the ring fills the cell,
// the button is inset by one padding step inside it, so the ring's stroke
// lands in that gap instead of underneath the button. Stacking the two at
// the same size hides the ring entirely - a Fyne button paints an opaque
// background across its whole area, including the DangerImportance red -
// and the card then looks identical whichever button is selected. Behind
// rather than on top so the ring can never sit between the pointer and the
// button it marks.
func Ringed(ring *canvas.Rectangle, btn *widget.Button) *fyne.Container {
	return container.NewStack(ring, container.NewPadded(btn))
}

// NewSelectionTint returns the translucent wash the grid overview draws over
// a cell that is part of a multi-select. Returned visible; callers hide it on
// the cells that start unselected.
//
// A filled wash rather than a second ring, because a cell can be both
// selected and the keyboard's current position at the same time, and two
// rings differing only in color would be unreadable. The tint is the same
// primary hue NewFocusRing strokes with, so the pair reads as one idea: the
// ring says "here", the wash says "picked".
func NewSelectionTint() *canvas.Rectangle {
	c := color.NRGBAModel.Convert(theme.Color(theme.ColorNamePrimary)).(color.NRGBA)
	c.A = SelectionTintAlpha

	tint := canvas.NewRectangle(c)
	tint.CornerRadius = RingRadius

	return tint
}
