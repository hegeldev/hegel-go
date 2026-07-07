package hegel

import (
	"fmt"
	"math"
	"testing"
)

// =============================================================================
// Integers generator integration test
// =============================================================================

func TestIntegersGeneratorHappyPath(t *testing.T) {
	t.Parallel()

	var vals []int64
	Test(t, func(ht *T) {
		v := Draw[int64](ht, Integers[int64](0, 100))
		vals = append(vals, v)
		if v < 0 || v > 100 {
			panic(fmt.Sprintf("out of range: %d", v))
		}
	}, WithTestCases(10))
	if len(vals) == 0 {
		t.Error("test function was never called")
	}
}

func TestIntegersInBounds(t *testing.T) {
	runIntegersBoundsCheck[int8](t, "int8", math.MinInt8, math.MaxInt8)
	runIntegersBoundsCheck[int16](t, "int16", math.MinInt16, math.MaxInt16)
	runIntegersBoundsCheck[int32](t, "int32", math.MinInt32, math.MaxInt32)
	runIntegersBoundsCheck[int64](t, "int64", math.MinInt64, math.MaxInt64)
	runIntegersBoundsCheck[int](t, "int", math.MinInt, math.MaxInt)
	runIntegersBoundsCheck[uint8](t, "uint8", 0, math.MaxUint8)
	runIntegersBoundsCheck[uint16](t, "uint16", 0, math.MaxUint16)
	runIntegersBoundsCheck[uint32](t, "uint32", 0, math.MaxUint32)
	runIntegersBoundsCheck[uint64](t, "uint64", 0, math.MaxUint64)
	runIntegersBoundsCheck[uint](t, "uint", 0, math.MaxUint)
}

func runIntegersBoundsCheck[I integer](t *testing.T, name string, lo, hi I) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		var drew bool
		Test(t, func(ht *T) {
			v := Draw[I](ht, Integers[I](lo, hi))
			drew = true
			if v < lo || v > hi {
				panic(fmt.Sprintf("out of range: lo=%d hi=%d v=%d", lo, hi, v))
			}
		}, WithTestCases(20))
		if !drew {
			t.Error("test function was never called")
		}
	})
}

// TestIntegersNegativeExact pins the signed round trip to exact values against
// the real engine. A single-point range [v, v] leaves v as the only legal draw,
// so any corruption of a negative signed value surfaces as got != v — a gap the
// full-range check in TestIntegersInBounds cannot see, since every int64
// satisfies MinInt64 <= v <= MaxInt64.
func TestIntegersNegativeExact(t *testing.T) {
	t.Parallel()
	Test(t, func(ht *T) {
		if got := Draw[int8](ht, Integers[int8](math.MinInt8, math.MinInt8)); got != math.MinInt8 {
			ht.Fatalf("int8: got %d, want %d", got, int8(math.MinInt8))
		}
		if got := Draw[int32](ht, Integers[int32](math.MinInt32, math.MinInt32)); got != math.MinInt32 {
			ht.Fatalf("int32: got %d, want %d", got, int32(math.MinInt32))
		}
		if got := Draw[int64](ht, Integers[int64](math.MinInt64, math.MinInt64)); got != math.MinInt64 {
			ht.Fatalf("int64: got %d, want %d", got, int64(math.MinInt64))
		}
		if got := Draw[int64](ht, Integers[int64](-5, -5)); got != -5 {
			ht.Fatalf("int64(-5): got %d", got)
		}
	}, WithTestCases(3))
}

// =============================================================================
// Integers big-integer encoding helpers
// =============================================================================

// TestFitsInt64 covers the signed and unsigned branches of fitsInt64.
func TestFitsInt64(t *testing.T) {
	t.Parallel()
	if !fitsInt64(int64(math.MinInt64)) {
		t.Error("signed value should always fit int64")
	}
	if !fitsInt64(uint64(math.MaxInt64)) {
		t.Error("uint64 == MaxInt64 should fit")
	}
	if fitsInt64(uint64(math.MaxInt64) + 1) {
		t.Error("uint64 above MaxInt64 should not fit")
	}
}

// =============================================================================
// Just generator tests
// =============================================================================

// TestJustE2E verifies that Just always generates the constant value against the real server.
func TestJustE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[int](ht, Just(42))
		if v != 42 {
			panic(fmt.Sprintf("Just: expected 42, got %v", v))
		}
	}, WithTestCases(20))
}

// TestJustNonPrimitive verifies that Just works with non-primitive values (pointer identity).
func TestJustNonPrimitive(t *testing.T) {
	t.Parallel()

	type myStruct struct{ x int }
	val := &myStruct{x: 99}
	Test(t, func(ht *T) {
		v := Draw[*myStruct](ht, Just(val))
		if v != val {
			panic("Just: pointer identity not preserved")
		}
	}, WithTestCases(10))
}

// =============================================================================
// SampledFrom generator tests
// =============================================================================

// TestSampledFromEmptyPanics verifies that SampledFrom panics for empty slice.
func TestSampledFromEmptyPanics(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("SampledFrom([]) should panic")
		}
	}()
	SampledFrom([]string{})
}

// TestSampledFromSingleElement verifies that a single-element slice always returns that element.
func TestSampledFromSingleElement(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		v := Draw[string](ht, SampledFrom([]string{"only"}))
		if v != "only" {
			panic(fmt.Sprintf("SampledFrom single: expected 'only', got %v", v))
		}
	}, WithTestCases(20))
}

// TestSampledFromE2E verifies that SampledFrom only returns elements from the list
// and that all elements appear (with enough test cases).
func TestSampledFromE2E(t *testing.T) {
	t.Parallel()

	choices := []string{"apple", "banana", "cherry"}
	seen := map[string]bool{}
	Test(t, func(ht *T) {
		v := Draw[string](ht, SampledFrom(choices))
		found := false
		for _, c := range choices {
			if c == v {
				found = true
				break
			}
		}
		if !found {
			panic(fmt.Sprintf("SampledFrom: value %q not in choices", v))
		}
		seen[v] = true
	})
	// After 100 cases we expect all 3 values to have appeared.
	for _, c := range choices {
		if !seen[c] {
			t.Errorf("SampledFrom: value %q never appeared in 100 cases", c)
		}
	}
}

// TestSampledFromNonPrimitive verifies that SampledFrom preserves pointer identity
// for non-primitive values.
func TestSampledFromNonPrimitive(t *testing.T) {
	t.Parallel()

	type myStruct struct{ x int }
	obj1 := &myStruct{x: 1}
	obj2 := &myStruct{x: 2}
	Test(t, func(ht *T) {
		v := Draw[*myStruct](ht, SampledFrom([]*myStruct{obj1, obj2}))
		if v != obj1 && v != obj2 {
			panic("SampledFrom: value is not one of the original pointers")
		}
	}, WithTestCases(10))
}

// =============================================================================
// FromRegex generator tests
// =============================================================================

// TestFromRegexE2E verifies that FromRegex generates strings that match the pattern.
func TestFromRegexE2E(t *testing.T) {
	t.Parallel()

	// Only digits, 1-5 chars
	Test(t, func(ht *T) {
		v := Draw[string](ht, FromRegex(`[0-9]{1,5}`, true))
		if len(v) == 0 || len(v) > 5 {
			panic(fmt.Sprintf("FromRegex: length out of range: %q", v))
		}
		for _, ch := range v {
			if ch < '0' || ch > '9' {
				panic(fmt.Sprintf("FromRegex: non-digit character %q in %q", ch, v))
			}
		}
	}, WithTestCases(50))
}

// =============================================================================
// Map: type classification and E2E
// =============================================================================

// TestGeneratorMapReturnsMapped verifies that Map always returns a
// *mappedGenerator, regardless of the source generator.
func TestGeneratorMapReturnsMapped(t *testing.T) {
	t.Parallel()
	mapped := Map[int64, int64](Integers[int64](0, 100), func(v int64) int64 { return v })
	if _, ok := mapped.(*mappedGenerator[int64, int64]); !ok {
		t.Errorf("Map should return *mappedGenerator, got %T", mapped)
	}
	// Mapping a mappedGenerator still yields a *mappedGenerator.
	again := Map[int64, int64](mapped, func(v int64) int64 { return v })
	if _, ok := again.(*mappedGenerator[int64, int64]); !ok {
		t.Errorf("Map on *mappedGenerator should return *mappedGenerator, got %T", again)
	}
}

// TestMapBasicGeneratorE2E verifies that mapping Integers[int](0,100) by doubling
// always produces even values in [0, 200].
func TestMapBasicGeneratorE2E(t *testing.T) {
	t.Parallel()

	gen := Map[int, int](Integers[int](0, 100), func(v int) int {
		return v * 2
	})
	Test(t, func(ht *T) {
		n := Draw[int](ht, gen)
		if n%2 != 0 {
			panic(fmt.Sprintf("map(x*2): expected even number, got %d", n))
		}
		if n < 0 || n > 200 {
			panic(fmt.Sprintf("map(x*2): expected [0,200], got %d", n))
		}
	}, WithTestCases(50))
}

// TestMapChainedBasicGeneratorE2E verifies that chaining two maps composes
// correctly: Integers[int](0,100).Map(x+1).Map(x*2) is even, in [2, 202].
func TestMapChainedBasicGeneratorE2E(t *testing.T) {
	t.Parallel()

	gen := Map[int, int](
		Map[int, int](Integers[int](0, 100), func(v int) int { return v + 1 }),
		func(v int) int { return v * 2 },
	)
	Test(t, func(ht *T) {
		n := Draw[int](ht, gen)
		if n%2 != 0 {
			panic(fmt.Sprintf("map(x+1).map(x*2): expected even, got %d", n))
		}
		if n < 2 || n > 202 {
			panic(fmt.Sprintf("map(x+1).map(x*2): expected [2,202], got %d", n))
		}
	}, WithTestCases(50))
}

// TestMapNonBasicGeneratorE2E verifies that mapping a mappedGenerator applies
// the function correctly.
func TestMapNonBasicGeneratorE2E(t *testing.T) {
	t.Parallel()

	nonBasic := &mappedGenerator[int, int]{
		inner: Integers[int](1, 5),
		fn:    func(v int) int { return v }, // identity
	}
	gen := Map[int, int](nonBasic, func(v int) int {
		return v * 3
	})
	Test(t, func(ht *T) {
		n := Draw[int](ht, gen)
		// inner is Integers[int](1,5)*1, map(*3): result is in {3, 6, 9, 12, 15}
		if n < 3 || n > 15 || n%3 != 0 {
			panic(fmt.Sprintf("map(*3) on [1,5]: expected multiple of 3 in [3,15], got %d", n))
		}
	}, WithTestCases(50))
}

// TestMapOnBooleansE2E exercises Map over Booleans, composing a bool→string transform.
func TestMapOnBooleansE2E(t *testing.T) {
	t.Parallel()

	gen := Map[bool, string](Booleans(), func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	})
	Test(t, func(ht *T) {
		v := Draw[string](ht, gen)
		if v != "yes" && v != "no" {
			panic(fmt.Sprintf("Map(Booleans): expected 'yes' or 'no', got %q", v))
		}
	}, WithTestCases(30))
}

// =============================================================================
// filteredGenerator tests
// =============================================================================

// TestFilteredGeneratorFromBasicIsNotBasic verifies that Filter returns a filteredGenerator.
func TestFilterReturnsFiltered(t *testing.T) {
	t.Parallel()
	g := Filter[int64](Integers[int64](0, 100), func(v int64) bool { return true })
	if _, ok := g.(*filteredGenerator[int64]); !ok {
		t.Fatalf("Filter should return *filteredGenerator[int64], got %T", g)
	}
}

// TestFilteredGeneratorFilterMethod verifies that Filter on a filteredGenerator
// returns another filteredGenerator.
func TestFilteredGeneratorNested(t *testing.T) {
	t.Parallel()
	g := Filter[int64](
		Filter[int64](Integers[int64](0, 100), func(v int64) bool { return true }),
		func(v int64) bool { return true },
	)
	if _, ok := g.(*filteredGenerator[int64]); !ok {
		t.Fatalf("Filter on filteredGenerator should return *filteredGenerator[int64], got %T", g)
	}
}

// TestFilteredGeneratorMapMethod verifies that Map on a filteredGenerator
// returns a mappedGenerator.
func TestFilteredGeneratorMapMethod(t *testing.T) {
	t.Parallel()
	g := Filter[int64](Integers[int64](0, 100), func(v int64) bool { return true })
	mapped := Map[int64, int64](g, func(v int64) int64 { return v })
	if _, ok := mapped.(*mappedGenerator[int64, int64]); !ok {
		t.Fatalf("Map on filteredGenerator should return *mappedGenerator, got %T", mapped)
	}
}

// TestFilteredGeneratorE2EAlwaysPasses verifies an e2e filter with a predicate
// that values greater than 50.
func TestFilteredGeneratorE2EAlwaysPasses(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		gen := Filter[int](Integers[int](0, 100), func(v int) bool {
			return v > 50
		})
		n := Draw[int](ht, gen)
		if n <= 50 {
			panic(fmt.Sprintf("filter(>50): expected n>50, got %d", n))
		}
	}, WithTestCases(50))
}

// TestFilteredGeneratorE2EEvenNumbers verifies filter for even numbers.
func TestFilteredGeneratorE2EEvenNumbers(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		gen := Filter[int](Integers[int](0, 10), func(v int) bool {
			return v%2 == 0
		})
		n := Draw[int](ht, gen)
		if n%2 != 0 {
			panic(fmt.Sprintf("filter(even): expected even, got %d", n))
		}
	}, WithTestCases(50))
}

// TestFilterOnNonBasicGenerators verifies that Filter works on the various
// non-primitive generator types.
func TestFilterOnNonBasicGenerators(t *testing.T) {
	t.Parallel()
	// mappedGenerator.Filter
	mg := &mappedGenerator[int64, int64]{inner: Integers[int64](0, 5), fn: func(v int64) int64 { return v }}
	fg := Filter[int64](mg, func(v int64) bool { return true })
	if _, ok := fg.(*filteredGenerator[int64]); !ok {
		t.Errorf("Filter on mappedGenerator should return *filteredGenerator, got %T", fg)
	}
	// ListGenerator.Filter
	cl := Lists[int64](mg).MaxSize(3)
	fg2 := Filter[[]int64](cl, func(v []int64) bool { return true })
	if _, ok := fg2.(*filteredGenerator[[]int64]); !ok {
		t.Errorf("Filter on ListGenerator should return *filteredGenerator, got %T", fg2)
	}
	// MapGenerator.Filter
	cd := Maps[int64, int64](mg, Integers[int64](0, 5))
	fg3 := Filter[map[int64]int64](cd, func(v map[int64]int64) bool { return true })
	if _, ok := fg3.(*filteredGenerator[map[int64]int64]); !ok {
		t.Errorf("Filter on MapGenerator should return *filteredGenerator, got %T", fg3)
	}
	// oneOfGenerator.Filter
	co := &oneOfGenerator[int64]{generators: []Generator[int64]{Integers[int64](0, 5), Integers[int64](6, 10)}}
	fg4 := Filter[int64](co, func(v int64) bool { return true })
	if _, ok := fg4.(*filteredGenerator[int64]); !ok {
		t.Errorf("Filter on oneOfGenerator should return *filteredGenerator, got %T", fg4)
	}
	// flatMappedGenerator.Filter
	fm := &flatMappedGenerator[int64, int64]{source: Integers[int64](0, 5), f: func(v int64) Generator[int64] { return Integers[int64](0, 5) }}
	fg5 := Filter[int64](fm, func(v int64) bool { return true })
	if _, ok := fg5.(*filteredGenerator[int64]); !ok {
		t.Errorf("Filter on flatMappedGenerator should return *filteredGenerator, got %T", fg5)
	}
}

// =============================================================================
// flatMappedGenerator tests
// =============================================================================

// TestFlatMapReturnsFlatMapped verifies that FlatMap returns a *flatMappedGenerator.
func TestFlatMapReturnsFlatMapped(t *testing.T) {
	t.Parallel()
	gen := FlatMap[int64, int64](Integers[int64](math.MinInt64, math.MaxInt64), func(v int64) Generator[int64] {
		return Integers[int64](math.MinInt64, math.MaxInt64)
	})
	if _, ok := gen.(*flatMappedGenerator[int64, int64]); !ok {
		t.Fatalf("FlatMap should return *flatMappedGenerator, got %T", gen)
	}
}

// TestFlatMappedGeneratorMapReturnsMapped verifies that Map on flatMappedGenerator returns a mappedGenerator.
func TestFlatMappedGeneratorMapReturnsMapped(t *testing.T) {
	t.Parallel()
	gen := FlatMap[int64, int64](Integers[int64](1, 5), func(v int64) Generator[int64] {
		return Integers[int64](0, 10)
	})
	mapped := Map[int64, int64](gen, func(v int64) int64 { return v })
	if _, ok := mapped.(*mappedGenerator[int64, int64]); !ok {
		t.Fatalf("Map on flatMappedGenerator should return *mappedGenerator, got %T", mapped)
	}
}

// TestFlatMappedGeneratorE2E verifies that flat_map produces a dependent value.
// integers(1,5).flat_map(n => text(min=n, max=n)) always produces text of length in [1,5].
func TestFlatMappedGeneratorE2E(t *testing.T) {
	t.Parallel()

	gen := FlatMap[int, string](Integers[int](1, 5), func(v int) Generator[string] {
		return Text().MinSize(v).MaxSize(v) // exact length = n
	})
	Test(t, func(ht *T) {
		v := Draw[string](ht, gen)
		count := len([]rune(v))
		// n is in [1,5], so text length is in [1,5].
		if count < 1 || count > 5 {
			panic(fmt.Sprintf("flat_map text length %d out of [1,5]", count))
		}
	}, WithTestCases(50))
}

// TestFlatMappedGeneratorDependency verifies that the second generation genuinely depends
// on the first generated value.
func TestFlatMappedGeneratorDependency(t *testing.T) {
	t.Parallel()

	gen := FlatMap[int64, []int64](Integers[int64](2, 4), func(v int64) Generator[[]int64] {
		sz := int(v)
		return Lists[int64](Integers[int64](0, 100)).MinSize(sz).MaxSize(sz)
	})
	Test(t, func(ht *T) {
		slice := Draw[[]int64](ht, gen)
		if len(slice) < 2 || len(slice) > 4 {
			panic(fmt.Sprintf("flat_map dependency: list length %d not in [2,4]", len(slice)))
		}
		for _, elem := range slice {
			if elem < 0 || elem > 100 {
				panic(fmt.Sprintf("flat_map dependency: element %d not in [0,100]", elem))
			}
		}
	}, WithTestCases(50))
}

// TestFlatMappedGeneratorSourceError covers the source.draw err path in
// flatMappedGenerator.draw. The source Filter always rejects, so after
// maxFilterAttempts it returns a rejection, which flatMappedGenerator.draw
// propagates from inside its withSpan body.
func TestFlatMappedGeneratorSourceError(t *testing.T) {
	t.Parallel()
	source := Filter(Booleans(), func(bool) bool { return false })
	gen := FlatMap[bool, bool](source, func(bool) Generator[bool] {
		return Booleans()
	})
	Test(t, func(ht *T) {
		_ = Draw(ht, gen)
	}, WithTestCases(20), SuppressHealthCheck(FilterTooMuch))
}

// =============================================================================
// Misc runner-facing helpers
// =============================================================================

// TestInSpan verifies that (*testCase).inSpan reflects the depth counter.
func TestInSpan(t *testing.T) {
	t.Parallel()
	s := &testCase{}
	if s.inSpan() {
		t.Fatalf("inSpan at depth 0: got true, want false")
	}
	s.depth = 1
	if !s.inSpan() {
		t.Fatalf("inSpan at depth 1: got false, want true")
	}
}

// TestRejectFinishedCollection covers the finished-collection no-op path of Reject.
func TestRejectFinishedCollection(t *testing.T) {
	t.Parallel()
	c := &collection{finished: true}
	c.Reject("duplicate key")
	if err := c.Err(); err != nil {
		t.Fatalf("Reject on finished: %v", err)
	}
}

// TestRejectE2E verifies that Reject sends collection_reject to the server
// without error.
func TestRejectE2E(t *testing.T) {

	Test(t, func(ht *T) {
		max := 5
		coll, err := ht.testCase.newCollection(0, &max)
		if err != nil {
			panic(err)
		}
		if coll.More() {
			coll.Reject("duplicate key")
		}
		for coll.More() {
		}
		if err := coll.Err(); err != nil {
			panic(err)
		}
	}, WithTestCases(10))
}
