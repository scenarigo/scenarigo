package template

import (
	"context"
	"strconv"

	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"

	"github.com/scenarigo/scenarigo/internal/queryutil"
	"github.com/scenarigo/scenarigo/template/ast"
	"github.com/scenarigo/scenarigo/template/token"
)

type errNotDefined struct {
	error
}

func lookup(ctx context.Context, node ast.Node, data interface{}) (interface{}, error) {
	v, err := extract(node, data)
	if err != nil {
		return nil, err
	}
	return Execute(ctx, v, data)
}

func extract(node ast.Node, data interface{}) (interface{}, error) {
	q, err := buildQuery(queryutil.New(), node)
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
	v, err = q.Extract(data)
	if err != nil {
		return nil, errNotDefined{err}
	}
	return v, nil
}

func buildQuery(q *query.Query, node ast.Node) (*query.Query, error) {
	var err error
	switch n := node.(type) {
	case *ast.Ident:
		return q.Key(n.Name), nil
	case *ast.SelectorExpr:
		q, err = buildQuery(q, n.X)
		if err != nil {
			return nil, err
		}
		return q.Key(n.Sel.Name), nil
	case *ast.IndexExpr:
		i, ok := n.Index.(*ast.BasicLit)
		if !ok || i.Kind != token.INT {
			return nil, errors.Errorf(`expected int but "%s"`, i.Kind.String())
		}
		idx, err := strconv.Atoi(i.Value)
		if err != nil {
			return nil, errors.Errorf(`expected int but "%s"`, i.Value)
		}
		q, err = buildQuery(q, n.X)
		if err != nil {
			return nil, err
		}
		return q.Index(idx), nil
	}
	return nil, errors.Errorf(`unknown node "%T"`, node)
}
