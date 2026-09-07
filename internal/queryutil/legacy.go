package queryutil

import (
	"context"
	"errors"
	"reflect"

	query "github.com/zoncoen/query-go/v2"
)

// The extractor shapes that query-go v1 defined. Plugins built against
// scenarigo v0.2x implement one of these, and because Go interfaces are
// satisfied structurally such a type keeps compiling against v2 while silently
// no longer being an extractor: the lookup falls back to reflection and quietly
// returns something else. Adapt those types instead, so existing plugins keep
// working without a source change.
//
// These are declared here rather than imported so that adapting them does not
// drag the v1 module back into the build.
type (
	legacyKeyExtractor interface {
		ExtractByKey(key string) (any, bool)
	}
	legacyKeyExtractorContext interface {
		ExtractByKey(ctx context.Context, key string) (any, bool)
	}
	legacyIndexExtractor interface {
		ExtractByIndex(idx int) (any, bool)
	}
	legacyIndexExtractorContext interface {
		ExtractByIndex(ctx context.Context, idx int) (any, bool)
	}
)

func legacyExtractFunc() func(query.ExtractFunc) query.ExtractFunc {
	return func(f query.ExtractFunc) query.ExtractFunc {
		return func(ctx context.Context, in reflect.Value) (reflect.Value, error) {
			v, ok := adaptLegacyExtractor(in)
			if !ok {
				return f(ctx, in)
			}
			out, err := f(ctx, v)
			if errors.Is(err, errLegacyKindNotImplemented) {
				// The value does not implement this kind of lookup. Run the
				// step on the value itself, which reflects over it as v1 did.
				// Values reached through it (an inlined field, an element) go
				// through the whole chain again, so a legacy extractor nested
				// inside is still adapted.
				return f(ctx, in)
			}
			return out, err
		}
	}
}

// errLegacyKindNotImplemented is returned by the adapter for the kind of
// lookup the adapted value does not implement. legacyExtractFunc turns it into
// a lookup over the original value; it does not escape the extract func,
// because optionsWith composes the hook innermost and so nothing runs between
// the two that could rewrite it or ask the stand-in anything of its own. A
// query built by appending a custom extract func to Options() rather than
// going through this package would not hold to that.
var errLegacyKindNotImplemented = errors.New("legacy extractor does not implement this lookup")

type (
	keyFunc   func(ctx context.Context, key string) (any, error)
	indexFunc func(ctx context.Context, idx int) (any, error)
)

// adaptLegacyExtractor reports whether v implements a v1 extractor shape and,
// if so, returns a stand-in that implements the current interfaces.
//
// The stand-in answers both kinds of lookup. For the kind v itself implements
// it delegates; for the other it reports errLegacyKindNotImplemented so that
// legacyExtractFunc reflects over v, which is what query-go v1 did once its
// extractor interface did not match. Answering only the adapted kind would send
// the other lookup into the adapter's own structure instead.
func adaptLegacyExtractor(in reflect.Value) (reflect.Value, bool) {
	if !in.IsValid() || !in.CanInterface() {
		return reflect.Value{}, false
	}
	v := in.Interface()

	// Almost every value a lookup meets is not a legacy extractor. Decide that
	// first, without allocating: this runs on every step of every template
	// lookup.
	switch v.(type) {
	case legacyKeyExtractor, legacyKeyExtractorContext, legacyIndexExtractor, legacyIndexExtractorContext:
	default:
		return reflect.Value{}, false
	}

	a := &legacyAdapter{}

	// A type can only have one method named ExtractByKey, so at most one of
	// these matches. The current interface wins, which keeps a partially
	// migrated type working through both halves.
	switch e := v.(type) {
	case query.KeyExtractor:
		a.key = e.ExtractByKey
	case legacyKeyExtractorContext:
		// The context carries the v2 options. A v1 extractor that asks
		// query-go/v1.IsCaseInsensitive about it always gets false, because the
		// two packages key the flag by their own unexported type and v1 exports
		// no way to set it. Such an extractor sees the key as written, which is
		// the case-sensitive behaviour; migrating it to the v2 interface is the
		// only way to observe the flag.
		a.key = func(ctx context.Context, k string) (any, error) {
			return legacyResult(e.ExtractByKey(ctx, k))
		}
	case legacyKeyExtractor:
		a.key = func(_ context.Context, k string) (any, error) {
			return legacyResult(e.ExtractByKey(k))
		}
	}

	switch e := v.(type) {
	case query.IndexExtractor:
		a.index = e.ExtractByIndex
	case legacyIndexExtractorContext:
		a.index = func(ctx context.Context, i int) (any, error) {
			return legacyResult(e.ExtractByIndex(ctx, i))
		}
	case legacyIndexExtractor:
		a.index = func(_ context.Context, i int) (any, error) {
			return legacyResult(e.ExtractByIndex(i))
		}
	}

	return reflect.ValueOf(a), true
}

func legacyResult(v any, ok bool) (any, error) {
	if !ok {
		return nil, query.ErrNotFound
	}
	return v, nil
}

type legacyAdapter struct {
	key   keyFunc
	index indexFunc
}

var (
	_ query.KeyExtractor   = (*legacyAdapter)(nil)
	_ query.IndexExtractor = (*legacyAdapter)(nil)
)

// ExtractByKey implements query.KeyExtractor interface.
func (a *legacyAdapter) ExtractByKey(ctx context.Context, key string) (any, error) {
	if a.key != nil {
		return a.key(ctx, key)
	}
	return nil, errLegacyKindNotImplemented
}

// ExtractByIndex implements query.IndexExtractor interface.
func (a *legacyAdapter) ExtractByIndex(ctx context.Context, idx int) (any, error) {
	if a.index != nil {
		return a.index(ctx, idx)
	}
	return nil, errLegacyKindNotImplemented
}
