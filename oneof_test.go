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
	g2 := Booleans(0.5)
	combined := OneOf(g1, g2)

	bg, ok := combined.(*BasicGenerator)
	if !ok {
		t.Fatalf("OneOf all-identity-basic should return *BasicGenerator, got %T", combined)
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
	// AsBasic() returns itself
	if bg.AsBasic() != bg {
		t.Error("AsBasic should return itself for BasicGenerator")
	}
}

// TestOneOfPath1E2E verifies that OneOf path 1 generates values from both branches.
func TestOneOfPath1E2E(t *testing.T) {
	hegelBinPath(t)
	sawInt := false
	sawBool := false
	combined := OneOf(Integers(0, 1000), Booleans(0.5))
	RunHegelTest(t.Name(), func() {
		v := Draw(combined)
		switch v.(type) {
		case uint64, int64:
			sawInt = true
		case bool:
			sawBool = true
		default:
			panic(fmt.Sprintf("OneOf: unexpected type %T", v))
		}
	}, WithTestCases(100))
	if !sawInt {
		t.Error("OneOf: never generated an integer")
	}
	if !sawBool {
		t.Error("OneOf: never generated a bool")
	}
}

// =============================================================================
// OneOf — Path 2: all basic, some have transforms
// =============================================================================

// TestOneOfPath2Schema verifies that OneOf with mapped BasicGenerators produces
// a tagged-tuple schema.
func TestOneOfPath2Schema(t *testing.T) {
	gen1 := Just(int64(1)).Map(func(v any) any { n, _ := ExtractInt(v); return n * 2 })
	gen2 := Just(int64(2)).Map(func(v any) any { n, _ := ExtractInt(v); return n * 3 })
	combined := OneOf(gen1, gen2)

	bg, ok := combined.(*BasicGenerator)
	if !ok {
		t.Fatalf("OneOf Path 2 should return *BasicGenerator, got %T", combined)
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
		constVal, _ := ExtractInt(constMap["const"])
		if constVal != int64(i) {
			t.Errorf("branch %d: const tag should be %d, got %d", i, i, constVal)
		}
	}
}

// TestOneOfPath2Transform verifies the tagged transform dispatching logic.
func TestOneOfPath2Transform(t *testing.T) {
	// just(1).map(*2) -> always 2; just(2).map(*3) -> always 6
	gen1 := Just(int64(1)).Map(func(v any) any { n, _ := ExtractInt(v); return n * 2 })
	gen2 := Just(int64(2)).Map(func(v any) any { n, _ := ExtractInt(v); return n * 3 })
	combined := OneOf(gen1, gen2)

	bg := combined.(*BasicGenerator)

	// Simulate tag=0, value=int64(1) → transform 0 (*2) → 2
	result0 := bg.transform([]any{int64(0), int64(1)})
	n0, _ := ExtractInt(result0)
	if n0 != 2 {
		t.Errorf("tag=0: expected 2, got %d", n0)
	}

	// Simulate tag=1, value=int64(2) → transform 1 (*3) → 6
	result1 := bg.transform([]any{int64(1), int64(2)})
	n1, _ := ExtractInt(result1)
	if n1 != 6 {
		t.Errorf("tag=1: expected 6, got %d", n1)
	}
}

// TestOneOfPath2TransformNilBranch verifies that when one branch has a nil transform,
// the tagged dispatcher returns the raw value for that branch.
func TestOneOfPath2TransformNilBranch(t *testing.T) {
	// Mix: one identity branch (no transform), one with transform
	gen1 := Integers(0, 10)                                                              // nil transform
	gen2 := Just(int64(5)).Map(func(v any) any { n, _ := ExtractInt(v); return n * 10 }) // has transform
	combined := OneOf(gen1, gen2)

	bg, ok := combined.(*BasicGenerator)
	if !ok {
		t.Fatalf("expected *BasicGenerator, got %T", combined)
	}
	// tag=0: identity branch — return value as-is
	result0 := bg.transform([]any{int64(0), int64(7)})
	n0, _ := ExtractInt(result0)
	if n0 != 7 {
		t.Errorf("tag=0 (identity): expected 7, got %d", n0)
	}
	// tag=1: mapped branch — return 5*10=50
	result1 := bg.transform([]any{int64(1), int64(5)})
	n1, _ := ExtractInt(result1)
	if n1 != 50 {
		t.Errorf("tag=1 (mapped): expected 50, got %d", n1)
	}
}

// TestOneOfPath2TransformShortTuple verifies graceful handling of malformed tuple.
func TestOneOfPath2TransformShortTuple(t *testing.T) {
	gen1 := Just(int64(1)).Map(func(v any) any { return v })
	gen2 := Just(int64(2)).Map(func(v any) any { return v })
	combined := OneOf(gen1, gen2)
	bg := combined.(*BasicGenerator)
	// Call with fewer-than-2 elements — should return tagged as-is
	result := bg.transform([]any{int64(0)})
	elems, ok := result.([]any)
	if !ok || len(elems) != 1 {
		t.Errorf("short tuple: expected original []any{0}, got %v", result)
	}
}

// TestOneOfPath2E2E verifies that Path 2 generates correctly through the real server.
func TestOneOfPath2E2E(t *testing.T) {
	hegelBinPath(t)
	gen1 := Just(int64(1)).Map(func(v any) any { n, _ := ExtractInt(v); return n * 2 })
	gen2 := Just(int64(2)).Map(func(v any) any { n, _ := ExtractInt(v); return n * 3 })
	combined := OneOf(gen1, gen2)

	RunHegelTest(t.Name(), func() {
		v := Draw(combined)
		n, _ := ExtractInt(v)
		if n != 2 && n != 6 {
			panic(fmt.Sprintf("OneOf Path2: expected 2 or 6, got %d", n))
		}
	}, WithTestCases(50))
}

// =============================================================================
// OneOf — Path 3: any non-basic generator
// =============================================================================

// TestOneOfPath3IsComposite verifies that OneOf with a non-basic generator
// returns a compositeOneOfGenerator.
func TestOneOfPath3IsComposite(t *testing.T) {
	// A filteredGenerator (from Filter) is not a BasicGenerator.
	filtered := Integers(0, 10).Map(func(v any) any { return v }) // still basic
	// mappedGenerator of mappedGenerator is still not basic if it's non-basic…
	// Actually just use a mappedGenerator wrapping mappedGenerator
	nonBasic := &mappedGenerator{
		inner: Integers(0, 10),
		fn:    func(v any) any { return v },
	}
	combined := OneOf(nonBasic, filtered)
	if _, ok := combined.(*compositeOneOfGenerator); !ok {
		t.Fatalf("OneOf with non-basic should return *compositeOneOfGenerator, got %T", combined)
	}
	if combined.AsBasic() != nil {
		t.Error("compositeOneOfGenerator.AsBasic() should return nil")
	}
}

// TestOneOfPath3MapReturnsMapGen verifies that mapping a compositeOneOfGenerator
// returns a mappedGenerator.
func TestOneOfPath3MapReturnsMapGen(t *testing.T) {
	nonBasic := &mappedGenerator{inner: Integers(0, 10), fn: func(v any) any { return v }}
	combined := OneOf(nonBasic, Integers(0, 5))
	mapped := combined.Map(func(v any) any { return v })
	if _, ok := mapped.(*mappedGenerator); !ok {
		t.Fatalf("compositeOneOfGenerator.Map should return *mappedGenerator, got %T", mapped)
	}
}

// TestOneOfPath3E2E verifies that Path 3 generates values from both branches
// using the real hegel binary.
func TestOneOfPath3E2E(t *testing.T) {
	hegelBinPath(t)
	// nonBasic: a mappedGenerator (not a *BasicGenerator)
	nonBasic := &mappedGenerator{
		inner: Integers(0, 1000),
		fn:    func(v any) any { return v }, // identity, but still a mappedGenerator
	}
	text := Text(1, 5)
	combined := OneOf(nonBasic, text)

	sawInt := false
	sawStr := false
	RunHegelTest(t.Name(), func() {
		v := Draw(combined)
		switch v.(type) {
		case uint64, int64:
			sawInt = true
		case string:
			sawStr = true
		default:
			panic(fmt.Sprintf("OneOf Path3: unexpected type %T", v))
		}
	}, WithTestCases(100))
	if !sawInt {
		t.Error("OneOf Path3: never generated an integer")
	}
	if !sawStr {
		t.Error("OneOf Path3: never generated a string")
	}
}

// TestOneOfPath3UnitFakeServer verifies the compositeOneOfGenerator through a fake server.
func TestOneOfPath3UnitFakeServer(t *testing.T) {
	nonBasic := &mappedGenerator{inner: Integers(0, 100), fn: func(v any) any { return v }}
	gen := &compositeOneOfGenerator{generators: []Generator{nonBasic, Booleans(0.5)}}

	// server side: handle ONE_OF span start + int generate for index + inner generate + stop_span
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel_id")])
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

		// Expect: start_span(LabelOneOf), start_span(LabelMapped), generate(index=0),
		// generate(inner), stop_span(mapped), stop_span(oneof), mark_complete
		// start_span (ONE_OF)
		ssID1, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID1, nil) //nolint:errcheck
		// start_span (MAPPED inner)
		ssID2, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID2, nil) //nolint:errcheck
		// generate (index selection: reply with 0 to pick branch 0)
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(0)) //nolint:errcheck
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
	err := cli.runTest("composite_oneof_unit", func() {
		v := Draw(gen)
		got, _ = ExtractInt(v)
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

// TestOptionalSchema verifies that Optional(basicGen) follows OneOf Path 1 or 2.
func TestOptionalSchema(t *testing.T) {
	// Optional(basicGen with no transform) → Path 2 because Just has a transform.
	// just(nil) has a transform (always returns nil), so we expect Path 2 (tagged tuples).
	g := Optional(Integers(0, 10))
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("Optional(basicGen) should return *BasicGenerator, got %T", g)
	}
	if _, hasOneOf := bg.schema["one_of"]; !hasOneOf {
		t.Errorf("Optional schema should have 'one_of' key; got %v", bg.schema)
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
			_, ok1 := v.(int64)
			_, ok2 := v.(uint64)
			if !ok1 && !ok2 {
				panic(fmt.Sprintf("Optional: expected nil or int, got %T: %v", v, v))
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
// falls back to Path 3 (compositeOneOfGenerator).
func TestOptionalNonBasicE2E(t *testing.T) {
	hegelBinPath(t)
	// Just(nil) is basic (has transform), so the pair Just(nil)+mappedGenerator
	// would be mixed: Just(nil).AsBasic() != nil, mappedGenerator.AsBasic() == nil
	// → Path 3
	nonBasic := &mappedGenerator{inner: Integers(0, 10), fn: func(v any) any { return v }}
	g := Optional(nonBasic)
	if _, ok := g.(*compositeOneOfGenerator); !ok {
		t.Fatalf("Optional(nonBasic) should return *compositeOneOfGenerator, got %T", g)
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
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("IPAddresses(v4) should return *BasicGenerator, got %T", g)
	}
	if bg.schema["type"] != "ipv4" {
		t.Errorf("IPAddresses(v4) type: expected ipv4, got %v", bg.schema["type"])
	}
}

// TestIPAddressesV6Schema verifies that IPAddresses(v6) produces {"type":"ipv6"}.
func TestIPAddressesV6Schema(t *testing.T) {
	g := IPAddresses(IPAddressOptions{Version: IPVersion6})
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("IPAddresses(v6) should return *BasicGenerator, got %T", g)
	}
	if bg.schema["type"] != "ipv6" {
		t.Errorf("IPAddresses(v6) type: expected ipv6, got %v", bg.schema["type"])
	}
}

// TestIPAddressesDefaultIsOneOf verifies that IPAddresses(no version) returns a OneOf generator.
func TestIPAddressesDefaultIsOneOf(t *testing.T) {
	g := IPAddresses(IPAddressOptions{})
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("IPAddresses(default) should return *BasicGenerator, got %T", g)
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
	RunHegelTest(t.Name(), func() {
		v := Draw(g)
		s, ok := v.(string)
		if !ok {
			panic(fmt.Sprintf("IPAddresses v4: expected string, got %T", v))
		}
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
		v := Draw(g)
		s, ok := v.(string)
		if !ok {
			panic(fmt.Sprintf("IPAddresses v6: expected string, got %T", v))
		}
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
		v := Draw(g)
		s, ok := v.(string)
		if !ok {
			panic(fmt.Sprintf("IPAddresses default: expected string, got %T", v))
		}
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
// generators produces correct values of mixed types.
func TestOneOfWithMapMixedTypesE2E(t *testing.T) {
	hegelBinPath(t)
	// Integers(0,10).Map(*2): always even numbers; just(true): always true
	gen := OneOf(
		Integers(0, 10).Map(func(v any) any { n, _ := ExtractInt(v); return n * 2 }),
		Just(true),
	)
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		switch val := v.(type) {
		case int64:
			if val%2 != 0 || val < 0 || val > 20 {
				panic(fmt.Sprintf("OneOf map: expected even [0,20], got %d", val))
			}
		case uint64:
			if val%2 != 0 || val > 20 {
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
		v := Draw(gen)
		s, ok := v.(string)
		if !ok {
			panic(fmt.Sprintf("OneOf text branches: expected string, got %T", v))
		}
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
// compositeOneOfGenerator.Generate when the server sends a RequestError
// in response to the index generate command.
func TestCompositeOneOfGenerateErrorResponse(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "error_response")
	// Use a compositeOneOfGenerator (non-basic branches → Path 3).
	nonBasic1 := &mappedGenerator{inner: Integers(0, 5), fn: func(v any) any { return v }}
	nonBasic2 := &mappedGenerator{inner: Integers(6, 10), fn: func(v any) any { return v }}
	gen := &compositeOneOfGenerator{generators: []Generator{nonBasic1, nonBasic2}}
	err := RunHegelTestE(t.Name(), func() {
		_ = Draw(gen) // should panic with RequestError
	})
	// error_response makes the test appear interesting (failing).
	_ = err
}
