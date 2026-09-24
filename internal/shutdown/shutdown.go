package shutdown

import (
	"context"
	"sync"
)

var (
	mu    sync.Mutex
	fired bool
	done  = make(chan struct{})
)

// Done returns a channel that is closed when shutdown begins.
// In-flight work (SSE streams, upstream readers) selects on it to stop promptly
// instead of holding server.Shutdown until its deadline.
func Done() <-chan struct{} {
	mu.Lock()
	defer mu.Unlock()
	return done
}

// Fired reports whether shutdown has been triggered.
func Fired() bool {
	mu.Lock()
	defer mu.Unlock()
	return fired
}

// ctxMu guards ctx (recreated on TestReset).
var (
	ctxMu        sync.Mutex
	ctx, ctxStop = context.WithCancel(context.Background())
)

// Context returns a context canceled when shutdown begins. Background loops
// (updater, catalog sync) take this instead of context.Background() so they
// exit promptly on ^C instead of leaking goroutines + tickers.
func Context() context.Context {
	ctxMu.Lock()
	defer ctxMu.Unlock()
	return ctx
}

// TestReset restores the package for tests (uncancel + unfire).
func TestReset() {
	mu.Lock()
	fired = false
	done = make(chan struct{})
	mu.Unlock()
	ctxMu.Lock()
	ctx, ctxStop = context.WithCancel(context.Background())
	ctxMu.Unlock()
}

// Cancel triggers shutdown, closing the Done channel. Safe to call multiple times.
func Cancel() {
	mu.Lock()
	defer mu.Unlock()
	if fired {
		return
	}
	fired = true
	close(done)
	ctxMu.Lock()
	defer ctxMu.Unlock()
	ctxStop()
}
