package help

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"
)

const (
	aboutW = 640.0
	aboutH = 320.0

	aboutArtSize = 260.0
)

// ShowAbout opens the About window: the welcome art on the right, and the
// app name in a large heading, its version/build, and a link to the manual
// on the left. A second call while it's still open just raises it instead of
// stacking up duplicates (see widgets.Singleton).
func (h *Help) ShowAbout() {
	h.aboutWin.Show(h.app, fmt.Sprintf(lang.L("About %s"), h.title), fyne.NewSize(aboutW, aboutH), func() fyne.CanvasObject {
		art := canvas.NewImageFromResource(fyne.NewStaticResource("welcome.webp", h.art))
		art.FillMode = canvas.ImageFillContain
		art.ScaleMode = canvas.ImageScaleSmooth
		art.SetMinSize(fyne.NewSize(aboutArtSize, aboutArtSize))

		title := widget.NewRichTextFromMarkdown("# " + h.title)

		meta := h.app.Metadata()
		version := widget.NewLabel(fmt.Sprintf(lang.L("Version %s (Build %d)"), meta.Version, meta.Build))

		manualLink := widget.NewHyperlink(lang.L("Open the manual"), nil)
		manualLink.OnTapped = h.ShowManual

		left := container.NewPadded(container.NewVBox(title, version, manualLink))

		return container.NewBorder(nil, nil, nil, art, left)
	}, nil)
}
