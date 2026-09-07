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
	"errors"
	"sync"
	"time"
)

// ErrClosed reports that the stream ended before the requested message
// arrived, so the message is definitively absent.
var ErrClosed = errors.New("stream closed")

// DefaultMessageWaitTimeout bounds how long a blocking message reference may
// wait. Callers that evaluate message templates apply it as a deadline on the
// evaluation context when no deadline is configured; without it, a scenario
// whose message depends on a message the other side only produces in response
// would deadlock and hang forever. Putting the guard on the context (rather
// than inside Buffer) lets the template engine distinguish a timed-out wait
// (ctx ended: an error) from a definitive absence (stream ended: undefined).
//
// It is a variable (not a constant) so that tests can shorten it.
var DefaultMessageWaitTimeout = 30 * time.Second

// Buffer is a concurrent, append-only list of streaming messages with blocking
// indexed access. A producer goroutine calls Append as messages arrive and
// Close when no more will arrive; consumers call At with the context of the
// caller that is currently evaluating a template, so each wait is bounded by
// that context and fails instead of hanging forever.
//
// The zero value is not usable; create one with NewBuffer.
type Buffer[T any] struct {
	mu      sync.Mutex
	cond    *sync.Cond
	items   []T
	done    bool
	waiters int // number of consumers currently parked in cond.Wait
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

// At returns the i-th message, blocking until it arrives. A negative index
// counts from the end, which the stream only has once it has ended, so it
// blocks until then. It returns ErrClosed when the stream ends before the
// message arrives, and the context error when ctx ends first, so callers can
// tell a definitive absence from an interrupted wait.
func (b *Buffer[T]) At(ctx context.Context, i int) (T, error) {
	// Wake this waiter when its context is done so it re-checks and stops waiting.
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		select {
		case <-ctx.Done():
			b.mu.Lock()
			b.cond.Broadcast()
			b.mu.Unlock()
		case <-stop:
		}
	}()

	b.mu.Lock()
	defer b.mu.Unlock()
	for {
		if i >= 0 {
			if i < len(b.items) {
				return b.items[i], nil
			}
		} else if b.done {
			// Which message an index counted from the end names is settled
			// only once no more can arrive.
			if n := len(b.items) + i; n >= 0 {
				return b.items[n], nil
			}
		}
		if b.done {
			var zero T
			return zero, ErrClosed
		}
		if err := ctx.Err(); err != nil {
			var zero T
			return zero, err
		}
		b.waiters++
		b.cond.Wait()
		b.waiters--
	}
}

// Done reports whether the stream has ended, i.e. Close has been called and no
// more messages will arrive.
func (b *Buffer[T]) Done() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.done
}

// numWaiters reports how many consumers are currently parked in cond.Wait. It
// exists so tests can deterministically wait for a consumer to block before
// signaling it, instead of relying on timing.
func (b *Buffer[T]) numWaiters() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.waiters
}

// Snapshot returns a copy of the messages received so far without blocking. It
// is used to dump partial results when a stream is aborted. A copy is returned
// (rather than the backing slice) so callers can safely read it while the
// producer goroutine keeps appending after an early, mid-stream return.
func (b *Buffer[T]) Snapshot() []T {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]T, len(b.items))
	copy(out, b.items)
	return out
}
