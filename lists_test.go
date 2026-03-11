package hegel

// lists_test.go contains unit tests and e2e integration tests for the Lists generator.

import (
	"fmt"
	"testing"
)

// =============================================================================
// Lists generator unit tests
// =============================================================================

// TestListsBasicElementSchema verifies that Lists on a basicGenerator[int64] (no transform)
// produces a basicGenerator[[]int64] with the correct list schema.
func TestListsBasicElementSchema(t *testing.T) {
	elem := Integers[int64](0, 100)
	gen := Lists(elem, ListMinSize(2), ListMaxSize(10))
	bg, ok := gen.(*basicGenerator[[]int64])
	if !ok {
		t.Fatalf("Lists(basic) should return *basicGenerator[[]int64], got %T", gen)
	}
	if bg.schema["type"] != "list" {
		t.Errorf("schema type: expected 'list', got %v", bg.schema["type"])
	}
	elemSchema, ok := bg.schema["elements"].(map[string]any)
	if !ok {
		t.Fatalf("schema elements: expected map[string]any, got %T", bg.schema["elements"])
	}
	if elemSchema["type"] != "integer" {
		t.Errorf("elements type: expected 'integer', got %v", elemSchema["type"])
	}
	minV, _ := extractCBORInt(bg.schema["min_size"])
	if minV != 2 {
		t.Errorf("min_size: expected 2, got %d", minV)
	}
	maxV, _ := extractCBORInt(bg.schema["max_size"])
	if maxV != 10 {
		t.Errorf("max_size: expected 10, got %d", maxV)
	}
}

// TestListsBasicElementNoMaxSchema verifies that when MaxSize < 0, max_size is omitted.
func TestListsBasicElementNoMaxSchema(t *testing.T) {
	elem := Integers[int64](0, 100)
	gen := Lists(elem, ListMaxSize(-1))
	bg, ok := gen.(*basicGenerator[[]int64])
	if !ok {
		t.Fatalf("Lists(basic, no max) should return *basicGenerator[[]int64], got %T", gen)
	}
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when MaxSize < 0")
	}
}

// TestListsBasicElementWithTransformSchema verifies that Lists on a basicGenerator with
// a transform applies the transform element-wise in the list transform.
func TestListsBasicElementWithTransformSchema(t *testing.T) {
	// Integers[int64](0, 100) mapped to double: basicGenerator with transform.
	elem := Map(Integers[int64](0, 100), func(n int64) int64 {
		return n * 2
	})
	gen := Lists(elem, ListMaxSize(5))
	bg, ok := gen.(*basicGenerator[[]int64])
	if !ok {
		t.Fatalf("Lists(basic with transform) should return *basicGenerator[[]int64], got %T", gen)
	}
	if bg.schema["type"] != "list" {
		t.Errorf("schema type: expected 'list', got %v", bg.schema["type"])
	}
	if bg.transform == nil {
		t.Fatal("transform should not be nil for element with transform")
	}
	// Apply the transform to a raw []any and verify element-wise doubling.
	raw := []any{uint64(3), uint64(7), uint64(0)}
	result := bg.transform(raw)
	if len(result) != 3 {
		t.Fatalf("transform result length: expected 3, got %d", len(result))
	}
	for i, want := range []int64{6, 14, 0} {
		if result[i] != want {
			t.Errorf("transform result[%d]: expected %d, got %d", i, want, result[i])
		}
	}
}

// TestListsBasicElementWithTransformNonSlicePassthrough verifies that the list transform
// returns nil for non-slice values (defensive path in transform).
func TestListsBasicElementWithTransformNonSlicePassthrough(t *testing.T) {
	elem := Map(Integers[int64](0, 10), func(n int64) int64 { return n })
	gen := Lists(elem, ListMaxSize(5))
	bg, ok := gen.(*basicGenerator[[]int64])
	if !ok {
		t.Fatalf("expected *basicGenerator[[]int64], got %T", gen)
	}
	// Pass a non-slice value to the transform -- should return nil.
	result := bg.transform("not-a-slice")
	if result != nil {
		t.Errorf("non-slice passthrough: expected nil, got %v", result)
	}
}

// TestListsBasicElementNoTransformNonSlicePassthrough verifies that the list transform
// for a basic element with no transform returns nil for non-slice values.
func TestListsBasicElementNoTransformNonSlicePassthrough(t *testing.T) {
	elem := Booleans()
	gen := Lists(elem, ListMaxSize(5))
	bg, ok := gen.(*basicGenerator[[]bool])
	if !ok {
		t.Fatalf("expected *basicGenerator[[]bool], got %T", gen)
	}
	// Pass a non-slice value to the transform -- should return nil.
	result := bg.transform("not-a-slice")
	if result != nil {
		t.Errorf("non-slice passthrough: expected nil, got %v", result)
	}
}

// TestListsNonBasicElementReturnsComposite verifies that Lists on a non-basic generator
// returns a compositeListGenerator (not a basicGenerator).
func TestListsNonBasicElementReturnsComposite(t *testing.T) {
	// mappedGenerator is non-basic.
	inner := Integers[int64](0, 10)
	nonBasic := &mappedGenerator[int64, int64]{inner: inner, fn: func(v int64) int64 { return v }}
	gen := Lists(nonBasic, ListMinSize(1), ListMaxSize(3))
	if _, ok := gen.(*compositeListGenerator[int64]); !ok {
		t.Fatalf("Lists(non-basic) should return *compositeListGenerator[int64], got %T", gen)
	}
}

// TestListsNegativeMinSizeClampedToZero verifies that a negative MinSize is clamped to 0.
func TestListsNegativeMinSizeClampedToZero(t *testing.T) {
	elem := Integers[int64](0, 100)
	gen := Lists(elem, ListMinSize(-5), ListMaxSize(10))
	bg, ok := gen.(*basicGenerator[[]int64])
	if !ok {
		t.Fatalf("expected *basicGenerator[[]int64], got %T", gen)
	}
	minV, _ := extractCBORInt(bg.schema["min_size"])
	if minV != 0 {
		t.Errorf("negative MinSize should be clamped to 0, got %d", minV)
	}
}

// =============================================================================
// Lists e2e integration tests (real hegel binary)
// =============================================================================

// TestListsBasicIntegersE2E verifies that Lists(Integers[int](0,100)) always produces
// a list where every element is in [0, 100].
func TestListsBasicIntegersE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		xs := Lists(Integers[int](0, 100), ListMaxSize(10)).draw(s)
		for _, x := range xs {
			if x < 0 || x > 100 {
				panic(fmt.Sprintf("Lists: element %d out of range [0, 100]", x))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestListsWithSizeBoundsE2E verifies that Lists with min_size and max_size constraints
// always produces slices whose length is within the specified bounds.
func TestListsWithSizeBoundsE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		xs := Lists(Booleans(), ListMinSize(3), ListMaxSize(5)).draw(s)
		if len(xs) < 3 || len(xs) > 5 {
			panic(fmt.Sprintf("Lists: length %d out of [3, 5]", len(xs)))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestListsNonBasicElementE2E verifies that Lists with a non-basic element generator
// (mapped integers) always produces elements satisfying the mapped condition.
func TestListsNonBasicElementE2E(t *testing.T) {
	hegelBinPath(t)
	// Mapped generator: integers in [0,100] then round to nearest even.
	mapped := Map(Integers[int](0, 100), func(n int) int {
		return (n / 2) * 2
	})
	nonBasic := &mappedGenerator[int, int]{inner: mapped, fn: func(v int) int { return v }}

	if _err := runHegel(func(s *TestCase) {
		xs := Lists(nonBasic, ListMaxSize(5)).draw(s)
		for _, x := range xs {
			if x%2 != 0 {
				panic(fmt.Sprintf("Lists(non-basic): expected even element, got %d", x))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestListsNestedE2E verifies that nested lists work correctly:
// Lists(Lists(Booleans)) produces a list of lists of booleans.
func TestListsNestedE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		outer := Lists(Lists(Booleans(), ListMaxSize(3)), ListMaxSize(3)).draw(s)
		for i, inner := range outer {
			for j, b := range inner {
				// b is already bool due to typed generators; verify it is true or false.
				if b != true && b != false {
					panic(fmt.Sprintf("nested Lists[%d][%d]: expected bool, got %v", i, j, b))
				}
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestListsBasicWithTransformE2E verifies that Lists on a basicGenerator with a transform
// applies the transform element-wise to the result.
func TestListsBasicWithTransformE2E(t *testing.T) {
	hegelBinPath(t)
	// Map Integers[int](0,10) -> double. Lists wraps this into a list schema with element transform.
	doubled := Map(Integers[int](0, 10), func(n int) int {
		return n * 2
	})
	if _err := runHegel(func(s *TestCase) {
		xs := Lists(doubled, ListMaxSize(5)).draw(s)
		for _, x := range xs {
			if x%2 != 0 || x < 0 || x > 20 {
				panic(fmt.Sprintf("Lists(basic+transform): element %d should be even in [0,20]", x))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}
