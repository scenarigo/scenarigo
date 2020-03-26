// NOTE: Fail on only macos-latest of GitHub Actions by duration errors.
// +build !darwin

package reporter

import (
	"testing"
	"time"
)

func TestTestContext(t *testing.T) {
	t.Run("serial", func(t *testing.T) {
		ctx := newTestContext(WithMaxParallel(1))
		if expect, got := 1, ctx.running; got != expect {
			t.Errorf("expected %d but got %d", expect, got)
		}

		now := time.Now()
		duration := 100 * time.Millisecond
		done := make(chan struct{})
		go func() {
			time.Sleep(duration)
			if expect, got := 1, ctx.numWaiting; got != expect {
				t.Errorf("expected %d but got %d", expect, got)
			}
			ctx.release()
			close(done)
		}()

		ctx.waitParallel() // wait goroutine function
		if expect, got := duration, time.Since(now).Truncate(10*time.Millisecond); got != expect {
			t.Errorf("expected %s but got %s", expect, got)
		}
		if expect, got := 0, ctx.numWaiting; got != expect {
			t.Errorf("expected %d but got %d", expect, got)
		}

		<-done
	})
	t.Run("parallel", func(t *testing.T) {
		ctx := newTestContext(WithMaxParallel(2))
		if expect, got := 1, ctx.running; got != expect {
			t.Errorf("expected %d but got %d", expect, got)
		}

		now := time.Now()
		duration := 100 * time.Millisecond
		done := make(chan struct{})
		go func() {
			time.Sleep(duration)
			if expect, got := 0, ctx.numWaiting; got != expect {
				t.Errorf("expected %d but got %d", expect, got)
			}
			ctx.release()
			close(done)
		}()

		ctx.waitParallel() // not wait goroutine function (run in parallel)
		if expect, got := time.Duration(0), time.Since(now).Truncate(10*time.Millisecond); got != expect {
			t.Errorf("expected %s but got %s", expect, got)
		}
		if expect, got := 0, ctx.numWaiting; got != expect {
			t.Errorf("expected %d but got %d", expect, got)
		}

		<-done
	})
}
