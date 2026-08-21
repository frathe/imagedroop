// The window-drag gesture: swirl the main window around the desktop in a
// spiral and the Hypno Spiral easter egg opens, on the pattern the
// direction picks - clockwise the Nautilus, counter-clockwise the Ripple.
// This file is only the app's binding of it - the recognition is
// internal/wingesture, which is pure geometry over timestamped positions,
// and the payoff is internal/ui/help's OpenSpiral.
//
// It rides on the position poller that already existed for remembering
// where the user left the window (windowtrack.go), rather than starting a
// second one: the two want the same readings, only at different rates, and
// one poller at the faster rate serves both.

package ui

import (
	"time"

	"github.com/frathe/picfetch/internal/wingesture"
)

// recordWindowPosition is the poller's callback: every sampled position
// updates the remembered one and is offered to the spiral detector. It runs
// on the UI goroutine (winpos.PollAt hops each reading there for AppKit's
// sake), which is what lets a detected gesture open a window from here
// without any further marshalling.
//
// The timestamp is taken here rather than passed in because the sample was
// read moments earlier in the same hop; nothing in the recognition is
// sensitive at that scale - the clock only distinguishes a continuous
// motion from a pause, four hundred milliseconds apart.
func (v *viewer) recordWindowPosition(x, y int) {
	v.winPos.Store(x, y)

	if v.spiralDrag == nil || v.spiralGesture == nil {
		return
	}

	if res := v.spiralDrag.Add(time.Now(), x, y); res.Detected {
		v.spiralGesture(res.Direction == wingesture.Clockwise)
	}
}
