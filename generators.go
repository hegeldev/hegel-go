package hegel

import (
	"fmt"
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
// Use Draw(gen) to produce a value from a Generator inside a Hegel test.
type Generator interface {
	// DoDraw produces a value from the Hegel server using the given goroutine state.
	// Callers should use the public Draw() function instead of calling this directly.
	DoDraw(data *TestCase) any

	// AsBasic returns the underlying basicGenerator if this generator is
	// a basic (schema-only) generator, or nil otherwise.
	// Used internally to optimise composed generators.
	AsBasic() *basicGenerator

	// Map returns a new generator that applies fn to each generated value.
	Map(fn func(any) any) Generator

	// Filter returns a new generator that only produces values satisfying pred.
	// It tries up to 3 times per test case; if all attempts fail, the test case
	// is rejected via Assume(false). The result is always non-basic.
	Filter(pred func(any) bool) Generator
}

// --- basicGenerator ---

// basicGenerator is a generator backed by a single JSON-schema sent to the
// Hegel server. An optional transform function is applied to the raw value.
// Mapping a basicGenerator preserves the schema optimisation.
type basicGenerator struct {
	schema    map[string]any
	transform func(any) any // nil means identity
}

// DoDraw sends a generate command to the server and returns the result,
// applying any registered transform.
func (g *basicGenerator) DoDraw(data *TestCase) any {
	v, err := generateFromSchema(data, g.schema)
	if err != nil {
		panic(err)
	}
	if g.transform != nil {
		return g.transform(v)
	}
	return v
}

// AsBasic returns g itself — basicGenerator is its own basic form.
func (g *basicGenerator) AsBasic() *basicGenerator { return g }

// Filter returns a filteredGenerator that only produces values satisfying pred.
func (g *basicGenerator) Filter(pred func(any) bool) Generator {
	return &filteredGenerator{source: g, predicate: pred}
}

// Map returns a new basicGenerator with the same schema and a composed
// transform function (preserving the single-generate-call optimisation).
func (g *basicGenerator) Map(fn func(any) any) Generator {
	if g.transform != nil {
		prev := g.transform
		return &basicGenerator{
			schema:    g.schema,
			transform: func(v any) any { return fn(prev(v)) },
		}
	}
	return &basicGenerator{
		schema:    g.schema,
		transform: fn,
	}
}

// --- mappedGenerator ---

// mappedGenerator wraps a non-basic generator and applies a transform.
// It emits start_span / stop_span around the inner Generate call so the
// server can track the mapping for better shrinking.
type mappedGenerator struct {
	inner Generator
	fn    func(any) any
}

// DoDraw calls the inner generator inside a MAPPED span and applies fn.
func (g *mappedGenerator) DoDraw(data *TestCase) any {
	var result any
	group(labelMapped, func() {
		result = g.fn(g.inner.DoDraw(data))
	}, data)
	return result
}

// AsBasic returns nil — mappedGenerator is not a basic generator.
func (g *mappedGenerator) AsBasic() *basicGenerator { return nil }

// Filter returns a filteredGenerator that only produces values satisfying pred.
func (g *mappedGenerator) Filter(pred func(any) bool) Generator {
	return &filteredGenerator{source: g, predicate: pred}
}

// Map returns a new mappedGenerator that composes fn with g's transform.
func (g *mappedGenerator) Map(fn func(any) any) Generator {
	return &mappedGenerator{
		inner: g,
		fn:    fn,
	}
}

// --- filteredGenerator ---

// filteredGenerator wraps a source generator and a predicate, retrying up to
// maxFilterAttempts times before rejecting the test case via Assume(false).
// Each attempt is wrapped in a discardable FILTER span so the server can
// reclaim the data budget for failed attempts.
type filteredGenerator struct {
	source    Generator
	predicate func(any) bool
}

const maxFilterAttempts = 3

// DoDraw tries up to maxFilterAttempts times to produce a value from source
// that satisfies predicate. Each attempt is wrapped in a FILTER span; spans
// for failed attempts are discarded. If all attempts fail, the test case is
// rejected via panic(assumeRejected{}).
func (g *filteredGenerator) DoDraw(data *TestCase) any {
	for range maxFilterAttempts {
		startSpan(labelFilter, data)
		value := g.source.DoDraw(data)
		if g.predicate(value) {
			stopSpan(false, data)
			return value
		}
		stopSpan(true, data)
	}
	panic(assumeRejected{})
}

// AsBasic returns nil — filteredGenerator is never a basic generator.
func (g *filteredGenerator) AsBasic() *basicGenerator { return nil }

// Filter returns a new filteredGenerator composed from this one and pred.
func (g *filteredGenerator) Filter(pred func(any) bool) Generator {
	return &filteredGenerator{source: g, predicate: pred}
}

// Map returns a new mappedGenerator that applies fn to values from this generator.
func (g *filteredGenerator) Map(fn func(any) any) Generator {
	return &mappedGenerator{inner: g, fn: fn}
}

// --- Span helpers ---

// startSpan notifies the server that a new generation span has started.
// label identifies the kind of span (e.g. labelList, labelMapped).
// No-op if the test has been aborted.
func startSpan(label spanLabel, data *TestCase) {
	ch := data.channel
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
// If discard is true, the span's data should be discarded from the shrinking budget.
// No-op if the test has been aborted.
func stopSpan(discard bool, data *TestCase) {
	if data.aborted {
		return
	}
	ch := data.channel
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
// The span is never discarded (discard=false).
func group(label spanLabel, fn func(), data *TestCase) {
	startSpan(label, data)
	fn()
	stopSpan(false, data)
}

// discardableGroup runs fn inside a start_span / stop_span pair.
// If fn panics, the span is ended with discard=true before re-panicking.
func discardableGroup(label spanLabel, fn func(), data *TestCase) {
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
func newCollection(minSize, maxSize int, data *TestCase) *collection {
	ch := data.channel
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
			data.aborted = true
			panic(&dataExhausted{msg: "server ran out of data (new_collection)"})
		}
		panic(fmt.Sprintf("hegel: unreachable: new_collection error: %v", err))
	}
	name, _ := ExtractString(v)
	return &collection{serverName: name}
}

// more asks the server whether another element should be generated.
// Returns false when the collection is exhausted; subsequent calls return false
// without sending any messages.
func (c *collection) more(data *TestCase) bool {
	if c.finished {
		return false
	}
	ch := data.channel
	payload, err := encodeCBOR(map[string]any{
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
		re, ok := err.(*requestError)
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

// --- Built-in generators ---

// Integers returns a Generator that produces integer values in [minVal, maxVal].
func Integers(minVal, maxVal int64) Generator {
	return &basicGenerator{
		schema: map[string]any{
			"type":      "integer",
			"min_value": minVal,
			"max_value": maxVal,
		},
	}
}

// IntegersUnbounded returns a Generator that produces unbounded integer values.
func IntegersUnbounded() Generator {
	return &basicGenerator{
		schema: map[string]any{
			"type": "integer",
		},
	}
}

// Emails returns a Generator that produces email address strings.
func Emails() Generator {
	return &basicGenerator{
		schema: map[string]any{
			"type": "email",
		},
	}
}

// URLs returns a Generator that produces URL strings.
func URLs() Generator {
	return &basicGenerator{
		schema: map[string]any{
			"type": "url",
		},
	}
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
func Domains(opts DomainOptions) Generator {
	maxLen := opts.MaxLength
	if maxLen <= 0 {
		maxLen = defaultDomainMaxLength
	}
	schema := map[string]any{
		"type":       "domain",
		"max_length": int64(maxLen),
	}
	return &basicGenerator{schema: schema}
}

// Dates returns a Generator that produces ISO 8601 date strings (YYYY-MM-DD).
func Dates() Generator {
	return &basicGenerator{
		schema: map[string]any{
			"type": "date",
		},
	}
}

// Times returns a Generator that produces time strings (HH:MM:SS or similar).
func Times() Generator {
	return &basicGenerator{
		schema: map[string]any{
			"type": "time",
		},
	}
}

// Datetimes returns a Generator that produces ISO 8601 datetime strings.
func Datetimes() Generator {
	return &basicGenerator{
		schema: map[string]any{
			"type": "datetime",
		},
	}
}

// Just returns a Generator that always produces the given constant value.
// The schema uses {"const": null} and the transform ignores the server result.
func Just(value any) *basicGenerator {
	return &basicGenerator{
		schema:    map[string]any{"const": nil},
		transform: func(_ any) any { return value },
	}
}

// SampledFrom returns a Generator that picks uniformly at random from values.
// The server generates an integer index in [0, len(values)-1], which is mapped
// to the corresponding element. Returns an error if values is empty.
func SampledFrom(values []any) (*basicGenerator, error) {
	elements := make([]any, len(values))
	copy(elements, values)
	if len(elements) == 0 {
		return nil, fmt.Errorf("sampled_from requires at least one element")
	}
	schema := map[string]any{
		"type":      "integer",
		"min_value": int64(0),
		"max_value": int64(len(elements) - 1),
	}
	return &basicGenerator{
		schema: schema,
		transform: func(v any) any {
			idx, _ := ExtractInt(v)
			return elements[idx]
		},
	}, nil
}

// MustSampledFrom returns a Generator that picks uniformly at random from values.
// Panics if values is empty.
func MustSampledFrom(values []any) *basicGenerator {
	g, err := SampledFrom(values)
	if err != nil {
		panic(err)
	}
	return g
}

// FromRegex returns a Generator that produces strings matching the given regular expression.
// If fullmatch is true (the default), the entire string must match.
func FromRegex(pattern string, fullmatch bool) *basicGenerator {
	return &basicGenerator{
		schema: map[string]any{
			"type":      "regex",
			"pattern":   pattern,
			"fullmatch": fullmatch,
		},
	}
}

// IntegersFrom returns a Generator that produces integers with optional bounds.
// Pass nil for minVal or maxVal to leave that bound unbounded.
func IntegersFrom(minVal, maxVal *int64) Generator {
	schema := map[string]any{"type": "integer"}
	if minVal != nil {
		schema["min_value"] = *minVal
	}
	if maxVal != nil {
		schema["max_value"] = *maxVal
	}
	return &basicGenerator{schema: schema}
}

// Floats returns a Generator that produces float64 values.
//
// minVal and maxVal set the inclusive bounds (nil means unbounded).
// allowNaN controls whether NaN is permitted; if nil, defaults to true only when
// both bounds are nil. allowInfinity controls whether +/-Inf is permitted; if nil,
// defaults to true unless both bounds are set.
// excludeMin and excludeMax make the respective bound exclusive.
func Floats(minVal, maxVal *float64, allowNaN, allowInfinity *bool, excludeMin, excludeMax bool) Generator {
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
	return &basicGenerator{schema: schema}
}

// Booleans returns a Generator that produces boolean values with probability p
// of generating true. p must be in [0, 1]; 0.5 gives equal probability.
func Booleans(p float64) Generator {
	return &basicGenerator{
		schema: map[string]any{
			"type": "boolean",
			"p":    p,
		},
	}
}

// Text returns a Generator that produces string values with codepoint count in
// [minSize, maxSize]. Pass maxSize < 0 for unbounded.
func Text(minSize int, maxSize int) Generator {
	schema := map[string]any{
		"type":     "string",
		"min_size": int64(minSize),
	}
	if maxSize >= 0 {
		schema["max_size"] = int64(maxSize)
	}
	return &basicGenerator{schema: schema}
}

// Binary returns a Generator that produces byte slices with length in
// [minSize, maxSize]. Pass maxSize < 0 for unbounded.
// The server returns CBOR byte strings decoded directly as []byte.
func Binary(minSize int, maxSize int) Generator {
	schema := map[string]any{
		"type":     "binary",
		"min_size": int64(minSize),
	}
	if maxSize >= 0 {
		schema["max_size"] = int64(maxSize)
	}
	return &basicGenerator{schema: schema}
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
// server call using a list schema, and the element transform (if any) is applied to each
// item in the result. This is the fast path.
//
// If elements is a non-basic generator (e.g., filtered), the collection protocol is used:
// the server controls iteration via new_collection / collection_more, and each element is
// generated individually inside a labelList span.
//
// opts.MinSize defaults to 0; opts.MaxSize < 0 means no upper bound.
func Lists(elements Generator, opts ListsOptions) Generator {
	minSize := opts.MinSize
	if minSize < 0 {
		minSize = 0
	}

	bg := elements.AsBasic()
	if bg != nil {
		// Fast path: build a list schema using the element's raw schema.
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
			listTransform := func(raw any) any {
				rawSlice, ok := raw.([]any)
				if !ok {
					return raw
				}
				result := make([]any, len(rawSlice))
				for i, x := range rawSlice {
					result[i] = t(x)
				}
				return result
			}
			return &basicGenerator{schema: rawSchema, transform: listTransform}
		}
		return &basicGenerator{schema: rawSchema}
	}

	// Non-basic path: use collection protocol.
	return &compositeListGenerator{
		elements: elements,
		minSize:  minSize,
		maxSize:  opts.MaxSize,
	}
}

// compositeListGenerator generates a list using the collection protocol.
// Used when the element generator is non-basic (e.g., filtered).
type compositeListGenerator struct {
	elements Generator
	minSize  int
	maxSize  int
}

// DoDraw produces a list by using the collection protocol inside a labelList span.
func (g *compositeListGenerator) DoDraw(data *TestCase) any {
	var result []any
	startSpan(labelList, data)
	panicked := true
	defer func() {
		stopSpan(panicked, data)
	}()
	coll := newCollection(g.minSize, g.maxSize, data)
	for coll.more(data) {
		result = append(result, g.elements.DoDraw(data))
	}
	panicked = false
	return result
}

// AsBasic returns nil — compositeListGenerator is not a basic generator.
func (g *compositeListGenerator) AsBasic() *basicGenerator { return nil }

// Filter returns a filteredGenerator that only produces values satisfying pred.
func (g *compositeListGenerator) Filter(pred func(any) bool) Generator {
	return &filteredGenerator{source: g, predicate: pred}
}

// Map returns a new mappedGenerator wrapping g.
func (g *compositeListGenerator) Map(fn func(any) any) Generator {
	return &mappedGenerator{inner: g, fn: fn}
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

// Dicts returns a Generator that produces map[any]any values with keys from
// the keys generator and values from the values generator.
//
// When both keys and values are BasicGenerators, a single schema-based
// generate command is sent to the server (the fast path). Otherwise, the
// collection protocol is used to build the map incrementally.
//
// Use DictOptions to control MinSize and MaxSize of the generated maps.
func Dicts(keys, values Generator, opts DictOptions) Generator {
	keyBasic := keys.AsBasic()
	valBasic := values.AsBasic()
	if keyBasic != nil && valBasic != nil {
		// Fast path: both generators are basic — compose a single schema.
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
		if keyTransform == nil && valTransform == nil {
			return &basicGenerator{
				schema: rawSchema,
				transform: func(v any) any {
					return pairsToMap(v, nil, nil)
				},
			}
		}
		return &basicGenerator{
			schema: rawSchema,
			transform: func(v any) any {
				return pairsToMap(v, keyTransform, valTransform)
			},
		}
	}
	// Slow path: use the collection protocol.
	return &compositeDictGenerator{
		keys:    keys,
		values:  values,
		minSize: opts.MinSize,
		maxSize: opts.MaxSize,
		hasMax:  opts.HasMaxSize,
	}
}

// pairsToMap converts a CBOR-decoded pair list [[k,v], ...] to a map[any]any,
// applying optional key and value transforms.
func pairsToMap(v any, keyTransform, valTransform func(any) any) any {
	result := map[any]any{}
	pairs, ok := v.([]any)
	if !ok {
		return result
	}
	for _, pair := range pairs {
		kv, ok := pair.([]any)
		if !ok || len(kv) < 2 {
			continue
		}
		k := kv[0]
		val := kv[1]
		if keyTransform != nil {
			k = keyTransform(k)
		}
		if valTransform != nil {
			val = valTransform(val)
		}
		result[k] = val
	}
	return result
}

// compositeDictGenerator generates maps using the collection protocol for
// non-basic key or value generators.
type compositeDictGenerator struct {
	keys    Generator
	values  Generator
	minSize int
	maxSize int
	hasMax  bool
}

// DoDraw implements Generator by using the MAP span and collection protocol.
func (g *compositeDictGenerator) DoDraw(data *TestCase) any {
	var result any
	discardableGroup(labelMap, func() {
		maxSz := g.maxSize
		if !g.hasMax {
			maxSz = g.minSize + 10
		}
		coll := newCollection(g.minSize, maxSz, data)
		m := map[any]any{}
		for coll.more(data) {
			group(labelMapEntry, func() {
				k := g.keys.DoDraw(data)
				v := g.values.DoDraw(data)
				m[k] = v
			}, data)
		}
		result = m
	}, data)
	return result
}

// AsBasic returns nil — compositeDictGenerator is not a basic generator.
func (g *compositeDictGenerator) AsBasic() *basicGenerator { return nil }

// Filter returns a filteredGenerator that only produces values satisfying pred.
func (g *compositeDictGenerator) Filter(pred func(any) bool) Generator {
	return &filteredGenerator{source: g, predicate: pred}
}

// Map returns a new mappedGenerator wrapping this generator.
func (g *compositeDictGenerator) Map(fn func(any) any) Generator {
	return &mappedGenerator{inner: g, fn: fn}
}

// --- OneOf generator ---

// compositeOneOfGenerator is a one_of generator for generators that cannot all
// be represented as BasicGenerators (e.g. filtered generators). It generates an
// integer index and delegates to the selected branch, wrapped in a ONE_OF span.
type compositeOneOfGenerator struct {
	generators []Generator
}

// DoDraw picks one of the generators at random (via the Hegel server) and
// returns a value from that generator, wrapped in a ONE_OF span.
func (g *compositeOneOfGenerator) DoDraw(data *TestCase) any {
	var result any
	group(labelOneOf, func() {
		n := len(g.generators)
		idx, err := generateFromSchema(data, map[string]any{
			"type":      "integer",
			"min_value": int64(0),
			"max_value": int64(n - 1),
		})
		if err != nil {
			panic(fmt.Sprintf("hegel: unreachable: OneOf generateFromSchema: %v", err))
		}
		i, _ := ExtractInt(idx)
		result = g.generators[i].DoDraw(data)
	}, data)
	return result
}

// AsBasic returns nil — compositeOneOfGenerator is not a basic generator.
func (g *compositeOneOfGenerator) AsBasic() *basicGenerator { return nil }

// Filter returns a filteredGenerator that only produces values satisfying pred.
func (g *compositeOneOfGenerator) Filter(pred func(any) bool) Generator {
	return &filteredGenerator{source: g, predicate: pred}
}

// Map returns a new mappedGenerator that applies fn to each generated value.
func (g *compositeOneOfGenerator) Map(fn func(any) any) Generator {
	return &mappedGenerator{inner: g, fn: fn}
}

// OneOf returns a Generator that produces values from one of the given generators.
//
// Path 1 — all basic with identity transforms: schema {"one_of": [s1, s2, ...]}
// Path 2 — all basic with some transforms: tagged-tuple schema, transform dispatches by tag
// Path 3 — any non-basic: compositeOneOfGenerator using ONE_OF span
//
// Requires at least 2 generators.
func OneOf(generators ...Generator) Generator {
	if len(generators) < 2 {
		panic("hegel: OneOf requires at least 2 generators")
	}

	// Check if all generators are basic.
	allBasic := true
	for _, g := range generators {
		if g.AsBasic() == nil {
			allBasic = false
			break
		}
	}

	if !allBasic {
		// Path 3: composite
		gens := make([]Generator, len(generators))
		copy(gens, generators)
		return &compositeOneOfGenerator{generators: gens}
	}

	// All are basic — collect them.
	basics := make([]*basicGenerator, len(generators))
	for i, g := range generators {
		basics[i] = g.AsBasic()
	}

	// Check if all have identity (nil) transforms.
	allIdentity := true
	for _, bg := range basics {
		if bg.transform != nil {
			allIdentity = false
			break
		}
	}

	if allIdentity {
		// Path 1: simple {"one_of": [s1, s2, ...]}
		schemas := make([]any, len(basics))
		for i, bg := range basics {
			schemas[i] = bg.schema
		}
		return &basicGenerator{
			schema: map[string]any{"one_of": schemas},
		}
	}

	// Path 2: tagged tuples — wrap each branch as {"type":"tuple","elements":[{"const":i},schema]}
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

	// Capture transforms slice (each entry may be nil = identity).
	transforms := make([]func(any) any, len(basics))
	for i, bg := range basics {
		transforms[i] = bg.transform
	}

	applyTagged := func(tagged any) any {
		// tagged is a CBOR-decoded tuple: []any{tag, value}
		elems, _ := tagged.([]any)
		if len(elems) < 2 {
			return tagged
		}
		tag, _ := ExtractInt(elems[0])
		value := elems[1]
		if t := transforms[tag]; t != nil {
			return t(value)
		}
		return value
	}

	return &basicGenerator{
		schema:    map[string]any{"one_of": taggedSchemas},
		transform: applyTagged,
	}
}

// Optional returns a Generator that produces either nil or a value from element.
// It is equivalent to OneOf(Just(nil), element).
func Optional(element Generator) Generator {
	return OneOf(Just(nil), element)
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
// With Version=4 it generates IPv4 addresses (dotted decimal).
// With Version=6 it generates IPv6 addresses (colon hex).
// With Version=0 (default) it generates either IPv4 or IPv6.
func IPAddresses(opts IPAddressOptions) Generator {
	switch opts.Version {
	case IPVersion4:
		return &basicGenerator{schema: map[string]any{"type": "ipv4"}}
	case IPVersion6:
		return &basicGenerator{schema: map[string]any{"type": "ipv6"}}
	default:
		return OneOf(
			&basicGenerator{schema: map[string]any{"type": "ipv4"}},
			&basicGenerator{schema: map[string]any{"type": "ipv6"}},
		)
	}
}

// --- Tuple generators ---

// compositeTupleGenerator generates a tuple by generating each element
// separately inside a TUPLE span. Used when one or more elements are
// non-basic (cannot be represented as a single schema).
type compositeTupleGenerator struct {
	elements []Generator
}

// DoDraw produces a []any tuple by generating each element in sequence
// inside a TUPLE span.
func (g *compositeTupleGenerator) DoDraw(data *TestCase) any {
	result := make([]any, len(g.elements))
	group(labelTuple, func() {
		for i, elem := range g.elements {
			result[i] = elem.DoDraw(data)
		}
	}, data)
	return result
}

// AsBasic returns nil — compositeTupleGenerator is not a basic generator.
func (g *compositeTupleGenerator) AsBasic() *basicGenerator { return nil }

// Filter returns a filteredGenerator that only produces values satisfying pred.
func (g *compositeTupleGenerator) Filter(pred func(any) bool) Generator {
	return &filteredGenerator{source: g, predicate: pred}
}

// Map returns a new mappedGenerator that applies fn to each generated tuple.
func (g *compositeTupleGenerator) Map(fn func(any) any) Generator {
	return &mappedGenerator{inner: g, fn: fn}
}

// tupleBasic builds a basicGenerator for a tuple from a slice of BasicGenerators.
// If all transforms are nil, no transform is attached. Otherwise, a per-position
// transform is composed.
func tupleBasic(basics []*basicGenerator) *basicGenerator {
	schemas := make([]any, len(basics))
	for i, b := range basics {
		schemas[i] = b.schema
	}
	combined := map[string]any{
		"type":     "tuple",
		"elements": schemas,
	}
	// Check if any element has a transform.
	hasTransform := false
	for _, b := range basics {
		if b.transform != nil {
			hasTransform = true
			break
		}
	}
	if !hasTransform {
		return &basicGenerator{schema: combined}
	}
	// Capture transforms for per-position application.
	transforms := make([]func(any) any, len(basics))
	for i, b := range basics {
		transforms[i] = b.transform
	}
	applyTransforms := func(raw any) any {
		rawSlice, _ := raw.([]any)
		result := make([]any, len(rawSlice))
		for i, v := range rawSlice {
			if transforms[i] != nil {
				result[i] = transforms[i](v)
			} else {
				result[i] = v
			}
		}
		return result
	}
	return &basicGenerator{schema: combined, transform: applyTransforms}
}

// Tuples2 returns a Generator that produces 2-element tuples ([]any of length 2).
// If both elements are basic (schema-backed), a single generate command is sent.
// Otherwise, elements are generated separately inside a TUPLE span.
func Tuples2(g1, g2 Generator) Generator {
	b1 := g1.AsBasic()
	b2 := g2.AsBasic()
	if b1 != nil && b2 != nil {
		return tupleBasic([]*basicGenerator{b1, b2})
	}
	return &compositeTupleGenerator{elements: []Generator{g1, g2}}
}

// Tuples3 returns a Generator that produces 3-element tuples ([]any of length 3).
// If all elements are basic (schema-backed), a single generate command is sent.
// Otherwise, elements are generated separately inside a TUPLE span.
func Tuples3(g1, g2, g3 Generator) Generator {
	b1 := g1.AsBasic()
	b2 := g2.AsBasic()
	b3 := g3.AsBasic()
	if b1 != nil && b2 != nil && b3 != nil {
		return tupleBasic([]*basicGenerator{b1, b2, b3})
	}
	return &compositeTupleGenerator{elements: []Generator{g1, g2, g3}}
}

// --- flatMappedGenerator ---

// flatMappedGenerator is a generator for dependent generation.
// It generates a value from a source generator, passes it to f, and then
// generates from the generator returned by f. The whole operation is wrapped
// in a FLAT_MAP span.
type flatMappedGenerator struct {
	source Generator
	f      func(any) Generator
}

// FlatMap returns a new flatMappedGenerator that generates a value from g,
// passes it to f, and generates from the returned generator.
// Always non-basic — wrapped in a FLAT_MAP span (label 11).
func FlatMap(g Generator, f func(any) Generator) Generator {
	return &flatMappedGenerator{source: g, f: f}
}

// DoDraw generates a value from the source generator, passes it to f,
// and generates from the returned generator, all inside a FLAT_MAP span.
func (g *flatMappedGenerator) DoDraw(data *TestCase) any {
	var result any
	discardableGroup(labelFlatMap, func() {
		first := g.source.DoDraw(data)
		secondGen := g.f(first)
		result = secondGen.DoDraw(data)
	}, data)
	return result
}

// AsBasic returns nil — flatMappedGenerator is never a basic generator.
func (g *flatMappedGenerator) AsBasic() *basicGenerator { return nil }

// Filter returns a filteredGenerator that only produces values satisfying pred.
func (g *flatMappedGenerator) Filter(pred func(any) bool) Generator {
	return &filteredGenerator{source: g, predicate: pred}
}

// Map returns a new mappedGenerator that applies fn to each generated value.
func (g *flatMappedGenerator) Map(fn func(any) any) Generator {
	return &mappedGenerator{inner: g, fn: fn}
}

// Tuples4 returns a Generator that produces 4-element tuples ([]any of length 4).
// If all elements are basic (schema-backed), a single generate command is sent.
// Otherwise, elements are generated separately inside a TUPLE span.
func Tuples4(g1, g2, g3, g4 Generator) Generator {
	b1 := g1.AsBasic()
	b2 := g2.AsBasic()
	b3 := g3.AsBasic()
	b4 := g4.AsBasic()
	if b1 != nil && b2 != nil && b3 != nil && b4 != nil {
		return tupleBasic([]*basicGenerator{b1, b2, b3, b4})
	}
	return &compositeTupleGenerator{elements: []Generator{g1, g2, g3, g4}}
}
