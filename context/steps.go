package context

import (
	"context"
	"sync"

	query "github.com/zoncoen/query-go/v2"
)

// Steps represents results of steps.
type Steps struct {
	mu      sync.Mutex
	results map[string]*Step
}

// Step represents a result of step.
type Step struct {
	Result string `yaml:"result,omitempty"`
	Steps  *Steps `yaml:"steps,omitempty"` // child steps
}

// NewSteps returns a *Steps.
func NewSteps() *Steps {
	return &Steps{
		mu:      sync.Mutex{},
		results: map[string]*Step{},
	}
}

// Add adds a result of step.
func (s *Steps) Add(id string, step *Step) {
	if id == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.results[id] = step
}

// Get gets a result of step by id.
func (s *Steps) Get(id string) *Step {
	if id == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.results[id]
}

var _ query.KeyExtractor = (*Steps)(nil)

// ExtractByKey implements query.KeyExtractor interface.
func (s *Steps) ExtractByKey(_ context.Context, key string) (any, error) {
	step := s.Get(key)
	if step != nil {
		return step, nil
	}
	return nil, query.ErrNotFound
}
