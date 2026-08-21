package ui

import (
	"fmt"
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/uitest"
)

// This file covers load.go's decode path: ShowImage starting a
// cancellable load lifecycle for each navigation, invalidateLoad
// superseding whatever load came before it, and what happens when a
// decode fails - a broken file is dropped from the set and navigation
// auto-advances past it, a decompression-bomb-sized header is refused on
// attemptLoad's cheap probe rather than paid for with a full decode, and a
// file set where every file is broken falls back to the empty state
// instead of getting stuck.
//
// Neighboring files own the rest of load.go: imgcache_test.go has the
// decoded-image cache and speculative neighbor preloading, animate_test.go
// has the animated-GIF path, and windowsize_test.go has the window resize
// finishLoad performs on a successful decode. This file is the decode
// itself and its failure handling.

// TestInvalidateLoad_CancelsPriorLoadContext checks invalidateLoad's own
// contract: advance the lifecycle and cancel the previous request token.
func TestInvalidateLoad_CancelsPriorLoadContext(t *testing.T) {
	v := newTestViewer(t)

	token := v.loadLifecycle.begin()

	got := v.invalidateLoad()

	if token.context().Err() == nil {
		t.Error("invalidateLoad should cancel the previous generation's load context")
	}
	if got != token.revision+1 {
		t.Errorf("invalidateLoad() = %d, want %d", got, token.revision+1)
	}
	if v.loadLifecycle.currentRevision() != got {
		t.Errorf("load revision = %d, want %d", v.loadLifecycle.currentRevision(), got)
	}
}

// TestInvalidateLoad_ZeroValueIsSafe covers the state before any image has
// ever been shown.
func TestInvalidateLoad_ZeroValueIsSafe(t *testing.T) {
	v := newTestViewer(t)

	v.invalidateLoad() // must not panic
}

// TestShowImage_StartsLoadLifecycle checks that navigation owns a cancellable
// lifecycle request rather than relying only on a revision comparison.
func TestShowImage_StartsLoadLifecycle(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	v.loadLifecycle.mu.Lock()
	hasCancel := v.loadLifecycle.cancel != nil
	v.loadLifecycle.mu.Unlock()
	if !hasCancel {
		t.Error("ShowImage should leave a cancellable load request for preloads and animation")
	}
}

func TestViewerShow_LoadsAndNavigates(t *testing.T) {
	v := newTestViewer(t)

	// Named so natural sort (the default) keeps them in drop order - this
	// test is about ShowImage's load/navigate/wraparound behavior, not
	// sorting, which gets its own test below.
	first := uitest.TempJPEGURI(t, "1.jpg", 10, 10, color.RGBA{R: 255, A: 255})
	second := uitest.TempJPEGURI(t, "2.jpg", 20, 10, color.RGBA{G: 255, A: 255})
	third := uitest.TempJPEGURI(t, "3.jpg", 15, 25, color.RGBA{B: 255, A: 255})

	dropAndWait(t, v, first, second, third)

	if v.state.index != 0 {
		t.Fatalf("index = %d, want 0 after the initial drop", v.state.index)
	}
	if v.img.Image == nil {
		t.Fatal("expected an image to be loaded")
	}
	if b := v.img.Image.Bounds(); b.Dx() != 10 || b.Dy() != 10 {
		t.Errorf("loaded image size = %dx%d, want 10x10", b.Dx(), b.Dy())
	}
	if v.dropzone.Visible() {
		t.Error("dropzone should be hidden once an image is showing")
	}

	// Step forward to the second image.
	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)

	if v.state.index != 1 {
		t.Fatalf("index = %d, want 1 after stepping forward", v.state.index)
	}
	if b := v.img.Image.Bounds(); b.Dx() != 20 || b.Dy() != 10 {
		t.Errorf("loaded image size = %dx%d, want 20x10", b.Dx(), b.Dy())
	}

	// Right at the end wraps around to the first image.
	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)
	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)

	if v.state.index != 0 {
		t.Fatalf("index = %d, want wraparound to 0", v.state.index)
	}

	// Left from the first image wraps around to the last one.
	v.ShowImage(v.state.index - 1)
	waitUntilLoaded(t, v)

	if v.state.index != 2 {
		t.Fatalf("index = %d, want wraparound to the last index (2)", v.state.index)
	}
	if b := v.img.Image.Bounds(); b.Dx() != 15 || b.Dy() != 25 {
		t.Errorf("loaded image size = %dx%d, want 15x25", b.Dx(), b.Dy())
	}
}

func TestViewerShow_DecodeErrorKeepsHint(t *testing.T) {
	v := newTestViewer(t)

	corrupt := storage.NewFileURI(uitest.WriteTempFile(t, "corrupt.jpg", []byte("not a jpeg")))

	dropAndWait(t, v, corrupt)

	if v.img.Image != nil {
		t.Error("no image should be loaded after a decode error")
	}
	if got, want := v.hint.Text, lang.L("Drop images here"); got != want {
		t.Errorf("hint text = %q, want %q after a decode error on first load", got, want)
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast to be shown after a decode error")
	}
	settleToast(t, v)
}

// TestViewerShow_RejectsAbsurdHeaderDimensions checks the end-to-end wiring
// from a decompression-bomb-sized header, through attemptLoad's
// errors.As(*imaging.InvalidDimensionsError) branch, to the same
// "invalid image dimensions" toast the old post-decode zero-dimension check
// used to produce - now reached via the cheap header probe instead of a
// full (and here, impossible - the file has no IDAT/IEND) decode.
func TestViewerShow_RejectsAbsurdHeaderDimensions(t *testing.T) {
	v := newTestViewer(t)

	bomb := storage.NewFileURI(uitest.WriteTempFile(t, "bomb.png", uitest.TruncatedPNGHeader(t, 60000, 60000)))

	dropAndWait(t, v, bomb)

	if v.img.Image != nil {
		t.Error("no image should be loaded after a rejected header")
	}
	if !v.toast.card.Visible() {
		t.Fatal("expected a toast to be shown after a rejected header")
	}
	if got, want := v.toast.text.Text, fmt.Sprintf(lang.L("invalid image dimensions for %q"), "bomb.png"); got != want {
		t.Errorf("toast text = %q, want %q", got, want)
	}
	settleToast(t, v)
}

func TestViewerShow_AutoAdvancesPastBrokenFileDuringNavigation(t *testing.T) {
	v := newTestViewer(t)

	// Named so natural sort (the default) keeps them in this order.
	first := uitest.TempJPEGURI(t, "1.jpg", 4, 4, color.RGBA{R: 255, A: 255})
	corrupt := storage.NewFileURI(uitest.WriteTempFile(t, "2.jpg", []byte("not a jpeg")))
	third := uitest.TempJPEGURI(t, "3.jpg", 4, 4, color.RGBA{B: 255, A: 255})

	dropAndWait(t, v, first, corrupt, third)

	if len(v.state.files) != 3 {
		t.Fatalf("files = %v, want all 3 dropped files kept until navigation actually reaches the broken one", v.state.files)
	}

	// Step onto the broken file.
	v.ShowImage(v.state.index + 1)
	waitUntilLoaded(t, v)

	if len(v.state.files) != 2 {
		t.Fatalf("files = %v, want the broken file dropped from the set", v.state.files)
	}
	for _, u := range v.state.files {
		if u.Name() == "2.jpg" {
			t.Errorf("files = %v, the broken file should have been removed", v.state.files)
		}
	}
	if got := v.state.files[v.state.index].Name(); got != "3.jpg" {
		t.Errorf("displayed file = %q, want auto-advance to land on 3.jpg", got)
	}
	if v.img.Image == nil {
		t.Fatal("expected the auto-advanced-to image to be loaded, not left blank")
	}
	if got, want := v.win.Title(), "3.jpg"; !strings.Contains(got, want) {
		t.Errorf("title = %q, want it to reflect the auto-advanced-to file %q, not the stale broken one", got, want)
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast reporting the broken file was skipped")
	}
	settleToast(t, v)
}

func TestViewerShow_AutoAdvancesPastBrokenFirstFile(t *testing.T) {
	v := newTestViewer(t)

	corrupt := storage.NewFileURI(uitest.WriteTempFile(t, "1.jpg", []byte("not a jpeg")))
	second := uitest.TempJPEGURI(t, "2.jpg", 4, 4, color.RGBA{G: 255, A: 255})

	dropAndWait(t, v, corrupt, second)

	if len(v.state.files) != 1 || v.state.files[0].Name() != "2.jpg" {
		t.Fatalf("files = %v, want only 2.jpg left after the broken first file was auto-skipped", v.state.files)
	}
	if v.img.Image == nil {
		t.Fatal("expected the app to auto-advance to the one good image instead of giving up on the first failure")
	}
	if v.dropzone.Visible() {
		t.Error("dropzone should be hidden once the auto-advanced-to image is showing")
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast reporting the broken file was skipped")
	}
	settleToast(t, v)
}

func TestViewerShow_AllFilesBrokenFallsBackToEmptyState(t *testing.T) {
	v := newTestViewer(t)

	corrupt1 := storage.NewFileURI(uitest.WriteTempFile(t, "1.jpg", []byte("not a jpeg")))
	corrupt2 := storage.NewFileURI(uitest.WriteTempFile(t, "2.jpg", []byte("also not a jpeg")))

	dropAndWait(t, v, corrupt1, corrupt2)

	if v.state.files != nil {
		t.Errorf("files = %v, want nil once every dropped file has failed to decode", v.state.files)
	}
	if v.img.Image != nil {
		t.Error("no image should be displayed once every file has failed")
	}
	if !v.dropzone.Visible() {
		t.Error("dropzone should be visible again once every file has failed")
	}
	if !v.emptyStateArt.Visible() {
		t.Error("emptyStateArt should be shown once every file has failed")
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast reporting the failure")
	}
	settleToast(t, v)
}

// TestViewerShow_RAWPreviewMarksTheTitle is the UI half of imaging's
// Preview flag: a camera RAW is shown via its embedded JPEG, and the
// window title has to say so rather than looking like a finished decode.
func TestViewerShow_RAWPreviewMarksTheTitle(t *testing.T) {
	v := newTestViewer(t)

	raw := uitest.TempRAWURI(t, "photo.cr2", 20, 10, color.White)
	dropAndWait(t, v, raw)

	got := v.win.Title()
	if !strings.Contains(got, "photo.cr2") {
		t.Errorf("title = %q, want the RAW filename", got)
	}
	if !strings.Contains(got, "20 x 10") {
		t.Errorf("title = %q, want the preview's pixel size", got)
	}
	if !strings.Contains(got, lang.L("(preview)")) {
		t.Errorf("title = %q, want %q so a RAW preview is not mistaken for a demosaic", got, lang.L("(preview)"))
	}
}
