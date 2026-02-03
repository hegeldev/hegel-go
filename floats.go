package hegel

import "unsafe"

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
// By default, allows NaN and infinity values.
func Floats[T Float]() *FloatGenerator[T] {
	return &FloatGenerator[T]{
		allowNan:      true,
		allowInfinity: true,
	}
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

// AllowNan sets whether NaN values can be generated.
func (g *FloatGenerator[T]) AllowNan(allow bool) *FloatGenerator[T] {
	g.allowNan = allow
	return g
}

// AllowInfinity sets whether infinity values can be generated.
func (g *FloatGenerator[T]) AllowInfinity(allow bool) *FloatGenerator[T] {
	g.allowInfinity = allow
	return g
}

// Generate produces a floating-point value within the configured bounds.
func (g *FloatGenerator[T]) Generate() T {
	return generateFromSchema[T](g.Schema())
}

// Schema returns the JSON schema for this generator.
func (g *FloatGenerator[T]) Schema() map[string]any {
	// Determine width from type size (float32 = 32 bits, float64 = 64 bits)
	var zero T
	width := int(unsafe.Sizeof(zero)) * 8

	schema := map[string]any{
		"type":             "number",
		"exclude_minimum":  g.excludeMin,
		"exclude_maximum":  g.excludeMax,
		"allow_nan":        g.allowNan,
		"allow_infinity":   g.allowInfinity,
		"width":            width,
	}

	if g.minValue != nil {
		schema["minimum"] = *g.minValue
	}

	if g.maxValue != nil {
		schema["maximum"] = *g.maxValue
	}

	return schema
}
