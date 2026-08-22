// Package completion is the one-shot "this background operation has
// finished" signal that the viewer grew nine hand-rolled copies of: a
// channel replaced at the start of each request and closed when that
// request finishes, which the test suite waits on instead of polling
// widget state a producer goroutine may still be writing.
//
// The rule it makes unbreakable is the one those nine copies could only
// state in prose: a request that has been superseded must still close its
// own channel, without touching the field a newer request now owns. Begin
// hands back a func closed over this generation's channel, so a stale
// producer cannot reach the newer one even by accident.
//
// It is deliberately viewer-independent: no Fyne types, no fyne.Do, no UI
// marshaling. The caller decides what counts as stale and what finishing
// means; Signal answers only "has the generation I am looking at finished
// yet".
package completion

import (
	"context"
	"sync"
)

// Signal is a replaceable one-shot completion signal: begun by one
// generation at a time, finished when that generation's work is done. The
// zero Signal is ready to use and reports Begun() == false.
//
// Safe for concurrent use. That matters more than it looks: the fields
// this type replaces were written by background goroutines and read by the
// test goroutine with nothing synchronizing the two, which is the hazard
// openfiles.go's runFileChooser was split out to dodge.
type Signal struct {
	mu   sync.Mutex
	done chan struct{}
}

// Begin supersedes any generation already in flight and returns the
// function that finishes *this* one. Call it exactly where the old code
// did `defer close(done)`.
//
// The returned func is idempotent: calling it twice is a no-op rather than
// the panic a repeated close(chan) would be, so a retry chain that can
// reach its finish along two paths stays correct.
//
// Deliberately no way to get the channel itself: the whole point is that a
// superseded producer holds a closer over its own generation and nothing
// else.
func (s *Signal) Begin() (done func()) {
	ch := make(chan struct{})

	s.mu.Lock()
	s.done = ch
	s.mu.Unlock()

	var once sync.Once

	return func() { once.Do(func() { close(ch) }) }
}

// Current returns a Handle naming the generation in flight right now, so a
// caller can wait out *that* request even after a newer one has superseded
// it. Taken from a Signal that has never begun, the zero Handle waits for
// nothing.
//
// This exists for one real case: a test that starts a request, starts a
// second one that replaces it, and then wants to prove the first one's
// goroutine actually exited. Waiting on the Signal itself would wait on
// the second generation and prove nothing.
func (s *Signal) Current() Handle {
	s.mu.Lock()
	defer s.mu.Unlock()

	return Handle{ch: s.done}
}

// Handle names one generation of a Signal. It keeps naming that generation
// for good, which is what separates it from Signal.Wait: the Signal moves
// on, a Handle does not. The zero Handle has nothing to wait for.
type Handle struct {
	ch chan struct{}
}

// Wait blocks until this handle's generation finishes or ctx is done,
// returning ctx.Err() only in the latter case. The zero Handle returns nil
// immediately.
func (h Handle) Wait(ctx context.Context) error {
	if h.ch == nil {
		return nil
	}

	select {
	case <-h.ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Wait blocks until the generation current *at the moment Wait is called*
// has finished, or until ctx is done, whichever happens first. It returns
// ctx.Err() only in the latter case.
//
// A Signal that has never begun has nothing to wait for and returns nil
// immediately - the "a viewer that never scanned has nothing to drain"
// case, which callers would otherwise each have to special-case.
//
// The channel is snapshotted under the lock and waited on outside it, so a
// waiter never blocks a producer's Begin. A generation that starts after
// this snapshot is not waited for, exactly as reading the old channel
// field once and selecting on it behaved. A caller that needs to keep
// waiting on a specific generation across a supersession takes a Handle
// instead.
//
// The application never needs this; tests do.
func (s *Signal) Wait(ctx context.Context) error {
	return s.Current().Wait(ctx)
}

// Begun reports whether Begin has ever been called - "did this operation
// ever start", not "is it still running". It replaces the `!= nil` checks
// tests used to make against the raw channel fields, and is monotonic for
// the same reason those were: nothing ever puts a Signal back to its zero
// state.
func (s *Signal) Begun() bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.done != nil
}
