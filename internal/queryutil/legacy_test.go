package queryutil

import (
	"context"
	"errors"
	"reflect"
	"testing"

	protobufextractor "github.com/zoncoen/query-go/extractor/protobuf/v2"
	query "github.com/zoncoen/query-go/v2"
)

// legacyKey has the extractor shape that plugins built against scenarigo v0.2x
// implement. It must keep working after the query-go v2 upgrade: the method
// still compiles, so nothing tells the plugin author that the type stopped
// satisfying the extractor interface.
type legacyKey struct {
	m map[string]any
}

func (l *legacyKey) ExtractByKey(key string) (any, bool) {
	v, ok := l.m[key]
	return v, ok
}

type legacyKeyCtx struct {
	m           map[string]any
	gotCanceled bool
}

func (l *legacyKeyCtx) ExtractByKey(ctx context.Context, key string) (any, bool) {
	if ctx.Err() != nil {
		l.gotCanceled = true
	}
	v, ok := l.m[key]
	return v, ok
}

type legacyIndex struct {
	s []any
}

func (l *legacyIndex) ExtractByIndex(i int) (any, bool) {
	if i < 0 || i >= len(l.s) {
		return nil, false
	}
	return l.s[i], true
}

// legacyBoth implements both shapes, which the adapter has to preserve
// together: claiming only one of them would send the other lookup down the
// reflection path.
type legacyBoth struct {
	m map[string]any
	s []any
}

func (l *legacyBoth) ExtractByKey(key string) (any, bool) {
	v, ok := l.m[key]
	return v, ok
}

func (l *legacyBoth) ExtractByIndex(i int) (any, bool) {
	if i < 0 || i >= len(l.s) {
		return nil, false
	}
	return l.s[i], true
}

func TestLegacyExtractor(t *testing.T) {
	t.Run("found", func(t *testing.T) {
		tests := map[string]struct {
			target any
			build  func(*query.Query) *query.Query
			expect any
		}{
			"key": {
				target: &legacyKey{m: map[string]any{"foo": "FOO"}},
				build:  func(q *query.Query) *query.Query { return q.Key("foo") },
				expect: "FOO",
			},
			"key with context": {
				target: &legacyKeyCtx{m: map[string]any{"foo": "FOO"}},
				build:  func(q *query.Query) *query.Query { return q.Key("foo") },
				expect: "FOO",
			},
			"index": {
				target: &legacyIndex{s: []any{"a", "b"}},
				build:  func(q *query.Query) *query.Query { return q.Index(1) },
				expect: "b",
			},
			"both (key)": {
				target: &legacyBoth{m: map[string]any{"foo": "FOO"}, s: []any{"a"}},
				build:  func(q *query.Query) *query.Query { return q.Key("foo") },
				expect: "FOO",
			},
			"both (index)": {
				target: &legacyBoth{m: map[string]any{"foo": "FOO"}, s: []any{"a"}},
				build:  func(q *query.Query) *query.Query { return q.Index(0) },
				expect: "a",
			},
			"nested under a map": {
				target: map[string]any{"p": &legacyKey{m: map[string]any{"foo": "FOO"}}},
				build:  func(q *query.Query) *query.Query { return q.Key("p").Key("foo") },
				expect: "FOO",
			},
		}
		for name, test := range tests {
			t.Run(name, func(t *testing.T) {
				got, err := test.build(New()).Extract(context.Background(), test.target)
				if err != nil {
					t.Fatalf("unexpected error: %s", err)
				}
				if got != test.expect {
					t.Errorf("expected %v but got %v", test.expect, got)
				}
			})
		}
	})

	t.Run("not found is reported as ErrNotFound", func(t *testing.T) {
		targets := map[string]any{
			"key":   &legacyKey{m: map[string]any{"foo": "FOO"}},
			"index": &legacyIndex{s: []any{"a"}},
		}
		for name, target := range targets {
			t.Run(name, func(t *testing.T) {
				q := New().Key("missing")
				if name == "index" {
					q = New().Index(9)
				}
				if _, err := q.Extract(context.Background(), target); !errors.Is(err, query.ErrNotFound) {
					t.Fatalf("expected ErrNotFound but got %v", err)
				}
			})
		}
	})

	t.Run("context reaches a legacy context extractor", func(t *testing.T) {
		l := &legacyKeyCtx{m: map[string]any{"foo": "FOO"}}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := New().Key("foo").Extract(ctx, l); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if !l.gotCanceled {
			t.Error("the caller's context did not reach the legacy extractor")
		}
	})

	t.Run("a v2 extractor is left alone", func(t *testing.T) {
		v := &modernKey{}
		if _, err := New().Key("foo").Extract(context.Background(), v); err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if !v.called {
			t.Error("the v2 extractor was not used")
		}
	})
}

type modernKey struct {
	called bool
}

func (m *modernKey) ExtractByKey(_ context.Context, key string) (any, error) {
	m.called = true
	return key, nil
}

// legacySlice is a slice type with the v1 key-extractor shape. query-go v1
// reflected over the slice for index lookups because it did not implement the
// index extractor, so adapting it must not take that away.
type legacySlice []any

func (l legacySlice) ExtractByKey(key string) (any, bool) {
	if key == "first" && len(l) > 0 {
		return l[0], true
	}
	return nil, false
}

// legacyStruct has the v1 index-extractor shape, so its fields must stay
// reachable by key.
type legacyStruct struct {
	Foo string `yaml:"foo"`
}

func (l legacyStruct) ExtractByIndex(idx int) (any, bool) {
	if idx == 0 {
		return "a", true
	}
	return nil, false
}

// legacyOuter has the v1 index-extractor shape and inlines a field whose type
// has the v1 key-extractor shape. A key lookup reflects over the outer value,
// and the inner extractor reached that way must still be adapted, as in v1.
type legacyOuter struct {
	Inner legacyInner `yaml:",inline"`
}

func (legacyOuter) ExtractByIndex(_ int) (any, bool) {
	return nil, false
}

type legacyInner struct{}

func (legacyInner) ExtractByKey(key string) (any, bool) {
	if key == "foo" {
		return "from-inner", true
	}
	return nil, false
}

func TestLegacyExtractor_KeepsTheOtherLookup(t *testing.T) {
	tests := map[string]struct {
		target any
		build  func(*query.Query) *query.Query
		expect any
	}{
		"index over a key-only legacy value": {
			target: legacySlice{"a", "b", "c"},
			build:  func(q *query.Query) *query.Query { return q.Index(1) },
			expect: "b",
		},
		"key over a key-only legacy value": {
			target: legacySlice{"a", "b", "c"},
			build:  func(q *query.Query) *query.Query { return q.Key("first") },
			expect: "a",
		},
		"key over an index-only legacy value": {
			target: legacyStruct{Foo: "FOO"},
			build:  func(q *query.Query) *query.Query { return q.Key("foo") },
			expect: "FOO",
		},
		"index over an index-only legacy value": {
			target: legacyStruct{Foo: "FOO"},
			build:  func(q *query.Query) *query.Query { return q.Index(0) },
			expect: "a",
		},
		"key over an inlined legacy field of an index-only legacy value": {
			target: legacyOuter{},
			build:  func(q *query.Query) *query.Query { return q.Key("foo") },
			expect: "from-inner",
		},
		"key over an inlined legacy field of a plain struct": {
			target: struct {
				Inner legacyInner `yaml:",inline"`
			}{},
			build:  func(q *query.Query) *query.Query { return q.Key("foo") },
			expect: "from-inner",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := test.build(New()).Extract(context.Background(), test.target)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got != test.expect {
				t.Errorf("expected %v but got %v", test.expect, got)
			}
		})
	}

	t.Run("a lookup neither side can answer is still not found", func(t *testing.T) {
		if _, err := New().Index(9).Extract(context.Background(), legacySlice{"a"}); !errors.Is(err, query.ErrNotFound) {
			t.Fatalf("expected ErrNotFound but got %v", err)
		}
	})
}

// BenchmarkLegacyExtractFunc measures the cost the legacy adapter adds to a
// lookup over values that are not legacy extractors, which is every lookup a
// template makes over plain data.
func BenchmarkLegacyExtractFunc(b *testing.B) {
	target := map[string]any{"a": map[string]any{"b": map[string]any{"c": 1}}}
	q := New().Key("a").Key("b").Key("c")
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if _, err := q.Extract(ctx, target); err != nil {
			b.Fatal(err)
		}
	}
}

func TestLegacyExtractor_ACustomFuncRewritingErrorsKeepsTheFallback(t *testing.T) {
	// The public contract lets a custom extract func rewrite the errors it
	// sees, as long as it keeps query.ErrNotFound. One composed inside the
	// legacy hook would rewrite the adapter's report of a lookup kind the
	// value does not have, and the value would lose its reflection fallback.
	rewriting := query.CustomExtractFunc(func(f query.ExtractFunc) query.ExtractFunc {
		return func(ctx context.Context, in reflect.Value) (reflect.Value, error) {
			v, err := f(ctx, in)
			if err != nil && !errors.Is(err, query.ErrNotFound) {
				return reflect.Value{}, errors.New("rewritten by the protocol")
			}
			return v, err
		}
	})

	got, err := New(rewriting).Index(1).Extract(context.Background(), legacySlice{"a", "b", "c"})
	if err != nil {
		t.Fatalf("unexpected error: %s", err)
	}
	if got != "b" {
		t.Errorf("expected %q but got %v", "b", got)
	}
}

// hidingKeyExtractor has the v1 key shape and reports every key as absent, the
// way an extractor that deliberately exposes nothing does.
type hidingKeyExtractor struct {
	Foo string `yaml:"foo"`
}

func (hidingKeyExtractor) ExtractByKey(_ string) (any, bool) { return nil, false }

func TestLegacyExtractor_ACustomFuncCannotProbeTheStandIn(t *testing.T) {
	// The stand-in satisfies both interfaces whatever the value does, so a
	// custom extract func that could reach it and ask for the kind the value
	// lacks would be able to decide, from outside, whether the lookup falls
	// back to reflection - turning an absence the extractor meant into the
	// field it declined to give. It cannot reach it: the hook is composed
	// innermost, so a custom func is handed the value, not the stand-in.
	probe := func(after bool, sawStandIn *bool) query.Option {
		return query.CustomExtractFunc(func(f query.ExtractFunc) query.ExtractFunc {
			return func(ctx context.Context, in reflect.Value) (reflect.Value, error) {
				ask := func() {
					e, ok := in.Interface().(query.IndexExtractor)
					if !ok {
						return
					}
					// Only the stand-in answers a kind the value does not
					// have, so reaching one here is reaching the stand-in.
					*sawStandIn = true
					_, _ = e.ExtractByIndex(ctx, 0)
				}
				if !after {
					ask()
					return f(ctx, in)
				}
				out, err := f(ctx, in)
				ask()
				return out, err
			}
		})
	}

	for name, after := range map[string]bool{"before the step": false, "after the step": true} {
		t.Run(name, func(t *testing.T) {
			var sawStandIn bool
			got, err := New(probe(after, &sawStandIn)).Key("foo").
				Extract(context.Background(), hidingKeyExtractor{Foo: "FOO"})
			if sawStandIn {
				t.Error("the custom func was handed the stand-in")
			}
			if err == nil {
				t.Fatalf("the extractor reported an absence but the field leaked: %v", got)
			}
			if !errors.Is(err, query.ErrNotFound) {
				t.Errorf("expected an absence but got %s", err)
			}

			// And the other direction: the fallback must not be lost either.
			// legacySlice has the key shape only, so an index lookup is the
			// one the stand-in reports on and the hook retries by reflection.
			sawStandIn = false
			if _, err := New(probe(after, &sawStandIn)).Index(1).
				Extract(context.Background(), legacySlice{"a", "b", "c"}); err != nil {
				t.Errorf("the reflection fallback was lost: %s", err)
			}
			if sawStandIn {
				t.Error("the custom func was handed the stand-in")
			}
		})
	}
}

// taggedAndLegacy has a protobuf-tagged field and also the v1 key shape, so
// the protobuf extract func a protocol registers and the adapter both have
// something to say about it.
type taggedAndLegacy struct {
	Foo string `protobuf:"bytes,1,opt,name=foo,proto3"`
}

func (taggedAndLegacy) ExtractByKey(key string) (any, bool) {
	if key == "onlyLegacy" {
		return "from-v1-extractor", true
	}
	return "from-v1-extractor", true
}

func TestLegacyExtractor_ARegisteredFuncSeesTheValueNotTheStandIn(t *testing.T) {
	// The adapter composes innermost, so the custom extract funcs a protocol
	// registers are handed the value itself. A type with a protobuf-tagged
	// field that is also a v1-shaped extractor therefore resolves that field
	// through the tag, as it did before there was an adapter; anywhere the
	// adapter came first, the stand-in would hide the tag from them.
	restore := swapOptions(query.CustomExtractFunc(protobufextractor.ExtractFunc()))
	defer restore()

	target := taggedAndLegacy{Foo: "from-protobuf-tag"}
	for name, test := range map[string]struct {
		key    string
		expect string
	}{
		"a tagged field goes to the protobuf extractor": {"foo", "from-protobuf-tag"},
		"anything else goes to the v1 extractor":        {"onlyLegacy", "from-v1-extractor"},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := New().Key(test.key).Extract(context.Background(), target)
			if err != nil {
				t.Fatalf("unexpected error: %s", err)
			}
			if got != test.expect {
				t.Errorf("expected %q but got %v", test.expect, got)
			}
		})
	}
}

// swapOptions installs the given process-wide options for the duration of a
// test, the way registering a protocol would, and returns a function to put
// the previous ones back. It replaces state the whole package reads, so a test
// using it must not call t.Parallel, and neither may one running beside it.
func swapOptions(o ...query.Option) func() {
	m.Lock()
	prev := opts
	opts = o
	m.Unlock()
	return func() {
		m.Lock()
		opts = prev
		m.Unlock()
	}
}
