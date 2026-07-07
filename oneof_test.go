package hegel

// oneof_test.go tests the OneOf, Optional, and IPAddresses generators.

import (
	"fmt"
	"testing"
)

// =============================================================================
// OneOf
// =============================================================================

// TestOneOfPath1E2E verifies that OneOf generates values from both branches.
func TestOneOfPath1E2E(t *testing.T) {
	t.Parallel()

	sawShort := false
	sawLong := false
	combined := OneOf(Text().MinSize(1).MaxSize(3), Text().MinSize(10).MaxSize(15))
	Test(t, func(ht *T) {
		v := Draw(ht, combined)
		n := len([]rune(v))
		if n >= 1 && n <= 3 {
			sawShort = true
		} else if n >= 10 && n <= 15 {
			sawLong = true
		}
	})
	if !sawShort {
		t.Error("OneOf: never generated a short string")
	}
	if !sawLong {
		t.Error("OneOf: never generated a long string")
	}
}

// TestOneOfWithTransformsE2E verifies that per-branch mapped generators are
// applied to the chosen branch when generating through the real engine.
func TestOneOfWithTransformsE2E(t *testing.T) {
	t.Parallel()

	gen1 := Map(Just(int(1)), func(v int) int { return v * 2 })
	gen2 := Map(Just(int(2)), func(v int) int { return v * 3 })
	combined := OneOf(gen1, gen2)

	Test(t, func(ht *T) {
		v := Draw(ht, combined)
		if v != 2 && v != 6 {
			panic(fmt.Sprintf("OneOf with transforms: expected 2 or 6, got %d", v))
		}
	}, WithTestCases(50))
}

// TestOneOfMapReturnsMapGen verifies that mapping a OneOf returns a mappedGenerator.
func TestOneOfMapReturnsMapGen(t *testing.T) {
	t.Parallel()
	combined := OneOf[int64](Integers[int64](0, 10), Integers[int64](0, 5))
	mapped := Map(combined, func(v int64) int64 { return v })
	if _, ok := mapped.(*mappedGenerator[int64, int64]); !ok {
		t.Fatalf("Map(OneOf) should return *mappedGenerator, got %T", mapped)
	}
}

// TestOneOfPath3E2E verifies that OneOf over generators of differing shapes
// generates values from both branches using the real hegel binary.
func TestOneOfPath3E2E(t *testing.T) {
	t.Parallel()

	nonBasic := &mappedGenerator[int, int]{
		inner: Integers[int](0, 1000),
		fn:    func(v int) int { return v },
	}
	text := Text().MinSize(1).MaxSize(5)
	nonBasicAny := Map[int, any](nonBasic, func(v int) any { return v })
	textAny := Map[string, any](text, func(v string) any { return v })
	combined := OneOf[any](nonBasicAny, textAny)

	sawInt := false
	sawStr := false
	Test(t, func(ht *T) {
		v := Draw(ht, combined)
		switch v.(type) {
		case int:
			sawInt = true
		case string:
			sawStr = true
		default:
			panic(fmt.Sprintf("OneOf Path3: unexpected type %T", v))
		}
	})
	if !sawInt {
		t.Error("OneOf Path3: never generated an integer")
	}
	if !sawStr {
		t.Error("OneOf Path3: never generated a string")
	}
}

// TestOneOfPanicsWithZeroGenerators verifies that OneOf panics when given no generators.
func TestOneOfPanicsWithZeroGenerators(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("OneOf with 0 generators should panic")
		}
	}()
	OneOf[int64]()
}

// =============================================================================
// Optional
// =============================================================================

// TestOptionalReturnsOptionalGenerator verifies that Optional returns an optionalGenerator.
func TestOptionalReturnsOptionalGenerator(t *testing.T) {
	t.Parallel()
	g := Optional(Integers[int64](0, 10))
	if _, ok := g.(*optionalGenerator[int64]); !ok {
		t.Fatalf("Optional(Integers) should return *optionalGenerator[int64], got %T", g)
	}
}

// TestOptionalE2E verifies that Optional generates both nil and integer values.
func TestOptionalE2E(t *testing.T) {
	t.Parallel()

	sawNil := false
	sawInt := false
	g := Optional(Integers[int](0, 100))
	Test(t, func(ht *T) {
		v := Draw(ht, g)
		if v == nil {
			sawNil = true
		} else {
			sawInt = true
			if *v < 0 || *v > 100 {
				panic(fmt.Sprintf("Optional: expected [0,100], got %d", *v))
			}
		}
	})
	if !sawNil {
		t.Error("Optional: nil value never appeared")
	}
	if !sawInt {
		t.Error("Optional: integer value never appeared")
	}
}

// TestOptionalNonBasicE2E verifies that Optional with a mapped element works.
func TestOptionalNonBasicE2E(t *testing.T) {
	t.Parallel()

	nonBasic := &mappedGenerator[int, int]{inner: Integers[int](0, 10), fn: func(v int) int { return v }}
	g := Optional[int](nonBasic)
	if _, ok := g.(*optionalGenerator[int]); !ok {
		t.Fatalf("Optional(nonBasic) should return *optionalGenerator[int], got %T", g)
	}
	sawNil := false
	sawVal := false
	Test(t, func(ht *T) {
		v := Draw(ht, g)
		if v == nil {
			sawNil = true
		} else {
			sawVal = true
		}
	})
	if !sawNil {
		t.Error("Optional(nonBasic): nil value never appeared")
	}
	if !sawVal {
		t.Error("Optional(nonBasic): non-nil value never appeared")
	}
}

// =============================================================================
// IPAddresses
// =============================================================================

// TestIPAddressesV4E2E verifies IPv4 addresses are v4.
func TestIPAddressesV4E2E(t *testing.T) {
	t.Parallel()

	g := IPAddresses().IPv4()
	Test(t, func(ht *T) {
		v := Draw(ht, g)
		if !v.Is4() {
			panic(fmt.Sprintf("IPv4 address should be v4: %v", v))
		}
	}, WithTestCases(50))
}

// TestIPAddressesV6E2E verifies IPv6 addresses are v6.
func TestIPAddressesV6E2E(t *testing.T) {
	t.Parallel()

	g := IPAddresses().IPv6()
	Test(t, func(ht *T) {
		v := Draw(ht, g)
		if !v.Is6() {
			panic(fmt.Sprintf("IPv6 address should be v6: %v", v))
		}
	}, WithTestCases(50))
}

// TestIPAddressesDefaultE2E verifies default produces both IPv4 and IPv6.
func TestIPAddressesDefaultE2E(t *testing.T) {
	t.Parallel()

	sawV4 := false
	sawV6 := false
	g := IPAddresses()
	Test(t, func(ht *T) {
		v := Draw(ht, g)
		if v.Is4() {
			sawV4 = true
		} else if v.Is6() {
			sawV6 = true
		}
	})
	if !sawV4 {
		t.Error("IPAddresses default: no IPv4 address generated")
	}
	if !sawV6 {
		t.Error("IPAddresses default: no IPv6 address generated")
	}
}

// TestOneOfWithMapMixedTypesE2E verifies that OneOf combining a mapped and a
// constant generator produces correct values.
func TestOneOfWithMapMixedTypesE2E(t *testing.T) {
	t.Parallel()

	gen := OneOf(
		Map(Integers[int](0, 10), func(v int) int { return v * 2 }),
		Just(int(0)),
	)
	Test(t, func(ht *T) {
		v := Draw(ht, gen)
		if v%2 != 0 {
			panic(fmt.Sprintf("OneOf map: expected even, got %d", v))
		}
		if v < 0 || v > 20 {
			panic(fmt.Sprintf("OneOf map: expected [0,20], got %d", v))
		}
	})
}

// TestOneOfAllBranchesAppear verifies that both branches of OneOf appear
// across enough test cases.
func TestOneOfAllBranchesAppear(t *testing.T) {
	t.Parallel()

	sawA := false
	sawB := false
	gen := OneOf(Text().MinSize(1).MaxSize(3), Text().MinSize(4).MaxSize(6))
	Test(t, func(ht *T) {
		v := Draw(ht, gen)
		n := len([]rune(v))
		if n >= 1 && n <= 3 {
			sawA = true
		} else if n >= 4 && n <= 6 {
			sawB = true
		}
	}, WithTestCases(200))
	if !sawA {
		t.Error("OneOf: Text(1,3) branch never appeared")
	}
	if !sawB {
		t.Error("OneOf: Text(4,6) branch never appeared")
	}
}

// TestOptionalCompositeInnerError covers the inner.draw error path in
// optionalGenerator.draw when the chosen branch's inner generator rejects
// (Filter exhausts its retries). With enough cases the value branch is taken.
func TestOptionalCompositeInnerError(t *testing.T) {
	t.Parallel()
	inner := Filter(Booleans(), func(bool) bool { return false })
	Test(t, func(ht *T) {
		Draw[*bool](ht, Optional(inner))
	}, WithTestCases(50))
}
