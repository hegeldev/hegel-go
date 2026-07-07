package hegel

// maps_test.go tests the Maps generator: composite Map behaviour and e2e
// integration against the real hegel binary.

import (
	"fmt"
	"testing"
	"unicode/utf8"
)

// TestMapsCompositeMap verifies that Map on a MapGenerator returns a mappedGenerator.
func TestMapsCompositeMap(t *testing.T) {
	t.Parallel()
	gen := Maps(Integers[int64](0, 10), Integers[int64](0, 10))
	mapped := Map(gen, func(m map[int64]int64) map[int64]int64 { return m })
	if _, ok := mapped.(*mappedGenerator[map[int64]int64, map[int64]int64]); !ok {
		t.Errorf("Map on MapGenerator should return *mappedGenerator, got %T", mapped)
	}
}

// =============================================================================
// Maps: E2E tests against real libhegel
// =============================================================================

// TestMapsBasicE2E verifies the basic Maps generator produces maps with
// string keys and integer values within bounds.
func TestMapsBasicE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		gen := Maps(Text().MaxSize(5), Integers[int](0, 100)).MaxSize(3)
		m := Draw(ht, gen)
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
		m := Draw(ht, gen)
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

// TestMapsCompositeNoMaxE2E verifies the Maps generator with no max_size.
func TestMapsCompositeNoMaxE2E(t *testing.T) {

	Test(t, func(ht *T) {
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 100),
			fn:    func(n int64) int64 { return n },
		}
		gen := Maps(nonBasicKeys, Just("v"))
		m := Draw(ht, gen)
		_ = m // just verify it doesn't panic
	}, WithTestCases(30))
}

// TestMapsCompositeE2E verifies the Maps generator with a mapped key generator
// produces valid maps.
func TestMapsCompositeE2E(t *testing.T) {
	t.Parallel()

	Test(t, func(ht *T) {
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 10),
			fn: func(n int64) int64 {
				if n > 5 {
					return n
				}
				return int64(6)
			},
		}
		gen := Maps(nonBasicKeys, Just("val")).MaxSize(3)
		m := Draw(ht, gen)
		for k, val := range m {
			if val != "val" {
				panic(fmt.Sprintf("Maps composite: expected value 'val', got %v for key %v", val, k))
			}
		}
	}, WithTestCases(50))
}

// TestMapsNonBasicCollisions exercises the duplicate-key rejection path.
func TestMapsNonBasicCollisions(t *testing.T) {
	t.Parallel()

	keys := Filter(Integers[int](0, 4), func(int) bool { return true })
	vals := Integers[int](0, 100)
	gen := Maps(keys, vals).MinSize(3).MaxSize(5)

	Test(t, func(ht *T) {
		_ = Draw(ht, gen)
	}, WithTestCases(50))
}
