package cmd

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

// useStdin feeds input to os.Stdin through a pipe. The write end stays open
// when keepOpen is set, so that the server blocks on reading.
func useStdin(t *testing.T, input string, keepOpen bool) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdin
	os.Stdin = r
	t.Cleanup(func() {
		os.Stdin = orig
		w.Close()
		r.Close()
	})
	go func() {
		_, _ = io.WriteString(w, input)
		if !keepOpen {
			w.Close()
		}
	}()
}

func TestLSP(t *testing.T) {
	devnull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = devnull
	t.Cleanup(func() {
		os.Stdout = origStdout
		devnull.Close()
	})

	t.Run("closed input", func(t *testing.T) {
		useStdin(t, "", false)
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		if err := lspCmd.RunE(cmd, nil); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})
	t.Run("invalid header", func(t *testing.T) {
		useStdin(t, "bogus\r\n\r\n", false)
		cmd := &cobra.Command{}
		cmd.SetContext(context.Background())
		err := lspCmd.RunE(cmd, nil)
		if err == nil || !strings.Contains(err.Error(), "invalid header line") {
			t.Fatalf("expected an invalid header error, got %v", err)
		}
	})
	t.Run("canceled context", func(t *testing.T) {
		useStdin(t, "", true)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		cmd := &cobra.Command{}
		cmd.SetContext(ctx)
		if err := lspCmd.RunE(cmd, nil); err != nil {
			t.Fatalf("cancellation must not be reported: %v", err)
		}
	})
}
