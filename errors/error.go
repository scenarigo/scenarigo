package errors

import (
	"errors"
	"fmt"
	"strings"

	"github.com/fatih/color"
	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/printer"
	"github.com/hashicorp/go-multierror"
	pkgerrors "github.com/pkg/errors"
	"github.com/zoncoen/query-go"
)

// Errorf call fmt.Errorf.
func Errorf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// New call errors.New.
func New(message string) error {
	return errors.New(message)
}

// Is call errors.Is.
func Is(err, target error) bool {
	return errors.Is(err, target)
}

// As call errors.As.
func As(err error, target any) bool {
	return errors.As(err, target)
}

// ErrorPathf create PathError instance with path and error message.
func ErrorPathf(path, msg string, args ...any) error {
	pe := &PathError{
		Err: Errorf(msg, args...),
	}
	pe.prependPath(path)
	return pe
}

// ErrorQueryf create PathError instance by query.Query.
func ErrorQueryf(q *query.Query, format string, args ...any) error {
	pe := &PathError{
		Err: Errorf(format, args...),
	}
	pe.prependPath(q.String())
	return pe
}

// ErrorPath create PathError instance with path and error message.
func ErrorPath(path, message string) error {
	pe := &PathError{
		Err: New(message),
	}
	pe.prependPath(path)
	return pe
}

// Errors create MultiPathError by error instances.
func Errors(errs ...error) error {
	if len(errs) == 0 {
		return nil
	}
	return &MultiPathError{Errs: errs}
}

// Wrap wrap error while paying attention to PathError and MultiPathError.
func Wrap(err error, message string) error {
	var e Error
	if errors.As(err, &e) {
		e.wrapf(message)
		return e
	}
	return &PathError{
		Err: pkgerrors.Wrap(err, message),
	}
}

// WrapPath wrap error with path while paying attention to PathError and MultiPathError.
func WrapPath(err error, path, message string) error {
	var e Error
	if errors.As(err, &e) {
		e.wrapf(message)
		e.prependPath(path)
		return e
	}
	pe := &PathError{
		Err: Wrap(err, message),
	}
	pe.prependPath(path)
	return pe
}

// Wrapf wrap error while paying attention to PathError and MultiPathError.
func Wrapf(err error, format string, args ...any) error {
	var e Error
	if errors.As(err, &e) {
		e.wrapf(format, args...)
		return e
	}
	return &PathError{
		Err: pkgerrors.Wrapf(err, format, args...),
	}
}

// WrapPathf wrap error with path while paying attention to PathError and MultiPathError.
func WrapPathf(err error, path, message string, args ...any) error {
	var e Error
	if errors.As(err, &e) {
		e.wrapf(message, args...)
		e.prependPath(path)
		return e
	}
	pe := &PathError{
		Err: Wrapf(err, message, args...),
	}
	pe.prependPath(path)
	return pe
}

// WithPath add path to error if errors instance is PathError or MultiPathError.
func WithPath(err error, path string) error {
	var e Error
	if errors.As(err, &e) {
		e.prependPath(path)
		return e
	}
	pe := &PathError{
		Err: err,
	}
	pe.prependPath(path)
	return pe
}

// WithQuery add path by query.Query to error if errors instance is PathError or MultiPathError.
func WithQuery(err error, q *query.Query) error {
	var e Error
	if errors.As(err, &e) {
		e.prependPath(q.String())
		return e
	}
	pe := &PathError{
		Err: err,
	}
	pe.prependPath(q.String())
	return pe
}

// WithNode set ast.Node to error if errors instance is PathError or MultiPathError.
func WithNode(err error, node ast.Node) error {
	return WithNodeAndColored(err, node, !color.NoColor)
}

// WithNodeAndColored set ast.Node and colored to error if errors instance is PathError or MultiPathError.
func WithNodeAndColored(err error, node ast.Node, colored bool) error {
	var e Error
	if errors.As(err, &e) {
		e.setNodeAndColored(node, colored)
		return e
	}
	return err
}

// ReplacePath replaces the path if errors instance is PathError or MultiPathError.
func ReplacePath(err error, old, newPath string) error {
	var e Error
	if errors.As(err, &e) {
		e.replacePath(old, newPath)
		return e
	}
	return err
}

// Error represents interface for PathError and MultiPathError.
type Error interface {
	prependPath(string)
	replacePath(string, string)
	wrapf(string, ...any)
	setNodeAndColored(ast.Node, bool)
	Error() string
}

// PathError represents error with path.
type PathError struct {
	Path         string
	Node         ast.Node
	EnabledColor bool
	Err          error
}

func (e *PathError) prependPath(path string) {
	if path == "" {
		return
	}
	if !strings.HasPrefix(path, ".") && !strings.HasPrefix(path, "[") {
		path = fmt.Sprintf(".%s", path)
	}
	e.Path = path + e.Path
}

func (e *PathError) replacePath(old, newPath string) {
	e.Path = strings.Replace(e.Path, old, newPath, 1)
}

func (e *PathError) wrapf(message string, args ...any) {
	e.Err = Wrapf(e.Err, message, args...)
}

func (e *PathError) setNodeAndColored(node ast.Node, colored bool) {
	e.Node = node
	e.EnabledColor = colored
}

func (e *PathError) yml() string {
	if e.Node == nil {
		return ""
	}
	s := e.Path
	if !strings.HasPrefix(s, "$") {
		s = fmt.Sprintf("$%s", s)
	}
	path, err := yaml.PathString(s)
	if path == nil || err != nil {
		return ""
	}
	node, err := path.FilterNode(e.Node)
	if node == nil || err != nil {
		return ""
	}
	var p printer.Printer
	return p.PrintErrorToken(node.GetToken(), e.EnabledColor)
}

func (e *PathError) Error() string {
	yml := e.yml()
	if yml != "" {
		var b strings.Builder
		for _, l := range strings.Split(yml, "\n") {
			if len(l) > 0 {
				b.WriteString("    ")
				b.WriteString(strings.TrimRight(l, " "))
				b.WriteString("\n")
			}
		}
		yml = b.String()
		if !strings.HasSuffix(yml, "\n") {
			yml += "\n"
		}
		return fmt.Sprintf("%s\n%s", e.Err.Error(), yml)
	}
	if e.Path != "" {
		return fmt.Sprintf("%s: %s", e.Path, e.Err.Error())
	}
	return e.Err.Error()
}

// MultiPathError represents multiple error with path.
type MultiPathError struct {
	Node ast.Node
	Errs []error
}

func (e *MultiPathError) Error() string {
	var mulerr error
	mulerr = &multierror.Error{
		Errors: nil,
		ErrorFormat: func(es []error) string {
			if len(es) == 1 {
				return fmt.Sprintf("1 error occurred: %s", strings.TrimLeft(es[0].Error(), "\t"))
			}

			points := make([]string, len(es))
			for i, err := range es {
				points[i] = strings.TrimLeft(err.Error(), "\t")
			}

			return fmt.Sprintf(
				"%d errors occurred: %s",
				len(es), strings.Join(points, "\n"))
		},
	}
	mulerr = multierror.Append(mulerr, e.Errs...)
	return mulerr.Error()
}

func (e *MultiPathError) prependPath(path string) {
	for _, err := range e.Errs {
		var e Error
		if !errors.As(err, &e) {
			continue
		}
		e.prependPath(path)
	}
}

func (e *MultiPathError) replacePath(old, newPath string) {
	for _, err := range e.Errs {
		var e Error
		if !errors.As(err, &e) {
			continue
		}
		e.replacePath(old, newPath)
	}
}

func (e *MultiPathError) wrapf(message string, args ...any) {
	for idx, err := range e.Errs {
		var pathErr Error
		if errors.As(err, &e) {
			pathErr.wrapf(message, args...)
		} else {
			e.Errs[idx] = Wrapf(err, message, args...)
		}
	}
}

func (e *MultiPathError) setNodeAndColored(node ast.Node, colored bool) {
	for _, err := range e.Errs {
		var e Error
		if !errors.As(err, &e) {
			continue
		}
		e.setNodeAndColored(node, colored)
	}
}
