package decodepool

import (
	"context"
	"sync/atomic"
	"testing"
)

func TestClaim_SecondClaimForSameValueIsRefused(t *testing.T) {
	p := New[string, int](1)
	if !p.Claim("a", 1) {
		t.Fatal("first claim refused")
	}
	if p.Claim("a", 1) {
		t.Fatal("duplicate claim accepted")
	}
}

func TestClaim_DifferentValueOverwrites(t *testing.T) {
	p := New[string, int](1)
	p.Claim("a", 1)
	if !p.Claim("a", 2) {
		t.Fatal("claim for a different value refused")
	}
	// The superseded worker's release must not drop the newer claim.
	p.Release("a", 1)
	if p.Claim("a", 2) {
		t.Fatal("stale release dropped the newer claim")
	}
}

func TestClaim_StructValueBehavesLikeLoadOrStore(t *testing.T) {
	p := New[string, struct{}](1)
	if !p.Claim("a", struct{}{}) {
		t.Fatal("first claim refused")
	}
	if p.Claim("a", struct{}{}) {
		t.Fatal("duplicate claim accepted")
	}
	p.Release("a", struct{}{})
	if !p.Claim("a", struct{}{}) {
		t.Fatal("claim after release refused")
	}
}

func TestGo_LimitBoundsConcurrency(t *testing.T) {
	p := New[int, int](2)
	var inFlight, peak atomic.Int64
	release := make(chan struct{})
	for range 8 {
		p.Go(context.Background(), func(acquired bool) {
			if !acquired {
				t.Error("acquired false with an uncancelled context")
				return
			}
			n := inFlight.Add(1)
			for {
				old := peak.Load()
				if n <= old || peak.CompareAndSwap(old, n) {
					break
				}
			}
			<-release
			inFlight.Add(-1)
		})
	}
	close(release)
	p.Wait()
	if peak.Load() > 2 {
		t.Fatalf("peak concurrency %d, want <= 2", peak.Load())
	}
}

func TestGo_CancelledWhileQueuedStillCallsFn(t *testing.T) {
	p := New[int, int](1)
	holding := make(chan struct{})
	block := make(chan struct{})
	p.Go(context.Background(), func(bool) {
		close(holding)
		<-block
	})

	// The pool's one slot has to be genuinely taken before the cancelled call
	// queues behind it: Go does its waiting on the spawned goroutine, so
	// without this handshake that goroutine might not have reached its select
	// yet and the cancelled call would find the slot free.
	<-holding

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var got atomic.Bool
	var sawAcquired atomic.Bool
	resolved := make(chan struct{})
	p.Go(ctx, func(acquired bool) {
		got.Store(true)
		sawAcquired.Store(acquired)
		close(resolved)
	})

	// And it has to resolve while the slot is still held. Releasing first
	// would leave its select with two ready cases - the freed slot and the
	// cancelled ctx - which select picks between at random.
	<-resolved

	close(block)
	p.Wait()
	if !got.Load() {
		t.Fatal("fn was not called for a queue-cancelled request")
	}
	if sawAcquired.Load() {
		t.Fatal("acquired was true for a queue-cancelled request")
	}
}

func TestWait_ReturnsOnlyAfterEveryFnReturns(t *testing.T) {
	p := New[int, int](4)
	var done atomic.Int64
	block := make(chan struct{})
	for range 4 {
		p.Go(context.Background(), func(bool) {
			<-block
			done.Add(1)
		})
	}
	close(block)
	p.Wait()
	if done.Load() != 4 {
		t.Fatalf("Wait returned with %d of 4 done", done.Load())
	}
}
