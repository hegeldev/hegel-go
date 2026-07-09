package hegel

import (
	"bytes"
	"regexp"
	"strings"
	"testing"
)

// End-to-end tests for the engine's explain phase: after shrinking, the
// engine varies each part of the minimal counterexample, and parts whose
// value is irrelevant to the failure are annotated on the final replay's
// draw-report lines as `// or any other generated value`.

// The first draw is irrelevant (the failure only needs the second to be
// non-negative), so only it is annotated.
func TestExplainAnnotatesIrrelevantDraw(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run(func(tc TestCase) {
		_ = Draw(tc, Integers(-100, 100))
		b := Draw(tc, Integers(-100, 100))
		if b >= 0 {
			tc.FailNow()
		}
	}, WithTestCases(50), WithSeed(0), withOutput(&buf))
	if err == nil {
		t.Fatal("expected the property to fail")
	}
	captured := buf.String()
	lines := strings.Split(strings.TrimRight(captured, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected two draw lines, got:\n%s", captured)
	}
	if !strings.HasSuffix(lines[0], " := 0 // or any other generated value") {
		t.Fatalf("expected the first draw annotated, got:\n%s", captured)
	}
	if !strings.HasSuffix(lines[1], " := 0") {
		t.Fatalf("expected the second draw unannotated, got:\n%s", captured)
	}
}

// When every draw is irrelevant, each is annotated and the whole-test
// "varied together" note leads the report.
func TestExplainTogetherNote(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run(func(tc TestCase) {
		_ = Draw(tc, Booleans())
		_ = Draw(tc, Booleans())
		tc.FailNow()
	}, WithTestCases(50), WithSeed(0), withOutput(&buf))
	if err == nil {
		t.Fatal("expected the property to fail")
	}
	captured := buf.String()
	if strings.Count(captured, "// or any other generated value") != 2 {
		t.Fatalf("expected both draws annotated, got:\n%s", captured)
	}
	if !strings.HasPrefix(captured,
		"// The test always failed when commented parts were varied together.\n") {
		t.Fatalf("expected the leading together note, got:\n%s", captured)
	}
}

// Compositional printing gives explain annotations element-level
// granularity: only the list elements that don't matter to the failure are
// annotated, and the comments force the composite literal to break so each
// annotated element sits on its own line.
func TestExplainAnnotatesListElementsIndividually(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run(func(tc TestCase) {
		xs := Draw(tc, Lists(Integers(-100, 100)).MinSize(3).MaxSize(3))
		if xs[2] >= 0 {
			tc.FailNow()
		}
	}, WithTestCases(50), WithSeed(0), withOutput(&buf))
	if err == nil {
		t.Fatal("expected the property to fail")
	}
	captured := buf.String()
	want := regexp.MustCompile(
		`xs := \[\]int\{\n` +
			`    0, // or any other generated value\n` +
			`    0, // or any other generated value\n` +
			`    0,\n` +
			`\}`)
	if !want.MatchString(captured) {
		t.Fatalf("expected per-element annotations on the first two elements, got:\n%s", captured)
	}
}

// Disabling the explain phase suppresses the annotations.
func TestExplainDisabledByPhases(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := run(func(tc TestCase) {
		_ = Draw(tc, Integers(-100, 100))
		tc.FailNow()
	}, WithTestCases(50), WithSeed(0),
		WithPhases(PhaseExplicit, PhaseReuse, PhaseGenerate, PhaseTarget, PhaseShrink),
		withOutput(&buf))
	if err == nil {
		t.Fatal("expected the property to fail")
	}
	if strings.Contains(buf.String(), "// or any other generated value") {
		t.Fatalf("expected no annotations with the explain phase off, got:\n%s", buf.String())
	}
}

// An annotation whose choice slice matches no reported draw (here: a draw
// nested inside a Composite span reports only as part of the whole value) is
// dropped, and a together note referencing only invisible parts is dropped
// with it.
func TestExplainInvisibleAnnotationsDropped(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	gen := Composite(func(tc TestCase) [2]int {
		a := Draw(tc, Integers(-100, 100))
		b := Draw(tc, Integers(-100, 100))
		return [2]int{a, b}
	})
	err := run(func(tc TestCase) {
		pair := Draw(tc, gen)
		if pair[1] >= 0 {
			tc.FailNow()
		}
	}, WithTestCases(50), WithSeed(0), withOutput(&buf))
	if err == nil {
		t.Fatal("expected the property to fail")
	}
	captured := buf.String()
	// The composite's whole-value slice includes the relevant component, so
	// varying it can pass: the top-level draw is not annotated, and the
	// component annotation has no printed line to attach to.
	if strings.Contains(captured, "// The test") {
		t.Fatalf("expected no together note, got:\n%s", captured)
	}
	if strings.Contains(captured, "// or any other generated value") {
		t.Fatalf("expected no visible annotation, got:\n%s", captured)
	}
}

// The annotated report renders identically under the testing.T entry point,
// through t.Log.
func TestExplainAnnotationsUnderTestingT(t *testing.T) {
	t.Parallel()
	newTempGoProject(t).
		testBody(`_ = hegel.Draw(ht, hegel.Integers(-100, 100))
b := hegel.Draw(ht, hegel.Integers(-100, 100))
if b >= 0 {
	ht.Fatal("BOOM-explain")
}`, "hegel.WithTestCases(50)", "hegel.WithSeed(0)").
		expectFailure(`(?m)draw_1 := 0 // or any other generated value$`).
		goTest()
}
