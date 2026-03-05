package hegel

import (
	"fmt"
	"math/big"
)

// --- Span label constants ---

// spanLabel identifies the kind of generation span being tracked.
// The server uses these labels for better test-case shrinking.
type spanLabel int

const (
	// labelList marks a list generation span.
	labelList spanLabel = 1
	// labelListElement marks a list element generation span.
	labelListElement spanLabel = 2
	// labelSet marks a set generation span.
	labelSet spanLabel = 3
	// labelSetElement marks a set element generation span.
	labelSetElement spanLabel = 4
	// labelMap marks a map (dict) generation span.
	labelMap spanLabel = 5
	// labelMapEntry marks a map entry generation span.
	labelMapEntry spanLabel = 6
	// labelTuple marks a tuple generation span.
	labelTuple spanLabel = 7
	// labelOneOf marks a one-of (union) generation span.
	labelOneOf spanLabel = 8
	// labelOptional marks an optional value generation span.
	labelOptional spanLabel = 9
	// labelFixedDict marks a fixed-key dict generation span.
	labelFixedDict spanLabel = 10
	// labelFlatMap marks a flat-map generation span.
	labelFlatMap spanLabel = 11
	// labelFilter marks a filter generation span.
	labelFilter spanLabel = 12
	// labelMapped marks a mapped (transformed) generation span.
	labelMapped spanLabel = 13
	// labelSampledFrom marks a sampled-from generation span.
	labelSampledFrom spanLabel = 14
	// labelEnumVariant marks an enum variant generation span.
	labelEnumVariant spanLabel = 15
)

// --- Generator interface ---

// Generator is the core abstraction for value generation in Hegel.
// It is a generic, sealed interface — only types within this package can implement it.
type Generator[T any] interface {
	// draw produces a value from the Hegel server using the given state.
	// Unexported to seal the interface to this package.
	draw(s *TestCase) T
}

// testCase is the test context for a Hegel property test.
type testCase interface {
	// Assume rejects the current test case if condition is false.
	Assume(condition bool)

	// Note prints message during the final (replay) test case only.
	Note(message string)

	// Target sends a target value to guide test generation.
	Target(value float64, label string)

	// internal returns the underlying TestCase. Unexported to seal the interface.
	internal() *TestCase
}

// Draw produces a value from a Generator using the given State context.
func Draw[T any](tc testCase, g Generator[T]) T {
	return g.draw(tc.internal())
}

// --- basicGenerator ---

// basicGenerator is a generator backed by a single JSON-schema sent to the
// Hegel server. An optional transform function converts the raw CBOR value to T.
type basicGenerator[T any] struct {
	schema    map[string]any
	transform func(any) T // nil means the raw CBOR value is T
}

// isSchemaIdentity reports whether this basicGenerator has no transform,
// meaning the raw CBOR value is used directly.
func (g *basicGenerator[T]) isSchemaIdentity() bool {
	return g.transform == nil
}

// draw sends a generate command to the server and returns the result.
func (g *basicGenerator[T]) draw(s *TestCase) T {
	v, err := generateFromSchema(s, g.schema)
	if err != nil {
		panic(err)
	}
	if g.transform != nil {
		return g.transform(v)
	}
	return v.(T)
}

// --- mappedGenerator ---

// mappedGenerator wraps a Generator[T] and transforms its output to U.
// It emits start_span / stop_span around the inner draw call.
type mappedGenerator[T, U any] struct {
	inner Generator[T]
	fn    func(T) U
}

// draw calls the inner generator inside a MAPPED span and applies fn.
func (g *mappedGenerator[T, U]) draw(s *TestCase) U {
	var result U
	group(s, labelMapped, func() {
		result = g.fn(g.inner.draw(s))
	})
	return result
}

// --- filteredGenerator ---

// filteredGenerator wraps a source generator and a predicate, retrying up to
// maxFilterAttempts times before rejecting the test case.
type filteredGenerator[T any] struct {
	source    Generator[T]
	predicate func(T) bool
}

const maxFilterAttempts = 3

// draw tries up to maxFilterAttempts times to produce a value satisfying predicate.
func (g *filteredGenerator[T]) draw(s *TestCase) T {
	for range maxFilterAttempts {
		startSpan(s, labelFilter)
		value := g.source.draw(s)
		if g.predicate(value) {
			stopSpan(s, false)
			return value
		}
		stopSpan(s, true)
	}
	panic(assumeRejected{})
	// unreachable
}

// --- flatMappedGenerator ---

// flatMappedGenerator generates a value from source, passes it to f, and then
// generates from the generator returned by f. Wrapped in a FLAT_MAP span.
type flatMappedGenerator[T, U any] struct {
	source Generator[T]
	f      func(T) Generator[U]
}

// draw generates from source, then from the dependent generator, inside a FLAT_MAP span.
func (g *flatMappedGenerator[T, U]) draw(s *TestCase) U {
	var result U
	discardableGroup(s, labelFlatMap, func() {
		first := g.source.draw(s)
		secondGen := g.f(first)
		result = secondGen.draw(s)
	})
	return result
}

// --- Free function combinators ---

// Map returns a new Generator that applies fn to each value from g.
// If g is a basicGenerator, the transform is composed (preserving single-generate optimization).
func Map[T, U any](g Generator[T], fn func(T) U) Generator[U] {
	if bg, ok := g.(*basicGenerator[T]); ok {
		if bg.transform != nil {
			prev := bg.transform
			return &basicGenerator[U]{
				schema:    bg.schema,
				transform: func(v any) U { return fn(prev(v)) },
			}
		}
		return &basicGenerator[U]{
			schema:    bg.schema,
			transform: func(v any) U { return fn(v.(T)) },
		}
	}
	return &mappedGenerator[T, U]{inner: g, fn: fn}
}

// FlatMap returns a Generator that generates a value from g, passes it to f,
// and generates from the returned Generator. Always non-basic.
func FlatMap[T, U any](g Generator[T], f func(T) Generator[U]) Generator[U] {
	return &flatMappedGenerator[T, U]{source: g, f: f}
}

// Filter returns a Generator that only produces values from g that satisfy pred.
// It tries up to 3 times per test case; if all fail, the test case is rejected.
func Filter[T any](g Generator[T], pred func(T) bool) Generator[T] {
	return &filteredGenerator[T]{source: g, predicate: pred}
}

// --- Span helpers ---

// startSpan notifies the server that a new generation span has started.
func startSpan(gs *TestCase, label spanLabel) {
	if gs == nil || gs.aborted {
		return
	}
	ch := gs.channel
	payload, err := encodeCBOR(map[string]any{
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
func stopSpan(gs *TestCase, discard bool) {
	if gs == nil || gs.aborted {
		return
	}
	ch := gs.channel
	payload, err := encodeCBOR(map[string]any{
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
func group(gs *TestCase, label spanLabel, fn func()) {
	startSpan(gs, label)
	fn()
	stopSpan(gs, false)
}

// discardableGroup runs fn inside a start_span / stop_span pair.
// If fn panics, the span is ended with discard=true before re-panicking.
func discardableGroup(gs *TestCase, label spanLabel, fn func()) {
	startSpan(gs, label)
	panicked := true
	defer func() {
		stopSpan(gs, panicked)
	}()
	fn()
	panicked = false
}

// --- collection protocol ---

// collection manages a server-side collection (list/set/map) generation session.
type collection struct {
	serverName string
	finished   bool
}

// newCollection starts a new collection on the server with the given size bounds.
func newCollection(gs *TestCase, minSize, maxSize int) *collection {
	ch := gs.channel
	payload, err := encodeCBOR(map[string]any{
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
		re, ok := err.(*requestError)
		if ok && re.ErrorType == "StopTest" {
			gs.aborted = true
			panic(&dataExhausted{msg: "server ran out of data (new_collection)"})
		}
		panic(fmt.Sprintf("hegel: unreachable: new_collection error: %v", err))
	}
	name, _ := v.(string)
	return &collection{serverName: name}
}

// More asks the server whether another element should be generated.
func (c *collection) More(gs *TestCase) bool {
	if c.finished {
		return false
	}
	ch := gs.channel
	payload, err := encodeCBOR(map[string]any{
		"command":    "collection_more",
		"collection": c.serverName,
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: collection.More encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: More request: %v", err))
	}
	v, err := pending.Get()
	if err != nil {
		re, ok := err.(*requestError)
		if ok && re.ErrorType == "StopTest" {
			gs.aborted = true
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

// Reject tells the server that the last generated element should not count.
func (c *collection) Reject(gs *TestCase) {
	if c.finished {
		return
	}
	ch := gs.channel
	payload, err := encodeCBOR(map[string]any{
		"command":    "collection_reject",
		"collection": c.serverName,
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: collection.Reject encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: Reject request: %v", err))
	}
	pending.Get() //nolint:errcheck
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

// Integers returns a Generator that produces integer values in [minVal, maxVal].
func Integers(minVal, maxVal int64) Generator[int64] {
	return &basicGenerator[int64]{
		schema: map[string]any{
			"type":      "integer",
			"min_value": minVal,
			"max_value": maxVal,
		},
		transform: func(v any) int64 { return extractInt(v) },
	}
}

// IntegersUnbounded returns a Generator that produces unbounded integer values.
func IntegersUnbounded() Generator[int64] {
	return &basicGenerator[int64]{
		schema:    map[string]any{"type": "integer"},
		transform: func(v any) int64 { return extractInt(v) },
	}
}

// IntegersFrom returns a Generator that produces integers with optional bounds.
// Pass nil for minVal or maxVal to leave that bound unbounded.
func IntegersFrom(minVal, maxVal *int64) Generator[int64] {
	schema := map[string]any{"type": "integer"}
	if minVal != nil {
		schema["min_value"] = *minVal
	}
	if maxVal != nil {
		schema["max_value"] = *maxVal
	}
	return &basicGenerator[int64]{
		schema:    schema,
		transform: func(v any) int64 { return extractInt(v) },
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

// Booleans returns a Generator that produces boolean values with probability p of true.
func Booleans(p float64) Generator[bool] {
	return &basicGenerator[bool]{
		schema: map[string]any{
			"type": "boolean",
			"p":    p,
		},
	}
}

// Text returns a Generator that produces string values with codepoint count in
// [minSize, maxSize]. Pass maxSize < 0 for unbounded.
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

// Binary returns a Generator that produces byte slices with length in
// [minSize, maxSize]. Pass maxSize < 0 for unbounded.
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

// Dates returns a Generator that produces ISO 8601 date strings (YYYY-MM-DD).
func Dates() Generator[string] {
	return &basicGenerator[string]{
		schema: map[string]any{"type": "date"},
	}
}

// Times returns a Generator that produces time strings (HH:MM:SS or similar).
func Times() Generator[string] {
	return &basicGenerator[string]{
		schema: map[string]any{"type": "time"},
	}
}

// Datetimes returns a Generator that produces ISO 8601 datetime strings.
func Datetimes() Generator[string] {
	return &basicGenerator[string]{
		schema: map[string]any{"type": "datetime"},
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

// --- Lists generator ---

// ListsOptions holds optional size constraints for the Lists generator.
type ListsOptions struct {
	// MinSize is the minimum number of elements (inclusive). Defaults to 0.
	MinSize int
	// MaxSize is the maximum number of elements (inclusive). Negative means unbounded.
	MaxSize int
}

// Lists returns a Generator that produces slices of values from the elements generator.
//
// If elements is a basicGenerator (schema-backed), the list is generated with a single
// server call using a list schema. Otherwise, the collection protocol is used.
func Lists[T any](elements Generator[T], opts ListsOptions) Generator[[]T] {
	minSize := opts.MinSize
	if minSize < 0 {
		minSize = 0
	}

	if bg, ok := elements.(*basicGenerator[T]); ok {
		rawSchema := map[string]any{
			"type":     "list",
			"elements": bg.schema,
			"min_size": int64(minSize),
		}
		if opts.MaxSize >= 0 {
			rawSchema["max_size"] = int64(opts.MaxSize)
		}
		if bg.transform != nil {
			t := bg.transform
			return &basicGenerator[[]T]{
				schema: rawSchema,
				transform: func(raw any) []T {
					rawSlice, ok := raw.([]any)
					if !ok {
						return nil
					}
					result := make([]T, len(rawSlice))
					for i, x := range rawSlice {
						result[i] = t(x)
					}
					return result
				},
			}
		}
		return &basicGenerator[[]T]{
			schema: rawSchema,
			transform: func(raw any) []T {
				rawSlice, ok := raw.([]any)
				if !ok {
					return nil
				}
				result := make([]T, len(rawSlice))
				for i, x := range rawSlice {
					result[i] = x.(T)
				}
				return result
			},
		}
	}

	return &compositeListGenerator[T]{
		elements: elements,
		minSize:  minSize,
		maxSize:  opts.MaxSize,
	}
}

// compositeListGenerator generates a list using the collection protocol.
type compositeListGenerator[T any] struct {
	elements Generator[T]
	minSize  int
	maxSize  int
}

// draw produces a list by using the collection protocol inside a labelList span.
func (g *compositeListGenerator[T]) draw(s *TestCase) []T {
	var result []T
	startSpan(s, labelList)
	panicked := true
	defer func() {
		stopSpan(s, panicked)
	}()
	coll := newCollection(s, g.minSize, g.maxSize)
	for coll.More(s) {
		result = append(result, g.elements.draw(s))
	}
	panicked = false
	return result
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

// Dicts returns a Generator that produces map[K]V values.
func Dicts[K comparable, V any](keys Generator[K], values Generator[V], opts DictOptions) Generator[map[K]V] {
	keyBasic, keyIsBasic := keys.(*basicGenerator[K])
	valBasic, valIsBasic := values.(*basicGenerator[V])
	if keyIsBasic && valIsBasic {
		rawSchema := map[string]any{
			"type":     "dict",
			"keys":     keyBasic.schema,
			"values":   valBasic.schema,
			"min_size": int64(opts.MinSize),
		}
		if opts.HasMaxSize {
			rawSchema["max_size"] = int64(opts.MaxSize)
		}
		keyTransform := keyBasic.transform
		valTransform := valBasic.transform
		return &basicGenerator[map[K]V]{
			schema: rawSchema,
			transform: func(v any) map[K]V {
				return pairsToMap[K, V](v, keyTransform, valTransform)
			},
		}
	}
	return &compositeDictGenerator[K, V]{
		keys:    keys,
		values:  values,
		minSize: opts.MinSize,
		maxSize: opts.MaxSize,
		hasMax:  opts.HasMaxSize,
	}
}

// pairsToMap converts a CBOR-decoded pair list [[k,v], ...] to a map[K]V.
func pairsToMap[K comparable, V any](v any, keyTransform func(any) K, valTransform func(any) V) map[K]V {
	result := map[K]V{}
	pairs, ok := v.([]any)
	if !ok {
		return result
	}
	for _, pair := range pairs {
		kv, ok := pair.([]any)
		if !ok || len(kv) < 2 {
			continue
		}
		var k K
		if keyTransform != nil {
			k = keyTransform(kv[0])
		} else {
			k = kv[0].(K)
		}
		var val V
		if valTransform != nil {
			val = valTransform(kv[1])
		} else {
			val = kv[1].(V)
		}
		result[k] = val
	}
	return result
}

// compositeDictGenerator generates maps using the collection protocol.
type compositeDictGenerator[K comparable, V any] struct {
	keys    Generator[K]
	values  Generator[V]
	minSize int
	maxSize int
	hasMax  bool
}

// draw implements Generator by using the MAP span and collection protocol.
func (g *compositeDictGenerator[K, V]) draw(s *TestCase) map[K]V {
	var result map[K]V
	discardableGroup(s, labelMap, func() {
		maxSz := g.maxSize
		if !g.hasMax {
			maxSz = g.minSize + 10
		}
		coll := newCollection(s, g.minSize, maxSz)
		m := map[K]V{}
		for coll.More(s) {
			group(s, labelMapEntry, func() {
				k := g.keys.draw(s)
				v := g.values.draw(s)
				m[k] = v
			})
		}
		result = m
	})
	return result
}

// --- OneOf generator ---

// compositeOneOfGenerator generates a value from one of the given generators
// using the Hegel server to pick the branch.
type compositeOneOfGenerator[T any] struct {
	generators []Generator[T]
}

// draw picks one generator and returns a value from it, wrapped in a ONE_OF span.
func (g *compositeOneOfGenerator[T]) draw(s *TestCase) T {
	var result T
	group(s, labelOneOf, func() {
		n := len(g.generators)
		idx, err := generateFromSchema(s, map[string]any{
			"type":      "integer",
			"min_value": int64(0),
			"max_value": int64(n - 1),
		})
		if err != nil {
			panic(fmt.Sprintf("hegel: unreachable: OneOf generateFromSchema: %v", err))
		}
		i := extractInt(idx)
		result = g.generators[i].draw(s)
	})
	return result
}

// OneOf returns a Generator that produces values from one of the given generators.
//
// Path 1 — all basic with identity transforms: schema {"one_of": [s1, s2, ...]}
// Path 2 — all basic with some transforms: tagged-tuple schema, transform dispatches by tag
// Path 3 — any non-basic: compositeOneOfGenerator using ONE_OF span
//
// Requires at least 2 generators.
func OneOf[T any](generators ...Generator[T]) Generator[T] {
	if len(generators) < 2 {
		panic("hegel: OneOf requires at least 2 generators")
	}

	// Check if all generators are basic.
	allBasic := true
	for _, g := range generators {
		if _, ok := g.(*basicGenerator[T]); !ok {
			allBasic = false
			break
		}
	}

	if !allBasic {
		gens := make([]Generator[T], len(generators))
		copy(gens, generators)
		return &compositeOneOfGenerator[T]{generators: gens}
	}

	basics := make([]*basicGenerator[T], len(generators))
	for i, g := range generators {
		basics[i] = g.(*basicGenerator[T])
	}

	allIdentity := true
	for _, bg := range basics {
		if bg.transform != nil {
			allIdentity = false
			break
		}
	}

	if allIdentity {
		schemas := make([]any, len(basics))
		for i, bg := range basics {
			schemas[i] = bg.schema
		}
		return &basicGenerator[T]{
			schema: map[string]any{"one_of": schemas},
		}
	}

	// Path 2: tagged tuples
	taggedSchemas := make([]any, len(basics))
	for i, bg := range basics {
		taggedSchemas[i] = map[string]any{
			"type": "tuple",
			"elements": []any{
				map[string]any{"const": int64(i)},
				bg.schema,
			},
		}
	}

	transforms := make([]func(any) T, len(basics))
	for i, bg := range basics {
		transforms[i] = bg.transform
	}

	return &basicGenerator[T]{
		schema: map[string]any{"one_of": taggedSchemas},
		transform: func(tagged any) T {
			elems, _ := tagged.([]any)
			if len(elems) < 2 {
				return tagged.(T)
			}
			tag := extractInt(elems[0])
			value := elems[1]
			if t := transforms[tag]; t != nil {
				return t(value)
			}
			return value.(T)
		},
	}
}

// Optional returns a Generator that produces either nil (as *T) or a value from element.
func Optional[T any](element Generator[T]) Generator[*T] {
	return &optionalGenerator[T]{inner: element}
}

// optionalGenerator generates either nil or a value from inner.
type optionalGenerator[T any] struct {
	inner Generator[T]
}

// draw generates either nil or a value, wrapped in an OPTIONAL/ONE_OF span.
func (g *optionalGenerator[T]) draw(s *TestCase) *T {
	var result *T
	group(s, labelOneOf, func() {
		idx, err := generateFromSchema(s, map[string]any{
			"type":      "integer",
			"min_value": int64(0),
			"max_value": int64(1),
		})
		if err != nil {
			panic(fmt.Sprintf("hegel: unreachable: Optional generateFromSchema: %v", err))
		}
		i := extractInt(idx)
		if i == 0 {
			result = nil
		} else {
			v := g.inner.draw(s)
			result = &v
		}
	})
	return result
}

// --- IPAddresses generator ---

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
func IPAddresses(opts IPAddressOptions) Generator[string] {
	switch opts.Version {
	case IPVersion4:
		return &basicGenerator[string]{schema: map[string]any{"type": "ipv4"}}
	case IPVersion6:
		return &basicGenerator[string]{schema: map[string]any{"type": "ipv6"}}
	default:
		return OneOf[string](
			&basicGenerator[string]{schema: map[string]any{"type": "ipv4"}},
			&basicGenerator[string]{schema: map[string]any{"type": "ipv6"}},
		)
	}
}
