package hegel

import (
	"fmt"
)

// --- Span label constants ---

// SpanLabel identifies the kind of generation span being tracked.
// The server uses these labels for better test-case shrinking.
type SpanLabel int

const (
	// LabelList marks a list generation span.
	LabelList SpanLabel = 1
	// LabelListElement marks a list element generation span.
	LabelListElement SpanLabel = 2
	// LabelSet marks a set generation span.
	LabelSet SpanLabel = 3
	// LabelSetElement marks a set element generation span.
	LabelSetElement SpanLabel = 4
	// LabelMap marks a map (dict) generation span.
	LabelMap SpanLabel = 5
	// LabelMapEntry marks a map entry generation span.
	LabelMapEntry SpanLabel = 6
	// LabelTuple marks a tuple generation span.
	LabelTuple SpanLabel = 7
	// LabelOneOf marks a one-of (union) generation span.
	LabelOneOf SpanLabel = 8
	// LabelOptional marks an optional value generation span.
	LabelOptional SpanLabel = 9
	// LabelFixedDict marks a fixed-key dict generation span.
	LabelFixedDict SpanLabel = 10
	// LabelFlatMap marks a flat-map generation span.
	LabelFlatMap SpanLabel = 11
	// LabelFilter marks a filter generation span.
	LabelFilter SpanLabel = 12
	// LabelMapped marks a mapped (transformed) generation span.
	LabelMapped SpanLabel = 13
	// LabelSampledFrom marks a sampled-from generation span.
	LabelSampledFrom SpanLabel = 14
	// LabelEnumVariant marks an enum variant generation span.
	LabelEnumVariant SpanLabel = 15
)

// --- Generator[T] ---

// Generator is the core type-safe value generation type in Hegel.
// Use [Draw] to produce a value from a Generator inside a Hegel test.
//
// Generators are constructed via primitive constructors (e.g. [Integers],
// [Text]) and combined via free functions ([Map], [Filter], [FlatMap],
// [Lists], [Dicts], [OneOf], etc.).
type Generator[T any] struct {
	drawFn            func(*testCaseData) T
	schema            map[string]any // nil = non-basic (composite)
	transform         func(any) T    // for basic generators: raw CBOR → T
	identityTransform bool           // true if transform is just type coercion (no user logic)
}

// isBasic returns true if this generator has a schema (can be sent in a single generate command).
func (g *Generator[T]) isBasic() bool {
	return g.schema != nil
}

// newBasicGenerator creates a basic (schema-backed) generator.
func newBasicGenerator[T any](schema map[string]any, transform func(any) T, identity bool) *Generator[T] {
	g := &Generator[T]{
		schema:            schema,
		transform:         transform,
		identityTransform: identity,
	}
	g.drawFn = func(data *testCaseData) T {
		v, err := generateFromSchema(schema, data)
		if err != nil {
			panic(err)
		}
		return transform(v)
	}
	return g
}

// newCompositeGenerator creates a non-basic generator with a custom draw function.
func newCompositeGenerator[T any](drawFn func(*testCaseData) T) *Generator[T] {
	return &Generator[T]{drawFn: drawFn}
}

// --- Span helpers ---

// startSpan notifies the server that a new generation span has started.
// label identifies the kind of span (e.g. LabelList, LabelMapped).
// No-op if the test has been aborted.
func startSpan(label SpanLabel, data *testCaseData) {
	ch := data.channel
	payload, err := EncodeCBOR(map[string]any{
		"command": "start_span",
		"label":   int64(label),
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: startSpan encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: startSpan request: %v", err))
	}
	pending.Get() //nolint:errcheck
}

// stopSpan notifies the server that the current generation span has ended.
// If discard is true, the span's data should be discarded from the shrinking budget.
// No-op if the test has been aborted.
func stopSpan(discard bool, data *testCaseData) {
	if data.aborted {
		return
	}
	ch := data.channel
	payload, err := EncodeCBOR(map[string]any{
		"command": "stop_span",
		"discard": discard,
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: stopSpan encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: stopSpan request: %v", err))
	}
	pending.Get() //nolint:errcheck
}

// group runs fn inside a start_span / stop_span pair with the given label.
// The span is never discarded (discard=false).
func group(label SpanLabel, fn func(), data *testCaseData) {
	startSpan(label, data)
	fn()
	stopSpan(false, data)
}

// discardableGroup runs fn inside a start_span / stop_span pair.
// If fn panics, the span is ended with discard=true before re-panicking.
func discardableGroup(label SpanLabel, fn func(), data *testCaseData) {
	startSpan(label, data)
	panicked := true
	defer func() {
		stopSpan(panicked, data)
	}()
	fn()
	panicked = false
}

// --- Collection protocol ---

// collection manages a server-side collection (list/set/map) generation session.
type collection struct {
	serverName string // assigned by server on new_collection
	finished   bool
}

// newCollection starts a new collection on the server with the given size bounds.
// It sends the new_collection command immediately.
func newCollection(minSize, maxSize int, data *testCaseData) *collection {
	ch := data.channel
	payload, err := EncodeCBOR(map[string]any{
		"command":  "new_collection",
		"min_size": int64(minSize),
		"max_size": int64(maxSize),
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: newCollection encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: newCollection request: %v", err))
	}
	v, err := pending.Get()
	if err != nil {
		re, ok := err.(*RequestError)
		if ok && re.ErrorType == "StopTest" {
			data.aborted = true
			panic(&dataExhausted{msg: "server ran out of data (new_collection)"})
		}
		panic(fmt.Sprintf("hegel: unreachable: new_collection error: %v", err))
	}
	name, _ := extractString(v)
	return &collection{serverName: name}
}

// more asks the server whether another element should be generated.
// Returns false when the collection is exhausted; subsequent calls return false
// without sending any messages.
func (c *collection) more(data *testCaseData) bool {
	if c.finished {
		return false
	}
	ch := data.channel
	payload, err := EncodeCBOR(map[string]any{
		"command":    "collection_more",
		"collection": c.serverName,
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: collection.more encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: more request: %v", err))
	}
	v, err := pending.Get()
	if err != nil {
		re, ok := err.(*RequestError)
		if ok && re.ErrorType == "StopTest" {
			data.aborted = true
			panic(&dataExhausted{msg: "server ran out of data (collection_more)"})
		}
		panic(fmt.Sprintf("hegel: unreachable: collection_more error: %v", err))
	}
	more, _ := v.(bool)
	if !more {
		c.finished = true
	}
	return more
}

// --- Primitive constructors ---

// toInt64 converts a CBOR-decoded value to int64 (for use as a Generator transform).
func toInt64(v any) int64 { n, _ := extractInt(v); return n }

// toFloat64 converts a CBOR-decoded value to float64 (for use as a Generator transform).
func toFloat64(v any) float64 { f, _ := extractFloat(v); return f }

// toString converts a CBOR-decoded value to string (for use as a Generator transform).
func toString(v any) string { s, _ := v.(string); return s }

// toBool converts a CBOR-decoded value to bool (for use as a Generator transform).
func toBool(v any) bool { b, _ := v.(bool); return b }

// toBytes converts a CBOR-decoded value to []byte (for use as a Generator transform).
func toBytes(v any) []byte { b, _ := v.([]byte); return b }

// Integers returns a Generator that produces integer values in [minVal, maxVal].
func Integers(minVal, maxVal int64) *Generator[int64] {
	return newBasicGenerator(
		map[string]any{
			"type":      "integer",
			"min_value": minVal,
			"max_value": maxVal,
		},
		toInt64, true,
	)
}

// IntegersUnbounded returns a Generator that produces unbounded integer values.
func IntegersUnbounded() *Generator[int64] {
	return newBasicGenerator(
		map[string]any{"type": "integer"},
		toInt64, true,
	)
}

// IntegersFrom returns a Generator that produces integers with optional bounds.
// Pass nil for minVal or maxVal to leave that bound unbounded.
func IntegersFrom(minVal, maxVal *int64) *Generator[int64] {
	schema := map[string]any{"type": "integer"}
	if minVal != nil {
		schema["min_value"] = *minVal
	}
	if maxVal != nil {
		schema["max_value"] = *maxVal
	}
	return newBasicGenerator(schema, toInt64, true)
}

// Floats returns a Generator that produces float64 values.
//
// minVal and maxVal set the inclusive bounds (nil means unbounded).
// allowNaN controls whether NaN is permitted; if nil, defaults to true only when
// both bounds are nil. allowInfinity controls whether +/-Inf is permitted; if nil,
// defaults to true unless both bounds are set.
// excludeMin and excludeMax make the respective bound exclusive.
func Floats(minVal, maxVal *float64, allowNaN, allowInfinity *bool, excludeMin, excludeMax bool) *Generator[float64] {
	hasMin := minVal != nil
	hasMax := maxVal != nil

	// Default allow_nan: true only when no bounds set.
	nan := !hasMin && !hasMax
	if allowNaN != nil {
		nan = *allowNaN
	}
	// Default allow_infinity: true unless both bounds set.
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
	return newBasicGenerator(schema, toFloat64, true)
}

// Booleans returns a Generator that produces boolean values with probability p
// of generating true. p must be in [0, 1]; 0.5 gives equal probability.
func Booleans(p float64) *Generator[bool] {
	return newBasicGenerator(
		map[string]any{"type": "boolean", "p": p},
		toBool, true,
	)
}

// Text returns a Generator that produces string values with codepoint count in
// [minSize, maxSize]. Pass maxSize < 0 for unbounded.
func Text(minSize int, maxSize int) *Generator[string] {
	schema := map[string]any{
		"type":     "string",
		"min_size": int64(minSize),
	}
	if maxSize >= 0 {
		schema["max_size"] = int64(maxSize)
	}
	return newBasicGenerator(schema, toString, true)
}

// Binary returns a Generator that produces byte slices with length in
// [minSize, maxSize]. Pass maxSize < 0 for unbounded.
func Binary(minSize int, maxSize int) *Generator[[]byte] {
	schema := map[string]any{
		"type":     "binary",
		"min_size": int64(minSize),
	}
	if maxSize >= 0 {
		schema["max_size"] = int64(maxSize)
	}
	return newBasicGenerator(schema, toBytes, true)
}

// Just returns a Generator that always produces the given constant value.
// The schema uses {"const": null} and the transform ignores the server result.
func Just[T any](value T) *Generator[T] {
	return newBasicGenerator(
		map[string]any{"const": nil},
		func(_ any) T { return value },
		false,
	)
}

// FromRegex returns a Generator that produces strings matching the given regular expression.
// If fullmatch is true (the default), the entire string must match.
func FromRegex(pattern string, fullmatch bool) *Generator[string] {
	return newBasicGenerator(
		map[string]any{
			"type":      "regex",
			"pattern":   pattern,
			"fullmatch": fullmatch,
		},
		toString, true,
	)
}

// --- Format generators ---

// Emails returns a Generator that produces email address strings.
func Emails() *Generator[string] {
	return newBasicGenerator(map[string]any{"type": "email"}, toString, true)
}

// URLs returns a Generator that produces URL strings.
func URLs() *Generator[string] {
	return newBasicGenerator(map[string]any{"type": "url"}, toString, true)
}

// DomainOptions holds options for the Domains generator.
type DomainOptions struct {
	// MaxLength is the maximum length of the domain name.
	// Zero means use the default maximum length (255, matching RFC 1035).
	MaxLength int
}

// defaultDomainMaxLength is the default maximum domain name length per RFC 1035,
// matching hypothesis's default for domains().
const defaultDomainMaxLength = 255

// Domains returns a Generator that produces domain name strings.
// If opts.MaxLength > 0, generated domains will not exceed that length.
// Otherwise, the default maximum length of 255 is used.
func Domains(opts DomainOptions) *Generator[string] {
	maxLen := opts.MaxLength
	if maxLen <= 0 {
		maxLen = defaultDomainMaxLength
	}
	return newBasicGenerator(
		map[string]any{"type": "domain", "max_length": int64(maxLen)},
		toString, true,
	)
}

// Dates returns a Generator that produces ISO 8601 date strings (YYYY-MM-DD).
func Dates() *Generator[string] {
	return newBasicGenerator(map[string]any{"type": "date"}, toString, true)
}

// Times returns a Generator that produces time strings (HH:MM:SS or similar).
func Times() *Generator[string] {
	return newBasicGenerator(map[string]any{"type": "time"}, toString, true)
}

// Datetimes returns a Generator that produces ISO 8601 datetime strings.
func Datetimes() *Generator[string] {
	return newBasicGenerator(map[string]any{"type": "datetime"}, toString, true)
}

// --- IP Addresses ---

// IPAddressVersion specifies which IP version to generate.
type IPAddressVersion int

const (
	// IPVersion4 generates IPv4 addresses.
	IPVersion4 IPAddressVersion = 4
	// IPVersion6 generates IPv6 addresses.
	IPVersion6 IPAddressVersion = 6
)

// IPAddressOptions holds options for the IPAddresses generator.
type IPAddressOptions struct {
	// Version selects the IP version (4 or 6). Zero means generate both.
	Version IPAddressVersion
}

// IPAddresses returns a Generator that produces IP address strings.
// With Version=4 it generates IPv4 addresses (dotted decimal).
// With Version=6 it generates IPv6 addresses (colon hex).
// With Version=0 (default) it generates either IPv4 or IPv6.
func IPAddresses(opts IPAddressOptions) *Generator[string] {
	switch opts.Version {
	case IPVersion4:
		return newBasicGenerator(map[string]any{"type": "ipv4"}, toString, true)
	case IPVersion6:
		return newBasicGenerator(map[string]any{"type": "ipv6"}, toString, true)
	default:
		return OneOf(
			newBasicGenerator[string](map[string]any{"type": "ipv4"}, toString, true),
			newBasicGenerator[string](map[string]any{"type": "ipv6"}, toString, true),
		)
	}
}

// --- Free-function combinators ---

const maxFilterAttempts = 3

// Map returns a new generator that applies fn to each value produced by g.
// If g is a basic generator, the schema is preserved and the transform is composed
// (preserving the single-generate-call optimisation).
func Map[T, U any](g *Generator[T], fn func(T) U) *Generator[U] {
	if g.schema != nil {
		// Basic path: preserve schema, compose transforms.
		oldTransform := g.transform
		return newBasicGenerator(g.schema, func(v any) U {
			return fn(oldTransform(v))
		}, false)
	}
	// Non-basic path: wrap in a MAPPED span.
	oldDraw := g.drawFn
	return newCompositeGenerator(func(data *testCaseData) U {
		var result U
		group(LabelMapped, func() {
			result = fn(oldDraw(data))
		}, data)
		return result
	})
}

// Filter returns a new generator that only produces values from g satisfying pred.
// It tries up to 3 times per test case; if all attempts fail, the test case
// is rejected via Assume(false). The result is always non-basic.
func Filter[T any](g *Generator[T], pred func(T) bool) *Generator[T] {
	oldDraw := g.drawFn
	return newCompositeGenerator(func(data *testCaseData) T {
		for range maxFilterAttempts {
			startSpan(LabelFilter, data)
			value := oldDraw(data)
			if pred(value) {
				stopSpan(false, data)
				return value
			}
			stopSpan(true, data)
		}
		panic(assumeRejected{})
	})
}

// FlatMap returns a new generator for dependent generation. It generates a value
// from g, passes it to fn, and generates from the returned generator.
// Always non-basic — wrapped in a FLAT_MAP span.
func FlatMap[T, U any](g *Generator[T], fn func(T) *Generator[U]) *Generator[U] {
	oldDraw := g.drawFn
	return newCompositeGenerator(func(data *testCaseData) U {
		var result U
		discardableGroup(LabelFlatMap, func() {
			first := oldDraw(data)
			secondGen := fn(first)
			result = secondGen.drawFn(data)
		}, data)
		return result
	})
}

// SampledFrom returns a Generator that picks uniformly at random from values.
// The server generates an integer index in [0, len(values)-1], which is mapped
// to the corresponding element. Returns an error if values is empty.
func SampledFrom[T any](values []T) (*Generator[T], error) {
	elements := make([]T, len(values))
	copy(elements, values)
	if len(elements) == 0 {
		return nil, fmt.Errorf("sampled_from requires at least one element")
	}
	schema := map[string]any{
		"type":      "integer",
		"min_value": int64(0),
		"max_value": int64(len(elements) - 1),
	}
	return newBasicGenerator(schema, func(v any) T {
		idx, _ := extractInt(v)
		return elements[idx]
	}, false), nil
}

// MustSampledFrom returns a Generator that picks uniformly at random from values.
// Panics if values is empty.
func MustSampledFrom[T any](values []T) *Generator[T] {
	g, err := SampledFrom(values)
	if err != nil {
		panic(err)
	}
	return g
}

// --- Lists generator ---

// ListsOptions holds optional size constraints for the Lists generator.
type ListsOptions struct {
	// MinSize is the minimum number of elements (inclusive). Defaults to 0.
	MinSize int
	// MaxSize is the maximum number of elements (inclusive). Negative means unbounded.
	MaxSize int
}

// Lists returns a Generator that produces slices of values from the elem generator.
//
// If elem is a basic generator (schema-backed), the list is generated with a single
// server call using a list schema. This is the fast path.
//
// If elem is a non-basic generator (e.g., filtered), the collection protocol is used:
// the server controls iteration via new_collection / collection_more.
func Lists[T any](elem *Generator[T], opts ListsOptions) *Generator[[]T] {
	minSize := opts.MinSize
	if minSize < 0 {
		minSize = 0
	}

	if elem.schema != nil {
		// Fast path: build a list schema.
		rawSchema := map[string]any{
			"type":     "list",
			"elements": elem.schema,
			"min_size": int64(minSize),
		}
		if opts.MaxSize >= 0 {
			rawSchema["max_size"] = int64(opts.MaxSize)
		}
		et := elem.transform
		return newBasicGenerator(rawSchema, func(v any) []T {
			rawSlice, ok := v.([]any)
			if !ok {
				return nil
			}
			result := make([]T, len(rawSlice))
			for i, x := range rawSlice {
				result[i] = et(x)
			}
			return result
		}, false)
	}

	// Non-basic path: use collection protocol.
	elemDraw := elem.drawFn
	maxSize := opts.MaxSize
	return newCompositeGenerator(func(data *testCaseData) []T {
		var result []T
		startSpan(LabelList, data)
		panicked := true
		defer func() {
			stopSpan(panicked, data)
		}()
		coll := newCollection(minSize, maxSize, data)
		for coll.more(data) {
			result = append(result, elemDraw(data))
		}
		panicked = false
		return result
	})
}

// --- Dicts generator ---

// DictOptions holds optional parameters for the Dicts generator.
type DictOptions struct {
	// MinSize is the minimum number of key-value pairs. Defaults to 0.
	MinSize int
	// MaxSize is the maximum number of key-value pairs. Negative means unbounded.
	MaxSize int
	// HasMaxSize indicates whether MaxSize should be applied.
	HasMaxSize bool
}

// Dicts returns a Generator that produces map[K]V values with keys from
// the keys generator and values from the vals generator.
//
// When both keys and vals are basic generators, a single schema-based
// generate command is sent to the server (the fast path). Otherwise, the
// collection protocol is used to build the map incrementally.
func Dicts[K comparable, V any](keys *Generator[K], vals *Generator[V], opts DictOptions) *Generator[map[K]V] {
	if keys.schema != nil && vals.schema != nil {
		// Fast path: both generators are basic — compose a single schema.
		rawSchema := map[string]any{
			"type":     "dict",
			"keys":     keys.schema,
			"values":   vals.schema,
			"min_size": int64(opts.MinSize),
		}
		if opts.HasMaxSize {
			rawSchema["max_size"] = int64(opts.MaxSize)
		}
		kt := keys.transform
		vt := vals.transform
		return newBasicGenerator(rawSchema, func(v any) map[K]V {
			return pairsToMap(v, kt, vt)
		}, false)
	}

	// Slow path: use the collection protocol.
	keyDraw := keys.drawFn
	valDraw := vals.drawFn
	return newCompositeGenerator(func(data *testCaseData) map[K]V {
		var result map[K]V
		discardableGroup(LabelMap, func() {
			maxSz := opts.MaxSize
			if !opts.HasMaxSize {
				maxSz = opts.MinSize + 10
			}
			coll := newCollection(opts.MinSize, maxSz, data)
			result = make(map[K]V)
			for coll.more(data) {
				group(LabelMapEntry, func() {
					k := keyDraw(data)
					v := valDraw(data)
					result[k] = v
				}, data)
			}
		}, data)
		return result
	})
}

// pairsToMap converts a CBOR-decoded pair list [[k,v], ...] to a map[K]V,
// applying key and value transforms.
func pairsToMap[K comparable, V any](v any, keyTransform func(any) K, valTransform func(any) V) map[K]V {
	result := make(map[K]V)
	pairs, ok := v.([]any)
	if !ok {
		return result
	}
	for _, pair := range pairs {
		kv, ok := pair.([]any)
		if !ok || len(kv) < 2 {
			continue
		}
		result[keyTransform(kv[0])] = valTransform(kv[1])
	}
	return result
}

// --- OneOf generator ---

// OneOf returns a Generator that produces values from one of the given generators.
//
// Path 1 — all basic with identity transforms: schema {"one_of": [s1, s2, ...]}
// Path 2 — all basic with some user transforms: tagged-tuple schema, transform dispatches by tag
// Path 3 — any non-basic: composite using ONE_OF span
//
// Requires at least 2 generators.
func OneOf[T any](gens ...*Generator[T]) *Generator[T] {
	if len(gens) < 2 {
		panic("hegel: OneOf requires at least 2 generators")
	}

	// Check if all generators are basic.
	allBasic := true
	for _, g := range gens {
		if g.schema == nil {
			allBasic = false
			break
		}
	}

	if !allBasic {
		// Path 3: composite
		draws := make([]func(*testCaseData) T, len(gens))
		for i, g := range gens {
			draws[i] = g.drawFn
		}
		return newCompositeGenerator(func(data *testCaseData) T {
			var result T
			group(LabelOneOf, func() {
				n := len(draws)
				idx, err := generateFromSchema(map[string]any{
					"type":      "integer",
					"min_value": int64(0),
					"max_value": int64(n - 1),
				}, data)
				if err != nil {
					panic(fmt.Sprintf("hegel: unreachable: OneOf generateFromSchema: %v", err))
				}
				i := toInt64(idx)
				result = draws[i](data)
			}, data)
			return result
		})
	}

	// All are basic — check if all have identity transforms.
	allIdentity := true
	for _, g := range gens {
		if !g.identityTransform {
			allIdentity = false
			break
		}
	}

	if allIdentity {
		// Path 1: simple {"one_of": [s1, s2, ...]}
		schemas := make([]any, len(gens))
		for i, g := range gens {
			schemas[i] = g.schema
		}
		return newBasicGenerator(
			map[string]any{"one_of": schemas},
			gens[0].transform,
			true,
		)
	}

	// Path 2: tagged tuples — wrap each branch as {"type":"tuple","elements":[{"const":i},schema]}
	taggedSchemas := make([]any, len(gens))
	for i, g := range gens {
		taggedSchemas[i] = map[string]any{
			"type": "tuple",
			"elements": []any{
				map[string]any{"const": int64(i)},
				g.schema,
			},
		}
	}

	// Capture transforms slice.
	transforms := make([]func(any) T, len(gens))
	for i, g := range gens {
		transforms[i] = g.transform
	}

	return newBasicGenerator(
		map[string]any{"one_of": taggedSchemas},
		func(tagged any) T {
			elems, _ := tagged.([]any)
			if len(elems) < 2 {
				var zero T
				return zero
			}
			tag := toInt64(elems[0])
			return transforms[tag](elems[1])
		},
		false,
	)
}

// Optional returns a Generator that produces either nil (*T == nil) or
// a pointer to a value from element (*T != nil).
func Optional[T any](element *Generator[T]) *Generator[*T] {
	nilGen := Just[*T](nil)
	nonNilGen := Map(element, func(v T) *T { return &v })
	return OneOf(nilGen, nonNilGen)
}

// --- Tuple types ---

// Tuple2 holds a pair of values.
type Tuple2[A, B any] struct {
	A A
	B B
}

// Tuple3 holds a triple of values.
type Tuple3[A, B, C any] struct {
	A A
	B B
	C C
}

// Tuple4 holds a quadruple of values.
type Tuple4[A, B, C, D any] struct {
	A A
	B B
	C C
	D D
}

// --- Tuple generators ---

// Tuples2 returns a Generator that produces 2-element tuples.
// If both elements are basic (schema-backed), a single generate command is sent.
// Otherwise, elements are generated separately inside a TUPLE span.
func Tuples2[A, B any](g1 *Generator[A], g2 *Generator[B]) *Generator[Tuple2[A, B]] {
	if g1.schema != nil && g2.schema != nil {
		schemas := []any{g1.schema, g2.schema}
		combined := map[string]any{"type": "tuple", "elements": schemas}
		t1 := g1.transform
		t2 := g2.transform
		return newBasicGenerator(combined, func(v any) Tuple2[A, B] {
			elems, _ := v.([]any)
			var a A
			var b B
			if len(elems) >= 2 {
				a = t1(elems[0])
				b = t2(elems[1])
			}
			return Tuple2[A, B]{A: a, B: b}
		}, false)
	}
	// Composite path.
	d1 := g1.drawFn
	d2 := g2.drawFn
	return newCompositeGenerator(func(data *testCaseData) Tuple2[A, B] {
		var result Tuple2[A, B]
		group(LabelTuple, func() {
			result.A = d1(data)
			result.B = d2(data)
		}, data)
		return result
	})
}

// Tuples3 returns a Generator that produces 3-element tuples.
// If all elements are basic, a single generate command is sent.
// Otherwise, elements are generated separately inside a TUPLE span.
func Tuples3[A, B, C any](g1 *Generator[A], g2 *Generator[B], g3 *Generator[C]) *Generator[Tuple3[A, B, C]] {
	if g1.schema != nil && g2.schema != nil && g3.schema != nil {
		schemas := []any{g1.schema, g2.schema, g3.schema}
		combined := map[string]any{"type": "tuple", "elements": schemas}
		t1 := g1.transform
		t2 := g2.transform
		t3 := g3.transform
		return newBasicGenerator(combined, func(v any) Tuple3[A, B, C] {
			elems, _ := v.([]any)
			var a A
			var b B
			var c C
			if len(elems) >= 3 {
				a = t1(elems[0])
				b = t2(elems[1])
				c = t3(elems[2])
			}
			return Tuple3[A, B, C]{A: a, B: b, C: c}
		}, false)
	}
	d1 := g1.drawFn
	d2 := g2.drawFn
	d3 := g3.drawFn
	return newCompositeGenerator(func(data *testCaseData) Tuple3[A, B, C] {
		var result Tuple3[A, B, C]
		group(LabelTuple, func() {
			result.A = d1(data)
			result.B = d2(data)
			result.C = d3(data)
		}, data)
		return result
	})
}

// Tuples4 returns a Generator that produces 4-element tuples.
// If all elements are basic, a single generate command is sent.
// Otherwise, elements are generated separately inside a TUPLE span.
func Tuples4[A, B, C, D any](g1 *Generator[A], g2 *Generator[B], g3 *Generator[C], g4 *Generator[D]) *Generator[Tuple4[A, B, C, D]] {
	if g1.schema != nil && g2.schema != nil && g3.schema != nil && g4.schema != nil {
		schemas := []any{g1.schema, g2.schema, g3.schema, g4.schema}
		combined := map[string]any{"type": "tuple", "elements": schemas}
		t1 := g1.transform
		t2 := g2.transform
		t3 := g3.transform
		t4 := g4.transform
		return newBasicGenerator(combined, func(v any) Tuple4[A, B, C, D] {
			elems, _ := v.([]any)
			var a A
			var b B
			var c C
			var d D
			if len(elems) >= 4 {
				a = t1(elems[0])
				b = t2(elems[1])
				c = t3(elems[2])
				d = t4(elems[3])
			}
			return Tuple4[A, B, C, D]{A: a, B: b, C: c, D: d}
		}, false)
	}
	d1 := g1.drawFn
	d2 := g2.drawFn
	d3 := g3.drawFn
	d4 := g4.drawFn
	return newCompositeGenerator(func(data *testCaseData) Tuple4[A, B, C, D] {
		var result Tuple4[A, B, C, D]
		group(LabelTuple, func() {
			result.A = d1(data)
			result.B = d2(data)
			result.C = d3(data)
			result.D = d4(data)
		}, data)
		return result
	})
}
