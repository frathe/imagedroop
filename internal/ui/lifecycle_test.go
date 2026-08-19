package ui

import (
	"sync"
	"testing"
)

func TestRevisionZeroValueAdvancesAndMatches(t *testing.T) {
	var r revision

	if got := r.current(); got != 0 {
		t.Fatalf("initial revision = %d, want 0", got)
	}
	first := r.advance()
	if first != 1 || !r.matches(first) {
		t.Fatalf("after advance: revision = %d, matches = %t; want 1, true", first, r.matches(first))
	}
	if r.matches(0) {
		t.Fatal("the zero revision must be stale after an advance")
	}
}

func TestRequestLifecycleBeginCancelsAndSupersedesPrevious(t *testing.T) {
	var lifecycle requestLifecycle

	first := lifecycle.begin()
	second := lifecycle.begin()

	if first.context().Err() == nil {
		t.Fatal("begin should cancel the previous token's context")
	}
	if first.current() {
		t.Fatal("the previous token must be stale after begin")
	}
	if !second.current() {
		t.Fatal("the newly begun token should be current")
	}
	if second.revision != first.revision+1 {
		t.Fatalf("revisions = %d then %d, want consecutive values", first.revision, second.revision)
	}
}

func TestRequestLifecycleInvalidateCancelsWithoutReplacement(t *testing.T) {
	var lifecycle requestLifecycle

	beforeFirst := lifecycle.invalidate()
	if beforeFirst != 1 {
		t.Fatalf("first invalidate revision = %d, want 1", beforeFirst)
	}

	token := lifecycle.begin()
	invalidated := lifecycle.invalidate()
	if token.context().Err() == nil || token.current() {
		t.Fatal("invalidate should cancel and stale the current token")
	}
	if invalidated != token.revision+1 {
		t.Fatalf("invalidate revision = %d, want %d", invalidated, token.revision+1)
	}
}

func TestRequestTokenCancelDoesNotSupersedeNewerRequest(t *testing.T) {
	var lifecycle requestLifecycle

	first := lifecycle.begin()
	second := lifecycle.begin()
	first.cancelContext()

	if !second.current() {
		t.Fatal("cancelling an older token must not supersede the newer request")
	}
	if got := lifecycle.currentRevision(); got != second.revision {
		t.Fatalf("current revision = %d, want %d", got, second.revision)
	}

	second.cancelContext()
	if second.current() {
		t.Fatal("a token whose own context was cancelled must not be current")
	}
	if got := lifecycle.currentRevision(); got != second.revision {
		t.Fatalf("local cancellation advanced revision to %d, want it unchanged at %d", got, second.revision)
	}
}

func TestRequestLifecycleConcurrentBeginLeavesOneCurrentToken(t *testing.T) {
	var lifecycle requestLifecycle
	const count = 32

	tokens := make([]requestToken, count)
	var wg sync.WaitGroup
	for i := range tokens {
		wg.Go(func() {
			tokens[i] = lifecycle.begin()
		})
	}
	wg.Wait()

	current := 0
	for _, token := range tokens {
		if token.current() {
			current++
		}
	}
	if current != 1 {
		t.Fatalf("current tokens = %d, want exactly 1", current)
	}
}
