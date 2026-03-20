package hegel

import (
	"fmt"
	"math/big"
	"time"
	"unsafe"

	"golang.org/x/exp/constraints"
)

// --- Built-in generators ---

// extractInt extracts an integer value from a CBOR-decoded value.
// Used internally by generators that need to convert CBOR integers.
func extractInt(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case uint64:
		return int64(x)
	case big.Int:
		return x.Int64()
	case *big.Int:
		return x.Int64()
	default: //nocov
		panic(fmt.Sprintf("hegel: expected int, got %T", v)) //nocov
	}
}

// extractFloat extracts a float64 from a CBOR-decoded value.
func extractFloat(v any) float64 {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int64:
		return float64(x)
	case uint64:
		return float64(x)
	default: //nocov
		panic(fmt.Sprintf("hegel: expected float, got %T", v)) //nocov
	}
}

// extractIntAs extracts an integer from a CBOR-decoded value and converts it to T.
func extractIntAs[T constraints.Integer](v any) T {
	return T(extractInt(v))
}

// extractFloatAs extracts a float from a CBOR-decoded value and converts it to T.
func extractFloatAs[T constraints.Float](v any) T {
	return T(extractFloat(v))
}

// Integers returns a Generator that produces integer values in [minVal, maxVal].
// For unbounded generation, use the full range of the type:
//
//	hegel.Integers[int](math.MinInt, math.MaxInt)
func Integers[T constraints.Integer](minVal, maxVal T) Generator[T] {
	if minVal > maxVal {
		panic(fmt.Sprintf("hegel: Cannot have max_value=%d < min_value=%d", maxVal, minVal))
	}
	return &basicGenerator[T]{
		schema: map[string]any{
			"type":      "integer",
			"min_value": int64(minVal),
			"max_value": int64(maxVal),
		},
		transform: extractIntAs[T],
	}
}

// FloatGenerator configures and generates floating-point values of type T.
// Use [Floats] to create one, then chain builder methods to configure bounds
// and behavior. Invalid configurations panic on the first [Draw] call.
type FloatGenerator[T constraints.Float] struct {
	minVal     *float64
	maxVal     *float64
	allowNaN   *bool
	allowInf   *bool
	excludeMin bool
	excludeMax bool
}

// Floats returns a FloatGenerator that produces floating-point values of type T.
// Configure bounds and behavior by chaining builder methods.
//
//	hegel.Floats[float64]()                         // any float64 including NaN and Inf
//	hegel.Floats[float64]().Min(0).Max(1)           // bounded [0, 1]
//	hegel.Floats[float32]().Min(0).ExcludeMin()     // (0, +Inf)
func Floats[T constraints.Float]() FloatGenerator[T] {
	return FloatGenerator[T]{}
}

// Min sets the minimum value for the float generator.
func (g FloatGenerator[T]) Min(v T) FloatGenerator[T] {
	f := float64(v)
	g.minVal = &f
	return g
}

// Max sets the maximum value for the float generator.
func (g FloatGenerator[T]) Max(v T) FloatGenerator[T] {
	f := float64(v)
	g.maxVal = &f
	return g
}

// AllowNaN sets whether the generator may produce NaN values.
// Default: true when no bounds are set, false otherwise.
func (g FloatGenerator[T]) AllowNaN(v bool) FloatGenerator[T] {
	g.allowNaN = &v
	return g
}

// AllowInfinity sets whether the generator may produce infinite values.
// Default: true unless both bounds are set.
func (g FloatGenerator[T]) AllowInfinity(v bool) FloatGenerator[T] {
	g.allowInf = &v
	return g
}

// ExcludeMin excludes the lower bound from the generated range.
func (g FloatGenerator[T]) ExcludeMin() FloatGenerator[T] {
	g.excludeMin = true
	return g
}

// ExcludeMax excludes the upper bound from the generated range.
func (g FloatGenerator[T]) ExcludeMax() FloatGenerator[T] {
	g.excludeMax = true
	return g
}

// buildSchema validates the configuration and returns the wire schema.
// Panics on invalid combinations of settings.
func (g FloatGenerator[T]) buildSchema() map[string]any {
	hasMin := g.minVal != nil
	hasMax := g.maxVal != nil

	nan := !hasMin && !hasMax
	if g.allowNaN != nil {
		nan = *g.allowNaN
	}
	inf := !hasMin || !hasMax
	if g.allowInf != nil {
		inf = *g.allowInf
	}

	if nan && (hasMin || hasMax) {
		panic("hegel: Cannot have allow_nan=true with min_value or max_value")
	}
	if hasMin && hasMax && *g.minVal > *g.maxVal {
		panic(fmt.Sprintf("hegel: Cannot have max_value=%v < min_value=%v", *g.maxVal, *g.minVal))
	}
	if inf && hasMin && hasMax {
		panic("hegel: Cannot have allow_infinity=true with both min_value and max_value")
	}

	width := int64(unsafe.Sizeof(T(1.0)) * 8)
	schema := map[string]any{
		"type":           "float",
		"allow_nan":      nan,
		"allow_infinity": inf,
		"exclude_min":    g.excludeMin,
		"exclude_max":    g.excludeMax,
		"width":          width,
	}
	if hasMin {
		schema["min_value"] = *g.minVal
	}
	if hasMax {
		schema["max_value"] = *g.maxVal
	}
	return schema
}

func (g FloatGenerator[T]) buildGenerator() Generator[T] {
	return &basicGenerator[T]{
		schema:    g.buildSchema(),
		transform: extractFloatAs[T],
	}
}

// draw produces a floating-point value from the Hegel server.
func (g FloatGenerator[T]) draw(s *TestCase) T {
	return g.buildGenerator().draw(s)
}

// Booleans returns a Generator that produces boolean values.
func Booleans() Generator[bool] {
	return &basicGenerator[bool]{
		schema: map[string]any{
			"type": "boolean",
		},
	}
}

// Text returns a Generator that produces string values with codepoint count in [minSize, maxSize].
//
// Pass maxSize < 0 for unbounded.
func Text(minSize int, maxSize int) Generator[string] {
	if minSize < 0 {
		panic(fmt.Sprintf("hegel: min_size=%d must be non-negative", minSize))
	}
	if maxSize >= 0 && minSize > maxSize {
		panic(fmt.Sprintf("hegel: Cannot have max_size=%d < min_size=%d", maxSize, minSize))
	}
	schema := map[string]any{
		"type":     "string",
		"min_size": int64(minSize),
	}
	if maxSize >= 0 {
		schema["max_size"] = int64(maxSize)
	}
	return &basicGenerator[string]{schema: schema}
}

// Binary returns a Generator that produces byte slices with length in [minSize, maxSize].
//
// Pass maxSize < 0 for unbounded.
func Binary(minSize int, maxSize int) Generator[[]byte] {
	if minSize < 0 {
		panic(fmt.Sprintf("hegel: min_size=%d must be non-negative", minSize))
	}
	if maxSize >= 0 && minSize > maxSize {
		panic(fmt.Sprintf("hegel: Cannot have max_size=%d < min_size=%d", maxSize, minSize))
	}
	schema := map[string]any{
		"type":     "binary",
		"min_size": int64(minSize),
	}
	if maxSize >= 0 {
		schema["max_size"] = int64(maxSize)
	}
	return &basicGenerator[[]byte]{schema: schema}
}

// Emails returns a Generator that produces email address strings.
func Emails() Generator[string] {
	return &basicGenerator[string]{
		schema: map[string]any{"type": "email"},
	}
}

// URLs returns a Generator that produces URL strings according to RFC3986.
//
// The schema is either "http" or "https".
func URLs() Generator[string] {
	return &basicGenerator[string]{
		schema: map[string]any{"type": "url"},
	}
}

const defaultDomainMaxLength = 255

// DomainGenerator configures and generates domain name strings.
// Use [Domains] to create one, then chain builder methods to configure it.
// Invalid configurations panic on the first [Draw] call.
type DomainGenerator struct {
	maxLength int
	hasMax    bool
}

// Domains returns a Generator that produces domain name strings.
func Domains() DomainGenerator {
	return DomainGenerator{}
}

// MaxLength sets the maximum domain length.
func (g DomainGenerator) MaxLength(n int) DomainGenerator {
	g.maxLength = n
	g.hasMax = true
	return g
}

func (g DomainGenerator) buildSchema() map[string]any {
	maxLen := defaultDomainMaxLength
	if g.hasMax {
		maxLen = g.maxLength
	}
	if maxLen < 4 || maxLen > 255 {
		panic(fmt.Sprintf("hegel: max_length=%d must be between 4 and 255", maxLen))
	}
	return map[string]any{
		"type":       "domain",
		"max_length": int64(maxLen),
	}
}

func (g DomainGenerator) buildGenerator() Generator[string] {
	return &basicGenerator[string]{
		schema: g.buildSchema(),
	}
}

func (g DomainGenerator) draw(s *TestCase) string {
	return g.buildGenerator().draw(s)
}

// Dates returns a Generator that produces time.Time values from ISO 8601 date strings (YYYY-MM-DD).
func Dates() Generator[time.Time] {
	return &basicGenerator[time.Time]{
		schema: map[string]any{"type": "date"},
		transform: func(a any) time.Time {
			t, err := time.Parse("2006-01-02", a.(string))
			if err != nil { //nocov
				panic(fmt.Sprintf("hegel: failed to parse date %q: %v", a, err)) //nocov
			}
			return t
		},
	}
}

// Datetimes returns a Generator that produces time.Time values from ISO 8601 datetime strings.
func Datetimes() Generator[time.Time] {
	return &basicGenerator[time.Time]{
		schema: map[string]any{"type": "datetime"},
		transform: func(a any) time.Time {
			// TODO: WTF?
			t, err := time.Parse("2006-01-02T15:04:05", a.(string))
			if err != nil { //nocov
				panic(fmt.Sprintf("hegel: failed to parse datetime %q: %v", a, err)) //nocov
			}
			return t
		},
	}
}

// Just returns a Generator that always produces the given constant value.
func Just[T any](value T) Generator[T] {
	return &basicGenerator[T]{
		schema:    map[string]any{"const": nil},
		transform: func(_ any) T { return value },
	}
}

// SampledFrom returns a Generator that picks uniformly at random from values.
//
// Panics if values is empty.
func SampledFrom[T any](values []T) Generator[T] {
	if len(values) == 0 {
		panic("hegel: SampledFrom requires at least one element")
	}
	elements := make([]T, len(values))
	copy(elements, values)
	return &basicGenerator[T]{
		schema: map[string]any{
			"type":      "integer",
			"min_value": int64(0),
			"max_value": int64(len(elements) - 1),
		},
		transform: func(v any) T {
			idx := extractInt(v)
			return elements[idx]
		},
	}
}

// FromRegex returns a Generator that produces strings matching the given regular expression.
func FromRegex(pattern string, fullmatch bool) Generator[string] {
	return &basicGenerator[string]{
		schema: map[string]any{
			"type":      "regex",
			"pattern":   pattern,
			"fullmatch": fullmatch,
		},
	}
}
