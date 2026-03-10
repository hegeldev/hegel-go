package hegel

import (
	"fmt"
	"math/big"
	"time"

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
	default:
		panic(fmt.Sprintf("hegel: unreachable: expected int, got %T", v))
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
	default:
		panic(fmt.Sprintf("hegel: unreachable: expected float, got %T", v))
	}
}

// extractIntAs extracts an integer from a CBOR-decoded value and converts it to T.
func extractIntAs[T constraints.Integer](v any) T {
	return T(extractInt(v))
}

// Integers returns a Generator that produces integer values in [minVal, maxVal].
// For unbounded generation, use the full range of the type:
//
//	hegel.Integers[int](math.MinInt, math.MaxInt)
func Integers[T constraints.Integer](minVal, maxVal T) Generator[T] {
	return &basicGenerator[T]{
		schema: map[string]any{
			"type":      "integer",
			"min_value": int64(minVal),
			"max_value": int64(maxVal),
		},
		transform: extractIntAs[T],
	}
}

// Floats returns a Generator that produces float64 values.
func Floats(minVal, maxVal *float64, allowNaN, allowInfinity *bool, excludeMin, excludeMax bool) Generator[float64] {
	hasMin := minVal != nil
	hasMax := maxVal != nil

	nan := !hasMin && !hasMax
	if allowNaN != nil {
		nan = *allowNaN
	}
	inf := !hasMin || !hasMax
	if allowInfinity != nil {
		inf = *allowInfinity
	}

	schema := map[string]any{
		"type":           "float",
		"allow_nan":      nan,
		"allow_infinity": inf,
		"exclude_min":    excludeMin,
		"exclude_max":    excludeMax,
		"width":          int64(64),
	}
	if hasMin {
		schema["min_value"] = *minVal
	}
	if hasMax {
		schema["max_value"] = *maxVal
	}
	return &basicGenerator[float64]{
		schema:    schema,
		transform: func(v any) float64 { return extractFloat(v) },
	}
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

// URLs returns a Generator that produces URL strings.
func URLs() Generator[string] {
	return &basicGenerator[string]{
		schema: map[string]any{"type": "url"},
	}
}

// DomainOptions holds options for the Domains generator.
type DomainOptions struct {
	// MaxLength is the maximum length of the domain name.
	// Zero means use the default maximum length (255, matching RFC 1035).
	MaxLength int
}

const defaultDomainMaxLength = 255

// Domains returns a Generator that produces domain name strings.
func Domains(opts DomainOptions) Generator[string] {
	maxLen := opts.MaxLength
	if maxLen <= 0 {
		maxLen = defaultDomainMaxLength
	}
	return &basicGenerator[string]{
		schema: map[string]any{
			"type":       "domain",
			"max_length": int64(maxLen),
		},
	}
}

// Dates returns a Generator that produces time.Time values from ISO 8601 date strings (YYYY-MM-DD).
func Dates() Generator[time.Time] {
	return &basicGenerator[time.Time]{
		schema: map[string]any{"type": "date"},
		transform: func(a any) time.Time {
			t, err := time.Parse("2006-01-02", a.(string))
			if err != nil {
				panic(fmt.Sprintf("hegel: failed to parse date %q: %v", a, err))
			}
			return t
		},
	}
}

// Times returns a Generator that produces time strings (HH:MM:SS or similar).
func Times() Generator[string] {
	return &basicGenerator[string]{
		schema: map[string]any{"type": "time"},
	}
}

// Datetimes returns a Generator that produces time.Time values from ISO 8601 datetime strings.
func Datetimes() Generator[time.Time] {
	return &basicGenerator[time.Time]{
		schema: map[string]any{"type": "datetime"},
		transform: func(a any) time.Time {
			t, err := time.Parse("2006-01-02T15:04:05", a.(string))
			if err != nil {
				panic(fmt.Sprintf("hegel: failed to parse datetime %q: %v", a, err))
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
