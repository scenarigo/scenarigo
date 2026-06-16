// Package grpcstream provides concurrency helpers shared by the gRPC request
// clients and the mock gRPC server for handling streaming messages.
//
// Bidirectional streaming lets a request message template reference a response
// (response.messages[N]) and a response template reference a request
// (request.messages[N]). Resolving such a reference must block until the Nth
// message has been received by a background goroutine. Buffer implements that
// blocking, context-aware accumulation so the request side and the mock server
// do not each reimplement the same tricky synchronization.
package grpcstream

import (
	"context"
	"sync"
	"time"
)

// DefaultMessageWaitTimeout bounds how long a blocking message reference waits
// before failing. Without it, a scenario whose message depends on a message the
// other side only produces in response would deadlock and hang forever when no
// deadline is configured.
//
// It is a variable (not a constant) so that tests can shorten it.
var DefaultMessageWaitTimeout = 30 * time.Second

// newWaitContext derives a context used to bound a blocking wait on a streaming
// message. It respects an existing deadline (e.g. a step timeout or a deadline
// propagated over gRPC) and otherwise applies DefaultMessageWaitTimeout as a
// deadlock guard.
func newWaitContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if _, ok := ctx.Deadline(); ok {
		return context.WithCancel(ctx)
	}
	return context.WithTimeout(ctx, DefaultMessageWaitTimeout)
}

// Buffer is a concurrent, append-only list of streaming messages with blocking
// indexed access. A producer goroutine calls Append as messages arrive and
// Close when no more will arrive; consumers call At with the context of the
// caller that is currently evaluating a template, so each wait is bounded by
// that context (and a deadlock guard) and fails instead of hanging forever.
//
// The zero value is not usable; create one with NewBuffer.
type Buffer[T any] struct {
	mu          sync.Mutex
	cond        *sync.Cond
	items       []T
	done        bool
	lastWaitErr error
}

// NewBuffer creates an empty Buffer.
func NewBuffer[T any]() *Buffer[T] {
	b := &Buffer[T]{}
	b.cond = sync.NewCond(&b.mu)
	return b
}

// Append adds a message and wakes any blocked consumers.
func (b *Buffer[T]) Append(item T) {
	b.mu.Lock()
	b.items = append(b.items, item)
	b.cond.Broadcast()
	b.mu.Unlock()
}

// Close marks that no more messages will arrive and wakes any blocked consumers.
func (b *Buffer[T]) Close() {
	b.mu.Lock()
	b.done = true
	b.cond.Broadcast()
	b.mu.Unlock()
}

// At blocks until the i-th message is available and returns it. It returns false
// if the buffer is closed, ctx is canceled, or the deadlock guard fires before
// the message arrives; in those cases WaitErr reports the cause.
func (b *Buffer[T]) At(ctx context.Context, i int) (T, bool) {
	waitCtx, cancel := newWaitContext(ctx)
	defer cancel()

	// Wake this waiter when its context is done so it re-checks and stops waiting.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-waitCtx.Done():
			b.mu.Lock()
			b.cond.Broadcast()
			b.mu.Unlock()
		case <-stop:
		}
	}()

	b.mu.Lock()
	defer b.mu.Unlock()
	for {
		if i < len(b.items) {
			return b.items[i], true
		}
		if b.done {
			// The stream ended. Record a context-driven end so callers can tell a
			// deadline/cancel from a normal close: when a deadline cancels the
			// stream, the producer may Close the buffer before waitCtx's
			// cancellation has propagated, but ctx's error is set before its Done
			// channel closes, so it is observable here.
			if err := ctx.Err(); err != nil {
				b.lastWaitErr = err
			}
			var zero T
			return zero, false
		}
		if err := waitCtx.Err(); err != nil {
			b.lastWaitErr = err
			var zero T
			return zero, false
		}
		b.cond.Wait()
	}
}

// Snapshot returns the messages received so far without blocking. It is used to
// dump partial results when a stream is aborted.
func (b *Buffer[T]) Snapshot() []T {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.items
}

// WaitErr reports whether the most recent blocking wait was cut short by
// cancellation or the deadlock guard timeout. It is non-nil only when an At call
// returned false because of the context rather than a normal stream close.
func (b *Buffer[T]) WaitErr() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.lastWaitErr
}
