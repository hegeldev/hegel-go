package hegel

import (
	"fmt"
	"hash/maphash"
	"math"
	"reflect"
	"slices"
	"time"
	"unsafe"

	"hegel.dev/go/hegel/internal/libhegel"
)

type integer interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 | ~uintptr
}

type float interface {
	~float32 | ~float64
}

type drawFunc[T any] func(tc TestCase) (T, error)

type integerGeneratorKind struct{}
type booleanGeneratorKind struct{}
type binaryGeneratorKind struct{}
type emailGeneratorKind struct{}
type urlGeneratorKind struct{}
type regexGeneratorKind struct{}
type justGeneratorKind struct{}
type sampledGeneratorKind struct{}

//lint:ignore U1000 promoted by functionGenerator to satisfy Generator; staticcheck misses generic dispatch
func (f drawFunc[T]) draw(tc TestCase) (T, error) { return f(tc) }

type functionGenerator[T, K any] struct {
	drawFunc[T]
	config []any
}

func (g functionGenerator[T, K]) hashFields(h *maphash.Hash) bool {
	return hashSlice(h, g.config)
}

func generatorFunc[T, K any](draw drawFunc[T], config ...any) Generator[T] {
	return functionGenerator[T, K]{drawFunc: draw, config: config}
}

// --- Integers ---

// fitsInt64 reports whether v is representable as an int64. Signed integer
// types are always within range; an unsigned value is only out of range when it
// exceeds math.MaxInt64.
func fitsInt64[T integer](v T) bool {
	var zero T
	if ^zero > zero { // unsigned
		return uint64(v) <= math.MaxInt64
	}
	return true
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
	return generatorFunc[T, integerGeneratorKind](func(tc TestCase) (T, error) {
		ctx, ltc := tc.engine()
		if fitsInt64(minVal) && fitsInt64(maxVal) {
			v, err := ltc.GenerateInteger(ctx, int64(minVal), int64(maxVal))
			return T(v), err
		}
		// Only unsigned bounds above math.MaxInt64 reach here; both are
		// non-negative and fit in a uint64.
		raw, err := ltc.GenerateIntegerBig(ctx, libhegel.NewBigInt(minVal), libhegel.NewBigInt(maxVal))
		if err != nil {
			return 0, err
		}
		return T(raw.Uint64()), nil
	}, minVal, maxVal)
}

// --- Floats ---

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

func (g FloatGenerator[T]) hashFields(h *maphash.Hash) bool {
	return hashOptional(h, g.minVal) &&
		hashOptional(h, g.maxVal) &&
		hashOptional(h, g.allowNaN) &&
		hashOptional(h, g.allowInf) &&
		hashValues(h, g.excludeMin, g.excludeMax)
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

// params validates the configuration and returns the arguments for
// hegel_generate_float. Returns an error on invalid combinations of settings.
func (g FloatGenerator[T]) params() (width uint32, minVal, maxVal float64, nan, inf bool, smallestNonzero float64, err error) {
	hasMin := g.minVal != nil
	hasMax := g.maxVal != nil

	nan = !hasMin && !hasMax
	if g.allowNaN != nil {
		nan = *g.allowNaN
	}
	inf = !hasMin || !hasMax
	if g.allowInf != nil {
		inf = *g.allowInf
	}

	if nan && (hasMin || hasMax) {
		return 0, 0, 0, false, false, 0, fmt.Errorf("cannot have allow_nan=true with min_value or max_value")
	}
	// max_value < min_value is validated by the engine (hegel_generate_float),
	// which also accounts for exclusive-bound adjustment; no Go-side check.
	if inf && hasMin && hasMax {
		return 0, 0, 0, false, false, 0, fmt.Errorf("cannot have allow_infinity=true with both min_value and max_value")
	}

	width = uint32(unsafe.Sizeof(T(1.0)) * 8)
	minVal = math.Inf(-1)
	if hasMin {
		minVal = *g.minVal
	}
	maxVal = math.Inf(1)
	if hasMax {
		maxVal = *g.maxVal
	}
	// The smallest positive magnitude the engine may draw; the width-specific
	// smallest subnormal imposes no restriction.
	smallestNonzero = math.SmallestNonzeroFloat64
	if width == 32 {
		smallestNonzero = math.SmallestNonzeroFloat32
	}
	return width, minVal, maxVal, nan, inf, smallestNonzero, nil
}

// draw produces a floating-point value from the engine.
func (g FloatGenerator[T]) draw(tc TestCase) (T, error) {
	width, minVal, maxVal, nan, inf, smallest, err := g.params()
	if err != nil {
		var zero T
		return zero, err
	}
	ctx, ltc := tc.engine()
	v, err := ltc.GenerateFloat(ctx, width, minVal, maxVal, nan, inf, g.excludeMin, g.excludeMax, smallest)
	return T(v), err
}

// --- Booleans ---

// Booleans returns a Generator that produces boolean values.
func Booleans() Generator[bool] {
	return WeightedBooleans(0.5)
}

// WeightedBooleans returns a Generator that produces true with probability p.
// The probability must be within [0, 1]. Values 0 and 1 produce constants
// without consuming entropy.
func WeightedBooleans(p float64) Generator[bool] {
	return generatorFunc[bool, booleanGeneratorKind](func(tc TestCase) (bool, error) {
		ctx, ltc := tc.engine()
		return ltc.GenerateBoolean(ctx, p, false, false)
	}, math.Float64bits(p))
}

// --- Text and characters ---

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

func (cf characterFields) hashFields(h *maphash.Hash) bool {
	return hashOptional(h, cf.codec) &&
		hashOptional(h, cf.minCodepoint) &&
		hashOptional(h, cf.maxCodepoint) &&
		hashSlice(h, cf.categories) &&
		hashSlice(h, cf.excludeCategories) &&
		hashOptional(h, cf.includeCharacters) &&
		hashOptional(h, cf.excludeCharacters) &&
		hashValue(h, cf.hasCategoriesSet)
}

// textArgs returns the character-set arguments for
// [libhegel.Context.StringGeneratorText], automatically injecting surrogate
// exclusion for Go's UTF-8 strings. Returns an error when a requested category
// includes surrogate codepoints.
func (cf *characterFields) textArgs() (codec string, minCP, maxCP uint32, categories, excludeCategories []string, err error) {
	codec = "utf-8"
	if cf.codec != nil {
		codec = *cf.codec
	}
	maxCP = math.MaxUint32
	if cf.minCodepoint != nil {
		minCP = uint32(*cf.minCodepoint)
	}
	if cf.maxCodepoint != nil {
		maxCP = uint32(*cf.maxCodepoint)
	}
	if cf.hasCategoriesSet {
		for _, cat := range cf.categories {
			if slices.Contains(surrogateCategories, cat) {
				return "", 0, 0, nil, nil, fmt.Errorf(
					"category %q includes surrogate codepoints (Cs), "+
						"which Go strings cannot represent", cat)
			}
		}
		// A non-nil (possibly empty) categories slice requests exactly that set.
		categories = cf.categories
		if categories == nil {
			categories = []string{}
		}
		return codec, minCP, maxCP, categories, nil, nil
	}
	excl := append([]string{}, cf.excludeCategories...)
	hasCs := slices.Contains(excl, "Cs")
	if !hasCs {
		excl = append(excl, "Cs")
	}
	return codec, minCP, maxCP, nil, excl, nil
}

// TextGenerator configures and generates Unicode text strings.
// Use [Text] to create one, then chain builder methods to configure
// the size bounds and character filtering. Invalid configurations panic
// on the first [Draw] call.
type TextGenerator struct {
	minSize         int
	maxSize         int
	hasMax          bool
	charFields      characterFields
	alphabetCalled  bool
	charParamCalled bool
}

func (g TextGenerator) hashFields(h *maphash.Hash) bool {
	return hashValues(h, g.minSize, g.maxSize, g.hasMax) &&
		g.charFields.hashFields(h) &&
		hashValues(h, g.alphabetCalled, g.charParamCalled)
}

// Text returns a TextGenerator that produces string values. By default
// strings have no size bounds; use [TextGenerator.MinSize] and
// [TextGenerator.MaxSize] to constrain the codepoint count.
func Text() TextGenerator {
	return TextGenerator{}
}

// MinSize sets the minimum codepoint count for generated strings.
func (g TextGenerator) MinSize(n int) TextGenerator {
	g.minSize = n
	return g
}

// MaxSize sets the maximum codepoint count for generated strings.
func (g TextGenerator) MaxSize(n int) TextGenerator {
	g.maxSize = n
	g.hasMax = true
	return g
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

// draw validates the configuration, constructs the libhegel string generator,
// and generates a value. Configuration errors are returned before the engine is
// touched, so validation is exercisable without a live engine.
func (g TextGenerator) draw(tc TestCase) (string, error) {
	if g.minSize < 0 {
		return "", fmt.Errorf("min_size=%d must be non-negative", g.minSize)
	}
	if g.hasMax && g.maxSize < 0 {
		return "", fmt.Errorf("max_size=%d must be non-negative", g.maxSize)
	}
	// max_size < min_size is validated by the engine (hegel_string_generator_text).
	if g.alphabetCalled && g.charParamCalled {
		return "", fmt.Errorf("cannot combine Alphabet with character filtering methods")
	}
	maxSize := uint64(math.MaxUint64)
	if g.hasMax {
		maxSize = uint64(g.maxSize)
	}
	codec, minCP, maxCP, cats, exclCats, err := g.charFields.textArgs()
	if err != nil {
		return "", err
	}
	ctx, ltc := tc.engine()
	sg, err := ctx.StringGeneratorText(uint64(g.minSize), maxSize, codec, minCP, maxCP,
		cats, exclCats, g.charFields.includeCharacters, g.charFields.excludeCharacters)
	if err != nil {
		return "", err
	}
	return ltc.GenerateString(ctx, sg)
}

// CharactersGenerator configures and generates single-character strings.
// Use [Characters] to create one, then chain builder methods to configure
// character filtering.
type CharactersGenerator struct {
	charFields characterFields
}

func (g CharactersGenerator) hashFields(h *maphash.Hash) bool {
	return g.charFields.hashFields(h)
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

// draw constructs a single-codepoint libhegel string generator and generates a
// value. Configuration errors are returned before the engine is touched.
func (g CharactersGenerator) draw(tc TestCase) (string, error) {
	codec, minCP, maxCP, cats, exclCats, err := g.charFields.textArgs()
	if err != nil {
		return "", err
	}
	ctx, ltc := tc.engine()
	sg, err := ctx.StringGeneratorText(1, 1, codec, minCP, maxCP,
		cats, exclCats, g.charFields.includeCharacters, g.charFields.excludeCharacters)
	if err != nil {
		return "", err
	}
	return ltc.GenerateString(ctx, sg)
}

// --- Binary ---

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
	maxVal := uint64(math.MaxUint64)
	if maxSize >= 0 {
		maxVal = uint64(maxSize)
	}
	return generatorFunc[[]byte, binaryGeneratorKind](func(tc TestCase) ([]byte, error) {
		ctx, ltc := tc.engine()
		return ltc.GenerateBytes(ctx, uint64(minSize), maxVal)
	}, minSize, maxSize)
}

// --- String formats ---

// Emails returns a Generator that produces email address strings.
func Emails() Generator[string] {
	return generatorFunc[string, emailGeneratorKind](func(tc TestCase) (string, error) {
		ctx, ltc := tc.engine()
		sg, err := ctx.StringGeneratorEmail()
		if err != nil { // coverage-ignore (email generator construction never fails)
			return "", err
		}
		return ltc.GenerateString(ctx, sg)
	})
}

// URLs returns a Generator that produces URL strings according to RFC3986.
//
// The scheme is either "http" or "https".
func URLs() Generator[string] {
	return generatorFunc[string, urlGeneratorKind](func(tc TestCase) (string, error) {
		ctx, ltc := tc.engine()
		sg, err := ctx.StringGeneratorURL()
		if err != nil { // coverage-ignore (url generator construction never fails)
			return "", err
		}
		return ltc.GenerateString(ctx, sg)
	})
}

const defaultDomainMaxLength = 255

// DomainGenerator configures and generates domain name strings.
// Use [Domains] to create one, then chain builder methods to configure it.
// Invalid configurations panic on the first [Draw] call.
type DomainGenerator struct {
	maxLength int
	hasMax    bool
}

func (g DomainGenerator) hashFields(h *maphash.Hash) bool {
	return hashValues(h, g.maxLength, g.hasMax)
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

// build constructs the libhegel domain string generator. The max_length bound
// (RFC 1035: at most 255 octets, and large enough to fit at least one label
// plus a TLD) is validated by the engine (hegel_string_generator_domain).
func (g DomainGenerator) build(ctx *libhegel.Context) (*libhegel.StringGenerator, error) {
	maxLen := defaultDomainMaxLength
	if g.hasMax {
		maxLen = g.maxLength
	}
	return ctx.StringGeneratorDomain(uint64(maxLen))
}

func (g DomainGenerator) draw(tc TestCase) (string, error) {
	ctx, ltc := tc.engine()
	sg, err := g.build(ctx)
	if err != nil {
		return "", err
	}
	return ltc.GenerateString(ctx, sg)
}

// FromRegex returns a Generator that produces strings matching the given regular expression.
func FromRegex(pattern string, fullmatch bool) Generator[string] {
	return generatorFunc[string, regexGeneratorKind](func(tc TestCase) (string, error) {
		ctx, ltc := tc.engine()
		// No alphabet constraint: padding and wildcards draw from the full
		// character set (NULL on the C side).
		sg, err := ctx.StringGeneratorRegex(pattern, fullmatch, nil)
		if err != nil {
			return "", err
		}
		return ltc.GenerateString(ctx, sg)
	}, pattern, fullmatch)
}

// --- Dates and datetimes ---

// fullDateMin / fullDateMax are the conventional full Gregorian date range.
var (
	fullDateMin = libhegel.Date{Year: 1, Month: 1, Day: 1}
	fullDateMax = libhegel.Date{Year: 9999, Month: 12, Day: 31}
	fullTimeMax = libhegel.Time{Hour: 23, Minute: 59, Second: 59, Nanosecond: 999999999}
)

func dateFromTime(v time.Time) (libhegel.Date, error) {
	year := v.Year()
	if year < -999999 || year > 999999 {
		return libhegel.Date{}, fmt.Errorf("date year must be between -999999 and 999999, got %d", year)
	}
	return libhegel.Date{Year: int32(year), Month: uint8(v.Month()), Day: uint8(v.Day())}, nil
}

func datetimeFromTime(v time.Time) (libhegel.Datetime, error) {
	date, err := dateFromTime(v)
	if err != nil {
		return libhegel.Datetime{}, err
	}
	return libhegel.Datetime{
		Date: date,
		Time: libhegel.Time{
			Hour:       uint8(v.Hour()),
			Minute:     uint8(v.Minute()),
			Second:     uint8(v.Second()),
			Nanosecond: uint32(v.Nanosecond()),
		},
	}, nil
}

// DateGenerator generates dates at midnight UTC within inclusive bounds.
type DateGenerator struct {
	minVal *time.Time
	maxVal *time.Time
}

func (g DateGenerator) hashFields(h *maphash.Hash) bool {
	return hashOptionalTime(h, g.minVal, false) && hashOptionalTime(h, g.maxVal, false)
}

var _ Generator[time.Time] = DateGenerator{}

// Dates returns a DateGenerator with default bounds of 0001-01-01 and
// 9999-12-31. Custom bounds may use years from -999999 through 999999.
func Dates() DateGenerator {
	return DateGenerator{}
}

// Min sets the inclusive minimum from v's date fields. It ignores the time and
// location.
func (g DateGenerator) Min(v time.Time) DateGenerator {
	g.minVal = &v
	return g
}

// Max sets the inclusive maximum from v's date fields. It ignores the time and
// location.
func (g DateGenerator) Max(v time.Time) DateGenerator {
	g.maxVal = &v
	return g
}

func (g DateGenerator) draw(tc TestCase) (time.Time, error) {
	minVal, maxVal := fullDateMin, fullDateMax
	if g.minVal != nil {
		var err error
		minVal, err = dateFromTime(*g.minVal)
		if err != nil {
			return time.Time{}, err
		}
	}
	if g.maxVal != nil {
		var err error
		maxVal, err = dateFromTime(*g.maxVal)
		if err != nil {
			return time.Time{}, err
		}
	}
	ctx, ltc := tc.engine()
	d, err := ltc.GenerateDate(ctx, minVal, maxVal)
	if err != nil {
		return time.Time{}, err
	}
	return d.ToTime(), nil
}

// DatetimeGenerator generates UTC datetimes within inclusive bounds.
type DatetimeGenerator struct {
	minVal *time.Time
	maxVal *time.Time
}

func (g DatetimeGenerator) hashFields(h *maphash.Hash) bool {
	return hashOptionalTime(h, g.minVal, true) && hashOptionalTime(h, g.maxVal, true)
}

var _ Generator[time.Time] = DatetimeGenerator{}

// Datetimes returns a DatetimeGenerator with default bounds from 0001-01-01
// through the last nanosecond of 9999-12-31. Custom bounds may use years from
// -999999 through 999999.
func Datetimes() DatetimeGenerator {
	return DatetimeGenerator{}
}

// Min sets the inclusive minimum from v's wall-clock fields. It ignores the
// location.
func (g DatetimeGenerator) Min(v time.Time) DatetimeGenerator {
	g.minVal = &v
	return g
}

// Max sets the inclusive maximum from v's wall-clock fields. It ignores the
// location.
func (g DatetimeGenerator) Max(v time.Time) DatetimeGenerator {
	g.maxVal = &v
	return g
}

func (g DatetimeGenerator) draw(tc TestCase) (time.Time, error) {
	minVal := libhegel.Datetime{Date: fullDateMin}
	maxVal := libhegel.Datetime{Date: fullDateMax, Time: fullTimeMax}
	if g.minVal != nil {
		var err error
		minVal, err = datetimeFromTime(*g.minVal)
		if err != nil {
			return time.Time{}, err
		}
	}
	if g.maxVal != nil {
		var err error
		maxVal, err = datetimeFromTime(*g.maxVal)
		if err != nil {
			return time.Time{}, err
		}
	}
	ctx, ltc := tc.engine()
	dt, err := ltc.GenerateDatetime(ctx, minVal, maxVal)
	if err != nil {
		return time.Time{}, err
	}
	return dt.ToTime(), nil
}

// --- Constants and sampling ---

// Just returns a Generator that always produces the given constant value.
func Just[T any](value T) Generator[T] {
	return generatorFunc[T, justGeneratorKind](func(tc TestCase) (T, error) {
		return value, nil
	}, reflect.TypeOf(value))
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
	return generatorFunc[T, sampledGeneratorKind](func(tc TestCase) (T, error) {
		ctx, ltc := tc.engine()
		idx, err := ltc.GenerateInteger(ctx, 0, int64(len(elements)-1))
		if err != nil {
			var zero T
			return zero, err
		}
		return elements[idx], nil
	}, reflect.TypeOf(elements))
}
