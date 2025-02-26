package template

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/zoncoen/query-go"
)

func TestLazy(t *testing.T) {
	tests := map[string]struct {
		tmpl      string
		data      any
		arg       any
		expect    any
		expectErr string
	}{
		"$": {
			tmpl:   `{{$}}`,
			arg:    1,
			expect: 1,
		},
		"$ + 1": {
			tmpl:   `{{$ + 1}}`,
			arg:    1,
			expect: int64(2),
		},
		"$ + a": {
			tmpl:   `{{$ + a}}`,
			data:   map[string]any{"a": 2},
			arg:    1,
			expect: int64(3),
		},
		"$ + $": {
			tmpl:   `{{$ + $}}`,
			arg:    1,
			expect: int64(2),
		},
		"initialization failed": {
			tmpl:      `{{$ + 1}}`,
			arg:       "str",
			expect:    int64(2),
			expectErr: `invalid operation: string(str) + int(1) not defined`,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			v, err := Execute(ctx, test.tmpl, test.data)
			if err != nil {
				if test.expectErr != "" && strings.Contains(err.Error(), test.expectErr) {
					return
				}
				t.Fatalf("failed to execute: %s", err)
			}
			lazy, ok := v.(Lazy)
			if !ok {
				t.Fatalf("expect Lazy but got %T", v)
			}
			got, err := lazy(test.arg)
			if err != nil {
				if test.expectErr != "" && strings.Contains(err.Error(), test.expectErr) {
					return
				}
				t.Fatalf("failed to initialize lazy value: %s", err)
			}
			if diff := cmp.Diff(test.expect, got); diff != "" {
				t.Errorf("diff: (-want +got)\n%s", diff)
			}
		})
	}
}

func TestWaitContext(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	wc := newWaitContext(ctx, map[string]string{"foo": "FOO"})
	if got, expect := extractVal(t, "$.foo", wc), "FOO"; got != expect {
		t.Fatalf("expect %q but got %q", expect, got)
	}

	// wait until setting a value
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if got, expect := extractVal(t, "$.$", wc), "BAR"; got != expect {
			t.Errorf("expect %q but got %q", expect, got)
		}
	}()
	go func() {
		defer wg.Done()
		if got, expect := extractVal(t, "$.$", wc), "BAR"; got != expect {
			t.Errorf("expect %q but got %q", expect, got)
		}
	}()

	if err := wc.set("BAR"); err != nil {
		t.Fatalf("failed to set: %s", err)
	}
	wg.Wait()

	// extract after setting
	if got, expect := extractVal(t, "$.$", wc), "BAR"; got != expect {
		t.Fatalf("expect %q but got %q", expect, got)
	}

	// ignore canceled if the value is already set
	cancel()
	for range 10 { // HACK: try many times for converage...
		if got, expect := extractVal(t, "$.$", wc), "BAR"; got != expect {
			t.Fatalf("expect %q but got %q", expect, got)
		}
	}

	// don't set twice
	if err := wc.set("BAR"); err == nil {
		t.Fatal("no error")
	}

	// canceled
	wc = newWaitContext(ctx, nil)
	q, err := query.ParseString("$.$", queryutil.Options()...)
	if err != nil {
		t.Fatalf("failed to parse query string: %s", err)
	}
	if _, err := q.Extract(wc); err == nil {
		t.Fatal("no error")
	}
}

func extractVal(t *testing.T, s string, target any) any {
	t.Helper()
	q, err := query.ParseString(s, queryutil.Options()...)
	if err != nil {
		t.Fatalf("failed to parse query string: %s", err)
	}
	v, err := q.Extract(target)
	if err != nil {
		t.Fatalf("failed to extract: %s", err)
	}
	return v
}
