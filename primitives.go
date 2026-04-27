package hegel

import (
	"fmt"
	"math/big"
	"time"
	"unsafe"
)

type integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

type float interface {
	~float32 | ~float64
}

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
	default: // coverage-ignore
		panic(fmt.Sprintf("expected int, got %T", v))
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
	default: // coverage-ignore
		panic(fmt.Sprintf("expected float, got %T", v))
	}
}

// extractIntAs extracts an integer from a CBOR-decoded value and converts it to T.
func extractIntAs[T integer](v any) T {
	return T(extractInt(v))
}

// extractFloatAs extracts a float from a CBOR-decoded value and converts it to T.
func extractFloatAs[T float](v any) T {
	return T(extractFloat(v))
}

func extractString(v any) string {
	return v.(string)
}

// Integers returns a Generator that produces integer values in [minVal, maxVal].
// For unbounded generation, use the full range of the type:
//
//	hegel.Integers[int](math.MinInt, math.MaxInt)
//	hegel.Integers[uint](0, math.MaxUint)
func Integers[T integer](minVal, maxVal T) Generator[T] {
	if minVal > maxVal {
		panic(fmt.Sprintf("Cannot have max_value=%d < min_value=%d", maxVal, minVal))
	}
	// Encode bounds in the widest type that preserves T's range.
	var minSchema, maxSchema any
	var zero T
	if ^zero > zero {
		minSchema = uint64(minVal)
		maxSchema = uint64(maxVal)
	} else {
		minSchema = int64(minVal)
		maxSchema = int64(maxVal)
	}
	return &basicGenerator[T]{
		schema: map[string]any{
			"type":      "integer",
			"min_value": minSchema,
			"max_value": maxSchema,
		},
		parse: extractIntAs[T],
	}
}

// FloatGenerator configures and generates floating-point values of type T.
// Use [Floats] to create one, then chain builder methods to configure bounds
// and behavior. Invalid configurations panic on the first [Draw] call.
type FloatGenerator[T float] struct {
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
func Floats[T float]() FloatGenerator[T] {
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

// asBasic validates the configuration and returns the basic generator with
// its wire schema. Returns an error on invalid combinations of settings.
func (g FloatGenerator[T]) asBasic() (*basicGenerator[T], bool, error) {
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
		return nil, false, fmt.Errorf("Cannot have allow_nan=true with min_value or max_value")
	}
	if hasMin && hasMax && *g.minVal > *g.maxVal {
		return nil, false, fmt.Errorf("Cannot have max_value=%v < min_value=%v", *g.maxVal, *g.minVal)
	}
	if inf && hasMin && hasMax {
		return nil, false, fmt.Errorf("Cannot have allow_infinity=true with both min_value and max_value")
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
	return &basicGenerator[T]{schema: schema, parse: extractFloatAs[T]}, true, nil
}

// draw produces a floating-point value from the Hegel server.
func (g FloatGenerator[T]) draw(s *TestCase) T {
	bg, _, err := g.asBasic()
	if err != nil {
		panic(err.Error())
	}
	return bg.draw(s)
}

// Booleans returns a Generator that produces boolean values.
func Booleans() Generator[bool] {
	return &basicGenerator[bool]{
		schema: map[string]any{
			"type": "boolean",
		},
		parse: func(v any) bool { return v.(bool) },
	}
}

// surrogateCategories lists Unicode general categories that include surrogate
// codepoints. Go strings are UTF-8 and cannot represent surrogates, so these
// categories are forbidden in the categories whitelist.
var surrogateCategories = []string{"Cs", "C"}

// characterFields holds the shared character filtering options used by both
// TextGenerator and CharactersGenerator.
type characterFields struct {
	codec             *string
	minCodepoint      *rune
	maxCodepoint      *rune
	categories        []string
	excludeCategories []string
	includeCharacters *string
	excludeCharacters *string
	hasCategoriesSet  bool
}

// toSchema returns schema fields for the character filtering options,
// automatically injecting surrogate exclusion for Go's UTF-8 strings.
func (cf *characterFields) toSchema() (map[string]any, error) {
	schema := map[string]any{}
	if cf.codec != nil {
		schema["codec"] = *cf.codec
	}
	if cf.minCodepoint != nil {
		schema["min_codepoint"] = int64(*cf.minCodepoint)
	}
	if cf.maxCodepoint != nil {
		schema["max_codepoint"] = int64(*cf.maxCodepoint)
	}
	if cf.hasCategoriesSet {
		for _, cat := range cf.categories {
			for _, sc := range surrogateCategories {
				if cat == sc {
					return nil, fmt.Errorf(
						"category %q includes surrogate codepoints (Cs), "+
							"which Go strings cannot represent", cat)
				}
			}
		}
		cats := make([]any, len(cf.categories))
		for i, c := range cf.categories {
			cats[i] = c
		}
		schema["categories"] = cats
	} else {
		excl := make([]string, len(cf.excludeCategories))
		copy(excl, cf.excludeCategories)
		hasCs := false
		for _, c := range excl {
			if c == "Cs" {
				hasCs = true
				break
			}
		}
		if !hasCs {
			excl = append(excl, "Cs")
		}
		cats := make([]any, len(excl))
		for i, c := range excl {
			cats[i] = c
		}
		schema["exclude_categories"] = cats
	}
	if cf.includeCharacters != nil {
		schema["include_characters"] = *cf.includeCharacters
	}
	if cf.excludeCharacters != nil {
		schema["exclude_characters"] = *cf.excludeCharacters
	}
	return schema, nil
}

// TextGenerator configures and generates Unicode text strings.
// Use [Text] to create one, then chain builder methods to configure
// character filtering. Invalid configurations panic on the first [Draw] call.
type TextGenerator struct {
	minSize         int
	maxSize         int
	charFields      characterFields
	alphabetCalled  bool
	charParamCalled bool
}

// Text returns a TextGenerator that produces string values with codepoint count
// in [minSize, maxSize].
//
// Pass maxSize < 0 for unbounded.
func Text(minSize int, maxSize int) TextGenerator {
	return TextGenerator{
		minSize: minSize,
		maxSize: maxSize,
	}
}

// Codec restricts generated text to characters encodable in the given codec
// (e.g. "ascii", "utf-8", "latin-1").
func (g TextGenerator) Codec(codec string) TextGenerator {
	g.charParamCalled = true
	g.charFields.codec = &codec
	return g
}

// MinCodepoint sets the minimum Unicode codepoint.
func (g TextGenerator) MinCodepoint(cp rune) TextGenerator {
	g.charParamCalled = true
	g.charFields.minCodepoint = &cp
	return g
}

// MaxCodepoint sets the maximum Unicode codepoint.
func (g TextGenerator) MaxCodepoint(cp rune) TextGenerator {
	g.charParamCalled = true
	g.charFields.maxCodepoint = &cp
	return g
}

// Categories restricts generated characters to those in the given Unicode
// general categories (e.g. []string{"L", "Nd"}).
func (g TextGenerator) Categories(cats []string) TextGenerator {
	g.charParamCalled = true
	g.charFields.hasCategoriesSet = true
	g.charFields.categories = cats
	return g
}

// ExcludeCategories excludes characters in the given Unicode general categories.
func (g TextGenerator) ExcludeCategories(cats []string) TextGenerator {
	g.charParamCalled = true
	g.charFields.excludeCategories = cats
	return g
}

// IncludeCharacters always includes these specific characters, even if
// excluded by other filters.
func (g TextGenerator) IncludeCharacters(chars string) TextGenerator {
	g.charParamCalled = true
	g.charFields.includeCharacters = &chars
	return g
}

// ExcludeCharacters always excludes these specific characters.
func (g TextGenerator) ExcludeCharacters(chars string) TextGenerator {
	g.charParamCalled = true
	g.charFields.excludeCharacters = &chars
	return g
}

// Alphabet restricts generated strings to only contain characters from the
// given set. Mutually exclusive with the character filtering methods like
// Codec, Categories, MinCodepoint, etc.
func (g TextGenerator) Alphabet(chars string) TextGenerator {
	g.alphabetCalled = true
	g.charFields = characterFields{
		hasCategoriesSet:  true,
		categories:        []string{},
		includeCharacters: &chars,
	}
	return g
}

// asBasic validates the configuration and returns the basic generator with
// its wire schema. Returns an error on invalid combinations of settings.
func (g TextGenerator) asBasic() (*basicGenerator[string], bool, error) {
	if g.minSize < 0 {
		return nil, false, fmt.Errorf("min_size=%d must be non-negative", g.minSize)
	}
	if g.maxSize >= 0 && g.minSize > g.maxSize {
		return nil, false, fmt.Errorf("Cannot have max_size=%d < min_size=%d", g.maxSize, g.minSize)
	}
	if g.alphabetCalled && g.charParamCalled {
		return nil, false, fmt.Errorf("cannot combine Alphabet with character filtering methods")
	}
	schema := map[string]any{
		"type":     "string",
		"min_size": int64(g.minSize),
	}
	if g.maxSize >= 0 {
		schema["max_size"] = int64(g.maxSize)
	}
	charSchema, err := g.charFields.toSchema()
	if err != nil {
		return nil, false, err
	}
	for k, v := range charSchema {
		schema[k] = v
	}
	return &basicGenerator[string]{schema: schema, parse: extractString}, true, nil
}

func (g TextGenerator) draw(s *TestCase) string {
	bg, _, err := g.asBasic()
	if err != nil {
		panic(err.Error())
	}
	return bg.draw(s)
}

// CharactersGenerator configures and generates single-character strings.
// Use [Characters] to create one, then chain builder methods to configure
// character filtering.
type CharactersGenerator struct {
	charFields characterFields
}

// Characters returns a CharactersGenerator that produces single-codepoint strings.
func Characters() CharactersGenerator {
	return CharactersGenerator{}
}

// Codec restricts generated characters to those encodable in the given codec.
func (g CharactersGenerator) Codec(codec string) CharactersGenerator {
	g.charFields.codec = &codec
	return g
}

// MinCodepoint sets the minimum Unicode codepoint.
func (g CharactersGenerator) MinCodepoint(cp rune) CharactersGenerator {
	g.charFields.minCodepoint = &cp
	return g
}

// MaxCodepoint sets the maximum Unicode codepoint.
func (g CharactersGenerator) MaxCodepoint(cp rune) CharactersGenerator {
	g.charFields.maxCodepoint = &cp
	return g
}

// Categories restricts generated characters to the given Unicode general categories.
func (g CharactersGenerator) Categories(cats []string) CharactersGenerator {
	g.charFields.hasCategoriesSet = true
	g.charFields.categories = cats
	return g
}

// ExcludeCategories excludes characters in the given Unicode general categories.
func (g CharactersGenerator) ExcludeCategories(cats []string) CharactersGenerator {
	g.charFields.excludeCategories = cats
	return g
}

// IncludeCharacters always includes these specific characters.
func (g CharactersGenerator) IncludeCharacters(chars string) CharactersGenerator {
	g.charFields.includeCharacters = &chars
	return g
}

// ExcludeCharacters always excludes these specific characters.
func (g CharactersGenerator) ExcludeCharacters(chars string) CharactersGenerator {
	g.charFields.excludeCharacters = &chars
	return g
}

// asBasic validates the configuration and returns the basic generator with
// its wire schema. Returns an error on invalid combinations of settings.
func (g CharactersGenerator) asBasic() (*basicGenerator[string], bool, error) {
	schema := map[string]any{
		"type":     "string",
		"min_size": int64(1),
		"max_size": int64(1),
	}
	charSchema, err := g.charFields.toSchema()
	if err != nil {
		return nil, false, err
	}
	for k, v := range charSchema {
		schema[k] = v
	}
	return &basicGenerator[string]{schema: schema, parse: extractString}, true, nil
}

func (g CharactersGenerator) draw(s *TestCase) string {
	bg, _, err := g.asBasic()
	if err != nil {
		panic(err.Error())
	}
	return bg.draw(s)
}

// Binary returns a Generator that produces byte slices with length in [minSize, maxSize].
//
// Pass maxSize < 0 for unbounded.
func Binary(minSize int, maxSize int) Generator[[]byte] {
	if minSize < 0 {
		panic(fmt.Sprintf("min_size=%d must be non-negative", minSize))
	}
	if maxSize >= 0 && minSize > maxSize {
		panic(fmt.Sprintf("Cannot have max_size=%d < min_size=%d", maxSize, minSize))
	}
	schema := map[string]any{
		"type":     "binary",
		"min_size": int64(minSize),
	}
	if maxSize >= 0 {
		schema["max_size"] = int64(maxSize)
	}
	return &basicGenerator[[]byte]{schema: schema, parse: func(v any) []byte { return v.([]byte) }}
}

// Emails returns a Generator that produces email address strings.
func Emails() Generator[string] {
	return &basicGenerator[string]{
		schema: map[string]any{"type": "email"},
		parse:  extractString,
	}
}

// URLs returns a Generator that produces URL strings according to RFC3986.
//
// The schema is either "http" or "https".
func URLs() Generator[string] {
	return &basicGenerator[string]{
		schema: map[string]any{"type": "url"},
		parse:  extractString,
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

// asBasic validates the configuration and returns the basic generator with
// its wire schema. Returns an error if max_length is outside [4, 255].
func (g DomainGenerator) asBasic() (*basicGenerator[string], bool, error) {
	maxLen := defaultDomainMaxLength
	if g.hasMax {
		maxLen = g.maxLength
	}
	if maxLen < 4 || maxLen > 255 {
		return nil, false, fmt.Errorf("max_length=%d must be between 4 and 255", maxLen)
	}
	return &basicGenerator[string]{
		schema: map[string]any{
			"type":       "domain",
			"max_length": int64(maxLen),
		},
		parse: extractString,
	}, true, nil
}

func (g DomainGenerator) draw(s *TestCase) string {
	bg, _, err := g.asBasic()
	if err != nil {
		panic(err.Error())
	}
	return bg.draw(s)
}

// Dates returns a Generator that produces time.Time values from ISO 8601 date strings (YYYY-MM-DD).
func Dates() Generator[time.Time] {
	return &basicGenerator[time.Time]{
		schema: map[string]any{"type": "date"},
		parse: func(a any) time.Time {
			t, err := time.Parse("2006-01-02", a.(string))
			if err != nil { // coverage-ignore
				panic(fmt.Sprintf("failed to parse date %q: %v", a, err))
			}
			return t
		},
	}
}

// Datetimes returns a Generator that produces time.Time values from ISO 8601 datetime strings.
func Datetimes() Generator[time.Time] {
	return &basicGenerator[time.Time]{
		schema: map[string]any{"type": "datetime"},
		parse: func(a any) time.Time {
			t, err := time.Parse("2006-01-02T15:04:05", a.(string))
			if err != nil { // coverage-ignore
				panic(fmt.Sprintf("failed to parse datetime %q: %v", a, err))
			}
			return t
		},
	}
}

// Just returns a Generator that always produces the given constant value.
func Just[T any](value T) Generator[T] {
	return &basicGenerator[T]{
		schema: map[string]any{"type": "constant", "value": nil},
		parse:  func(_ any) T { return value },
	}
}

// SampledFrom returns a Generator that picks at random from values.
//
// Panics if values is empty.
func SampledFrom[T any](values []T) Generator[T] {
	if len(values) == 0 {
		panic("SampledFrom requires at least one element")
	}
	elements := make([]T, len(values))
	copy(elements, values)
	return &basicGenerator[T]{
		schema: map[string]any{
			"type":      "integer",
			"min_value": int64(0),
			"max_value": int64(len(elements) - 1),
		},
		parse: func(v any) T {
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
		parse: extractString,
	}
}
