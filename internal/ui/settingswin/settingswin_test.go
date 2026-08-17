package settingswin

import (
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"

	"github.com/frathe/picfetch/internal/filesort"
)

// testApp is shared across every test below, mirroring internal/ui's own
// testApp (library_test.go): test.NewApp() resets process-global caches
// (font shaping, theme), so building one per test would pay that cost on
// every single test instead of once for the package.
var testApp fyne.App

func TestMain(m *testing.M) {
	testApp = test.NewApp()
	os.Exit(m.Run())
}

// fakeHost records what the panel asked the app to do, and answers every
// getter from its own fields - the whole point of Host being a
// consumer-side interface: the panel can be driven, and every effect
// observed, without a real viewer or window. Mirrors
// internal/ui/deletion's own fakeHost.
type fakeHost struct {
	sortMode     filesort.Mode
	mergeMode    bool
	slideShuffle bool
	slideInt     time.Duration
	maxScan      int
	maxWinW      float32
	maxWinH      float32
	imgCacheMB   int
	thumbCacheMB int
	maxFileMB    int

	sortModeCalls     []filesort.Mode
	mergeModeCalls    []bool
	slideShuffleCalls []bool
	slideIntCalls     []time.Duration
	maxScanCalls      []int
	maxWinWCalls      []float32
	maxWinHCalls      []float32
	imgCacheCalls     []int
	thumbCacheCalls   []int
	maxFileCalls      []int
}

func (f *fakeHost) SortMode() filesort.Mode { return f.sortMode }
func (f *fakeHost) SetSortMode(m filesort.Mode) {
	f.sortMode = m
	f.sortModeCalls = append(f.sortModeCalls, m)
}
func (f *fakeHost) MergeMode() bool { return f.mergeMode }
func (f *fakeHost) SetMergeMode(b bool) {
	f.mergeMode = b
	f.mergeModeCalls = append(f.mergeModeCalls, b)
}
func (f *fakeHost) SlideShuffle() bool { return f.slideShuffle }
func (f *fakeHost) SetSlideShuffle(b bool) {
	f.slideShuffle = b
	f.slideShuffleCalls = append(f.slideShuffleCalls, b)
}
func (f *fakeHost) SlideInterval() time.Duration { return f.slideInt }
func (f *fakeHost) SetSlideInterval(d time.Duration) {
	f.slideInt = d
	f.slideIntCalls = append(f.slideIntCalls, d)
}
func (f *fakeHost) MaxScan() int            { return f.maxScan }
func (f *fakeHost) SetMaxScan(n int)        { f.maxScan = n; f.maxScanCalls = append(f.maxScanCalls, n) }
func (f *fakeHost) MaxWindowWidth() float32 { return f.maxWinW }
func (f *fakeHost) SetMaxWindowWidth(w float32) {
	f.maxWinW = w
	f.maxWinWCalls = append(f.maxWinWCalls, w)
}
func (f *fakeHost) MaxWindowHeight() float32 { return f.maxWinH }
func (f *fakeHost) SetMaxWindowHeight(h float32) {
	f.maxWinH = h
	f.maxWinHCalls = append(f.maxWinHCalls, h)
}
func (f *fakeHost) MaxImageCacheMB() int { return f.imgCacheMB }
func (f *fakeHost) SetMaxImageCacheMB(n int) {
	f.imgCacheMB = n
	f.imgCacheCalls = append(f.imgCacheCalls, n)
}
func (f *fakeHost) MaxThumbCacheMB() int { return f.thumbCacheMB }
func (f *fakeHost) SetMaxThumbCacheMB(n int) {
	f.thumbCacheMB = n
	f.thumbCacheCalls = append(f.thumbCacheCalls, n)
}
func (f *fakeHost) MaxFileSizeMB() int { return f.maxFileMB }
func (f *fakeHost) SetMaxFileSizeMB(n int) {
	f.maxFileMB = n
	f.maxFileCalls = append(f.maxFileCalls, n)
}

// TestShow_SeedsEveryControlFromHostWithoutRoundTripping checks both halves
// of build's own contract: every control reflects the host's current value,
// and none of that seeding round-trips back into the host as a spurious
// Set* call - which Select.SetSelected/Check.SetChecked/Entry.SetText would
// each risk, since every one of them fires its own OnChanged.
func TestShow_SeedsEveryControlFromHostWithoutRoundTripping(t *testing.T) {
	host := &fakeHost{
		sortMode: filesort.BySize, mergeMode: true, slideShuffle: true,
		slideInt: 42 * time.Second, maxScan: 777, maxWinW: 1800, maxWinH: 1100,
		imgCacheMB: 384, thumbCacheMB: 192, maxFileMB: 256,
	}
	w := New(testApp, host)

	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	if got, want := w.sortSelect.Selected, filesort.DisplayName(filesort.BySize); got != want {
		t.Errorf("sortSelect.Selected = %q, want %q", got, want)
	}
	if !w.mergeCheck.Checked {
		t.Error("mergeCheck should be checked, seeded from host.MergeMode() = true")
	}
	if !w.shuffleCheck.Checked {
		t.Error("shuffleCheck should be checked, seeded from host.SlideShuffle() = true")
	}
	if got, want := w.intervalEntry.Text, "42"; got != want {
		t.Errorf("intervalEntry.Text = %q, want %q", got, want)
	}
	if got, want := w.maxScanEntry.Text, "777"; got != want {
		t.Errorf("maxScanEntry.Text = %q, want %q", got, want)
	}
	if got, want := w.maxWidthEntry.Text, "1800"; got != want {
		t.Errorf("maxWidthEntry.Text = %q, want %q", got, want)
	}
	if got, want := w.maxHeightEntry.Text, "1100"; got != want {
		t.Errorf("maxHeightEntry.Text = %q, want %q", got, want)
	}
	if got, want := w.imgCacheEntry.Text, "384"; got != want {
		t.Errorf("imgCacheEntry.Text = %q, want %q", got, want)
	}
	if got, want := w.thumbCacheEntry.Text, "192"; got != want {
		t.Errorf("thumbCacheEntry.Text = %q, want %q", got, want)
	}
	if got, want := w.maxFileSizeEntry.Text, "256"; got != want {
		t.Errorf("maxFileSizeEntry.Text = %q, want %q", got, want)
	}

	if len(host.sortModeCalls)+len(host.mergeModeCalls)+len(host.slideShuffleCalls)+
		len(host.slideIntCalls)+len(host.maxScanCalls)+len(host.maxWinWCalls)+len(host.maxWinHCalls)+
		len(host.imgCacheCalls)+len(host.thumbCacheCalls)+len(host.maxFileCalls) != 0 {
		t.Errorf("seeding the controls should not call any Set* method on the host, got calls: %+v", host)
	}
}

func TestShow_RaisesTheSameWindowOnASecondCall(t *testing.T) {
	w := New(testApp, &fakeHost{})

	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })
	win := w.win.Window()

	w.Show()

	if w.win.Window() != win {
		t.Error("a second Show should raise the existing window, not open a new one")
	}
}

func TestSortSelect_ChangeCallsSetSortMode(t *testing.T) {
	host := &fakeHost{sortMode: filesort.ByName}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	w.sortSelect.SetSelected(filesort.DisplayName(filesort.ByCaptureDate))

	if len(host.sortModeCalls) != 1 || host.sortModeCalls[0] != filesort.ByCaptureDate {
		t.Errorf("SetSortMode calls = %v, want one call with ByCaptureDate", host.sortModeCalls)
	}
}

func TestChecks_ChangeCallTheMatchingSetter(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	w.mergeCheck.SetChecked(true)
	w.shuffleCheck.SetChecked(true)

	if len(host.mergeModeCalls) != 1 || !host.mergeModeCalls[0] {
		t.Errorf("SetMergeMode calls = %v, want one call with true", host.mergeModeCalls)
	}
	if len(host.slideShuffleCalls) != 1 || !host.slideShuffleCalls[0] {
		t.Errorf("SetSlideShuffle calls = %v, want one call with true", host.slideShuffleCalls)
	}
}

func TestIntervalEntry_ValidChangeCallsSetSlideInterval(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	w.intervalEntry.SetText("15")

	if len(host.slideIntCalls) != 1 || host.slideIntCalls[0] != 15*time.Second {
		t.Errorf("SetSlideInterval calls = %v, want one call with 15s", host.slideIntCalls)
	}
}

// TestIntervalEntry_InvalidTextIsIgnored checks that neither an empty field
// (the natural mid-edit state while retyping a value) nor outright garbage
// reaches the host. Values too large to fit in time.Duration are rejected
// too, rather than overflowing to a negative duration.
func TestIntervalEntry_InvalidTextIsIgnored(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	for _, text := range []string{"", "abc", "-5", "0", "9223372037", "999999999999999999999999"} {
		w.intervalEntry.SetText(text)
	}

	if len(host.slideIntCalls) != 0 {
		t.Errorf("SetSlideInterval calls = %v, want none for invalid input", host.slideIntCalls)
	}
}

func TestMaxScanEntry_ValidChangeCallsSetMaxScan(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	w.maxScanEntry.SetText("250000")

	if len(host.maxScanCalls) != 1 || host.maxScanCalls[0] != 250000 {
		t.Errorf("SetMaxScan calls = %v, want one call with 250000", host.maxScanCalls)
	}
}

func TestMaxScanEntry_InvalidTextIsIgnored(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	for _, text := range []string{"", "abc", "-1", "0"} {
		w.maxScanEntry.SetText(text)
	}

	if len(host.maxScanCalls) != 0 {
		t.Errorf("SetMaxScan calls = %v, want none for invalid input", host.maxScanCalls)
	}
}

func TestMaxWidthEntry_ValidChangeCallsSetMaxWindowWidth(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	w.maxWidthEntry.SetText("1600")

	if len(host.maxWinWCalls) != 1 || host.maxWinWCalls[0] != 1600 {
		t.Errorf("SetMaxWindowWidth calls = %v, want one call with 1600", host.maxWinWCalls)
	}
}

func TestMaxWidthEntry_InvalidTextIsIgnored(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	for _, text := range []string{"", "abc", "-1", "0"} {
		w.maxWidthEntry.SetText(text)
	}

	if len(host.maxWinWCalls) != 0 {
		t.Errorf("SetMaxWindowWidth calls = %v, want none for invalid input", host.maxWinWCalls)
	}
}

func TestMaxHeightEntry_ValidChangeCallsSetMaxWindowHeight(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	w.maxHeightEntry.SetText("1000")

	if len(host.maxWinHCalls) != 1 || host.maxWinHCalls[0] != 1000 {
		t.Errorf("SetMaxWindowHeight calls = %v, want one call with 1000", host.maxWinHCalls)
	}
}

func TestMaxHeightEntry_InvalidTextIsIgnored(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	for _, text := range []string{"", "abc", "-1", "0"} {
		w.maxHeightEntry.SetText(text)
	}

	if len(host.maxWinHCalls) != 0 {
		t.Errorf("SetMaxWindowHeight calls = %v, want none for invalid input", host.maxWinHCalls)
	}
}

func TestImgCacheEntry_ValidChangeCallsSetMaxImageCacheMB(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	w.imgCacheEntry.SetText("768")

	if len(host.imgCacheCalls) != 1 || host.imgCacheCalls[0] != 768 {
		t.Errorf("SetMaxImageCacheMB calls = %v, want one call with 768", host.imgCacheCalls)
	}
}

func TestThumbCacheEntry_ValidChangeCallsSetMaxThumbCacheMB(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	w.thumbCacheEntry.SetText("128")

	if len(host.thumbCacheCalls) != 1 || host.thumbCacheCalls[0] != 128 {
		t.Errorf("SetMaxThumbCacheMB calls = %v, want one call with 128", host.thumbCacheCalls)
	}
}

func TestMaxFileSizeEntry_ValidChangeCallsSetMaxFileSizeMB(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	w.maxFileSizeEntry.SetText("64")

	if len(host.maxFileCalls) != 1 || host.maxFileCalls[0] != 64 {
		t.Errorf("SetMaxFileSizeMB calls = %v, want one call with 64", host.maxFileCalls)
	}
}

// The memory entries reject one thing the other numeric entries don't: a
// value past maxMemoryMB, which the host would shift left by 20 into an
// int64 byte budget. "1048577" is one megabyte past the ceiling, and
// "99999999999999999999" is past what Atoi can parse at all - both have to
// be ignored rather than wrapped into a nonsense budget, the same guard
// TestIntervalEntry_InvalidTextIsIgnored makes for time.Duration.
func TestMemoryEntries_InvalidTextIsIgnored(t *testing.T) {
	bad := []string{"", "abc", "-1", "0", "1048577", "99999999999999999999"}

	cases := []struct {
		name  string
		entry func(*Window) *widget.Entry
		calls func(*fakeHost) []int
	}{
		{"image cache", func(w *Window) *widget.Entry { return w.imgCacheEntry },
			func(f *fakeHost) []int { return f.imgCacheCalls }},
		{"thumbnail cache", func(w *Window) *widget.Entry { return w.thumbCacheEntry },
			func(f *fakeHost) []int { return f.thumbCacheCalls }},
		{"max file size", func(w *Window) *widget.Entry { return w.maxFileSizeEntry },
			func(f *fakeHost) []int { return f.maxFileCalls }},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			host := &fakeHost{}
			w := New(testApp, host)
			w.Show()
			t.Cleanup(func() { w.win.Window().Close() })

			for _, text := range bad {
				c.entry(w).SetText(text)
			}

			if got := c.calls(host); len(got) != 0 {
				t.Errorf("setter calls = %v, want none for invalid input", got)
			}
		})
	}
}

func TestOpen_ReflectsWindowLifecycle(t *testing.T) {
	w := New(testApp, &fakeHost{})

	if w.Open() {
		t.Fatal("Open() = true before Show was ever called")
	}

	w.Show()
	if !w.Open() {
		t.Error("Open() = false, want true once Show has run")
	}

	w.win.Window().Close()
	if w.Open() {
		t.Error("Open() = true, want false once the window is closed")
	}
	if w.sortSelect != nil {
		t.Error("expected the widget fields to be cleared once the window closes")
	}
}
