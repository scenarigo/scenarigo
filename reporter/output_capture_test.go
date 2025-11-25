package reporter

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

func TestOutputCapturerCapturesStdout(t *testing.T) {
	capturer := newOutputCapturer("stdout", &os.Stdout)
	stop, err := capturer.start()
	if err != nil {
		t.Fatalf("failed to start capture: %v", err)
	}
	stopped := false
	defer func() {
		if stopped {
			return
		}
		if _, stopErr := stop(); stopErr != nil {
			t.Fatalf("failed to stop capture in cleanup: %v", stopErr)
		}
	}()

	fmt.Fprint(os.Stdout, "hello")
	captured, err := stop()
	stopped = true
	if err != nil {
		t.Fatalf("failed to stop capture: %v", err)
	}
	if captured != "hello" {
		t.Fatalf("unexpected capture: %q", captured)
	}
}

func TestOutputCapturerStartTwice(t *testing.T) {
	capturer := newOutputCapturer("stdout", &os.Stdout)
	stop, err := capturer.start()
	if err != nil {
		t.Fatalf("failed to start capture: %v", err)
	}
	stopped := false
	defer func() {
		if stopped {
			return
		}
		if _, stopErr := stop(); stopErr != nil {
			t.Fatalf("failed to stop capture in cleanup: %v", stopErr)
		}
	}()

	if _, err := capturer.start(); err == nil {
		t.Fatal("expected error but got nil")
	} else if !strings.Contains(err.Error(), "already started") {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, err := stop(); err != nil {
		t.Fatalf("failed to stop capture: %v", err)
	}
	stopped = true
}

func TestOutputCapturerCapturesStderr(t *testing.T) {
	capturer := newOutputCapturer("stderr", &os.Stderr)
	stop, err := capturer.start()
	if err != nil {
		t.Fatalf("failed to start capture: %v", err)
	}
	stopped := false
	defer func() {
		if stopped {
			return
		}
		if _, stopErr := stop(); stopErr != nil {
			t.Fatalf("failed to stop capture in cleanup: %v", stopErr)
		}
	}()

	fmt.Fprint(os.Stderr, "error")
	captured, err := stop()
	stopped = true
	if err != nil {
		t.Fatalf("failed to stop capture: %v", err)
	}
	if captured != "error" {
		t.Fatalf("unexpected capture: %q", captured)
	}
}
