package reporter

import (
	"bytes"
	"fmt"
	"strings"
	"sync"
	"testing"
)

// TestRun_ParallelExecution verifies behavior when Run is called concurrently.
func TestRun_ParallelExecution(t *testing.T) {
	t.Run("concurrent Run calls should panic", func(t *testing.T) {
		// Channel to coordinate concurrent test execution
		running := make(chan struct{})
		// Wait for both Run calls to reach the test body (or panic)
		var wg1 sync.WaitGroup
		// Wait for both goroutines to complete
		var wg2 sync.WaitGroup
		panics := make(chan any, 2)

		for i := 0; i < 2; i++ {
			wg1.Add(1)
			wg2.Add(1)
			go func() {
				defer wg2.Done()
				defer func() {
					if r := recover(); r != nil {
						panics <- r
						wg1.Done() // Signal completion even on panic
					}
				}()

				var b bytes.Buffer
				Run(func(r Reporter) {
					r.Run("test", func(r Reporter) {
						r.Log("test log")
						wg1.Done()
						<-running
					})
				}, WithWriter(&b))
			}()
		}

		wg1.Wait()
		close(running)
		wg2.Wait()
		close(panics)

		var panicMessages []any
		for p := range panics {
			panicMessages = append(panicMessages, p)
		}

		if len(panicMessages) == 0 {
			t.Fatal("expected at least one panic, but got none")
		}

		foundExpectedPanic := false
		for _, p := range panicMessages {
			if str, ok := p.(string); ok {
				t.Logf("panic message: %s", str)
				if strings.Contains(str, "stdout capture already started") {
					foundExpectedPanic = true
				}
			}
		}

		if !foundExpectedPanic {
			t.Errorf("expected panic with 'stdout capture already started' message, but got: %v", panicMessages)
		}
	})

	t.Run("sequential Run calls should succeed", func(t *testing.T) {
		for i := 0; i < 3; i++ {
			var b bytes.Buffer
			success := Run(func(r Reporter) {
				r.Run("test", func(r Reporter) {
					r.Log("test log")
				})
			}, WithWriter(&b))

			if !success {
				t.Errorf("Run #%d failed unexpectedly", i)
			}
		}
	})

	t.Run("concurrent Run calls with WithoutStdoutCapture should succeed", func(t *testing.T) {
		// Channel to coordinate concurrent test execution
		running := make(chan struct{})
		// Wait for all Run calls to reach the test body
		var wg1 sync.WaitGroup
		// Wait for all goroutines to complete
		var wg2 sync.WaitGroup
		errors := make(chan error, 2)

		for i := 0; i < 2; i++ {
			wg1.Add(1)
			wg2.Add(1)
			go func(id int) {
				defer wg2.Done()
				defer func() {
					if r := recover(); r != nil {
						errors <- fmt.Errorf("goroutine %d panicked: %v", id, r)
					}
				}()

				var b bytes.Buffer
				success := Run(func(r Reporter) {
					r.Run("test", func(r Reporter) {
						r.Log("test log")
						wg1.Done()
						<-running
					})
				}, WithWriter(&b), WithoutStdoutCapture())

				if !success {
					errors <- fmt.Errorf("goroutine %d failed", id)
				}
			}(i)
		}

		wg1.Wait()
		close(running)
		wg2.Wait()
		close(errors)

		var errMsgs []string
		for err := range errors {
			errMsgs = append(errMsgs, err.Error())
		}

		if len(errMsgs) > 0 {
			t.Fatalf("expected no errors or panics, but got: %v", errMsgs)
		}
	})
}
