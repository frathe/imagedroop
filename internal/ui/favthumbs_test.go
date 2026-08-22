package ui

import (
	"errors"
	"image/color"
	"os"
	"slices"
	"testing"

	"fyne.io/fyne/v2"

	"github.com/frathe/picfetch/internal/favstore"
	"github.com/frathe/picfetch/internal/favthumbs"
	"github.com/frathe/picfetch/internal/uitest"
)

// settleFavoritePreviews waits out the background pass SyncFavoritePreviews
// runs on - favThumb is finished once that pass has fully completed, sweep
// included, so reading the preview directory afterwards is race-free. The
// same discipline settleWallpaper gives the wallpaper goroutine.
func settleFavoritePreviews(t *testing.T, v *viewer) {
	t.Helper()

	if !v.favThumb.Begun() {
		t.Fatal("no favorite-preview pass pending to settle")
	}

	waitFor(t, "the favorite-preview pass", &v.favThumb)
}

// storeFavorite writes a favorite holding files into a fresh temporary
// storage directory, points the viewer's Favorites menu at it, and returns
// that favorite's own directory - the one previews land under.
func storeFavorite(t *testing.T, v *viewer, name string, files ...fyne.URI) string {
	t.Helper()

	dir := t.TempDir()
	if err := favstore.Save(dir, name, files); err != nil {
		t.Fatalf("favstore.Save: %v", err)
	}
	v.favorites.SetDir(dir)

	return favstore.Dir(dir, name)
}

// previewNames lists the preview files currently stored for a favorite.
func previewNames(t *testing.T, favDir string) []string {
	t.Helper()

	entries, err := os.ReadDir(favthumbs.Dir(favDir))
	if err != nil {
		t.Fatalf("read the preview directory: %v", err)
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestSyncFavoritePreviews_OpeningAFavoriteWritesPreviews(t *testing.T) {
	v := newTestViewer(t)
	first := uitest.TempJPEGURI(t, "one.jpg", 64, 48, color.White)
	second := uitest.TempJPEGURI(t, "two.jpg", 32, 32, color.Black)
	favDir := storeFavorite(t, v, "Trip", first, second)

	v.favorites.Menu().Items[2].Action()
	settleFavoritePreviews(t, v)
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if got := previewNames(t, favDir); len(got) != 2 {
		t.Errorf("preview files = %v, want one per favorited file", got)
	}
}

func TestSyncFavoritePreviews_PreferenceOffWritesNothing(t *testing.T) {
	v := newTestViewer(t)
	v.SetFavoritePreviewCache(false)
	image := uitest.TempJPEGURI(t, "one.jpg", 64, 48, color.White)
	favDir := storeFavorite(t, v, "Trip", image)

	v.favorites.Menu().Items[2].Action()
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if v.favThumb.Begun() {
		t.Error("a preview pass was started with the preference off")
	}
	if _, err := os.Stat(favthumbs.Dir(favDir)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("stat the preview directory = %v, want it never created", err)
	}
}

// TestSyncFavoritePreviews_SecondOpenAddsNothing is the point of persisting
// previews at all: the second open finds every one of them current and
// neither re-encodes nor accumulates a duplicate beside it.
func TestSyncFavoritePreviews_SecondOpenAddsNothing(t *testing.T) {
	v := newTestViewer(t)
	first := uitest.TempJPEGURI(t, "one.jpg", 64, 48, color.White)
	second := uitest.TempJPEGURI(t, "two.jpg", 32, 32, color.Black)
	favDir := storeFavorite(t, v, "Trip", first, second)

	v.favorites.Menu().Items[2].Action()
	settleFavoritePreviews(t, v)
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)
	before := previewNames(t, favDir)

	v.favorites.Menu().Items[2].Action()
	settleFavoritePreviews(t, v)
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if after := previewNames(t, favDir); !slices.Equal(before, after) {
		t.Errorf("preview files = %v after a second open, want the first pass's %v", after, before)
	}
}

// TestSyncFavoritePreviews_WarmsTheGridThumbnailCache is the half of the
// feature the user actually sees: the pass does not only fill a directory,
// it hands what it produced to the grid's in-memory cache, so the overview
// paints from it instead of decoding the originals again.
func TestSyncFavoritePreviews_WarmsTheGridThumbnailCache(t *testing.T) {
	v := newTestViewer(t)
	image := uitest.TempJPEGURI(t, "one.jpg", 64, 48, color.White)
	storeFavorite(t, v, "Trip", image)

	v.favorites.Menu().Items[2].Action()
	settleFavoritePreviews(t, v)
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if _, ok := v.grid.CachedThumb(image); !ok {
		t.Errorf("grid thumbnail cache has no entry for %q after a preview pass", image.Name())
	}
}

// TestSyncFavoritePreviews_EmptyListSweepsStalePreviews pins the one case
// an early "nothing to do" return would quietly break: a favorite the user
// re-saved as empty still has to lose the previews it used to own, and a
// pass over no files is exactly what deletes them.
func TestSyncFavoritePreviews_EmptyListSweepsStalePreviews(t *testing.T) {
	v := newTestViewer(t)
	image := uitest.TempJPEGURI(t, "one.jpg", 64, 48, color.White)
	favDir := storeFavorite(t, v, "Trip", image)

	v.SyncFavoritePreviews(favDir, []fyne.URI{image})
	settleFavoritePreviews(t, v)
	if got := previewNames(t, favDir); len(got) != 1 {
		t.Fatalf("preview files = %v, want one for the single favorited file", got)
	}

	v.SyncFavoritePreviews(favDir, nil)
	settleFavoritePreviews(t, v)

	if got := previewNames(t, favDir); len(got) != 0 {
		t.Errorf("preview files = %v, want them swept for an emptied favorite", got)
	}
}

// TestSetFavoritePreviewCacheOffCancelsAnInFlightPass covers the gap between
// what the checkbox says and what it does. Turning it off only stopped
// *future* passes: a pass already walking a large favorite kept decoding and
// kept writing preview files to disk, for as long as it took to finish,
// after the user had explicitly asked for exactly that to stop.
func TestSetFavoritePreviewCacheOffCancelsAnInFlightPass(t *testing.T) {
	v := newTestViewer(t)

	token := v.favThumbLifecycle.begin()
	if !token.current() {
		t.Fatal("a freshly begun token should be current")
	}

	v.SetFavoritePreviewCache(false)

	if token.current() {
		t.Error("turning the preference off should supersede the pass running under it")
	}
	if token.context().Err() == nil {
		t.Error("turning the preference off should cancel the running pass's context")
	}
}

// TestSetFavoritePreviewCacheOnLeavesAPassAlone is the other half: only
// switching the preference off is a reason to abandon work in flight.
func TestSetFavoritePreviewCacheOnLeavesAPassAlone(t *testing.T) {
	v := newTestViewer(t)

	token := v.favThumbLifecycle.begin()
	v.SetFavoritePreviewCache(true)

	if !token.current() {
		t.Error("turning the preference on should not disturb a pass already running")
	}
}
