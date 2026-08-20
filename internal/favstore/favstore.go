// Package favstore persists named lists of image files.
package favstore

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/trash"
)

const fileListName = "file-list.json"

// DefaultDir returns the directory used for favorites in production.
func DefaultDir() string {
	base, err := os.UserConfigDir()
	if err != nil || base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "picfetch", "favorites")
}

// ValidName reports whether name is safe to use as one directory component.
func ValidName(name string) bool {
	return name != "" &&
		name != "." &&
		name != ".." &&
		!strings.ContainsAny(name, `/\:*?"<>|`)
}

// Dir returns the directory holding the favorite named name, or "" when
// name is not a valid favorite name.
func Dir(dir, name string) string {
	if !ValidName(name) {
		return ""
	}
	return filepath.Join(dir, name)
}

// Exists reports whether a favorite with name exists.
func Exists(dir, name string) bool {
	if !ValidName(name) {
		return false
	}
	info, err := os.Stat(filepath.Join(dir, name, fileListName))
	return err == nil && !info.IsDir()
}

// List returns favorite names sorted case-insensitively.
func List(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !ValidName(entry.Name()) {
			continue
		}
		info, err := os.Stat(filepath.Join(dir, entry.Name(), fileListName))
		if err == nil && !info.IsDir() {
			names = append(names, entry.Name())
			continue
		}
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}

	sort.Slice(names, func(i, j int) bool {
		left, right := strings.ToLower(names[i]), strings.ToLower(names[j])
		if left == right {
			return names[i] < names[j]
		}
		return left < right
	})
	return names, nil
}

// Save atomically writes files as the favorite named name.
func Save(dir, name string, files []fyne.URI) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid favorite name %q", name)
	}

	list := make(map[string]string, len(files))
	for i, file := range files {
		if file == nil {
			return fmt.Errorf("favorite file %d is nil", i)
		}
		list[strconv.Itoa(i)] = file.Path()
	}

	favoriteDir := filepath.Join(dir, name)
	if err := os.MkdirAll(favoriteDir, 0o755); err != nil {
		return err
	}

	tmp, err := os.CreateTemp(favoriteDir, ".file-list-*.json")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := json.NewEncoder(tmp).Encode(list); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, filepath.Join(favoriteDir, fileListName))
}

// readList reads and decodes the favorite named name's file-list.json into
// its raw index-to-path map, rejecting only an invalid name or data that
// isn't valid JSON. It does not validate that each key is a well-formed
// index - Load and Count each decide for themselves what to do with a key
// that isn't, since they disagree about whether they need to know the index
// at all. Load and Count both build on this so they never disagree about
// what is actually stored on disk.
func readList(dir, name string) (map[string]string, error) {
	if !ValidName(name) {
		return nil, fmt.Errorf("invalid favorite name %q", name)
	}

	data, err := os.ReadFile(filepath.Join(dir, name, fileListName))
	if err != nil {
		return nil, err
	}

	var list map[string]string
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// Load returns the files stored in the favorite named name.
func Load(dir, name string) ([]fyne.URI, error) {
	list, err := readList(dir, name)
	if err != nil {
		return nil, err
	}

	type indexedPath struct {
		index int
		path  string
	}
	paths := make([]indexedPath, 0, len(list))
	for key, path := range list {
		index, err := strconv.Atoi(key)
		if err != nil || index < 0 {
			return nil, fmt.Errorf("invalid file index %q", key)
		}
		paths = append(paths, indexedPath{index: index, path: path})
	}
	sort.Slice(paths, func(i, j int) bool { return paths[i].index < paths[j].index })

	files := make([]fyne.URI, len(paths))
	for i, item := range paths {
		files[i] = storage.NewFileURI(item.path)
	}
	return files, nil
}

// Count returns how many files the favorite named name stores.
//
// It validates each key exactly as Load does rather than just returning
// len(list): a hand-edited file-list.json with a non-numeric or negative
// key is not a valid stored list, and Count reporting a count for it anyway
// would let a menu label claim a number Load then refuses to open - the two
// must agree on what counts as a stored list, not just on how many keys are
// present. A missing favorite, an unreadable file, and malformed JSON are
// likewise errors here, never 0 - 0 is what a favorite saved with no files
// legitimately reports, and callers need to tell the two apart.
func Count(dir, name string) (int, error) {
	list, err := readList(dir, name)
	if err != nil {
		return 0, err
	}

	for key := range list {
		if index, err := strconv.Atoi(key); err != nil || index < 0 {
			return 0, fmt.Errorf("invalid file index %q", key)
		}
	}
	return len(list), nil
}

// Remove moves the favorite named name to the operating system's trash.
func Remove(dir, name string) error {
	if !ValidName(name) {
		return fmt.Errorf("invalid favorite name %q", name)
	}
	return trash.Move(filepath.Join(dir, name))
}
