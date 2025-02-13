package template

import (
	"fmt"

	"github.com/scenarigo/scenarigo/template/val"
)

var functions = map[string]any{
	"size": size,
}

func size(in any) (any, error) {
	v := val.NewValue(in)
	if s, ok := v.(val.Sizer); ok {
		vv, err := s.Size()
		if err != nil {
			return nil, err
		}
		if vv != nil {
			return vv.GoValue(), nil
		}
	}
	return nil, fmt.Errorf("size(%s) is not defined", v.Type().Name())
}
