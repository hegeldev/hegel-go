package hegel

// filter_test.go tests the Filter function and filteredGenerator type.

import (
	"fmt"
	"testing"
)

// =============================================================================
// Filter function unit tests — verify return types
// =============================================================================

// TestBasicGeneratorFilterReturnsfilteredGenerator verifies that calling Filter
// on a basicGenerator returns a *filteredGenerator.
func TestBasicGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	t.Parallel()
	g := Integers[int](0, 100)
	filtered := Filter(g, func(v int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[int]); !ok {
		t.Fatalf("Filter(basicGenerator) should return *filteredGenerator[int], got %T", filtered)
	}
}

// TestMappedGeneratorFilterReturnsfilteredGenerator verifies that calling Filter
// on a mappedGenerator returns a *filteredGenerator.
func TestMappedGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	t.Parallel()
	inner := Integers[int](0, 100)
	mapped := Map(inner, func(v int) int { return v })
	filtered := Filter(mapped, func(v int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[int]); !ok {
		t.Fatalf("Filter(mappedGenerator) should return *filteredGenerator[int], got %T", filtered)
	}
}

// TestFilteredGeneratorFilterChainsfilteredGenerators verifies that calling Filter
// on a filteredGenerator returns another *filteredGenerator (chained filtering).
func TestFilteredGeneratorFilterChainsfilteredGenerators(t *testing.T) {
	t.Parallel()
	g := Integers[int](0, 100)
	fg := Filter(g, func(v int) bool { return true })
	fg2 := Filter(fg, func(v int) bool { return true })
	if _, ok := fg2.(*filteredGenerator[int]); !ok {
		t.Fatalf("Filter(filteredGenerator) should return *filteredGenerator[int], got %T", fg2)
	}
}

// TestFilteredGeneratorMapReturnsmappedGenerator verifies that calling Map
// on a filteredGenerator returns a *mappedGenerator.
func TestFilteredGeneratorMapReturnsmappedGenerator(t *testing.T) {
	t.Parallel()
	g := Integers[int](0, 100)
	fg := Filter(g, func(v int) bool { return true })
	mapped := Map(fg, func(v int) int { return v })
	if _, ok := mapped.(*mappedGenerator[int, int]); !ok {
		t.Fatalf("Map(filteredGenerator) should return *mappedGenerator, got %T", mapped)
	}
}

// =============================================================================
// Filter on composite generators — verify return types
// =============================================================================

// TestCompositeListGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a compositeListGenerator returns a *filteredGenerator.
func TestCompositeListGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	t.Parallel()
	// compositeListGenerator is produced when elements are non-basic.
	// Filter produces a filteredGenerator (non-basic), forcing Lists into composite path.
	nonBasic := Filter(Integers[int](0, 10), func(v int) bool { return true })
	listGen := Lists(nonBasic).MaxSize(5)
	filtered := Filter(listGen, func(v []int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[[]int]); !ok {
		t.Fatalf("Filter(compositeListGenerator) should return *filteredGenerator, got %T", filtered)
	}
}

// TestCompositeDictGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a compositeDictGenerator returns a *filteredGenerator.
func TestCompositeDictGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	t.Parallel()
	// compositeDictGenerator is produced when key or value is non-basic.
	// Filter produces a filteredGenerator (non-basic), forcing Dicts into composite path.
	nonBasic := Filter(Integers[int](0, 10), func(v int) bool { return true })
	dictGen := Dicts(nonBasic, Integers[int](0, 100))
	filtered := Filter(dictGen, func(v map[int]int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[map[int]int]); !ok {
		t.Fatalf("Filter(compositeDictGenerator) should return *filteredGenerator, got %T", filtered)
	}
}

// TestCompositeOneOfGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a compositeOneOfGenerator returns a *filteredGenerator.
func TestCompositeOneOfGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	t.Parallel()
	// compositeOneOfGenerator is produced when any branch is non-basic.
	// Filter produces a filteredGenerator (non-basic), forcing OneOf into composite path.
	nonBasic := Filter(Integers[int](0, 10), func(v int) bool { return true })
	oneOf := OneOf[int](nonBasic, Integers[int](0, 5))
	filtered := Filter(oneOf, func(v int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[int]); !ok {
		t.Fatalf("Filter(compositeOneOfGenerator) should return *filteredGenerator, got %T", filtered)
	}
}

// TestFlatMappedGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a flatMappedGenerator returns a *filteredGenerator.
func TestFlatMappedGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	t.Parallel()
	flatGen := FlatMap(Integers[int](1, 5), func(v int) Generator[int] {
		return Integers[int](0, v)
	})
	filtered := Filter(flatGen, func(v int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[int]); !ok {
		t.Fatalf("Filter(flatMappedGenerator) should return *filteredGenerator, got %T", filtered)
	}
}

// =============================================================================
// filteredGenerator.draw tests using real hegel binary
// =============================================================================

// TestFilteredGeneratorGeneratePredicatePassesFirstTry verifies that when the
// predicate passes on the first attempt, the value is returned immediately.
func TestFilteredGeneratorGeneratePredicatePassesFirstTry(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	// Filter that always passes: every value is accepted on first try.
	gen := Filter(Integers[int](0, 100), func(v int) bool { return true })
	if _err := runHegel(func(s *TestCase) {
		n := gen.draw(s)
		if n < 0 || n > 100 {
			panic(fmt.Sprintf("Filter: expected [0,100], got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// TestFilteredGeneratorGenerateWithRealPredicate verifies that Filter correctly
// filters values: only even numbers should pass.
func TestFilteredGeneratorGenerateWithRealPredicate(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	// Filter integers [0,50] keeping only even ones.
	gen := Filter(Integers[int](0, 50), func(v int) bool {
		return v%2 == 0
	})
	if _err := runHegel(func(s *TestCase) {
		n := gen.draw(s)
		if n%2 != 0 {
			panic(fmt.Sprintf("Filter even: expected even number, got %d", n))
		}
		if n < 0 || n > 50 {
			panic(fmt.Sprintf("Filter even: expected [0,50], got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestFilteredGeneratorGenerateChainedFilters verifies that chaining two Filter
// calls composes the predicates: both must be satisfied.
func TestFilteredGeneratorGenerateChainedFilters(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	// First filter: even numbers; second filter: divisible by 4.
	// Combined: only multiples of 4.
	gen := Filter(
		Filter(Integers[int](0, 100), func(v int) bool { return v%2 == 0 }),
		func(v int) bool { return v%4 == 0 },
	)
	if _err := runHegel(func(s *TestCase) {
		n := gen.draw(s)
		if n%4 != 0 {
			panic(fmt.Sprintf("chained filter: expected multiple of 4, got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// TestFilteredGeneratorGenerateThenMap verifies that Filter followed by Map
// correctly applies the predicate first and then the transform.
func TestFilteredGeneratorGenerateThenMap(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	// Filter odd numbers from [1,20], then multiply by 10.
	gen := Map(
		Filter(Integers[int](1, 20), func(v int) bool { return v%2 != 0 }),
		func(v int) int { return v * 10 },
	)
	if _, ok := gen.(*mappedGenerator[int, int]); !ok {
		t.Fatalf("Map(Filter(...)) should return *mappedGenerator, got %T", gen)
	}
	if _err := runHegel(func(s *TestCase) {
		n := gen.draw(s)
		// result must be odd*10, so divisible by 10 but result/10 must be odd
		quotient := n / 10
		if quotient*10 != n {
			panic(fmt.Sprintf("filter+map: expected multiple of 10, got %d", n))
		}
		if quotient%2 == 0 {
			panic(fmt.Sprintf("filter+map: expected odd*10, got %d (quotient=%d is even)", n, quotient))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}
