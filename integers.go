package hegel

import "math"

// IntegerGenerator generates integer values with configurable bounds.
type IntegerGenerator[T Integer] struct {
	minValue *T
	maxValue *T
}

// Integers returns a new integer generator.
// The bounds default to the type's min/max values (e.g., int8: -128 to 127).
func Integers[T Integer]() *IntegerGenerator[T] {
	return &IntegerGenerator[T]{}
}

// Min sets the minimum value (inclusive).
func (g *IntegerGenerator[T]) Min(v T) *IntegerGenerator[T] {
	g.minValue = &v
	return g
}

// Max sets the maximum value (inclusive).
func (g *IntegerGenerator[T]) Max(v T) *IntegerGenerator[T] {
	g.maxValue = &v
	return g
}

// Generate produces an integer value within the configured bounds.
func (g *IntegerGenerator[T]) Generate() T {
	return generateFromSchema[T](g.Schema())
}

// Schema returns the JSON schema for this generator.
func (g *IntegerGenerator[T]) Schema() map[string]any {
	schema := map[string]any{"type": "integer"}

	if g.minValue != nil {
		schema["minimum"] = *g.minValue
	} else {
		schema["minimum"] = minValueFor[T]()
	}

	if g.maxValue != nil {
		schema["maximum"] = *g.maxValue
	} else {
		schema["maximum"] = maxValueFor[T]()
	}

	return schema
}

// minValueFor returns the minimum value for an integer type.
func minValueFor[T Integer]() int64 {
	var zero T
	switch any(zero).(type) {
	case int8:
		return math.MinInt8
	case int16:
		return math.MinInt16
	case int32:
		return math.MinInt32
	case int64:
		return math.MinInt64
	case int:
		return math.MinInt
	case uint8, uint16, uint32, uint64, uint:
		return 0
	default:
		return math.MinInt64
	}
}

// maxValueFor returns the maximum value for an integer type.
func maxValueFor[T Integer]() int64 {
	var zero T
	switch any(zero).(type) {
	case int8:
		return math.MaxInt8
	case int16:
		return math.MaxInt16
	case int32:
		return math.MaxInt32
	case int64:
		return math.MaxInt64
	case int:
		return math.MaxInt
	case uint8:
		return math.MaxUint8
	case uint16:
		return math.MaxUint16
	case uint32:
		return math.MaxUint32
	// uint64 max is too large for JSON number, so we use MaxInt64
	case uint64:
		return math.MaxInt64
	case uint:
		return math.MaxInt64
	default:
		return math.MaxInt64
	}
}
