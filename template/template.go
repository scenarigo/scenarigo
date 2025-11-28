// Package template implements data-driven templates for generating a value.
package template

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/pkg/errors"
	"github.com/scenarigo/scenarigo/internal/reflectutil"
	"github.com/scenarigo/scenarigo/template/ast"
	"github.com/scenarigo/scenarigo/template/parser"
	"github.com/scenarigo/scenarigo/template/token"
	"github.com/scenarigo/scenarigo/template/val"
)

// Template is the representation of a parsed template.
type Template struct {
	str  string
	expr ast.Expr

	executingLeftArrowExprArg bool
	argFuncs                  *funcStash
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
		str:      str,
		expr:     expr,
		argFuncs: &funcStash{},
	}, nil
}

// Execute applies a parsed template to the specified data.
// If the Lazy isn't called, the context value should be canceled to avoid a goroutine leak.
func (t *Template) Execute(ctx context.Context, data any) (any, error) {
	return t.executeLazyTemplate(ctx, data)
}

func (t *Template) execute(ctx context.Context, data any) (_ any, retErr error) {
	defer func() {
		if err := recover(); err != nil {
			retErr = fmt.Errorf("failed to execute: panic: %s", err)
		}
	}()
	v, err := t.executeExpr(ctx, t.expr, data)
	if err != nil {
		if strings.Contains(t.str, "\n") {
			return nil, errors.Wrapf(err, "failed to execute: \n%s\n", t.str) //nolint:revive
		}
		return nil, errors.Wrapf(err, "failed to execute: %s", t.str)
	}
	return v, nil
}

func (t *Template) executeExpr(ctx context.Context, expr ast.Expr, data any) (any, error) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		return t.executeBasicLit(e)
	case *ast.ParameterExpr:
		return t.executeParameterExpr(ctx, e, data)
	case *ast.ParenExpr:
		return t.executeExpr(ctx, e.X, data)
	case *ast.UnaryExpr:
		v, err := t.executeUnaryExpr(ctx, e, data)
		if err != nil {
			return nil, fmt.Errorf("invalid operation: %w", err)
		}
		return v, nil
	case *ast.BinaryExpr:
		v, err := t.executeBinaryExpr(ctx, e, data)
		if err != nil {
			return nil, fmt.Errorf("invalid operation: %w", err)
		}
		return v, nil
	case *ast.ConditionalExpr:
		return t.executeConditionalExpr(ctx, e, data)
	case *ast.Ident:
		return lookup(ctx, e, data)
	case *ast.SelectorExpr:
		return lookup(ctx, e, data)
	case *ast.IndexExpr:
		return lookup(ctx, e, data)
	case *ast.CallExpr:
		return t.executeFuncCall(ctx, e, data)
	case *ast.LeftArrowExpr:
		return t.executeLeftArrowExpr(ctx, e, data)
	case *ast.DefinedExpr:
		return t.executeDefinedExpr(e, data)
	default:
		return nil, errors.Errorf(`unknown expression "%T"`, e)
	}
}

func (t *Template) executeBasicLit(lit *ast.BasicLit) (any, error) {
	switch lit.Kind {
	case token.STRING:
		return lit.Value, nil
	case token.INT:
		i, err := strconv.ParseInt(lit.Value, 0, 64)
		if err != nil {
			return nil, errors.Wrapf(err, `invalid AST: "%s" is not an integer`, lit.Value)
		}
		return i, nil
	case token.FLOAT:
		f, err := strconv.ParseFloat(lit.Value, 64)
		if err != nil {
			return nil, errors.Wrapf(err, `invalid AST: "%s" is not a float`, lit.Value)
		}
		return f, nil
	case token.BOOL:
		switch lit.Value {
		case "true":
			return true, nil
		case "false":
			return false, nil
		default:
			return nil, errors.Errorf(`invalid bool literal "%s"`, lit.Value)
		}
	default:
		return nil, errors.Errorf(`unknown basic literal "%s"`, lit.Kind.String())
	}
}

func (t *Template) executeParameterExpr(ctx context.Context, e *ast.ParameterExpr, data any) (any, error) {
	if e.X == nil {
		return "", nil
	}
	v, err := t.executeExpr(ctx, e.X, data)
	if err != nil {
		return nil, err
	}
	if t.executingLeftArrowExprArg {
		// HACK: Left arrow function requires its argument is YAML string to unmarshal into the Go value.
		// Replace the function into a string temporary.
		// It will be restored in UnmarshalArg method.
		if reflectutil.Elem(reflect.ValueOf(v)).Kind() == reflect.Func {
			name := t.argFuncs.save(v)
			if e.Quoted {
				return fmt.Sprintf("'{{%s}}'", name), nil
			}
			return fmt.Sprintf("{{%s}}", name), nil
		}

		// Left arrow function arguments must be a string in YAML.
		// If the `v` is a map or slice(array) instance, directly concatenating the output string would result in something like
		// "a: key: value", which is invalid YAML.
		// To resolve this problem, flow mode is used to create valid YAML, such as 'a: {key: value}'.
		b, err := yaml.MarshalWithOptions(v, yaml.Flow(true))
		if err != nil {
			return nil, err
		}
		return strings.TrimSuffix(string(b), "\n"), nil
	}
	return v, nil
}

func typeValue(v val.Value) string {
	if _, ok := v.(val.Nil); ok {
		return "nil"
	}
	return fmt.Sprintf("%s(%v)", v.Type().Name(), v.GoValue())
}

func (t *Template) executeUnaryExpr(ctx context.Context, e *ast.UnaryExpr, data any) (any, error) {
	x, err := t.executeExpr(ctx, e.X, data)
	if err != nil {
		return nil, err
	}
	xv := val.NewValue(x)
	v, err := t.executeUnaryOperation(e.Op, xv)
	if err != nil {
		if errors.Is(err, val.ErrOperationNotDefined) {
			return nil, fmt.Errorf("operator %s not defined on %s", e.Op, typeValue(xv))
		}
		return nil, err
	}
	return v.GoValue(), err
}

func (t *Template) executeUnaryOperation(op token.Token, x val.Value) (val.Value, error) {
	switch op {
	case token.SUB:
		if o, ok := x.(val.Negator); ok {
			return o.Neg()
		}
	case token.NOT:
		if o, ok := x.(val.LogicalValue); ok {
			return val.Bool(!o.IsTruthy()), nil
		}
	}
	return nil, val.ErrOperationNotDefined
}

func (t *Template) executeBinaryExpr(ctx context.Context, e *ast.BinaryExpr, data any) (any, error) {
	// coalescing expr allows undefined variables as the left-hand side expr
	if e.Op == token.COALESCING {
		return t.executeCoalescingExpr(ctx, e, data)
	}

	x, err := t.executeExpr(ctx, e.X, data)
	if err != nil {
		return nil, err
	}
	y, err := t.executeExpr(ctx, e.Y, data)
	if err != nil {
		return nil, err
	}
	xv, yv := val.NewValue(x), val.NewValue(y)
	v, err := t.executeBinaryOperation(e.Op, xv, yv, e.Y)
	if err != nil {
		if errors.Is(err, val.ErrOperationNotDefined) {
			return nil, fmt.Errorf("%s %s %s not defined", typeValue(xv), e.Op, typeValue(yv))
		}
		return nil, err
	}
	return v.GoValue(), nil
}

//nolint:gocyclo,cyclop,maintidx
func (t *Template) executeBinaryOperation(op token.Token, x, y val.Value, yExpr ast.Expr) (val.Value, error) {
	switch op {
	case token.EQL:
		if o, ok := x.(val.Equaler); ok {
			return o.Equal(y)
		}
	case token.NEQ:
		if o, ok := x.(val.Equaler); ok {
			b, err := o.Equal(y)
			if err != nil {
				return nil, err
			}
			return val.Bool(!b.IsTruthy()), nil
		}
	case token.LSS:
		if o, ok := x.(val.Comparer); ok {
			i, err := o.Compare(y)
			if err != nil {
				return nil, err
			}
			return oneOf(i, val.Int(-1))
		}
	case token.LEQ:
		if o, ok := x.(val.Comparer); ok {
			i, err := o.Compare(y)
			if err != nil {
				return nil, err
			}
			return oneOf(i, val.Int(-1), val.Int(0))
		}
	case token.GTR:
		if o, ok := x.(val.Comparer); ok {
			i, err := o.Compare(y)
			if err != nil {
				return nil, err
			}
			return oneOf(i, val.Int(1))
		}
	case token.GEQ:
		if o, ok := x.(val.Comparer); ok {
			i, err := o.Compare(y)
			if err != nil {
				return nil, err
			}
			return oneOf(i, val.Int(1), val.Int(0))
		}
	case token.CONCAT:
		if _, ok := x.(val.String); !ok {
			if v, err := val.GetType("string").Convert(x); err == nil {
				x = v
			}
		}
		if _, ok := y.(val.String); !ok {
			if v, err := val.GetType("string").Convert(y); err == nil {
				y = v
			}
		}
		fallthrough
	case token.ADD:
		if o, ok := x.(val.Adder); ok {
			// hack for strings of left arrow functions
			if t.executingLeftArrowExprArg {
				if _, ok := (yExpr).(*ast.ParameterExpr); ok {
					if xs, ok := x.(val.String); ok {
						if ys, ok := y.(val.String); ok {
							var err error
							s, err := t.addIndent(string(ys), string(xs))
							if err != nil {
								return nil, errors.Wrap(err, "failed to concat strings")
							}
							y = val.String(s)
						}
					}
				}
			}
			return o.Add(y)
		}
	case token.SUB:
		if o, ok := x.(val.Subtractor); ok {
			return o.Sub(y)
		}
	case token.MUL:
		if o, ok := x.(val.Multiplier); ok {
			return o.Mul(y)
		}
	case token.QUO:
		if o, ok := x.(val.Divider); ok {
			return o.Div(y)
		}
	case token.REM:
		if o, ok := x.(val.Modder); ok {
			return o.Mod(y)
		}
	case token.LAND:
		if o, ok := x.(val.LogicalValue); ok {
			if ylv, ok := y.(val.LogicalValue); ok {
				return val.Bool(o.IsTruthy() && ylv.IsTruthy()), nil
			}
		}
	case token.LOR:
		if o, ok := x.(val.LogicalValue); ok {
			if ylv, ok := y.(val.LogicalValue); ok {
				return val.Bool(o.IsTruthy() || ylv.IsTruthy()), nil
			}
		}
	}
	return nil, val.ErrOperationNotDefined
}

func (t *Template) executeCoalescingExpr(ctx context.Context, e *ast.BinaryExpr, data any) (any, error) {
	switch e.X.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr:
		extracted, err := extract(e.X, data)
		if err != nil {
			var notDefined notDefinedError
			if errors.As(err, &notDefined) {
				return t.executeExpr(ctx, e.Y, data)
			}
			return nil, err
		}
		if extracted == nil || isNil(reflect.ValueOf(extracted)) {
			return t.executeExpr(ctx, e.Y, data)
		}
		return extracted, nil
	default:
		x, err := t.executeExpr(ctx, e.X, data)
		if err != nil {
			return nil, err
		}
		if x == nil || isNil(reflect.ValueOf(x)) {
			return t.executeExpr(ctx, e.Y, data)
		}
		return x, nil
	}
}

func oneOf(x val.Value, ys ...val.Value) (val.Bool, error) {
	if xv, ok := x.(val.Equaler); ok {
		for _, yv := range ys {
			b, err := xv.Equal(yv)
			if err != nil {
				return val.Bool(false), err
			}
			if b.IsTruthy() {
				return val.Bool(true), nil
			}
		}
		return val.Bool(false), nil
	}
	return val.Bool(false), val.ErrOperationNotDefined
}

func (t *Template) executeConditionalExpr(ctx context.Context, e *ast.ConditionalExpr, data any) (any, error) {
	c, err := t.executeExpr(ctx, e.Condition, data)
	if err != nil {
		return nil, err
	}
	cv := val.NewValue(c)
	cond, ok := cv.(val.LogicalValue)
	if !ok {
		return nil, fmt.Errorf("invalid operation: operator ? not defined on %s", typeValue(cv))
	}
	if cond.IsTruthy() {
		return t.executeExpr(ctx, e.X, data)
	}
	return t.executeExpr(ctx, e.Y, data)
}

// align indents of marshaled texts
//
//	example: addIndent("a: 1\nb:2", "- ")
//	=== before ===
//	- a: 1
//	b: 2
//	=== after ===
//	- a: 1
//	  b: 2
func (t *Template) addIndent(str, preStr string) (string, error) {
	if t.executingLeftArrowExprArg {
		if strings.ContainsRune(str, '\n') && preStr != "" {
			lines := strings.Split(preStr, "\n")
			prefix := strings.Repeat(" ", len([]rune(lines[len(lines)-1])))
			var b strings.Builder
			for i, s := range strings.Split(str, "\n") {
				if i != 0 {
					if _, err := b.WriteRune('\n'); err != nil {
						return "", err
					}
					if s != "" {
						if _, err := b.WriteString(prefix); err != nil {
							return "", err
						}
					}
				}
				if _, err := b.WriteString(s); err != nil {
					return "", err
				}
			}
			return b.String(), nil
		}
	}
	return str, nil
}

func (t *Template) executeFuncCall(ctx context.Context, call *ast.CallExpr, data any) (any, error) {
	var fn reflect.Value
	fnName := "function"
	args := make([]reflect.Value, 0, len(call.Args)+1)
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if ok {
		x, err := t.executeExpr(ctx, selector.X, data)
		if err != nil {
			return nil, err
		}
		v, err := lookup(ctx, selector.Sel, x)
		if err == nil {
			fn = reflect.ValueOf(v)
		} else {
			r, m, ok := getMethod(reflect.ValueOf(x), selector.Sel.Name)
			if !ok {
				return nil, err
			}
			fn = m.Func
			args = append(args, r)
		}
		fnName = selector.Sel.Name
	} else {
		f, err := t.executeExpr(ctx, call.Fun, data)
		if err != nil {
			return nil, err
		}
		fn = reflect.ValueOf(f)
		if id, ok := call.Fun.(*ast.Ident); ok {
			fnName = id.Name
		}
	}
	if fn.Kind() != reflect.Func {
		if r, m, ok := getMethod(fn, "Call"); ok {
			fn = m.Func
			args = append(args, r)
		} else {
			return nil, errors.Errorf("not function")
		}
	}
	fnType := fn.Type()
	args, err := t.buildFuncCallArgs(ctx, data, fnName, fnType, args, call.Args)
	if err != nil {
		return nil, err
	}

	vs := fn.Call(args)
	switch len(vs) {
	case 1:
		if !vs[0].IsValid() || !vs[0].CanInterface() {
			return nil, errors.Errorf("function returns an invalid value")
		}
		return vs[0].Interface(), nil
	case 2:
		if !vs[0].IsValid() || !vs[0].CanInterface() {
			return nil, errors.Errorf("first reruned value is invalid")
		}
		if !vs[1].IsValid() || !vs[1].CanInterface() {
			return nil, errors.Errorf("second reruned value is invalid")
		}
		if vs[1].Type() != reflectutil.TypeError {
			return nil, errors.Errorf("second returned value must be an error")
		}
		if !vs[1].IsNil() {
			return nil, vs[1].Interface().(error) //nolint:forcetypeassert
		}
		return vs[0].Interface(), nil
	default:
		return nil, errors.Errorf("function should return a value or a value and an error")
	}
}

func getMethod(in reflect.Value, name string) (reflect.Value, *reflect.Method, bool) {
	r := reflectutil.Elem(in)
	m, ok := r.Type().MethodByName(name)
	if ok {
		return r, &m, true
	}
	if r.CanAddr() {
		r = r.Addr()
		m, ok := r.Type().MethodByName(name)
		if ok {
			return r, &m, true
		}
	} else {
		ptr := makePtr(r)
		m, ok := ptr.Type().MethodByName(name)
		if ok {
			return ptr, &m, true
		}
	}
	return reflect.Value{}, nil, false
}

func (t *Template) buildFuncCallArgs(
	ctx context.Context,
	data any,
	fnName string,
	fnType reflect.Type,
	baseArgs []reflect.Value,
	callArgs []ast.Expr,
) ([]reflect.Value, error) {
	values := make([]reflect.Value, len(callArgs))
	for i, arg := range callArgs {
		a, err := t.executeExpr(ctx, arg, data)
		if err != nil {
			return nil, err
		}
		if a == nil {
			values[i] = reflect.Value{}
			continue
		}
		values[i] = reflect.ValueOf(a)
	}

	// Pull scenarigo context from template execution context so we can inject it
	// into plugin helpers transparently.
	ctxVal, hasCtx := contextValueFromExecutionContext(ctx)
	minArgs := minRequiredArgs(fnType, len(baseArgs), ctxVal)
	if fnType.IsVariadic() {
		if len(values) < minArgs {
			return nil, errors.Errorf(
				"too few arguments to function: expected minimum argument number is %d. but specified %d arguments",
				minArgs, len(values),
			)
		}
	} else {
		maxArgs := fnType.NumIn() - len(baseArgs)
		if len(values) < minArgs {
			return nil, errors.Errorf(
				"too few arguments to function: expected minimum argument number is %d. but specified %d arguments",
				minArgs, len(values),
			)
		}
		if len(values) > maxArgs {
			return nil, errors.Errorf(
				"expected function argument number is %d but specified %d arguments",
				maxArgs, len(values),
			)
		}
	}

	args := append([]reflect.Value{}, baseArgs...)
	userIdx := 0
	fixedParamTotal := fnType.NumIn()
	isVariadic := fnType.IsVariadic()
	if isVariadic {
		fixedParamTotal--
	}
	for paramPos := len(baseArgs); paramPos < fixedParamTotal; paramPos++ {
		paramType := fnType.In(paramPos)
		if hasCtx && shouldInjectContext(paramType, ctxVal, values, userIdx) {
			args = append(args, ctxVal)
			continue
		}
		if userIdx >= len(values) {
			return nil, errors.Errorf(
				"too few arguments to function: expected minimum argument number is %d. but specified %d arguments",
				minArgs, len(values),
			)
		}
		v, err := convertArgument(fnName, paramType, values[userIdx], userIdx)
		if err != nil {
			return nil, err
		}
		args = append(args, v)
		userIdx++
	}

	if isVariadic {
		elemType := fnType.In(fnType.NumIn() - 1).Elem()
		// Remaining user arguments populate the variadic tail.
		for ; userIdx < len(values); userIdx++ {
			v, err := convertArgument(fnName, elemType, values[userIdx], userIdx)
			if err != nil {
				return nil, err
			}
			args = append(args, v)
		}
		return args, nil
	}

	return args, nil
}

func shouldInjectContext(paramType reflect.Type, ctxVal reflect.Value, values []reflect.Value, userIdx int) bool {
	if !ctxVal.IsValid() || !ctxVal.Type().AssignableTo(paramType) {
		return false
	}
	if userIdx < len(values) && isAssignable(values[userIdx], paramType) {
		return false
	}
	return true
}

func contextValueFromExecutionContext(ctx context.Context) (reflect.Value, bool) {
	v := getExecutionContextValue(ctx)
	if v == nil {
		return reflect.Value{}, false
	}
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return reflect.Value{}, false
	}
	return rv, true
}

func convertArgument(fnName string, requiredType reflect.Type, v reflect.Value, argIdx int) (reflect.Value, error) {
	if !v.IsValid() {
		if isNilable(requiredType) {
			return reflect.Zero(requiredType), nil
		}
		return reflect.Value{}, errors.Errorf("can't use nil as %s in arguments[%d] to %s", requiredType, argIdx, fnName)
	}
	vv, ok, _ := reflectutil.Convert(requiredType, v)
	if ok {
		v = vv
	}
	if !v.IsValid() {
		return reflect.Value{}, errors.Errorf("can't use nil as %s in arguments[%d] to %s", requiredType, argIdx, fnName)
	}
	if v.Type() != requiredType {
		return reflect.Value{}, errors.Errorf("can't use %s as %s in arguments[%d] to %s", v.Type(), requiredType, argIdx, fnName)
	}
	return v, nil
}

func minRequiredArgs(fnType reflect.Type, baseArgs int, ctxVal reflect.Value) int {
	total := 0
	fixedParamTotal := fnType.NumIn()
	if fnType.IsVariadic() {
		fixedParamTotal--
	}
	for paramPos := baseArgs; paramPos < fixedParamTotal; paramPos++ {
		paramType := fnType.In(paramPos)
		if ctxVal.IsValid() && ctxVal.Type().AssignableTo(paramType) {
			continue
		}
		total++
	}
	return total
}

func isNilable(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return true
	default:
		return false
	}
}

func isAssignable(v reflect.Value, t reflect.Type) bool {
	if !v.IsValid() {
		return false
	}
	return v.Type().AssignableTo(t)
}

func (t *Template) executeLeftArrowExpr(ctx context.Context, e *ast.LeftArrowExpr, data any) (any, error) {
	v, err := t.executeExpr(ctx, e.Fun, data)
	if err != nil {
		return nil, err
	}
	f, ok := v.(Func)
	if !ok {
		return nil, errors.Errorf(`expect template function but got %T`, e)
	}

	// without arg in map key
	if e.Arg == nil {
		return &FuncCall{f}, nil
	}

	v, err = t.executeLeftArrowExprArg(ctx, e.Arg, data)
	if err != nil {
		return nil, err
	}
	argStr, ok := v.(string)
	if !ok {
		return nil, errors.Errorf(`expect string but got %T`, v)
	}
	arg, err := f.UnmarshalArg(func(v any) error {
		if err := yaml.NewDecoder(strings.NewReader(argStr), yaml.UseOrderedMap(), yaml.Strict()).Decode(v); err != nil {
			return err
		}
		// Restore functions that are replaced into strings.
		// See the "HACK" comment of *Template.executeParameterExpr method.
		arg, err := Execute(ctx, v, t.argFuncs)
		if err != nil {
			return err
		}
		// NOTE: Decode method ensures that v is a pointer.
		rv := reflect.ValueOf(v).Elem()
		ev, err := convert(rv.Type())(reflect.ValueOf(arg), nil)
		if err != nil {
			return err
		}
		rv.Set(ev)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return f.Exec(arg)
}

func (t *Template) executeDefinedExpr(e *ast.DefinedExpr, data any) (any, error) {
	switch e.Arg.(type) {
	case *ast.Ident, *ast.SelectorExpr, *ast.IndexExpr:
		if _, err := extract(e.Arg, data); err != nil {
			var notDefined notDefinedError
			if errors.As(err, &notDefined) {
				return false, nil
			}
			return nil, err
		}
		return true, nil
	}
	return nil, errors.New("invalid argument to defined()")
}

func (t *Template) executeLeftArrowExprArg(ctx context.Context, arg ast.Expr, data any) (any, error) {
	tt := &Template{
		expr:                      arg,
		executingLeftArrowExprArg: true,
		argFuncs:                  t.argFuncs,
	}
	v, err := tt.Execute(ctx, data)
	return v, err
}

type funcStash map[string]any

func (s *funcStash) save(f any) string {
	if *s == nil {
		*s = funcStash{}
	}
	name := fmt.Sprintf("func-%d", len(*s))
	(*s)[name] = f
	return name
}

// Func represents a left arrow function.
type Func interface {
	Exec(arg any) (any, error)
	UnmarshalArg(unmarshal func(any) error) (any, error)
}

// FuncCall represents a left arrow function call like '{{func <-}}'.
type FuncCall struct {
	Func
}

// Do executes the function f.
// This function executes templates of the argument v with data before calling the function f.
func (f *FuncCall) Do(ctx context.Context, v, data any) (any, error) {
	val, err := executeLeftArrowFunction(ctx, f.Func, reflect.ValueOf(v), data, "")
	if err != nil {
		return nil, err
	}
	if val.IsValid() && val.CanInterface() {
		return val.Interface(), nil
	}
	return nil, nil //nolint:nilnil
}
