package val

// Value represents an abstract operand value.
type Value interface {
	Type() Type
	GoValue() any
}

// NewValue returns v as a an abstract value.
func NewValue(v any) Value {
	if vv, ok := v.(Value); ok {
		return vv
	}
	tm.RLock()
	defer tm.RUnlock()
	for i := len(types) - 1; i >= 0; i-- {
		val, err := types[i].NewValue(v)
		if err == nil {
			return val
		}
	}
	return Any{v}
}

// LogicalValue is an interface that can be used in a boolean context.
type LogicalValue interface {
	Value
	IsTruthy() bool
}

// Negator is an interface that supports unary '-' operator.
type Negator interface {
	Neg() (Value, error)
}

// Equaler is an interface that supports '==' operator.
type Equaler interface {
	Equal(Value) (LogicalValue, error)
}

// Comparer is an interface that supports '<', '<=', '>', '>=' operators.
type Comparer interface {
	Compare(Value) (Value, error)
}

// Adder is an interface that supports '+' operator.
type Adder interface {
	Add(Value) (Value, error)
}

// Subtractor is an interface that supports '-' operator.
type Subtractor interface {
	Sub(Value) (Value, error)
}

// Multiplier is an interface that supports '*' operator.
type Multiplier interface {
	Mul(Value) (Value, error)
}

// Divider is an interface that supports '/' operator.
type Divider interface {
	Div(Value) (Value, error)
}

// Modder is an interface that supports '%' operator.
type Modder interface {
	Mod(Value) (Value, error)
}

// Sizer is an interface that supports 'size()' overloads.
type Sizer interface {
	Size() (Value, error)
}
