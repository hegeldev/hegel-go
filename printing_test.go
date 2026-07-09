package hegel

import (
	"bytes"
	"go/parser"
	"io"
	"regexp"
	"strings"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

// End-to-end tests for the printing twins: on an emitting test case each
// structural generator prints its value compositionally — element by
// element — so explain-phase annotations can attach to the parts of a value
// rather than only the whole.

// runPrinting runs body once (no shrinking or replay) and returns the
// rendered report.
func runPrinting(t *testing.T, body func(tc TestCase)) string {
	t.Helper()
	var buf bytes.Buffer
	if err := run(body, WithSingleTestCase(), withOutput(&buf)); err != nil {
		t.Fatalf("run: %v", err)
	}
	return buf.String()
}

func TestListPrintsCompositionally(t *testing.T) {
	t.Parallel()
	out := runPrinting(t, func(tc TestCase) {
		xs := Draw(tc, Lists(Integers(7, 7)).MinSize(3).MaxSize(3))
		_ = xs
	})
	if !strings.Contains(out, "xs := []int{7, 7, 7}") {
		t.Fatalf("expected a composite-literal list, got:\n%s", out)
	}
}

// A list too wide for one line breaks the way gofmt would: every element on
// its own line, a trailing comma, and the close brace at the outer
// indentation. The report must be valid Go — the trailing comma is
// mandatory in the broken form — so the draw line is required to parse.
func TestBrokenListPrintsInGofmtShape(t *testing.T) {
	t.Parallel()
	out := runPrinting(t, func(tc TestCase) {
		xs := Draw(tc, Lists(Integers(100, 100)).MinSize(20).MaxSize(20))
		_ = xs
	})
	if !strings.HasPrefix(out, "xs := []int{\n    100,\n") {
		t.Fatalf("expected the first element on its own line, got:\n%s", out)
	}
	if !strings.HasSuffix(out, "    100,\n}\n") {
		t.Fatalf("expected a trailing comma and the close brace on its own line, got:\n%s", out)
	}
	src := "package p\n\nfunc f() {\n" + out + "\t_ = xs\n}\n"
	if _, err := parser.ParseFile(newSourceCache().fset, "report.go", src, 0); err != nil {
		t.Fatalf("report is not valid Go: %v\nreport:\n%s", err, out)
	}
}

func TestEmptyListPrints(t *testing.T) {
	t.Parallel()
	out := runPrinting(t, func(tc TestCase) {
		xs := Draw(tc, Lists(Integers(0, 100)).MaxSize(0))
		_ = xs
	})
	if !strings.Contains(out, "xs := []int{}") {
		t.Fatalf("expected an empty composite literal, got:\n%s", out)
	}
}

// Map entries print in draw order, and an entry retracted for a duplicate
// key leaves no text behind. The key generator maps its first two draws to
// the same key, forcing exactly one duplicate rejection mid-print.
func TestMapPrintsInDrawOrderRetractingDuplicates(t *testing.T) {
	t.Parallel()
	calls := 0
	keys := Map(Integers(0, 100), func(int) int {
		calls++
		if calls <= 2 {
			return 7
		}
		return 3
	})
	out := runPrinting(t, func(tc TestCase) {
		m := Draw(tc, Maps(keys, Integers(9, 9)).MinSize(2).MaxSize(2))
		_ = m
	})
	if !strings.Contains(out, "m := map[int]int{7: 9, 3: 9}") {
		t.Fatalf("expected draw-order map entries with the duplicate retracted, got:\n%s", out)
	}
}

func TestOneOfPrintsChosenBranch(t *testing.T) {
	t.Parallel()
	out := runPrinting(t, func(tc TestCase) {
		n := Draw(tc, OneOf(Integers(3, 3), Integers(3, 3)))
		_ = n
	})
	if !strings.Contains(out, "n := 3") {
		t.Fatalf("expected the chosen branch's value, got:\n%s", out)
	}
}

// A present Optional prints as &value through the inner generator (%#v on a
// pointer would print its address); an absent one prints as nil. Failing
// properties pin each arm deterministically via the final replay.
func TestOptionalPrintsPresentAsAmpersand(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run(func(tc TestCase) {
		p := Draw(tc, Optional(Integers(0, 100)))
		if p != nil {
			tc.FailNow()
		}
	}, WithTestCases(50), WithSeed(0), withOutput(&buf))
	if err == nil {
		t.Fatal("expected the property to fail")
	}
	if !strings.Contains(buf.String(), "p := &0") {
		t.Fatalf("expected the present arm printed as &0, got:\n%s", buf.String())
	}
}

func TestOptionalPrintsAbsentAsNil(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run(func(tc TestCase) {
		p := Draw(tc, Optional(Integers(0, 100)))
		if p == nil {
			tc.FailNow()
		}
	}, WithTestCases(50), WithSeed(0), withOutput(&buf))
	if err == nil {
		t.Fatal("expected the property to fail")
	}
	if !strings.Contains(buf.String(), "p := nil") {
		t.Fatalf("expected the absent arm printed as nil, got:\n%s", buf.String())
	}
}

// Only the accepted filter attempt's text survives: the predicate rejects
// the first attempt, so its retracted text must not appear before the
// accepted value.
func TestFilterPrintsOnlyAcceptedAttempt(t *testing.T) {
	t.Parallel()
	calls := 0
	gen := Filter(Integers(9, 9), func(int) bool {
		calls++
		return calls > 1
	})
	out := runPrinting(t, func(tc TestCase) {
		n := Draw(tc, gen)
		_ = n
	})
	if !regexp.MustCompile(`(?m)^n := 9$`).MatchString(out) {
		t.Fatalf("expected only the accepted attempt's value, got:\n%s", out)
	}
}

// An exhausted filter rejects the test case; the partial draw line — the
// last attempt's text included — is retracted and nothing is reported.
func TestFilterExhaustionRetractsDrawLine(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := run(func(tc TestCase) {
		n := Draw(tc, Filter(Integers(9, 9), func(int) bool { return false }))
		_ = n
	}, WithSingleTestCase(), withOutput(&buf)); err != nil {
		t.Fatalf("run: %v", err)
	}
	if buf.String() != "" {
		t.Fatalf("expected an empty report for the rejected case, got:\n%s", buf.String())
	}
}

// FlatMap's source draws silently — its value only parameterizes the
// dependent generator — and the dependent value is what prints.
func TestFlatMapPrintsDependentValue(t *testing.T) {
	t.Parallel()
	out := runPrinting(t, func(tc TestCase) {
		n := Draw(tc, FlatMap(Integers(2, 2), func(v int) Generator[int] {
			return Integers(v*3, v*3)
		}))
		_ = n
	})
	if !regexp.MustCompile(`(?m)^n := 6$`).MatchString(out) {
		t.Fatalf("expected only the dependent value, got:\n%s", out)
	}
}

// Notes made in the middle of a printing draw — a Composite body calling
// Note — buffer and land after the draw's line instead of inside it.
func TestNoteInsideDrawBuffersUntilAfterTheLine(t *testing.T) {
	t.Parallel()
	gen := Composite(func(tc TestCase) int {
		tc.Note("mid-draw note")
		return Draw(tc, Integers(5, 5))
	})
	out := runPrinting(t, func(tc TestCase) {
		n := Draw(tc, gen)
		_ = n
	})
	if !regexp.MustCompile(`(?m)^n := 5\nmid-draw note$`).MatchString(out) {
		t.Fatalf("expected the note on its own line after the draw line, got:\n%s", out)
	}
}

// Lists / Maps printing twins validate configuration before touching the
// test case or the report, mirroring draw.
func TestListsPrintDrawInvalidConfigReturnsError(t *testing.T) {
	t.Parallel()
	_, err := Lists(Booleans()).MinSize(-1).printDraw(nil, nil)
	assertErrorContains(t, "min_size", err)
}

func TestMapsPrintDrawInvalidConfigReturnsError(t *testing.T) {
	t.Parallel()
	_, err := Maps(Integers(0, 1), Integers(0, 1)).MinSize(-1).printDraw(nil, nil)
	assertErrorContains(t, "min_size", err)
}

// --- engine / inner errors inside the printing twins ---
//
// These mirror the draw error-propagation tests: each twin is driven
// against a Stub scripted to fail at the relevant primitive (or against an
// invalidFloats inner generator, which errors before any engine call). Where
// the failure happens before the twin touches the report, rep is nil.

// stubReporter prepares a stub-backed test case for a printing draw: out is
// set so the case emits, and the reporter fetch consumes the scripted
// test_case_printer return.
func stubReporter(t *testing.T, opReturns ...any) (*testCase, *reporter) {
	t.Helper()
	tc := newStubTestCase(t, append([]any{uintptr(1), libhegel.OK}, opReturns...)...)
	tc.out = io.Discard
	return tc, tc.reporter()
}

func TestOneOfPrintDrawIndexError(t *testing.T) {
	t.Parallel()
	// start_span succeeds, then the branch-index integer draw fails.
	tc := newStubTestCase(t, libhegel.OK, int64(0), libhegel.E_BACKEND, "boom")
	if _, err := drawAndPrint(tc, OneOf(Booleans(), Booleans()), nil); err == nil {
		t.Fatal("expected one_of index draw error")
	}
}

func TestOptionalPrintDrawIndexError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, libhegel.OK, int64(0), libhegel.E_BACKEND, "boom")
	if _, err := drawAndPrint(tc, Optional(Booleans()), nil); err == nil {
		t.Fatal("expected optional index draw error")
	}
}

func TestOptionalPrintDrawInnerErrorPropagates(t *testing.T) {
	t.Parallel()
	// start_span(OPTIONAL), generate_integer=1 (draw the inner value),
	// printer_text("&"), then the inner draw fails in params().
	tc, rep := stubReporter(t,
		libhegel.OK,
		int64(1), libhegel.OK,
		libhegel.OK,
	)
	_, err := drawAndPrint(tc, Optional(invalidFloats()), rep)
	assertErrorContains(t, "allow_nan", err)
}

func TestListsPrintDrawCollectionError(t *testing.T) {
	t.Parallel()
	// start_span succeeds, then collection_new fails, before any printing.
	tc := newStubTestCase(t, libhegel.OK, libhegel.Collection(0), libhegel.E_BACKEND, "boom")
	if _, err := Lists(Booleans()).printDraw(tc, nil); err == nil {
		t.Fatal("expected collection allocation error")
	}
}

func TestListsPrintDrawElementErrorPropagates(t *testing.T) {
	t.Parallel()
	// start_span, collection_new, printer_begin_group, printer_shift_indent,
	// collection_more=true, the first element's leading breakable, then the
	// element draw fails in params().
	tc, rep := stubReporter(t,
		libhegel.OK,
		libhegel.Collection(0), libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		true, libhegel.OK,
		libhegel.OK,
	)
	_, err := Lists(invalidFloats()).printDraw(tc, rep)
	assertErrorContains(t, "allow_nan", err)
}

func TestListsPrintDrawMoreError(t *testing.T) {
	t.Parallel()
	// collection_more itself fails; the loop exits and coll.Err() surfaces it.
	tc, rep := stubReporter(t,
		libhegel.OK,
		libhegel.Collection(0), libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		false, libhegel.E_BACKEND, "boom",
	)
	if _, err := Lists(Booleans()).printDraw(tc, rep); err == nil {
		t.Fatal("expected collection_more error")
	}
}

func TestMapsPrintDrawCollectionError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, libhegel.OK, libhegel.Collection(0), libhegel.E_BACKEND, "boom")
	if _, err := Maps(Integers(0, 1), Integers(0, 1)).printDraw(tc, nil); err == nil {
		t.Fatal("expected collection allocation error")
	}
}

func TestMapsPrintDrawKeyErrorPropagates(t *testing.T) {
	t.Parallel()
	// start_span, collection_new, printer_begin_group, printer_shift_indent,
	// collection_more=true, printer_begin_speculative, the first entry's
	// leading breakable, the key draw fails in params(), and the entry's
	// region is retracted (printer_abort_speculative).
	tc, rep := stubReporter(t,
		libhegel.OK,
		libhegel.Collection(0), libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		true, libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		libhegel.OK,
	)
	_, err := Maps[float64, int](invalidFloats(), Integers(0, 1)).printDraw(tc, rep)
	assertErrorContains(t, "allow_nan", err)
}

func TestMapsPrintDrawValueErrorPropagates(t *testing.T) {
	t.Parallel()
	// As above, but the key draw succeeds (generate_integer plus its
	// printer_text) and printer_text(": ") precedes the failing value draw.
	tc, rep := stubReporter(t,
		libhegel.OK,
		libhegel.Collection(0), libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		true, libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		int64(0), libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		libhegel.OK,
	)
	_, err := Maps[int, float64](Integers(0, 1), invalidFloats()).printDraw(tc, rep)
	assertErrorContains(t, "allow_nan", err)
}

func TestMapsPrintDrawMoreError(t *testing.T) {
	t.Parallel()
	tc, rep := stubReporter(t,
		libhegel.OK,
		libhegel.Collection(0), libhegel.OK,
		libhegel.OK,
		libhegel.OK,
		false, libhegel.E_BACKEND, "boom",
	)
	if _, err := Maps(Integers(0, 1), Integers(0, 1)).printDraw(tc, rep); err == nil {
		t.Fatal("expected collection_more error")
	}
}

func TestFlatMapPrintDrawSourceErrorPropagates(t *testing.T) {
	t.Parallel()
	// The silent source draw fails in params() inside the FLAT_MAP span.
	tc := newStubTestCase(t, libhegel.OK)
	_, err := drawAndPrint(tc, FlatMap(invalidFloats(), func(float64) Generator[int] {
		return Integers(0, 1)
	}), nil)
	assertErrorContains(t, "allow_nan", err)
}
