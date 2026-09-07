package grpcstream

import (
	"context"
	"errors"
	"testing"
	"time"
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
	b.Close()

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
		b.Close()
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
		b.Close()
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
		b.Close()
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
	b.Close()
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
