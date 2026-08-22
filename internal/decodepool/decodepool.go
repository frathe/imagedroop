// Package decodepool bounds background decode work. It is the semaphore +
// per-key in-flight claim + completion counter that internal/ui/grid's
// thumbnails and internal/ui's speculative preload each grew independently:
// a small worker pool, a gate against spawning a second goroutine for work
// already underway, and a WaitGroup a test can wait out so no goroutine
// outlives the test that started it.
//
// It is deliberately viewer-independent: no Fyne types, no UI marshaling. The
// caller decides what a key is, what staleness means, and when to release.
package decodepool

import (
	"context"
	"sync"
)

// Pool bounds concurrent work over keys of type K, each claim carrying a
// value of type V that says what the in-flight work is for. The zero Pool is
// not usable; call New.
type Pool[K, V comparable] struct {
	sem      chan struct{}
	inflight sync.Map
	pending  sync.WaitGroup
}

// New returns a pool that runs at most limit functions at once. limit must be
// positive.
func New[K, V comparable](limit int) *Pool[K, V] {
	if limit <= 0 {
		panic("decodepool: limit must be positive")
	}
	return &Pool[K, V]{sem: make(chan struct{}, limit)}
}

// Claim records that work is being spawned for key toward v, reporting
// whether the caller should actually spawn it. False means identical work is
// already in flight. A claim for the same key with a *different* v succeeds
// and overwrites: the caller has moved on to different work for that key, and
// the superseded worker's Release will not clobber the new claim (see
// Release).
func (p *Pool[K, V]) Claim(key K, v V) bool {
	if existing, ok := p.inflight.Load(key); ok && existing == v {
		return false
	}
	p.inflight.Store(key, v)
	return true
}

// Release drops key's claim, but only if it still names v - so a superseded
// worker finishing late cannot drop a newer claim made over it.
func (p *Pool[K, V]) Release(key K, v V) {
	p.inflight.CompareAndDelete(key, v)
}

// Go spawns fn on its own goroutine, which first waits for a free slot in the
// pool. acquired is false when ctx was cancelled while fn was still queued;
// fn is called either way, so a caller can always undo its Claim, but must
// not start work when acquired is false. The slot is held for the whole of
// fn and released when it returns.
//
// Wait returns only after every fn spawned so far has returned.
func (p *Pool[K, V]) Go(ctx context.Context, fn func(acquired bool)) {
	p.pending.Go(func() {
		// The wait for a slot happens here, on the spawned goroutine, not in
		// the caller's - the shape both callers already had before this type
		// existed, and the reason Go never blocks whoever calls it. A caller
		// on the UI goroutine (the grid's cell-update pass) therefore cannot
		// stall behind a full pool.
		//
		// With a free slot and an already-cancelled ctx both ready, select
		// picks between them at random, exactly as the hand-rolled version in
		// internal/ui's preloadOne did; callers re-check staleness after this
		// returns rather than relying on which one won.
		acquired := false
		select {
		case p.sem <- struct{}{}:
			acquired = true
		case <-ctx.Done():
		}
		if acquired {
			defer func() { <-p.sem }()
		}

		fn(acquired)
	})
}

// Wait blocks until every function spawned so far has returned. The
// application never needs this; tests do.
func (p *Pool[K, V]) Wait() {
	p.pending.Wait()
}
