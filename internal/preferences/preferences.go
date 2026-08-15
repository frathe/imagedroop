// Package preferences persists and restores standing UI preferences - sort
// order, merge mode, the picture-frame slideshow interval, and window size
// and position - across launches, via Fyne's app-scoped Preferences store.
// Unlike
// internal/session (which persists the transient dropped file set),
// everything here is a setting the user deliberately chose and expects to
// stick, so it belongs in fyne.Preferences: unlike the app cache, it's
// meant for this and survives cache clearing.
package preferences

import (
	"time"

	"fyne.io/fyne/v2"
)

const (
	keySortMode       = "sortMode"
	keyMergeMode      = "mergeMode"
	keySlideIntervalS = "slideIntervalSeconds"
	keyWindowWidth    = "windowWidth"
	keyWindowHeight   = "windowHeight"
	keyWindowPosX     = "windowPosX"
	keyWindowPosY     = "windowPosY"
	keyWindowPosSet   = "windowPosSet"
)

// Valid values for State.SortMode, persisted under keySortMode. Defined as
// strings here - rather than reusing the root package's own sortMode enum,
// which this package can't import without an import cycle - so the on-disk
// representation stays stable and human-readable even if that enum's
// members are ever reordered or renamed.
const (
	SortByName        = "name"
	SortByCaptureDate = "date"
	SortByModTime     = "modified"
	SortBySize        = "size"
	SortByDropOrder   = "drop"
)

// State is the set of standing preferences Save/Load round-trip.
type State struct {
	// SortMode is one of the SortBy* constants above. See Load's comment
	// for how an empty or unrecognized value is handled.
	SortMode      string
	MergeMode     bool
	SlideInterval time.Duration
	WindowSize    fyne.Size // zero Size means "nothing saved yet"

	// WindowPosX/WindowPosY are the on-screen position (see
	// internal/winpos, the only way to read one back at all) a manual move
	// last left the window at; WindowPositionSet distinguishes "saved at
	// (0,0)" from "never saved", which a zero-value check like WindowSize's
	// can't - (0,0) is a perfectly valid on-screen position, unlike a 0x0
	// size.
	WindowPosX, WindowPosY int
	WindowPositionSet      bool
}

// Save persists s via app.Preferences(). SlideInterval and WindowSize are
// only written when non-zero, and WindowPosX/WindowPosY only when
// WindowPositionSet, so a run that never touched picture-frame mode or
// never got a window-size/position reading (see windowSizeTracker and
// startWindowPosPolling in internal/ui/windowtrack.go) doesn't clobber a
// good value saved by an earlier run.
func Save(app fyne.App, s State) {
	p := app.Preferences()
	p.SetString(keySortMode, s.SortMode)
	p.SetBool(keyMergeMode, s.MergeMode)

	if s.SlideInterval > 0 {
		p.SetFloat(keySlideIntervalS, s.SlideInterval.Seconds())
	}
	if s.WindowSize.Width > 0 && s.WindowSize.Height > 0 {
		p.SetFloat(keyWindowWidth, float64(s.WindowSize.Width))
		p.SetFloat(keyWindowHeight, float64(s.WindowSize.Height))
	}
	if s.WindowPositionSet {
		p.SetInt(keyWindowPosX, s.WindowPosX)
		p.SetInt(keyWindowPosY, s.WindowPosY)
		p.SetBool(keyWindowPosSet, true)
	}
}

// Load returns the previously saved State. SortMode defaults to
// SortByName, matching the app's shipped default, when nothing has been
// saved yet - and the root package's sortModeFromPref falls back to the
// same default for any value it doesn't recognize, so a preferences file
// written by a newer build with a since-removed mode still loads cleanly.
// Every other field defaults to its zero value, which callers already treat
// as "use the built-in default" (a zero SlideInterval falls back to
// slideshow.DefaultInterval, a zero WindowSize to internal/ui's
// startW/startH).
func Load(app fyne.App) State {
	p := app.Preferences()
	return State{
		SortMode:      p.StringWithFallback(keySortMode, SortByName),
		MergeMode:     p.Bool(keyMergeMode),
		SlideInterval: time.Duration(p.Float(keySlideIntervalS) * float64(time.Second)),
		WindowSize: fyne.NewSize(
			float32(p.Float(keyWindowWidth)),
			float32(p.Float(keyWindowHeight)),
		),
		WindowPosX:        p.Int(keyWindowPosX),
		WindowPosY:        p.Int(keyWindowPosY),
		WindowPositionSet: p.Bool(keyWindowPosSet),
	}
}
