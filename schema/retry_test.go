package schema

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/lestrrat-go/backoff"
)

func TestNeverBackoff(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	policy := &retryPolicyNever{}
	b, backoffCancel := policy.Start(ctx)
	defer backoffCancel()

	done := make(chan error)
	go func() {
		if expect, got := true, backoff.Continue(b); got != expect {
			done <- errors.New(fmt.Sprintf("expect %t but got %t", expect, got))
		}
		if expect, got := false, backoff.Continue(b); got != expect {
			done <- errors.New(fmt.Sprintf("expect %t but got %t", expect, got))
		}
		done <- nil
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("timeout")
	}
}
