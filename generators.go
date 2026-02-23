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
