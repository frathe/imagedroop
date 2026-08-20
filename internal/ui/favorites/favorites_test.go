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
)

type fakeHost struct {
	files  []fyne.URI
	opened []fyne.URI
	toasts []string

	// syncedDirs/syncedFiles record every SyncFavoritePreviews call, and
	// calls records the order OpenFiles and SyncFavoritePreviews arrived
	// in - the open path deliberately reports the new list before handing
	// it over, so the background pass starts against the scan rather than
	// after it.
	syncedDirs  []string
	syncedFiles [][]fyne.URI
	calls       []string
}

func (h *fakeHost) FileCount() int        { return len(h.files) }
func (h *fakeHost) FileAt(i int) fyne.URI { return h.files[i] }
func (h *fakeHost) OpenFiles(files []fyne.URI) {
	h.opened = slices.Clone(files)
	h.calls = append(h.calls, "open")
}
func (h *fakeHost) ShowToast(message string) { h.toasts = append(h.toasts, message) }
func (h *fakeHost) SyncFavoritePreviews(favDir string, files []fyne.URI) {
	h.syncedDirs = append(h.syncedDirs, favDir)
	h.syncedFiles = append(h.syncedFiles, slices.Clone(files))
	h.calls = append(h.calls, "sync")
}

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

// TestNewSetsManageItemAccelerator covers the display-only *desktop.
// CustomShortcut New sets on f.manageItem, mirroring
// TestSetDirAssignsDigitShortcutsToFirstTenFavorites below - distinct from
// wireManageFavoritesShortcut (internal/ui/shortcuts.go), which is what
// actually binds Cmd/Ctrl+Shift+F; this only pins what the menu shows next
// to the item as a hint.
func TestNewSetsManageItemAccelerator(t *testing.T) {
	app := test.NewApp()
	t.Cleanup(app.Quit)
	win := app.NewWindow("favorites test")
	t.Cleanup(win.Close)

	f := New(&fakeHost{}, win)

	got, ok := f.manageItem.Shortcut.(*desktop.CustomShortcut)
	if !ok {
		t.Fatalf("manage item shortcut type = %T, want *desktop.CustomShortcut", f.manageItem.Shortcut)
	}
	if got.KeyName != fyne.KeyF || got.Modifier != fyne.KeyModifierShortcutDefault|fyne.KeyModifierShift {
		t.Errorf("manage item shortcut = %+v, want {KeyF, KeyModifierShortcutDefault|KeyModifierShift}", got)
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
	want := []string{"Alpha (0)", "beta (0)", "zebra (0)"}
	if !slices.Equal(got, want) {
		t.Errorf("favorite items = %v, want %v", got, want)
	}
	if !f.menu.Items[1].IsSeparator || !f.menu.Items[5].IsSeparator {
		t.Error("dynamic entries should be enclosed by separators")
	}
}

// TestRefreshMenuLabelsCarryStoredCounts pins the label format the Favorites
// menu commits to: name and stored count together, sourced from
// favstore.Count rather than len(files) at save time, so a favorite edited
// on disk between refreshes still reports what Load would actually return.
func TestRefreshMenuLabelsCarryStoredCounts(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	counts := map[string]int{"Alpha": 3, "beta": 0, "zebra": 12}
	for name, n := range counts {
		files := make([]fyne.URI, n)
		for i := range files {
			files[i] = storage.NewFileURI(fmt.Sprintf("/photos/%s/%02d.jpg", name, i))
		}
		if err := favstore.Save(f.dir, name, files); err != nil {
			t.Fatal(err)
		}
	}

	f.SetDir(f.dir)

	if len(f.names) != len(counts) {
		t.Fatalf("f.names = %v, want %d favorites", f.names, len(counts))
	}
	for i, name := range f.names {
		want := fmt.Sprintf("%s (%d)", name, counts[name])
		if got := f.menu.Items[i+2].Label; got != want {
			t.Errorf("favorite %q label = %q, want %q", name, got, want)
		}
	}
}

// TestRefreshMenuFallsBackToBareNameForUnreadableCount pins the fallback
// this stage adds: a favorite whose file-list.json can't be read still
// lists, in its accelerator slot, under its bare name. favstore.Count and
// favstore.Load share the exact same readList/index validation (see Stage
// 1), so a file broken enough to make Count fail also makes Load fail -
// the point of this test isn't that the click opens files (it can't), it's
// that the click still resolves to *this* favorite's name and reaches the
// host through it, proving the fallback label cost it a count, not its
// identity. A refresh must also not toast per unreadable favorite: that
// would bury the user in toasts on every SetDir.
func TestRefreshMenuFallsBackToBareNameForUnreadableCount(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	if err := favstore.Save(f.dir, "Broken", nil); err != nil {
		t.Fatal(err)
	}
	listPath := filepath.Join(f.dir, "Broken", "file-list.json")
	if err := os.WriteFile(listPath, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}

	f.SetDir(f.dir)

	if len(host.toasts) != 0 {
		t.Errorf("SetDir raised toasts for an unreadable count: %v, want none", host.toasts)
	}
	if len(f.menu.Items) != 5 {
		t.Fatalf("menu item count = %d, want 5 (add, sep, Broken, sep, manage)", len(f.menu.Items))
	}
	item := f.menu.Items[2]
	if item.Label != "Broken" {
		t.Errorf("label = %q, want bare name %q", item.Label, "Broken")
	}
	if item.Shortcut == nil {
		t.Error("Broken favorite lost its accelerator slot")
	}
	if !slices.Equal(f.names, []string{"Broken"}) {
		t.Fatalf("f.names = %v, want [Broken]", f.names)
	}

	item.Action()

	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "Broken") {
		t.Errorf("clicking the fallback item produced toasts = %v, want one naming %q", host.toasts, "Broken")
	}
	if host.opened != nil {
		t.Errorf("opened = %v, want nil: the corrupt list can't load either", host.opened)
	}
}

// TestOpenMapsDigitSlotsThroughNamesDespiteCountLabels guards the seam this
// stage must not disturb: f.names, not the menu item's text, is what Open
// resolves a Cmd/Ctrl+digit slot through. Favorites are saved with distinct
// counts so a label carrying the wrong count would still be an obviously
// wrong label, but Open must still land on the right favorite's files.
func TestOpenMapsDigitSlotsThroughNamesDespiteCountLabels(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)
	type saved struct {
		name  string
		count int
	}
	favs := []saved{{"Alpha", 3}, {"beta", 1}, {"zebra", 5}}
	for _, fv := range favs {
		files := make([]fyne.URI, fv.count)
		for i := range files {
			files[i] = storage.NewFileURI(fmt.Sprintf("/photos/%s/%02d.jpg", fv.name, i))
		}
		if err := favstore.Save(f.dir, fv.name, files); err != nil {
			t.Fatal(err)
		}
	}
	f.SetDir(f.dir)

	for i, name := range f.names {
		label := f.menu.Items[i+2].Label
		if !strings.HasPrefix(label, name+" (") {
			t.Fatalf("item %d label = %q, does not name %q", i, label, name)
		}

		f.Open(i)
		want := fmt.Sprintf("/photos/%s/00.jpg", name)
		if len(host.opened) == 0 || host.opened[0].Path() != want {
			t.Errorf("Open(%d) opened %v, want first file %q for %q", i, host.opened, want, name)
		}
	}
}

func TestSetDirAssignsDigitShortcutsToFirstTenFavorites(t *testing.T) {
	f := newFeature(t, &fakeHost{})
	for i := range ShortcutCount + 1 {
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
	for i := range ShortcutCount + 1 {
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
	for i := range ShortcutCount + 1 {
		name := fmt.Sprintf("Favorite %02d", i+1)
		files := []fyne.URI{storage.NewFileURI(fmt.Sprintf("/photos/%02d.jpg", i+1))}
		if err := favstore.Save(f.dir, name, files); err != nil {
			t.Fatal(err)
		}
	}
	f.SetDir(f.dir)

	for i := range ShortcutCount {
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
	if len(f.menu.Items) != 5 || f.menu.Items[2].Label != "Trip (2)" {
		t.Errorf("menu not refreshed after save: %+v", f.menu.Items)
	}
	if len(host.toasts) != 1 || !strings.Contains(host.toasts[0], "Trip") {
		t.Errorf("toasts = %v, want saved Trip", host.toasts)
	}
}

// TestWriteFavoriteSyncsPreviewsForSavedList is the save half of the same
// trigger: the list the user just captured is reported straight away, so
// its previews can be generated while the favorite sits unopened rather
// than on the open that eventually wants them.
func TestWriteFavoriteSyncsPreviewsForSavedList(t *testing.T) {
	host := &fakeHost{files: []fyne.URI{
		storage.NewFileURI("/photos/a.jpg"),
		storage.NewFileURI("/photos/b.jpg"),
	}}
	f := newFeature(t, host)

	f.writeFavorite("Trip")

	wantDir := favstore.Dir(f.dir, "Trip")
	if len(host.syncedDirs) != 1 || host.syncedDirs[0] != wantDir {
		t.Fatalf("synced dirs = %v, want [%q]", host.syncedDirs, wantDir)
	}
	if len(host.syncedFiles) != 1 || len(host.syncedFiles[0]) != 2 ||
		host.syncedFiles[0][0].Path() != "/photos/a.jpg" ||
		host.syncedFiles[0][1].Path() != "/photos/b.jpg" {
		t.Errorf("synced files = %v, want the two files just saved", host.syncedFiles)
	}
}

func TestWriteFavoriteDoesNotSyncPreviewsForAFailedSave(t *testing.T) {
	host := &fakeHost{}
	f := newFeature(t, host)

	f.writeFavorite("Empty")

	if len(host.syncedDirs) != 0 {
		t.Errorf("synced dirs = %v, want none when nothing was saved", host.syncedDirs)
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

// TestOpenFavoriteSyncsPreviewsForLoadedList pins the open half of the
// preview-cache trigger: the feature itself knows nothing about previews,
// it only reports which directory now holds which files, and internal/ui
// decides what that means. The report has to precede OpenFiles so the
// background pass gets a head start on the scan the open kicks off.
func TestOpenFavoriteSyncsPreviewsForLoadedList(t *testing.T) {
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

	wantDir := favstore.Dir(f.dir, "Trip")
	if len(host.syncedDirs) != 1 || host.syncedDirs[0] != wantDir {
		t.Fatalf("synced dirs = %v, want [%q]", host.syncedDirs, wantDir)
	}
	if len(host.syncedFiles) != 1 || len(host.syncedFiles[0]) != 2 ||
		host.syncedFiles[0][0].Path() != files[0].Path() ||
		host.syncedFiles[0][1].Path() != files[1].Path() {
		t.Errorf("synced files = %v, want %v", host.syncedFiles, files)
	}
	if want := []string{"sync", "open"}; !slices.Equal(host.calls, want) {
		t.Errorf("call order = %v, want %v", host.calls, want)
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
