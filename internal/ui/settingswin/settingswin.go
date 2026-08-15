// Package settingswin is the Settings window, reachable from the File menu:
// one place to see and change every standing preference the app has - sort
// order, merge mode, picture-frame shuffle and interval, the folder-scan
// cap, and the window-size cap - instead of only discovering them by
// stumbling onto their keyboard shortcuts.
//
// Every control applies live, through its own OnChanged, the same
// immediate-effect behavior the S/M/Shift+P keys already give their own
// preferences - there is no separate Save/Apply step and so nothing here
// needs to track a "dirty" draft state.
package settingswin

import (
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/validation"
	"fyne.io/fyne/v2/lang"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/imagedrop/internal/filesort"
	"github.com/frathe/imagedrop/internal/ui/widgets"
)

const (
	windowW = 460.0
	windowH = 340.0
)

// Host is what the settings window needs from the app: read/write access to
// every standing preference it exposes. Every setter is expected to apply
// its change immediately, the same as the keyboard shortcut that already
// exists for it (where one exists) - the window itself holds no state of
// its own to reconcile later.
type Host interface {
	SortMode() filesort.Mode
	SetSortMode(filesort.Mode)

	MergeMode() bool
	SetMergeMode(bool)

	SlideShuffle() bool
	SetSlideShuffle(bool)

	SlideInterval() time.Duration
	SetSlideInterval(time.Duration)

	MaxScan() int
	SetMaxScan(int)

	MaxWindowWidth() float32
	SetMaxWindowWidth(float32)

	MaxWindowHeight() float32
	SetMaxWindowHeight(float32)
}

// Window is the settings panel. At most one is open at a time (widgets.
// Singleton): a second request raises the existing window rather than
// stacking up duplicates.
type Window struct {
	app  fyne.App
	host Host

	win widgets.Singleton

	// The controls themselves, live only while the window is open (nil
	// otherwise - the same pattern exifwin.Window's text field uses). Kept
	// as fields rather than locals inside build so this package's own tests
	// can drive them directly, the same way internal/ui/deletion's tests
	// drive that confirmation card's widgets.
	sortSelect                    *widget.Select
	mergeCheck, shuffleCheck      *widget.Check
	intervalEntry, maxScanEntry   *widget.Entry
	maxWidthEntry, maxHeightEntry *widget.Entry
}

// New returns the settings window for application, reading and writing its
// preferences through host.
func New(application fyne.App, host Host) *Window {
	return &Window{app: application, host: host}
}

// Show opens the settings window, or raises it if it's already open.
func (w *Window) Show() {
	w.win.Show(w.app, lang.L("Settings"), fyne.NewSize(windowW, windowH), w.build, func() {
		w.sortSelect = nil
		w.mergeCheck, w.shuffleCheck = nil, nil
		w.intervalEntry, w.maxScanEntry = nil, nil
		w.maxWidthEntry, w.maxHeightEntry = nil, nil
	})
}

// Open reports whether the settings window is currently showing.
func (w *Window) Open() bool {
	return w.win.Open()
}

// build lays out every control, each one seeded from the host's current
// value and wired to push a change straight back through it. Initial
// seeding sets the widgets' fields directly rather than through their own
// SetSelected/SetChecked/SetText - those fire OnChanged themselves, which
// would otherwise round-trip the freshly read value straight back into the
// host before the window has even been shown.
func (w *Window) build() fyne.CanvasObject {
	positiveInt := validation.NewRegexp(`^[1-9][0-9]*$`, lang.L("must be a positive whole number"))

	modes := filesort.Modes()
	labels := make([]string, len(modes))
	for i, m := range modes {
		labels[i] = filesort.DisplayName(m)
	}

	w.sortSelect = widget.NewSelect(labels, func(s string) {
		for i, l := range labels {
			if l == s {
				w.host.SetSortMode(modes[i])
				return
			}
		}
	})
	w.sortSelect.Selected = filesort.DisplayName(w.host.SortMode())

	w.intervalEntry = widget.NewEntry()
	w.intervalEntry.Validator = positiveInt
	w.intervalEntry.Text = strconv.Itoa(int(w.host.SlideInterval().Seconds()))
	w.intervalEntry.OnChanged = func(s string) {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			w.host.SetSlideInterval(time.Duration(n) * time.Second)
		}
	}

	w.maxScanEntry = widget.NewEntry()
	w.maxScanEntry.Validator = positiveInt
	w.maxScanEntry.Text = strconv.Itoa(w.host.MaxScan())
	w.maxScanEntry.OnChanged = func(s string) {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			w.host.SetMaxScan(n)
		}
	}

	maxScanItem := widget.NewFormItem(lang.L("Max files per folder scan"), w.maxScanEntry)
	maxScanItem.HintText = lang.L("Caps how many images a single recursive folder scan will gather")

	w.maxWidthEntry = widget.NewEntry()
	w.maxWidthEntry.Validator = positiveInt
	w.maxWidthEntry.Text = strconv.Itoa(int(w.host.MaxWindowWidth()))
	w.maxWidthEntry.OnChanged = func(s string) {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			w.host.SetMaxWindowWidth(float32(n))
		}
	}

	w.maxHeightEntry = widget.NewEntry()
	w.maxHeightEntry.Validator = positiveInt
	w.maxHeightEntry.Text = strconv.Itoa(int(w.host.MaxWindowHeight()))
	w.maxHeightEntry.OnChanged = func(s string) {
		if n, err := strconv.Atoi(s); err == nil && n > 0 {
			w.host.SetMaxWindowHeight(float32(n))
		}
	}

	form := widget.NewForm(
		widget.NewFormItem(lang.L("Sort order"), w.sortSelect),
		widget.NewFormItem(lang.L("Picture-frame interval (seconds)"), w.intervalEntry),
		maxScanItem,
		widget.NewFormItem(lang.L("Max window width"), w.maxWidthEntry),
		widget.NewFormItem(lang.L("Max window height"), w.maxHeightEntry),
	)

	w.mergeCheck = widget.NewCheck(lang.L("Merge newly dropped files into the current set"), w.host.SetMergeMode)
	w.mergeCheck.Checked = w.host.MergeMode()

	w.shuffleCheck = widget.NewCheck(lang.L("Shuffle picture-frame order"), w.host.SetSlideShuffle)
	w.shuffleCheck.Checked = w.host.SlideShuffle()

	return container.NewPadded(container.NewVBox(form, widget.NewSeparator(), w.mergeCheck, w.shuffleCheck))
}
