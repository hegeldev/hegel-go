package hegel

// maps_test.go tests the Maps generator: schema structure, parse, basic/composite paths,
// StopTest handling, and e2e integration against the real hegel binary.

import (
	"fmt"
	"testing"
	"unicode/utf8"
)

// =============================================================================
// Maps: schema unit tests (no server)
// =============================================================================

// TestMapsBasicSchema verifies that Maps with two basic generators builds
// a dict schema containing the expected fields.
func TestMapsBasicSchema(t *testing.T) {
	t.Parallel()
	keys := Text().MaxSize(5)
	vals := Integers[int64](0, 100)
	gen := Maps(keys, vals).MaxSize(3)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Maps(Text, Integers) should be basic")
	}
	if bg.schema["type"] != "dict" {
		t.Errorf("schema type: expected 'dict', got %v", bg.schema["type"])
	}
	minSz, _ := extractCBORInt(bg.schema["min_size"])
	if minSz != 0 {
		t.Errorf("min_size: expected 0, got %d", minSz)
	}
	maxSz, _ := extractCBORInt(bg.schema["max_size"])
	if maxSz != 3 {
		t.Errorf("max_size: expected 3, got %d", maxSz)
	}
	keySchema, ok := bg.schema["keys"].(map[string]any)
	if !ok {
		t.Fatalf("schema['keys'] should be a map, got %T", bg.schema["keys"])
	}
	if keySchema["type"] != "string" {
		t.Errorf("keys schema type: expected 'string', got %v", keySchema["type"])
	}
	valSchema, ok := bg.schema["values"].(map[string]any)
	if !ok {
		t.Fatalf("schema['values'] should be a map, got %T", bg.schema["values"])
	}
	if valSchema["type"] != "integer" {
		t.Errorf("values schema type: expected 'integer', got %v", valSchema["type"])
	}
}

// TestMapsBasicSchemaNoMaxSize verifies that when HasMaxSize=false, max_size is omitted.
func TestMapsBasicSchemaNoMaxSize(t *testing.T) {
	t.Parallel()
	gen := Maps(Text().MaxSize(5), Integers[int64](0, 100)).MinSize(1)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Maps(Text, Integers) should be basic")
	}
	if _, has := bg.schema["max_size"]; has {
		t.Error("max_size should not be present when HasMaxSize=false")
	}
}

// TestMapsBasicSchemaMinSize verifies that MinSize is propagated to the schema.
func TestMapsBasicSchemaMinSize(t *testing.T) {
	t.Parallel()
	gen := Maps(Text().MaxSize(5), Integers[int64](0, 100)).MinSize(2).MaxSize(5)
	bg, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Maps(Text, Integers) should be basic")
	}
	minSz, _ := extractCBORInt(bg.schema["min_size"])
	if minSz != 2 {
		t.Errorf("min_size: expected 2, got %d", minSz)
	}
}

// TestMapsBasicIsBasic verifies the direct schema path.
func TestMapsBasicIsBasic(t *testing.T) {
	t.Parallel()
	gen := Maps(Text().MaxSize(5), Integers[int64](0, 100))
	_, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("Maps(basic, basic) should be basic")
	}
}

// TestMapsCompositeIsNotBasic verifies non-basic input cannot produce a basic schema.
func TestMapsCompositeIsNotBasic(t *testing.T) {
	t.Parallel()
	// Use a non-basic key generator (mappedGenerator wrapping a basic generator)
	nonBasicKeys := &mappedGenerator[int64, int64]{
		inner: Integers[int64](0, 10),
		fn:    func(v int64) int64 { return v },
	}
	gen := Maps(nonBasicKeys, Integers[int64](0, 10))
	_, ok, err := gen.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Maps(non-basic, basic) should not be basic")
	}
}

// TestMapsCompositeMap verifies that Map on a MapGenerator with non-basic keys returns a mappedGenerator.
func TestMapsCompositeMap(t *testing.T) {
	t.Parallel()
	nonBasicKeys := &mappedGenerator[int64, int64]{
		inner: Integers[int64](0, 10),
		fn:    func(v int64) int64 { return v },
	}
	gen := Maps(nonBasicKeys, Integers[int64](0, 10))
	mapped := Map(gen, func(m map[int64]int64) map[int64]int64 { return m })
	if _, ok := mapped.(*mappedGenerator[map[int64]int64, map[int64]int64]); !ok {
		t.Errorf("Map on MapGenerator(non-basic) should return *mappedGenerator, got %T", mapped)
	}
}

// =============================================================================
// Maps: parse tests
// =============================================================================

// TestPairsToMapIdentityParse verifies pairsToMap converts pairs to a map with identity parse functions.
func TestPairsToMapIdentityParse(t *testing.T) {
	t.Parallel()
	pairs := []any{
		[]any{"a", int64(1)},
		[]any{"b", int64(2)},
	}
	keyParse := func(v any) string { return v.(string) }
	valParse := func(v any) int64 { return v.(int64) }
	result := pairsToMap[string, int64](pairs, keyParse, valParse)
	if result["a"] != int64(1) {
		t.Errorf("m['a']: expected 1, got %v", result["a"])
	}
	if result["b"] != int64(2) {
		t.Errorf("m['b']: expected 2, got %v", result["b"])
	}
}

// TestPairsToMapWithKeyParse verifies that the key parse function is applied.
func TestPairsToMapWithKeyParse(t *testing.T) {
	t.Parallel()
	pairs := []any{
		[]any{"hello", int64(1)},
	}
	keyParse := func(v any) string {
		s, _ := v.(string)
		return s + "_key"
	}
	valParse := func(v any) int64 { return v.(int64) }
	result := pairsToMap[string, int64](pairs, keyParse, valParse)
	if _, has := result["hello_key"]; !has {
		t.Errorf("key parse not applied: expected 'hello_key', got %v", result)
	}
}

// TestPairsToMapWithValParse verifies that the value parse function is applied.
func TestPairsToMapWithValParse(t *testing.T) {
	t.Parallel()
	pairs := []any{
		[]any{"x", int64(5)},
	}
	keyParse := func(v any) string { return v.(string) }
	valParse := func(v any) int64 {
		n, _ := extractCBORInt(v)
		return n * 2
	}
	result := pairsToMap[string, int64](pairs, keyParse, valParse)
	if result["x"] != int64(10) {
		t.Errorf("val parse not applied: expected 10, got %v", result["x"])
	}
}

// TestPairsToMapBothParse verifies both key and value parse functions are applied.
func TestPairsToMapBothParse(t *testing.T) {
	t.Parallel()
	pairs := []any{
		[]any{"k", int64(3)},
	}
	keyParse := func(v any) string { return "K" }
	valParse := func(v any) int64 {
		n, _ := extractCBORInt(v)
		return n * 3
	}
	result := pairsToMap[string, int64](pairs, keyParse, valParse)
	if result["K"] != int64(9) {
		t.Errorf("expected m['K']=9, got %v", result["K"])
	}
}

// TestPairsToMapNonSliceInput verifies pairsToMap handles non-slice input gracefully.
func TestPairsToMapNonSliceInput(t *testing.T) {
	t.Parallel()
	keyParse := func(v any) string { return v.(string) }
	valParse := func(v any) int64 { return v.(int64) }
	result := pairsToMap[string, int64]("not a slice", keyParse, valParse)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

// TestPairsToMapShortPair verifies that short pairs (len < 2) are skipped.
func TestPairsToMapShortPair(t *testing.T) {
	t.Parallel()
	pairs := []any{
		[]any{"only_key"}, // only one element -- skip
		[]any{"a", int64(1)},
	}
	keyParse := func(v any) string { return v.(string) }
	valParse := func(v any) int64 { return v.(int64) }
	result := pairsToMap[string, int64](pairs, keyParse, valParse)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d: %v", len(result), result)
	}
}

// TestPairsToMapNonSlicePair verifies that non-slice pair entries are skipped.
func TestPairsToMapNonSlicePair(t *testing.T) {
	t.Parallel()
	pairs := []any{
		"not a pair",
		[]any{"a", int64(1)},
	}
	keyParse := func(v any) string { return v.(string) }
	valParse := func(v any) int64 { return v.(int64) }
	result := pairsToMap[string, int64](pairs, keyParse, valParse)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

// =============================================================================
// Maps: StopTest during collection operations
// =============================================================================

// TestMapsStopTestOnNewCollection verifies that StopTest during new_collection
// aborts the test without panicking or sending further commands.
func TestMapsStopTestOnNewCollection(t *testing.T) {

	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_new_collection")
	// StopTest causes test to be skipped or aborted, not fail
	Test(t, func(ht *T) {
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 10),
			fn:    func(v int64) int64 { return v },
		}
		gen := Maps(nonBasicKeys, Integers[int64](0, 100)).MaxSize(3)
		_ = gen.draw(ht.TestCase)
	})
}

// TestMapsStopTestOnCollectionMore verifies that StopTest during collection_more
// aborts the test cleanly.
func TestMapsStopTestOnCollectionMore(t *testing.T) {

	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_collection_more")
	Test(t, func(ht *T) {
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 10),
			fn:    func(v int64) int64 { return v },
		}
		gen := Maps(nonBasicKeys, Integers[int64](0, 100)).MaxSize(3)
		_ = gen.draw(ht.TestCase)
	})
}

// =============================================================================
// Maps: E2E tests against real hegel binary
// =============================================================================

// TestMapsBasicE2E verifies the basic Maps generator produces maps with
// string keys and integer values within bounds.
func TestMapsBasicE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		gen := Maps(Text().MaxSize(5), Integers[int](0, 100)).MaxSize(3)
		m := gen.draw(ht.TestCase)
		if len(m) > 3 {
			panic(fmt.Sprintf("Maps: expected at most 3 entries, got %d", len(m)))
		}
		for k, val := range m {
			if utf8.RuneCountInString(k) > 5 {
				panic(fmt.Sprintf("Maps: key %q longer than max codepoints", k))
			}
			if val < 0 || val > 100 {
				panic(fmt.Sprintf("Maps: value %d out of [0,100]", val))
			}
		}
	}, WithTestCases(50))
}

// TestMapsBasicWithBoundsE2E verifies that Maps with min_size/max_size constraints
// produces maps with the right number of entries.
func TestMapsBasicWithBoundsE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		gen := Maps(Integers[int](0, 10), Booleans()).MinSize(1).MaxSize(3)
		m := gen.draw(ht.TestCase)
		if len(m) < 1 || len(m) > 3 {
			panic(fmt.Sprintf("Maps bounded: expected 1-3 entries, got %d", len(m)))
		}
		for k := range m {
			if k < 0 || k > 10 {
				panic(fmt.Sprintf("Maps bounded: key %d out of [0,10]", k))
			}
		}
	}, WithTestCases(50))
}

// TestMapsCompositeNoMaxE2E verifies the composite Maps generator with no max_size
// uses the default (min_size + 10).
func TestMapsCompositeNoMaxE2E(t *testing.T) {

	Test(t, func(ht *T) {
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 100),
			fn:    func(n int64) int64 { return n },
		}
		// Omit MaxSize to trigger the !g.hasMax branch.
		gen := Maps(nonBasicKeys, Just("v"))
		m := gen.draw(ht.TestCase)
		_ = m // just verify it doesn't panic
	}, WithTestCases(30))
}

// TestMapsCompositeE2E verifies the composite Maps generator (non-basic keys)
// produces valid maps.
func TestMapsCompositeE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		// mappedGenerator makes this non-basic -> composite path
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 10),
			fn: func(n int64) int64 {
				if n > 5 {
					return n
				}
				return int64(6) // clamp to > 5
			},
		}
		gen := Maps(nonBasicKeys, Just("val")).MaxSize(3)
		m := gen.draw(ht.TestCase)
		// All values must be "val"
		for k, val := range m {
			if val != "val" {
				panic(fmt.Sprintf("Maps composite: expected value 'val', got %v for key %v", val, k))
			}
		}
	}, WithTestCases(50))
}

func TestMapsNonBasicCollisions(t *testing.T) {
	t.Parallel()

	keys := Filter(Integers[int](0, 4), func(int) bool { return true })
	vals := Integers[int](0, 100)
	gen := Maps(keys, vals).MinSize(3).MaxSize(5)

	Test(t, func(ht *T) {
		_ = gen.draw(ht.TestCase)
	}, WithTestCases(50))
}
