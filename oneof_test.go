package hegel

// oneof_test.go tests the OneOf, Optional, and IPAddresses generators.

import (
	"fmt"
	"testing"
)

// =============================================================================
// OneOf — all basic generators
// =============================================================================

// TestOneOfAllBasicSchema verifies that OneOf with all basic generators
// produces a flat {"type": "one_of", "generators": [...]} schema —
// child schemas pass through unchanged. Under the new protocol the server
// supplies the branch index in the response, so we don't wrap children
// in tagged tuples.
func TestOneOfAllBasicSchema(t *testing.T) {
	t.Parallel()
	g1 := Booleans()
	g2 := Booleans()
	combined := OneOf(g1, g2)

	bg, ok, err := combined.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("OneOf all-basic should be basic")
	}
	if bg.schema["type"] != "one_of" {
		t.Fatalf("OneOf schema should have type 'one_of'; got %v", bg.schema)
	}
	generators, hasGenerators := bg.schema["generators"]
	if !hasGenerators {
		t.Fatalf("OneOf schema should have 'generators' key; got %v", bg.schema)
	}
	schemas, ok := generators.([]any)
	if !ok {
		t.Fatalf("one_of value should be []any, got %T", generators)
	}
	if len(schemas) != 2 {
		t.Errorf("one_of should have 2 branches, got %d", len(schemas))
	}
	// Each branch is the underlying basicGenerator schema, unmodified —
	// Booleans() produces {"type": "boolean"} and that survives the OneOf
	// wrapping unchanged, with no tagged-tuple / constant tag injection.
	for i, s := range schemas {
		m, ok := s.(map[string]any)
		if !ok {
			t.Errorf("branch %d should be map[string]any, got %T", i, s)
			continue
		}
		if m["type"] != "boolean" {
			t.Errorf("branch %d: expected child schema type 'boolean', got %v", i, m["type"])
		}
	}
}

// TestOneOfPath1E2E verifies that OneOf path 1 generates values from both branches.
func TestOneOfPath1E2E(t *testing.T) {
	t.Parallel()

	sawShort := false
	sawLong := false
	combined := OneOf(Text().MinSize(1).MaxSize(3), Text().MinSize(10).MaxSize(15))
	if _err := Run(func(s *TestCase) {
		v := combined.draw(s)
		n := len([]rune(v))
		if n >= 1 && n <= 3 {
			sawShort = true
		} else if n >= 10 && n <= 15 {
			sawLong = true
		}
	}, WithTestCases(100)); _err != nil {
		panic(_err)
	}
	if !sawShort {
		t.Error("OneOf: never generated a short string")
	}
	if !sawLong {
		t.Error("OneOf: never generated a long string")
	}
}

// =============================================================================
// OneOf — all basic, with parse transforms
// =============================================================================

// TestOneOfWithTransformsSchema verifies that OneOf over mapped basicGenerators
// emits the same flat {"type": "one_of", "generators": [...]} schema —
// child schemas pass through, no constant tags injected.
func TestOneOfWithTransformsSchema(t *testing.T) {
	t.Parallel()
	gen1 := Map(Just(int64(1)), func(v int64) int64 { return v * 2 })
	gen2 := Map(Just(int64(2)), func(v int64) int64 { return v * 3 })
	combined := OneOf(gen1, gen2)

	bg, ok, err := combined.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("OneOf with transforms should be basic")
	}
	if bg.schema["type"] != "one_of" {
		t.Fatalf("schema should have type 'one_of'; got %v", bg.schema)
	}
	generators, hasGenerators := bg.schema["generators"]
	if !hasGenerators {
		t.Fatalf("schema should have 'generators' key; got %v", bg.schema)
	}
	schemas, ok := generators.([]any)
	if !ok {
		t.Fatalf("one_of value should be []any")
	}
	if len(schemas) != 2 {
		t.Errorf("one_of should have 2 branches, got %d", len(schemas))
	}
	// Each branch is the underlying basicGenerator schema, unmodified —
	// Just(...) produces a {"type": "constant", ...} schema and that survives
	// the OneOf wrapping unchanged.
	for i, s := range schemas {
		m, ok := s.(map[string]any)
		if !ok {
			t.Errorf("branch %d should be map[string]any, got %T", i, s)
			continue
		}
		if m["type"] == "tuple" {
			t.Errorf("branch %d: must not wrap child schema in a tagged tuple", i)
		}
		if m["type"] != "constant" {
			t.Errorf("branch %d: expected child schema type 'constant', got %v", i, m["type"])
		}
	}
}

// TestOneOfWithTransformsParse verifies that the synthesized parse dispatches
// on the wire-side index to the matching per-branch parse fn.
func TestOneOfWithTransformsParse(t *testing.T) {
	t.Parallel()
	// just(1).map(*2) -> always 2; just(2).map(*3) -> always 6
	gen1 := Map(Just(int64(1)), func(v int64) int64 { return v * 2 })
	gen2 := Map(Just(int64(2)), func(v int64) int64 { return v * 3 })
	combined := OneOf(gen1, gen2)

	bg, _, err := combined.asBasic()
	if err != nil {
		t.Fatal(err)
	}

	// Simulate the wire response [0, 1] — index=0 selects parse 0 (*2): 1*2=2.
	result0 := bg.parse([]any{int64(0), int64(1)})
	if result0 != 2 {
		t.Errorf("index=0: expected 2, got %d", result0)
	}

	// Simulate the wire response [1, 2] — index=1 selects parse 1 (*3): 2*3=6.
	result1 := bg.parse([]any{int64(1), int64(2)})
	if result1 != 6 {
		t.Errorf("index=1: expected 6, got %d", result1)
	}
}

// TestOneOfParseDispatchMixedBranches verifies that when branches have
// different parse functions, the dispatcher uses the wire-side index to
// call the matching one for each branch.
func TestOneOfParseDispatchMixedBranches(t *testing.T) {
	t.Parallel()
	// Mix: one identity-like branch, one with a composed parse.
	// Booleans has a simple type-assertion parse, Just(true).Map(!v) has a composed parse.
	gen1 := Booleans()                                       // identity parse
	gen2 := Map(Just(true), func(v bool) bool { return !v }) // negating parse
	combined := OneOf(gen1, gen2)

	bg, ok, err := combined.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("OneOf(all-basic) should be basic")
	}
	// index=0: identity branch — return value as-is.
	result0 := bg.parse([]any{int64(0), true})
	if result0 != true {
		t.Errorf("index=0 (identity): expected true, got %v", result0)
	}
	// index=1: mapped branch — negate true → false.
	result1 := bg.parse([]any{int64(1), true})
	if result1 != false {
		t.Errorf("index=1 (mapped): expected false, got %v", result1)
	}
}

// TestOneOfWithTransformsE2E verifies that per-branch parse transforms are
// applied to the matching wire-side index when generating through the real
// server.
func TestOneOfWithTransformsE2E(t *testing.T) {
	t.Parallel()

	gen1 := Map(Just(int(1)), func(v int) int { return v * 2 })
	gen2 := Map(Just(int(2)), func(v int) int { return v * 3 })
	combined := OneOf(gen1, gen2)

	if _err := Run(func(s *TestCase) {
		v := combined.draw(s)
		if v != 2 && v != 6 {
			panic(fmt.Sprintf("OneOf with transforms: expected 2 or 6, got %d", v))
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// OneOf — Path 3: any non-basic generator
// =============================================================================

// TestOneOfPath3IsComposite verifies that OneOf with a non-basic generator
// reports asBasic=false (forcing the composite draw path).
func TestOneOfPath3IsComposite(t *testing.T) {
	t.Parallel()
	// A mappedGenerator is not a basicGenerator.
	nonBasic := &mappedGenerator[int64, int64]{
		inner: Integers[int64](0, 10),
		fn:    func(v int64) int64 { return v },
	}
	basic := Integers[int64](0, 10)
	combined := OneOf[int64](nonBasic, basic)
	_, ok, err := combined.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatalf("OneOf with non-basic branch should not be basic")
	}
}

// TestOneOfPath3MapReturnsMapGen verifies that mapping a OneOf with non-basic
// branches returns a mappedGenerator.
func TestOneOfPath3MapReturnsMapGen(t *testing.T) {
	t.Parallel()
	nonBasic := &mappedGenerator[int64, int64]{inner: Integers[int64](0, 10), fn: func(v int64) int64 { return v }}
	combined := OneOf[int64](nonBasic, Integers[int64](0, 5))
	mapped := Map(combined, func(v int64) int64 { return v })
	if _, ok := mapped.(*mappedGenerator[int64, int64]); !ok {
		t.Fatalf("Map(OneOf(non-basic)) should return *mappedGenerator, got %T", mapped)
	}
}

// TestOneOfPath3E2E verifies that Path 3 generates values from both branches
// using the real hegel binary.
func TestOneOfPath3E2E(t *testing.T) {
	t.Parallel()

	// nonBasic: a mappedGenerator (not a *basicGenerator)
	nonBasic := &mappedGenerator[int, int]{
		inner: Integers[int](0, 1000),
		fn:    func(v int) int { return v }, // identity, but still a mappedGenerator
	}
	text := Text().MinSize(1).MaxSize(5)
	// These have different types so we need to unify. Use any.
	nonBasicAny := Map[int, any](nonBasic, func(v int) any { return v })
	textAny := Map[string, any](text, func(v string) any { return v })
	combined := OneOf[any](nonBasicAny, textAny)

	sawInt := false
	sawStr := false
	if _err := Run(func(s *TestCase) {
		v := combined.draw(s)
		switch v.(type) {
		case int:
			sawInt = true
		case string:
			sawStr = true
		default:
			panic(fmt.Sprintf("OneOf Path3: unexpected type %T", v))
		}
	}, WithTestCases(100)); _err != nil {
		panic(_err)
	}
	if !sawInt {
		t.Error("OneOf Path3: never generated an integer")
	}
	if !sawStr {
		t.Error("OneOf Path3: never generated a string")
	}
}

// =============================================================================
// OneOf — requires at least 2 generators
// =============================================================================

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

// TestOptionalSchema verifies that Optional returns an optionalGenerator.
func TestOptionalSchema(t *testing.T) {
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
	if _err := Run(func(s *TestCase) {
		v := g.draw(s)
		if v == nil {
			sawNil = true
		} else {
			sawInt = true
			if *v < 0 || *v > 100 {
				panic(fmt.Sprintf("Optional: expected [0,100], got %d", *v))
			}
		}
	}, WithTestCases(100)); _err != nil {
		panic(_err)
	}
	if !sawNil {
		t.Error("Optional: nil value never appeared")
	}
	if !sawInt {
		t.Error("Optional: integer value never appeared")
	}
}

// TestOptionalNonBasicE2E verifies that Optional with a non-basic element
// works correctly (optionalGenerator handles any inner generator).
func TestOptionalNonBasicE2E(t *testing.T) {
	t.Parallel()

	nonBasic := &mappedGenerator[int, int]{inner: Integers[int](0, 10), fn: func(v int) int { return v }}
	g := Optional[int](nonBasic)
	if _, ok := g.(*optionalGenerator[int]); !ok {
		t.Fatalf("Optional(nonBasic) should return *optionalGenerator[int], got %T", g)
	}
	sawNil := false
	sawVal := false
	if _err := Run(func(s *TestCase) {
		v := g.draw(s)
		if v == nil {
			sawNil = true
		} else {
			sawVal = true
		}
	}, WithTestCases(100)); _err != nil {
		panic(_err)
	}
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

// TestIPAddressesV4Schema verifies that IPAddresses(v4) produces
// {"type":"ip_address", "version": 4}.
func TestIPAddressesV4Schema(t *testing.T) {
	t.Parallel()
	g := IPAddresses().IPv4()
	bg, ok, err := g.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("IPAddresses(v4) should be basic")
	}
	if bg.schema["type"] != "ip_address" {
		t.Errorf("IPAddresses(v4) type: expected ip_address, got %v", bg.schema["type"])
	}
	if bg.schema["version"].(int64) != 4 {
		t.Errorf("IPAddresses(v4) version: expected 4, got %v", bg.schema["version"])
	}
}

// TestIPAddressesV6Schema verifies that IPAddresses(v6) produces
// {"type":"ip_address", "version": 6}.
func TestIPAddressesV6Schema(t *testing.T) {
	t.Parallel()
	g := IPAddresses().IPv6()
	bg, ok, err := g.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("IPAddresses(v6) should be basic")
	}
	if bg.schema["type"] != "ip_address" {
		t.Errorf("IPAddresses(v6) type: expected ip_address, got %v", bg.schema["type"])
	}
	if bg.schema["version"].(int64) != 6 {
		t.Errorf("IPAddresses(v6) version: expected 6, got %v", bg.schema["version"])
	}
}

// TestIPAddressesDefaultIsOneOf verifies that IPAddresses(no version) returns
// a one_of of two ip_address branches, one per IP version.
func TestIPAddressesDefaultIsOneOf(t *testing.T) {
	t.Parallel()
	g := IPAddresses()
	bg, ok, err := g.asBasic()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("IPAddresses() should be basic")
	}
	// Should be a one_of of ipv4 and ipv6
	if bg.schema["type"] != "one_of" {
		t.Fatalf("IPAddresses(default) schema should have type 'one_of'; got %v", bg.schema)
	}
	generators, hasGenerators := bg.schema["generators"]
	if !hasGenerators {
		t.Fatalf("IPAddresses(default) schema should have 'generators' key; got %v", bg.schema)
	}
	schemas, ok := generators.([]any)
	if !ok || len(schemas) != 2 {
		t.Fatalf("IPAddresses(default) generators should have 2 branches, got %v", generators)
	}
	wantVersions := []int64{4, 6}
	for i, s := range schemas {
		m, ok := s.(map[string]any)
		if !ok {
			t.Fatalf("branch %d should be map[string]any, got %T", i, s)
		}
		if m["type"] != "ip_address" {
			t.Errorf("branch %d type: expected ip_address, got %v", i, m["type"])
		}
		if m["version"].(int64) != wantVersions[i] {
			t.Errorf("branch %d version: expected %d, got %v", i, wantVersions[i], m["version"])
		}
	}
}

// TestIPAddressesV4E2E verifies IPv4 addresses contain dots.
func TestIPAddressesV4E2E(t *testing.T) {
	t.Parallel()

	g := IPAddresses().IPv4()
	if _err := Run(func(s *TestCase) {
		v := g.draw(s)
		if !v.Is4() {
			panic(fmt.Sprintf("IPv4 address should be v4: %v", v))
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestIPAddressesV6E2E verifies IPv6 addresses contain colons.
func TestIPAddressesV6E2E(t *testing.T) {
	t.Parallel()

	g := IPAddresses().IPv6()
	if _err := Run(func(s *TestCase) {
		v := g.draw(s)
		if !v.Is6() {
			panic(fmt.Sprintf("IPv6 address should be v6: %v", v))
		}
	}, WithTestCases(50)); _err != nil {
		panic(_err)
	}
}

// TestIPAddressesDefaultE2E verifies default produces both IPv4 and IPv6.
func TestIPAddressesDefaultE2E(t *testing.T) {
	t.Parallel()

	sawV4 := false
	sawV6 := false
	g := IPAddresses()
	if _err := Run(func(s *TestCase) {
		v := g.draw(s)
		if v.Is4() {
			sawV4 = true
		} else if v.Is6() {
			sawV6 = true
		}
	}, WithTestCases(100)); _err != nil {
		panic(_err)
	}
	if !sawV4 {
		t.Error("IPAddresses default: no IPv4 address generated")
	}
	if !sawV6 {
		t.Error("IPAddresses default: no IPv6 address generated")
	}
}

// TestOneOfWithMapMixedTypesE2E verifies that OneOf combining mapped and identity
// generators produces correct values.
func TestOneOfWithMapMixedTypesE2E(t *testing.T) {
	t.Parallel()

	// Integers[int](0,10).Map(*2): always even numbers; Just(int(0)): always 0
	gen := OneOf(
		Map(Integers[int](0, 10), func(v int) int { return v * 2 }),
		Just(int(0)),
	)
	if _err := Run(func(s *TestCase) {
		v := gen.draw(s)
		if v%2 != 0 {
			panic(fmt.Sprintf("OneOf map: expected even, got %d", v))
		}
		if v < 0 || v > 20 {
			panic(fmt.Sprintf("OneOf map: expected [0,20], got %d", v))
		}
	}, WithTestCases(100)); _err != nil {
		panic(_err)
	}
}

// TestOneOfAllBranchesAppear verifies that both branches of OneOf appear
// across enough test cases.
func TestOneOfAllBranchesAppear(t *testing.T) {
	t.Parallel()

	sawA := false
	sawB := false
	gen := OneOf(Text().MinSize(1).MaxSize(3), Text().MinSize(4).MaxSize(6))
	if _err := Run(func(s *TestCase) {
		v := gen.draw(s)
		n := len([]rune(v))
		if n >= 1 && n <= 3 {
			sawA = true
		} else if n >= 4 && n <= 6 {
			sawB = true
		}
	}, WithTestCases(200)); _err != nil {
		panic(_err)
	}
	if !sawA {
		t.Error("OneOf: Text(1,3) branch never appeared")
	}
	if !sawB {
		t.Error("OneOf: Text(4,6) branch never appeared")
	}
}

// TestCompositeOneOfGenerateErrorResponse covers the error path in
// oneOfGenerator.draw when the server sends a requestError in response
// to the index generate command on the composite path.
func TestCompositeOneOfGenerateErrorResponse(t *testing.T) {

	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "error_response")
	// Non-basic branches force the composite draw path.
	nonBasic1 := &mappedGenerator[int64, int64]{inner: Integers[int64](0, 5), fn: func(v int64) int64 { return v }}
	nonBasic2 := &mappedGenerator[int64, int64]{inner: Integers[int64](6, 10), fn: func(v int64) int64 { return v }}
	gen := &oneOfGenerator[int64]{generators: []Generator[int64]{nonBasic1, nonBasic2}}
	err := Run(func(s *TestCase) {
		_ = gen.draw(s) // should panic with requestError
	})
	// error_response makes the test appear interesting (failing).
	_ = err
}
