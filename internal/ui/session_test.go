package ui

import (
	"image/color"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/session"
	"github.com/frathe/picfetch/internal/uitest"
)

// Save/Load round-trip behavior (empty-clears, corrupt-cache-entry, etc.) is
// covered by internal/session's own tests; the tests below exercise the
// viewer's integration with a saved session (restoreLink visibility,
// restoreSession, and reset's interaction with an unconsumed session) which
// can't move since they depend on *viewer/buildViewer.

// --- restoreLink visibility on launch ---------------------------------------

func TestBuildViewer_NoSavedSessionHidesRestoreLink(t *testing.T) {
	app := test.NewApp()

	v, win := buildViewer(app)
	defer win.Close()

	if v.restoreLink.Visible() {
		t.Error("restoreLink should be hidden when there is nothing to restore")
	}
}

func TestBuildViewer_SavedSessionShowsRestoreLink(t *testing.T) {
	app := test.NewApp()
	session.Save(app, []fyne.URI{
		storage.NewFileURI("/tmp/a.jpg"),
		storage.NewFileURI("/tmp/b.jpg"),
	})

	v, win := buildViewer(app)
	defer win.Close()

	if !v.restoreLink.Visible() {
		t.Fatal("restoreLink should be visible when a saved session exists")
	}
	if !strings.Contains(v.restoreLink.Text, "2") {
		t.Errorf("restoreLink.Text = %q, want it to mention the file count", v.restoreLink.Text)
	}
	if len(v.savedSession) != 2 {
		t.Errorf("len(v.savedSession) = %d, want 2", len(v.savedSession))
	}
}

// --- restoreSession ----------------------------------------------------------

func TestRestoreSession_LoadsSavedFilesAndHidesLink(t *testing.T) {
	app := test.NewApp()

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	session.Save(app, []fyne.URI{a, b})

	v, win := buildViewer(app)
	defer win.Close()

	if !v.restoreLink.Visible() {
		t.Fatal("restoreLink should start visible with a saved session present")
	}

	v.restoreSession()
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if len(v.state.files) != 2 {
		t.Fatalf("len(v.state.files) = %d, want 2", len(v.state.files))
	}
	if v.restoreLink.Visible() {
		t.Error("restoreLink should hide once the saved session has been restored")
	}
	if v.savedSession != nil {
		t.Error("savedSession should be consumed (nil) after restoreSession")
	}
}

// --- interaction with reset/handleDrop --------------------------------------

func TestHandleDrop_HidesRestoreLinkEvenWithoutUsingIt(t *testing.T) {
	app := test.NewApp()

	saved := uitest.TempJPEGURI(t, "saved.jpg", 4, 4, color.White)
	session.Save(app, []fyne.URI{saved})

	v, win := buildViewer(app)
	defer win.Close()

	if !v.restoreLink.Visible() {
		t.Fatal("restoreLink should start visible")
	}

	dropped := uitest.TempJPEGURI(t, "dropped.jpg", 4, 4, color.White)
	dropAndWait(t, v, dropped)

	if v.restoreLink.Visible() {
		t.Error("restoreLink should hide once the user drops files directly, ignoring the offer")
	}
	// The offer itself (v.savedSession) is untouched by an ordinary drop -
	// only restoreSession consumes it - so a later reset can still surface
	// it again; see TestViewerReset_ReshowsRestoreLinkWhenSessionUnconsumed.
	if len(v.savedSession) != 1 {
		t.Errorf("len(v.savedSession) = %d, want 1 (untouched by a plain drop)", len(v.savedSession))
	}
}

func TestViewerReset_ReshowsRestoreLinkWhenSessionUnconsumed(t *testing.T) {
	app := test.NewApp()

	saved := uitest.TempJPEGURI(t, "saved.jpg", 4, 4, color.White)
	session.Save(app, []fyne.URI{saved})

	v, win := buildViewer(app)
	defer win.Close()

	dropped := uitest.TempJPEGURI(t, "dropped.jpg", 4, 4, color.White)
	dropAndWait(t, v, dropped)

	v.reset()

	if !v.restoreLink.Visible() {
		t.Error("reset should re-offer a still-unconsumed saved session")
	}
}

func TestViewerReset_DoesNotReshowRestoreLinkOnceConsumed(t *testing.T) {
	app := test.NewApp()

	saved := uitest.TempJPEGURI(t, "saved.jpg", 4, 4, color.White)
	session.Save(app, []fyne.URI{saved})

	v, win := buildViewer(app)
	defer win.Close()

	v.restoreSession()
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	v.reset()

	if v.restoreLink.Visible() {
		t.Error("reset should not re-offer a session that was already restored")
	}
}
