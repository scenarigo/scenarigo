package errors

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/printer"
	"github.com/hashicorp/go-multierror"
	"github.com/pkg/errors"
	"github.com/zoncoen/query-go"
)

func Errorf(format string, args ...interface{}) error {
	return errors.Errorf(format, args...)
}

func ErrorPathf(path, msg string, args ...interface{}) error {
	return &PathError{
		Path: fmt.Sprintf(".%s", path),
		Err:  errors.Errorf(msg, args...),
	}
}

func ErrorQueryf(q *query.Query, format string, args ...interface{}) error {
	return &PathError{
		Path: q.String(),
		Err:  errors.Errorf(format, args...),
	}
}

func New(message string) error {
	return errors.New(message)
}

func ErrorPath(path, message string) error {
	return &PathError{
		Path: path,
		Err:  errors.New(message),
	}
}

func Errors(errs ...error) error {
	return &MultiPathError{Errs: errs}
}

func Wrap(err error, message string) error {
	e, ok := err.(Error)
	if ok {
		e.Wrapf(message)
		return e
	}
	return &PathError{
		Err: errors.Wrap(err, message),
	}
}

func WrapPath(err error, path, message string) error {
	e, ok := err.(Error)
	if ok {
		e.Wrapf(message)
		e.AppendPath(fmt.Sprintf(".%s", path))
		return e
	}
	return &PathError{
		Err:  errors.Wrap(err, message),
		Path: fmt.Sprintf(".%s", path),
	}
}

func Wrapf(err error, format string, args ...interface{}) error {
	e, ok := err.(Error)
	if ok {
		e.Wrapf(format, args...)
		return e
	}
	return &PathError{
		Err: errors.Wrapf(err, format, args...),
	}
}

func WrapPathf(err error, path, message string, args ...interface{}) error {
	e, ok := err.(Error)
	if ok {
		e.Wrapf(message, args...)
		e.AppendPath(fmt.Sprintf(".%s", path))
		return e
	}
	return &PathError{
		Err:  errors.Wrapf(err, message, args...),
		Path: fmt.Sprintf(".%s", path),
	}
}

// WithPath add path to error
func WithPath(err error, path string) error {
	e, ok := err.(Error)
	if ok {
		e.AppendPath(fmt.Sprintf(".%s", path))
		return e
	}
	return &PathError{
		Err:  err,
		Path: fmt.Sprintf(".%s", path),
	}
}

func WithQuery(err error, q *query.Query) error {
	e, ok := err.(Error)
	if ok {
		e.AppendPath(q.String())
		return e
	}
	return &PathError{
		Err:  err,
		Path: q.String(),
	}
}

func WithNode(err error, node ast.Node) error {
	e, ok := err.(Error)
	if ok {
		e.SetNode(node)
		return e
	}
	return err
}

type Error interface {
	AppendPath(string)
	Wrapf(string, ...interface{})
	SetNode(ast.Node)
	Error() string
}

type PathError struct {
	Path string
	Node ast.Node
	Err  error
}

func (e *PathError) AppendPath(path string) {
	e.Path = path + e.Path
}

func (e *PathError) Wrapf(message string, args ...interface{}) {
	e.Err = errors.Wrapf(e.Err, message, args...)
}

func (e *PathError) SetNode(node ast.Node) {
	e.Node = node
}

func (e *PathError) yml() string {
	if e.Node == nil {
		return ""
	}
	path, err := yaml.PathString(fmt.Sprintf("$%s", e.Path))
	fmt.Println("path = ", path)
	if path == nil || err != nil {
		return ""
	}
	node, err := path.FilterNode(e.Node)
	if node == nil || err != nil {
		return ""
	}
	var p printer.Printer
	return p.PrintErrorToken(node.GetToken(), true)
}

func (e *PathError) Error() string {
	return fmt.Sprintf("\t%s\n%s", e.yml(), e.Err.Error())
}

type MultiPathError struct {
	Node ast.Node
	Errs []error
	err  error
}

func (e *MultiPathError) Error() string {
	prefix := ""
	if e.err != nil {
		prefix = e.err.Error()
	}
	var mulerr error
	mulerr = &multierror.Error{
		ErrorFormat: func(es []error) string {
			if len(es) == 1 {
				return fmt.Sprintf("1 error occurred:\n%s\n\n", strings.TrimLeft(es[0].Error(), "\t"))
			}

			points := make([]string, len(es))
			for i, err := range es {
				points[i] = fmt.Sprintf("%s", strings.TrimLeft(err.Error(), "\t"))
			}

			return fmt.Sprintf(
				"%d errors occurred:\n%s\n\n",
				len(es), strings.Join(points, "\n"))
		},
	}
	for _, err := range e.Errs {
		mulerr = multierror.Append(mulerr, errors.Errorf("%s%s", prefix, err.Error()))
	}
	return mulerr.Error()
}

func (e *MultiPathError) AppendPath(path string) {
	for _, err := range e.Errs {
		e, ok := err.(Error)
		if !ok {
			continue
		}
		e.AppendPath(path)
	}
}

func (e *MultiPathError) Wrapf(message string, args ...interface{}) {
	if e.err == nil {
		e.err = errors.New("")
	}
	e.err = errors.Wrapf(e.err, message, args...)
}

func (e *MultiPathError) SetNode(node ast.Node) {
	for _, err := range e.Errs {
		e, ok := err.(Error)
		if !ok {
			continue
		}
		e.SetNode(node)
	}
}
