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

// --- Generator interface ---

// Generator is the core abstraction for value generation in Hegel.
// Each call to Generate() produces a new value from the Hegel server.
type Generator interface {
	// Generate produces a value from the Hegel server.
	// Must be called from within a test body passed to RunHegelTest.
	Generate() any

	// AsBasic returns the underlying BasicGenerator if this generator is
	// a basic (schema-only) generator, or nil otherwise.
	// Used internally to optimise composed generators.
	AsBasic() *BasicGenerator

	// Map returns a new generator that applies fn to each generated value.
	Map(fn func(any) any) Generator
}

// --- BasicGenerator ---

// BasicGenerator is a generator backed by a single JSON-schema sent to the
// Hegel server. An optional transform function is applied to the raw value.
// Mapping a BasicGenerator preserves the schema optimisation.
type BasicGenerator struct {
	schema    map[string]any
	transform func(any) any // nil means identity
}

// Generate sends a generate command to the server and returns the result,
// applying any registered transform.
func (g *BasicGenerator) Generate() any {
	v, err := generateFromSchema(g.schema)
	if err != nil {
		panic(err)
	}
	if g.transform != nil {
		return g.transform(v)
	}
	return v
}

// AsBasic returns g itself — BasicGenerator is its own basic form.
func (g *BasicGenerator) AsBasic() *BasicGenerator { return g }

// Map returns a new BasicGenerator with the same schema and a composed
// transform function (preserving the single-generate-call optimisation).
func (g *BasicGenerator) Map(fn func(any) any) Generator {
	if g.transform != nil {
		prev := g.transform
		return &BasicGenerator{
			schema:    g.schema,
			transform: func(v any) any { return fn(prev(v)) },
		}
	}
	return &BasicGenerator{
		schema:    g.schema,
		transform: fn,
	}
}

// --- MappedGenerator ---

// MappedGenerator wraps a non-basic generator and applies a transform.
// It emits start_span / stop_span around the inner Generate call so the
// server can track the mapping for better shrinking.
type MappedGenerator struct {
	inner Generator
	fn    func(any) any
}

// Generate calls the inner generator inside a MAPPED span and applies fn.
func (g *MappedGenerator) Generate() any {
	var result any
	Group(LabelMapped, func() {
		result = g.fn(g.inner.Generate())
	})
	return result
}

// AsBasic returns nil — MappedGenerator is not a basic generator.
func (g *MappedGenerator) AsBasic() *BasicGenerator { return nil }

// Map returns a new MappedGenerator that composes fn with g's transform.
func (g *MappedGenerator) Map(fn func(any) any) Generator {
	return &MappedGenerator{
		inner: g,
		fn:    fn,
	}
}

// --- Span helpers ---

// StartSpan notifies the server that a new generation span has started.
// label identifies the kind of span (e.g. LabelList, LabelMapped).
// Must be called from within a test body. No-op if the test has been aborted.
func StartSpan(label SpanLabel) {
	s := getState()
	if s == nil || s.aborted {
		return
	}
	ch := s.channel
	payload, err := EncodeCBOR(map[string]any{
		"command": "start_span",
		"label":   int64(label),
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: StartSpan encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: StartSpan request: %v", err))
	}
	pending.Get() //nolint:errcheck
}

// StopSpan notifies the server that the current generation span has ended.
// If discard is true, the span's data should be discarded from the shrinking budget.
// No-op if the test has been aborted.
func StopSpan(discard bool) {
	s := getState()
	if s == nil || s.aborted {
		return
	}
	ch := s.channel
	payload, err := EncodeCBOR(map[string]any{
		"command": "stop_span",
		"discard": discard,
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: StopSpan encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: StopSpan request: %v", err))
	}
	pending.Get() //nolint:errcheck
}

// Group runs fn inside a start_span / stop_span pair with the given label.
// The span is never discarded (discard=false).
func Group(label SpanLabel, fn func()) {
	StartSpan(label)
	fn()
	StopSpan(false)
}

// DiscardableGroup runs fn inside a start_span / stop_span pair.
// If fn panics, the span is ended with discard=true before re-panicking.
func DiscardableGroup(label SpanLabel, fn func()) {
	StartSpan(label)
	panicked := true
	defer func() {
		StopSpan(panicked)
	}()
	fn()
	panicked = false
}

// --- Collection protocol ---

// Collection manages a server-side collection (list/set/map) generation session.
// Use NewCollection to create one, then call More in a loop, and optionally
// Reject to discard the last element.
type Collection struct {
	serverName string // assigned by server on new_collection
	finished   bool
}

// NewCollection starts a new collection on the server with the given size bounds.
// It sends the new_collection command immediately.
// Must be called from within a test body.
func NewCollection(minSize, maxSize int) *Collection {
	ch := getChannel()
	payload, err := EncodeCBOR(map[string]any{
		"command":  "new_collection",
		"min_size": int64(minSize),
		"max_size": int64(maxSize),
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: NewCollection encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: NewCollection request: %v", err))
	}
	v, err := pending.Get()
	if err != nil {
		re, ok := err.(*RequestError)
		if ok && re.ErrorType == "StopTest" {
			setAborted()
			panic(&dataExhausted{msg: "server ran out of data (new_collection)"})
		}
		panic(fmt.Sprintf("hegel: unreachable: new_collection error: %v", err))
	}
	name, _ := ExtractString(v)
	return &Collection{serverName: name}
}

// More asks the server whether another element should be generated.
// Returns false when the collection is exhausted; subsequent calls return false
// without sending any messages.
func (c *Collection) More() bool {
	if c.finished {
		return false
	}
	ch := getChannel()
	payload, err := EncodeCBOR(map[string]any{
		"command":    "collection_more",
		"collection": c.serverName,
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: Collection.More encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: More request: %v", err))
	}
	v, err := pending.Get()
	if err != nil {
		re, ok := err.(*RequestError)
		if ok && re.ErrorType == "StopTest" {
			setAborted()
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

// Reject tells the server that the last generated element should not count
// toward the collection's size budget (e.g. because it was filtered out).
// No-op if the collection has already finished.
func (c *Collection) Reject() {
	if c.finished {
		return
	}
	ch := getChannel()
	payload, err := EncodeCBOR(map[string]any{
		"command":    "collection_reject",
		"collection": c.serverName,
	})
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: Collection.Reject encode: %v", err))
	}
	pending, err := ch.Request(payload)
	if err != nil {
		panic(fmt.Sprintf("hegel: unreachable: Reject request: %v", err))
	}
	pending.Get() //nolint:errcheck
}

// --- Built-in generators ---

// Integers returns a Generator that produces integer values in [minVal, maxVal].
func Integers(minVal, maxVal int64) Generator {
	return &BasicGenerator{
		schema: map[string]any{
			"type":      "integer",
			"min_value": minVal,
			"max_value": maxVal,
		},
	}
}

// IntegersUnbounded returns a Generator that produces unbounded integer values.
func IntegersUnbounded() Generator {
	return &BasicGenerator{
		schema: map[string]any{
			"type": "integer",
		},
	}
}

// Emails returns a Generator that produces email address strings.
func Emails() Generator {
	return &BasicGenerator{
		schema: map[string]any{
			"type": "email",
		},
	}
}

// URLs returns a Generator that produces URL strings.
func URLs() Generator {
	return &BasicGenerator{
		schema: map[string]any{
			"type": "url",
		},
	}
}

// DomainOptions holds options for the Domains generator.
type DomainOptions struct {
	// MaxLength is the maximum length of the domain name.
	// Zero means no maximum length constraint.
	MaxLength int
}

// Domains returns a Generator that produces domain name strings.
// If opts.MaxLength > 0, generated domains will not exceed that length.
func Domains(opts DomainOptions) Generator {
	schema := map[string]any{
		"type": "domain",
	}
	if opts.MaxLength > 0 {
		schema["max_length"] = int64(opts.MaxLength)
	}
	return &BasicGenerator{schema: schema}
}

// Dates returns a Generator that produces ISO 8601 date strings (YYYY-MM-DD).
func Dates() Generator {
	return &BasicGenerator{
		schema: map[string]any{
			"type": "date",
		},
	}
}

// Times returns a Generator that produces time strings (HH:MM:SS or similar).
func Times() Generator {
	return &BasicGenerator{
		schema: map[string]any{
			"type": "time",
		},
	}
}

// Datetimes returns a Generator that produces ISO 8601 datetime strings.
func Datetimes() Generator {
	return &BasicGenerator{
		schema: map[string]any{
			"type": "datetime",
		},
	}
}

// Just returns a Generator that always produces the given constant value.
// The schema uses {"const": null} and the transform ignores the server result.
func Just(value any) *BasicGenerator {
	return &BasicGenerator{
		schema:    map[string]any{"const": nil},
		transform: func(_ any) any { return value },
	}
}

// SampledFrom returns a Generator that picks uniformly at random from values.
// The server generates an integer index in [0, len(values)-1], which is mapped
// to the corresponding element. Returns an error if values is empty.
func SampledFrom(values []any) (*BasicGenerator, error) {
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
	return &BasicGenerator{
		schema: schema,
		transform: func(v any) any {
			idx, _ := ExtractInt(v)
			return elements[idx]
		},
	}, nil
}

// MustSampledFrom returns a Generator that picks uniformly at random from values.
// Panics if values is empty.
func MustSampledFrom(values []any) *BasicGenerator {
	g, err := SampledFrom(values)
	if err != nil {
		panic(err)
	}
	return g
}

// FromRegex returns a Generator that produces strings matching the given regular expression.
// If fullmatch is true (the default), the entire string must match.
func FromRegex(pattern string, fullmatch bool) *BasicGenerator {
	return &BasicGenerator{
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
	return &BasicGenerator{schema: schema}
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
		"type":           "number",
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
	return &BasicGenerator{schema: schema}
}

// Booleans returns a Generator that produces boolean values with probability p
// of generating true. p must be in [0, 1]; 0.5 gives equal probability.
func Booleans(p float64) Generator {
	return &BasicGenerator{
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
	return &BasicGenerator{schema: schema}
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
	return &BasicGenerator{schema: schema}
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
// If elements is a BasicGenerator (schema-backed), the list is generated with a single
// server call using a list schema, and the element transform (if any) is applied to each
// item in the result. This is the fast path.
//
// If elements is a non-basic generator (e.g., filtered), the collection protocol is used:
// the server controls iteration via new_collection / collection_more, and each element is
// generated individually inside a LabelList span.
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
			return &BasicGenerator{schema: rawSchema, transform: listTransform}
		}
		return &BasicGenerator{schema: rawSchema}
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

// Generate produces a list by using the collection protocol inside a LabelList span.
func (g *compositeListGenerator) Generate() any {
	var result []any
	StartSpan(LabelList)
	panicked := true
	defer func() {
		StopSpan(panicked)
	}()
	coll := NewCollection(g.minSize, g.maxSize)
	for coll.More() {
		result = append(result, g.elements.Generate())
	}
	panicked = false
	return result
}

// AsBasic returns nil — compositeListGenerator is not a basic generator.
func (g *compositeListGenerator) AsBasic() *BasicGenerator { return nil }

// Map returns a new MappedGenerator wrapping g.
func (g *compositeListGenerator) Map(fn func(any) any) Generator {
	return &MappedGenerator{inner: g, fn: fn}
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
			return &BasicGenerator{
				schema: rawSchema,
				transform: func(v any) any {
					return pairsToMap(v, nil, nil)
				},
			}
		}
		return &BasicGenerator{
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

// Generate implements Generator by using the MAP span and collection protocol.
func (g *compositeDictGenerator) Generate() any {
	var result any
	DiscardableGroup(LabelMap, func() {
		maxSz := g.maxSize
		if !g.hasMax {
			maxSz = g.minSize + 10
		}
		coll := NewCollection(g.minSize, maxSz)
		m := map[any]any{}
		for coll.More() {
			Group(LabelMapEntry, func() {
				k := g.keys.Generate()
				v := g.values.Generate()
				m[k] = v
			})
		}
		result = m
	})
	return result
}

// AsBasic returns nil — compositeDictGenerator is not a basic generator.
func (g *compositeDictGenerator) AsBasic() *BasicGenerator { return nil }

// Map returns a new MappedGenerator wrapping this generator.
func (g *compositeDictGenerator) Map(fn func(any) any) Generator {
	return &MappedGenerator{inner: g, fn: fn}
}

// --- OneOf generator ---

// CompositeOneOfGenerator is a one_of generator for generators that cannot all
// be represented as BasicGenerators (e.g. filtered generators). It generates an
// integer index and delegates to the selected branch, wrapped in a ONE_OF span.
type CompositeOneOfGenerator struct {
	generators []Generator
}

// Generate picks one of the generators at random (via the Hegel server) and
// returns a value from that generator, wrapped in a ONE_OF span.
func (g *CompositeOneOfGenerator) Generate() any {
	var result any
	Group(LabelOneOf, func() {
		n := len(g.generators)
		idx, err := generateFromSchema(map[string]any{
			"type":      "integer",
			"min_value": int64(0),
			"max_value": int64(n - 1),
		})
		if err != nil {
			panic(fmt.Sprintf("hegel: unreachable: OneOf generateFromSchema: %v", err))
		}
		i, _ := ExtractInt(idx)
		result = g.generators[i].Generate()
	})
	return result
}

// AsBasic returns nil — CompositeOneOfGenerator is not a basic generator.
func (g *CompositeOneOfGenerator) AsBasic() *BasicGenerator { return nil }

// Map returns a new MappedGenerator that applies fn to each generated value.
func (g *CompositeOneOfGenerator) Map(fn func(any) any) Generator {
	return &MappedGenerator{inner: g, fn: fn}
}

// OneOf returns a Generator that produces values from one of the given generators.
//
// Path 1 — all basic with identity transforms: schema {"one_of": [s1, s2, ...]}
// Path 2 — all basic with some transforms: tagged-tuple schema, transform dispatches by tag
// Path 3 — any non-basic: CompositeOneOfGenerator using ONE_OF span
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
		return &CompositeOneOfGenerator{generators: gens}
	}

	// All are basic — collect them.
	basics := make([]*BasicGenerator, len(generators))
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
		return &BasicGenerator{
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

	return &BasicGenerator{
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
		return &BasicGenerator{schema: map[string]any{"type": "ipv4"}}
	case IPVersion6:
		return &BasicGenerator{schema: map[string]any{"type": "ipv6"}}
	default:
		return OneOf(
			&BasicGenerator{schema: map[string]any{"type": "ipv4"}},
			&BasicGenerator{schema: map[string]any{"type": "ipv6"}},
		)
	}
}
