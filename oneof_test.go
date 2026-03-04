package hegel

// oneof_test.go tests the OneOf, Optional, and IPAddresses generators.

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// =============================================================================
// OneOf — Path 1: all basic, all identity transforms
// =============================================================================

// TestOneOfPath1Schema verifies that OneOf with all-identity-transform basic
// generators produces a simple {"one_of": [...]} schema.
func TestOneOfPath1Schema(t *testing.T) {
	g1 := Booleans(0.5)
	g2 := Booleans(0.3)
	combined := OneOf(g1, g2)

	bg, ok := combined.(*basicGenerator[bool])
	if !ok {
		t.Fatalf("OneOf all-identity-basic should return *basicGenerator[bool], got %T", combined)
	}
	oneOf, hasOneOf := bg.schema["one_of"]
	if !hasOneOf {
		t.Fatalf("OneOf Path 1 schema should have 'one_of' key; got %v", bg.schema)
	}
	schemas, ok := oneOf.([]any)
	if !ok {
		t.Fatalf("one_of value should be []any, got %T", oneOf)
	}
	if len(schemas) != 2 {
		t.Errorf("one_of should have 2 branches, got %d", len(schemas))
	}
	// No tagged tuples
	for i, s := range schemas {
		m, ok := s.(map[string]any)
		if !ok {
			t.Errorf("branch %d should be map[string]any, got %T", i, s)
			continue
		}
		if _, hasTupleType := m["type"]; hasTupleType && m["type"] == "tuple" {
			t.Errorf("branch %d should not be a tagged tuple in path 1", i)
		}
	}
	// transform must be nil for path 1
	if bg.transform != nil {
		t.Error("OneOf Path 1 should have nil transform")
	}
}

// TestOneOfPath1E2E verifies that OneOf path 1 generates values from both branches.
func TestOneOfPath1E2E(t *testing.T) {
	hegelBinPath(t)
	sawShort := false
	sawLong := false
	combined := OneOf(Text(1, 3), Text(10, 15))
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := combined.draw(s)
		n := len([]rune(v))
		if n >= 1 && n <= 3 {
			sawShort = true
		} else if n >= 10 && n <= 15 {
			sawLong = true
		}
	}, stderrNoteFn, []Option{WithTestCases(100)}); _err != nil {
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
// OneOf — Path 2: all basic, some have transforms
// =============================================================================

// TestOneOfPath2Schema verifies that OneOf with mapped basicGenerators produces
// a tagged-tuple schema.
func TestOneOfPath2Schema(t *testing.T) {
	gen1 := Map(Just(int64(1)), func(v int64) int64 { return v * 2 })
	gen2 := Map(Just(int64(2)), func(v int64) int64 { return v * 3 })
	combined := OneOf(gen1, gen2)

	bg, ok := combined.(*basicGenerator[int64])
	if !ok {
		t.Fatalf("OneOf Path 2 should return *basicGenerator[int64], got %T", combined)
	}
	oneOf, hasOneOf := bg.schema["one_of"]
	if !hasOneOf {
		t.Fatalf("Path 2 schema should have 'one_of' key; got %v", bg.schema)
	}
	schemas, ok := oneOf.([]any)
	if !ok {
		t.Fatalf("one_of value should be []any")
	}
	if len(schemas) != 2 {
		t.Errorf("one_of should have 2 tagged branches, got %d", len(schemas))
	}
	// Each branch should be a tuple with {"type": "tuple", "elements": [{"const": i}, ...]}
	for i, s := range schemas {
		m, ok := s.(map[string]any)
		if !ok {
			t.Errorf("branch %d should be map[string]any, got %T", i, s)
			continue
		}
		if m["type"] != "tuple" {
			t.Errorf("branch %d: expected type=tuple, got %v", i, m["type"])
		}
		elems, ok := m["elements"].([]any)
		if !ok || len(elems) < 2 {
			t.Errorf("branch %d: elements should be []any with >=2 entries", i)
			continue
		}
		constMap, ok := elems[0].(map[string]any)
		if !ok {
			t.Errorf("branch %d: first element should be {const: N}", i)
			continue
		}
		constVal, _ := extractCBORInt(constMap["const"])
		if constVal != int64(i) {
			t.Errorf("branch %d: const tag should be %d, got %d", i, i, constVal)
		}
	}
}

// TestOneOfPath2Transform verifies the tagged transform dispatching logic.
func TestOneOfPath2Transform(t *testing.T) {
	// just(1).map(*2) -> always 2; just(2).map(*3) -> always 6
	gen1 := Map(Just(int64(1)), func(v int64) int64 { return v * 2 })
	gen2 := Map(Just(int64(2)), func(v int64) int64 { return v * 3 })
	combined := OneOf(gen1, gen2)

	bg := combined.(*basicGenerator[int64])

	// Simulate tag=0, value=int64(1) → transform 0 (*2) → 2
	result0 := bg.transform([]any{int64(0), int64(1)})
	if result0 != 2 {
		t.Errorf("tag=0: expected 2, got %d", result0)
	}

	// Simulate tag=1, value=int64(2) → transform 1 (*3) → 6
	result1 := bg.transform([]any{int64(1), int64(2)})
	if result1 != 6 {
		t.Errorf("tag=1: expected 6, got %d", result1)
	}
}

// TestOneOfPath2TransformNilBranch verifies that when one branch has a nil transform,
// the tagged dispatcher returns the raw value for that branch.
func TestOneOfPath2TransformNilBranch(t *testing.T) {
	// Mix: one identity branch (no transform), one with transform.
	// Booleans has no transform (identity), Just(true) has a transform.
	gen1 := Booleans(0.5)                                    // nil transform
	gen2 := Map(Just(true), func(v bool) bool { return !v }) // has transform (negate)
	combined := OneOf(gen1, gen2)

	bg, ok := combined.(*basicGenerator[bool])
	if !ok {
		t.Fatalf("expected *basicGenerator[bool], got %T", combined)
	}
	// tag=0: identity branch — return value as-is
	result0 := bg.transform([]any{int64(0), true})
	if result0 != true {
		t.Errorf("tag=0 (identity): expected true, got %v", result0)
	}
	// tag=1: mapped branch — negate true → false
	result1 := bg.transform([]any{int64(1), true})
	if result1 != false {
		t.Errorf("tag=1 (mapped): expected false, got %v", result1)
	}
}

// TestOneOfPath2TransformShortTuple verifies graceful handling of malformed tuple.
func TestOneOfPath2TransformShortTuple(t *testing.T) {
	gen1 := Map(Just(int64(1)), func(v int64) int64 { return v })
	gen2 := Map(Just(int64(2)), func(v int64) int64 { return v })
	combined := OneOf(gen1, gen2)
	bg := combined.(*basicGenerator[int64])
	// Call with fewer-than-2 elements — should return tagged as-is via .(T) cast.
	// The short tuple path does tagged.(T), which will be []any{int64(0)}.
	// Since []any is not int64, this will panic. We verify the panic.
	defer func() {
		if r := recover(); r == nil {
			t.Error("short tuple: expected panic from type assertion")
		}
	}()
	_ = bg.transform([]any{int64(0)})
}

// TestOneOfPath2E2E verifies that Path 2 generates correctly through the real server.
func TestOneOfPath2E2E(t *testing.T) {
	hegelBinPath(t)
	gen1 := Map(Just(int64(1)), func(v int64) int64 { return v * 2 })
	gen2 := Map(Just(int64(2)), func(v int64) int64 { return v * 3 })
	combined := OneOf(gen1, gen2)

	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := combined.draw(s)
		if v != 2 && v != 6 {
			panic(fmt.Sprintf("OneOf Path2: expected 2 or 6, got %d", v))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// OneOf — Path 3: any non-basic generator
// =============================================================================

// TestOneOfPath3IsComposite verifies that OneOf with a non-basic generator
// returns a compositeOneOfGenerator.
func TestOneOfPath3IsComposite(t *testing.T) {
	// A mappedGenerator is not a basicGenerator.
	nonBasic := &mappedGenerator[int64, int64]{
		inner: Integers(0, 10),
		fn:    func(v int64) int64 { return v },
	}
	basic := Integers(0, 10)
	combined := OneOf[int64](nonBasic, basic)
	if _, ok := combined.(*compositeOneOfGenerator[int64]); !ok {
		t.Fatalf("OneOf with non-basic should return *compositeOneOfGenerator[int64], got %T", combined)
	}
}

// TestOneOfPath3MapReturnsMapGen verifies that mapping a compositeOneOfGenerator
// returns a mappedGenerator.
func TestOneOfPath3MapReturnsMapGen(t *testing.T) {
	nonBasic := &mappedGenerator[int64, int64]{inner: Integers(0, 10), fn: func(v int64) int64 { return v }}
	combined := OneOf[int64](nonBasic, Integers(0, 5))
	mapped := Map(combined, func(v int64) int64 { return v })
	if _, ok := mapped.(*mappedGenerator[int64, int64]); !ok {
		t.Fatalf("Map(compositeOneOfGenerator) should return *mappedGenerator, got %T", mapped)
	}
}

// TestOneOfPath3E2E verifies that Path 3 generates values from both branches
// using the real hegel binary.
func TestOneOfPath3E2E(t *testing.T) {
	hegelBinPath(t)
	// nonBasic: a mappedGenerator (not a *basicGenerator)
	nonBasic := &mappedGenerator[int64, int64]{
		inner: Integers(0, 1000),
		fn:    func(v int64) int64 { return v }, // identity, but still a mappedGenerator
	}
	text := Text(1, 5)
	// These have different types so we need to unify. Use any.
	nonBasicAny := Map[int64, any](nonBasic, func(v int64) any { return v })
	textAny := Map[string, any](text, func(v string) any { return v })
	combined := OneOf[any](nonBasicAny, textAny)

	sawInt := false
	sawStr := false
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := combined.draw(s)
		switch v.(type) {
		case int64:
			sawInt = true
		case string:
			sawStr = true
		default:
			panic(fmt.Sprintf("OneOf Path3: unexpected type %T", v))
		}
	}, stderrNoteFn, []Option{WithTestCases(100)}); _err != nil {
		panic(_err)
	}
	if !sawInt {
		t.Error("OneOf Path3: never generated an integer")
	}
	if !sawStr {
		t.Error("OneOf Path3: never generated a string")
	}
}

// TestOneOfPath3UnitFakeServer verifies the compositeOneOfGenerator through a fake server.
func TestOneOfPath3UnitFakeServer(t *testing.T) {
	nonBasic := &mappedGenerator[int64, int64]{inner: Integers(0, 100), fn: func(v int64) int64 { return v }}
	gen := &compositeOneOfGenerator[int64]{generators: []Generator[int64]{nonBasic, Integers(0, 5)}}

	// server side: handle ONE_OF span start + int generate for index + inner generate + stop_span
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chID, _ := extractCBORInt(m[any("channel_id")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
			"is_final":   false,
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// Expect: start_span(ONE_OF), start_span(MAPPED), generate(index=0),
		// generate(inner), stop_span(MAPPED), stop_span(ONE_OF), mark_complete
		// start_span (ONE_OF)
		ssID1, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID1, nil) //nolint:errcheck
		// generate (index selection: reply with 0 to pick branch 0)
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(0)) //nolint:errcheck
		// start_span (MAPPED inner)
		ssID2, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID2, nil) //nolint:errcheck
		// generate (inner integer for mappedGenerator branch 0)
		innerGenID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(innerGenID, int64(42)) //nolint:errcheck
		// stop_span (MAPPED)
		spID2, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(spID2, nil) //nolint:errcheck
		// stop_span (ONE_OF)
		spID1, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(spID1, nil) //nolint:errcheck
		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var got int64
	err := cli.runTest("composite_oneof_unit", func(s *TestCase) {
		got = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

// =============================================================================
// OneOf — requires at least 2 generators
// =============================================================================

// TestOneOfPanicsWithFewerThanTwo verifies that OneOf panics when < 2 generators.
func TestOneOfPanicsWithFewerThanTwo(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("OneOf with 1 generator should panic")
		}
	}()
	OneOf(Integers(0, 10))
}

// =============================================================================
// Optional
// =============================================================================

// TestOptionalSchema verifies that Optional returns an optionalGenerator.
func TestOptionalSchema(t *testing.T) {
	g := Optional(Integers(0, 10))
	if _, ok := g.(*optionalGenerator[int64]); !ok {
		t.Fatalf("Optional(Integers) should return *optionalGenerator[int64], got %T", g)
	}
}

// TestOptionalE2E verifies that Optional generates both nil and integer values.
func TestOptionalE2E(t *testing.T) {
	hegelBinPath(t)
	sawNil := false
	sawInt := false
	g := Optional(Integers(0, 100))
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := g.draw(s)
		if v == nil {
			sawNil = true
		} else {
			sawInt = true
			if *v < 0 || *v > 100 {
				panic(fmt.Sprintf("Optional: expected [0,100], got %d", *v))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(100)}); _err != nil {
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
	hegelBinPath(t)
	nonBasic := &mappedGenerator[int64, int64]{inner: Integers(0, 10), fn: func(v int64) int64 { return v }}
	g := Optional[int64](nonBasic)
	if _, ok := g.(*optionalGenerator[int64]); !ok {
		t.Fatalf("Optional(nonBasic) should return *optionalGenerator[int64], got %T", g)
	}
	sawNil := false
	sawVal := false
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := g.draw(s)
		if v == nil {
			sawNil = true
		} else {
			sawVal = true
		}
	}, stderrNoteFn, []Option{WithTestCases(100)}); _err != nil {
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

// TestIPAddressesV4Schema verifies that IPAddresses(v4) produces {"type":"ipv4"}.
func TestIPAddressesV4Schema(t *testing.T) {
	g := IPAddresses(IPAddressOptions{Version: IPVersion4})
	bg, ok := g.(*basicGenerator[string])
	if !ok {
		t.Fatalf("IPAddresses(v4) should return *basicGenerator[string], got %T", g)
	}
	if bg.schema["type"] != "ipv4" {
		t.Errorf("IPAddresses(v4) type: expected ipv4, got %v", bg.schema["type"])
	}
}

// TestIPAddressesV6Schema verifies that IPAddresses(v6) produces {"type":"ipv6"}.
func TestIPAddressesV6Schema(t *testing.T) {
	g := IPAddresses(IPAddressOptions{Version: IPVersion6})
	bg, ok := g.(*basicGenerator[string])
	if !ok {
		t.Fatalf("IPAddresses(v6) should return *basicGenerator[string], got %T", g)
	}
	if bg.schema["type"] != "ipv6" {
		t.Errorf("IPAddresses(v6) type: expected ipv6, got %v", bg.schema["type"])
	}
}

// TestIPAddressesDefaultIsOneOf verifies that IPAddresses(no version) returns a OneOf generator.
func TestIPAddressesDefaultIsOneOf(t *testing.T) {
	g := IPAddresses(IPAddressOptions{})
	bg, ok := g.(*basicGenerator[string])
	if !ok {
		t.Fatalf("IPAddresses(default) should return *basicGenerator[string], got %T", g)
	}
	// Should be a one_of of ipv4 and ipv6
	oneOf, hasOneOf := bg.schema["one_of"]
	if !hasOneOf {
		t.Fatalf("IPAddresses(default) schema should have 'one_of' key; got %v", bg.schema)
	}
	schemas, ok := oneOf.([]any)
	if !ok || len(schemas) != 2 {
		t.Fatalf("IPAddresses(default) one_of should have 2 branches, got %v", oneOf)
	}
}

// TestIPAddressesV4E2E verifies IPv4 addresses contain dots.
func TestIPAddressesV4E2E(t *testing.T) {
	hegelBinPath(t)
	g := IPAddresses(IPAddressOptions{Version: IPVersion4})
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := g.draw(s)
		if !strings.Contains(v, ".") {
			panic(fmt.Sprintf("IPv4 address should contain '.': %q", v))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestIPAddressesV6E2E verifies IPv6 addresses contain colons.
func TestIPAddressesV6E2E(t *testing.T) {
	hegelBinPath(t)
	g := IPAddresses(IPAddressOptions{Version: IPVersion6})
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := g.draw(s)
		if !strings.Contains(v, ":") {
			panic(fmt.Sprintf("IPv6 address should contain ':': %q", v))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestIPAddressesDefaultE2E verifies default produces both IPv4 and IPv6.
func TestIPAddressesDefaultE2E(t *testing.T) {
	hegelBinPath(t)
	sawV4 := false
	sawV6 := false
	g := IPAddresses(IPAddressOptions{})
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := g.draw(s)
		if strings.Contains(v, ".") {
			sawV4 = true
		} else if strings.Contains(v, ":") {
			sawV6 = true
		} else {
			panic(fmt.Sprintf("IPAddresses default: unrecognized address format: %q", v))
		}
	}, stderrNoteFn, []Option{WithTestCases(100)}); _err != nil {
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
	hegelBinPath(t)
	// Integers(0,10).Map(*2): always even numbers; Just(int64(0)): always 0
	gen := OneOf(
		Map(Integers(0, 10), func(v int64) int64 { return v * 2 }),
		Just(int64(0)),
	)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := gen.draw(s)
		if v%2 != 0 {
			panic(fmt.Sprintf("OneOf map: expected even, got %d", v))
		}
		if v < 0 || v > 20 {
			panic(fmt.Sprintf("OneOf map: expected [0,20], got %d", v))
		}
	}, stderrNoteFn, []Option{WithTestCases(100)}); _err != nil {
		panic(_err)
	}
}

// TestOneOfAllBranchesAppear verifies that both branches of OneOf appear
// across enough test cases.
func TestOneOfAllBranchesAppear(t *testing.T) {
	hegelBinPath(t)
	sawA := false
	sawB := false
	gen := OneOf(Text(1, 3), Text(4, 6))
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := gen.draw(s)
		n := len([]rune(v))
		if n >= 1 && n <= 3 {
			sawA = true
		} else if n >= 4 && n <= 6 {
			sawB = true
		}
	}, stderrNoteFn, []Option{WithTestCases(200)}); _err != nil {
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
// compositeOneOfGenerator.draw when the server sends a requestError
// in response to the index generate command.
func TestCompositeOneOfGenerateErrorResponse(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "error_response")
	// Use a compositeOneOfGenerator (non-basic branches -> Path 3).
	nonBasic1 := &mappedGenerator[int64, int64]{inner: Integers(0, 5), fn: func(v int64) int64 { return v }}
	nonBasic2 := &mappedGenerator[int64, int64]{inner: Integers(6, 10), fn: func(v int64) int64 { return v }}
	gen := &compositeOneOfGenerator[int64]{generators: []Generator[int64]{nonBasic1, nonBasic2}}
	err := runHegel(t.Name(), func(s *TestCase) {
		_ = gen.draw(s) // should panic with requestError
	}, stderrNoteFn, nil)
	// error_response makes the test appear interesting (failing).
	_ = err
}
