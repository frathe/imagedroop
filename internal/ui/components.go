// App-owned component construction: the fixed-height layout and the small
// widget clusters buildViewer composes into the window. Each new*UI
// constructor builds one cluster and returns it as a small struct - the
// widgets themselves still land in the viewer's flat fields for now.
package ui

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/picfetch/internal/ui/assets"
	"github.com/frathe/picfetch/internal/ui/widgets"
)

// fixedHeightLayout wraps a single object, forcing its MinSize height to a
// fixed value instead of the object's natural (themed) size, while the
// object still fills whatever size it's ultimately resized to.
type fixedHeightLayout struct {
	height float32
}

func (f fixedHeightLayout) MinSize(objects []fyne.CanvasObject) fyne.Size {
	var w float32
	for _, o := range objects {
		w = fyne.Max(w, o.MinSize().Width)
	}
	return fyne.NewSize(w, f.height)
}

func (f fixedHeightLayout) Layout(objects []fyne.CanvasObject, size fyne.Size) {
	for _, o := range objects {
		o.Resize(size)
		o.Move(fyne.NewPos(0, 0))
	}
}

// dropzoneUI is the empty-state drop zone: the rounded border box, the
// "Drop images here" hint, the restore-session link, the welcome and
// empty-state art, all inside one tappable area (root) that doubles as an
// "open files" button.
type dropzoneUI struct {
	hint          *widget.Label
	restoreLink   *widget.Hyperlink
	welcomeArt    *canvas.Image
	emptyStateArt *canvas.Image
	art           *widgets.TappableArea
	root          *fyne.Container
}

// newDropzoneUI builds the drop zone. onOpen runs when the zone is tapped
// (the "open files" fallback for users who never drag-and-drop - see
// openFileDialog in openfiles.go); onRestore when the restore-session link
// is. Both callbacks are invoked only ever on a later tap, so buildViewer
// can hand in closures over a viewer variable that isn't assigned yet.
func newDropzoneUI(onOpen, onRestore func()) dropzoneUI {
	border := canvas.NewRectangle(color.Transparent)
	border.StrokeColor = widgets.DropzoneBorderColor
	border.StrokeWidth = widgets.DropzoneBorderWidth
	border.CornerRadius = widgets.DropzoneBorderRadius

	hint := widget.NewLabelWithStyle(lang.L("Drop images here"),
		fyne.TextAlignCenter, fyne.TextStyle{Bold: true})

	// restoreLink offers to reload the file set saved when the window last
	// closed (see session.go). Shown only if a saved session actually
	// exists - buildViewer sets its text and visibility once savedSession
	// is known.
	restoreLink := widget.NewHyperlink("", nil)
	restoreLink.Hide()
	restoreLink.OnTapped = onRestore

	// welcomeArt greets the user on first launch; handleDrop hides it for
	// good the moment the first drop happens. emptyStateArt is shown only
	// once an error subsequently leaves the drop zone empty (see ShowToast
	// call sites in drop.go/load.go). Both share one min size so they occupy
	// the exact same box on the right of the drop zone, and ImageFillContain
	// scales their (much larger) source art down to fit inside it.
	welcomeArt := canvas.NewImageFromResource(fyne.NewStaticResource("welcome.webp", assets.WelcomeWebP))
	welcomeArt.FillMode = canvas.ImageFillContain
	welcomeArt.ScaleMode = canvas.ImageScaleSmooth
	welcomeArt.SetMinSize(fyne.NewSize(widgets.WelcomeArtSize, widgets.WelcomeArtSize))

	emptyStateArt := canvas.NewImageFromResource(fyne.NewStaticResource("placeholder.webp", assets.PlaceholderWebP))
	emptyStateArt.FillMode = canvas.ImageFillContain
	emptyStateArt.ScaleMode = canvas.ImageScaleSmooth
	emptyStateArt.SetMinSize(fyne.NewSize(widgets.WelcomeArtSize, widgets.WelcomeArtSize))
	emptyStateArt.Hide()

	// Tappable so the whole drop zone - not just the art - doubles as an
	// "open files" button. restoreLink still gets its own taps: Fyne
	// resolves a tap to the deepest matching Tappable under the pointer, so
	// tapping restoreLink itself reaches its own OnTapped rather than this
	// wrapper's, even though it's nested inside it.
	art := widgets.NewTappableArea(container.NewBorder(nil, nil, nil,
		container.NewStack(welcomeArt, emptyStateArt),
		container.NewCenter(container.NewVBox(hint, restoreLink))), onOpen)
	art.OnHover = func(hovering bool) {
		if hovering {
			border.StrokeColor = widgets.DropzoneHoverColor
		} else {
			border.StrokeColor = widgets.DropzoneBorderColor
		}
		border.Refresh()
	}

	return dropzoneUI{
		hint:          hint,
		restoreLink:   restoreLink,
		welcomeArt:    welcomeArt,
		emptyStateArt: emptyStateArt,
		art:           art,
		root:          container.NewStack(border, art),
	}
}

// scanUI is the folder-scan progress indicator: an infinite spinner over a
// "Scanning... N images" counter, both hidden until handleDrop shows them.
type scanUI struct {
	spinner *widget.ProgressBarInfinite
	label   *widget.Label
}

func newScanUI() scanUI {
	spinner := widget.NewProgressBarInfinite()
	label := widget.NewLabelWithStyle(lang.L("Scanning... 0 images"), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	spinner.Hide()
	label.Hide()

	return scanUI{spinner: spinner, label: label}
}

// sortUI is the background-reorder progress indicator: an infinite spinner
// over a static "Sorting..." label, both hidden until startSort (sort.go)
// shows them - for a sort-mode change or for the reorder a finished drop
// hands over. A dedicated pair rather than reusing scanUI's - a background
// scan (a merge-mode drop) can still be in flight when a sort-mode change is
// requested, since handleKeyEvent's S-key guard only checks
// len(v.state.files)<2/v.loading, not v.scanning, and the two would otherwise
// fight over one pair of widgets. Unlike scanUI's label, this one's text
// never changes: the ask is only to show that a sort is running, not to
// track its progress the way the scan counter does.
type sortUI struct {
	spinner *widget.ProgressBarInfinite
	label   *widget.Label
}

func newSortUI() sortUI {
	spinner := widget.NewProgressBarInfinite()
	label := widget.NewLabelWithStyle(lang.L("Sorting..."), fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	spinner.Hide()
	label.Hide()

	return sortUI{spinner: spinner, label: label}
}

// infoUI is the persistent info overlay (I key, see toggleInfoOverlay in
// info.go) - unlike the toast it never auto-hides itself, and it's several
// distinct lines rather than one centered message, so it uses the theme's
// own overlay-background/foreground pairing (the same one dialogs use)
// instead of the toast's fixed, deliberately loud warning colors - legible
// in both light and dark themes without hardcoding either.
type infoUI struct {
	text     *widget.Label
	exifLink *widget.Hyperlink
	card     *fyne.Container
}

// newInfoOverlayUI builds the info card. onShowExif backs the "Show EXIF
// data" link right below the card's own text (the click equivalent of the E
// key, see internal/ui/exifwin); like newDropzoneUI's callbacks it
// only ever runs on a later tap, so it may close over a not-yet-assigned
// viewer variable.
func newInfoOverlayUI(onShowExif func()) infoUI {
	bg := canvas.NewRectangle(theme.Color(theme.ColorNameOverlayBackground))
	bg.CornerRadius = widgets.CardRadius
	text := widget.NewLabel("")
	text.Alignment = fyne.TextAlignLeading

	exifLink := widget.NewHyperlink(lang.L("Show EXIF data"), nil)
	exifLink.OnTapped = onShowExif

	card := container.NewStack(bg, container.NewPadded(container.NewVBox(text, exifLink)))
	card.Hide()

	return infoUI{text: text, exifLink: exifLink, card: card}
}
