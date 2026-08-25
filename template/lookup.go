package template

import (
	"context"
	"encoding/json"
	"math"
	"reflect"
	"strconv"

	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"

	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/scenarigo/scenarigo/template/ast"
)

type notDefinedError struct {
	error
}

func (t *Template) lookup(ctx context.Context, node ast.Node, data any) (any, error) {
	v, err := t.extract(ctx, node, data)
	if err != nil {
		return nil, err
	}
	return Execute(ctx, v, data)
}

func (t *Template) extract(ctx context.Context, node ast.Node, data any) (any, error) {
	q, err := t.buildQuery(ctx, queryutil.New(), node, data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create query from AST")
	}

	f, err := q.Extract(functions)
	if err == nil {
		return f, nil
	}
	v, err := q.Extract(typeFunctions)
	if err == nil {
		return v, nil
	}
	// Pass ctx so that context-aware extractors (e.g. a blocking streaming
	// response accessor) can observe the caller's deadline and cancellation.
	// Snapshot the context state first: only a context that ends DURING the
	// extraction can have interrupted a blocking wait.
	//
	// TODO(#811): this is inference, not causation — remove the snapshot and
	// classify via errors.Is on the extraction error once query-go extractors
	// can propagate typed errors (ErrNotFound vs a wrapped ctx error).
	preErr := ctx.Err()
	v, err = q.ExtractContext(ctx, data)
	if err != nil {
		// If the context ended while extracting, the failure is (or may be)
		// caused by the cancellation or deadline rather than the value being
		// genuinely absent. Report a hard error so that ?? and defined() do not
		// silently treat an interrupted wait (e.g. a streaming deadlock guard
		// firing) as "not defined". A context that had already ended before the
		// extraction started cannot have cut a wait short — e.g. bind evaluation
		// after a step timeout — so ordinary absence keeps its undefined
		// semantics there.
		if ctxErr := ctx.Err(); ctxErr != nil && preErr == nil {
			return nil, errors.Wrap(err, ctxErr.Error())
		}
		return nil, notDefinedError{err}
	}
	return v, nil
}

func (t *Template) buildQuery(ctx context.Context, q *query.Query, node ast.Node, data any) (*query.Query, error) {
	var err error
	switch n := node.(type) {
	case *ast.Ident:
		return q.Key(n.Name), nil
	case *ast.SelectorExpr:
		q, err = t.buildQuery(ctx, q, n.X, data)
		if err != nil {
			return nil, err
		}
		return q.Key(n.Sel.Name), nil
	case *ast.IndexExpr:
		q, err = t.buildQuery(ctx, q, n.X, data)
		if err != nil {
			return nil, err
		}
		return t.appendIndexQuery(ctx, q, n.Index, data)
	}
	return nil, errors.Errorf(`unknown node "%T"`, node)
}

// appendIndexQuery evaluates the index expression and appends the
// corresponding extractor to q: an integer extracts by index, a string
// extracts by key.
func (t *Template) appendIndexQuery(ctx context.Context, q *query.Query, expr ast.Expr, data any) (*query.Query, error) {
	v, err := t.executeExpr(ctx, expr, data)
	if err != nil {
		return nil, err
	}
	// JSON numbers are decoded as json.Number, whose kind is string. Interpret
	// them as integers so that an index taken from a response body works.
	if n, ok := v.(json.Number); ok {
		i, err := n.Int64()
		if err != nil {
			if errors.Is(err, strconv.ErrRange) {
				return nil, errors.Errorf("index %s overflows int", n.String())
			}
			return nil, errors.Errorf("expected an integer or string index but got %s", n.String())
		}
		v = i
	}
	rv := reflect.ValueOf(v)
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		i := rv.Int()
		if i < 0 {
			return nil, errors.Errorf("index must not be negative but got %d", i)
		}
		if i > math.MaxInt {
			return nil, errors.Errorf("index %d overflows int", i)
		}
		return q.Index(int(i)), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		u := rv.Uint()
		if u > math.MaxInt {
			return nil, errors.Errorf("index %d overflows int", u)
		}
		return q.Index(int(u)), nil
	case reflect.String:
		return q.Key(rv.String()), nil
	default:
		if v == nil {
			return nil, errors.New("expected an integer or string index but got nil")
		}
		return nil, errors.Errorf("expected an integer or string index but got %T", v)
	}
}
