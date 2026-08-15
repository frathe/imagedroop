package settingswin

import (
	"os"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"

	"github.com/frathe/imagedrop/internal/filesort"
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
	infoVisible  bool
	maxScan      int

	sortModeCalls     []filesort.Mode
	mergeModeCalls    []bool
	slideShuffleCalls []bool
	slideIntCalls     []time.Duration
	infoVisibleCalls  []bool
	maxScanCalls      []int
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
func (f *fakeHost) InfoVisible() bool { return f.infoVisible }
func (f *fakeHost) SetInfoVisible(b bool) {
	f.infoVisible = b
	f.infoVisibleCalls = append(f.infoVisibleCalls, b)
}
func (f *fakeHost) MaxScan() int     { return f.maxScan }
func (f *fakeHost) SetMaxScan(n int) { f.maxScan = n; f.maxScanCalls = append(f.maxScanCalls, n) }

// TestShow_SeedsEveryControlFromHostWithoutRoundTripping checks both halves
// of build's own contract: every control reflects the host's current value,
// and none of that seeding round-trips back into the host as a spurious
// Set* call - which Select.SetSelected/Check.SetChecked/Entry.SetText would
// each risk, since every one of them fires its own OnChanged.
func TestShow_SeedsEveryControlFromHostWithoutRoundTripping(t *testing.T) {
	host := &fakeHost{
		sortMode: filesort.BySize, mergeMode: true, slideShuffle: true,
		slideInt: 42 * time.Second, infoVisible: true, maxScan: 777,
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
	if !w.infoCheck.Checked {
		t.Error("infoCheck should be checked, seeded from host.InfoVisible() = true")
	}
	if got, want := w.intervalEntry.Text, "42"; got != want {
		t.Errorf("intervalEntry.Text = %q, want %q", got, want)
	}
	if got, want := w.maxScanEntry.Text, "777"; got != want {
		t.Errorf("maxScanEntry.Text = %q, want %q", got, want)
	}

	if len(host.sortModeCalls)+len(host.mergeModeCalls)+len(host.slideShuffleCalls)+
		len(host.slideIntCalls)+len(host.infoVisibleCalls)+len(host.maxScanCalls) != 0 {
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
	w.infoCheck.SetChecked(true)

	if len(host.mergeModeCalls) != 1 || !host.mergeModeCalls[0] {
		t.Errorf("SetMergeMode calls = %v, want one call with true", host.mergeModeCalls)
	}
	if len(host.slideShuffleCalls) != 1 || !host.slideShuffleCalls[0] {
		t.Errorf("SetSlideShuffle calls = %v, want one call with true", host.slideShuffleCalls)
	}
	if len(host.infoVisibleCalls) != 1 || !host.infoVisibleCalls[0] {
		t.Errorf("SetInfoVisible calls = %v, want one call with true", host.infoVisibleCalls)
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
// reaches the host - only strconv.Atoi-parseable positive integers should.
func TestIntervalEntry_InvalidTextIsIgnored(t *testing.T) {
	host := &fakeHost{}
	w := New(testApp, host)
	w.Show()
	t.Cleanup(func() { w.win.Window().Close() })

	for _, text := range []string{"", "abc", "-5", "0"} {
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
