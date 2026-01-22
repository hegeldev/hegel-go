package hegel

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// SampledFromGenerator picks uniformly from a fixed collection.
type SampledFromGenerator[T any] struct {
	elements []T
}

// SampledFrom returns a generator that picks uniformly from the given elements.
func SampledFrom[T any](elements []T) *SampledFromGenerator[T] {
	if len(elements) == 0 {
		panic("SampledFrom: cannot sample from empty slice")
	}
	return &SampledFromGenerator[T]{elements: elements}
}

// Generate picks a random element from the collection.
func (g *SampledFromGenerator[T]) Generate() T {
	if schema := g.Schema(); schema != nil {
		return generateFromSchema[T](schema)
	}

	// Compositional fallback for non-primitive types
	return Group(LabelSampledFrom, func() T {
		idx := Integers[int]().Min(0).Max(len(g.elements) - 1).Generate()
		return g.elements[idx]
	})
}

// Schema returns the JSON schema for this generator.
// Returns a sampled_from schema for primitive types, nil otherwise.
func (g *SampledFromGenerator[T]) Schema() map[string]any {
	// Check if all elements are JSON-primitive (can be represented in sampled_from)
	if !isPrimitiveType[T]() {
		return nil
	}

	// Convert elements to JSON values for sampled_from schema
	jsonElements := make([]any, len(g.elements))
	for i, elem := range g.elements {
		jsonElements[i] = elem
	}

	return map[string]any{"sampled_from": jsonElements}
}

// isPrimitiveType checks if T is a JSON-primitive type.
func isPrimitiveType[T any]() bool {
	var zero T
	switch any(zero).(type) {
	case bool, int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64,
		float32, float64, string:
		return true
	default:
		return false
	}
}

// OneOfGenerator chooses from multiple generators of the same type.
type OneOfGenerator[T any] struct {
	generators []Generator[T]
}

// OneOf returns a generator that chooses from the given generators.
func OneOf[T any](generators ...Generator[T]) *OneOfGenerator[T] {
	if len(generators) == 0 {
		panic("OneOf: no generators provided")
	}
	return &OneOfGenerator[T]{generators: generators}
}

// Generate produces a value from one of the generators.
func (g *OneOfGenerator[T]) Generate() T {
	if schema := g.Schema(); schema != nil {
		return generateFromSchema[T](schema)
	}

	// Compositional fallback
	return Group(LabelOneOf, func() T {
		idx := Integers[int]().Min(0).Max(len(g.generators) - 1).Generate()
		return g.generators[idx].Generate()
	})
}

// Schema returns the JSON schema for this generator.
// Returns one_of schema if all generators have schemas, nil otherwise.
func (g *OneOfGenerator[T]) Schema() map[string]any {
	schemas := make([]map[string]any, 0, len(g.generators))
	for _, gen := range g.generators {
		s := gen.Schema()
		if s == nil {
			return nil // Fall back to compositional generation
		}
		schemas = append(schemas, s)
	}

	return map[string]any{"one_of": schemas}
}

// OneOfAnyGenerator chooses from generators returning different types.
type OneOfAnyGenerator struct {
	generators []Generator[any]
}

// OneOfAny returns a generator that chooses from generators of different types.
// This is useful when you need to generate values of varying types.
func OneOfAny(generators ...Generator[any]) *OneOfAnyGenerator {
	if len(generators) == 0 {
		panic("OneOfAny: no generators provided")
	}
	return &OneOfAnyGenerator{generators: generators}
}

// Generate produces a value from one of the generators.
func (g *OneOfAnyGenerator) Generate() any {
	if schema := g.Schema(); schema != nil {
		return generateFromSchema[any](schema)
	}

	// Compositional fallback
	return Group(LabelOneOf, func() any {
		idx := Integers[int]().Min(0).Max(len(g.generators) - 1).Generate()
		return g.generators[idx].Generate()
	})
}

// Schema returns the JSON schema for this generator.
func (g *OneOfAnyGenerator) Schema() map[string]any {
	schemas := make([]map[string]any, 0, len(g.generators))
	for _, gen := range g.generators {
		s := gen.Schema()
		if s == nil {
			return nil
		}
		schemas = append(schemas, s)
	}

	return map[string]any{"one_of": schemas}
}

// AsAny converts a Generator[T] to Generator[any].
// This is useful for passing generators to OneOfAny.
func AsAny[T any](gen Generator[T]) Generator[any] {
	return &FuncGenerator[any]{
		genFn: func() any {
			return gen.Generate()
		},
		schema: gen.Schema(),
	}
}

// OptionalGenerator generates either nil or a value.
type OptionalGenerator[T any] struct {
	inner Generator[T]
}

// Optional returns a generator that produces either nil or a value.
func Optional[T any](gen Generator[T]) *OptionalGenerator[T] {
	return &OptionalGenerator[T]{inner: gen}
}

// Generate produces either nil or a value.
func (g *OptionalGenerator[T]) Generate() *T {
	if schema := g.Schema(); schema != nil {
		// The schema uses one_of with null, so result could be null
		result := generateFromSchema[json.RawMessage](schema)

		// Check if the result is null
		if string(result) == "null" {
			return nil
		}

		var value T
		err := json.Unmarshal(result, &value)
		if err != nil {
			panic(fmt.Sprintf("hegel: failed to deserialize optional value: %v\nValue: %s", err, result))
		}
		return &value
	}

	// Compositional fallback
	return Group(LabelOptional, func() *T {
		isNil := Booleans().Generate()
		if isNil {
			return nil
		}
		value := g.inner.Generate()
		return &value
	})
}

// Schema returns the JSON schema for this generator.
func (g *OptionalGenerator[T]) Schema() map[string]any {
	innerSchema := g.inner.Schema()
	if innerSchema == nil {
		return nil
	}

	return map[string]any{
		"one_of": []map[string]any{
			{"type": "null"},
			innerSchema,
		},
	}
}

// reflectSchema attempts to build a schema from a reflect.Type.
// Used by Make[T]() for automatic schema generation.
func reflectSchema(t reflect.Type) map[string]any {
	switch t.Kind() {
	case reflect.Bool:
		return map[string]any{"type": "boolean"}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return map[string]any{"type": "integer"}
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return map[string]any{"type": "integer", "minimum": 0}
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}
	case reflect.String:
		return map[string]any{"type": "string"}
	case reflect.Slice:
		elemSchema := reflectSchema(t.Elem())
		if elemSchema == nil {
			return nil
		}
		return map[string]any{"type": "list", "elements": elemSchema}
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil // Only string keys supported
		}
		valueSchema := reflectSchema(t.Elem())
		if valueSchema == nil {
			return nil
		}
		return map[string]any{"type": "dict", "values": valueSchema}
	case reflect.Ptr:
		innerSchema := reflectSchema(t.Elem())
		if innerSchema == nil {
			return nil
		}
		return map[string]any{
			"one_of": []map[string]any{
				{"type": "null"},
				innerSchema,
			},
		}
	default:
		return nil
	}
}
