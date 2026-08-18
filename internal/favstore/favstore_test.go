package favstore

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/uitest"
)

func TestDefaultDirUsesUserConfigDirectory(t *testing.T) {
	t.Parallel()

	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	want := filepath.Join(base, "picfetch", "favorites")
	if got := DefaultDir(); got != want {
		t.Errorf("DefaultDir() = %q, want %q", got, want)
	}
}

func TestValidName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: "", want: false},
		{name: ".", want: false},
		{name: "..", want: false},
		{name: "Summer 2026", want: true},
		{name: "Trip.2026", want: true},
		{name: " leading space", want: true},
		{name: "trailing space ", want: true},
		{name: "a/b", want: false},
		{name: `a\b`, want: false},
		{name: "a:b", want: false},
		{name: "a*b", want: false},
		{name: "a?b", want: false},
		{name: `a"b`, want: false},
		{name: "a<b", want: false},
		{name: "a>b", want: false},
		{name: "a|b", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ValidName(tt.name); got != tt.want {
				t.Errorf("ValidName(%q) = %v, want %v", tt.name, got, tt.want)
			}
		})
	}
}

func TestSaveLoadRoundTripPreservesNumericOrder(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	files := make([]fyne.URI, 12)
	for i := range files {
		files[i] = storage.NewFileURI(filepath.Join(dir, "images", string(rune('a'+i))+".jpg"))
	}

	if err := Save(dir, "Trip", files); err != nil {
		t.Fatalf("Save: %v", err)
	}

	got, err := Load(dir, "Trip")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	gotPaths := make([]string, len(got))
	wantPaths := make([]string, len(files))
	for i := range got {
		gotPaths[i] = got[i].Path()
		wantPaths[i] = files[i].Path()
	}
	if !slices.Equal(gotPaths, wantPaths) {
		t.Errorf("Load paths = %v, want %v", gotPaths, wantPaths)
	}

	data, err := os.ReadFile(filepath.Join(dir, "Trip", fileListName))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var stored map[string]string
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if stored["10"] != files[10].Path() {
		t.Errorf("stored[10] = %q, want %q", stored["10"], files[10].Path())
	}
}

func TestSaveOverwritesExistingFavorite(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	first := []fyne.URI{storage.NewFileURI("/first.jpg")}
	second := []fyne.URI{storage.NewFileURI("/second.jpg")}

	if err := Save(dir, "Set", first); err != nil {
		t.Fatalf("first Save: %v", err)
	}
	if err := Save(dir, "Set", second); err != nil {
		t.Fatalf("second Save: %v", err)
	}
	got, err := Load(dir, "Set")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got) != 1 || got[0].Path() != "/second.jpg" {
		t.Errorf("Load = %v, want only /second.jpg", got)
	}
}

func TestSaveRejectsInvalidNameAndNilURI(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if err := Save(dir, "../escape", nil); err == nil {
		t.Error("Save accepted an invalid name")
	}
	if err := Save(dir, "Nil", []fyne.URI{nil}); err == nil {
		t.Error("Save accepted a nil URI")
	}
	if _, err := os.Stat(filepath.Join(dir, "Nil")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Save left a directory behind for invalid input: %v", err)
	}
}

func TestLoadRejectsMalformedData(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		data string
	}{
		{name: "invalid JSON", data: "{"},
		{name: "non-numeric index", data: `{"first":"/a.jpg"}`},
		{name: "negative index", data: `{"-1":"/a.jpg"}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			favoriteDir := filepath.Join(dir, "Broken")
			if err := os.Mkdir(favoriteDir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(favoriteDir, fileListName), []byte(tt.data), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Load(dir, "Broken"); err == nil {
				t.Error("Load accepted malformed data")
			}
		})
	}
}

func TestList(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for _, name := range []string{"zebra", "Alpha", "beta"} {
		if err := Save(dir, name, nil); err != nil {
			t.Fatalf("Save %q: %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "not-a-favorite"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "plain-file"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := List(dir)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []string{"Alpha", "beta", "zebra"}
	if !slices.Equal(got, want) {
		t.Errorf("List = %v, want %v", got, want)
	}
}

func TestListMissingDirectoryIsEmpty(t *testing.T) {
	t.Parallel()

	got, err := List(filepath.Join(t.TempDir(), "missing"))
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if got != nil {
		t.Errorf("List = %v, want nil", got)
	}
}

func TestExists(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	if Exists(dir, "Set") {
		t.Fatal("Exists reported a missing favorite")
	}
	if err := Save(dir, "Set", nil); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir, "Set") {
		t.Error("Exists did not report a saved favorite")
	}
	if Exists(dir, "../Set") {
		t.Error("Exists accepted an invalid name")
	}
}

func TestRemoveMovesFavoriteDirectoryToTrash(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, "Set", nil); err != nil {
		t.Fatal(err)
	}

	var moved string
	uitest.StubTrashMove(t, func(path string) error {
		moved = path
		return os.RemoveAll(path)
	})
	if err := Remove(dir, "Set"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	want := filepath.Join(dir, "Set")
	if moved != want {
		t.Errorf("trash path = %q, want %q", moved, want)
	}
	if _, err := os.Stat(want); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("removed favorite still exists: %v", err)
	}
}

func TestRemoveRejectsInvalidNameAndPropagatesError(t *testing.T) {
	if err := Remove(t.TempDir(), "../escape"); err == nil {
		t.Error("Remove accepted an invalid name")
	}

	wantErr := errors.New("trash failed")
	uitest.StubTrashMove(t, func(string) error { return wantErr })
	if err := Remove(t.TempDir(), "Set"); !errors.Is(err, wantErr) {
		t.Errorf("Remove error = %v, want %v", err, wantErr)
	}
}
