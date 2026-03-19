package deepcopy

import (
	"fmt"

	"github.com/mitchellh/copystructure"
)

// Copy returns a deep copy of v.
func Copy(v any) (any, error) {
	if v == nil {
		return nil, nil //nolint:nilnil
	}
	x, err := copystructure.Copy(v)
	if err != nil {
		return nil, fmt.Errorf("deep copy failed: %w", err)
	}
	return x, nil
}
