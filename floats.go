package hegel

// FloatGenerator generates floating-point values with configurable bounds.
type FloatGenerator[T Float] struct {
	minValue      *T
	maxValue      *T
	excludeMin    bool
	excludeMax    bool
	allowNan      bool
	allowInfinity bool
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

// AllowNan allows NaN values to be generated.
func (g *FloatGenerator[T]) AllowNan() *FloatGenerator[T] {
	g.allowNan = true
	return g
}

// AllowInfinity allows infinity values to be generated.
func (g *FloatGenerator[T]) AllowInfinity() *FloatGenerator[T] {
	g.allowInfinity = true
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
		schema["minimum"] = *g.minValue
		if g.excludeMin {
			schema["exclude_minimum"] = true
		}
	}

	if g.maxValue != nil {
		schema["maximum"] = *g.maxValue
		if g.excludeMax {
			schema["exclude_maximum"] = true
		}
	}

	if g.allowNan {
		schema["allow_nan"] = true
	}

	if g.allowInfinity {
		schema["allow_infinity"] = true
	}

	return schema
}
