package hegel

import (
	"fmt"
	"reflect"

	"hegel.dev/go/hegel/internal/libhegel"
)

// --- Generator interface ---

// Generator is the core abstraction for value generation in Hegel.
//
// It is a sealed interface — only types within this package can implement it.
type Generator[T any] interface {
	// draw produces a value from the Hegel engine using the given test case.
	// Unexported to seal the interface to this package.
	draw(tc TestCase) (T, error)
}

// printableGenerator is implemented by the structural generators (Lists,
// Maps, OneOf, Optional, Filter, FlatMap) that can print a value's
// representation while drawing it, emitting their delimiters around their
// component generators' own printing draws. That interleaving is what lets
// an explain-phase annotation attach to exactly the printed part it
// describes — a single slice element, one map entry.
//
// printDraw must consume exactly the choices (and spans) draw consumes: the
// engine explores with the silent path and replays failures with the
// printing path, so any divergence makes the failing replay flaky.
// Everything else prints by value: draw, then the %#v rendering.
type printableGenerator[T any] interface {
	printDraw(tc TestCase, rep *reporter) (T, error)
}

// drawAndPrint draws from g while printing its representation into rep, as
// one tracked explain-annotation region: if the choice slice the draw
// consumes carries an engine annotation, it is attached as a trailing
// comment. Compositional generators route their component draws back
// through here, so every nested printed region can carry its own annotation.
func drawAndPrint[T any](tc TestCase, g Generator[T], rep *reporter) (T, error) {
	start, tracking := tc.explainRegionStart()
	var v T
	var err error
	if pg, ok := g.(printableGenerator[T]); ok {
		v, err = pg.printDraw(tc, rep)
	} else {
		v, err = g.draw(tc)
		if err == nil {
			rep.text(fmt.Sprintf("%#v", v))
		}
	}
	if err != nil {
		return v, err
	}
	if tracking {
		if comment := tc.explainComment(start); comment != "" {
			rep.comment(" // " + comment)
		}
	}
	return v, nil
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

	// engine returns the libhegel context and test-case handle backing this
	// test case, so generators can drive the typed primitive draws directly.
	engine() (*libhegel.Context, *libhegel.TestCase)

	// startSpan begins a generation span. label is one of the [libhegel.Label]
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

	// reporter returns the test case's report writer, or nil when this case
	// does not emit (every exploratory case).
	reporter() *reporter

	// displayDrawName allocates the display name for a draw from its source
	// binding name ("" when there is no unambiguous one).
	displayDrawName(name string) string

	// explainRegionStart opens an explain-annotation region: the choice
	// index the enclosing draw starts at, and whether tracking is active
	// (only on the final replay of a failure the engine annotated).
	explainRegionStart() (uint64, bool)

	// explainComment closes an explain-annotation region: if the choice
	// slice consumed since start carries an annotation, it is returned
	// (consumed, so a slice reports at most once) — otherwise "".
	explainComment(start uint64) string

	// inSpan reports whether the test case is inside one or more
	// generation spans.
	inSpan() bool

	// beginPrintingDraw marks a top-level printing draw as in progress:
	// notes made during it buffer instead of landing mid-line.
	beginPrintingDraw()

	// endPrintingDraw clears the printing-draw mark and emits the buffered
	// notes after the draw's line.
	endPrintingDraw()
}

// Draw produces a value from a Generator using the given TestCase context.
//
// On an emitting test case (the final replay of a failure, or under
// [WithSingleTestCase]) a top-level draw prints into the case's report as a
// `file:line: name := value` line, where name is the binding receiving the
// draw.
// Nested draws (inside a span) are reported as part of the parent's value.
// The whole line sits in a speculative region, so a draw that unwinds — a
// failed assumption, an exhausted choice budget — retracts the partial line
// instead of corrupting the report.
func Draw[T any](tc TestCase, g Generator[T]) T {
	if h, ok := tc.(interface{ Helper() }); ok {
		h.Helper()
	}
	rep := tc.reporter()
	if rep == nil || tc.inSpan() {
		v, err := g.draw(tc)
		if err != nil {
			tc.abort(err)
		}
		return v
	}
	loc, srcName := drawCallSite(1)
	name := tc.displayDrawName(srcName)
	committed := false
	rep.beginSpeculative()
	tc.beginPrintingDraw()
	defer func() {
		if !committed {
			rep.abortSpeculative()
		}
		tc.endPrintingDraw()
	}()
	rep.text(fmt.Sprintf("%s: %s := ", loc, name))
	v, err := drawAndPrint(tc, g, rep)
	if err != nil {
		tc.abort(err)
	}
	rep.hardBreak()
	rep.commitSpeculative()
	committed = true
	return v
}

// typeName renders T the way %#v spells it in composite literals.
func typeName[T any]() string {
	return reflect.TypeFor[T]().String()
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

// --- filteredGenerator ---

// filteredGenerator wraps a source generator and a predicate, retrying up to
// maxFilterAttempts times before rejecting the test case.
type filteredGenerator[T any] struct {
	source    Generator[T]
	predicate func(T) bool
}

//lint:ignore U1000 used by filteredGenerator.draw, which is reached via Generator interface
const maxFilterAttempts = 3

// attemptLoop tries up to maxFilterAttempts times to produce a value
// satisfying predicate, running each attempt inside a FILTER span (discarded
// on rejection). attempt does the drawing — silently for draw, printing for
// printDraw — so both paths share the span discipline and consume identical
// choices.
func (g *filteredGenerator[T]) attemptLoop(tc TestCase, attempt func() (T, error)) (T, error) {
	var zero T
	for range maxFilterAttempts {
		if err := tc.startSpan(libhegel.LABEL_FILTER); err != nil {
			return zero, err
		}
		value, err := attempt()
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

// draw tries up to maxFilterAttempts times to produce a value satisfying predicate.
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
func (g *filteredGenerator[T]) draw(tc TestCase) (T, error) {
	return g.attemptLoop(tc, func() (T, error) {
		return g.source.draw(tc)
	})
}

// printDraw is the printing twin of draw: each attempt prints inside a
// speculative region, so only the accepted attempt's text survives. The last
// region's fate follows the loop's verdict — a nil error means its attempt
// was accepted, any error (a rejected budget, a failed draw) means its text
// must be retracted.
func (g *filteredGenerator[T]) printDraw(tc TestCase, rep *reporter) (T, error) {
	var opened bool
	value, err := g.attemptLoop(tc, func() (T, error) {
		if opened { // the previous attempt was rejected; retract its text
			rep.abortSpeculative()
		}
		opened = true
		rep.beginSpeculative()
		return drawAndPrint(tc, g.source, rep)
	})
	if opened {
		if err == nil {
			rep.commitSpeculative()
		} else {
			rep.abortSpeculative()
		}
	}
	return value, err
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

// printDraw is the printing twin of draw: the source draws silently (its
// value only parameterizes the dependent generator) and the dependent
// generator prints the produced value.
func (g *flatMappedGenerator[T, U]) printDraw(tc TestCase, rep *reporter) (U, error) {
	return withSpan(tc, libhegel.LABEL_FLAT_MAP, func() (U, error) {
		var zero U
		first, err := g.source.draw(tc)
		if err != nil {
			return zero, err
		}
		return drawAndPrint(tc, g.f(first), rep)
	})
}

// --- Free function combinators ---

// Map returns a new Generator that applies fn to each value from g.
func Map[T, U any](g Generator[T], fn func(T) U) Generator[U] {
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
