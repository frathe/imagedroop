package ui

import (
	"fmt"
	"image/color"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/storage"

	"github.com/frathe/picfetch/internal/uitest"
)

// This file covers drop.go: the path from a dropped or opened set of paths
// to a loaded file set. That includes handleDrop's synchronous behaviour,
// the recursive folder scan and its three guards - a symlink back to an
// ancestor directory would loop forever without tracking each directory's
// real, resolved path; the same folder reached via two dropped paths must
// not double-count its images; and maxScan stops a runaway tree before it
// stalls the scan goroutine - merge vs. replace (the M key toggles whether a
// second drop adds to the current set or starts over, and a merge that
// finds nothing supported must leave the existing set untouched), and scan
// cancellation: Escape during an in-flight scan cancels it rather than
// resetting the session, clearToDropzone must finish the scan overlay the
// same way invalidateSort finishes a reorder (reset and ShowEmptyStateError
// have no cancelScan of their own), and a superseded scan's goroutine must
// notice it's stale and stop touching the filesystem instead of racing a
// large tree to completion for a result that will be discarded.
//
// toggleMergeMode and SetMergeMode themselves live in viewer.go, not
// drop.go, but their tests belong here: what they assert is entirely about
// how a drop composes with the set already loaded.

// --- viewer.handleDrop (synchronous behaviour) -----------------------------

func TestHandleDrop_EmptyDrop(t *testing.T) {
	v := newTestViewer(t)

	v.handleDrop(nil)

	if v.state.files != nil {
		t.Errorf("files = %v, want nil after an empty drop", v.state.files)
	}

	if n := len(v.win.Canvas().Overlays().List()); n != 0 {
		t.Errorf("overlays = %d, want 0 after an empty drop", n)
	}
}

func TestHandleDrop_NoSupportedImages(t *testing.T) {
	v := newTestViewer(t)

	v.handleDrop([]fyne.URI{
		uitest.FakeURI{FileName: "a.txt", Ext: ".txt"},
		uitest.FakeURI{FileName: "b.pdf", Ext: ".pdf"},
	})
	waitForScan(t, v)

	if v.state.files != nil {
		t.Errorf("files = %v, want nil when nothing dropped is a supported image", v.state.files)
	}

	if !v.toast.card.Visible() {
		t.Error("expected a toast to be shown when nothing dropped is a supported image")
	}

	if !v.dropzone.Visible() {
		t.Error("dropzone (\"Drop images here\") should be restored once the scan finds nothing to show")
	}
	settleToast(t, v)
}

func TestHandleDrop_ErrorAfterImagesClearsDisplay(t *testing.T) {
	v := newTestViewer(t)

	jpegURI := uitest.TempJPEGURI(t, "one.jpg", 10, 10, color.RGBA{R: 255, A: 255})
	dropAndWait(t, v, jpegURI)

	if v.img.Image == nil {
		t.Fatal("expected an image to be loaded before the second, bad drop")
	}

	// A second drop that yields nothing displayable must not leave the
	// previous image sitting behind the error toast and placeholder art.
	dropAndWaitScan(t, v, uitest.FakeURI{FileName: "notes.txt", Ext: ".txt"})

	if v.state.files != nil {
		t.Errorf("files = %v, want nil after a drop with nothing supported", v.state.files)
	}
	if v.img.Image != nil {
		t.Error("the previous image should be cleared, not left showing behind the error")
	}
	if v.img.Visible() {
		t.Error("the previous image should be hidden, not left showing behind the error")
	}
	if !v.dropzone.Visible() {
		t.Error("dropzone should be visible again after the error")
	}
	if !v.emptyStateArt.Visible() {
		t.Error("emptyStateArt should be shown in place of the cleared image")
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast to be shown for the bad drop")
	}
	settleToast(t, v)
}

func TestHandleDrop_FiltersUnsupportedFiles(t *testing.T) {
	v := newTestViewer(t)

	jpegURI := uitest.TempJPEGURI(t, "keep.jpg", 4, 4, color.White)

	v.handleDrop([]fyne.URI{
		jpegURI,
		uitest.FakeURI{FileName: "skip.txt", Ext: ".txt"},
	})
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if len(v.state.files) != 1 || v.state.files[0].Name() != jpegURI.Name() {
		t.Errorf("files = %v, want only %q kept", v.state.files, jpegURI.Name())
	}
}

func TestHandleDrop_AcceptsPNGAndGIF(t *testing.T) {
	v := newTestViewer(t)

	pngPath := uitest.WriteTempFile(t, "a.png", uitest.EncodePNG(t, 4, 4, color.White))
	gifPath := uitest.WriteTempFile(t, "b.gif", uitest.EncodeGIF(t, 4, 4, color.White))

	dropAndWait(t, v, storage.NewFileURI(pngPath), storage.NewFileURI(gifPath))

	if len(v.state.files) != 2 {
		t.Fatalf("files = %v, want both the PNG and the GIF kept", v.state.files)
	}
}

func TestHandleDrop_AcceptsRAW(t *testing.T) {
	v := newTestViewer(t)

	raw := uitest.TempRAWURI(t, "photo.cr2", 8, 8, color.White)
	dropAndWait(t, v, raw)

	if len(v.state.files) != 1 || v.state.files[0].Name() != "photo.cr2" {
		t.Errorf("files = %v, want the RAW file kept", v.state.files)
	}
	if v.img.Image == nil {
		t.Fatal("expected the embedded JPEG preview to be on screen")
	}
}

// --- handleDrop merge vs. replace (M toggles merge mode) --------------------

func TestHandleDrop_SecondDropWithoutMergeModeReplaces(t *testing.T) {
	v := newTestViewer(t)

	first := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, first)

	second := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	v.handleDrop([]fyne.URI{second}) // mergeMode defaults to false
	waitForScan(t, v)
	waitForSort(t, v)
	waitUntilLoaded(t, v)

	if len(v.state.files) != 1 || v.state.files[0].Name() != "b.jpg" {
		t.Errorf("files = %v, want only %q - the second drop should replace the first", v.state.files, "b.jpg")
	}
}

func TestHandleDrop_MergeModeMergesIntoExistingSet(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	v.state.SetMergeMode(true)
	dropAndWait(t, v, b)

	if len(v.state.files) != 2 {
		t.Fatalf("files = %v, want both a.jpg and b.jpg after a merge-mode drop", v.state.files)
	}

	// The merge should have jumped to the file just added, not stayed on a.jpg.
	if got := v.state.files[v.state.index].Name(); got != "b.jpg" {
		t.Errorf("displayed file = %q, want b.jpg (the just-merged file) in view", got)
	}
}

func TestHandleDrop_MergeModeDropWithNothingSupportedKeepsExistingSet(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	v.state.SetMergeMode(true)
	dropAndWaitScan(t, v, uitest.FakeURI{FileName: "notes.txt", Ext: ".txt"})

	if len(v.state.files) != 1 || v.state.files[0].Name() != "a.jpg" {
		t.Errorf("files = %v, want the existing a.jpg untouched by a merge-mode drop with nothing supported", v.state.files)
	}
	if v.img.Image == nil {
		t.Error("the existing image should stay displayed, not cleared, when a merge-mode drop finds nothing new")
	}
	if v.emptyStateArt.Visible() {
		t.Error("emptyStateArt should not appear - this isn't an empty-state error, just nothing to add")
	}
	if !v.toast.card.Visible() {
		t.Error("expected a toast explaining nothing supported was found")
	}
	settleToast(t, v)
}

func TestToggleMergeMode_PrefixesTitleAndPersistsAcrossDrops(t *testing.T) {
	v := newTestViewer(t)

	if title := v.win.Title(); strings.Contains(title, "[merge]") {
		t.Fatalf("title = %q, should not start prefixed before M is ever pressed", title)
	}

	// M works even with nothing loaded yet, and takes effect immediately.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyM})
	if title := v.win.Title(); !strings.HasPrefix(title, "[merge] ") {
		t.Fatalf("title = %q, want it prefixed with [merge] right after toggling M", title)
	}

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	dropAndWait(t, v, a)

	if title := v.win.Title(); !strings.HasPrefix(title, "[merge] ") {
		t.Errorf("title = %q, want the [merge] prefix to persist once files are loaded", title)
	}

	// M again turns it back off.
	v.handleKeyEvent(&fyne.KeyEvent{Name: fyne.KeyM})
	if title := v.win.Title(); strings.Contains(title, "[merge]") {
		t.Errorf("title = %q, want the [merge] prefix gone after toggling M again", title)
	}
}

// TestMergeModeGetterSetter is MergeMode/SetMergeMode - the settings
// window's binding - as opposed to toggleMergeMode's own M-key flip already
// covered above.
func TestMergeModeGetterSetter(t *testing.T) {
	v := newTestViewer(t)

	if v.MergeMode() {
		t.Fatal("MergeMode() = true, want false by default")
	}

	v.SetMergeMode(true)
	if !v.MergeMode() {
		t.Error("MergeMode() = false, want true after SetMergeMode(true)")
	}
	if title := v.win.Title(); !strings.HasPrefix(title, "[merge] ") {
		t.Errorf("title = %q, want it prefixed right after SetMergeMode(true)", title)
	}

	v.SetMergeMode(false)
	if v.MergeMode() {
		t.Error("MergeMode() = true, want false after SetMergeMode(false)")
	}
}

// --- directory recursion and the scan cap -----------------------------------

func TestHandleDrop_RecursesIntoNestedDirectories(t *testing.T) {
	v := newTestViewer(t)

	root := t.TempDir()
	for i := range 3 {
		dir := filepath.Join(root, fmt.Sprintf("sub%d", i))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		// Non-image clutter that a real photo folder always contains -
		// directories and files like these have no recognized extension,
		// so imaging.IsSupportedImage must not open them to find out.
		if err := os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("junk"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("photo%d.jpg", i)), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dropAndWait(t, v, storage.NewFileURI(root))

	if len(v.state.files) != 3 {
		t.Fatalf("files = %v, want the 3 nested photos, none of the .DS_Store junk", v.state.files)
	}

	if v.dropzone.Visible() {
		t.Error("dropzone (\"Drop images here\") should be hidden once an image is showing")
	}
}

// TestHandleDrop_SymlinkCycleDoesNotHang guards the visitedDirs check in
// handleDrop: a symlink back to an ancestor directory turns the recursive
// expansion into a cycle (listing root/loop lists root again, including
// root/loop, forever) unless each directory's real, symlink-resolved path is
// tracked and a repeat visit is skipped.
func TestHandleDrop_SymlinkCycleDoesNotHang(t *testing.T) {
	v := newTestViewer(t)

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "photo.jpg"), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Skipf("symlinks not supported on this filesystem: %v", err)
	}

	dropAndWait(t, v, storage.NewFileURI(root))

	if len(v.state.files) != 1 {
		t.Fatalf("files = %v, want the 1 real photo, not one entry per pass through the symlink cycle", v.state.files)
	}
}

// TestHandleDrop_CapsFileCountForLargeTrees exercises maxScan, the safety
// valve that stops a recursive scan once it's gathered enough images -
// shrunk here so the test doesn't need to create tens of thousands of temp
// files to hit it. A per-viewer field, so no global to save and restore.
func TestHandleDrop_CapsFileCountForLargeTrees(t *testing.T) {
	v := newTestViewer(t)
	v.settings.maxScan = 3

	root := t.TempDir()
	for i := range 5 {
		name := filepath.Join(root, fmt.Sprintf("photo%d.jpg", i))
		if err := os.WriteFile(name, uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	dropAndWait(t, v, storage.NewFileURI(root))

	if len(v.state.files) != 3 {
		t.Fatalf("files = %d, want the scan to stop at maxScan (3)", len(v.state.files))
	}

	if !v.toast.card.Visible() {
		t.Fatal("want a toast warning that the scan was truncated")
	}
	if !strings.Contains(v.toast.text.Text, "3") {
		t.Errorf("toast text = %q, want it to mention the cap (3)", v.toast.text.Text)
	}
	settleToast(t, v)
}

// TestMaxScanGetterSetter is MaxScan/SetMaxScan - the settings window's
// binding for the same v.settings.maxScan field
// TestHandleDrop_CapsFileCountForLargeTrees above exercises by writing it
// directly.
func TestMaxScanGetterSetter(t *testing.T) {
	v := newTestViewer(t)

	if got := v.MaxScan(); got != defaultMaxScannedFiles {
		t.Errorf("MaxScan() = %d, want the shipped default %d", got, defaultMaxScannedFiles)
	}

	v.SetMaxScan(5)
	if got := v.MaxScan(); got != 5 {
		t.Errorf("MaxScan() = %d, want 5 after SetMaxScan(5)", got)
	}
}

// TestSetMaxScan_FloorsAtOne guards the scan path's own n >= v.settings.maxScan
// check (drop.go): a 0 or negative cap would stop a scan before it ever
// gathered anything, which isn't what a settings-window typo should do.
func TestSetMaxScan_FloorsAtOne(t *testing.T) {
	v := newTestViewer(t)

	v.SetMaxScan(0)
	if got := v.MaxScan(); got != 1 {
		t.Errorf("MaxScan() = %d, want it floored to 1 for a 0 input", got)
	}

	v.SetMaxScan(-5)
	if got := v.MaxScan(); got != 1 {
		t.Errorf("MaxScan() = %d, want it floored to 1 for a negative input", got)
	}
}

// --- scan cancellation and dedupe -------------------------------------------

// TestCancelScan_NoOpWhenNotScanning covers the guard at the top of
// cancelScan: calling it with no scan in flight (the common case - Escape's
// other two branches, close/reset, are what normally run) must do nothing,
// not bump gen or raise a spurious "cancelled scanning" toast.
func TestCancelScan_NoOpWhenNotScanning(t *testing.T) {
	v := newTestViewer(t)
	revisionBefore := v.scanOp.lifecycle.currentRevision()

	v.cancelScan()

	if v.scanOp.lifecycle.currentRevision() != revisionBefore {
		t.Error("cancelScan should not invalidate the scan lifecycle when nothing is scanning")
	}
	if v.toast.card.Visible() {
		t.Error("cancelScan should not raise a toast when nothing is scanning")
	}
}

// TestCancelScan_CancelsInFlightScanWithNoFilesYet drives cancelScan
// directly against the UI state handleDrop leaves in place while its scan
// is still in flight (token started, spinner/counter shown, drop zone hidden),
// without racing handleDrop's own background goroutine to reproduce that
// state - see the note on TestHandleDrop_SupersededScanGoroutineExits below
// for why the goroutine itself is exercised separately instead.
func TestCancelScan_CancelsInFlightScanWithNoFilesYet(t *testing.T) {
	v := newTestViewer(t)

	token := v.scanOp.lifecycle.begin()
	v.scanOp.active = true
	v.scanOp.spinner.Show()
	v.scanOp.label.Show()
	v.dropzone.Hide()
	v.welcomeArt.Hide()

	v.cancelScan()

	if v.scanOp.active {
		t.Error("scanOp.active should be false after cancelScan")
	}
	if v.scanOp.spinner.Visible() || v.scanOp.label.Visible() {
		t.Error("scan spinner/label should be hidden after cancelScan")
	}
	if !v.dropzone.Visible() || !v.welcomeArt.Visible() {
		t.Error("drop zone/welcome art should be restored after cancelling a scan that had no files loaded yet")
	}
	if token.current() || token.context().Err() == nil {
		t.Error("cancelScan should cancel and supersede the in-flight scan token")
	}
	if !v.toast.card.Visible() {
		t.Error("want a toast confirming the scan was cancelled")
	}
	if !strings.Contains(v.toast.text.Text, "cancelled") {
		t.Errorf("toast text = %q, want it to mention the cancellation", v.toast.text.Text)
	}
	settleToast(t, v)
}

// TestCancelScan_PreservesExistingFilesInMergeMode checks that cancelling a
// merge-mode scan never touches files already loaded before that scan
// started - unlike reset (Escape with no scan running), which always clears
// back to the drop zone.
func TestCancelScan_PreservesExistingFilesInMergeMode(t *testing.T) {
	v := newTestViewer(t)

	existing := uitest.TempJPEGURI(t, "existing.jpg", 4, 4, color.White)
	v.state.files = []fyne.URI{existing}
	v.state.unsortedFiles = []fyne.URI{existing}
	v.dropzone.Hide()

	v.scanOp.active = true
	v.scanOp.spinner.Show()
	v.scanOp.label.Show()

	v.cancelScan()

	if len(v.state.files) != 1 || v.state.files[0].String() != existing.String() {
		t.Errorf("files = %v, want the pre-existing file untouched by cancelling a merge-mode scan", v.state.files)
	}
	if v.dropzone.Visible() {
		t.Error("drop zone should stay hidden - an image was already loaded before the cancelled scan started")
	}
	if v.scanOp.active {
		t.Error("scanOp.active should be false after cancelScan")
	}
	settleToast(t, v)
}

// TestClearToDropzone_FinishesInFlightScan is the scan-side of
// invalidateSort: returning to the empty drop zone must not leave
// scanOp.active set or the scan overlay showing. Escape cannot reach
// this (it cancelScan's first), and File > Close Files currently works
// around it the same way, but reset and ShowEmptyStateError both go
// through clearToDropzone with no such guard. The scan's own completion
// then finds its token stale and returns without cleaning up either -
// there is no newer scan to own the flag, unlike a superseded drop.
func TestClearToDropzone_FinishesInFlightScan(t *testing.T) {
	v := newTestViewer(t)

	token := v.scanOp.lifecycle.begin()
	v.scanOp.active = true
	v.scanOp.show()
	v.dropzone.Hide()
	v.welcomeArt.Hide()

	v.clearToDropzone()

	if v.scanOp.active {
		t.Error("scanOp.active should be false after clearToDropzone")
	}
	if v.scanOp.art.Visible() || v.scanOp.spinner.Visible() || v.scanOp.label.Visible() {
		t.Error("scan art/spinner/label should be hidden after clearToDropzone")
	}
	if !v.dropzone.Visible() {
		t.Error("dropzone should be visible after clearToDropzone")
	}
	if token.current() || token.context().Err() == nil {
		t.Error("clearToDropzone should cancel and supersede the in-flight scan token")
	}
}

// TestHandleDrop_SupersededScanGoroutineExits drops a folder large enough to
// force several storage.List round trips, then immediately drops a second,
// unrelated file before the first scan can finish. gen is bumped
// synchronously by the second handleDrop call, on this same goroutine,
// before the first scan's background goroutine has any chance to run -
// so by the time that goroutine makes its first post-bump gen check
// (whichever of the several in handleDrop it reaches first), it's
// already stale. This exercises the gen check inside the directory-walk
// loop (added so a superseded scan stops touching the filesystem instead
// of racing a large tree to completion for a discarded result) without
// depending on real-time scheduling to land the cancellation mid-scan.
func TestHandleDrop_SupersededScanGoroutineExits(t *testing.T) {
	v := newTestViewer(t)

	rootA := t.TempDir()
	for i := range 20 {
		sub := filepath.Join(rootA, fmt.Sprintf("d%d", i))
		if err := os.MkdirAll(sub, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(sub, "photo.jpg"), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	v.handleDrop([]fyne.URI{storage.NewFileURI(rootA)})
	scanDoneA := v.scanOp.done

	jpegB := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, jpegB)

	select {
	case <-scanDoneA:
	case <-time.After(5 * time.Second):
		t.Fatal("superseded scan's goroutine never exited - scanOp.done was never closed")
	}

	if len(v.state.files) != 1 || v.state.files[0].String() != jpegB.String() {
		t.Errorf("files = %v, want only the second drop's file applied", v.state.files)
	}
}

// TestNavigationDoesNotInvalidateScan pins the lifecycle split: a user may
// browse an existing set while a merge-mode directory scan is in flight, and
// that navigation must not silently strand scanOp.active=true or discard the scan.
func TestNavigationDoesNotInvalidateScan(t *testing.T) {
	v := newTestViewer(t)

	a := uitest.TempJPEGURI(t, "a.jpg", 4, 4, color.White)
	b := uitest.TempJPEGURI(t, "b.jpg", 4, 4, color.White)
	dropAndWait(t, v, a, b)

	scanToken := v.scanOp.lifecycle.begin()
	v.scanOp.active = true

	v.ShowImage(1)
	waitUntilLoaded(t, v)

	if !scanToken.current() {
		t.Fatal("navigation invalidated an unrelated in-flight scan")
	}
	if !v.scanOp.active {
		t.Fatal("navigation cleared scanning before the scan completed")
	}

	v.cancelScan()
	settleToast(t, v)
}

// TestHandleDrop_DedupesOverlappingDirectories drops a folder together with
// one of its own subfolders in the same call - a folder tree reached via two
// different dropped paths - and checks the subfolder's photo isn't counted
// twice in the resulting set.
func TestHandleDrop_DedupesOverlappingDirectories(t *testing.T) {
	v := newTestViewer(t)

	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "top.jpg"), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "nested.jpg"), uitest.EncodeJPEG(t, 4, 4, color.White), 0o644); err != nil {
		t.Fatal(err)
	}

	dropAndWait(t, v, storage.NewFileURI(root), storage.NewFileURI(sub))

	if len(v.state.files) != 2 {
		t.Fatalf("files = %v, want top.jpg and nested.jpg once each, not nested.jpg twice from the overlapping drop", v.state.files)
	}
}

// TestHandleDrop_DedupesDuplicateURIsInDirectDrop covers the fast (no
// directories) path in handleDrop: passing the same file twice in one drop -
// which os.Args launch or a native chooser's output could in principle
// produce - should not add it to v.state.files twice.
func TestHandleDrop_DedupesDuplicateURIsInDirectDrop(t *testing.T) {
	v := newTestViewer(t)

	photo := uitest.TempJPEGURI(t, "photo.jpg", 4, 4, color.White)

	dropAndWait(t, v, photo, photo)

	if len(v.state.files) != 1 {
		t.Fatalf("files = %v, want the duplicate URI collapsed to a single entry", v.state.files)
	}
}
