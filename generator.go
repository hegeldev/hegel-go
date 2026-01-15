// Package hegel provides property-based testing with JSON Schema generation.
//
// This package allows test binaries to communicate with the Hegel server
// via Unix sockets to generate random test data according to JSON schemas.
package hegel

// Generator is the core interface for all generators.
// It produces values of type T and optionally carries a JSON schema.
type Generator[T any] interface {
	// Generate produces a value of type T.
	Generate() T

	// Schema returns the JSON schema for this generator, or nil if unavailable.
	// When a schema is available, generation is more efficient (single socket round-trip).
	Schema() map[string]any
}

// SchemaGenerator is a generator backed by a JSON schema.
// This is the primary generator type used by most strategies.
type SchemaGenerator[T any] struct {
	schema map[string]any
}

// Generate produces a value by sending the schema to the Hegel server.
func (g *SchemaGenerator[T]) Generate() T {
	return generateFromSchema[T](g.schema)
}

// Schema returns the JSON schema for this generator.
func (g *SchemaGenerator[T]) Schema() map[string]any {
	return g.schema
}

// FuncGenerator wraps a generation function.
// Used by Custom() and compositional fallback when schemas are unavailable.
type FuncGenerator[T any] struct {
	genFn  func() T
	schema map[string]any // May be nil
}

// Generate produces a value by calling the wrapped function.
func (g *FuncGenerator[T]) Generate() T {
	return g.genFn()
}

// Schema returns the JSON schema for this generator, or nil if unavailable.
func (g *FuncGenerator[T]) Schema() map[string]any {
	return g.schema
}

// Filter returns a generator that only produces values satisfying the predicate.
// If maxAttempts consecutive values fail the predicate, Assume(false) is called.
func Filter[T any](gen Generator[T], predicate func(T) bool, maxAttempts int) Generator[T] {
	return &FuncGenerator[T]{
		genFn: func() T {
			for i := 0; i < maxAttempts; i++ {
				value := gen.Generate()
				if predicate(value) {
					return value
				}
			}
			Assume(false)
			panic("unreachable") // Assume(false) exits
		},
		schema: nil, // Filter invalidates schema
	}
}

// Integer is a constraint for all integer types.
type Integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64
}

// Float is a constraint for all floating-point types.
type Float interface {
	~float32 | ~float64
}
