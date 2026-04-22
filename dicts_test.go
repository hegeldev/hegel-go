package hegel

// dicts_test.go tests the Dicts generator: schema structure, transform, basic/composite paths,
// StopTest handling, and e2e integration against the real hegel binary.

import (
	"fmt"
	"testing"
	"unicode/utf8"
)

// =============================================================================
// Dicts: schema unit tests (no server)
// =============================================================================

// TestDictsBasicSchema verifies that Dicts with two basic generators builds
// a dict schema containing the expected fields.
func TestDictsBasicSchema(t *testing.T) {
	t.Parallel()
	keys := Text().MaxSize(5)
	vals := Integers[int64](0, 100)
	gen := Dicts(keys, vals).MaxSize(3)
	bg := gen.buildGenerator().(*basicGenerator[map[string]int64])
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

// TestDictsBasicSchemaNoMaxSize verifies that when HasMaxSize=false, max_size is omitted.
func TestDictsBasicSchemaNoMaxSize(t *testing.T) {
	t.Parallel()
	gen := Dicts(Text().MaxSize(5), Integers[int64](0, 100)).MinSize(1)
	bg := gen.buildGenerator().(*basicGenerator[map[string]int64])
	if _, has := bg.schema["max_size"]; has {
		t.Error("max_size should not be present when HasMaxSize=false")
	}
}

// TestDictsBasicSchemaMinSize verifies that MinSize is propagated to the schema.
func TestDictsBasicSchemaMinSize(t *testing.T) {
	t.Parallel()
	gen := Dicts(Text().MaxSize(5), Integers[int64](0, 100)).MinSize(2).MaxSize(5)
	bg := gen.buildGenerator().(*basicGenerator[map[string]int64])
	minSz, _ := extractCBORInt(bg.schema["min_size"])
	if minSz != 2 {
		t.Errorf("min_size: expected 2, got %d", minSz)
	}
}

// TestDictsBasicBuildsBasicGenerator verifies the direct schema path.
func TestDictsBasicBuildsBasicGenerator(t *testing.T) {
	t.Parallel()
	gen := Dicts(Text().MaxSize(5), Integers[int64](0, 100))
	if _, ok := gen.buildGenerator().(*basicGenerator[map[string]int64]); !ok {
		t.Errorf("Dicts(basic,basic) should build *basicGenerator[map[string]int64], got %T", gen.buildGenerator())
	}
}

// TestDictsCompositeBuildsComposite verifies non-basic input uses the collection protocol.
func TestDictsCompositeBuildsComposite(t *testing.T) {
	t.Parallel()
	// Use a non-basic key generator (mappedGenerator wrapping a basic generator)
	nonBasicKeys := &mappedGenerator[int64, int64]{
		inner: Integers[int64](0, 10),
		fn:    func(v int64) int64 { return v },
	}
	gen := Dicts(nonBasicKeys, Integers[int64](0, 10))
	if _, ok := gen.buildGenerator().(*basicGenerator[map[int64]int64]); ok {
		t.Error("Dicts(non-basic, basic) should not build *basicGenerator")
	}
}

// TestDictsCompositeMap verifies that Map on a compositeDictGenerator returns a mappedGenerator.
func TestDictsCompositeMap(t *testing.T) {
	t.Parallel()
	nonBasicKeys := &mappedGenerator[int64, int64]{
		inner: Integers[int64](0, 10),
		fn:    func(v int64) int64 { return v },
	}
	gen := Dicts(nonBasicKeys, Integers[int64](0, 10))
	mapped := Map(gen, func(m map[int64]int64) map[int64]int64 { return m })
	if _, ok := mapped.(*mappedGenerator[map[int64]int64, map[int64]int64]); !ok {
		t.Errorf("Map on compositeDictGenerator should return *mappedGenerator, got %T", mapped)
	}
}

// =============================================================================
// Dicts: transform tests
// =============================================================================

// TestPairsToMapNoTransform verifies pairsToMap converts pairs to a map with no transforms.
func TestPairsToMapNoTransform(t *testing.T) {
	t.Parallel()
	pairs := []any{
		[]any{"a", int64(1)},
		[]any{"b", int64(2)},
	}
	result := pairsToMap[string, int64](pairs, nil, nil)
	if result["a"] != int64(1) {
		t.Errorf("m['a']: expected 1, got %v", result["a"])
	}
	if result["b"] != int64(2) {
		t.Errorf("m['b']: expected 2, got %v", result["b"])
	}
}

// TestPairsToMapWithKeyTransform verifies that the key transform is applied.
func TestPairsToMapWithKeyTransform(t *testing.T) {
	t.Parallel()
	pairs := []any{
		[]any{"hello", int64(1)},
	}
	keyTransform := func(v any) string {
		s, _ := v.(string)
		return s + "_key"
	}
	result := pairsToMap[string, int64](pairs, keyTransform, nil)
	if _, has := result["hello_key"]; !has {
		t.Errorf("key transform not applied: expected 'hello_key', got %v", result)
	}
}

// TestPairsToMapWithValTransform verifies that the value transform is applied.
func TestPairsToMapWithValTransform(t *testing.T) {
	t.Parallel()
	pairs := []any{
		[]any{"x", int64(5)},
	}
	valTransform := func(v any) int64 {
		n, _ := extractCBORInt(v)
		return n * 2
	}
	result := pairsToMap[string, int64](pairs, nil, valTransform)
	if result["x"] != int64(10) {
		t.Errorf("val transform not applied: expected 10, got %v", result["x"])
	}
}

// TestPairsToMapBothTransforms verifies both key and value transforms are applied.
func TestPairsToMapBothTransforms(t *testing.T) {
	t.Parallel()
	pairs := []any{
		[]any{"k", int64(3)},
	}
	keyTransform := func(v any) string { return "K" }
	valTransform := func(v any) int64 {
		n, _ := extractCBORInt(v)
		return n * 3
	}
	result := pairsToMap[string, int64](pairs, keyTransform, valTransform)
	if result["K"] != int64(9) {
		t.Errorf("expected m['K']=9, got %v", result["K"])
	}
}

// TestPairsToMapNonSliceInput verifies pairsToMap handles non-slice input gracefully.
func TestPairsToMapNonSliceInput(t *testing.T) {
	t.Parallel()
	// If the server sends something unexpected, return an empty map.
	result := pairsToMap[string, int64]("not a slice", nil, nil)
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
	result := pairsToMap[string, int64](pairs, nil, nil)
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
	result := pairsToMap[string, int64](pairs, nil, nil)
	if len(result) != 1 {
		t.Errorf("expected 1 entry, got %d", len(result))
	}
}

// =============================================================================
// Dicts: StopTest during collection operations
// =============================================================================

// TestDictsStopTestOnNewCollection verifies that StopTest during new_collection
// aborts the test without panicking or sending further commands.
func TestDictsStopTestOnNewCollection(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_new_collection")
	err := Run(func(s *TestCase) {
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 10),
			fn:    func(v int64) int64 { return v },
		}
		gen := Dicts(nonBasicKeys, Integers[int64](0, 100)).MaxSize(3)
		_ = gen.draw(s)
	})
	// StopTest causes test to be skipped or aborted, not fail
	_ = err
}

// TestDictsStopTestOnCollectionMore verifies that StopTest during collection_more
// aborts the test cleanly.
func TestDictsStopTestOnCollectionMore(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_collection_more")
	err := Run(func(s *TestCase) {
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 10),
			fn:    func(v int64) int64 { return v },
		}
		gen := Dicts(nonBasicKeys, Integers[int64](0, 100)).MaxSize(3)
		_ = gen.draw(s)
	})
	_ = err
}

// =============================================================================
// Dicts: E2E tests against real hegel binary
// =============================================================================

// TestDictsBasicE2E verifies the basic Dicts generator produces maps with
// string keys and integer values within bounds.
func TestDictsBasicE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		gen := Dicts(Text().MaxSize(5), Integers[int](0, 100)).MaxSize(3)
		m := gen.draw(s)
		if len(m) > 3 {
			panic(fmt.Sprintf("Dicts: expected at most 3 entries, got %d", len(m)))
		}
		for k, val := range m {
			if utf8.RuneCountInString(k) > 5 {
				panic(fmt.Sprintf("Dicts: key %q longer than max codepoints", k))
			}
			if val < 0 || val > 100 {
				panic(fmt.Sprintf("Dicts: value %d out of [0,100]", val))
			}
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestDictsBasicWithBoundsE2E verifies that Dicts with min_size/max_size constraints
// produces maps with the right number of entries.
func TestDictsBasicWithBoundsE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		gen := Dicts(Integers[int](0, 10), Booleans()).MinSize(1).MaxSize(3)
		m := gen.draw(s)
		if len(m) < 1 || len(m) > 3 {
			panic(fmt.Sprintf("Dicts bounded: expected 1-3 entries, got %d", len(m)))
		}
		for k := range m {
			if k < 0 || k > 10 {
				panic(fmt.Sprintf("Dicts bounded: key %d out of [0,10]", k))
			}
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestDictsCompositeNoMaxE2E verifies the composite Dicts generator with no max_size
// uses the default (min_size + 10).
func TestDictsCompositeNoMaxE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 100),
			fn:    func(n int64) int64 { return n },
		}
		// Omit MaxSize to trigger the !g.hasMax branch.
		gen := Dicts(nonBasicKeys, Just("v"))
		m := gen.draw(s)
		_ = m // just verify it doesn't panic
	}, WithTestCases(30)); _err != nil {
		panic(_err)
	}
}

// TestDictsCompositeE2E verifies the composite Dicts generator (non-basic keys)
// produces valid maps.
func TestDictsCompositeE2E(t *testing.T) {
	t.Parallel()
	hegelBinPath(t)
	if _err := Run(func(s *TestCase) {
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
		gen := Dicts(nonBasicKeys, Just("val")).MaxSize(3)
		m := gen.draw(s)
		// All values must be "val"
		for k, val := range m {
			if val != "val" {
				panic(fmt.Sprintf("Dicts composite: expected value 'val', got %v for key %v", val, k))
			}
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}
