// Command imagedrop is a desktop image viewer: drop files or folders onto
// it (or open them from the file dialog) and page through them.
//
// This file is the whole of package main - app setup, translations, and
// the command-line arguments. Everything else lives in internal/ui; see
// ARCHITECTURE.md for the package map.
package main

import (
	"embed"
	"os"
	"path/filepath"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/imagedrop/internal/ui"
)

// translationsFS stays here rather than moving into internal/ui with the
// rest of the app: lang.AddTranslationsFS loads into fyne's process-wide
// bundle, which every lang.L call reads from wherever it lives, so this is
// app setup rather than UI code.
//
//go:embed translations/*.json
var translationsFS embed.FS

// argsToURIs converts command-line paths (os.Args[1:]) into file URIs
// handleDrop can ingest, so launching the binary with paths - the way a
// macOS file association or "Open With" launches it - works the same as a
// drag-and-drop. Relative paths are resolved against the current working
// directory; anything that fails to resolve is skipped rather than aborting
// the whole batch, since one bad argument shouldn't stop the rest from
// loading. Existence/format isn't checked here - handleDrop's own scan and
// attemptLoad's retry chain already handle a bad path gracefully, the same
// as a bad drag-drop.
func argsToURIs(args []string) []fyne.URI {
	uris := make([]fyne.URI, 0, len(args))
	for _, a := range args {
		abs, err := filepath.Abs(a)
		if err != nil {
			continue
		}
		uris = append(uris, storage.NewFileURI(abs))
	}
	return uris
}

func main() {
	application := app.NewWithID("image_drop")

	if err := lang.AddTranslationsFS(translationsFS, "translations"); err != nil {
		fyne.LogError("failed to load translations", err)
	}

	ui.Run(application, argsToURIs(os.Args[1:]))
}
