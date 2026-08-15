package session

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"
)

func TestLoadSession_NothingSavedReturnsNil(t *testing.T) {
	app := test.NewApp()

	if got := Load(app); got != nil {
		t.Errorf("Load() = %v, want nil", got)
	}
}

func TestSaveSession_RoundTrip(t *testing.T) {
	app := test.NewApp()

	want := []fyne.URI{
		storage.NewFileURI("/tmp/a.jpg"),
		storage.NewFileURI("/tmp/b.png"),
	}
	Save(app, want)

	got := Load(app)
	if len(got) != len(want) {
		t.Fatalf("Load() returned %d URIs, want %d", len(got), len(want))
	}
	for i, u := range got {
		if u.String() != want[i].String() {
			t.Errorf("Load()[%d] = %q, want %q", i, u.String(), want[i].String())
		}
	}
}

func TestSaveSession_EmptyFilesClearsPreviouslySavedSession(t *testing.T) {
	app := test.NewApp()

	Save(app, []fyne.URI{storage.NewFileURI("/tmp/a.jpg")})
	if Load(app) == nil {
		t.Fatal("expected a saved session before clearing")
	}

	Save(app, nil)

	if got := Load(app); got != nil {
		t.Errorf("Load() after clearing = %v, want nil", got)
	}
	if app.Cache().Exists(cacheKey) {
		t.Error("cache entry should be removed, not left empty")
	}
}

func TestLoadSession_CorruptCacheEntryReturnsNil(t *testing.T) {
	app := test.NewApp()

	w, err := app.Cache().Write(cacheKey)
	if err != nil {
		t.Fatalf("Cache().Write: %v", err)
	}
	if _, err := w.Write([]byte("not valid json")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	w.Close()

	if got := Load(app); got != nil {
		t.Errorf("Load() = %v, want nil for corrupt cache entry", got)
	}
}
