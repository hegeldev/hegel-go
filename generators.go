package hegel

import "hegel.dev/go/hegel/internal/libhegel"

// --- Span label constants ---

// --- Generator interface ---

// Generator is the core abstraction for value generation in Hegel.
//
// It is a sealed interface — only types within this package can implement it.
type Generator[T any] interface {
	// draw produces a value from the Hegel engine using the given test case.
	// Unexported to seal the interface to this package.
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

	// Abort the current test case and update status.
	//
	// Passing nil for err is valid and aborts without changing state.
	abort(err error)

	// Recover from a previous call to abort.
	//
	// Must be a deferred call.
	recoverAbort()

	// Get the current status.
	getStatus() libhegel.Status

	// Set the current status.
	setStatus(status libhegel.Status)

	// generate requests a value from the engine for schema. The schema is a
	// JSON-shaped map (CBOR-encoded on the wire to libhegel). Returns the
	// decoded value or a sentinel error on abort.
	generate(schema map[string]any) (any, error)

	// startSpan begins a generation span. label is one of the [spanLabel]
	// constants; the engine uses labels for shrinking.
	startSpan(label libhegel.Label) error

	// stopSpan ends the current generation span. discard=true tells the
	// engine the entire span's choices should be reverted.
	stopSpan(discard bool) error

	// newCollectionCmd allocates a collection on the engine, returning its id.
	// maxSize=nil means unbounded.
	newCollection(minSize int, maxSize *int) (*collection, error)

	// isSingleTestCase reports whether this case is running under
	// [WithSingleTestCase].
	isSingleTestCase() bool

	// stateMachineNew registers an engine-owned state machine with the
	// named rules and invariants, returning its id. The engine owns rule
	// selection (including swarm testing).
	stateMachineNew(ruleNames, invariantNames []string) (libhegel.StateMachine, error)

	// stateMachineNextRule draws the index of the next rule to run, in
	// [0, len(rules)). Returns an [libhegel.E_STOP_TEST] error when the
	// engine's choice budget is exhausted.
	stateMachineNextRule(machine libhegel.StateMachine) (int64, error)

	// reportDraw emits one draw-report line for value through the
	// implementation's note channel, or no-ops when notes are suppressed.
	// skip is the number of stack frames to skip when resolving the source
	// position of the originating [Draw] call.
	reportDraw(skip int, value any)

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
		tc.abort(err)
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

// draw sends a generate command to the engine and returns the result.
func (g *basicGenerator[T]) draw(tc TestCase) (T, error) {
	v, err := tc.generate(g.schema)
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
	return withSpan(tc, libhegel.LABEL_MAPPED, func() (U, error) {
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
		if err := tc.startSpan(libhegel.LABEL_FILTER); err != nil {
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
	return zero, libhegel.E_ASSUME
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
	return withSpan(tc, libhegel.LABEL_FLAT_MAP, func() (U, error) {
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
// that aborts the test case, and the runner
// tears down engine state regardless.
//
// Use the inline (*testCase).startSpan/stopSpan pair when the discard
// decision depends on the body's outcome (see filteredGenerator.draw).
func withSpan[T any](tc TestCase, label libhegel.Label, body func() (T, error)) (T, error) {
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
