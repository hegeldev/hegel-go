package hegel

// Custom creates a generator from a user-provided function.
// Use this for complex generation logic that can't be expressed with other combinators.
//
// Example:
//
//	// Generate even numbers
//	even := Custom(func() int {
//	    return Integers[int]().Min(0).Max(50).Generate() * 2
//	})
func Custom[T any](fn func() T) Generator[T] {
	return &FuncGenerator[T]{
		genFn:  fn,
		schema: nil, // No schema for custom generators
	}
}

// CustomWithSchema creates a generator with an explicit schema.
// This enables schema composition for efficient single-socket round-trips.
//
// Example:
//
//	// Generate positive integers with a schema for composition
//	positiveInt := CustomWithSchema(
//	    func() int { return Integers[int]().Min(1).Generate() },
//	    map[string]any{"type": "integer", "minimum": 1},
//	)
func CustomWithSchema[T any](fn func() T, schema map[string]any) Generator[T] {
	return &FuncGenerator[T]{
		genFn:  fn,
		schema: schema,
	}
}
