package favthumbs

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"fyne.io/fyne/v2"
)

// Sweep deletes previews under favDir that no file in files maps to.
//
// A preview is kept when either its base name matches EntryName(f) for some
// f in files - the current version of a source that could be stat-ed - or
// its base name begins with pathHash(f)+"-" for some f whose EntryName
// reported false. That second case is the offline-volume guard: a source
// that cannot be stat-ed right now has an unknown current version, so every
// preview that could belong to it is retained rather than destroyed on the
// strength of a stat error that may only be temporary.
func Sweep(favDir string, files []fyne.URI) error {
	dir := Dir(favDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		// No thumbs directory means nothing to prune - not an error, since
		// a favorite that has never had a preview written is the common
		// case for a fresh save, not a fault.
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	expected := make(map[string]bool, len(files))
	var offlineHashes []string
	for _, f := range files {
		if name, ok := EntryName(f); ok {
			expected[name] = true
			continue
		}
		// f could not be stat-ed: an offline volume, removed media, or
		// similar transient state. Its current version is unknown, so
		// every preview that could belong to it is retained below rather
		// than treated as stale on the strength of a stat error that may
		// not even be permanent.
		if hash, ok := pathHash(f); ok {
			offlineHashes = append(offlineHashes, hash)
		}
	}

	var firstErr error
	for _, entry := range entries {
		// Candidates are only regular files: a subdirectory or symlink that
		// happens to carry a .jpg/.png name is not something Write ever
		// produced, so it is left alone rather than risk removing something
		// this package doesn't own.
		if !entry.Type().IsRegular() {
			continue
		}
		name := entry.Name()
		ext := filepath.Ext(name)
		if ext != ".jpg" && ext != ".png" {
			continue
		}
		base := strings.TrimSuffix(name, ext)
		if expected[base] || retainedByOfflineGuard(base, offlineHashes) {
			continue
		}

		// One file failing to remove (permissions, a concurrent delete)
		// should not stop the rest of the sweep from running; report the
		// first error once the whole directory has been walked.
		if err := os.Remove(filepath.Join(dir, name)); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// retainedByOfflineGuard reports whether base begins with one of hashes
// followed by a dash. Matching on the hash plus the dash, not a bare
// prefix, keeps one hash from accidentally prefix-matching a different
// hash that happens to extend it.
func retainedByOfflineGuard(base string, hashes []string) bool {
	for _, h := range hashes {
		if strings.HasPrefix(base, h+"-") {
			return true
		}
	}
	return false
}
