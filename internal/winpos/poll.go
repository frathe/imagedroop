// The background sampler that keeps a Tracker current. It lives here rather
// than at each call site because there is nothing app-specific about it: any
// window whose manually-dragged position should outlive it needs exactly
// this loop (internal/ui's main window, and every widgets.Singleton window
// that remembers where it was).

package winpos

import (
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver"
)

// PollInterval is how often Poll samples a window's on-screen position when
// all that is wanted is the last known one - a second is ample for a value
// only read back at shutdown.
const PollInterval = 1 * time.Second

// GestureInterval is the far shorter interval used when the *path* the
// window took matters rather than just where it ended up (see
// internal/wingesture). It is deliberately faster than the roughly ten
// updates a second the OS itself reports a dragged window at, so no genuine
// movement is missed to sampling luck; the duplicate readings that fall out
// of oversampling are the consumer's to discard.
const GestureInterval = 60 * time.Millisecond

// PollAt launches a background goroutine that samples win's on-screen
// position every interval for as long as the window lives, handing each
// successful reading to fn. Failed readings are dropped rather than
// reported: "couldn't ask right now" is not a position.
//
// It has to be a poller rather than an event hook: unlike a resize, a pure
// window drag-move triggers no layout pass at all, and fyne.Window has
// neither a position getter nor a "window moved" callback in the first place
// (see this package's Get, whose RunNative read is the only way to ask the
// OS directly).
//
// win must satisfy driver.NativeWindow for Get to ever succeed; checked once
// up front so a window that can't - the fyne test driver's windows, which is
// what every headless test gets - never has a poller goroutine running
// behind it at all.
//
// skip, when non-nil, suppresses a single reading whenever it reports true:
// internal/ui passes the slideshow's Active, because picture-frame mode
// full-screens the window and a full-screen reading is not the
// manually-placed position this exists to remember - it would clobber the
// value the slideshow captured on the way in for its own exit to restore.
//
// Each reading is hopped onto the main goroutine via fyne.DoAndWait, the
// same pattern internal/filepicker/darwin.go uses for NSOpenPanel: darwin's
// Get reaches into NSWindow.frame through cgo, and RunNative
// (window_darwin.go) runs that callback synchronously on whatever goroutine
// called it rather than marshaling to the main thread itself, so without
// this hop every tick reads AppKit state directly from this background
// goroutine - a plain violation of AppKit's main-thread-only rule. It's
// harmless in isolation, but contends with the main thread's own AppKit/GL
// work closely enough to stall it badly under load - e.g. internal/ui/grid
// uploading a screenful of thumbnail textures - which is what turned
// "instant" grid population into a multi-minute one. Safe to call from here
// specifically because this goroutine is never the UI goroutine (DoAndWait
// would deadlock if it were) - the same guarantee chooseFilesDarwin's own
// doc comment spells out for its call site.
//
// fn therefore runs *inside* that hop, on the UI goroutine, which is both
// why it may touch widgets directly and why it must stay cheap: whatever it
// does is time the main thread is not rendering.
//
// The returned func stops the goroutine, and callers must run it before the
// event loop winds down - at shutdown for a window that lives as long as the
// app, on close for one that doesn't - so the goroutine isn't left blocked
// inside fyne.DoAndWait against a loop that will never run it. A no-op func,
// never nil, when no poller started.
func PollAt(win fyne.Window, interval time.Duration, skip func() bool, fn func(x, y int)) (stop func()) {
	if _, ok := win.(driver.NativeWindow); !ok {
		return func() {}
	}

	done := make(chan struct{})

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
			case <-done:
				return
			}
			if skip != nil && skip() {
				continue
			}
			fyne.DoAndWait(func() {
				if x, y, ok := Get(win); ok {
					fn(x, y)
				}
			})
		}
	}()

	return func() { close(done) }
}

// Poll keeps t current with win's position at PollInterval - the common
// case, and a thin binding of PollAt above, which owns the loop itself and
// every reason it has to be a poller at all. The tracker keeps its last
// reading after the returned stop func runs, so a save afterwards still has
// a value.
func Poll(win fyne.Window, t *Tracker, skip func() bool) (stop func()) {
	return PollAt(win, PollInterval, skip, t.Store)
}
