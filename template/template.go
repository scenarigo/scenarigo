// Package template implements data-driven templates for generating a value.
package template

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/pkg/errors"
	"github.com/zoncoen/yaml"

	"github.com/zoncoen/scenarigo/template/ast"
	"github.com/zoncoen/scenarigo/template/parser"
	"github.com/zoncoen/scenarigo/template/token"
)

// Template is the representation of a parsed template.
type Template struct {
	expr ast.Expr

	executingLeftArrowExprArg bool
}

// New parses text as a template and returns it.
func New(str string) (*Template, error) {
	p := parser.NewParser(strings.NewReader(str))
	node, err := p.Parse()
	if err != nil {
		return nil, errors.Wrapf(err, `failed to parse "%s"`, str)
	}
	expr, ok := node.(ast.Expr)
	if !ok {
		return nil, errors.Errorf(`unknown node "%T"`, node)
	}
	return &Template{
		expr: expr,
	}, nil
}

// Execute applies a parsed template to the specified data.
func (t *Template) Execute(data interface{}) (interface{}, error) {
	return t.executeExpr(t.expr, data)
}

func (t *Template) executeExpr(expr ast.Expr, data interface{}) (interface{}, error) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return t.executeBasicLit(e)
	case *ast.ParameterExpr:
		return t.executeParameterExpr(e, data)
	case *ast.BinaryExpr:
		return t.executeBinaryExpr(e, data)
	case *ast.Ident:
		return lookup(e, data)
	case *ast.SelectorExpr:
		return lookup(e, data)
	case *ast.IndexExpr:
		return lookup(e, data)
	case *ast.CallExpr:
		return t.executeFuncCall(e, data)
	case *ast.LeftArrowExpr:
		return t.executeLeftArrowExpr(e, data)
	default:
		return nil, errors.Errorf(`unknown expression "%T"`, e)
	}
}

func (t *Template) executeBasicLit(lit *ast.BasicLit) (interface{}, error) {
	switch lit.Kind {
	case token.STRING:
		return lit.Value, nil
	case token.INT:
		i, err := strconv.Atoi(lit.Value)
		if err != nil {
			return nil, errors.Wrapf(err, `invalid AST: "%s" is not a integer`, lit.Value)
		}
		return i, nil
	default:
		return nil, errors.Errorf(`unknown basic literal "%s"`, lit.Kind.String())
	}
}

func (t *Template) executeParameterExpr(e *ast.ParameterExpr, data interface{}) (interface{}, error) {
	if e.X == nil {
		return "", nil
	}
	return t.executeExpr(e.X, data)
}

func (t *Template) executeBinaryExpr(e *ast.BinaryExpr, data interface{}) (interface{}, error) {
	x, err := t.executeExpr(e.X, data)
	if err != nil {
		return nil, err
	}
	y, err := t.executeExpr(e.Y, data)
	if err != nil {
		return nil, err
	}
	switch e.Op {
	case token.ADD:
		return t.add(x, y)
	default:
		return nil, errors.Errorf(`unknown operation "%s"`, e.Op.String())
	}
}

func (t *Template) add(x, y interface{}) (interface{}, error) {
	strX, err := t.stringize(x)
	if err != nil {
		return nil, errors.Wrap(err, "failed to concat strings")
	}
	strY, err := t.stringize(y)
	if err != nil {
		return nil, errors.Wrap(err, "failed to concat strings")
	}
	return strX + strY, nil
}

func (t *Template) stringize(v interface{}) (string, error) {
	s, ok := v.(string)
	if ok {
		return s, nil
	}
	if t.executingLeftArrowExprArg {
		b, err := yaml.Marshal(v)
		if err != nil {
			return "", errors.Wrap(err, "failed to marshal")
		}
		return string(b), nil
	}
	return "", errors.Errorf("expect string but got %T", v)
}

func (t *Template) executeFuncCall(call *ast.CallExpr, data interface{}) (interface{}, error) {
	fun, err := t.executeExpr(call.Fun, data)
	if err != nil {
		return nil, err
	}
	args := make([]reflect.Value, len(call.Args))
	for i, arg := range call.Args {
		a, err := t.executeExpr(arg, data)
		if err != nil {
			return nil, err
		}
		args[i] = reflect.ValueOf(a)
	}

	funv := reflect.ValueOf(fun)
	if funv.Kind() != reflect.Func {
		return nil, errors.Errorf("not function")
	}
	vs := funv.Call(args)
	if len(vs) != 1 || !vs[0].IsValid() {
		return nil, errors.Errorf("function should return a value")
	}
	return vs[0].Interface(), nil
}

func (t *Template) executeLeftArrowExpr(e *ast.LeftArrowExpr, data interface{}) (interface{}, error) {
	v, err := t.executeExpr(e.Fun, data)
	if err != nil {
		return nil, err
	}
	f, ok := v.(Func)
	if !ok {
		return nil, errors.Errorf(`expect template function but got %T`, e)
	}

	v, err = t.executeLeftArrowExprArg(e.Arg, data)
	if err != nil {
		return nil, err
	}
	argStr, ok := v.(string)
	if !ok {
		return nil, errors.Errorf(`expect string but got %T`, v)
	}
	arg, err := f.UnmarshalArg(func(v interface{}) error {
		return yaml.Unmarshal([]byte(argStr), v)
	})
	if err != nil {
		return nil, err
	}
	return f.Exec(arg)
}

func (t *Template) executeLeftArrowExprArg(arg ast.Expr, data interface{}) (interface{}, error) {
	tt := &Template{
		expr:                      arg,
		executingLeftArrowExprArg: true,
	}
	return tt.Execute(data)
}

// Func represents a left arrow function.
type Func interface {
	Exec(arg interface{}) (interface{}, error)
	UnmarshalArg(unmarshal func(interface{}) error) (interface{}, error)
}
