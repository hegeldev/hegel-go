package hegel

import (
	"fmt"
	"math"
	"math/big"
	"testing"
)

// =============================================================================
// Generator interface and basicGenerator tests
// =============================================================================

// --- Map free function on basicGenerator: no existing transform ---

func TestBasicGeneratorMapNoTransform(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "boolean"}
	g := &basicGenerator[bool]{schema: schema}
	mapped := Map[bool, string](g, func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	})
	// Map on basicGenerator returns another basicGenerator with same schema.
	bg, ok := mapped.(*basicGenerator[string])
	if !ok {
		t.Fatalf("Map on basicGenerator should return *basicGenerator[string], got %T", mapped)
	}
	if bg.schema["type"] != "boolean" {
		t.Errorf("schema not preserved by Map")
	}
	if bg.transform == nil {
		t.Error("transform should not be nil after Map")
	}
}

// --- Map free function on basicGenerator: compose transforms ---

func TestBasicGeneratorMapComposesTransforms(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "integer"}
	g := &basicGenerator[int64]{
		schema:    schema,
		transform: func(v any) int64 { return extractInt(v) + 1 },
	}
	// Map again: result should be (n+1)*2
	mapped := Map[int64, int64](g, func(v int64) int64 {
		return v * 2
	})
	bg, ok := mapped.(*basicGenerator[int64])
	if !ok {
		t.Fatalf("double Map should return *basicGenerator[int64]")
	}
	// Simulate applying: start with int64(5) -> +1 -> 6 -> *2 -> 12
	result := bg.transform(int64(5))
	if result != 12 {
		t.Errorf("composed transform: expected 12, got %d", result)
	}
}

// --- Map on basicGenerator inner returns basicGenerator ---

func TestMappedGeneratorMapOnBasicInner(t *testing.T) {
	t.Parallel()
	inner := &basicGenerator[int64]{schema: map[string]any{"type": "integer"}, transform: func(v any) int64 { return extractInt(v) }}
	// Map on basicGenerator returns basicGenerator.
	result := Map[int64, int64](inner, func(v int64) int64 { return v })
	if _, ok := result.(*basicGenerator[int64]); !ok {
		t.Errorf("Map on basicGenerator should return *basicGenerator[int64]")
	}
}

// =============================================================================
// collection protocol tests
// =============================================================================

// --- collection StopTest on new_collection ---

func TestCollectionStopTestOnNewCollection(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_new_collection")
	err := Run(func(s *TestCase) {
		coll := newCollection(s, 0, 5)
		_ = coll.More(s)
	})
	// Should not error -- the test was stopped, not failed.
	_ = err
}

// --- collection StopTest on collection_more ---

func TestCollectionStopTestOnCollectionMore(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_collection_more")
	err := Run(func(s *TestCase) {
		coll := newCollection(s, 0, 5)
		_ = coll.More(s)
	})
	_ = err
}

// =============================================================================
// Label constants
// =============================================================================

func TestLabelConstants(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		val  spanLabel
		want int
	}{
		{"List", labelList, 1},
		{"ListElement", labelListElement, 2},
		{"Set", labelSet, 3},
		{"SetElement", labelSetElement, 4},
		{"Map", labelMap, 5},
		{"MapEntry", labelMapEntry, 6},
		{"Tuple", labelTuple, 7},
		{"OneOf", labelOneOf, 8},
		{"Optional", labelOptional, 9},
		{"FixedDict", labelFixedDict, 10},
		{"flatMap", labelFlatMap, 11},
		{"Filter", labelFilter, 12},
		{"Mapped", labelMapped, 13},
		{"SampledFrom", labelSampledFrom, 14},
		{"EnumVariant", labelEnumVariant, 15},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if int(tc.val) != tc.want {
				t.Errorf("%s: expected %d, got %d", tc.name, tc.want, int(tc.val))
			}
		})
	}
}

// =============================================================================
// Integers generator integration test
// =============================================================================

func TestIntegersGeneratorHappyPath(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	var vals []int64
	if _err := Run(func(s *TestCase) {
		v := Draw[int64](s, Integers[int64](0, 100))
		vals = append(vals, v)
		if v < 0 || v > 100 {
			panic(fmt.Sprintf("out of range: %d", v))
		}
	}, WithTestCases(10)); _err != nil {
		panic(_err)
	}
	if len(vals) == 0 {
		t.Error("test function was never called")
	}
}

// --- Integers: schema is correct ---

func TestIntegersSchema(t *testing.T) {
	t.Parallel()
	g := Integers[int64](-5, 5)
	bg, ok := g.(*basicGenerator[int64])
	if !ok {
		t.Fatalf("Integers should return *basicGenerator[int64]")
	}
	min := bg.schema["min_value"].(int64)
	max := bg.schema["max_value"].(int64)
	if min != -5 {
		t.Errorf("min_value: expected -5, got %d", min)
	}
	if max != 5 {
		t.Errorf("max_value: expected 5, got %d", max)
	}
	if bg.schema["type"] != "integer" {
		t.Errorf("type: expected integer, got %v", bg.schema["type"])
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

func runIntegersBoundsCheck[T integer](t *testing.T, name string, lo, hi T) {
	t.Helper()
	t.Run(name, func(t *testing.T) {
		t.Parallel()
		var drew bool
		if _err := Run(func(s *TestCase) {
			v := Draw[T](s, Integers[T](lo, hi))
			drew = true
			if v < lo || v > hi {
				panic(fmt.Sprintf("out of range: lo=%d hi=%d v=%d", lo, hi, v))
			}
		}, WithTestCases(20)); _err != nil {
			t.Fatalf("run failed: %v", _err)
		}
		if !drew {
			t.Error("test function was never called")
		}
	})
}

// =============================================================================
// Just generator tests
// =============================================================================

// TestJustSchema verifies that Just produces a schema with type "constant".
func TestJustSchema(t *testing.T) {
	t.Parallel()
	g := Just(42)
	bg := g.(*basicGenerator[int])
	if bg.schema["type"] != "constant" {
		t.Errorf("Just schema type should be 'constant', got %v", bg.schema["type"])
	}
	// The value field in schema should be nil (null)
	if bg.schema["value"] != nil {
		t.Errorf("Just schema 'value' should be nil, got %v", bg.schema["value"])
	}
}

// TestJustTransformIgnoresInput verifies that Just always returns the constant value.
func TestJustTransformIgnoresInput(t *testing.T) {
	t.Parallel()
	g := Just("hello")
	bg := g.(*basicGenerator[string])
	// transform should ignore the server value and always return "hello"
	result := bg.transform(nil)
	if result != "hello" {
		t.Errorf("Just transform: expected 'hello', got %v", result)
	}
	result = bg.transform(int64(999))
	if result != "hello" {
		t.Errorf("Just transform with non-nil input: expected 'hello', got %v", result)
	}
}

// TestJustE2E verifies that Just always generates the constant value against the real server.
func TestJustE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		v := Draw[int](s, Just(42))
		if v != 42 {
			panic(fmt.Sprintf("Just: expected 42, got %v", v))
		}
	}, WithTestCases(20)); _err != nil {
		panic(_err)
	}
}

// TestJustNonPrimitive verifies that Just works with non-primitive values (pointer identity).
func TestJustNonPrimitive(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	type myStruct struct{ x int }
	val := &myStruct{x: 99}
	if _err := Run(func(s *TestCase) {
		v := Draw[*myStruct](s, Just(val))
		if v != val {
			panic("Just: pointer identity not preserved")
		}
	}, WithTestCases(10)); _err != nil {
		panic(_err)
	}
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

// TestSampledFromSchema verifies that SampledFrom produces an integer schema with correct bounds.
func TestSampledFromSchema(t *testing.T) {
	t.Parallel()
	g := SampledFrom([]string{"a", "b", "c"})
	bg := g.(*basicGenerator[string])
	if bg.schema["type"] != "integer" {
		t.Errorf("schema type: expected 'integer', got %v", bg.schema["type"])
	}
	minVal := bg.schema["min_value"].(int64)
	maxVal := bg.schema["max_value"].(int64)
	if minVal != 0 {
		t.Errorf("min_value: expected 0, got %d", minVal)
	}
	if maxVal != 2 {
		t.Errorf("max_value: expected 2 (len-1), got %d", maxVal)
	}
}

// TestSampledFromTransformMapsIndices verifies that the transform correctly maps
// integer indices to the corresponding elements.
func TestSampledFromTransformMapsIndices(t *testing.T) {
	t.Parallel()
	items := []string{"x", "y", "z"}
	g := SampledFrom(items)
	bg := g.(*basicGenerator[string])
	// Index 0 -> "x", 1 -> "y", 2 -> "z"
	for i, want := range items {
		got := bg.transform(uint64(i))
		if got != want {
			t.Errorf("transform(%d): expected %v, got %v", i, want, got)
		}
	}
}

// TestSampledFromSingleElement verifies that a single-element slice always returns that element.
func TestSampledFromSingleElement(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		v := Draw[string](s, SampledFrom([]string{"only"}))
		if v != "only" {
			panic(fmt.Sprintf("SampledFrom single: expected 'only', got %v", v))
		}
	}, WithTestCases(20)); _err != nil {
		panic(_err)
	}
}

// TestSampledFromE2E verifies that SampledFrom only returns elements from the list
// and that all elements appear (with enough test cases).
func TestSampledFromE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	choices := []string{"apple", "banana", "cherry"}
	seen := map[string]bool{}
	if _err := Run(func(s *TestCase) {
		v := Draw[string](s, SampledFrom(choices))
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
	}, WithTestCases(100)); _err != nil {
		panic(_err)
	}
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
	hegelBinPath(t)
	type myStruct struct{ x int }
	obj1 := &myStruct{x: 1}
	obj2 := &myStruct{x: 2}
	if _err := Run(func(s *TestCase) {
		v := Draw[*myStruct](s, SampledFrom([]*myStruct{obj1, obj2}))
		if v != obj1 && v != obj2 {
			panic("SampledFrom: value is not one of the original pointers")
		}
	}, WithTestCases(10)); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// FromRegex generator tests
// =============================================================================

// TestFromRegexSchema verifies that FromRegex produces the correct schema.
func TestFromRegexSchema(t *testing.T) {
	t.Parallel()
	g := FromRegex(`\d+`, true)
	bg := g.(*basicGenerator[string])
	if bg.schema["type"] != "regex" {
		t.Errorf("schema type: expected 'regex', got %v", bg.schema["type"])
	}
	if bg.schema["pattern"] != `\d+` {
		t.Errorf("pattern: expected '\\d+', got %v", bg.schema["pattern"])
	}
	if bg.schema["fullmatch"] != true {
		t.Errorf("fullmatch: expected true, got %v", bg.schema["fullmatch"])
	}
}

// TestFromRegexFullmatchFalse verifies that fullmatch=false is stored correctly.
func TestFromRegexFullmatchFalse(t *testing.T) {
	t.Parallel()
	g := FromRegex(`abc`, false)
	bg := g.(*basicGenerator[string])
	if bg.schema["fullmatch"] != false {
		t.Errorf("fullmatch: expected false, got %v", bg.schema["fullmatch"])
	}
}

// TestFromRegexE2E verifies that FromRegex generates strings that match the pattern.
func TestFromRegexE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	// Only digits, 1-5 chars
	if _err := Run(func(s *TestCase) {
		v := Draw[string](s, FromRegex(`[0-9]{1,5}`, true))
		if len(v) == 0 || len(v) > 5 {
			panic(fmt.Sprintf("FromRegex: length out of range: %q", v))
		}
		for _, ch := range v {
			if ch < '0' || ch > '9' {
				panic(fmt.Sprintf("FromRegex: non-digit character %q in %q", ch, v))
			}
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// basicGenerator.draw error path (line 78-79)
// =============================================================================

// TestBasicGeneratorGenerateErrorResponse covers the error path in
// basicGenerator.draw when generateFromSchema returns a non-StopTest error.
func TestBasicGeneratorGenerateErrorResponse(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "error_response")
	err := Run(func(s *TestCase) {
		g := &basicGenerator[int64]{schema: map[string]any{"type": "integer"}, transform: func(v any) int64 { return extractInt(v) }}
		_ = g.draw(s) // should panic with requestError -> caught as INTERESTING
	})
	// error_response causes the test to appear interesting (failing).
	_ = err
}

// =============================================================================
// Map on a Generator interface (non-basic returns mappedGenerator)
// =============================================================================

func TestGeneratorMapOnNonBasic(t *testing.T) {
	t.Parallel()
	// A custom generator that is not a basicGenerator.
	schema := map[string]any{"type": "integer"}
	inner := &basicGenerator[int64]{schema: schema, transform: func(v any) int64 { return extractInt(v) }}
	// mappedGenerator is not a basicGenerator.
	mg := &mappedGenerator[int64, int64]{inner: inner, fn: func(v int64) int64 { return v }}
	mapped := Map[int64, int64](mg, func(v int64) int64 { return v })
	// Mapping a non-basic generator should produce a mappedGenerator.
	if _, ok := mapped.(*mappedGenerator[int64, int64]); !ok {
		t.Errorf("Map on non-basic Generator should return *mappedGenerator, got %T", mapped)
	}
}

// =============================================================================
// Map generator E2E tests
// =============================================================================

// TestMapBasicGeneratorE2E verifies that mapping Integers[int](0,100) by doubling
// always produces even values in [0, 200], and the result is still a basicGenerator.
func TestMapBasicGeneratorE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	gen := Map[int, int](Integers[int](0, 100), func(v int) int {
		return v * 2
	})
	// Map on basic generator must preserve basicGenerator type.
	if _, ok := gen.(*basicGenerator[int]); !ok {
		t.Fatalf("Map on basicGenerator should return *basicGenerator[int], got %T", gen)
	}
	if _err := Run(func(s *TestCase) {
		n := Draw[int](s, gen)
		if n%2 != 0 {
			panic(fmt.Sprintf("map(x*2): expected even number, got %d", n))
		}
		if n < 0 || n > 200 {
			panic(fmt.Sprintf("map(x*2): expected [0,200], got %d", n))
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestMapChainedBasicGeneratorE2E verifies that chaining two maps on a basicGenerator
// preserves the basicGenerator type and composes the transforms correctly.
// Integers[int](0,100).Map(x+1).Map(x*2): result must be even, in [2, 202].
func TestMapChainedBasicGeneratorE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	gen := Map[int, int](
		Map[int, int](Integers[int](0, 100), func(v int) int { return v + 1 }),
		func(v int) int { return v * 2 },
	)
	// Both chained maps should still return a basicGenerator (schema preserved).
	if _, ok := gen.(*basicGenerator[int]); !ok {
		t.Fatalf("chained Map on basicGenerator should return *basicGenerator[int], got %T", gen)
	}
	if _err := Run(func(s *TestCase) {
		n := Draw[int](s, gen)
		// (x+1)*2 is always even. x in [0,100] -> result in [2, 202].
		if n%2 != 0 {
			panic(fmt.Sprintf("map(x+1).map(x*2): expected even, got %d", n))
		}
		if n < 2 || n > 202 {
			panic(fmt.Sprintf("map(x+1).map(x*2): expected [2,202], got %d", n))
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestMapNonBasicGeneratorE2E verifies that mapping a mappedGenerator (non-basic)
// wraps it in a MAPPED span and applies the transform correctly.
// The result must be a mappedGenerator (not basicGenerator).
func TestMapNonBasicGeneratorE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	// Create a non-basic generator by wrapping a basicGenerator in mappedGenerator.
	inner := Integers[int](1, 5)
	nonBasic := &mappedGenerator[int, int]{
		inner: inner,
		fn:    func(v int) int { return v }, // identity
	}
	gen := Map[int, int](nonBasic, func(v int) int {
		return v * 3
	})
	if _, ok := gen.(*mappedGenerator[int, int]); !ok {
		t.Fatalf("Map on non-basic Generator should return *mappedGenerator, got %T", gen)
	}
	if _err := Run(func(s *TestCase) {
		n := Draw[int](s, gen)
		// inner is Integers[int](1,5)*1, map(*3): result is in {3, 6, 9, 12, 15}
		if n < 3 || n > 15 || n%3 != 0 {
			panic(fmt.Sprintf("map(*3) on [1,5]: expected multiple of 3 in [3,15], got %d", n))
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestMapSchemaPreservedUnit verifies unit-level schema properties of Map on basicGenerator.
func TestMapSchemaPreservedUnit(t *testing.T) {
	t.Parallel()
	base := Integers[int64](0, 100)
	mapped := Map[int64, int64](base, func(v int64) int64 { return v })
	bg, ok := mapped.(*basicGenerator[int64])
	if !ok {
		t.Fatalf("Map on basicGenerator: expected *basicGenerator[int64], got %T", mapped)
	}
	if bg.schema["type"] != "integer" {
		t.Errorf("schema type: expected 'integer', got %v", bg.schema["type"])
	}
	if bg.transform == nil {
		t.Error("transform should not be nil after Map")
	}
	// Map on basicGenerator must preserve min/max bounds in the schema.
	minV := bg.schema["min_value"].(int64)
	maxV := bg.schema["max_value"].(int64)
	if minV != 0 {
		t.Errorf("min_value: expected 0, got %d", minV)
	}
	if maxV != 100 {
		t.Errorf("max_value: expected 100, got %d", maxV)
	}

	// Double Map on basicGenerator: schema still preserved, transforms compose correctly.
	doubled := Map[int64, int64](
		Map[int64, int64](base, func(v int64) int64 { return v + 10 }),
		func(v int64) int64 { return v * 2 },
	)
	bg2, ok := doubled.(*basicGenerator[int64])
	if !ok {
		t.Fatalf("double Map on basicGenerator: expected *basicGenerator[int64], got %T", doubled)
	}
	if bg2.schema["type"] != "integer" {
		t.Errorf("double map schema type: expected 'integer', got %v", bg2.schema["type"])
	}
	// Verify composition: input 5 -> +10 -> 15 -> *2 -> 30.
	result := bg2.transform(int64(5))
	if result != 30 {
		t.Errorf("double map compose: input 5, expected 30, got %d", result)
	}

	// Map on mappedGenerator: returns a mappedGenerator.
	mg := &mappedGenerator[int64, int64]{inner: base, fn: func(v int64) int64 { return v }}
	mappedMG := Map[int64, int64](mg, func(v int64) int64 { return v })
	if _, ok := mappedMG.(*mappedGenerator[int64, int64]); !ok {
		t.Errorf("mapping a mappedGenerator should produce *mappedGenerator, got %T", mappedMG)
	}
}

// =============================================================================
// Primitive generator schema unit tests
// =============================================================================

// =============================================================================
// filteredGenerator tests
// =============================================================================

// TestFilteredGeneratorFromBasicIsNotBasic verifies that Filter on a basicGenerator
// returns a filteredGenerator (not a basicGenerator).
func TestFilteredGeneratorFromBasicIsNotBasic(t *testing.T) {
	t.Parallel()
	g := Filter[int64](Integers[int64](0, 100), func(v int64) bool { return true })
	if _, ok := g.(*filteredGenerator[int64]); !ok {
		t.Fatalf("Filter on basicGenerator should return *filteredGenerator[int64], got %T", g)
	}
}

// TestFilteredGeneratorFilterMethod verifies that calling Filter on a filteredGenerator
// returns another filteredGenerator.
func TestFilteredGeneratorFilterMethod(t *testing.T) {
	t.Parallel()
	g := Filter[int64](
		Filter[int64](Integers[int64](0, 100), func(v int64) bool { return true }),
		func(v int64) bool { return true },
	)
	if _, ok := g.(*filteredGenerator[int64]); !ok {
		t.Fatalf("Filter on filteredGenerator should return *filteredGenerator[int64], got %T", g)
	}
}

// TestFilteredGeneratorMapMethod verifies that calling Map on a filteredGenerator
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
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		gen := Filter[int](Integers[int](0, 100), func(v int) bool {
			return v > 50
		})
		n := Draw[int](s, gen)
		if n <= 50 {
			panic(fmt.Sprintf("filter(>50): expected n>50, got %d", n))
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestFilteredGeneratorE2EEvenNumbers verifies filter for even numbers.
func TestFilteredGeneratorE2EEvenNumbers(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		gen := Filter[int](Integers[int](0, 10), func(v int) bool {
			return v%2 == 0
		})
		n := Draw[int](s, gen)
		if n%2 != 0 {
			panic(fmt.Sprintf("filter(even): expected even, got %d", n))
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestFilterOnNonBasicGenerators verifies that Filter works on non-basic generators.
func TestFilterOnNonBasicGenerators(t *testing.T) {
	t.Parallel()
	// mappedGenerator.Filter
	mg := &mappedGenerator[int64, int64]{inner: Integers[int64](0, 5), fn: func(v int64) int64 { return v }}
	fg := Filter[int64](mg, func(v int64) bool { return true })
	if _, ok := fg.(*filteredGenerator[int64]); !ok {
		t.Errorf("Filter on mappedGenerator should return *filteredGenerator, got %T", fg)
	}
	// compositeListGenerator.Filter
	cl := &compositeListGenerator[int64]{elements: Integers[int64](0, 5), minSize: 0, maxSize: 3}
	fg2 := Filter[[]int64](cl, func(v []int64) bool { return true })
	if _, ok := fg2.(*filteredGenerator[[]int64]); !ok {
		t.Errorf("Filter on compositeListGenerator should return *filteredGenerator, got %T", fg2)
	}
	// compositeDictGenerator.Filter
	cd := &compositeDictGenerator[int64, int64]{keys: Integers[int64](0, 5), values: Integers[int64](0, 5), minSize: 0}
	fg3 := Filter[map[int64]int64](cd, func(v map[int64]int64) bool { return true })
	if _, ok := fg3.(*filteredGenerator[map[int64]int64]); !ok {
		t.Errorf("Filter on compositeDictGenerator should return *filteredGenerator, got %T", fg3)
	}
	// compositeOneOfGenerator.Filter
	co := &compositeOneOfGenerator[int64]{generators: []Generator[int64]{Integers[int64](0, 5), Integers[int64](6, 10)}}
	fg4 := Filter[int64](co, func(v int64) bool { return true })
	if _, ok := fg4.(*filteredGenerator[int64]); !ok {
		t.Errorf("Filter on compositeOneOfGenerator should return *filteredGenerator, got %T", fg4)
	}
	// flatMappedGenerator.Filter
	fm := &flatMappedGenerator[int64, int64]{source: Integers[int64](0, 5), f: func(v int64) Generator[int64] { return Integers[int64](0, 5) }}
	fg5 := Filter[int64](fm, func(v int64) bool { return true })
	if _, ok := fg5.(*filteredGenerator[int64]); !ok {
		t.Errorf("Filter on flatMappedGenerator should return *filteredGenerator, got %T", fg5)
	}
}

// TestBooleansSchema verifies that Booleans produces a schema with type=boolean.
func TestBooleansSchema(t *testing.T) {
	t.Parallel()
	g := Booleans()
	bg, ok := g.(*basicGenerator[bool])
	if !ok {
		t.Fatalf("Booleans should return *basicGenerator[bool], got %T", g)
	}
	if bg.schema["type"] != "boolean" {
		t.Errorf("type: expected 'boolean', got %v", bg.schema["type"])
	}
}

// TestTextSchema verifies that Text produces the correct schema structure.
func TestTextSchema(t *testing.T) {
	t.Parallel()
	g := Text().MinSize(3).MaxSize(10)
	bg, ok := g.buildGenerator().(*basicGenerator[string])
	if !ok {
		t.Fatalf("Text should build *basicGenerator[string], got %T", g.buildGenerator())
	}
	if bg.schema["type"] != "string" {
		t.Errorf("type: expected 'string', got %v", bg.schema["type"])
	}
	minSize := bg.schema["min_size"].(int64)
	if minSize != 3 {
		t.Errorf("min_size: expected 3, got %d", minSize)
	}
	maxSize := bg.schema["max_size"].(int64)
	if maxSize != 10 {
		t.Errorf("max_size: expected 10, got %d", maxSize)
	}
	// No transform.
	if bg.transform != nil {
		t.Error("Text should have no transform")
	}
}

// TestTextSchemaNoMax verifies that Text with maxSize<0 omits max_size from schema.
func TestTextSchemaNoMax(t *testing.T) {
	t.Parallel()
	g := Text()
	bg := g.buildGenerator().(*basicGenerator[string])
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when maxSize < 0")
	}
	minSize := bg.schema["min_size"].(int64)
	if minSize != 0 {
		t.Errorf("min_size: expected 0, got %d", minSize)
	}
}

// TestBinarySchema verifies that Binary produces the correct schema structure.
func TestBinarySchema(t *testing.T) {
	t.Parallel()
	g := Binary(1, 20)
	bg, ok := g.(*basicGenerator[[]byte])
	if !ok {
		t.Fatalf("Binary should return *basicGenerator[[]byte], got %T", g)
	}
	if bg.schema["type"] != "binary" {
		t.Errorf("type: expected 'binary', got %v", bg.schema["type"])
	}
	minSize := bg.schema["min_size"].(int64)
	if minSize != 1 {
		t.Errorf("min_size: expected 1, got %d", minSize)
	}
	maxSize := bg.schema["max_size"].(int64)
	if maxSize != 20 {
		t.Errorf("max_size: expected 20, got %d", maxSize)
	}
	// No transform needed -- server returns []byte directly via CBOR byte strings.
	if bg.transform != nil {
		t.Error("Binary should have no transform")
	}
}

// TestBinarySchemaNoMax verifies that Binary with maxSize<0 omits max_size from schema.
func TestBinarySchemaNoMax(t *testing.T) {
	t.Parallel()
	g := Binary(0, -1)
	bg := g.(*basicGenerator[[]byte])
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when maxSize < 0")
	}
}

// TestFloatsSchemaWithBounds verifies that Floats with explicit bounds sets all schema fields.
func TestFloatsSchemaWithBounds(t *testing.T) {
	t.Parallel()
	schema := Floats[float64]().Min(0.0).Max(1.0).AllowNaN(false).AllowInfinity(false).buildSchema()
	if schema["type"] != "float" {
		t.Errorf("type: expected 'float', got %v", schema["type"])
	}
	if schema["allow_nan"] != false {
		t.Errorf("allow_nan: expected false, got %v", schema["allow_nan"])
	}
	if schema["allow_infinity"] != false {
		t.Errorf("allow_infinity: expected false, got %v", schema["allow_infinity"])
	}
	if schema["exclude_min"] != false {
		t.Errorf("exclude_min: expected false, got %v", schema["exclude_min"])
	}
	if schema["exclude_max"] != false {
		t.Errorf("exclude_max: expected false, got %v", schema["exclude_max"])
	}
	minVal, _ := schema["min_value"].(float64)
	maxVal, _ := schema["max_value"].(float64)
	if minVal != 0.0 {
		t.Errorf("min_value: expected 0.0, got %v", minVal)
	}
	if maxVal != 1.0 {
		t.Errorf("max_value: expected 1.0, got %v", maxVal)
	}
}

// TestFloatsSchemaUnbounded verifies that Floats with no bounds defaults allow_nan=true, allow_infinity=true.
func TestFloatsSchemaUnbounded(t *testing.T) {
	t.Parallel()
	schema := Floats[float64]().buildSchema()
	if schema["allow_nan"] != true {
		t.Errorf("allow_nan: expected true (no bounds), got %v", schema["allow_nan"])
	}
	if schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true (no bounds), got %v", schema["allow_infinity"])
	}
	if _, hasMin := schema["min_value"]; hasMin {
		t.Error("min_value should not be present when minVal is nil")
	}
	if _, hasMax := schema["max_value"]; hasMax {
		t.Error("max_value should not be present when maxVal is nil")
	}
}

// TestFloatsSchemaOnlyMin verifies Floats with only min bound: allow_nan=false, allow_infinity=true.
func TestFloatsSchemaOnlyMin(t *testing.T) {
	t.Parallel()
	schema := Floats[float64]().Min(0.0).buildSchema()
	// has_min=true, has_max=false -> allow_nan=false, allow_infinity=true
	if schema["allow_nan"] != false {
		t.Errorf("allow_nan: expected false when min set, got %v", schema["allow_nan"])
	}
	if schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true when only min set, got %v", schema["allow_infinity"])
	}
}

// TestFloatsSchemaOnlyMax verifies Floats with only max bound: allow_nan=false, allow_infinity=true.
func TestFloatsSchemaOnlyMax(t *testing.T) {
	t.Parallel()
	schema := Floats[float64]().Max(1.0).buildSchema()
	// has_min=false, has_max=true -> allow_nan=false, allow_infinity=true
	if schema["allow_nan"] != false {
		t.Errorf("allow_nan: expected false when max set, got %v", schema["allow_nan"])
	}
	if schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true when only max set, got %v", schema["allow_infinity"])
	}
}

// TestFloatsSchemaExcludeBounds verifies that excludeMin/excludeMax are stored correctly.
func TestFloatsSchemaExcludeBounds(t *testing.T) {
	t.Parallel()
	schema := Floats[float64]().Min(0.0).Max(1.0).AllowNaN(false).AllowInfinity(false).ExcludeMin().ExcludeMax().buildSchema()
	if schema["exclude_min"] != true {
		t.Errorf("exclude_min: expected true, got %v", schema["exclude_min"])
	}
	if schema["exclude_max"] != true {
		t.Errorf("exclude_max: expected true, got %v", schema["exclude_max"])
	}
}

// =============================================================================
// flatMappedGenerator tests
// =============================================================================

// TestFlatMappedGeneratorIsNotBasic verifies that FlatMap returns a *flatMappedGenerator (not basicGenerator).
func TestFlatMappedGeneratorIsNotBasic(t *testing.T) {
	t.Parallel()
	gen := FlatMap[int64, int64](Integers[int64](math.MinInt64, math.MaxInt64), func(v int64) Generator[int64] {
		return Integers[int64](math.MinInt64, math.MaxInt64)
	})
	if _, ok := gen.(*flatMappedGenerator[int64, int64]); !ok {
		t.Fatalf("FlatMap should return *flatMappedGenerator, got %T", gen)
	}
	// flatMappedGenerator is never a basicGenerator.
	if _, ok := gen.(*basicGenerator[int64]); ok {
		t.Error("FlatMap result should not be a *basicGenerator")
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
	hegelBinPath(t)
	gen := FlatMap[int, string](Integers[int](1, 5), func(v int) Generator[string] {
		return Text().MinSize(v).MaxSize(v) // exact length = n
	})
	if _err := Run(func(s *TestCase) {
		v := Draw[string](s, gen)
		count := len([]rune(v))
		// n is in [1,5], so text length is in [1,5].
		if count < 1 || count > 5 {
			panic(fmt.Sprintf("flat_map text length %d out of [1,5]", count))
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestFlatMappedGeneratorDependency verifies that the second generation genuinely depends
// on the first generated value. We generate n in [2,4] and a list of exactly n elements.
// Every list must have length in [2,4] and all elements must be in [0,100].
func TestFlatMappedGeneratorDependency(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	gen := FlatMap[int64, []int64](Integers[int64](2, 4), func(v int64) Generator[[]int64] {
		sz := int(v)
		return Lists[int64](Integers[int64](0, 100)).MinSize(sz).MaxSize(sz)
	})
	if _err := Run(func(s *TestCase) {
		slice := Draw[[]int64](s, gen)
		if len(slice) < 2 || len(slice) > 4 {
			panic(fmt.Sprintf("flat_map dependency: list length %d not in [2,4]", len(slice)))
		}
		for _, elem := range slice {
			if elem < 0 || elem > 100 {
				panic(fmt.Sprintf("flat_map dependency: element %d not in [0,100]", elem))
			}
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// isSchemaIdentity
// =============================================================================

func TestIsSchemaIdentityTrue(t *testing.T) {
	t.Parallel()
	bg := &basicGenerator[string]{schema: map[string]any{"type": "string"}}
	if !bg.isSchemaIdentity() {
		t.Error("expected identity when transform is nil")
	}
}

func TestIsSchemaIdentityFalse(t *testing.T) {
	t.Parallel()
	bg := &basicGenerator[string]{
		schema:    map[string]any{"type": "string"},
		transform: func(v any) string { return v.(string) },
	}
	if bg.isSchemaIdentity() {
		t.Error("expected non-identity when transform is set")
	}
}

// =============================================================================
// extractFloat — all branches
// =============================================================================

func TestExtractFloatFloat64(t *testing.T) {
	t.Parallel()
	if extractFloat(float64(1.5)) != 1.5 {
		t.Error("float64 branch failed")
	}
}

func TestExtractFloatFloat32(t *testing.T) {
	t.Parallel()
	if extractFloat(float32(1.5)) != float64(float32(1.5)) {
		t.Error("float32 branch failed")
	}
}

func TestExtractFloatInt64(t *testing.T) {
	t.Parallel()
	if extractFloat(int64(42)) != 42.0 {
		t.Error("int64 branch failed")
	}
}

func TestExtractFloatUint64(t *testing.T) {
	t.Parallel()
	if extractFloat(uint64(42)) != 42.0 {
		t.Error("uint64 branch failed")
	}
}

func TestExtractFloatPanicsOnInvalidType(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid type")
		}
	}()
	extractFloat("not a number")
}

// =============================================================================
// extractInt — uint64 branch
// =============================================================================

func TestExtractIntUint64(t *testing.T) {
	t.Parallel()
	if extractInt(uint64(99)) != 99 {
		t.Error("uint64 branch failed")
	}
}

func TestExtractIntBigIntValue(t *testing.T) {
	t.Parallel()
	v := *new(big.Int).SetInt64(456)
	if extractInt(v) != 456 {
		t.Error("big.Int value branch failed")
	}
}

func TestExtractIntBigIntPointer(t *testing.T) {
	t.Parallel()
	v := new(big.Int).SetInt64(123)
	if extractInt(v) != 123 {
		t.Error("*big.Int branch failed")
	}
}

func TestExtractIntPanicsOnInvalidType(t *testing.T) {
	t.Parallel()
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid type")
		}
	}()
	extractInt("not a number")
}

// =============================================================================
// Floats: schema check with only allowNaN set, allowInfinity nil
// =============================================================================

func TestFloatsSchemaExplicitNaNNilInf(t *testing.T) {
	t.Parallel()
	schema := Floats[float64]().AllowNaN(true).buildSchema()
	if schema["allow_nan"] != true {
		t.Errorf("allow_nan: expected true, got %v", schema["allow_nan"])
	}
	if schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true (default with no bounds), got %v", schema["allow_infinity"])
	}
}

// =============================================================================
// Floats: schema check with allowNaN nil, allowInfinity set
// =============================================================================

func TestFloatsSchemaExplicitInfNilNaN(t *testing.T) {
	t.Parallel()
	schema := Floats[float64]().AllowInfinity(true).buildSchema()
	if schema["allow_nan"] != true {
		t.Errorf("allow_nan: expected true (default with no bounds), got %v", schema["allow_nan"])
	}
	if schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true, got %v", schema["allow_infinity"])
	}
}

// TestFloatsFloat32SchemaWidth verifies that Floats[float32]() has "width": int64(32).
func TestFloatsFloat32SchemaWidth(t *testing.T) {
	t.Parallel()
	schema := Floats[float32]().buildSchema()
	w := schema["width"].(int64)
	if w != 32 {
		t.Errorf("width: expected 32, got %d", w)
	}
}

// TestExtractFloatAsFloat32 verifies extractFloatAs[float32].
func TestExtractFloatAsFloat32(t *testing.T) {
	t.Parallel()
	v := extractFloatAs[float32](float64(1.5))
	if v != float32(1.5) {
		t.Errorf("extractFloatAs[float32]: expected 1.5, got %v", v)
	}
}

// TestExtractFloatAsFloat64 verifies extractFloatAs[float64].
func TestExtractFloatAsFloat64(t *testing.T) {
	t.Parallel()
	v := extractFloatAs[float64](float64(2.5))
	if v != 2.5 {
		t.Errorf("extractFloatAs[float64]: expected 2.5, got %v", v)
	}
}

// =============================================================================
// startSpan/stopSpan: aborted path (no-op)
// =============================================================================

func TestStartSpanAborted(t *testing.T) {
	t.Parallel()
	s := &TestCase{aborted: true}
	// Should be a no-op, not panic.
	startSpan(s, labelOneOf)
}

func TestStopSpanAborted(t *testing.T) {
	t.Parallel()
	s := &TestCase{aborted: true}
	// Should be a no-op, not panic.
	stopSpan(s, false)
}

// =============================================================================
// Reject: finished collection path
// =============================================================================

func TestRejectFinishedCollection(t *testing.T) {
	t.Parallel()
	c := &collection{finished: true}
	s := &TestCase{}
	// Should be a no-op since finished = true.
	c.Reject(s)
}

// TestRejectE2E verifies that Reject sends collection_reject to the server
// without error. We create a collection, reject its first element, and
// continue iterating — the server should handle this gracefully.
func TestRejectE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		coll := newCollection(s, 0, 5)
		if coll.More(s) {
			// Reject the first element — tells the server it doesn't count.
			coll.Reject(s)
		}
		// Drain remaining elements.
		for coll.More(s) {
		}
	}, WithTestCases(10)); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// Lists: MaxSize >= 0, MinSize < 0 (clamping path) - schema check
// =============================================================================

func TestListsNegativeMinSizeSchema(t *testing.T) {
	t.Parallel()
	assertPanicsWithMessage(t, "min_size", func() {
		Lists(Integers[int64](0, 10)).MinSize(-5).MaxSize(10).buildGenerator()
	})
}
