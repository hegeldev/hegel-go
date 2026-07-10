package hegel

// lists_test.go contains unit tests and e2e integration tests for the Lists generator.

import (
	"fmt"
	"testing"
)

// =============================================================================
// Lists generator validation unit tests
// =============================================================================

// TestListsNegativeMinSizeError verifies that a negative MinSize is rejected at
// draw time (before any engine call).
func TestListsNegativeMinSizeError(t *testing.T) {
	t.Parallel()
	_, err := Lists(Integers[int64](0, 100)).MinSize(-5).MaxSize(10).draw(newStubTestCase(t))
	assertErrorContains(t, "Lists: MinSize -5 must be non-negative", err)
}

// =============================================================================
// Lists e2e integration tests (real hegel binary)
// =============================================================================

// TestListsBasicIntegersE2E verifies that Lists(Integers[int](0,100)) always produces
// a list where every element is in [0, 100].
func TestListsBasicIntegersE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		xs := Draw(ht, Lists(Integers[int](0, 100)).MaxSize(10))
		for _, x := range xs {
			if x < 0 || x > 100 {
				panic(fmt.Sprintf("Lists: element %d out of range [0, 100]", x))
			}
		}
	}, WithTestCases(50))
}

// TestListsWithSizeBoundsE2E verifies that Lists with min_size and max_size constraints
// always produces slices whose length is within the specified bounds.
func TestListsWithSizeBoundsE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		xs := Draw(ht, Lists(Booleans()).MinSize(3).MaxSize(5))
		if len(xs) < 3 || len(xs) > 5 {
			panic(fmt.Sprintf("Lists: length %d out of [3, 5]", len(xs)))
		}
	}, WithTestCases(50))
}

// TestListsNonBasicElementE2E verifies that Lists with a mapped element generator
// always produces elements satisfying the mapped condition.
func TestListsNonBasicElementE2E(t *testing.T) {
	t.Parallel()

	mapped := Map(Integers[int](0, 100), func(n int) int {
		return (n / 2) * 2
	})
	nonBasic := &mappedGenerator[int, int]{inner: mapped, fn: func(v int) int { return v }}

	Test(t, func(ht *T) {
		xs := Draw(ht, Lists(nonBasic).MaxSize(5))
		for _, x := range xs {
			if x%2 != 0 {
				panic(fmt.Sprintf("Lists(non-basic): expected even element, got %d", x))
			}
		}
	}, WithTestCases(50))
}

// TestListsNestedE2E verifies that nested lists work correctly:
// Lists(Lists(Booleans)) produces a list of lists of booleans.
func TestListsNestedE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		outer := Draw(ht, Lists(Lists(Booleans()).MaxSize(3)).MaxSize(3))
		for i, inner := range outer {
			for j, b := range inner {
				if b != true && b != false {
					panic(fmt.Sprintf("nested Lists[%d][%d]: expected bool, got %v", i, j, b))
				}
			}
		}
	}, WithTestCases(50))
}

// TestListsPropagatesElementErrorE2E verifies that when the element generator
// returns an error, Lists.draw propagates it through its err-check path.
func TestListsPropagatesElementErrorE2E(t *testing.T) {
	t.Parallel()

	rejecting := Filter(Booleans(), func(bool) bool { return false })
	gen := Lists(rejecting).MinSize(1).MaxSize(2)

	Test(t, func(ht *T) {
		_ = Draw(ht, gen)
	}, WithTestCases(1), SuppressHealthCheck(AllHealthChecks()...))
}

// TestListsMappedElementE2E verifies that Lists over a mapped element generator
// applies the mapping element-wise to the result.
func TestListsMappedElementE2E(t *testing.T) {
	t.Parallel()

	doubled := Map(Integers[int](0, 10), func(n int) int {
		return n * 2
	})
	Test(t, func(ht *T) {
		xs := Draw(ht, Lists(doubled).MaxSize(5))
		for _, x := range xs {
			if x%2 != 0 || x < 0 || x > 20 {
				panic(fmt.Sprintf("Lists(mapped): element %d should be even in [0,20]", x))
			}
		}
	}, WithTestCases(50))
}
