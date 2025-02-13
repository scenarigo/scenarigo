package protocol

import (
	"errors"
	"fmt"
	"sync"

	"github.com/scenarigo/scenarigo/internal/yamlutil"
)

// Mock represents a mock.
type Mock struct {
	Protocol string              `yaml:"protocol"`
	Expect   yamlutil.RawMessage `yaml:"expect"`
	Response yamlutil.RawMessage `yaml:"response"`
}

// MockIterator is an iterator over Mocks.
type MockIterator struct {
	m     sync.Mutex
	mocks []Mock
}

// New returns a new MockIterator.
func NewMockIterator(mocks []Mock) *MockIterator {
	//nolint:exhaustruct
	return &MockIterator{
		mocks: mocks,
	}
}

// Next returns the next mock.
func (i *MockIterator) Next() (*Mock, error) {
	i.m.Lock()
	defer i.m.Unlock()
	return i.next()
}

func (i *MockIterator) next() (*Mock, error) {
	if len(i.mocks) == 0 {
		return nil, errors.New("no mocks remain")
	}
	var mock Mock
	mock, i.mocks = i.mocks[0], i.mocks[1:]
	return &mock, nil
}

// Stop terminates the iteration.
// It should be called after you finish using the iterator.
// If mocks not consumed remain returns a MocksRemainError.
func (i *MockIterator) Stop() error {
	i.m.Lock()
	defer i.m.Unlock()

	var count int
	for {
		if _, err := i.next(); err != nil {
			break
		}
		count++
	}

	if count > 0 {
		return &MocksRemainError{count: count}
	}
	return nil
}

// MocksRemainError is the error returned by Stop when mocks not consumed remain.
type MocksRemainError struct {
	count int
}

// Error implements error interface.
func (e *MocksRemainError) Error() string {
	return fmt.Sprintf("last %d mocks remain", e.count)
}
