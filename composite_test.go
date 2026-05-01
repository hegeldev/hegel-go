package hegel

// composite_test.go tests the Composite function and compositeGenerator type.

import (
	"fmt"
	"testing"
)

// =============================================================================
// Composite function unit tests — verify return types
// =============================================================================

// TestCompositeReturnsCompositeGenerator verifies that Composite returns a
// *compositeGenerator.
func TestCompositeReturnsCompositeGenerator(t *testing.T) {
	t.Parallel()
	gen := Composite(func(s *TestCase) int {
		return Draw(s, Integers[int](0, 10))
	})
	if _, ok := gen.(*compositeGenerator[int]); !ok {
		t.Fatalf("Composite should return *compositeGenerator[int], got %T", gen)
	}
}

// TestCompositeGeneratorAsBasicReturnsNotBasic verifies that compositeGenerator
// is never basic — composite generators have no schema.
func TestCompositeGeneratorAsBasicReturnsNotBasic(t *testing.T) {
	t.Parallel()
	gen := Composite(func(s *TestCase) int {
		return Draw(s, Integers[int](0, 10))
	})
	bg, ok, err := gen.(*compositeGenerator[int]).asBasic()
	if err != nil {
		t.Fatalf("asBasic returned error: %v", err)
	}
	if ok {
		t.Fatal("compositeGenerator should never be basic")
	}
	if bg != nil {
		t.Fatal("compositeGenerator.asBasic should return nil bg")
	}
}

// TestCompositeGeneratorMapReturnsMappedGenerator verifies that Map on a
// composite generator returns a *mappedGenerator (the non-basic path).
func TestCompositeGeneratorMapReturnsMappedGenerator(t *testing.T) {
	t.Parallel()
	gen := Composite(func(s *TestCase) int {
		return Draw(s, Integers[int](0, 10))
	})
	mapped := Map(gen, func(v int) string { return fmt.Sprintf("%d", v) })
	if _, ok := mapped.(*mappedGenerator[int, string]); !ok {
		t.Fatalf("Map(compositeGenerator) should return *mappedGenerator, got %T", mapped)
	}
}

// =============================================================================
// compositeGenerator.draw tests using real hegel binary
// =============================================================================

// TestCompositeDrawsSubGenerators verifies that a Composite generator correctly
// draws values from its sub-generators and combines them.
func TestCompositeDrawsSubGenerators(t *testing.T) {
	t.Parallel()
	type point struct {
		x int
		y int
	}
	gen := Composite(func(s *TestCase) point {
		return point{
			x: Draw(s, Integers[int](0, 100)),
			y: Draw(s, Integers[int](-100, 0)),
		}
	})
	Test(t, func(ht *T) {
		p := Draw(ht, gen)
		if p.x < 0 || p.x > 100 {
			ht.Fatalf("x=%d out of range [0,100]", p.x)
		}
		if p.y < -100 || p.y > 0 {
			ht.Fatalf("y=%d out of range [-100,0]", p.y)
		}
	}, WithTestCases(30))
}

// TestCompositeNestedInLists verifies that a Composite generator works when
// used as the element generator for Lists — exercising the compositional draw
// path through the collection protocol.
func TestCompositeNestedInLists(t *testing.T) {
	t.Parallel()
	type pair struct {
		a int
		b int
	}
	pairGen := Composite(func(s *TestCase) pair {
		return pair{
			a: Draw(s, Integers[int](0, 10)),
			b: Draw(s, Integers[int](0, 10)),
		}
	})
	listGen := Lists(pairGen).MaxSize(5)
	Test(t, func(ht *T) {
		ps := Draw(ht, listGen)
		if len(ps) > 5 {
			ht.Fatalf("len=%d > 5", len(ps))
		}
		for _, p := range ps {
			if p.a < 0 || p.a > 10 || p.b < 0 || p.b > 10 {
				ht.Fatalf("pair out of range: %+v", p)
			}
		}
	}, WithTestCases(30))
}

// TestCompositeUsesAssume verifies that Assume works inside a composite
// generator body — exercising the same TestCase methods test bodies use.
func TestCompositeUsesAssume(t *testing.T) {
	t.Parallel()
	gen := Composite(func(s *TestCase) int {
		v := Draw(s, Integers[int](0, 100))
		s.Assume(v%2 == 0)
		return v
	})
	Test(t, func(ht *T) {
		v := Draw(ht, gen)
		if v%2 != 0 {
			ht.Fatalf("expected even, got %d", v)
		}
	}, WithTestCases(30))
}

// TestCompositeDataDependentDrawsE2E exercises the use case Composite exists
// for: the number of draws depends on a previously drawn value. This is the
// pattern that FlatMap cascades cannot express cleanly.
func TestCompositeDataDependentDrawsE2E(t *testing.T) {
	t.Parallel()

	listGen := Composite(func(tc *TestCase) []int {
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
	pairGen := Composite(func(tc *TestCase) pair {
		return pair{
			a: Draw(tc, Integers[int](0, 10)),
			b: Draw(tc, Integers[int](0, 10)),
		}
	})

	// listOfPairs draws a length, then draws that many pairs — proving that
	// pairGen can be composed inside another composite without restructuring.
	listOfPairs := Composite(func(tc *TestCase) []pair {
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

// TestCompositeShrinksToFailingCase verifies that when a composite-generated
// value triggers a failure, the engine shrinks toward a minimal failing input.
func TestCompositeShrinksToFailingCase(t *testing.T) {
	t.Parallel()

	// Generator: a struct with two ints. Property under test: a + b < 100.
	// Smallest counterexample: a=0, b=100 (or a=100, b=0).
	type pair struct{ a, b int }
	pairGen := Composite(func(tc *TestCase) pair {
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
		t.Fatal("never found a failing case")
	}

	// The shrinker minimizes left-to-right: it drives a to 0, then finds the
	// smallest b satisfying a+b >= 100, which is 100.
	if minimalA != 0 || minimalB != 100 {
		t.Errorf("shrinker did not minimize: got a=%d b=%d, want a=0 b=100",
			minimalA, minimalB)
	}
}
