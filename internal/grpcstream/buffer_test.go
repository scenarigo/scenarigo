package grpcstream

import (
	"context"
	"testing"
	"time"
)

func TestBuffer_AtUnblocksOnAppend(t *testing.T) {
	b := NewBuffer[int]()

	got := make(chan int, 1)
	go func() {
		v, ok := b.At(context.Background(), 1)
		if ok {
			got <- v
		} else {
			got <- -1
		}
	}()

	b.Append(10)
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

func TestBuffer_AtReturnsFalseOnClose(t *testing.T) {
	b := NewBuffer[int]()

	got := make(chan bool, 1)
	go func() {
		_, ok := b.At(context.Background(), 5)
		got <- ok
	}()

	b.Append(1)
	b.Close()

	select {
	case ok := <-got:
		if ok {
			t.Fatal("expected At to return false after Close")
		}
	case <-time.After(time.Second):
		t.Fatal("At did not unblock after Close")
	}
	// A normal close is not a wait error.
	if err := b.WaitErr(); err != nil {
		t.Fatalf("expected no WaitErr after a normal close but got %v", err)
	}
}

func TestBuffer_AtReturnsFalseOnContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := NewBuffer[int]()

	got := make(chan bool, 1)
	go func() {
		_, ok := b.At(ctx, 0)
		got <- ok
	}()

	cancel()

	select {
	case ok := <-got:
		if ok {
			t.Fatal("expected At to return false after context cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("At did not unblock after context cancel")
	}
	if b.WaitErr() == nil {
		t.Fatal("expected WaitErr to be non-nil after cancel")
	}
}

func TestBuffer_AtTimesOutWithDefaultGuard(t *testing.T) {
	old := DefaultMessageWaitTimeout
	DefaultMessageWaitTimeout = 100 * time.Millisecond
	defer func() { DefaultMessageWaitTimeout = old }()

	b := NewBuffer[int]()

	got := make(chan bool, 1)
	go func() {
		// No deadline on the context, so the default guard must fire.
		_, ok := b.At(context.Background(), 0)
		got <- ok
	}()

	select {
	case ok := <-got:
		if ok {
			t.Fatal("expected At to return false after the guard timeout")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("At did not unblock; the default guard did not fire")
	}
	if b.WaitErr() == nil {
		t.Fatal("expected WaitErr to be non-nil after the guard timeout")
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
