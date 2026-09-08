package grpcstream

import (
	"context"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// waitUntilBlocked blocks until at least one consumer is parked in cond.Wait,
// so a test can signal the waiter deterministically instead of racing it.
func waitUntilBlocked[T any](t *testing.T, b *Buffer[T]) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for b.numWaiters() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("consumer never blocked in At")
		}
		time.Sleep(time.Millisecond)
	}
}

func TestBuffer_AtUnblocksOnAppend(t *testing.T) {
	b := NewBuffer[int]()

	got := make(chan int, 1)
	go func() {
		v, err := b.At(context.Background(), 1)
		if err == nil {
			got <- v
		} else {
			got <- -1
		}
	}()

	// Ensure the consumer is parked waiting for index 1 before appending, so the
	// Append→Broadcast wakeup path is exercised rather than the fast path.
	b.Append(10)
	waitUntilBlocked(t, b)
	b.Append(20)

	select {
	case v := <-got:
		if v != 20 {
			t.Fatalf("expected 20 but got %d", v)
		}
	case <-time.After(time.Second):
		t.Fatal("At did not unblock after Append")
	}
}

func TestBuffer_AtReturnsErrClosedOnClose(t *testing.T) {
	b := NewBuffer[int]()

	got := make(chan error, 1)
	go func() {
		_, err := b.At(context.Background(), 5)
		got <- err
	}()

	b.Append(1)
	b.Close(nil)

	select {
	case err := <-got:
		if !errors.Is(err, ErrClosed) {
			t.Fatalf("expected ErrClosed after Close but got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("At did not unblock after Close")
	}
}

func TestBuffer_AtReturnsContextErrorOnCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := NewBuffer[int]()

	got := make(chan error, 1)
	go func() {
		_, err := b.At(ctx, 0)
		got <- err
	}()

	// Park the consumer in cond.Wait before canceling, so the cancel-driven
	// broadcast wakeup path is exercised rather than an early ctx.Err() check.
	waitUntilBlocked(t, b)
	cancel()

	select {
	case err := <-got:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("expected context.Canceled after cancel but got %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("At did not unblock after context cancel")
	}
}

func TestBuffer_AtReturnsContextErrorOnDeadline(t *testing.T) {
	// Callers bound blocking waits by putting a deadline (the deadlock guard)
	// on the context; At must unblock when it expires.
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	b := NewBuffer[int]()

	got := make(chan error, 1)
	go func() {
		_, err := b.At(ctx, 0)
		got <- err
	}()

	select {
	case err := <-got:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected context.DeadlineExceeded after the deadline but got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("At did not unblock; the context deadline did not fire")
	}
}

func TestBuffer_AtResolvesNegativeIndexWhenClosed(t *testing.T) {
	// query-go v2 hands a negative index to the extractor as it is. Which
	// message it names is settled only once the stream has ended, so At waits
	// for that and then counts from the end.
	t.Run("a closed buffer counts from the end", func(t *testing.T) {
		b := NewBuffer[int]()
		b.Append(1)
		b.Append(2)
		b.Append(3)
		b.Close(nil)
		for i, expect := range map[int]int{-1: 3, -2: 2, -3: 1} {
			got, err := b.At(context.Background(), i)
			if err != nil {
				t.Fatalf("At(%d): unexpected error: %s", i, err)
			}
			if got != expect {
				t.Errorf("At(%d) = %d, want %d", i, got, expect)
			}
		}
	})

	t.Run("reaching past the start is an absence", func(t *testing.T) {
		b := NewBuffer[int]()
		b.Append(1)
		b.Append(2)
		b.Append(3)
		b.Close(nil)
		if _, err := b.At(context.Background(), -5); !errors.Is(err, ErrClosed) {
			t.Fatalf("expected ErrClosed but got %v", err)
		}
	})

	t.Run("an open buffer waits for the end", func(t *testing.T) {
		b := NewBuffer[int]()
		b.Append(1)
		type result struct {
			v   int
			err error
		}
		ch := make(chan result, 1)
		go func() {
			v, err := b.At(context.Background(), -1)
			ch <- result{v, err}
		}()
		select {
		case r := <-ch:
			t.Fatalf("At returned %v/%v before the stream ended", r.v, r.err)
		case <-time.After(50 * time.Millisecond):
		}
		// The last message is not the one that was there when At was called.
		b.Append(2)
		b.Close(nil)
		select {
		case r := <-ch:
			if r.err != nil {
				t.Fatalf("unexpected error: %s", r.err)
			}
			if r.v != 2 {
				t.Errorf("expected the last message 2 but got %d", r.v)
			}
		case <-time.After(time.Second):
			t.Fatal("At did not return after the stream ended")
		}
	})

	t.Run("a context that ends first is an interrupted wait", func(t *testing.T) {
		b := NewBuffer[int]()
		b.Append(1)
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		if _, err := b.At(ctx, -1); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("expected the context error but got %v", err)
		}
	})
}

func TestBuffer_Done(t *testing.T) {
	b := NewBuffer[int]()
	if b.Done() {
		t.Fatal("expected Done to be false before Close")
	}
	b.Append(1)
	if b.Done() {
		t.Fatal("expected Done to be false after Append")
	}
	b.Close(nil)
	if !b.Done() {
		t.Fatal("expected Done to be true after Close")
	}
}

func TestBuffer_Snapshot(t *testing.T) {
	b := NewBuffer[string]()

	if got := b.Snapshot(); len(got) != 0 {
		t.Fatalf("expected empty snapshot but got %v", got)
	}
	b.Append("a")
	b.Append("b")
	got := b.Snapshot()
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected snapshot: %v", got)
	}
}

func TestBuffer_AtReportsWhyTheStreamStopped(t *testing.T) {
	// A stream that stopped is not a stream that finished: the message is not
	// absent, it is unobtainable. Reporting it as an absence would let ?? and
	// defined() hide the reason, which is what the typed error exists to stop.
	t.Run("an index counted from the end reports the reason too", func(t *testing.T) {
		// A stream that stopped has no last message, only the last one that
		// happened to arrive before it did. Counting from an end it never
		// reached would report that one as if the stream had finished on it.
		b := NewBuffer[int]()
		b.Append(1)
		b.Append(2)
		stopped := errors.New("rpc error: code = Unavailable")
		b.Close(stopped)
		if _, err := b.At(context.Background(), -1); !errors.Is(err, stopped) {
			t.Fatalf("expected the reason the stream stopped but got %v", err)
		}
	})

	t.Run("a stream that stopped reports the reason", func(t *testing.T) {
		b := NewBuffer[int]()
		stopped := errors.New("rpc error: code = Canceled")
		b.Close(stopped)
		if _, err := b.At(context.Background(), 0); !errors.Is(err, stopped) {
			t.Fatalf("expected the reason the stream stopped but got %v", err)
		}
		if _, err := b.At(context.Background(), 0); errors.Is(err, ErrClosed) {
			t.Fatal("the reason was reported as a plain absence")
		}
	})

	t.Run("a stream that finished is an absence", func(t *testing.T) {
		// End is what turns an ending into the nil Close takes.
		for _, err := range []error{nil, io.EOF, status.Error(codes.Aborted, "aborted")} {
			b := NewBuffer[int]()
			b.Close(End(context.Background(), err))
			if _, got := b.At(context.Background(), 0); !errors.Is(got, ErrClosed) {
				t.Fatalf("Close(End(%v)): expected ErrClosed but got %v", err, got)
			}
		}
	})

	t.Run("a waiter interrupted by the close learns the reason", func(t *testing.T) {
		b := NewBuffer[int]()
		stopped := errors.New("rpc error: code = Unavailable")
		ch := make(chan error, 1)
		go func() {
			_, err := b.At(context.Background(), 0)
			ch <- err
		}()
		time.Sleep(20 * time.Millisecond)
		b.Close(stopped)
		select {
		case err := <-ch:
			if !errors.Is(err, stopped) {
				t.Fatalf("expected the reason the stream stopped but got %v", err)
			}
		case <-time.After(time.Second):
			t.Fatal("At did not return after the stream stopped")
		}
	})
}

func TestBuffer_AtPrefersWhatTheStreamDid(t *testing.T) {
	// A context that has ended says only that this caller stopped waiting; what
	// the stream did says whether the message can ever arrive. The latter is
	// the more useful answer and does not depend on how the two raced, so a
	// stream that has ended outranks a context that has.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("a finished stream is an absence", func(t *testing.T) {
		b := NewBuffer[int]()
		b.Append(1)
		b.Close(nil)
		if _, err := b.At(ctx, 5); !errors.Is(err, ErrClosed) {
			t.Fatalf("expected ErrClosed but got %v", err)
		}
	})

	t.Run("a stopped stream reports the reason", func(t *testing.T) {
		b := NewBuffer[int]()
		stopped := errors.New("rpc error: code = Canceled")
		b.Close(stopped)
		if _, err := b.At(ctx, 0); !errors.Is(err, stopped) {
			t.Fatalf("expected the reason the stream stopped but got %v", err)
		}
	})

	t.Run("an open stream reports the interrupted wait", func(t *testing.T) {
		b := NewBuffer[int]()
		if _, err := b.At(ctx, 0); !errors.Is(err, context.Canceled) {
			t.Fatalf("expected the context error but got %v", err)
		}
	})
}

func TestEnd(t *testing.T) {
	// What counts as an ending is the protocol's business. A gRPC stream that
	// ends with a non-OK status the peer sent has ended, and the status is
	// something the scenario asserts on with expect.status.code, so a message
	// the stream ended before producing is absent - not unobtainable, which
	// would make ?? and defined() fail instead of falling back.
	//
	// A status gRPC synthesised for a context that ended on this side is a
	// different thing. Our own deadline is what bounds a blocking message
	// reference, so reading it as an ending would turn the message that
	// reference was waiting for into an absence, and ?? would hide the timeout.
	clientBug := errors.New("nil stream returned by the plugin")
	ours, cancel := context.WithCancel(context.Background())
	cancel()
	timedOut := status.Error(codes.DeadlineExceeded, "context deadline exceeded")
	aborted := status.Error(codes.Aborted, "stream aborted")
	for name, test := range map[string]struct {
		ctx    context.Context
		err    error
		expect error
	}{
		"no error":                       {context.Background(), nil, nil},
		"the OK ending":                  {context.Background(), io.EOF, nil},
		"a wrapped OK ending":            {context.Background(), fmt.Errorf("recv: %w", io.EOF), nil},
		"a non-OK status":                {context.Background(), status.Error(codes.Aborted, "stream aborted"), nil},
		"a cancelled call":               {context.Background(), status.Error(codes.Canceled, "context canceled"), nil},
		"an unavailable peer":            {context.Background(), status.Error(codes.Unavailable, "connection refused"), nil},
		"anything else":                  {context.Background(), clientBug, clientBug},
		"our own context ended":          {ours, timedOut, timedOut},
		"our own context, anything else": {ours, clientBug, clientBug},
		"our own context, but the stream finished first": {ours, io.EOF, nil},
		// Kept deliberately, although the peer really did send this one: it
		// cannot be told apart from the status gRPC synthesises for our own
		// context, and the waiter racing that context reports an interrupted
		// wait on the other branch. Ending the stream here would decide
		// absence-or-failure by who won the race.
		"our own context ended as the peer ended it": {ours, aborted, aborted},
	} {
		t.Run(name, func(t *testing.T) {
			if got := End(test.ctx, test.err); !errors.Is(got, test.expect) {
				t.Errorf("End(%v) = %v, want %v", test.err, got, test.expect)
			}
		})
	}
}
