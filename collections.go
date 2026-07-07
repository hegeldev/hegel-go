package hegel

import (
	"fmt"

	"hegel.dev/go/hegel/internal/libhegel"
)

// --- Lists generator ---

// ListGenerator configures and generates slices of values from an element generator.
// Use [Lists] to create one, then chain builder methods to configure bounds.
// Invalid configurations panic on the first [Draw] call.
type ListGenerator[T any] struct {
	elements Generator[T]
	minSize  int
	maxSize  int
	hasMax   bool
}

// Lists returns a Generator that produces slices of values from the elements generator.
func Lists[T any](elements Generator[T]) ListGenerator[T] {
	return ListGenerator[T]{elements: elements}
}

// MinSize sets the minimum number of elements (inclusive). Default: 0.
func (g ListGenerator[T]) MinSize(n int) ListGenerator[T] {
	g.minSize = n
	return g
}

// MaxSize sets the maximum number of elements (inclusive).
func (g ListGenerator[T]) MaxSize(n int) ListGenerator[T] {
	g.maxSize = n
	g.hasMax = true
	return g
}

// draw produces a list using the engine's collection protocol.
func (g ListGenerator[T]) draw(tc TestCase) ([]T, error) {
	if g.minSize < 0 {
		return nil, fmt.Errorf("min_size=%d must be non-negative", g.minSize)
	}
	if g.hasMax && g.maxSize < 0 {
		return nil, fmt.Errorf("max_size=%d must be non-negative", g.maxSize)
	}
	if g.hasMax && g.minSize > g.maxSize {
		return nil, fmt.Errorf("cannot have max_size=%d < min_size=%d", g.maxSize, g.minSize)
	}
	var maxSize *int
	if g.hasMax {
		maxSize = &g.maxSize
	}
	return withSpan(tc, libhegel.LABEL_LIST, func() ([]T, error) {
		var result []T
		coll, err := tc.newCollection(g.minSize, maxSize)
		if err != nil {
			return nil, err
		}
		for coll.More() {
			v, err := g.elements.draw(tc)
			if err != nil {
				return nil, err
			}
			result = append(result, v)
		}
		if err := coll.Err(); err != nil {
			return nil, err
		}
		return result, nil
	})
}

// --- Maps generator ---

// MapGenerator configures and generates map[K]V values.
// Use [Maps] to create one, then chain builder methods to configure bounds.
// Invalid configurations panic on the first [Draw] call.
type MapGenerator[K comparable, V any] struct {
	keys    Generator[K]
	values  Generator[V]
	minSize int
	maxSize int
	hasMax  bool
}

// Maps returns a Generator that produces map[K]V values.
func Maps[K comparable, V any](keys Generator[K], values Generator[V]) MapGenerator[K, V] {
	return MapGenerator[K, V]{keys: keys, values: values}
}

// MinSize sets the minimum number of key-value pairs. Default: 0.
func (g MapGenerator[K, V]) MinSize(n int) MapGenerator[K, V] {
	g.minSize = n
	return g
}

// MaxSize sets the maximum number of key-value pairs.
func (g MapGenerator[K, V]) MaxSize(n int) MapGenerator[K, V] {
	g.maxSize = n
	g.hasMax = true
	return g
}

// draw produces a map using the engine's collection protocol, rejecting
// duplicate keys so the engine retries.
func (g MapGenerator[K, V]) draw(tc TestCase) (map[K]V, error) {
	if g.minSize < 0 {
		return nil, fmt.Errorf("min_size=%d must be non-negative", g.minSize)
	}
	if g.hasMax && g.maxSize < 0 {
		return nil, fmt.Errorf("max_size=%d must be non-negative", g.maxSize)
	}
	if g.hasMax && g.minSize > g.maxSize {
		return nil, fmt.Errorf("cannot have max_size=%d < min_size=%d", g.maxSize, g.minSize)
	}
	var maxSize *int
	if g.hasMax {
		maxSize = &g.maxSize
	}
	return withSpan(tc, libhegel.LABEL_MAP, func() (map[K]V, error) {
		result := map[K]V{}
		coll, err := tc.newCollection(g.minSize, maxSize)
		if err != nil {
			return nil, err
		}
		for coll.More() {
			k, err := g.keys.draw(tc)
			if err != nil {
				return nil, err
			}
			if _, exists := result[k]; exists {
				coll.Reject("duplicate key")
				continue
			}
			v, err := g.values.draw(tc)
			if err != nil {
				return nil, err
			}
			result[k] = v
		}
		if err := coll.Err(); err != nil {
			return nil, err
		}
		return result, nil
	})
}
