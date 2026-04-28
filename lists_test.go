package hegel

// lists_test.go contains unit tests and e2e integration tests for the Lists generator.

import (
	"fmt"
	"testing"
)

// =============================================================================
// Lists generator unit tests
// =============================================================================

// TestListsBasicElementSchema verifies that Lists on a basic generator produces
// a list schema with the expected fields.
func TestListsBasicElementSchema(t *testing.T) {
	t.Parallel()
	elem := Integers[int64](0, 100)
	gen := Lists(elem).MinSize(2).MaxSize(10)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Lists(Integers) should be basic")
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
	t.Parallel()
	elem := Integers[int64](0, 100)
	gen := Lists(elem)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Lists(Integers) should be basic")
	}
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when MaxSize < 0")
	}
}

// TestListsBasicElementWithParseSchema verifies that Lists on a basicGenerator with
// a parse function applies it element-wise in the list parse.
func TestListsBasicElementWithParseSchema(t *testing.T) {
	t.Parallel()
	// Integers[int64](0, 100) mapped to double: basicGenerator with composed parse.
	elem := Map(Integers[int64](0, 100), func(n int64) int64 {
		return n * 2
	})
	gen := Lists(elem).MaxSize(5)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Lists(Map(Integers)) should be basic")
	}
	if bg.schema["type"] != "list" {
		t.Errorf("schema type: expected 'list', got %v", bg.schema["type"])
	}
	// Apply the parse to a raw []any and verify element-wise doubling.
	raw := []any{uint64(3), uint64(7), uint64(0)}
	result := bg.parse(raw)
	if len(result) != 3 {
		t.Fatalf("parse result length: expected 3, got %d", len(result))
	}
	for i, want := range []int64{6, 14, 0} {
		if result[i] != want {
			t.Errorf("parse result[%d]: expected %d, got %d", i, want, result[i])
		}
	}
}

// TestListsBasicElementParseNonSlicePassthrough verifies that the list parse
// returns nil for non-slice values (defensive path in parse).
func TestListsBasicElementParseNonSlicePassthrough(t *testing.T) {
	t.Parallel()
	elem := Map(Integers[int64](0, 10), func(n int64) int64 { return n })
	gen := Lists(elem).MaxSize(5)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Lists should be basic")
	}
	// Pass a non-slice value to the parse -- should return nil.
	result := bg.parse("not-a-slice")
	if result != nil {
		t.Errorf("non-slice passthrough: expected nil, got %v", result)
	}
}

// TestListsBasicElementNoParseNonSlicePassthrough verifies that the list parse
// for a basic element returns nil for non-slice values.
func TestListsBasicElementNoParseNonSlicePassthrough(t *testing.T) {
	t.Parallel()
	elem := Booleans()
	gen := Lists(elem).MaxSize(5)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Lists(Booleans) should be basic")
	}
	// Pass a non-slice value to the parse -- should return nil.
	result := bg.parse("not-a-slice")
	if result != nil {
		t.Errorf("non-slice passthrough: expected nil, got %v", result)
	}
}

// TestListsNonBasicElementIsNotBasic verifies that Lists on a non-basic generator
// returns ok=false from asBasic.
func TestListsNonBasicElementIsNotBasic(t *testing.T) {
	t.Parallel()
	// mappedGenerator is non-basic.
	inner := Integers[int64](0, 10)
	nonBasic := &mappedGenerator[int64, int64]{inner: inner, fn: func(v int64) int64 { return v }}
	gen := Lists(nonBasic).MinSize(1).MaxSize(3)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if ok || bg != nil {
		t.Fatal("Lists(non-basic) should not be basic")
	}
}

// TestListsNegativeMinSizeError verifies that a negative MinSize is rejected.
func TestListsNegativeMinSizeError(t *testing.T) {
	t.Parallel()
	_, _, err := Lists(Integers[int64](0, 100)).MinSize(-5).MaxSize(10).asBasic()
	assertErrorContains(t, "min_size", err)
}

// =============================================================================
// Lists e2e integration tests (real hegel binary)
// =============================================================================

// TestListsBasicIntegersE2E verifies that Lists(Integers[int](0,100)) always produces
// a list where every element is in [0, 100].
func TestListsBasicIntegersE2E(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		xs := Lists(Integers[int](0, 100)).MaxSize(10).draw(s)
		for _, x := range xs {
			if x < 0 || x > 100 {
				panic(fmt.Sprintf("Lists: element %d out of range [0, 100]", x))
			}
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestListsWithSizeBoundsE2E verifies that Lists with min_size and max_size constraints
// always produces slices whose length is within the specified bounds.
func TestListsWithSizeBoundsE2E(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		xs := Lists(Booleans()).MinSize(3).MaxSize(5).draw(s)
		if len(xs) < 3 || len(xs) > 5 {
			panic(fmt.Sprintf("Lists: length %d out of [3, 5]", len(xs)))
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestListsNonBasicElementE2E verifies that Lists with a non-basic element generator
// (mapped integers) always produces elements satisfying the mapped condition.
func TestListsNonBasicElementE2E(t *testing.T) {
	t.Parallel()

	// Mapped generator: integers in [0,100] then round to nearest even.
	mapped := Map(Integers[int](0, 100), func(n int) int {
		return (n / 2) * 2
	})
	nonBasic := &mappedGenerator[int, int]{inner: mapped, fn: func(v int) int { return v }}

	if _err := Run(func(s *TestCase) {
		xs := Lists(nonBasic).MaxSize(5).draw(s)
		for _, x := range xs {
			if x%2 != 0 {
				panic(fmt.Sprintf("Lists(non-basic): expected even element, got %d", x))
			}
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestListsNestedE2E verifies that nested lists work correctly:
// Lists(Lists(Booleans)) produces a list of lists of booleans.
func TestListsNestedE2E(t *testing.T) {
	t.Parallel()

	if _err := Run(func(s *TestCase) {
		outer := Lists(Lists(Booleans()).MaxSize(3)).MaxSize(3).draw(s)
		for i, inner := range outer {
			for j, b := range inner {
				// b is already bool due to typed generators; verify it is true or false.
				if b != true && b != false {
					panic(fmt.Sprintf("nested Lists[%d][%d]: expected bool, got %v", i, j, b))
				}
			}
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestListsBasicWithParseE2E verifies that Lists on a basicGenerator with a composed
// parse applies it element-wise to the result.
func TestListsBasicWithParseE2E(t *testing.T) {
	t.Parallel()

	// Map Integers[int](0,10) -> double. Lists wraps this into a list schema with element parse.
	doubled := Map(Integers[int](0, 10), func(n int) int {
		return n * 2
	})
	if _err := Run(func(s *TestCase) {
		xs := Lists(doubled).MaxSize(5).draw(s)
		for _, x := range xs {
			if x%2 != 0 || x < 0 || x > 20 {
				panic(fmt.Sprintf("Lists(basic+parse): element %d should be even in [0,20]", x))
			}
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestListsUniqueBasicSchema verifies that Lists(...).Unique(true) with a basic
// element generator sets "unique": true in the list schema.
func TestListsUniqueBasicSchema(t *testing.T) {
	t.Parallel()
	elem := Integers[int64](0, 100)
	gen := Lists(elem).MinSize(2).MaxSize(10).Unique(true)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Lists(Integers).Unique should be basic")
	}
	u, ok := bg.schema["unique"].(bool)
	if !ok || !u {
		t.Errorf("schema unique: expected true, got %v (%T)", bg.schema["unique"], bg.schema["unique"])
	}
}

// TestListsUniqueBasicWithParseSchema verifies that Unique propagates to the
// list schema even when the element generator has a composed parse.
func TestListsUniqueBasicWithParseSchema(t *testing.T) {
	t.Parallel()
	elem := Map(Integers[int64](0, 100), func(n int64) int64 { return n * 2 })
	gen := Lists(elem).MinSize(1).MaxSize(5).Unique(true)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Lists(Map(Integers)).Unique should be basic")
	}
	u, ok := bg.schema["unique"].(bool)
	if !ok || !u {
		t.Errorf("schema unique: expected true, got %v (%T)", bg.schema["unique"], bg.schema["unique"])
	}
}

// TestListsUniqueDefaultFalseSchema verifies that Unique is not set in the schema
// when the builder was not configured with Unique(true).
func TestListsUniqueDefaultFalseSchema(t *testing.T) {
	t.Parallel()
	gen := Lists(Integers[int64](0, 100)).MinSize(0).MaxSize(5)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Lists(Integers) should be basic")
	}
	if _, has := bg.schema["unique"]; has {
		t.Error("unique should not be present in schema when Unique not set")
	}
}

// TestListsUniqueNonBasicE2E verifies that Lists with Unique(true) and a non-basic
// element generator produces distinct elements by rejecting duplicates through
// the collection protocol.
func TestListsUniqueNonBasicE2E(t *testing.T) {
	t.Parallel()

	inner := Integers[int](0, 20)
	nonBasic := &mappedGenerator[int, int]{inner: inner, fn: func(v int) int { return v }}
	if _err := Run(func(s *TestCase) {
		xs := Lists(nonBasic).MinSize(3).MaxSize(10).Unique(true).draw(s)
		seen := map[int]struct{}{}
		for _, x := range xs {
			if _, exists := seen[x]; exists {
				panic(fmt.Sprintf("Lists(non-basic).Unique: duplicate element %d in %v", x, xs))
			}
			seen[x] = struct{}{}
		}
	}, WithTestCases(20)); _err != nil {
		panic(_err)
	}
}

// TestListsCompositeEmptyReturnsEmptySlice verifies that an empty composite list
// yields an empty (non-nil) slice so CBOR encoding produces `[]` rather than `null`.
func TestListsCompositeEmptyReturnsEmptySlice(t *testing.T) {
	t.Parallel()

	inner := Integers[int](0, 10)
	nonBasic := &mappedGenerator[int, int]{inner: inner, fn: func(v int) int { return v }}
	if _err := Run(func(s *TestCase) {
		xs := Lists(nonBasic).MaxSize(0).draw(s)
		if xs == nil {
			panic("Lists(non-basic) with MaxSize(0) returned nil; expected empty slice")
		}
		if len(xs) != 0 {
			panic(fmt.Sprintf("expected empty slice, got %v", xs))
		}
	}, WithTestCases(5)); _err != nil {
		panic(_err)
	}
}
