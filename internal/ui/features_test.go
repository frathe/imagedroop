package ui

import "testing"

func TestBuildViewer_RegistersAllEightFeatures(t *testing.T) {
	view := newTestViewer(t)

	features := []struct {
		name       string
		registered bool
	}{
		{name: "help", registered: view.help != nil},
		{name: "EXIF", registered: view.exif != nil},
		{name: "zoom", registered: view.zoom != nil},
		{name: "grid", registered: view.grid != nil},
		{name: "deletion", registered: view.deletion != nil},
		{name: "slideshow", registered: view.slides != nil},
		{name: "settings", registered: view.settings != nil},
		{name: "favorites", registered: view.favorites != nil},
	}

	for _, feature := range features {
		if !feature.registered {
			t.Errorf("%s feature was not registered", feature.name)
		}
	}
}
