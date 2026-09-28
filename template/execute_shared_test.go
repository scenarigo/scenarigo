package template

import (
	"context"
	"reflect"
	"runtime"
	"sync/atomic"
	"testing"
)

// sharedReceiver stands in for a value that scenarios share across goroutines,
// such as the cached plugin instance behind "plugins.<name>".
type sharedReceiver struct {
	Exported *string
	state    atomic.Uint64
}

func (*sharedReceiver) ExtractByKey(key string) (any, bool) {
	if key == "Hello" {
		return func(s string) string { return s }, true
	}
	return nil, false
}

// TestExecute_SharedPointerIsNotWrittenBack checks that executing a template whose
// receiver is a pointer to a shared struct does not copy the struct back into
// itself. The concurrent writer touches an unexported field only; with the race
// detector enabled, a write-back of the whole struct is reported as a data race.
func TestExecute_SharedPointerIsNotWrittenBack(t *testing.T) {
	text := "hello"
	p := &sharedReceiver{Exported: &text}
	data := map[string]any{"plugins": map[string]any{"p1": p}}

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			p.state.Store(1)
			runtime.Gosched()
			p.state.Store(0)
		}
	}()
	defer func() {
		close(stop)
		<-done
	}()

	for range 200 {
		got, err := Execute(context.Background(), `{{plugins.p1.Hello("a")}}`, data)
		if err != nil {
			t.Fatal(err)
		}
		if got != "a" {
			t.Fatalf("expected %q but got %v", "a", got)
		}
	}
	if p.Exported != &text {
		t.Fatal("exported pointer field was replaced")
	}
}

// TestExecute_PointerReceiverKeepsIdentity checks that a pointer to a struct that
// contains templates is still updated in place and returned as the same pointer.
func TestExecute_PointerReceiverKeepsIdentity(t *testing.T) {
	text := `{{"pointer"}}`
	p := &struct {
		Text string
		Ptr  *string
	}{Text: `{{"value"}}`, Ptr: &text}
	got, err := Execute(context.Background(), p, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got != p {
		t.Fatalf("expected the same pointer but got %T", got)
	}
	if p.Text != "value" || *p.Ptr != "pointer" || p.Ptr != &text {
		t.Fatalf("unexpected result: %+v", p)
	}
}

type selfRef struct {
	next *selfRef
}

// TestExecute_SameAddressDifferentType covers a value whose address equals the
// address of its first field. Aliasing must be decided by type as well as
// address, otherwise the returned type changes.
func TestExecute_SameAddressDifferentType(t *testing.T) {
	n := &selfRef{}
	n.next = n
	got, err := Execute(context.Background(), &n.next, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := got.(selfRef); !ok {
		t.Fatalf("expected %T but got %T", selfRef{}, got)
	}
}

type mapOwner struct {
	M map[string]any
}

type returnValue struct{ value any }

func (f returnValue) UnmarshalArg(unmarshal func(any) error) (any, error) {
	var v any
	err := unmarshal(&v)
	return v, err
}

func (f returnValue) Exec(any) (any, error) { return f.value, nil }

// TestExecute_RejectSameAddressDifferentPointerType covers a struct field whose
// address equals the address of another value of a different pointer type. The
// type mismatch must still be reported instead of leaving the field unevaluated.
func TestExecute_RejectSameAddressDifferentPointerType(t *testing.T) {
	owner := &mapOwner{M: map[string]any{"{{f <-}}": "arg"}}
	input := &struct{ M *map[string]any }{&owner.M}
	if reflect.ValueOf(owner).Pointer() != reflect.ValueOf(input.M).Pointer() {
		t.Fatal("expected the owner and its first field to share an address")
	}
	_, err := Execute(context.Background(), input, map[string]any{"f": returnValue{owner}})
	if err == nil {
		t.Fatalf("expected an error but the field was left as %v", *input.M)
	}
}
