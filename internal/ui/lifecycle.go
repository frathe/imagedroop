package ui

import (
	"context"
	"sync"
	"sync/atomic"
)

// revision is a monotonically increasing identity for state observed by
// background work. Its zero value is ready to use.
type revision struct {
	value atomic.Uint64
}

func (r *revision) advance() uint64 {
	return r.value.Add(1)
}

func (r *revision) current() uint64 {
	return r.value.Load()
}

func (r *revision) matches(value uint64) bool {
	return r.current() == value
}

// requestLifecycle owns the cancellation and revision of one logical class
// of work. Starting or invalidating a request permanently supersedes the
// previous token. Its zero value is ready to use.
type requestLifecycle struct {
	revision revision

	mu     sync.Mutex
	cancel context.CancelFunc
}

// begin supersedes the previous request and returns the token for the new
// one. Descendant work should share this token rather than begin another
// request of its own.
func (l *requestLifecycle) begin() requestToken {
	ctx, cancel := context.WithCancel(context.Background())

	l.mu.Lock()
	if l.cancel != nil {
		l.cancel()
	}
	value := l.revision.advance()
	l.cancel = cancel
	l.mu.Unlock()

	return requestToken{
		ctx:       ctx,
		cancel:    cancel,
		lifecycle: l,
		revision:  value,
	}
}

// invalidate supersedes and cancels the current request without starting a
// replacement. It is safe before the first begin and safe to repeat.
func (l *requestLifecycle) invalidate() uint64 {
	l.mu.Lock()
	value := l.revision.advance()
	if l.cancel != nil {
		l.cancel()
		l.cancel = nil
	}
	l.mu.Unlock()

	return value
}

func (l *requestLifecycle) currentRevision() uint64 {
	return l.revision.current()
}

// requestToken is the immutable identity and context captured by one logical
// request. cancel only releases this token's context; it does not advance the
// lifecycle revision and therefore cannot supersede a newer request.
type requestToken struct {
	ctx       context.Context
	cancel    context.CancelFunc
	lifecycle *requestLifecycle
	revision  uint64
}

func (t requestToken) context() context.Context {
	return t.ctx
}

func (t requestToken) current() bool {
	return t.lifecycle != nil && t.ctx != nil && t.ctx.Err() == nil && t.lifecycle.revision.matches(t.revision)
}

func (t requestToken) cancelContext() {
	if t.cancel != nil {
		t.cancel()
	}
}
