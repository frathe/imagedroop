//go:build !darwin

package wallpaper

import "errors"

// setDarwin's real implementation (darwin.go) is cgo/AppKit and only
// compiles on macOS. Set's GOOS switch makes this stub unreachable; it
// exists so the package still compiles everywhere else.
func setDarwin(string) error {
	return errors.New("the macOS wallpaper setter only exists in darwin builds")
}
