package hegel

import "encoding/json"

// SliceGenerator generates slices with configurable size bounds.
type SliceGenerator[T any] struct {
	elements Generator[T]
	minSize  int
	maxSize  *int
	unique   bool
}

// Slices returns a generator for slices containing elements from the given generator.
func Slices[T any](elements Generator[T]) *SliceGenerator[T] {
	return &SliceGenerator[T]{
		elements: elements,
		minSize:  0,
	}
}

// MinSize sets the minimum slice length.
func (g *SliceGenerator[T]) MinSize(n int) *SliceGenerator[T] {
	g.minSize = n
	return g
}

// MaxSize sets the maximum slice length.
func (g *SliceGenerator[T]) MaxSize(n int) *SliceGenerator[T] {
	g.maxSize = &n
	return g
}

// Unique requires all elements to be unique.
func (g *SliceGenerator[T]) Unique() *SliceGenerator[T] {
	g.unique = true
	return g
}

// Generate produces a slice of values.
func (g *SliceGenerator[T]) Generate() []T {
	if schema := g.Schema(); schema != nil {
		return generateFromSchema[[]T](schema)
	}

	// Compositional fallback when element schema is unavailable
	return Group(LabelList, func() []T {
		maxSize := 100
		if g.maxSize != nil {
			maxSize = *g.maxSize
		}

		length := Integers[int]().Min(g.minSize).Max(maxSize).Generate()
		result := make([]T, length)

		for i := 0; i < length; i++ {
			result[i] = Group(LabelListElement, func() T {
				return g.elements.Generate()
			})
		}

		return result
	})
}

// Schema returns the JSON schema for this generator, or nil if unavailable.
func (g *SliceGenerator[T]) Schema() map[string]any {
	elemSchema := g.elements.Schema()
	if elemSchema == nil {
		return nil // Fall back to compositional generation
	}

	schemaType := "list"
	if g.unique {
		schemaType = "set"
	}

	schema := map[string]any{
		"type":     schemaType,
		"elements": elemSchema,
		"min_size": g.minSize,
	}

	if g.maxSize != nil {
		schema["max_size"] = *g.maxSize
	}

	return schema
}

// MapGenerator generates maps with configurable key and value types.
type MapGenerator[K comparable, V any] struct {
	keys    Generator[K]
	values  Generator[V]
	minSize int
	maxSize *int
}

// Maps returns a generator for maps with configurable key and value types.
func Maps[K comparable, V any](keys Generator[K], values Generator[V]) *MapGenerator[K, V] {
	return &MapGenerator[K, V]{
		keys:    keys,
		values:  values,
		minSize: 0,
	}
}

// MinSize sets the minimum number of entries.
func (g *MapGenerator[K, V]) MinSize(n int) *MapGenerator[K, V] {
	g.minSize = n
	return g
}

// MaxSize sets the maximum number of entries.
func (g *MapGenerator[K, V]) MaxSize(n int) *MapGenerator[K, V] {
	g.maxSize = &n
	return g
}

// Generate produces a map.
func (g *MapGenerator[K, V]) Generate() map[K]V {
	if schema := g.Schema(); schema != nil {
		// Wire format is [[key, value], ...]
		pairs := generateFromSchema[[][2]any](schema)
		result := make(map[K]V, len(pairs))
		for _, pair := range pairs {
			// Both key and value need type conversion from any
			keyJSON, _ := json.Marshal(pair[0])
			var key K
			json.Unmarshal(keyJSON, &key)

			valueJSON, _ := json.Marshal(pair[1])
			var value V
			json.Unmarshal(valueJSON, &value)
			result[key] = value
		}
		return result
	}

	// Compositional fallback when schema is unavailable
	return Group(LabelMap, func() map[K]V {
		maxSize := 100
		if g.maxSize != nil {
			maxSize = *g.maxSize
		}

		length := Integers[int]().Min(g.minSize).Max(maxSize).Generate()
		result := make(map[K]V)

		maxAttempts := length * 10
		attempts := 0

		for len(result) < length && attempts < maxAttempts {
			Group(LabelMapEntry, func() struct{} {
				key := g.keys.Generate()
				if _, exists := result[key]; !exists {
					result[key] = g.values.Generate()
				}
				return struct{}{}
			})
			attempts++
		}

		Assume(len(result) >= g.minSize)

		return result
	})
}

// Schema returns the JSON schema for this generator, or nil if unavailable.
func (g *MapGenerator[K, V]) Schema() map[string]any {
	keySchema := g.keys.Schema()
	valueSchema := g.values.Schema()
	if keySchema == nil || valueSchema == nil {
		return nil // Fall back to compositional generation
	}

	schema := map[string]any{
		"type":     "dict",
		"keys":     keySchema,
		"values":   valueSchema,
		"min_size": g.minSize,
	}

	if g.maxSize != nil {
		schema["max_size"] = *g.maxSize
	}

	return schema
}
