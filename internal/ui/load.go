// Loading, displaying, preloading, and animating images.

package ui

import (
	"errors"
	"fmt"
	"image"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/lang"

	"github.com/frathe/imagedrop/internal/imaging"
)

// show loads and displays the file at index i, wrapping around at both
// ends. A file that fails to decode is dropped from the set and the next
// one is tried automatically - see attemptLoad - so a bad file never gets
// stuck on screen or left inconsistent with v.index.
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

	v.loading.Store(true)
	v.loadingBar.Show()

	if firstLoad {
		v.hint.SetText(lang.L("Loading..."))
		v.dropzone.Show()
	}
	v.ForceRepaint()

	// A new generation invalidates any decode/retry chain still in flight,
	// so a slow load can never overwrite a newer selection. Every retry in
	// attemptLoad below - for a file that turns out to be broken - shares
	// this one generation and this one done channel: they're all part of
	// the same logical navigation, not independent ones, so a genuinely
	// newer show() call (which bumps gen again) correctly invalidates the
	// whole chain, and a waiter on done sees the chain as finished only
	// once it truly settles instead of racing whichever retry closes a
	// channel first.
	gen := v.gen.Add(1)

	done := make(chan struct{})
	v.loadDone = done

	v.attemptLoad(i, gen, done)
}

// attemptLoad decodes and displays v.files[i] (wrapped into range), sharing
// gen and done with the rest of its retry chain - see show's comment. It
// first reads the file and probes just its header (imaging.ReadAndProbe), which is
// enough to reject an invalid file instantly, without spending time on a
// full pixel decode that was only going to be thrown away, and to resize
// the window to its final size before that full decode even starts. On
// failure it drops that file via RemoveFile and retries at the same
// position, which now holds what used to be the next file (or wraps
// around to the first, if i was the last); once nothing is left it falls
// back to the empty-state error screen.
func (v *viewer) attemptLoad(i int, gen uint64, done chan struct{}) {
	n := len(v.files)
	i = ((i % n) + n) % n
	v.index = i
	u := v.files[i]

	// A cache hit - either a file already viewed this session, or one
	// preloadNeighbors decoded speculatively ahead of time - skips the disk
	// read and decode entirely and finishes synchronously, right here on
	// the UI goroutine that called show(). No fyne.Do hop is needed since
	// we're already on it.
	if loaded, ok := v.imgCache.Get(u.String()); ok {
		v.finishLoad(i, u, loaded, gen, done)
		return
	}

	go func() {
		data, bounds, err := imaging.ReadAndProbe(u)

		if err == nil {
			fyne.Do(func() {
				// In picture-frame mode the window is already full-screen
				// and there's nothing to resize to, same as the final
				// resize below.
				if gen == v.gen.Load() && !v.slides.Active() {
					v.undoGridMaximize()
					resizeToImage(v.win, bounds)
				}
			})
		}

		var loaded *imaging.LoadedImage
		if err == nil {
			loaded, err = imaging.DecodeLoaded(data)
			if err == nil {
				loaded.FileSize = int64(len(data))
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
				if errors.As(err, &dimErr) {
					msg = fmt.Sprintf(lang.L("invalid image dimensions for %q"), u.Name())
				}
				v.retryAfterLoadFailure(msg, i, gen, done)
				return
			}

			b := loaded.Frames[0].Bounds()

			if b.Dx() == 0 || b.Dy() == 0 {
				msg := fmt.Sprintf(lang.L("invalid image dimensions for %q"), u.Name())
				v.retryAfterLoadFailure(msg, i, gen, done)
				return
			}

			v.imgCache.Add(u.String(), loaded)
			v.finishLoad(i, u, loaded, gen, done)
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
// goroutine called show()).
func (v *viewer) finishLoad(i int, u fyne.URI, loaded *imaging.LoadedImage, gen uint64, done chan struct{}) {
	b := loaded.Frames[0].Bounds()

	v.displayFrames = loaded.Frames
	v.displayFrameIdx = 0
	v.rotation = 0
	v.redrawRotatedFrame()
	v.img.Show()
	v.dropzone.Hide()
	v.emptyStateArt.Hide()

	v.currentFileSize = loaded.FileSize
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
	// asking for platform-specific trouble.
	if !v.slides.Active() {
		v.undoGridMaximize()
		resizeToImage(v.win, b)
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
	v.preloadNeighbors(gen)

	close(done)
}

// preloadNeighbors speculatively decodes the files immediately before and
// after v.index in the background, so stepping to either one next is a
// cache hit instead of a fresh disk read + decode. Always called from
// finishLoad before done closes - see its comment - so reading
// v.files/v.index here can't race a waiter that's about to mutate them.
func (v *viewer) preloadNeighbors(gen uint64) {
	n := len(v.files)
	if n < 2 {
		return
	}

	next := ((v.index+1)%n + n) % n
	prev := ((v.index-1)%n + n) % n

	v.preloadOne(v.files[next], gen)
	if prev != next {
		v.preloadOne(v.files[prev], gen)
	}
}

// preloadConcurrency bounds how many preloadOne decodes run at once - see
// the preloadSem field comment on the viewer struct.
const preloadConcurrency = 2

// preloadOne decodes u in the background and adds it to imgCache, unless
// it's already cached or another preload of the same URI is already in
// flight. gen is checked before and after the decode so a preload started
// for a set of files that's since been replaced by a fresh drop doesn't
// keep working, or land a stale result, after the fact.
func (v *viewer) preloadOne(u fyne.URI, gen uint64) {
	key := u.String()

	if _, cached := v.imgCache.Get(key); cached {
		return
	}
	if _, inFlight := v.preloading.LoadOrStore(key, struct{}{}); inFlight {
		return
	}

	v.preloadPending.Add(1)
	go func() {
		defer v.preloadPending.Done()
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

		data, _, err := imaging.ReadAndProbe(u)
		if err != nil {
			return
		}

		loaded, err := imaging.DecodeLoaded(data)
		if err != nil {
			return
		}
		loaded.FileSize = int64(len(data))

		b := loaded.Frames[0].Bounds()
		if b.Dx() == 0 || b.Dy() == 0 {
			return
		}

		if gen != v.gen.Load() {
			return
		}

		v.imgCache.Add(key, loaded)
	}()
}

// stopAnimation wakes the current animate goroutine, if any, out of its
// frame-delay sleep so it exits right away. Called wherever a gen bump
// supersedes a possibly-playing animation (show, clearToDropzone,
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
// for why gen and done are threaded through unchanged rather than starting
// a fresh chain.
func (v *viewer) retryAfterLoadFailure(msg string, i int, gen uint64, done chan struct{}) {
	v.RemoveFile(i)

	if len(v.files) == 0 {
		v.ShowEmptyStateError(msg)
		close(done)
		return
	}

	v.ShowToast(msg)
	v.attemptLoad(i, gen, done)
}

// animate cycles an animated GIF's frames on their own goroutine, sleeping
// between frames for each one's delay and updating the canvas image via
// fyne.Do. It stops on its own once gen no longer matches the viewer's
// current generation, the same staleness check show's decode goroutine
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

func resizeToImage(w fyne.Window, b image.Rectangle) {
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
