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
