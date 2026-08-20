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
// A var so tests can substitute a short fixture: rendering the real manuals
// panics under Fyne's test theme (bold inside a code span).
var currentManual = func() string {
	if strings.HasPrefix(systemLocale().LanguageString(), "de") {
		return manualDE
	}
	return manualMD
}

// manualView is the search-enabled manual page: a fixed entry above the
// scrollable markdown. Submit (Enter) highlights matches and scrolls the
// current hit into view; a repeated submit of the same query walks forward.
type manualView struct {
	source  string
	text    *widget.RichText
	scroll  *container.Scroll
	entry   *widget.Entry
	state   searchState
	current *widget.TextSegment
}

func newManualView(source string) *manualView {
	text := widget.NewRichTextFromMarkdown(source)
	text.Wrapping = fyne.TextWrapWord
	scroll := container.NewScroll(text)
	entry := widget.NewEntry()
	entry.SetPlaceHolder(lang.L("Search for..."))

	v := &manualView{source: source, text: text, scroll: scroll, entry: entry}
	entry.OnSubmitted = v.submit

	return v
}

func (v *manualView) content() fyne.CanvasObject {
	return container.NewBorder(container.NewPadded(v.entry), nil, nil, nil, v.scroll)
}

func (v *manualView) submit(q string) {
	q = normalizeQuery(q)
	v.text.ParseMarkdown(v.source)

	if q == "" {
		v.state = searchState{}
		v.current = nil
		v.text.Refresh()

		return
	}

	n := countMatches(v.text.Segments, q)
	idx := v.state.nextIndex(q, n)
	if idx < 0 {
		v.current = nil
		v.text.Refresh()

		return
	}

	var loc *widget.TextSegment
	v.text.Segments, _, loc = highlightSegments(v.text.Segments, q, idx)
	v.current = loc
	v.text.Refresh()
	v.scrollTo(loc)
}

func (v *manualView) scrollTo(loc *widget.TextSegment) {
	if v.scroll == nil || loc == nil {
		return
	}

	y, ok := currentHitY(v.text, loc.Text)
	if !ok {
		return
	}

	viewH := v.scroll.Size().Height
	contentH := v.text.MinSize().Height
	offset := y - viewH/3
	if offset < 0 {
		offset = 0
	}

	maxOffset := contentH - viewH
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}

	v.scroll.ScrollToOffset(fyne.NewPos(0, offset))
}

// ShowManual opens the manual in its own scrollable window. A second call
// while the window is still open just raises it instead of opening a
// duplicate (see widgets.Singleton).
func (h *Help) ShowManual() {
	h.manualWin.Show(h.app, lang.L("PicFetch Manual"), fyne.NewSize(manualW, manualH), func() fyne.CanvasObject {
		h.manual = newManualView(currentManual())

		return h.manual.content()
	}, func() {
		h.manual = nil
	})

	if win := h.manualWin.Window(); win != nil && h.manual != nil {
		win.Canvas().Focus(h.manual.entry)
	}
}
