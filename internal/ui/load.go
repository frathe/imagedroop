// Loading, displaying, preloading, and animating images.

package ui

import (
	"context"
	"errors"
	"fmt"
	"image"
	"math/rand/v2"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/picfetch/internal/imaging"
)

// ShowImage loads and displays the file at index i, wrapping around at
// both ends. A file that fails to decode is dropped from the set and the
// next one is tried automatically - see attemptLoad - so a bad file never
// gets stuck on screen or left inconsistent with v.index.
func (v *viewer) ShowImage(i int) {
	if len(v.files) == 0 {
		return
	}

	// A playing animation belongs to the image being navigated away from;
	// stop it now, alongside the gen bump below that would make it stale
	// anyway, so it exits immediately instead of at its next frame tick.
	v.stopAnimation()

	// Once an image is on screen we keep showing it until the new one is
	// ready, instead of blanking out to the drop-hint on every navigation.
	firstLoad := v.img.Image == nil

	// In picture-frame mode, fade the outgoing image out instead of the
	// usual instant swap - finishLoad fades the incoming one back in once
	// it's ready. Skipped on the very first image of a session (nothing on
	// screen yet to fade from) and left alone everywhere else, so ordinary
	// browsing stays an instant swap exactly as before.
	if v.slides.Active() && !firstLoad {
		v.startFade(0, 1)
	}

	v.loading.Store(true)
	v.loadingBar.Show()
	v.updateFileMenuState() // grey out Save Changes immediately - see canSaveRotation's !v.loading.Load() guard

	if firstLoad {
		v.hint.SetText(lang.L("Loading..."))
		v.dropzone.Show()
	}
	v.ForceRepaint()

	// A new generation invalidates any decode/retry chain still in flight,
	// so a slow load can never overwrite a newer selection. Every retry in
	// attemptLoad below - for a file that turns out to be broken - shares
	// this one generation, this one done channel, and this one ctx: they're
	// all part of the same logical navigation, not independent ones, so a
	// genuinely newer ShowImage() call (which bumps gen again, via
	// invalidateLoad below) correctly invalidates the whole chain - and,
	// via ctx, stops attemptLoad's/preloadOne's I/O instead of just
	// discarding a result they'd otherwise run to completion for - and a
	// waiter on done sees the chain as finished only once it truly settles
	// instead of racing whichever retry closes a channel first.
	gen := v.invalidateLoad()

	ctx, cancel := context.WithCancel(context.Background())
	v.loadCancel = cancel

	done := make(chan struct{})
	v.loadDone = done

	v.attemptLoad(ctx, i, gen, done)
}

// invalidateLoad bumps gen and, if a load's decode/preload work is
// currently in flight, cancels its context - mirroring invalidateSort
// (sort.go) for the load/preload generation instead of the sort one. This
// is what makes attemptLoad's and preloadOne's own ReadAndProbe/
// DecodeLoaded calls notice and stop doing I/O for a superseded generation,
// instead of running to completion for a result the gen check they already
// make would only end up discarding anyway. Called by ShowImage (a fresh
// navigation) and every other site that already bumped gen directly before
// this existed: cancelScan, handleDrop (drop.go), and clearToDropzone
// (viewer.go).
func (v *viewer) invalidateLoad() uint64 {
	gen := v.gen.Add(1)
	if v.loadCancel != nil {
		v.loadCancel()
	}
	return gen
}

// attemptLoad decodes and displays v.files[i] (wrapped into range), sharing
// gen, done, and ctx with the rest of its retry chain - see ShowImage's
// comment. It first reads the file and probes just its header
// (imaging.ReadAndProbe), which is enough to reject an invalid file
// instantly, without spending time on a full pixel decode that was only
// going to be thrown away, and to resize the window to its final size
// before that full decode even starts. On failure it drops that file via
// RemoveFile and retries at the same position, which now holds what used
// to be the next file (or wraps around to the first, if i was the last);
// once nothing is left it falls back to the empty-state error screen.
func (v *viewer) attemptLoad(ctx context.Context, i int, gen uint64, done chan struct{}) {
	n := len(v.files)
	i = ((i % n) + n) % n
	v.index = i
	u := v.files[i]

	// A cache hit - either a file already viewed this session, or one
	// preloadNeighbors decoded speculatively ahead of time - skips the disk
	// read and decode entirely and finishes synchronously, right here on
	// the UI goroutine that called ShowImage(). No fyne.Do hop is needed since
	// we're already on it.
	if loaded, ok := v.imgCache.Get(u.String()); ok {
		v.finishLoad(ctx, i, u, loaded, gen, done)
		return
	}

	go func() {
		data, bounds, err := imaging.ReadAndProbe(ctx, u)

		if err == nil {
			fyne.Do(func() {
				// In picture-frame mode the window is already full-screen
				// and there's nothing to resize to, same as the final
				// resize below. The grid overview is skipped for the same
				// reason it maximized the window in the first place: it
				// fills the whole window, so sizing that window to one
				// image means nothing while it's up - and undoGridMaximize
				// would actively shrink it back out from under the open
				// grid. Only reachable since the grid's batch delete, which
				// re-shows whatever takes a deleted file's place without
				// closing the grid first.
				if gen == v.gen.Load() && !v.slides.Active() && !v.grid.Visible() {
					v.undoGridMaximize()
					resizeToImage(v.win, bounds, v.maxWinW, v.maxWinH)
				}
			})
		}

		var loaded *imaging.LoadedImage
		if err == nil {
			// The image cache's budget doubles as the animation budget: an
			// animation whose composited frames couldn't fit in the cache
			// at all is exactly the one not worth compositing, so this
			// needs no limit of its own.
			loaded, err = imaging.DecodeLoaded(ctx, data, v.imgCache.Budget())
			if err == nil {
				loaded.FileSize = int64(len(data))
				loaded.HasEXIF = !imaging.ReadMetadata(data).Empty()
			}
		}

		fyne.Do(func() {
			if gen != v.gen.Load() {
				close(done) // user already navigated elsewhere
				return
			}

			if err != nil {
				msg := fmt.Sprintf(lang.L("could not read %q: %v"), u.Name(), err)

				var dimErr *imaging.InvalidDimensionsError
				var bigErr *imaging.InputTooLargeError

				switch {
				case errors.As(err, &dimErr):
					msg = fmt.Sprintf(lang.L("invalid image dimensions for %q"), u.Name())
				case errors.As(err, &bigErr):
					msg = fmt.Sprintf(lang.L("%q is too large to open"), u.Name())
				}

				v.retryAfterLoadFailure(ctx, msg, i, gen, done)
				return
			}

			b := loaded.Frames[0].Bounds()

			if b.Dx() == 0 || b.Dy() == 0 {
				msg := fmt.Sprintf(lang.L("invalid image dimensions for %q"), u.Name())
				v.retryAfterLoadFailure(ctx, msg, i, gen, done)
				return
			}

			// Reported here rather than in finishLoad, which a cache hit
			// also runs: the user needs telling once, on the decode that
			// discovered it, not again every time they navigate back.
			if loaded.AnimationTruncated {
				v.ShowToast(fmt.Sprintf(lang.L("animation in %q is too large to play"), u.Name()))
			}

			v.imgCache.Add(u.String(), loaded)
			v.finishLoad(ctx, i, u, loaded, gen, done)
		})
	}()
}

// finishLoad displays loaded - already decoded, either just now or earlier
// and pulled from imgCache - as v.files[i], updates the window title/size
// and animation state, kicks off speculative preloading of its neighbors,
// and closes done last. Shared by attemptLoad's disk-decode path (called
// from inside its completion fyne.Do, which - like every fyne.Do callback
// in this file - the real driver runs on the UI goroutine but the fyne
// test driver runs synchronously on whatever goroutine called it) and its
// cache-hit path (called directly from attemptLoad, always on whichever
// goroutine called ShowImage()).
func (v *viewer) finishLoad(ctx context.Context, _ int, u fyne.URI, loaded *imaging.LoadedImage, gen uint64, done chan struct{}) {
	b := loaded.Frames[0].Bounds()

	v.displayFrames = loaded.Frames
	v.displayFrameIdx = 0
	v.rotation = 0

	// In picture-frame mode, the outgoing image was left fading toward
	// invisible by ShowImage's startFade(0, 1) above (or already is, if
	// that fade had time to finish); forcing it the rest of the way there
	// right before the swap hides the new pixels landing mid-fade, then
	// the fade-in below takes over from a clean, fully-invisible start.
	if v.slides.Active() {
		v.img.Translucency = 1
	}
	v.redrawRotatedFrame()
	if v.slides.Active() {
		v.startFade(1, 0)
	}
	v.img.Show()
	v.dropzone.Hide()
	v.emptyStateArt.Hide()

	v.currentFileSize = loaded.FileSize
	v.currentHasEXIF = loaded.HasEXIF
	v.syncInfoOverlayVisibility()
	v.exif.Refresh()

	// Every fresh navigation starts back at fit-to-window rather than
	// keeping whatever zoom/pan the previous image was left at - a manual
	// zoom level rarely still makes sense for an unrelated next image.
	// Applied directly (not just left for the resize below to trigger)
	// since picture-frame mode skips that resize entirely.
	v.zoom.ResetToFit()

	// In picture-frame mode the window is already full-screen and
	// ImageFillContain scales the image to fit it without stretching, so
	// there's nothing to resize to - and resizing a full-screen window is
	// asking for platform-specific trouble. The grid overview is skipped on
	// the same grounds and for the reason spelled out at the probe-time
	// resize above: it fills the window it maximized, and undoGridMaximize
	// would shrink that window while the grid is still drawn over it.
	if !v.slides.Active() && !v.grid.Visible() {
		v.undoGridMaximize()
		resizeToImage(v.win, b, v.maxWinW, v.maxWinH)
	}

	title := fmt.Sprintf("%s — %d x %d", u.Name(), b.Dx(), b.Dy())

	// The slideshow uses this so an animated GIF always gets to play at
	// least one full loop before auto-advancing - see
	// internal/ui/slideshow. Set unconditionally (0 for a static image) so
	// a GIF's duration never leaks into the next, static image.
	animDuration := time.Duration(0)
	if len(loaded.Frames) > 1 {
		title += " (animated)"
		for _, d := range loaded.Delays {
			animDuration += d
		}
	}
	v.slides.SetAnimDuration(animDuration)

	if n := len(v.files); n > 1 {
		title = fmt.Sprintf("%s  (%d/%d)", title, v.index+1, n)
	}

	v.setTitle(title)

	v.loading.Store(false)
	v.loadingBar.Hide()
	v.updateFileMenuState() // rotation just reset to 0 above, and loading has just cleared - see canSaveRotation
	v.ForceRepaint()

	// Animated GIFs keep playing until a newer generation (a navigation or
	// a fresh drop) supersedes this one; animate checks gen itself so no
	// separate cancellation is needed. It's spawned only after
	// ForceRepaint above has finished, not before via a defer: under the
	// real driver both go through the same serialized fyne.Do queue either
	// way, but the fyne test driver runs fyne.Do synchronously on the
	// calling goroutine, so spawning animate first let its own first-frame
	// Refresh race with this goroutine's still-running ForceRepaint.
	if len(loaded.Frames) > 1 {
		stopped := make(chan struct{})
		stop := make(chan struct{})
		v.animStopped = stopped
		v.animStop = stop
		go v.animate(gen, loaded.Frames, loaded.Delays, stop, stopped)
	}

	// Must run - and finish reading v.files/v.index - before done closes
	// below: done's close is what a waiter (a test's waitUntilLoaded, or a
	// future navigation) synchronizes on to know this call is finished
	// touching viewer state. Under the fyne test driver, this whole
	// function already runs on whatever goroutine called fyne.Do rather
	// than a dedicated UI goroutine (see attemptLoad's comment on gen), so
	// closing done first would let a waiter go on to mutate v.files - via
	// reset() or a fresh drop - concurrently with this read.
	v.preloadNeighbors(ctx, gen)

	close(done)
}

// preloadNeighbors speculatively decodes the files immediately before and
// after v.index in the background, so stepping to either one next is a
// cache hit instead of a fresh disk read + decode. Always called from
// finishLoad before done closes - see its comment - so reading
// v.files/v.index here can't race a waiter that's about to mutate them.
// ctx is the same one ShowImage created for this generation - the
// preloads it starts belong to the generation that's now on screen, so
// they get cancelled alongside its own decode the moment a newer
// navigation or drop supersedes it (see invalidateLoad).
func (v *viewer) preloadNeighbors(ctx context.Context, gen uint64) {
	n := len(v.files)
	if n < 2 {
		return
	}

	next := ((v.index+1)%n + n) % n
	prev := ((v.index-1)%n + n) % n

	v.preloadOne(ctx, v.files[next], gen)
	if prev != next {
		v.preloadOne(ctx, v.files[prev], gen)
	}
}

// preloadConcurrency bounds how many preloadOne decodes run at once - see
// the preloadSem field comment on the viewer struct.
const preloadConcurrency = 2

// preloadOne decodes u in the background and adds it to imgCache, unless
// it's already cached or another preload of the same URI is already in
// flight. gen is checked before and after the decode so a preload started
// for a set of files that's since been replaced by a fresh drop doesn't
// keep working, or land a stale result, after the fact; ctx backs that up
// by making ReadAndProbe/DecodeLoaded themselves stop doing I/O partway
// through, for a preload that goes stale while it's actually running
// rather than while still queued behind preloadSem.
func (v *viewer) preloadOne(ctx context.Context, u fyne.URI, gen uint64) {
	key := u.String()

	// Contains, not Get: a presence test on a speculative path shouldn't
	// promote the neighbor to most-recently-used, which under a tight byte
	// budget could make it outlive the image actually on screen.
	if v.imgCache.Contains(key) {
		return
	}
	if _, inFlight := v.preloading.LoadOrStore(key, struct{}{}); inFlight {
		return
	}

	v.preloadPending.Go(func() {
		defer v.preloading.Delete(key)

		// Bounded the same way the grid's thumbnail decodes are:
		// preloadNeighbors only ever asks for two files per settled image,
		// but rapid navigation could otherwise stack an unbounded number
		// of these full-size decode goroutines.
		v.preloadSem <- struct{}{}
		defer func() { <-v.preloadSem }()

		if gen != v.gen.Load() {
			return
		}

		data, bounds, err := imaging.ReadAndProbe(ctx, u)
		if err != nil {
			return
		}

		// Read once: the settings window can change the budget between
		// these two uses, and a gate that passed under one value shouldn't
		// then decode under another.
		budget := v.imgCache.Budget()

		// Preloading exists to make the *next* navigation instant. An
		// image big enough that caching it would evict what's on screen
		// turns that speculative win into a guaranteed re-decode of the
		// current image, so bail on the header alone rather than paying
		// for the decode first. Half the budget is where the current image
		// and one neighbor stop both fitting.
		if imaging.EstimateDecodedBytes(bounds) > budget/2 {
			return
		}

		loaded, err := imaging.DecodeLoaded(ctx, data, budget)
		if err != nil {
			return
		}
		loaded.FileSize = int64(len(data))
		loaded.HasEXIF = !imaging.ReadMetadata(data).Empty()

		b := loaded.Frames[0].Bounds()
		if b.Dx() == 0 || b.Dy() == 0 {
			return
		}

		if gen != v.gen.Load() {
			return
		}

		// AddIfFits, not Add: nothing is displaying this image, so a
		// refusal costs only the decode that just happened, whereas Add's
		// never-evict-the-newest rule would let a preloaded neighbor
		// displace the image the user is looking at.
		_ = v.imgCache.AddIfFits(key, loaded)
	})
}

// stopAnimation wakes the current animate goroutine, if any, out of its
// frame-delay sleep so it exits right away. Called wherever a gen bump
// supersedes a possibly-playing animation (ShowImage, clearToDropzone,
// handleDrop, cancelScan); the nil-out keeps it idempotent, and it only
// ever runs on the UI goroutine so the field swap needs no
// synchronization. animStopped still signals the actual exit.
func (v *viewer) stopAnimation() {
	if v.animStop != nil {
		close(v.animStop)
		v.animStop = nil
	}
}

// retryAfterLoadFailure reports msg, drops v.files[i], and either continues
// the retry chain via attemptLoad or, if that emptied the set, falls back
// to the empty-state error screen and finalizes done. See show/attemptLoad
// for why gen, done, and ctx are threaded through unchanged rather than
// starting a fresh chain.
func (v *viewer) retryAfterLoadFailure(ctx context.Context, msg string, i int, gen uint64, done chan struct{}) {
	v.RemoveFile(i)

	if len(v.files) == 0 {
		v.ShowEmptyStateError(msg)
		close(done)
		return
	}

	v.ShowToast(msg)
	v.attemptLoad(ctx, i, gen, done)
}

// animate cycles an animated GIF's frames on their own goroutine, sleeping
// between frames for each one's delay and updating the canvas image via
// fyne.Do. It stops on its own once gen no longer matches the viewer's
// current generation, the same staleness check ShowImage's decode goroutine
// uses, so a navigation or a fresh drop ends the previous animation without
// any extra cancellation plumbing. stopped is closed right before it
// returns, and animFrame is bumped after every frame write, so tests can
// wait on those instead of reading v.img.Image from another goroutine - see
// the animFrame/animStopped comment on the viewer struct.
func (v *viewer) animate(gen uint64, frames []image.Image, delays []time.Duration, stop, stopped chan struct{}) {
	idx := 0

	for {
		select {
		case <-time.After(delays[idx]):
		case <-stop:
			// stopAnimation woke us mid-delay: a navigation or reset has
			// already superseded this animation, so exit right away instead
			// of sleeping out the rest of the frame delay just to discover
			// the stale generation below.
			close(stopped)
			return
		}

		stale := false

		fyne.Do(func() {
			if gen != v.gen.Load() {
				stale = true
				return
			}

			idx = (idx + 1) % len(frames)
			v.displayFrameIdx = idx
			v.redrawRotatedFrame()
		})

		if stale {
			close(stopped)
			return
		}
	}
}

// defaultMaxWindowWidth/defaultMaxWindowHeight cap how large the window is
// ever allowed to auto-grow to fit a loaded image, until the settings
// window (internal/ui/settingswin) changes them - see the viewer's
// maxWinW/maxWinH fields and MaxWindowWidth/MaxWindowHeight below.
const (
	defaultMaxWindowWidth  = 1500.0
	defaultMaxWindowHeight = 950.0
)

// MaxWindowWidth/MaxWindowHeight report the current window-size cap - the
// settings window's getters.
func (v *viewer) MaxWindowWidth() float32  { return v.maxWinW }
func (v *viewer) MaxWindowHeight() float32 { return v.maxWinH }

// SetMaxWindowWidth/SetMaxWindowHeight set the window-size cap directly -
// the settings window's binding. Floored at the drop-zone size
// (startW/startH): resizeToImage already never shrinks the window below
// that regardless of the cap, so a lower value would silently have no
// effect - flooring here instead of just letting that happen keeps what
// the settings window shows in sync with what the window actually does.
func (v *viewer) SetMaxWindowWidth(w float32) {
	if w < startW {
		w = startW
	}
	v.maxWinW = w
}

func (v *viewer) SetMaxWindowHeight(h float32) {
	if h < startH {
		h = startH
	}
	v.maxWinH = h
}

// resizeToImage resizes w to fit b, scaled down (preserving aspect ratio)
// so neither dimension exceeds maxW/maxH, and never below startW/startH.
func resizeToImage(w fyne.Window, b image.Rectangle, maxW, maxH float32) {
	width := float32(b.Dx())
	height := float32(b.Dy())

	if f := min(maxW/width, maxH/height, float32(1.0)); f < 1 {
		width *= f
		height *= f
	}

	// Never shrink below the drop-zone size — a tiny thumbnail would
	// otherwise produce a window too small to grab or read the title of.
	// ImageFillContain letterboxes the image within the larger frame.
	w.Resize(fyne.NewSize(max(width, startW), max(height, startH)))
}

// slideshowFadeDuration is how long each half of a picture-frame-mode
// transition takes: the outgoing image fades to invisible, then the
// incoming one fades in from invisible - a full transition takes about
// twice this long, overlapping however much of the load it happens to
// take. Ordinary browsing (picture-frame mode off) never calls startFade
// at all, so it stays an instant swap exactly as before.
const slideshowFadeDuration = 400 * time.Millisecond

// startFade stops whatever fade is already running - a no-op if none is -
// and starts a fresh one ticking v.img's Translucency from start to end
// over slideshowFadeDuration, refreshing the canvas on every tick.
// Stopping the previous animation first matters when a fade-in begins
// before the fade-out before it has finished (a fast, likely
// cache-hit load - see attemptLoad): without it, the outgoing animation's
// next tick could overwrite a value the new one already set. Under the
// fyne test driver, Start ticks straight to the end state synchronously
// (see fyne/test's driver.StartAnimation), so a test never observes an
// in-between value.
func (v *viewer) startFade(start, end float64) {
	if v.fadeAnim != nil {
		v.fadeAnim.Stop()
	}

	v.fadeAnim = fyne.NewAnimation(slideshowFadeDuration, func(t float32) {
		v.img.Translucency = start + float64(t)*(end-start)
		v.img.Refresh()
	})
	v.fadeAnim.Start()
}

// resetFade cancels any fade transition in progress and puts v.img back to
// fully opaque. Called from every place picture-frame mode ends, so
// leaving it mid-transition never strands the image invisible or
// half-faded once it's back in the normal, instant-swap view.
func (v *viewer) resetFade() {
	if v.fadeAnim != nil {
		v.fadeAnim.Stop()
		v.fadeAnim = nil
	}
	v.img.Translucency = 0
	v.img.Refresh()
}

// randomOtherIndex picks a uniformly random index in [0,n) other than
// current - Advance's shuffle-mode step. Picking from the n-1 indices that
// aren't current and shifting the ones at or past it up by one, rather
// than rejection-sampling rand.IntN(n) until it misses, keeps this O(1)
// and never repeats the image already on screen, which a plain
// rand.IntN(n) would occasionally do. n<=1 has no "other" index to pick,
// so it returns current unchanged - Advance never calls this in that case
// (the slideshow doesn't run at all with fewer than two files), but a
// direct caller (this function's own tests included) gets a safe answer
// either way instead of a panic or an out-of-range index.
func randomOtherIndex(n, current int) int {
	if n <= 1 {
		return current
	}

	next := rand.IntN(n - 1)
	if next >= current {
		next++
	}

	return next
}
