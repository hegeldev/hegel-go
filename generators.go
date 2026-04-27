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
//
// It is a sealed interface — only types within this package can implement it.
type Generator[T any] interface {
	// draw produces a value from the Hegel server using the given state.
	// Unexported to seal the interface to this package.
	draw(s *TestCase) T

	// asBasic returns the basic-schema form of this generator, when one exists.
	// The three return values encode three distinct states:
	//   (bg, true, nil)   — generator is basic; bg holds the schema and parser.
	//   (nil, false, nil) — generator is composite (e.g. filtered, flat-mapped,
	//                       or has non-basic element generators); no schema.
	//   (nil, false, err) — configuration is invalid (e.g. min > max).
	// Unexported to seal the interface to this package.
	asBasic() (*basicGenerator[T], bool, error)
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
// Hegel server. The parse function converts the raw CBOR value to T.
type basicGenerator[T any] struct {
	schema map[string]any
	parse  func(any) T
}

// draw sends a generate command to the server and returns the result.
func (g *basicGenerator[T]) draw(s *TestCase) T {
	v, err := generateFromSchema(s, g.schema)
	if err != nil {
		panic(err)
	}
	return g.parse(v)
}

// asBasic returns the receiver — a basicGenerator is trivially basic.
func (g *basicGenerator[T]) asBasic() (*basicGenerator[T], bool, error) {
	return g, true, nil
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
	startSpan(s, labelMapped)
	result := g.fn(g.inner.draw(s))
	stopSpan(s, false)
	return result
}

// asBasic always returns not-basic. Map() composes basic-with-basic at
// construction time, so a mappedGenerator only exists when wrapping a
// non-basic source — collapsing it back through here would never match a
// caller's expectations.
func (g *mappedGenerator[T, U]) asBasic() (*basicGenerator[U], bool, error) {
	return nil, false, nil
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

// asBasic always returns not-basic — filtering cannot be expressed as a schema.
func (g *filteredGenerator[T]) asBasic() (*basicGenerator[T], bool, error) {
	return nil, false, nil
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
	startSpan(s, labelFlatMap)
	first := g.source.draw(s)
	secondGen := g.f(first)
	result := secondGen.draw(s)
	stopSpan(s, false)
	return result
}

// asBasic always returns not-basic — flat-map's dependent generator is dynamic.
func (g *flatMappedGenerator[T, U]) asBasic() (*basicGenerator[U], bool, error) {
	return nil, false, nil
}

// --- Free function combinators ---

// Map returns a new Generator that applies fn to each value from g.
func Map[T, U any](g Generator[T], fn func(T) U) Generator[U] {
	bg, ok, err := g.asBasic()
	if err != nil {
		panic(err.Error())
	}
	if ok {
		prev := bg.parse
		return &basicGenerator[U]{
			schema: bg.schema,
			parse:  func(v any) U { return fn(prev(v)) },
		}
	}
	return &mappedGenerator[T, U]{inner: g, fn: fn}
}

// FlatMap returns a Generator that generates a value from g, passes it to f,
// and generates from the returned Generator.
func FlatMap[T, U any](g Generator[T], f func(T) Generator[U]) Generator[U] {
	return &flatMappedGenerator[T, U]{source: g, f: f}
}

// Filter returns a Generator that only produces values from g that satisfy pred.
//
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
	st := gs.stream
	payload, err := encodeCBOR(map[string]any{
		"command": "start_span",
		"label":   int64(label),
	})
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("startSpan encode: %v", err))
	}
	pending, err := st.Request(payload)
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("startSpan request: %v", err))
	}
	pending.Get() //nolint:errcheck
}

// stopSpan notifies the server that the current generation span has ended.
func stopSpan(gs *TestCase, discard bool) {
	if gs == nil || gs.aborted {
		return
	}
	st := gs.stream
	payload, err := encodeCBOR(map[string]any{
		"command": "stop_span",
		"discard": discard,
	})
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("stopSpan encode: %v", err))
	}
	pending, err := st.Request(payload)
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("stopSpan request: %v", err))
	}
	pending.Get() //nolint:errcheck
}

// --- collection protocol ---

// collection manages a server-side collection (list/set/map) generation session.
type collection struct {
	collectionID uint64
	finished     bool
}

// newCollection starts a new collection on the server with the given size bounds.
func newCollection(gs *TestCase, minSize, maxSize int) *collection {
	st := gs.stream
	payload, err := encodeCBOR(map[string]any{
		"command":  "new_collection",
		"min_size": int64(minSize),
		"max_size": int64(maxSize),
	})
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("newCollection encode: %v", err))
	}
	pending, err := st.Request(payload)
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("newCollection request: %v", err))
	}
	v, err := pending.Get()
	if err != nil {
		re, ok := err.(*requestError)
		if ok && re.ErrorType == "StopTest" {
			gs.aborted = true
			panic(&dataExhausted{msg: "server ran out of data (new_collection)"})
		}
		panic(fmt.Sprintf("new_collection error: %v", err)) // coverage-ignore
	}
	id, _ := v.(uint64)
	return &collection{collectionID: id}
}

// More asks the server whether another element should be generated.
func (c *collection) More(gs *TestCase) bool {
	if c.finished { // coverage-ignore
		return false
	}
	st := gs.stream
	payload, err := encodeCBOR(map[string]any{
		"command":       "collection_more",
		"collection_id": c.collectionID,
	})
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("collection.More encode: %v", err))
	}
	pending, err := st.Request(payload)
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("More request: %v", err))
	}
	v, err := pending.Get()
	if err != nil {
		re, ok := err.(*requestError)
		if ok && re.ErrorType == "StopTest" {
			gs.aborted = true
			panic(&dataExhausted{msg: "server ran out of data (collection_more)"})
		}
		panic(fmt.Sprintf("collection_more error: %v", err)) // coverage-ignore
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
	st := gs.stream
	payload, err := encodeCBOR(map[string]any{
		"command":       "collection_reject",
		"collection_id": c.collectionID,
	})
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("collection.Reject encode: %v", err))
	}
	pending, err := st.Request(payload)
	if err != nil { // coverage-ignore
		panic(fmt.Sprintf("Reject request: %v", err))
	}
	pending.Get() //nolint:errcheck
}
