package help

import (
	_ "embed"
	"strings"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
)

const (
	manualW = 720.0
	manualH = 820.0
)

// manualMD is the English end-user manual, embedded so the packaged app
// carries its own help text and doesn't depend on the file being shipped
// alongside the binary. It is rendered with Fyne's markdown support, which
// has no table extension — keep manual.md (and manual_de.md) free of
// markdown tables.
//
//go:embed manual.md
var manualMD string

// manualDE is the German translation of manualMD, picked by currentManual
// when the system locale is German. Unlike translations/*.json (checked
// key-for-key against en.json in main_test.go), there's no automatic check
// that this stays in sync with manual.md's content - update it by hand
// alongside any manual.md change.
//
//go:embed manual_de.md
var manualDE string

// systemLocale is lang.SystemLocale; a var so tests can force either
// outcome instead of depending on the locale of the machine running the
// test.
var systemLocale = lang.SystemLocale

// currentManual picks the manual matching the system's language - German for
// a German-language locale (de, de-AT, de-CH, ...), English otherwise. Only
// these two are shipped, so a plain prefix check is enough; a third locale
// would need lang.SystemLocale's own closest-match machinery instead.
func currentManual() string {
	if strings.HasPrefix(systemLocale().LanguageString(), "de") {
		return manualDE
	}
	return manualMD
}

// ShowManual opens the manual in its own scrollable window. A second call
// while the window is still open just raises it instead of opening a
// duplicate (see widgets.Singleton).
func (h *Help) ShowManual() {
	h.manualWin.Show(h.app, lang.L("Image Drop Manual"), fyne.NewSize(manualW, manualH), func() fyne.CanvasObject {
		text := widget.NewRichTextFromMarkdown(currentManual())
		text.Wrapping = fyne.TextWrapWord

		return container.NewScroll(text)
	}, nil)
}
