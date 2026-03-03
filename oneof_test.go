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
	g1 := Integers(0, 10)
	g2 := Integers(20, 30)
	combined := OneOf(g1, g2)

	if !combined.isBasic() {
		t.Fatal("OneOf all-identity-basic should be basic")
	}
	oneOf, hasOneOf := combined.schema["one_of"]
	if !hasOneOf {
		t.Fatalf("OneOf Path 1 schema should have 'one_of' key; got %v", combined.schema)
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
	// transform should be identity for path 1
	if !combined.identityTransform {
		t.Error("OneOf Path 1 should have identityTransform=true")
	}
}

// TestOneOfPath1E2E verifies that OneOf path 1 generates values from both branches.
func TestOneOfPath1E2E(t *testing.T) {
	hegelBinPath(t)
	sawLow := false
	sawHigh := false
	combined := OneOf(Integers(0, 100), Integers(1000, 2000))
	RunHegelTest(t.Name(), func() {
		v := Draw(combined)
		if v <= 100 {
			sawLow = true
		}
		if v >= 1000 {
			sawHigh = true
		}
	}, WithTestCases(100))
	if !sawLow {
		t.Error("OneOf: never generated a low integer (0-100)")
	}
	if !sawHigh {
		t.Error("OneOf: never generated a high integer (1000-2000)")
	}
}

// =============================================================================
// OneOf — Path 2: all basic, some have transforms
// =============================================================================

// TestOneOfPath2Schema verifies that OneOf with mapped BasicGenerators produces
// a tagged-tuple schema.
func TestOneOfPath2Schema(t *testing.T) {
	gen1 := Map(Just(int64(1)), func(v int64) int64 { return v * 2 })
	gen2 := Map(Just(int64(2)), func(v int64) int64 { return v * 3 })
	combined := OneOf(gen1, gen2)

	if !combined.isBasic() {
		t.Fatalf("OneOf Path 2 should be basic")
	}
	oneOf, hasOneOf := combined.schema["one_of"]
	if !hasOneOf {
		t.Fatalf("Path 2 schema should have 'one_of' key; got %v", combined.schema)
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
		constVal, _ := extractInt(constMap["const"])
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

	// Simulate tag=0, value=int64(1) -> transform 0 (*2) -> 2
	result0 := combined.transform([]any{int64(0), int64(1)})
	if result0 != 2 {
		t.Errorf("tag=0: expected 2, got %d", result0)
	}

	// Simulate tag=1, value=int64(2) -> transform 1 (*3) -> 6
	result1 := combined.transform([]any{int64(1), int64(2)})
	if result1 != 6 {
		t.Errorf("tag=1: expected 6, got %d", result1)
	}
}

// TestOneOfPath2TransformNilBranch verifies that when one branch has an identity transform,
// the tagged dispatcher returns the coerced value for that branch.
func TestOneOfPath2TransformNilBranch(t *testing.T) {
	// Mix: one identity branch, one with transform
	gen1 := Integers(0, 10)                                            // identity transform
	gen2 := Map(Just(int64(5)), func(v int64) int64 { return v * 10 }) // has transform
	combined := OneOf(gen1, gen2)

	if !combined.isBasic() {
		t.Fatalf("expected basic generator")
	}
	// tag=0: identity branch — return value coerced to int64
	result0 := combined.transform([]any{int64(0), int64(7)})
	if result0 != 7 {
		t.Errorf("tag=0 (identity): expected 7, got %d", result0)
	}
	// tag=1: mapped branch — return 5*10=50
	result1 := combined.transform([]any{int64(1), int64(5)})
	if result1 != 50 {
		t.Errorf("tag=1 (mapped): expected 50, got %d", result1)
	}
}

// TestOneOfPath2TransformShortTuple verifies graceful handling of malformed tuple.
func TestOneOfPath2TransformShortTuple(t *testing.T) {
	gen1 := Map(Just(int64(1)), func(v int64) int64 { return v })
	gen2 := Map(Just(int64(2)), func(v int64) int64 { return v })
	combined := OneOf(gen1, gen2)
	// Call with fewer-than-2 elements — should return zero value of T
	result := combined.transform([]any{int64(0)})
	if result != 0 {
		t.Errorf("short tuple: expected zero value (0), got %d", result)
	}
}

// TestOneOfPath2E2E verifies that Path 2 generates correctly through the real server.
func TestOneOfPath2E2E(t *testing.T) {
	hegelBinPath(t)
	gen1 := Map(Just(int64(1)), func(v int64) int64 { return v * 2 })
	gen2 := Map(Just(int64(2)), func(v int64) int64 { return v * 3 })
	combined := OneOf(gen1, gen2)

	RunHegelTest(t.Name(), func() {
		v := Draw(combined)
		if v != 2 && v != 6 {
			panic(fmt.Sprintf("OneOf Path2: expected 2 or 6, got %d", v))
		}
	}, WithTestCases(50))
}

// =============================================================================
// OneOf — Path 3: any non-basic generator
// =============================================================================

// TestOneOfPath3IsComposite verifies that OneOf with a non-basic generator
// returns a non-basic generator.
func TestOneOfPath3IsComposite(t *testing.T) {
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	basic := Integers(0, 5)
	combined := OneOf(nonBasic, basic)
	if combined.isBasic() {
		t.Fatal("OneOf with non-basic should not be basic")
	}
}

// TestOneOfPath3MapReturnsMapGen verifies that mapping a non-basic OneOf
// returns a non-basic generator.
func TestOneOfPath3MapReturnsMapGen(t *testing.T) {
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	combined := OneOf(nonBasic, Integers(0, 5))
	mapped := Map(combined, func(v int64) int64 { return v })
	if mapped.isBasic() {
		t.Fatal("Map on non-basic should be non-basic")
	}
}

// TestOneOfPath3E2E verifies that Path 3 generates values from both branches
// using the real hegel binary.
func TestOneOfPath3E2E(t *testing.T) {
	hegelBinPath(t)
	nonBasic := Filter(Integers(0, 1000), func(v int64) bool { return true })
	basic := Integers(-1000, -1)
	combined := OneOf(nonBasic, basic)

	sawPos := false
	sawNeg := false
	RunHegelTest(t.Name(), func() {
		v := Draw(combined)
		if v >= 0 {
			sawPos = true
		}
		if v < 0 {
			sawNeg = true
		}
	}, WithTestCases(100))
	if !sawPos {
		t.Error("OneOf Path3: never generated a non-negative integer")
	}
	if !sawNeg {
		t.Error("OneOf Path3: never generated a negative integer")
	}
}

// TestOneOfPath3UnitFakeServer verifies non-basic OneOf through a fake server.
func TestOneOfPath3UnitFakeServer(t *testing.T) {
	nonBasic := Filter(Integers(0, 100), func(v int64) bool { return true })
	gen := OneOf(nonBasic, Integers(200, 300))

	// server side: handle ONE_OF span start + int generate for index + inner generate (filter path) + stop_span
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(payload)
		m, _ := extractDict(decoded)
		chID, _ := extractInt(m[any("channel_id")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := EncodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
			"is_final":   false,
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// Expect: start_span(ONE_OF), generate(index=0 to pick branch 0),
		// then Filter branch: start_span(FILTER), generate(inner integer), stop_span(FILTER),
		// then stop_span(ONE_OF), mark_complete

		// start_span (ONE_OF)
		ssID1, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID1, nil) //nolint:errcheck
		// generate (index selection: reply with 0 to pick branch 0)
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(0)) //nolint:errcheck
		// start_span (FILTER — from the Filter generator on branch 0)
		ssID2, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID2, nil) //nolint:errcheck
		// generate (inner integer for Filter branch 0)
		innerGenID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(innerGenID, int64(42)) //nolint:errcheck
		// stop_span (FILTER)
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
	err := cli.runTest("composite_oneof_unit", func() {
		got = Draw(gen)
	}, runOptions{testCases: 1})
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

// TestOptionalSchema verifies that Optional(basicGen) produces a basic generator with one_of.
func TestOptionalSchema(t *testing.T) {
	g := Optional(Integers(0, 10))
	if !g.isBasic() {
		t.Fatalf("Optional(basicGen) should be basic")
	}
	if _, hasOneOf := g.schema["one_of"]; !hasOneOf {
		t.Errorf("Optional schema should have 'one_of' key; got %v", g.schema)
	}
}

// TestOptionalE2E verifies that Optional generates both nil and integer values.
func TestOptionalE2E(t *testing.T) {
	hegelBinPath(t)
	sawNil := false
	sawInt := false
	g := Optional(Integers(0, 100))
	RunHegelTest(t.Name(), func() {
		v := Draw(g)
		if v == nil {
			sawNil = true
		} else {
			if *v < 0 || *v > 100 {
				panic(fmt.Sprintf("Optional: expected [0,100], got %d", *v))
			}
			sawInt = true
		}
	}, WithTestCases(100))
	if !sawNil {
		t.Error("Optional: nil value never appeared")
	}
	if !sawInt {
		t.Error("Optional: integer value never appeared")
	}
}

// TestOptionalNonBasicE2E verifies that Optional with a non-basic element
// falls back to Path 3 (non-basic composite).
func TestOptionalNonBasicE2E(t *testing.T) {
	hegelBinPath(t)
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	g := Optional(nonBasic)
	if g.isBasic() {
		t.Fatal("Optional(nonBasic) should not be basic")
	}
	sawNil := false
	sawVal := false
	RunHegelTest(t.Name(), func() {
		v := Draw(g)
		if v == nil {
			sawNil = true
		} else {
			sawVal = true
		}
	}, WithTestCases(100))
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
	if !g.isBasic() {
		t.Fatalf("IPAddresses(v4) should be basic")
	}
	if g.schema["type"] != "ipv4" {
		t.Errorf("IPAddresses(v4) type: expected ipv4, got %v", g.schema["type"])
	}
}

// TestIPAddressesV6Schema verifies that IPAddresses(v6) produces {"type":"ipv6"}.
func TestIPAddressesV6Schema(t *testing.T) {
	g := IPAddresses(IPAddressOptions{Version: IPVersion6})
	if !g.isBasic() {
		t.Fatalf("IPAddresses(v6) should be basic")
	}
	if g.schema["type"] != "ipv6" {
		t.Errorf("IPAddresses(v6) type: expected ipv6, got %v", g.schema["type"])
	}
}

// TestIPAddressesDefaultIsOneOf verifies that IPAddresses(no version) returns a OneOf generator.
func TestIPAddressesDefaultIsOneOf(t *testing.T) {
	g := IPAddresses(IPAddressOptions{})
	if !g.isBasic() {
		t.Fatalf("IPAddresses(default) should be basic")
	}
	// Should be a one_of of ipv4 and ipv6
	oneOf, hasOneOf := g.schema["one_of"]
	if !hasOneOf {
		t.Fatalf("IPAddresses(default) schema should have 'one_of' key; got %v", g.schema)
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
	RunHegelTest(t.Name(), func() {
		s := Draw(g)
		if !strings.Contains(s, ".") {
			panic(fmt.Sprintf("IPv4 address should contain '.': %q", s))
		}
	}, WithTestCases(50))
}

// TestIPAddressesV6E2E verifies IPv6 addresses contain colons.
func TestIPAddressesV6E2E(t *testing.T) {
	hegelBinPath(t)
	g := IPAddresses(IPAddressOptions{Version: IPVersion6})
	RunHegelTest(t.Name(), func() {
		s := Draw(g)
		if !strings.Contains(s, ":") {
			panic(fmt.Sprintf("IPv6 address should contain ':': %q", s))
		}
	}, WithTestCases(50))
}

// TestIPAddressesDefaultE2E verifies default produces both IPv4 and IPv6.
func TestIPAddressesDefaultE2E(t *testing.T) {
	hegelBinPath(t)
	sawV4 := false
	sawV6 := false
	g := IPAddresses(IPAddressOptions{})
	RunHegelTest(t.Name(), func() {
		s := Draw(g)
		if strings.Contains(s, ".") {
			sawV4 = true
		} else if strings.Contains(s, ":") {
			sawV6 = true
		} else {
			panic(fmt.Sprintf("IPAddresses default: unrecognized address format: %q", s))
		}
	}, WithTestCases(100))
	if !sawV4 {
		t.Error("IPAddresses default: no IPv4 address generated")
	}
	if !sawV6 {
		t.Error("IPAddresses default: no IPv6 address generated")
	}
}

// TestOneOfWithMapMixedTypesE2E verifies that OneOf combining mapped and identity
// generators produces correct values of mixed types via Map to any.
func TestOneOfWithMapMixedTypesE2E(t *testing.T) {
	hegelBinPath(t)
	gen := OneOf(
		Map(Integers(0, 10), func(n int64) any { return n * 2 }),
		Map(Just(true), func(v bool) any { return v }),
	)
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		switch val := v.(type) {
		case int64:
			if val%2 != 0 || val < 0 || val > 20 {
				panic(fmt.Sprintf("OneOf map: expected even [0,20], got %d", val))
			}
		case bool:
			if !val {
				panic("OneOf Just(true): expected true")
			}
		default:
			panic(fmt.Sprintf("OneOf mixed: unexpected type %T: %v", v, v))
		}
	}, WithTestCases(100))
}

// TestOneOfAllBranchesAppear verifies that both branches of OneOf appear
// across enough test cases.
func TestOneOfAllBranchesAppear(t *testing.T) {
	hegelBinPath(t)
	sawA := false
	sawB := false
	gen := OneOf(Text(1, 3), Text(4, 6))
	RunHegelTest(t.Name(), func() {
		s := Draw(gen)
		n := len([]rune(s))
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

// TestCompositeOneOfGenerateErrorResponse covers the error path in
// non-basic OneOf when the server sends a RequestError
// in response to the index generate command.
func TestCompositeOneOfGenerateErrorResponse(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "error_response")
	// Use non-basic branches (via Filter) to get Path 3.
	nonBasic1 := Filter(Integers(0, 5), func(v int64) bool { return true })
	nonBasic2 := Filter(Integers(6, 10), func(v int64) bool { return true })
	gen := OneOf(nonBasic1, nonBasic2)
	err := RunHegelTestE(t.Name(), func() {
		_ = Draw(gen) // should panic with RequestError
	})
	// error_response makes the test appear interesting (failing).
	_ = err
}
