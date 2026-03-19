package val

import "errors"

var (
	// ErrUnsupportedType is the error that the value type is not supported.
	ErrUnsupportedType = errors.New("unsupported type")
	// ErrOperationNotDefined is the error that the operation is not defined.
	ErrOperationNotDefined = errors.New("operation not defined")
)
