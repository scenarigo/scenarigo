package errors

import (
	"fmt"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
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
		Path: fmt.Sprintf(".%s", path),
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

func WithNodeAndColored(err error, node ast.Node, colored bool) error {
	e, ok := err.(Error)
	if ok {
		e.SetNodeAndColored(node, colored)
		return e
	}
	return err
}

type Error interface {
	AppendPath(string)
	Wrapf(string, ...interface{})
	SetNodeAndColored(ast.Node, bool)
	Error() string
}

type PathError struct {
	Path         string
	Node         ast.Node
	EnabledColor bool
	Err          error
}

func (e *PathError) AppendPath(path string) {
	e.Path = path + e.Path
}

func (e *PathError) Wrapf(message string, args ...interface{}) {
	e.Err = errors.Wrapf(e.Err, message, args...)
}

func (e *PathError) SetNodeAndColored(node ast.Node, colored bool) {
	e.Node = node
	e.EnabledColor = colored
}

func (e *PathError) cloneCurrentNode() ast.Node {
	if e.Node == nil {
		return nil
	}
	file, err := parser.ParseBytes([]byte(e.Node.String()), 0)
	if err != nil {
		return nil
	}
	if len(file.Docs) == 0 {
		return nil
	}
	return file.Docs[0].Body
}

func (e *PathError) yml() string {
	// clone current node
	// because PrintErrorToken make disruptive changes token.Token
	node := e.cloneCurrentNode()
	if node == nil {
		return ""
	}
	path, err := yaml.PathString(fmt.Sprintf("$%s", e.Path))
	if path == nil || err != nil {
		return ""
	}
	filteredNode, err := path.FilterNode(node)
	if filteredNode == nil || err != nil {
		return ""
	}
	var p printer.Printer
	return p.PrintErrorToken(filteredNode.GetToken(), e.EnabledColor)
}

func (e *PathError) Error() string {
	return fmt.Sprintf("\n%s\n%s", e.yml(), e.Err.Error())
}

type MultiPathError struct {
	Node ast.Node
	Errs []error
}

func (e *MultiPathError) Error() string {
	var mulerr error
	mulerr = &multierror.Error{
		ErrorFormat: func(es []error) string {
			if len(es) == 1 {
				return fmt.Sprintf("1 error occurred:%s\n\n", strings.TrimLeft(es[0].Error(), "\t"))
			}

			points := make([]string, len(es))
			for i, err := range es {
				points[i] = fmt.Sprintf("%s", strings.TrimLeft(err.Error(), "\t"))
			}

			return fmt.Sprintf(
				"%d errors occurred:%s\n\n",
				len(es), strings.Join(points, "\n"))
		},
	}
	mulerr = multierror.Append(mulerr, e.Errs...)
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
	for idx, err := range e.Errs {
		pathErr, ok := err.(Error)
		if ok {
			pathErr.Wrapf(message, args...)
		} else {
			e.Errs[idx] = errors.Wrapf(err, message, args...)
		}
	}
}

func (e *MultiPathError) SetNodeAndColored(node ast.Node, colored bool) {
	for _, err := range e.Errs {
		e, ok := err.(Error)
		if !ok {
			continue
		}
		e.SetNodeAndColored(node, colored)
	}
}
