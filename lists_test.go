package hegel

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

type sequenceGenerator[T any] struct {
	values []T
	next   int
}

type failingIntGenerator struct{}

func (failingIntGenerator) draw(TestCase) (int, error) {
	return 0, errors.New("element failed")
}

func (g *sequenceGenerator[T]) draw(TestCase) (T, error) {
	v := g.values[g.next]
	g.next++
	return v, nil
}

type collectionTestCase struct {
	TestCase
	minSize int
	maxSize *int
}

func (tc *collectionTestCase) startSpan(libhegel.Label) error { return nil }
func (tc *collectionTestCase) stopSpan(bool) error            { return nil }
func (tc *collectionTestCase) newCollection(min int, max *int) (*collection, error) {
	tc.minSize, tc.maxSize = min, max
	return tc.TestCase.newCollection(min, max)
}

func TestUniqueListsRejectsDuplicatesAndKeepsDrawOrder(t *testing.T) {
	t.Parallel()
	tc := &collectionTestCase{TestCase: newStubTestCase(t,
		uintptr(1), libhegel.OK, // new_collection
		true, libhegel.OK, // first element
		true, libhegel.OK, // duplicate
		libhegel.OK,       // collection_reject
		true, libhegel.OK, // second accepted element
		false, libhegel.OK, // collection complete
	)}
	elements := &sequenceGenerator[int]{values: []int{2, 2, 1}}
	got, err := UniqueLists[int](elements).MinSize(2).MaxSize(3).draw(tc)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []int{2, 1}) {
		t.Fatalf("got %v, want [2 1]", got)
	}
	if tc.minSize != 2 || tc.maxSize == nil || *tc.maxSize != 3 {
		t.Fatalf("collection bounds = %d, %v; want 2, 3", tc.minSize, tc.maxSize)
	}
}

func TestUniqueListsByNonComparableElements(t *testing.T) {
	t.Parallel()
	tc := &collectionTestCase{TestCase: newStubTestCase(t,
		uintptr(1), libhegel.OK,
		true, libhegel.OK,
		true, libhegel.OK,
		libhegel.OK, // duplicate key rejection
		true, libhegel.OK,
		false, libhegel.OK,
	)}
	elements := &sequenceGenerator[[]int]{values: [][]int{{2}, {2, 9}, {1}}}
	got, err := UniqueListsBy(elements, func(v []int) int { return v[0] }).MinSize(2).MaxSize(2).draw(tc)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, [][]int{{2}, {1}}) {
		t.Fatalf("got %v, want [[2] [1]]", got)
	}
}

func TestUniqueListsDefaultBounds(t *testing.T) {
	t.Parallel()
	tc := &collectionTestCase{TestCase: newStubTestCase(t,
		uintptr(1), libhegel.OK, false, libhegel.OK,
	)}
	_, err := UniqueLists(Just(1)).draw(tc)
	if err != nil {
		t.Fatal(err)
	}
	if tc.minSize != 0 || tc.maxSize != nil {
		t.Fatalf("default collection bounds = %d, %v; want 0, nil", tc.minSize, tc.maxSize)
	}
}

func TestUniqueListsInvalidBounds(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		gen  UniqueListGenerator[int, int]
		want string
	}{
		{"negative minimum", UniqueLists(Just(1)).MinSize(-1), "min_size=-1 must be non-negative"},
		{"negative maximum", UniqueLists(Just(1)).MaxSize(-1), "max_size=-1 must be non-negative"},
		{"inverted bounds", UniqueLists(Just(1)).MinSize(2).MaxSize(1), "cannot have max_size=1 < min_size=2"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := tt.gen.draw(newStubTestCase(t))
			if err == nil || err.Error() != tt.want {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestUniqueListsByNonComparableDynamicKey(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name string
		key  func(int) any
		want string
	}{
		{"interface", func(int) any { return []int{1} }, "[]int"},
		{"nested interface", func(int) any { return struct{ Value any }{[]int{1}} }, "struct"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tc := &collectionTestCase{TestCase: newStubTestCase(t,
				uintptr(1), libhegel.OK, true, libhegel.OK,
			)}
			_, err := UniqueListsBy(Just(1), tt.key).draw(tc)
			if err == nil || !strings.Contains(err.Error(), "non-comparable dynamic type "+tt.want) {
				t.Fatalf("error = %v, want non-comparable dynamic key error", err)
			}
		})
	}
}

func TestUniqueListsPropagatesCollectionAndElementErrors(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct {
		name    string
		returns []any
		gen     Generator[[]int]
		want    string
	}{
		{"new collection", []any{uintptr(0), libhegel.E_BACKEND, "collection failed"}, UniqueLists(Just(1)), "collection failed"},
		{"more", []any{uintptr(1), libhegel.OK, false, libhegel.E_BACKEND, "more failed"}, UniqueLists(Just(1)), "more failed"},
		{"element", []any{uintptr(1), libhegel.OK, true, libhegel.OK}, UniqueLists[int](failingIntGenerator{}), "element failed"},
		{"reject", []any{uintptr(1), libhegel.OK, true, libhegel.OK, true, libhegel.OK, libhegel.E_BACKEND, "reject failed"}, UniqueLists(&sequenceGenerator[int]{values: []int{1, 1}}), "reject failed"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			tc := &collectionTestCase{TestCase: newStubTestCase(t, tt.returns...)}
			_, err := tt.gen.draw(tc)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
		})
	}
}

func TestUniqueListsBoundsE2E(t *testing.T) {
	t.Parallel()
	Test(t, func(ht *T) {
		values := Draw(ht, UniqueLists(Integers[int](0, 20)).MinSize(2).MaxSize(4))
		if len(values) < 2 || len(values) > 4 {
			panic(fmt.Sprintf("unique list length %d out of [2, 4]", len(values)))
		}
		seen := make(map[int]struct{})
		for _, value := range values {
			if _, exists := seen[value]; exists {
				panic(fmt.Sprintf("duplicate unique list element %d", value))
			}
			seen[value] = struct{}{}
		}
	}, WithTestCases(50))
}

// =============================================================================
// Lists generator validation unit tests
// =============================================================================

// TestListsNegativeMinSizeError verifies that a negative MinSize is rejected at
// draw time (before any engine call).
func TestListsNegativeMinSizeError(t *testing.T) {
	t.Parallel()
	_, err := Lists(Integers[int64](0, 100)).MinSize(-5).MaxSize(10).draw(newStubTestCase(t))
	assertErrorContains(t, "min_size", err)
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
