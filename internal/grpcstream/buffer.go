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
	"io"
	"sync"
	"time"

	"google.golang.org/grpc/status"
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

// End reports why a receive stopped the stream, in the form Close takes: nil
// when the stream ended, and the error when it stopped in a way that is not an
// ending. streamCtx is the context the receive ran under.
//
// io.EOF is the OK ending, and a gRPC status is an ending too - including a
// non-OK one, which the scenario asserts on through expect.status.code, the
// same line protocol/grpc's openFailure draws. A message the stream ended
// before producing is absent; only an error that is neither leaves it
// unobtainable.
//
// Unless streamCtx ended first, in which case the error is kept whatever it
// says. gRPC synthesises a status for a context that ended on this side - a
// DeadlineExceeded that does not wrap context.DeadlineExceeded - so the error
// alone cannot tell whose ending it is. Our own deadline is what bounds a
// blocking message reference, and calling it an ending would turn the message
// it was waiting for into an absence that ?? and defined() absorb, hiding the
// very timeout the guard exists to report.
//
// That keeps a status the peer really did send at the same moment, which
// cannot be told apart from the synthesised one. Deliberately: the waiter is
// racing its own context here, and the branch where it notices that first
// reports the interrupted wait. Ending the stream on the other branch would
// make a scenario absorb or report the same timeout depending on who won.
func End(streamCtx context.Context, err error) error {
	if err == nil || errors.Is(err, io.EOF) {
		return nil
	}
	if streamCtx.Err() != nil {
		return err
	}
	if _, ok := status.FromError(err); ok {
		return nil
	}
	return err
}

// Buffer is a concurrent, append-only list of streaming messages with blocking
// indexed access. A producer goroutine calls Append as messages arrive and
// Close when no more will arrive; consumers call At with the context of the
// caller that is currently evaluating a template, so each wait is bounded by
// that context and fails instead of hanging forever.
//
// The zero value is not usable; create one with NewBuffer.
type Buffer[T any] struct {
	mu    sync.Mutex
	cond  *sync.Cond
	items []T
	done  bool
	// closeErr is why the stream stopped, when it did not simply finish.
	closeErr error
	waiters  int // number of consumers currently parked in cond.Wait
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

// Close ends the stream and wakes any blocked consumers. err is nil for a
// stream that ended, however it ended, and non-nil only for one that stopped
// in a way the caller cannot make sense of. A message that never arrived is
// absent in the first case and unobtainable in the second.
//
// Only the caller can tell those apart, because what counts as an ending is
// the protocol's business: a gRPC stream that ends with a non-OK status has
// ended, and the status is something the scenario asserts on.
func (b *Buffer[T]) Close(err error) {
	b.mu.Lock()
	b.done = true
	b.closeErr = err
	b.cond.Broadcast()
	b.mu.Unlock()
}

// At returns the i-th message, blocking until it arrives. A negative index
// counts from the end, which the stream only has once it has ended, so it
// blocks until then.
//
// A message it cannot return has one of three fates, which callers must tell
// apart: ErrClosed when the stream finished without it, so it is absent; the
// reason the stream stopped when it did not finish, so it is unobtainable; and
// the context error when ctx ended first, so the wait was interrupted.
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
		} else if b.done && b.closeErr == nil {
			// Which message an index counted from the end names is settled
			// only once no more can arrive - and only if the stream reached
			// the end rather than stopping short of it. One that stopped has
			// no last message, just the last one that happened to arrive, so
			// it falls through to the reason it stopped.
			if n := len(b.items) + i; n >= 0 {
				return b.items[n], nil
			}
		}
		// A stream that has ended outranks a context that has: what the stream
		// did is a fact about the message, while the context only says this
		// caller stopped being willing to wait. A cancellation cannot be
		// mistaken for a finish, because End asks the stream's own context
		// before it calls a receive error an ending, so a receive this side cut
		// short reaches Close as the reason rather than as nil.
		if b.done {
			var zero T
			if b.closeErr != nil {
				// The stream did not finish, it stopped. The message is not
				// absent, it is unobtainable, and reporting it as an absence
				// would let ?? and defined() hide the reason.
				return zero, b.closeErr
			}
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
