package hegel

// NullGenerator generates null values (represented as struct{} in Go).
type NullGenerator struct{}

// Nulls returns a generator that produces null values.
func Nulls() *NullGenerator {
	return &NullGenerator{}
}

// Generate returns an empty struct (representing null).
func (g *NullGenerator) Generate() struct{} {
	generateFromSchema[any](g.Schema())
	return struct{}{}
}

// Schema returns the JSON schema for null values.
func (g *NullGenerator) Schema() map[string]any {
	return map[string]any{"type": "null"}
}

// BoolGenerator generates boolean values.
type BoolGenerator struct{}

// Booleans returns a generator that produces boolean values.
func Booleans() *BoolGenerator {
	return &BoolGenerator{}
}

// Generate produces a boolean value.
func (g *BoolGenerator) Generate() bool {
	return generateFromSchema[bool](g.Schema())
}

// Schema returns the JSON schema for boolean values.
func (g *BoolGenerator) Schema() map[string]any {
	return map[string]any{"type": "boolean"}
}

// JustGenerator always produces the same value.
type JustGenerator[T any] struct {
	value T
}

// Just returns a generator that always produces the given value.
// The value is included in the schema as a const, enabling schema composition.
func Just[T any](value T) *JustGenerator[T] {
	return &JustGenerator[T]{value: value}
}

// Generate returns the constant value.
func (g *JustGenerator[T]) Generate() T {
	return g.value
}

// Schema returns a const schema for this value.
func (g *JustGenerator[T]) Schema() map[string]any {
	return map[string]any{"const": g.value}
}
