package hegel

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

	schema := map[string]any{
		"type":     "array",
		"items":    elemSchema,
		"minItems": g.minSize,
	}

	if g.maxSize != nil {
		schema["maxItems"] = *g.maxSize
	}

	if g.unique {
		schema["uniqueItems"] = true
	}

	return schema
}

// MapGenerator generates maps with string keys.
type MapGenerator[V any] struct {
	values  Generator[V]
	minSize int
	maxSize *int
}

// Maps returns a generator for maps with string keys.
// Keys are always strings due to JSON limitations.
func Maps[V any](values Generator[V]) *MapGenerator[V] {
	return &MapGenerator[V]{
		values:  values,
		minSize: 0,
	}
}

// MinSize sets the minimum number of entries.
func (g *MapGenerator[V]) MinSize(n int) *MapGenerator[V] {
	g.minSize = n
	return g
}

// MaxSize sets the maximum number of entries.
func (g *MapGenerator[V]) MaxSize(n int) *MapGenerator[V] {
	g.maxSize = &n
	return g
}

// Generate produces a map with string keys.
func (g *MapGenerator[V]) Generate() map[string]V {
	if schema := g.Schema(); schema != nil {
		return generateFromSchema[map[string]V](schema)
	}

	// Compositional fallback when value schema is unavailable
	return Group(LabelMap, func() map[string]V {
		maxSize := 100
		if g.maxSize != nil {
			maxSize = *g.maxSize
		}

		length := Integers[int]().Min(g.minSize).Max(maxSize).Generate()
		result := make(map[string]V)

		keyGen := Text().MinSize(1).MaxSize(20)
		maxAttempts := length * 10
		attempts := 0

		for len(result) < length && attempts < maxAttempts {
			Group(LabelMapEntry, func() struct{} {
				key := keyGen.Generate()
				if _, exists := result[key]; !exists {
					result[key] = g.values.Generate()
				}
				return struct{}{}
			})
			attempts++
		}

		if len(result) < g.minSize {
			Reject("Maps: failed to generate enough unique keys")
		}

		return result
	})
}

// Schema returns the JSON schema for this generator, or nil if unavailable.
func (g *MapGenerator[V]) Schema() map[string]any {
	valueSchema := g.values.Schema()
	if valueSchema == nil {
		return nil // Fall back to compositional generation
	}

	schema := map[string]any{
		"type":                 "object",
		"additionalProperties": valueSchema,
		"minProperties":        g.minSize,
	}

	if g.maxSize != nil {
		schema["maxProperties"] = *g.maxSize
	}

	return schema
}
