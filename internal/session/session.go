// Package session persists and restores the set of files that were open
// when the window last closed, via Fyne's app-scoped cache.
package session

import (
	"encoding/json"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"
)

// cacheKey names the cache entry Save/Load read and write via app.Cache() -
// the same mechanism Fyne uses for any other app-scoped cache blob, keyed by
// name rather than a raw file path so it works the same under the test
// driver's in-memory cache as it does under the real on-disk one.
const cacheKey = "session.json"

// state is the on-disk representation of the file set that was loaded when
// the window last closed. A struct rather than a bare string slice so the
// format can grow a field later without breaking decode of what's already
// on disk.
type state struct {
	Files []string `json:"files"`
}

// Save records files as the session offered on the next launch. An empty
// files removes any previously saved session instead of writing one, so
// quitting after Escape (reset) - a deliberate "start fresh" - doesn't leave
// a stale offer for a session the user already walked away from.
func Save(app fyne.App, files []fyne.URI) {
	cache := app.Cache()

	if len(files) == 0 {
		_ = cache.Remove(cacheKey)
		return
	}

	uris := make([]string, len(files))
	for i, u := range files {
		uris[i] = u.String()
	}

	w, err := cache.Write(cacheKey)
	if err != nil {
		fyne.LogError("failed to save session", err)
		return
	}
	defer w.Close()

	if err := json.NewEncoder(w).Encode(state{Files: uris}); err != nil {
		fyne.LogError("failed to save session", err)
	}
}

// Load returns the file set saved by the previous run's Save call, or nil
// if there is none - first launch, a cleared session, or a corrupt cache
// entry are all treated the same as "nothing to restore" rather than
// surfaced as an error, since there's no user action to report one to yet.
func Load(app fyne.App) []fyne.URI {
	cache := app.Cache()
	if !cache.Exists(cacheKey) {
		return nil
	}

	r, err := cache.Read(cacheKey)
	if err != nil {
		return nil
	}
	defer r.Close()

	var s state
	if err := json.NewDecoder(r).Decode(&s); err != nil {
		return nil
	}

	uris := make([]fyne.URI, 0, len(s.Files))
	for _, u := range s.Files {
		if parsed, err := storage.ParseURI(u); err == nil {
			uris = append(uris, parsed)
		}
	}
	return uris
}
