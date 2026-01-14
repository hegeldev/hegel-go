package hegel

// FloatGenerator generates floating-point values with configurable bounds.
type FloatGenerator[T Float] struct {
	minValue   *T
	maxValue   *T
	excludeMin bool
	excludeMax bool
}

// Floats returns a new floating-point generator.
func Floats[T Float]() *FloatGenerator[T] {
	return &FloatGenerator[T]{}
}

// Min sets the minimum value.
func (g *FloatGenerator[T]) Min(v T) *FloatGenerator[T] {
	g.minValue = &v
	return g
}

// Max sets the maximum value.
func (g *FloatGenerator[T]) Max(v T) *FloatGenerator[T] {
	g.maxValue = &v
	return g
}

// ExcludeMin excludes the minimum value from the range.
func (g *FloatGenerator[T]) ExcludeMin() *FloatGenerator[T] {
	g.excludeMin = true
	return g
}

// ExcludeMax excludes the maximum value from the range.
func (g *FloatGenerator[T]) ExcludeMax() *FloatGenerator[T] {
	g.excludeMax = true
	return g
}

// Generate produces a floating-point value within the configured bounds.
func (g *FloatGenerator[T]) Generate() T {
	return generateFromSchema[T](g.Schema())
}

// Schema returns the JSON schema for this generator.
func (g *FloatGenerator[T]) Schema() map[string]any {
	schema := map[string]any{"type": "number"}

	if g.minValue != nil {
		if g.excludeMin {
			schema["exclusiveMinimum"] = *g.minValue
		} else {
			schema["minimum"] = *g.minValue
		}
	}

	if g.maxValue != nil {
		if g.excludeMax {
			schema["exclusiveMaximum"] = *g.maxValue
		} else {
			schema["maximum"] = *g.maxValue
		}
	}

	return schema
}
