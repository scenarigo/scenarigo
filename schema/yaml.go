package schema

import (
	"slices"

	"github.com/goccy/go-yaml"
)

// NewOrderedMap creates a new order-preserving map.
func NewOrderedMap[K comparable, V any]() OrderedMap[K, V] {
	return OrderedMap[K, V]{
		idx:   map[K]int{},
		items: []OrderedMapItem[K, V]{},
	}
}

// OrderedMap represents an order-preserving map.
type OrderedMap[K comparable, V any] struct {
	idx   map[K]int
	items []OrderedMapItem[K, V]
}

// OrderedMapItem represents an item in order-preserving map.
type OrderedMapItem[K, V any] struct {
	Key   K
	Value V
}

// Set sets a key-value pair.
func (m *OrderedMap[K, V]) Set(key K, value V) {
	i, ok := m.idx[key]
	if ok {
		m.items[i].Value = value
		return
	}
	m.items = append(m.items, OrderedMapItem[K, V]{
		Key:   key,
		Value: value,
	})
	m.idx[key] = len(m.items) - 1
}

// Get gets a value by key.
func (m OrderedMap[K, V]) Get(key K) (V, bool) {
	var zero V
	i, ok := m.idx[key]
	if !ok {
		return zero, false
	}
	if i >= len(m.items) {
		return zero, false
	}
	v := m.items[i]
	if v.Key != key {
		return zero, false
	}
	return v.Value, true
}

// Delete deletes a value by key.
func (m *OrderedMap[K, V]) Delete(key K) bool {
	i, ok := m.idx[key]
	if !ok {
		return false
	}
	delete(m.idx, key)
	m.items = slices.Delete(m.items, i, i+1)
	for j, item := range m.items[i:] {
		m.idx[item.Key] = i + j
	}
	return true
}

// Len returns the length of m.
func (m OrderedMap[K, V]) Len() int {
	return len(m.items)
}

// ToMap returns m as a map.
func (m OrderedMap[K, V]) ToMap() map[K]V {
	result := map[K]V{}
	for _, item := range m.ToSlice() {
		result[item.Key] = item.Value
	}
	return result
}

// ToSlice returns m as a slice.
func (m OrderedMap[K, V]) ToSlice() []OrderedMapItem[K, V] {
	return m.items
}

// UnmarshalYAML implements yaml.BytesUnmarshaler interface.
func (m *OrderedMap[K, V]) UnmarshalYAML(b []byte) error {
	var s yaml.MapSlice
	if err := yaml.UnmarshalWithOptions(b, &s, yaml.UseOrderedMap()); err != nil {
		return err
	}
	if len(s) == 0 {
		return nil
	}
	result := OrderedMap[K, V]{
		idx:   map[K]int{},
		items: make([]OrderedMapItem[K, V], len(s)),
	}
	for i, item := range s {
		kb, err := yaml.Marshal(item.Key)
		if err != nil {
			return err
		}
		var k K
		if err := yaml.Unmarshal(kb, &k); err != nil {
			return err
		}
		vb, err := yaml.Marshal(item.Value)
		if err != nil {
			return err
		}
		var v V
		if err := yaml.UnmarshalWithOptions(vb, &v, yaml.UseOrderedMap(), yaml.Strict()); err != nil {
			return err
		}
		result.idx[k] = i
		result.items[i].Key = k
		result.items[i].Value = v
	}
	*m = result
	return nil
}

// MarshalYAML implements yaml.BytesMarshaler interface.
func (m OrderedMap[K, V]) MarshalYAML() ([]byte, error) {
	var s yaml.MapSlice
	for _, item := range m.ToSlice() {
		s = append(s, yaml.MapItem{
			Key:   item.Key,
			Value: item.Value,
		})
	}
	return yaml.Marshal(s)
}

// IsZero implements yaml.IsZeroer interface.
func (m OrderedMap[K, V]) IsZero() bool {
	return m.Len() == 0
}
