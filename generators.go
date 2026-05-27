package hegel

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
	// labelStateful marks a single rule or invariant call inside a stateful
	// test, letting the shrinker treat each step as an atomic unit.
	labelStateful spanLabel = 16
	// labelComposite marks a composite generator's body, so draws inside
	// the body are bracketed in a span. This lets [Draw] suppress nested
	// draw reports the same way it does for element generators inside
	// Lists/Maps/OneOf.
	labelComposite spanLabel = 17
)

// --- Generator interface ---

// Generator is the core abstraction for value generation in Hegel.
//
// It is a sealed interface — only types within this package can implement it.
type Generator[T any] interface {
	// draw produces a value from the Hegel server using the given test case.
	// Unexported to seal the interface to this package.
	//
	// Returns the sentinel errors [*dataExhausted] / [flakyAbort] when the
	// server aborts the test case, [*connectionError] for transport
	// failures, and [assumeRejected] when a Filter exhausts its retries.
	draw(tc TestCase) (T, error)

	// asBasic returns the basic-schema form of this generator, when one exists.
	// The three return values encode three distinct states:
	//   (bg, true, nil)   — generator is basic; bg holds the schema and parser.
	//   (nil, false, nil) — generator is composite (e.g. filtered, flat-mapped,
	//                       or has non-basic element generators); no schema.
	//   (nil, false, err) — configuration is invalid (e.g. min > max).
	// Unexported to seal the interface to this package.
	asBasic() (*basicGenerator[T], bool, error)
}

// TestCase is the test context for a Hegel property test.
//
// It is passed to [Run] bodies and [Composite] callbacks directly, and is
// embedded in the *[T] passed to [Test] bodies, so generator code written
// against TestCase works under either entry point.
type TestCase interface {
	// Assume rejects the current test case if condition is false.
	Assume(condition bool)

	// Note prints message during the final (replay) test case or under
	// [WithSingleTestCase]. Output is routed to t.Log for [Test], or stdout
	// for [Run].
	Note(message string)

	// Target sends a target value to guide test generation.
	Target(value float64, label string)

	// Errorf logs the formatted message via Note and marks the test case as
	// failed. The test case continues running but is treated as a failure
	// after return.
	Errorf(format string, args ...any)

	// Fail marks the test case as failed without stopping it.
	Fail()

	// FailNow marks the test case as failed and stops the test body.
	FailNow()

	// Log routes the message through Note (only emitted on final replay or
	// under [WithSingleTestCase]).
	Log(args ...any)

	// doRequest sends a CBOR-encoded request to the server and returns the
	// decoded reply. Unexported to seal the interface to this package.
	doRequest(msg map[string]any) (any, error)

	// generateFromSchema sends a generate command for schema and returns the
	// decoded value. Unexported to seal the interface to this package.
	generateFromSchema(schema map[string]any) (any, error)

	// isSingleTestCase reports whether this case is running under
	// [WithSingleTestCase]. Unexported to seal the interface to this package.
	isSingleTestCase() bool

	// reportDraw emits one draw-report line for value through the
	// implementation's note channel, or no-ops when notes are suppressed.
	// skip is the number of stack frames to skip when resolving the source
	// position of the originating [Draw] call.
	reportDraw(skip int, value any)

	// startSpan notifies the server that a new generation span has started.
	startSpan(label spanLabel) error

	// stopSpan notifies the server that the current generation span has ended.
	stopSpan(discard bool) error

	// inSpan reports whether the test case is inside one or more
	// generation spans.
	inSpan() bool
}

// Draw produces a value from a Generator using the given TestCase context.
func Draw[T any](tc TestCase, g Generator[T]) T {
	// Mark this frame as a helper so t.Log file:line decoration walks past
	// it to the user's call site. testCase has no-op Helper; *T inherits
	// from *testing.T.
	if h, ok := tc.(interface{ Helper() }); ok {
		h.Helper()
	}
	v, err := g.draw(tc)
	if err != nil {
		panic(err)
	}
	// Nested draws are reported as part of the parent's value.
	if !tc.inSpan() {
		tc.reportDraw(1, v)
	}
	return v
}

// --- basicGenerator ---

// basicGenerator is a generator backed by a single JSON-schema sent to the
// Hegel server. The parse function converts the raw CBOR value to T.
type basicGenerator[T any] struct {
	schema map[string]any
	parse  func(any) T
}

// draw sends a generate command to the server and returns the result.
func (g *basicGenerator[T]) draw(tc TestCase) (T, error) {
	v, err := tc.generateFromSchema(g.schema)
	if err != nil {
		var zero T
		return zero, err
	}
	return g.parse(v), nil
}

// asBasic returns the receiver — a basicGenerator is trivially basic.
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
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
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
func (g *mappedGenerator[T, U]) draw(tc TestCase) (U, error) {
	return withSpan(tc, labelMapped, func() (U, error) {
		var zero U
		v, err := g.inner.draw(tc)
		if err != nil {
			return zero, err
		}
		return g.fn(v), nil
	})
}

// asBasic always returns not-basic. Map() composes basic-with-basic at
// construction time, so a mappedGenerator only exists when wrapping a
// non-basic source — collapsing it back through here would never match a
// caller's expectations.
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
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

//lint:ignore U1000 used by filteredGenerator.draw, which is reached via Generator interface
const maxFilterAttempts = 3

// draw tries up to maxFilterAttempts times to produce a value satisfying predicate.
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
func (g *filteredGenerator[T]) draw(tc TestCase) (T, error) {
	var zero T
	for range maxFilterAttempts {
		if err := tc.startSpan(labelFilter); err != nil {
			return zero, err
		}
		value, err := g.source.draw(tc)
		if err != nil {
			return zero, err
		}
		if g.predicate(value) {
			if err := tc.stopSpan(false); err != nil { // coverage-ignore
				return zero, err
			}
			return value, nil
		}
		if err := tc.stopSpan(true); err != nil { // coverage-ignore
			return zero, err
		}
	}
	return zero, assumeRejected{}
}

// asBasic always returns not-basic — filtering cannot be expressed as a schema.
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
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
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
func (g *flatMappedGenerator[T, U]) draw(tc TestCase) (U, error) {
	return withSpan(tc, labelFlatMap, func() (U, error) {
		var zero U
		first, err := g.source.draw(tc)
		if err != nil {
			return zero, err
		}
		return g.f(first).draw(tc)
	})
}

// asBasic always returns not-basic — flat-map's dependent generator is dynamic.
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
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

// withSpan brackets body in a span that always commits (discard=false).
//
// On body error the span is left open: every error path here is a sentinel
// that aborts the test case (dataExhausted, flakyAbort, connectionError,
// assumeRejected), and the runner tears down server-side state regardless.
//
// Use the inline (*testCase).startSpan/stopSpan pair when the discard
// decision depends on the body's outcome (see filteredGenerator.draw).
func withSpan[T any](tc TestCase, label spanLabel, body func() (T, error)) (T, error) {
	if h, ok := tc.(interface{ Helper() }); ok {
		h.Helper()
	}
	var zero T
	if err := tc.startSpan(label); err != nil {
		return zero, err
	}
	v, err := body()
	if err != nil {
		return zero, err
	}
	if err := tc.stopSpan(false); err != nil { // coverage-ignore
		return zero, err
	}
	return v, nil
}

// --- collection protocol ---

// collection manages a server-side collection (list/set/map) generation session.
//
// Errors from More and Reject are stashed on err. Callers iterate with
// `for coll.More(s) { ... }` and check `coll.Err()` once after the loop.
type collection struct {
	collectionID uint64
	finished     bool
	err          error
}

// Err returns the first error encountered by More or Reject, or nil.
func (c *collection) Err() error {
	return c.err
}

// newCollection starts a new collection on the server with the given size bounds.
// A nil maxSize means unbounded (omitted from the payload).
func newCollection(tc TestCase, minSize int, maxSize *int) (*collection, error) {
	msg := map[string]any{
		"command":  "new_collection",
		"min_size": int64(minSize),
	}
	if maxSize != nil {
		msg["max_size"] = int64(*maxSize)
	}
	v, err := tc.doRequest(msg)
	if err != nil {
		return nil, err
	}
	id, _ := v.(uint64)
	return &collection{collectionID: id}, nil
}

// More asks the server whether another element should be generated.
//
// Returns false once the collection is finished or an error has been
// recorded; check Err after the loop to distinguish those cases.
func (c *collection) More(tc TestCase) bool {
	if c.finished || c.err != nil {
		return false
	}
	v, err := tc.doRequest(map[string]any{
		"command":       "collection_more",
		"collection_id": c.collectionID,
	})
	if err != nil {
		c.err = err
		return false
	}
	more, _ := v.(bool)
	if !more {
		c.finished = true
	}
	return more
}

// Reject tells the server that the last generated element should not count.
//
// Errors are recorded on the collection and surfaced via Err.
func (c *collection) Reject(tc TestCase) {
	if c.finished || c.err != nil {
		return
	}
	if _, err := tc.doRequest(map[string]any{
		"command":       "collection_reject",
		"collection_id": c.collectionID,
	}); err != nil {
		c.err = err
	}
}
