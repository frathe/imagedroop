package favorites

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/storage"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/picfetch/internal/favstore"
	"github.com/frathe/picfetch/internal/uitest"
)

type fakeHost struct {
	files  []fyne.URI
	opened []fyne.URI
	toasts []string
}

func (h *fakeHost) FileCount() int        { return len(h.files) }
func (h *fakeHost) FileAt(i int) fyne.URI { return h.files[i] }
func (h *fakeHost) OpenFiles(files []fyne.URI) {
	h.opened = slices.Clone(files)
}
func (h *fakeHost) ShowToast(message string) { h.toasts = append(h.toasts, message) }

func newFeature(t *testing.T, host *fakeHost) *Feature {
	t.Helper()

	app := test.NewApp()
	t.Cleanup(app.Quit)
	win := app.NewWindow("favorites test")
	t.Cleanup(win.Close)

	f := New(host, win)
	f.SetDir(t.TempDir())
	return f
}

func TestNewBuildsStaticMenuWithoutDiskAccess(t *testing.T) {
	host := &fakeHost{}
	app := test.NewApp()
	t.Cleanup(app.Quit)
	win := app.NewWindow("favorites test")
	t.Cleanup(win.Close)

	f := New(host, win)

	if f.dir != "" {
		t.Errorf("New set storage dir to %q, want no disk initialization", f.dir)
	}
	if f.menu.Label != "Favorites" {
		t.Errorf("menu label = %q, want Favorites", f.menu.Label)
	}
	if len(f.menu.Items) != 3 || f.menu.Items[0] != f.addItem ||
		!f.menu.Items[1].IsSeparator || f.menu.Items[2] != f.manageItem {
		t.Errorf("static menu items = %+v", f.menu.Items)
	}
	if !f.addItem.Disabled {
		t.Error("Add should start disabled")
	}
}

func TestSetDirBuildsSortedFavoriteItems(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	for _, name := range []string{"zebra", "Alpha", "beta"} {
		if err := favstore.Save(f.dir, name, nil); err != nil {
			t.Fatal(err)
		}
	}

	f.SetDir(f.dir)

	if len(f.menu.Items) != 7 {
		t.Fatalf("menu item count = %d, want 7", len(f.menu.Items))
	}
	got := []string{f.menu.Items[2].Label, f.menu.Items[3].Label, f.menu.Items[4].Label}
	want := []string{"Alpha", "beta", "zebra"}
	if !slices.Equal(got, want) {
		t.Errorf("favorite items = %v, want %v", got, want)
	}
	if !f.menu.Items[1].IsSeparator || !f.menu.Items[5].IsSeparator {
		t.Error("dynamic entries should be enclosed by separators")
	}
}

func TestSetDirAssignsDigitShortcutsToFirstTenFavorites(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	for i := 0; i < ShortcutCount+1; i++ {
		name := fmt.Sprintf("Favorite %02d", i+1)
		if err := favstore.Save(f.dir, name, nil); err != nil {
			t.Fatal(err)
		}
	}

	f.SetDir(f.dir)

	wantKeys := []fyne.KeyName{
		fyne.Key1,
		fyne.Key2,
		fyne.Key3,
		fyne.Key4,
		fyne.Key5,
		fyne.Key6,
		fyne.Key7,
		fyne.Key8,
		fyne.Key9,
		fyne.Key0,
	}
	for i := 0; i < ShortcutCount+1; i++ {
		item := f.menu.Items[i+2]
		if i == ShortcutCount {
			if item.Shortcut != nil {
				t.Errorf("favorite 11 shortcut = %v, want nil", item.Shortcut)
			}
			continue
		}

		got, ok := item.Shortcut.(*desktop.CustomShortcut)
		if !ok {
			t.Fatalf("favorite %d shortcut type = %T, want *desktop.CustomShortcut", i+1, item.Shortcut)
		}
		if got.KeyName != wantKeys[i] || got.Modifier != fyne.KeyModifierShortcutDefault {
			t.Errorf("favorite %d shortcut = %+v, want key %s with default modifier",
				i+1, got, wantKeys[i])
		}
	}
	if ShortcutForIndex(-1) != nil || ShortcutForIndex(ShortcutCount) != nil {
		t.Error("ShortcutForIndex should return nil outside the ten favorite slots")
	}
}

func TestOpenUsesCurrentSortedShortcutSlots(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	for i := 0; i < ShortcutCount+1; i++ {
		name := fmt.Sprintf("Favorite %02d", i+1)
		files := []fyne.URI{storage.NewFileURI(fmt.Sprintf("/photos/%02d.jpg", i+1))}
		if err := favstore.Save(f.dir, name, files); err != nil {
			t.Fatal(err)
		}
	}
	f.SetDir(f.dir)

	for i := 0; i < ShortcutCount; i++ {
		f.Open(i)
		want := fmt.Sprintf("/photos/%02d.jpg", i+1)
		if len(host.opened) != 1 || host.opened[0].Path() != want {
			t.Errorf("Open(%d) opened %v, want %q", i, host.opened, want)
		}
	}

	if err := favstore.Save(f.dir, "A Favorite", []fyne.URI{storage.NewFileURI("/photos/new-first.jpg")}); err != nil {
		t.Fatal(err)
	}
	f.SetDir(f.dir)
	f.Open(0)
	if len(host.opened) != 1 || host.opened[0].Path() != "/photos/new-first.jpg" {
		t.Errorf("Open(0) after refresh opened %v, want the newly sorted first favorite", host.opened)
	}

	host.opened = nil
	f.Open(-1)
	f.Open(ShortcutCount)
	if host.opened != nil {
		t.Errorf("out-of-range shortcut opened %v", host.opened)
	}
}

func TestSetHasFilesTogglesAddItem(t *testing.T) {
	f := newFeature(t, &fakeHost{})

	f.SetHasFiles(true)
	if f.addItem.Disabled {
		t.Error("Add should be enabled with files")
	}
	f.SetHasFiles(false)
	if !f.addItem.Disabled {
		t.Error("Add should be disabled without files")
	}
}

func TestWriteFavoriteSavesCurrentListAndRefreshesMenu(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{
		storage.NewFileURI("/photos/a.jpg"),
		storage.NewFileURI("/photos/b.jpg"),
	}}
	f := newFeature(t, host)

	f.writeFavorite("Trip")

	got, err := favstore.Load(f.dir, "Trip")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 2 || got[0].Path() != "/photos/a.jpg" || got[1].Path() != "/photos/b.jpg" {
		t.Errorf("stored files = %v", got)
	}
	if len(f.menu.Items) != 5 || f.menu.Items[2].Label != "Trip" {
		t.Errorf("menu not refreshed after save: %+v", f.menu.Items)
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "Trip") {
		t.Errorf("toasts = %v, want saved Trip", host.toasts)
	}
}

func TestAddDialogSubmitsValidatedName(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{storage.NewFileURI("/photos/a.jpg")}}
	f := newFeature(t, host)
	form, entry := f.newAddDialog()
	form.Show()

	entry.SetText("  Trip  ")
	form.Submit()

	if !favstore.Exists(f.dir, "Trip") {
		t.Error("submitting the add dialog did not save the trimmed favorite name")
	}
}

func TestAddDialogRejectsInvalidName(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{storage.NewFileURI("/photos/a.jpg")}}
	f := newFeature(t, host)
	form, entry := f.newAddDialog()
	form.Show()

	entry.SetText("../escape")
	form.Submit()
	form.Dismiss()

	if favstore.Exists(f.dir, "../escape") {
		t.Error("submitting the add dialog accepted an invalid favorite name")
	}
	if len(host.toasts) != 0 {
		t.Errorf("disabled form submission unexpectedly ran its callback: %v", host.toasts)
	}
}

func TestWriteFavoriteRejectsEmptyCurrentList(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)

	f.writeFavorite("Empty")

	if favstore.Exists(f.dir, "Empty") {
		t.Error("empty current list was saved")
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "no open files") {
		t.Errorf("toasts = %v", host.toasts)
	}
}

func TestSaveFavoriteRejectsInvalidName(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{storage.NewFileURI("/a.jpg")}}
	f := newFeature(t, host)

	f.saveFavorite("../escape")

	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "enter a name") {
		t.Errorf("toasts = %v", host.toasts)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(f.dir), "escape")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("invalid favorite escaped storage dir: %v", err)
	}
}

func TestOpenFavoriteLoadsStoredList(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	files := []fyne.URI{
		storage.NewFileURI("/photos/one.jpg"),
		storage.NewFileURI("/photos/two.jpg"),
	}
	if err := favstore.Save(f.dir, "Trip", files); err != nil {
		t.Fatal(err)
	}

	f.openFavorite("Trip")

	if len(host.opened) != 2 || host.opened[0].Path() != files[0].Path() ||
		host.opened[1].Path() != files[1].Path() {
		t.Errorf("opened = %v, want %v", host.opened, files)
	}
}

func TestOpenFavoriteReportsLoadError(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)

	f.openFavorite("Missing")

	if host.opened != nil {
		t.Errorf("opened = %v, want nil", host.opened)
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "Missing") {
		t.Errorf("toasts = %v", host.toasts)
	}
}

func TestPerformRemoveTrashesDirectoryAndRefreshesMenu(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	if err := favstore.Save(f.dir, "Trip", nil); err != nil {
		t.Fatal(err)
	}
	f.SetDir(f.dir)
	uitest.StubTrashMove(t, func(path string) error { return os.RemoveAll(path) })

	f.performRemove("Trip")
	f.pending.Wait()

	if favstore.Exists(f.dir, "Trip") {
		t.Error("favorite still exists after removal")
	}
	if len(f.menu.Items) != 3 {
		t.Errorf("menu item count = %d, want static 3 after removal", len(f.menu.Items))
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "Trip") {
		t.Errorf("toasts = %v", host.toasts)
	}
}

func TestPerformRemoveReportsTrashError(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	if err := favstore.Save(f.dir, "Trip", nil); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("trash unavailable")
	uitest.StubTrashMove(t, func(string) error { return wantErr })

	f.performRemove("Trip")
	f.pending.Wait()

	if !favstore.Exists(f.dir, "Trip") {
		t.Error("favorite disappeared after failed removal")
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], wantErr.Error()) {
		t.Errorf("toasts = %v", host.toasts)
	}
}

func TestShowManageBuildsEmptyAndPopulatedDialogs(t *testing.T) {
	f := newFeature(t, &fakeHost{})

	f.showManage()
	if f.manageDialog == nil {
		t.Fatal("showManage did not build an empty dialog")
	}
	f.manageDialog.Hide()

	if err := favstore.Save(f.dir, "Trip", nil); err != nil {
		t.Fatal(err)
	}
	f.showManage()
	if f.manageDialog == nil {
		t.Fatal("showManage did not build a populated dialog")
	}
	f.manageDialog.Hide()
}
