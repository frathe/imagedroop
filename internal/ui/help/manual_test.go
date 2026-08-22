package help

import (
	"path"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
)

// manuals is shared by every check below that should hold for both shipped
// editions, keyed by embed filename for error messages.
var manuals = map[string]string{
	"manual.md":    manualMD,
	"manual_de.md": manualDE,
}

func TestManualIsEmbedded(t *testing.T) {
	for name, md := range manuals {
		if len(md) == 0 {
			t.Errorf("%s was not embedded", name)
			continue
		}

		if !strings.HasPrefix(md, "# PicFetch") {
			t.Errorf("%s does not start with the expected heading, got %q", name, firstLine(md))
		}
	}
}

// TestManualsShareMascotImages keeps the English and German editions in lockstep
// on the three Trane pictures: same files, same order, so a translation update
// cannot drop one or shuffle them.
func TestManualsShareMascotImages(t *testing.T) {
	want := []string{"TaneWithFrame.webp", "trane_digging.webp", "trane_wags.webp"}
	for name, md := range manuals {
		if got := mascotRefs(md); !reflect.DeepEqual(got, want) {
			t.Errorf("%s mascot images = %v, want %v in that order", name, got, want)
		}
	}
}

var mascotMarkdownRef = regexp.MustCompile(`]\(([^)]+\.webp)\)`)

func mascotRefs(md string) []string {
	var out []string
	for _, m := range mascotMarkdownRef.FindAllStringSubmatch(md, -1) {
		out = append(out, path.Base(m[1]))
	}
	return out
}

// TestManualHasNoMarkdownTables guards the in-app rendering: Fyne's markdown
// support has no table extension, so a table in either manual would show up
// as a wall of pipe characters in the help window.
func TestManualHasNoMarkdownTables(t *testing.T) {
	for name, md := range manuals {
		for i, line := range strings.Split(md, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "|") {
				t.Errorf("%s:%d looks like a markdown table row, which Fyne cannot render: %q", name, i+1, line)
			}
		}
	}
}

// TestManualUnicodeArrowsStayInCodeSpans guards a Fyne font quirk: the
// regular (non-monospace) face shapes U+2192 as a visible arrow plus a
// .notdef glyph, which the painter draws as � after every arrow. Key
// arrows belong in backticks (the monospace face has them); cycles and
// menu paths use ASCII "->", as the keyboard-shortcuts section already does.
func TestManualUnicodeArrowsStayInCodeSpans(t *testing.T) {
	fenced := regexp.MustCompile("(?s)```.*?```")
	inline := regexp.MustCompile("`[^`]*`")
	arrow := regexp.MustCompile(`[←↑→↓]`)

	for name, md := range manuals {
		body := inline.ReplaceAllString(fenced.ReplaceAllString(md, ""), "")
		for i, line := range strings.Split(body, "\n") {
			if arrow.MatchString(line) {
				t.Errorf("%s:%d has a Unicode arrow outside a code span: %q", name, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// TestManualDocumentsItsOwnShortcut is a cheap consistency check that each
// manual keeps mentioning the key that opens it. F1 is a physical key name,
// not translated, so it reads the same in both editions.
func TestManualDocumentsItsOwnShortcut(t *testing.T) {
	for name, md := range manuals {
		if !strings.Contains(md, "F1") {
			t.Errorf("%s does not mention the F1 shortcut", name)
		}
	}
}

// TestCurrentManual_GermanLocaleUsesGermanManual and
// TestCurrentManual_OtherLocaleFallsBackToEnglish cover currentManual's
// locale switch (manual.go) via the systemLocale var, the way this codebase
// stubs an OS-dependent lookup elsewhere (e.g. clipboard's lookupXClip).
func TestCurrentManual_GermanLocaleUsesGermanManual(t *testing.T) {
	orig := systemLocale
	t.Cleanup(func() { systemLocale = orig })

	for _, loc := range []fyne.Locale{"de", "de-DE", "de-AT", "de-CH"} {
		systemLocale = func() fyne.Locale { return loc }
		if got := currentManual(); got != manualDE {
			t.Errorf("currentManual() for locale %q did not return the German manual", loc)
		}
	}
}

func TestCurrentManual_OtherLocaleFallsBackToEnglish(t *testing.T) {
	orig := systemLocale
	t.Cleanup(func() { systemLocale = orig })

	for _, loc := range []fyne.Locale{"en", "en-US", "fr", "fr-FR"} {
		systemLocale = func() fyne.Locale { return loc }
		if got := currentManual(); got != manualMD {
			t.Errorf("currentManual() for locale %q did not fall back to the English manual", loc)
		}
	}
}

func TestHelpMenu(t *testing.T) {
	help := New(nil, "PicFetch", nil).Menu()

	if help.Label != "Help" {
		t.Errorf("expected menu label %q, got %q", "Help", help.Label)
	}

	if got := len(help.Items); got != 3 {
		t.Fatalf("expected 3 help items, got %d", got)
	}

	manual := help.Items[0]

	if manual.Label != "Manual" {
		t.Errorf("expected item label %q, got %q", "Manual", manual.Label)
	}

	if manual.Action == nil {
		t.Error("manual menu item has no action")
	}

	shortcut, ok := manual.Shortcut.(*desktop.CustomShortcut)
	if !ok {
		t.Fatalf("Manual item Shortcut = %#v, want a *desktop.CustomShortcut for F1", manual.Shortcut)
	}
	if shortcut.KeyName != fyne.KeyF1 || shortcut.Modifier != 0 {
		t.Errorf("Manual accelerator = %+v, want {KeyF1, 0}", shortcut)
	}

	if !help.Items[1].IsSeparator {
		t.Error("expected a separator between Manual and About")
	}

	about := help.Items[2]

	if about.Label != "About" {
		t.Errorf("expected item label %q, got %q", "About", about.Label)
	}

	if about.Action == nil {
		t.Error("about menu item has no action")
	}
}

func firstLine(s string) string {
	if before, _, ok := strings.Cut(s, "\n"); ok {
		return before
	}

	return s
}
