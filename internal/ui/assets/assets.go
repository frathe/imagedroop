// Package assets holds the images the UI embeds into the binary.
//
// They live here, beside the package that draws them, because //go:embed
// cannot reference a parent directory - a file can only be embedded by a
// package at or below its own directory. The app's other artwork stays at
// the module root's assets/ directory instead, since it's consumed by the
// build rather than the program: appIcon.png by the Makefile/FyneApp.toml
// packaging step, header.png by the README.
package assets

import _ "embed"

// WelcomeWebP is the artwork shown beside the drop zone on first launch,
// and again in the About window.
//
//go:embed welcome.webp
var WelcomeWebP []byte

// PlaceholderWebP replaces it once an error has left the drop zone empty.
//
//go:embed placeholder.webp
var PlaceholderWebP []byte
