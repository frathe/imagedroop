//go:build !darwin

package trash

import "errors"

// moveDarwin's real implementation (darwin.go) is cgo/AppKit and only
// compiles on macOS. Move's GOOS switch makes this stub unreachable; it
// exists so the package still compiles everywhere else.
func moveDarwin(string) error {
	return errors.New("the macOS trash mover only exists in darwin builds")
}
