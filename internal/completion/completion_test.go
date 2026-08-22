package completion_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/frathe/picfetch/internal/completion"
)

// waitTimeout is the deadline every wait in this file gets. A passing test
// returns as soon as its channel closes and never waits this long.
const waitTimeout = 2 * time.Second

func waitCtx(t *testing.T) context.Context {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	t.Cleanup(cancel)

	return ctx
}

// A Signal nobody has begun has nothing to wait for, so Wait returns
// immediately - the behavior drain's "skip the nil channel" relies on.
func TestSignal_WaitBeforeBeginReturnsImmediately(t *testing.T) {
	var s completion.Signal

	if err := s.Wait(waitCtx(t)); err != nil {
		t.Fatalf("Wait on a never-begun Signal = %v, want nil", err)
	}
	if s.Begun() {
		t.Error("Begun on a never-begun Signal = true, want false")
	}
}

func TestSignal_WaitBlocksUntilDone(t *testing.T) {
	var s completion.Signal

	done := s.Begin()
	if !s.Begun() {
		t.Fatal("Begun after Begin = false, want true")
	}

	released := make(chan error, 1)
	go func() { released <- s.Wait(waitCtx(t)) }()

	select {
	case err := <-released:
		t.Fatalf("Wait returned %v before done was called, want it to block", err)
	case <-time.After(20 * time.Millisecond):
	}

	done()

	select {
	case err := <-released:
		if err != nil {
			t.Fatalf("Wait after done = %v, want nil", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Wait never returned after done was called")
	}
}

func TestSignal_WaitRespectsContextCancellation(t *testing.T) {
	var s completion.Signal

	s.Begin() // deliberately never called: the operation never finishes

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := s.Wait(ctx); err == nil {
		t.Fatal("Wait with a cancelled context = nil, want the context's error")
	}
}

// The rule the whole type exists for: a superseded generation still closes
// its own channel, and doing so must not release a waiter on the newer one.
func TestSignal_SupersededGenerationClosesItsOwnChannel(t *testing.T) {
	var s completion.Signal

	stale := s.Begin()
	fresh := s.Begin()

	released := make(chan error, 1)
	go func() { released <- s.Wait(waitCtx(t)) }()

	stale()

	select {
	case err := <-released:
		t.Fatalf("the stale generation's done released a waiter on the fresh one (err = %v)", err)
	case <-time.After(20 * time.Millisecond):
	}

	fresh()

	select {
	case err := <-released:
		if err != nil {
			t.Fatalf("Wait after the fresh done = %v, want nil", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Wait never returned after the fresh generation finished")
	}
}

// A Handle keeps naming the generation it was taken from, so a caller can
// still wait out a request that has since been superseded. This is what
// drop_test.go's TestHandleDrop_SupersededScanGoroutineExits needs: it
// waits for the *first* scan's goroutine to exit after a second drop has
// already replaced the generation.
func TestHandle_WaitsItsOwnGenerationAfterSupersession(t *testing.T) {
	var s completion.Signal

	stale := s.Begin()
	staleHandle := s.Current()

	fresh := s.Begin()

	released := make(chan error, 1)
	go func() { released <- staleHandle.Wait(waitCtx(t)) }()

	// Finishing the *newer* generation must not release a waiter holding a
	// handle on the older one.
	fresh()

	select {
	case err := <-released:
		t.Fatalf("the fresh generation released a waiter on the stale handle (err = %v)", err)
	case <-time.After(20 * time.Millisecond):
	}

	stale()

	select {
	case err := <-released:
		if err != nil {
			t.Fatalf("Handle.Wait after its own done = %v, want nil", err)
		}
	case <-time.After(waitTimeout):
		t.Fatal("Handle.Wait never returned after its own generation finished")
	}
}

// The zero Handle - taken from a Signal nobody has begun - has nothing to
// wait for, matching Signal.Wait's own never-begun case.
func TestHandle_ZeroValueWaitsForNothing(t *testing.T) {
	var s completion.Signal

	if err := s.Current().Wait(waitCtx(t)); err != nil {
		t.Fatalf("Wait on a never-begun Signal's handle = %v, want nil", err)
	}
}

// Idempotent, so a retry chain that can reach its finish twice reports
// completion instead of panicking on a second close.
func TestSignal_DoneIsIdempotent(t *testing.T) {
	var s completion.Signal

	done := s.Begin()
	done()
	done()
	done()

	if err := s.Wait(waitCtx(t)); err != nil {
		t.Fatalf("Wait after a repeated done = %v, want nil", err)
	}

	// Begun answers "did this operation ever start", not "is it still
	// running" - it must stay true after done, not flip back to false.
	if !s.Begun() {
		t.Error("Begun after done = false, want true: Begun is ever-started, not still-running")
	}
}

// Begin/Wait/Begun from several goroutines at once must be race-free: this
// is the hazard openfiles.go:42 documents, and the reason Signal is
// mutex-guarded rather than a plain field. Each goroutine finishes its own
// generation before waiting: nobody may Wait on a generation whose finisher
// they are themselves holding, since whichever goroutine begins the last
// generation would then be the only one able to release its own Wait.
func TestSignal_ConcurrentBeginWaitAndBegun(t *testing.T) {
	var s completion.Signal

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			done := s.Begin()
			_ = s.Begun()
			done()

			if err := s.Wait(waitCtx(t)); err != nil {
				t.Errorf("Wait blocked: %v", err)
			}
		})
	}
	wg.Wait()

	// Whichever generation ended up current, its done was called by the
	// goroutine that began it, so a final Wait must not block.
	if err := s.Wait(waitCtx(t)); err != nil {
		t.Fatalf("Wait after the concurrent burst = %v, want nil", err)
	}
}
