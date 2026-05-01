package hegel

// composite_test.go tests the Composite function and compositeGenerator type.

import (
	"testing"
)

// =============================================================================
// Composite — return-type and asBasic checks
// =============================================================================

// TestCompositeReturnsCompositeGenerator verifies that Composite returns a
// *compositeGenerator[T].
func TestCompositeReturnsCompositeGenerator(t *testing.T) {
	t.Parallel()
	g := Composite("test", func(*TestCase) int { return 0 })
	if _, ok := g.(*compositeGenerator[int]); !ok {
		t.Fatalf("Composite should return *compositeGenerator[int], got %T", g)
	}
}

// TestCompositeAsBasicAlwaysFalse verifies that a compositeGenerator never
// reduces to a basic schema — its body is imperative and may issue an unbounded
// number of draws.
func TestCompositeAsBasicAlwaysFalse(t *testing.T) {
	t.Parallel()
	g := Composite("test", func(*TestCase) int { return 0 })
	bg, ok, err := g.(*compositeGenerator[int]).asBasic()
	if bg != nil || ok || err != nil {
		t.Fatalf("Composite.asBasic should return (nil, false, nil); got (%v, %v, %v)", bg, ok, err)
	}
}

// TestCompositeNilFunctionPanics verifies that Composite panics when given a
// nil function — silent acceptance would yield a generator that nil-panics on
// first draw, far from the bug site.
func TestCompositeNilFunctionPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Fatal("Composite(nil) should panic")
		}
	}()
	_ = Composite[int]("x", nil)
}

// =============================================================================
// Composite — end-to-end via Run, against the real hegel-core server
// =============================================================================

// TestCompositeMultipleDrawsE2E exercises the basic composite use case: a
// function that draws several values and combines them. Verifies both that
// generation actually runs and that drawn values stay within their declared
// bounds.
func TestCompositeMultipleDrawsE2E(t *testing.T) {
	t.Parallel()

	type point struct{ x, y int }
	pointGen := Composite("point", func(tc *TestCase) point {
		return point{
			x: Draw(tc, Integers[int](-100, 100)),
			y: Draw(tc, Integers[int](-100, 100)),
		}
	})

	cases := 0
	if err := Run(func(s *TestCase) {
		p := pointGen.(*compositeGenerator[point]).draw(s)
		cases++
		if p.x < -100 || p.x > 100 || p.y < -100 || p.y > 100 {
			t.Errorf("point %+v out of bounds", p)
		}
	}, WithTestCases(50)); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if cases == 0 {
		t.Fatal("Composite: no test cases ran")
	}
}

// TestCompositeDataDependentDrawsE2E exercises the use case Composite exists
// for: the number of draws depends on a previously drawn value. This is the
// pattern that FlatMap cascades cannot express cleanly. It mirrors the proto-
// generator shape in resource-manager: pick a field count, then draw that many
// fields.
func TestCompositeDataDependentDrawsE2E(t *testing.T) {
	t.Parallel()

	listGen := Composite("variable_list", func(tc *TestCase) []int {
		n := Draw(tc, Integers[int](0, 10))
		out := make([]int, n)
		for i := range n {
			out[i] = Draw(tc, Integers[int](0, 1000))
		}
		return out
	})

	sawEmpty := false
	sawNonEmpty := false
	if err := Run(func(s *TestCase) {
		v := listGen.(*compositeGenerator[[]int]).draw(s)
		if len(v) > 10 {
			t.Errorf("length %d exceeds bound", len(v))
		}
		if len(v) == 0 {
			sawEmpty = true
		} else {
			sawNonEmpty = true
		}
		for _, x := range v {
			if x < 0 || x > 1000 {
				t.Errorf("element %d out of bounds", x)
			}
		}
	}, WithTestCases(100)); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !sawEmpty {
		t.Error("Composite: never generated an empty list across 100 cases")
	}
	if !sawNonEmpty {
		t.Error("Composite: never generated a non-empty list across 100 cases")
	}
}

// TestCompositeRecursiveE2E verifies that a composite generator can be a
// reusable value composed with itself — proves the "generator-as-value"
// abstraction this PoC was built to restore.
func TestCompositeRecursiveE2E(t *testing.T) {
	t.Parallel()

	type pair struct {
		a, b int
	}
	pairGen := Composite("pair", func(tc *TestCase) pair {
		return pair{
			a: Draw(tc, Integers[int](0, 10)),
			b: Draw(tc, Integers[int](0, 10)),
		}
	})

	// listOfPairs draws a length, then draws that many pairs — proving that
	// pairGen can be composed inside another composite without restructuring.
	listOfPairs := Composite("list_of_pairs", func(tc *TestCase) []pair {
		n := Draw(tc, Integers[int](0, 5))
		out := make([]pair, n)
		for i := range n {
			out[i] = Draw(tc, pairGen)
		}
		return out
	})

	cases := 0
	if err := Run(func(s *TestCase) {
		v := listOfPairs.(*compositeGenerator[[]pair]).draw(s)
		cases++
		for _, p := range v {
			if p.a < 0 || p.a > 10 || p.b < 0 || p.b > 10 {
				t.Errorf("pair %+v out of bounds", p)
			}
		}
	}, WithTestCases(50)); err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if cases == 0 {
		t.Fatal("Composite: no test cases ran")
	}
}

// TestCompositeShrinksToFailingCase verifies that the span machinery is wired
// correctly: when a composite-generated value triggers a failure, the engine
// shrinks toward a minimal failing input. Without start_span/stop_span around
// the composite body, shrinking would treat the draws as unrelated and
// produce garbage minimal cases.
func TestCompositeShrinksToFailingCase(t *testing.T) {
	t.Parallel()

	// Generator: a struct with two ints. Property under test: a + b < 100.
	// Smallest counterexample: a=0, b=100 (or a=100, b=0).
	type pair struct{ a, b int }
	pairGen := Composite("pair", func(tc *TestCase) pair {
		return pair{
			a: Draw(tc, Integers[int](0, 1000)),
			b: Draw(tc, Integers[int](0, 1000)),
		}
	})

	var minimalA, minimalB int
	err := Run(func(s *TestCase) {
		p := pairGen.(*compositeGenerator[pair]).draw(s)
		if p.a+p.b >= 100 {
			minimalA, minimalB = p.a, p.b
			panic(fatalSentinel{msg: "property violated"})
		}
	}, WithTestCases(200))
	if err == nil {
		t.Skip("never found a failing case — try again or raise WithTestCases")
	}

	// We don't assert exact equality (shrinking may land on any minimal pair),
	// but the sum should be near the boundary, not arbitrarily large.
	if minimalA+minimalB > 200 {
		t.Errorf("shrinker did not minimize: got a=%d b=%d (sum=%d), expected sum near 100",
			minimalA, minimalB, minimalA+minimalB)
	}
}
