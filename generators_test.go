package hegel

import (
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// Generator[T] and basic generator tests
// =============================================================================

// --- Basic generator: generate with identity transform ---

func TestBasicGeneratorGenerateNoTransform(t *testing.T) {
	// Set up fake server that responds to a generate command with int64(42).
	schema := map[string]any{"type": "integer", "min_value": int64(0), "max_value": int64(100)}
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(payload)
		m, _ := extractDict(decoded)
		chID, _ := extractInt(m[any("channel_id")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		// Send one test_case.
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := EncodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
			"is_final":   false,
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// Respond to generate with 42.
		genID, genPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		_, _ = genPayload, DecodeCBOR           // consumed
		caseCh.SendReplyValue(genID, int64(42)) //nolint:errcheck

		// Wait for mark_complete.
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		// Send test_done (passed, no interesting).
		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotVal int64
	err := cli.runTest("basic_gen_no_transform", func() {
		g := newBasicGenerator(schema, toInt64, true)
		gotVal = Draw(g)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 42 {
		t.Errorf("expected 42, got %d", gotVal)
	}
}

// --- Basic generator: generate with user transform ---

func TestBasicGeneratorGenerateWithTransform(t *testing.T) {
	schema := map[string]any{"type": "integer"}
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

		// Respond to generate with 7.
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(7)) //nolint:errcheck

		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotVal int64
	err := cli.runTest("basic_gen_with_transform", func() {
		// transform: multiply by 2
		g := newBasicGenerator(schema, func(v any) int64 { n, _ := extractInt(v); return n * 2 }, false)
		gotVal = Draw(g)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 14 {
		t.Errorf("expected 14, got %d", gotVal)
	}
}

// --- Map free function: basic generator with identity transform ---

func TestBasicGeneratorMapNoTransform(t *testing.T) {
	g := Booleans(0.5)
	mapped := Map(g, func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	})
	// Map on basic generator returns another basic generator with same schema.
	if !mapped.isBasic() {
		t.Fatal("Map on basic generator should return a basic generator")
	}
	if mapped.schema["type"] != "boolean" {
		t.Errorf("schema not preserved by Map")
	}
	if mapped.identityTransform {
		t.Error("identityTransform should be false after Map")
	}
}

// --- Map free function: compose transforms ---

func TestBasicGeneratorMapComposesTransforms(t *testing.T) {
	g := newBasicGenerator(
		map[string]any{"type": "integer"},
		func(v any) int64 { n, _ := extractInt(v); return n + 1 },
		false,
	)
	// Map again: result should be (n+1)*2
	mapped := Map(g, func(v int64) int64 {
		return v * 2
	})
	if !mapped.isBasic() {
		t.Fatal("double Map should return a basic generator")
	}
	// Simulate applying: start with int64(5) → +1 → 6 → *2 → 12
	result := mapped.transform(int64(5))
	if result != 12 {
		t.Errorf("composed transform: expected 12, got %d", result)
	}
}

// --- isBasic ---

func TestBasicGeneratorIsBasic(t *testing.T) {
	g := newBasicGenerator(map[string]any{"type": "boolean"}, toBool, true)
	if !g.isBasic() {
		t.Error("isBasic should return true for a basic generator")
	}
}

// --- Map on non-basic generator (composite path) ---

func TestMappedGeneratorGenerate(t *testing.T) {
	// Map on a non-basic generator wraps it in a MAPPED span.
	schema := map[string]any{"type": "integer"}
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

		// Map(Filter(basic, true), fn) protocol:
		// start_span(MAPPED), start_span(FILTER), generate, stop_span(FILTER), stop_span(MAPPED), mark_complete
		for i := 0; i < 6; i++ {
			mid, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := DecodeCBOR(nil) // not needed, just reply
			_ = dec
			// respond to generate with int64(3); everything else gets nil
			if i == 2 {
				caseCh.SendReplyValue(mid, int64(3)) //nolint:errcheck
			} else {
				caseCh.SendReplyValue(mid, nil) //nolint:errcheck
			}
		}

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotVal int64
	err := cli.runTest("mapped_gen", func() {
		inner := newBasicGenerator(schema, toInt64, true)
		// Make it non-basic by filtering, then Map on the non-basic generator.
		nonBasic := Filter(inner, func(v int64) bool { return true })
		mg := Map(nonBasic, func(v int64) int64 { return v * 10 })
		gotVal = Draw(mg)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 30 {
		t.Errorf("expected 30, got %d", gotVal)
	}
}

// --- Non-basic generator is not basic ---

func TestMappedGeneratorIsNotBasic(t *testing.T) {
	inner := newBasicGenerator(map[string]any{"type": "integer"}, toInt64, true)
	nonBasic := Filter(inner, func(v int64) bool { return true })
	mg := Map(nonBasic, func(v int64) int64 { return v })
	if mg.isBasic() {
		t.Error("Map on non-basic generator should not be basic")
	}
}

// --- Map on basic generator returns basic generator ---

func TestMapOnBasicInnerReturnsBasic(t *testing.T) {
	inner := newBasicGenerator(map[string]any{"type": "integer"}, toInt64, true)
	result := Map(inner, func(v int64) int64 { return v })
	if !result.isBasic() {
		t.Error("Map on basic generator should return a basic generator")
	}
}

// =============================================================================
// Span helper tests
// =============================================================================

func fakeTestEnv(t *testing.T, fn func(caseCh *channel)) *connection {
	t.Helper()
	return fakeServerConn(t, func(serverConn *connection) {
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

		fn(caseCh)

		// Wait for mark_complete.
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})
}

// --- collection.more: cached false after first false ---

func TestCollectionMoreCachesFalse(t *testing.T) {
	// After more() returns false, subsequent calls return false without network calls.
	c := &collection{finished: true}
	if c.more(nil) {
		t.Error("expected false from finished collection")
	}
}

// =============================================================================
// Label constants
// =============================================================================

func TestLabelConstants(t *testing.T) {
	cases := []struct {
		name string
		val  SpanLabel
		want int
	}{
		{"List", LabelList, 1},
		{"ListElement", LabelListElement, 2},
		{"Set", LabelSet, 3},
		{"SetElement", LabelSetElement, 4},
		{"Map", LabelMap, 5},
		{"MapEntry", LabelMapEntry, 6},
		{"Tuple", LabelTuple, 7},
		{"OneOf", LabelOneOf, 8},
		{"Optional", LabelOptional, 9},
		{"FixedDict", LabelFixedDict, 10},
		{"FlatMap", LabelFlatMap, 11},
		{"Filter", LabelFilter, 12},
		{"Mapped", LabelMapped, 13},
		{"SampledFrom", LabelSampledFrom, 14},
		{"EnumVariant", LabelEnumVariant, 15},
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
	hegelBinPath(t)
	var vals []int64
	RunHegelTest("integers_happy", func() {
		v := Draw(Integers(0, 100))
		vals = append(vals, v)
		if v < 0 || v > 100 {
			panic(fmt.Sprintf("out of range: %d", v))
		}
	}, WithTestCases(10))
	if len(vals) == 0 {
		t.Error("test function was never called")
	}
}

// --- Integers: schema is correct ---

func TestIntegersSchema(t *testing.T) {
	g := Integers(-5, 5)
	if !g.isBasic() {
		t.Fatal("Integers should return a basic generator")
	}
	min, _ := extractInt(g.schema["min_value"])
	max, _ := extractInt(g.schema["max_value"])
	if min != -5 {
		t.Errorf("min_value: expected -5, got %d", min)
	}
	if max != 5 {
		t.Errorf("max_value: expected 5, got %d", max)
	}
	if g.schema["type"] != "integer" {
		t.Errorf("type: expected integer, got %v", g.schema["type"])
	}
}

// --- Integers: nil min/max omitted from schema ---

func TestIntegersNoBounds(t *testing.T) {
	g := IntegersUnbounded()
	if !g.isBasic() {
		t.Fatal("IntegersUnbounded should return a basic generator")
	}
	if _, hasMin := g.schema["min_value"]; hasMin {
		t.Error("min_value should not be present when no min bound given")
	}
	if _, hasMax := g.schema["max_value"]; hasMax {
		t.Error("max_value should not be present when no max bound given")
	}
}

// =============================================================================
// StopTest in collection protocol — error injection via HEGEL_PROTOCOL_TEST_MODE
// =============================================================================

// The stop_test_on_collection_more and stop_test_on_new_collection modes are
// now tested above via the real hegel binary. The skips have been removed since
// Stage 5 implements the collection protocol.

// Verify that after skip removal, the runner_test.go skips are gone.
// (These are integration tests that run against the real binary.)

// =============================================================================
// Just generator tests
// =============================================================================

// TestJustSchema verifies that Just produces a schema with "const" key.
func TestJustSchema(t *testing.T) {
	g := Just(42)
	if _, hasConst := g.schema["const"]; !hasConst {
		t.Error("Just schema should have 'const' key")
	}
	// The const value in schema should be nil (null)
	if g.schema["const"] != nil {
		t.Errorf("Just schema 'const' should be nil, got %v", g.schema["const"])
	}
}

// TestJustTransformIgnoresInput verifies that Just always returns the constant value.
func TestJustTransformIgnoresInput(t *testing.T) {
	g := Just("hello")
	// transform should ignore the server value and always return "hello"
	result := g.transform(nil)
	if result != "hello" {
		t.Errorf("Just transform: expected 'hello', got %v", result)
	}
	result = g.transform(int64(999))
	if result != "hello" {
		t.Errorf("Just transform with non-nil input: expected 'hello', got %v", result)
	}
}

// TestJustE2E verifies that Just always generates the constant value against the real server.
func TestJustE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		v := Draw(Just(42))
		if v != 42 {
			panic(fmt.Sprintf("Just: expected 42, got %v", v))
		}
	}, WithTestCases(20))
}

// TestJustNonPrimitive verifies that Just works with non-primitive values (pointer identity).
func TestJustNonPrimitive(t *testing.T) {
	hegelBinPath(t)
	type myStruct struct{ x int }
	val := &myStruct{x: 99}
	RunHegelTest(t.Name(), func() {
		v := Draw(Just(val))
		if v != val {
			panic("Just: pointer identity not preserved")
		}
	}, WithTestCases(10))
}

// =============================================================================
// SampledFrom generator tests
// =============================================================================

// TestSampledFromEmptyError verifies that SampledFrom returns an error for empty slice.
func TestSampledFromEmptyError(t *testing.T) {
	_, err := SampledFrom([]string{})
	if err == nil {
		t.Fatal("SampledFrom([]): expected error, got nil")
	}
	if err.Error() != "sampled_from requires at least one element" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestMustSampledFromEmptyPanics verifies MustSampledFrom panics with empty slice.
func TestMustSampledFromEmptyPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustSampledFrom([]) should panic")
		}
	}()
	MustSampledFrom([]string{})
}

// TestSampledFromSchema verifies that SampledFrom produces an integer schema with correct bounds.
func TestSampledFromSchema(t *testing.T) {
	g, err := SampledFrom([]string{"a", "b", "c"})
	if err != nil {
		t.Fatalf("SampledFrom: unexpected error: %v", err)
	}
	if g.schema["type"] != "integer" {
		t.Errorf("schema type: expected 'integer', got %v", g.schema["type"])
	}
	minVal, _ := extractInt(g.schema["min_value"])
	maxVal, _ := extractInt(g.schema["max_value"])
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
	items := []string{"x", "y", "z"}
	g, err := SampledFrom(items)
	if err != nil {
		t.Fatalf("SampledFrom: unexpected error: %v", err)
	}
	// Index 0 → "x", 1 → "y", 2 → "z"
	for i, want := range items {
		got := g.transform(uint64(i))
		if got != want {
			t.Errorf("transform(%d): expected %v, got %v", i, want, got)
		}
	}
}

// TestSampledFromSingleElement verifies that a single-element slice always returns that element.
func TestSampledFromSingleElement(t *testing.T) {
	hegelBinPath(t)
	g, _ := SampledFrom([]string{"only"})
	RunHegelTest(t.Name(), func() {
		v := Draw(g)
		if v != "only" {
			panic(fmt.Sprintf("SampledFrom single: expected 'only', got %v", v))
		}
	}, WithTestCases(20))
}

// TestSampledFromE2E verifies that SampledFrom only returns elements from the list
// and that all elements appear (with enough test cases).
func TestSampledFromE2E(t *testing.T) {
	hegelBinPath(t)
	choices := []string{"apple", "banana", "cherry"}
	g, _ := SampledFrom(choices)
	seen := map[string]bool{}
	RunHegelTest(t.Name(), func() {
		v := Draw(g)
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
	}, WithTestCases(100))
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
	hegelBinPath(t)
	type myStruct struct{ x int }
	obj1 := &myStruct{x: 1}
	obj2 := &myStruct{x: 2}
	g, _ := SampledFrom([]*myStruct{obj1, obj2})
	RunHegelTest(t.Name(), func() {
		v := Draw(g)
		if v != obj1 && v != obj2 {
			panic("SampledFrom: value is not one of the original pointers")
		}
	}, WithTestCases(10))
}

// =============================================================================
// FromRegex generator tests
// =============================================================================

// TestFromRegexSchema verifies that FromRegex produces the correct schema.
func TestFromRegexSchema(t *testing.T) {
	g := FromRegex(`\d+`, true)
	if g.schema["type"] != "regex" {
		t.Errorf("schema type: expected 'regex', got %v", g.schema["type"])
	}
	if g.schema["pattern"] != `\d+` {
		t.Errorf("pattern: expected '\\d+', got %v", g.schema["pattern"])
	}
	if g.schema["fullmatch"] != true {
		t.Errorf("fullmatch: expected true, got %v", g.schema["fullmatch"])
	}
}

// TestFromRegexFullmatchFalse verifies that fullmatch=false is stored correctly.
func TestFromRegexFullmatchFalse(t *testing.T) {
	g := FromRegex(`abc`, false)
	if g.schema["fullmatch"] != false {
		t.Errorf("fullmatch: expected false, got %v", g.schema["fullmatch"])
	}
}

// TestFromRegexE2E verifies that FromRegex generates strings that match the pattern.
func TestFromRegexE2E(t *testing.T) {
	hegelBinPath(t)
	// Only digits, 1-5 chars
	g := FromRegex(`[0-9]{1,5}`, true)
	RunHegelTest(t.Name(), func() {
		s := Draw(g)
		if len(s) == 0 || len(s) > 5 {
			panic(fmt.Sprintf("FromRegex: length out of range: %q", s))
		}
		for _, ch := range s {
			if ch < '0' || ch > '9' {
				panic(fmt.Sprintf("FromRegex: non-digit character %q in %q", ch, s))
			}
		}
	}, WithTestCases(50))
}

// =============================================================================
// Basic generator error path
// =============================================================================

// TestBasicGeneratorGenerateErrorResponse covers the error path in
// basic generator's drawFn when generateFromSchema returns a non-StopTest error.
func TestBasicGeneratorGenerateErrorResponse(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "error_response")
	err := RunHegelTestE(t.Name(), func() {
		g := newBasicGenerator(map[string]any{"type": "integer"}, toInt64, true)
		_ = Draw(g) // should panic with RequestError → caught as INTERESTING
	})
	// error_response causes the test to appear interesting (failing).
	_ = err
}

// =============================================================================
// Map on non-basic generator returns non-basic
// =============================================================================

func TestGeneratorMapOnNonBasic(t *testing.T) {
	inner := Integers(0, 10)
	// Make it non-basic by filtering.
	nonBasic := Filter(inner, func(v int64) bool { return true })
	mapped := Map(nonBasic, func(v int64) int64 { return v })
	// Mapping a non-basic generator should produce a non-basic generator.
	if mapped.isBasic() {
		t.Error("Map on non-basic Generator should return a non-basic generator")
	}
}

// =============================================================================
// MustSampledFrom happy-path test
// =============================================================================

// TestMustSampledFromHappyPath verifies that MustSampledFrom returns a valid generator
// when given a non-empty slice.
func TestMustSampledFromHappyPath(t *testing.T) {
	g := MustSampledFrom([]string{"only"})
	if g == nil {
		t.Fatal("MustSampledFrom should return non-nil generator")
	}
	result := g.transform(uint64(0))
	if result != "only" {
		t.Errorf("MustSampledFrom transform(0): expected 'only', got %v", result)
	}
}

// =============================================================================
// Map generator E2E tests
// =============================================================================

// TestMapBasicGeneratorE2E verifies that mapping Integers(0,100) by doubling
// always produces even values in [0, 200], and the result is still basic.
func TestMapBasicGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	gen := Map(Integers(0, 100), func(v int64) int64 {
		return v * 2
	})
	// Map on basic generator must preserve basic status.
	if !gen.isBasic() {
		t.Fatal("Map on basic generator should return a basic generator")
	}
	RunHegelTest(t.Name(), func() {
		n := Draw(gen)
		if n%2 != 0 {
			panic(fmt.Sprintf("map(x*2): expected even number, got %d", n))
		}
		if n < 0 || n > 200 {
			panic(fmt.Sprintf("map(x*2): expected [0,200], got %d", n))
		}
	}, WithTestCases(50))
}

// TestMapChainedBasicGeneratorE2E verifies that chaining two maps on a basic generator
// preserves the basic status and composes the transforms correctly.
// Map(Map(Integers(0,100), x+1), x*2): result must be even, in [2, 202].
func TestMapChainedBasicGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	gen := Map(
		Map(Integers(0, 100), func(v int64) int64 {
			return v + 1
		}),
		func(v int64) int64 {
			return v * 2
		},
	)
	// Both chained maps should still be basic (schema preserved).
	if !gen.isBasic() {
		t.Fatal("chained Map on basic generator should return a basic generator")
	}
	RunHegelTest(t.Name(), func() {
		n := Draw(gen)
		// (x+1)*2 is always even. x in [0,100] → result in [2, 202].
		if n%2 != 0 {
			panic(fmt.Sprintf("map(x+1).map(x*2): expected even, got %d", n))
		}
		if n < 2 || n > 202 {
			panic(fmt.Sprintf("map(x+1).map(x*2): expected [2,202], got %d", n))
		}
	}, WithTestCases(50))
}

// TestMapNonBasicGeneratorE2E verifies that mapping a non-basic generator
// wraps it in a MAPPED span and applies the transform correctly.
// The result must be non-basic.
func TestMapNonBasicGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	// Create a non-basic generator by filtering.
	inner := Integers(1, 5)
	nonBasic := Filter(inner, func(v int64) bool { return true })
	gen := Map(nonBasic, func(v int64) int64 {
		return v * 3
	})
	if gen.isBasic() {
		t.Fatal("Map on non-basic Generator should return a non-basic generator")
	}
	RunHegelTest(t.Name(), func() {
		n := Draw(gen)
		// inner is Integers(1,5), map(*3): result is in {3, 6, 9, 12, 15}
		if n < 3 || n > 15 || n%3 != 0 {
			panic(fmt.Sprintf("map(*3) on [1,5]: expected multiple of 3 in [3,15], got %d", n))
		}
	}, WithTestCases(50))
}

// TestMapSchemaPreservedUnit verifies unit-level schema properties of Map on basic generator.
func TestMapSchemaPreservedUnit(t *testing.T) {
	base := Integers(0, 100)
	mapped := Map(base, func(v int64) int64 { return v })
	if !mapped.isBasic() {
		t.Fatal("Map on basic generator should return a basic generator")
	}
	if mapped.schema["type"] != "integer" {
		t.Errorf("schema type: expected 'integer', got %v", mapped.schema["type"])
	}
	if mapped.identityTransform {
		t.Error("identityTransform should be false after Map")
	}
	// Map on basic generator must preserve min/max bounds in the schema.
	minV, _ := extractInt(mapped.schema["min_value"])
	maxV, _ := extractInt(mapped.schema["max_value"])
	if minV != 0 {
		t.Errorf("min_value: expected 0, got %d", minV)
	}
	if maxV != 100 {
		t.Errorf("max_value: expected 100, got %d", maxV)
	}

	// Double Map on basic generator: schema still preserved, transforms compose correctly.
	doubled := Map(
		Map(base, func(v int64) int64 { return v + 10 }),
		func(v int64) int64 { return v * 2 },
	)
	if !doubled.isBasic() {
		t.Fatal("double Map on basic generator should return a basic generator")
	}
	if doubled.schema["type"] != "integer" {
		t.Errorf("double map schema type: expected 'integer', got %v", doubled.schema["type"])
	}
	// Verify composition: input 5 → +10 → 15 → *2 → 30.
	result := doubled.transform(int64(5))
	if result != 30 {
		t.Errorf("double map compose: input 5, expected 30, got %d", result)
	}

	// Map on non-basic: isBasic() returns false.
	nonBasic := Filter(base, func(v int64) bool { return true })
	mappedNB := Map(nonBasic, func(v int64) int64 { return v })
	if mappedNB.isBasic() {
		t.Error("mapping a non-basic generator should produce non-basic result")
	}
}

// =============================================================================
// Tuple generator tests
// =============================================================================

// TestTuples2AllBasicNoTransform verifies that Tuples2 of two basic (identity-transform)
// generators returns a basic generator with schema type=tuple and two elements.
func TestTuples2AllBasicNoTransform(t *testing.T) {
	g1 := Integers(0, 10)
	g2 := Booleans(0.5)
	gen := Tuples2(g1, g2)
	if !gen.isBasic() {
		t.Fatal("Tuples2 of basic generators should return a basic generator")
	}
	if gen.schema["type"] != "tuple" {
		t.Errorf("schema type: expected 'tuple', got %v", gen.schema["type"])
	}
	elements, ok := gen.schema["elements"].([]any)
	if !ok {
		t.Fatalf("schema 'elements' should be []any, got %T", gen.schema["elements"])
	}
	if len(elements) != 2 {
		t.Errorf("expected 2 elements, got %d", len(elements))
	}
	// Tuples always have a transform (to construct Tuple2 struct).
	if gen.transform == nil {
		t.Error("transform should not be nil for Tuples2")
	}
}

// TestTuples2AllBasicWithTransforms verifies that Tuples2 of mapped basic generators
// returns a basic generator with the raw schemas (not transformed), and the combined
// transform applies per-position transforms.
func TestTuples2AllBasicWithTransforms(t *testing.T) {
	g1 := Map(Integers(0, 10), func(v int64) int64 {
		return v * 2
	})
	g2 := Map(Just(5), func(v int) int {
		return v + 1
	})
	// Both g1 and g2 are basic with transforms.
	if !g1.isBasic() {
		t.Fatal("g1 should be basic")
	}
	if !g2.isBasic() {
		t.Fatal("g2 should be basic")
	}
	gen := Tuples2(g1, g2)
	if !gen.isBasic() {
		t.Fatal("Tuples2 of mapped basic generators should return a basic generator")
	}
	if gen.schema["type"] != "tuple" {
		t.Errorf("schema type: expected 'tuple', got %v", gen.schema["type"])
	}
	if gen.transform == nil {
		t.Error("transform should not be nil when elements have transforms")
	}
	// Verify per-position transform: input [4, nil] → Tuple2{A: 4*2=8, B: 5+1=6}
	// (g2 = Map(Just(5), x+1): raw value from server is nil (const), transform of Just gives 5,
	// then +1 gives 6)
	raw := []any{uint64(4), nil}
	result := gen.transform(raw)
	// position 0: 4*2=8
	if result.A != 8 {
		t.Errorf("position 0: expected 8, got %d", result.A)
	}
	// position 1: Just(5).transform(nil)=5, then +1=6
	if result.B != 6 {
		t.Errorf("position 1: expected 6, got %d", result.B)
	}
}

// TestTuples2MixedBasicNonBasic verifies that Tuples2 with a non-basic element
// returns a non-basic generator.
func TestTuples2MixedBasicNonBasic(t *testing.T) {
	// Create a non-basic generator by filtering.
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	g2 := Booleans(0.5)
	gen := Tuples2(nonBasic, g2)
	if gen.isBasic() {
		t.Fatal("Tuples2 with non-basic element should return a non-basic generator")
	}
}

// TestTuples3AllBasic verifies that Tuples3 of all-basic generators returns a basic generator
// with schema type=tuple and three elements.
func TestTuples3AllBasic(t *testing.T) {
	gen := Tuples3(Integers(0, 5), Booleans(0.5), Text(1, 5))
	if !gen.isBasic() {
		t.Fatal("Tuples3 of basic generators should return a basic generator")
	}
	if gen.schema["type"] != "tuple" {
		t.Errorf("schema type: expected 'tuple', got %v", gen.schema["type"])
	}
	elements, ok := gen.schema["elements"].([]any)
	if !ok {
		t.Fatalf("schema 'elements' should be []any, got %T", gen.schema["elements"])
	}
	if len(elements) != 3 {
		t.Errorf("expected 3 elements, got %d", len(elements))
	}
}

// TestTuples3WithNonBasic verifies that Tuples3 falls back to non-basic
// when any element is non-basic.
func TestTuples3WithNonBasic(t *testing.T) {
	nonBasic := Filter(Integers(0, 5), func(v int64) bool { return true })
	gen := Tuples3(nonBasic, Booleans(0.5), Text(1, 5))
	if gen.isBasic() {
		t.Fatal("Tuples3 with non-basic should return a non-basic generator")
	}
}

// TestTuples4AllBasic verifies that Tuples4 of all-basic generators returns a basic generator
// with schema type=tuple and four elements.
func TestTuples4AllBasic(t *testing.T) {
	gen := Tuples4(Integers(0, 5), Booleans(0.5), Text(1, 5), IntegersUnbounded())
	if !gen.isBasic() {
		t.Fatal("Tuples4 of basic generators should return a basic generator")
	}
	elements, ok := gen.schema["elements"].([]any)
	if !ok {
		t.Fatalf("schema 'elements' should be []any, got %T", gen.schema["elements"])
	}
	if len(elements) != 4 {
		t.Errorf("expected 4 elements, got %d", len(elements))
	}
}

// TestTuples4WithNonBasic verifies that Tuples4 falls back to non-basic
// when any element is non-basic.
func TestTuples4WithNonBasic(t *testing.T) {
	nonBasic := Filter(Integers(0, 5), func(v int64) bool { return true })
	gen := Tuples4(Integers(0, 5), Booleans(0.5), nonBasic, Text(1, 5))
	if gen.isBasic() {
		t.Fatal("Tuples4 with non-basic should return a non-basic generator")
	}
}

// TestCompositeTupleGeneratorMap verifies that Map on a non-basic tuple generator returns
// a non-basic generator.
func TestCompositeTupleGeneratorMap(t *testing.T) {
	nonBasic := Filter(Integers(0, 5), func(v int64) bool { return true })
	comp := Tuples2(nonBasic, Booleans(0.5))
	mapped := Map(comp, func(v Tuple2[int64, bool]) Tuple2[int64, bool] { return v })
	if mapped.isBasic() {
		t.Fatal("Map on non-basic tuple generator should return a non-basic generator")
	}
}

// TestTuples2AllBasicNoTransformE2E runs Tuples2(Integers, Booleans) against the real server.
func TestTuples2AllBasicNoTransformE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		gen := Tuples2(Integers(0, 10), Booleans(0.5))
		result := Draw(gen)
		if result.A < 0 || result.A > 10 {
			panic(fmt.Sprintf("Tuples2.A: out of range [0,10]: %d", result.A))
		}
		// result.B is a bool — just verify it compiles (bool type is enforced at compile time)
		_ = result.B
	}, WithTestCases(50))
}

// TestTuples2WithTransformsE2E runs Tuples2 with mapped elements against the real server.
// Property: position A is always even; position B is always 6.
func TestTuples2WithTransformsE2E(t *testing.T) {
	hegelBinPath(t)
	g1 := Map(Integers(0, 10), func(v int64) int64 {
		return v * 2
	})
	g2 := Map(Just(5), func(v int) int {
		return v + 1
	})
	gen := Tuples2(g1, g2)
	RunHegelTest(t.Name(), func() {
		result := Draw(gen)
		if result.A%2 != 0 || result.A < 0 || result.A > 20 {
			panic(fmt.Sprintf("Tuples2 mapped.A: expected even in [0,20], got %d", result.A))
		}
		if result.B != 6 {
			panic(fmt.Sprintf("Tuples2 mapped.B: expected 6, got %d", result.B))
		}
	}, WithTestCases(50))
}

// TestTuples3E2E runs Tuples3(Text, Integers, Floats) against the real server.
func TestTuples3E2E(t *testing.T) {
	hegelBinPath(t)
	falseBool := false
	gen := Tuples3(Text(1, 5), Integers(0, 5), Floats(floatPtr(0.0), floatPtr(1.0), &falseBool, &falseBool, false, false))
	RunHegelTest(t.Name(), func() {
		result := Draw(gen)
		if len(result.A) == 0 {
			panic("Tuples3.A: expected non-empty string")
		}
		if result.B < 0 || result.B > 5 {
			panic(fmt.Sprintf("Tuples3.B: expected [0,5], got %d", result.B))
		}
		if result.C < 0.0 || result.C > 1.0 {
			panic(fmt.Sprintf("Tuples3.C: expected [0,1], got %v", result.C))
		}
	}, WithTestCases(50))
}

// TestTuples2NonBasicE2E runs a non-basic tuple generator against the real server.
// Uses a filtered generator so the first element is always generated via span protocol.
func TestTuples2NonBasicE2E(t *testing.T) {
	hegelBinPath(t)
	// Make non-basic by filtering, then Map to add 100.
	nonBasic := Map(
		Filter(Integers(0, 10), func(v int64) bool { return true }),
		func(v int64) int64 { return v + 100 },
	)
	gen := Tuples2(nonBasic, Booleans(0.5))
	RunHegelTest(t.Name(), func() {
		result := Draw(gen)
		if result.A < 100 || result.A > 110 {
			panic(fmt.Sprintf("Tuples2 non-basic.A: expected [100,110], got %d", result.A))
		}
		// result.B is a bool — type-safe at compile time
		_ = result.B
	}, WithTestCases(50))
}

// TestTuples4E2E runs Tuples4 of all-basic generators against the real server.
func TestTuples4E2E(t *testing.T) {
	hegelBinPath(t)
	gen := Tuples4(Integers(0, 5), Booleans(0.5), Text(1, 3), Integers(10, 20))
	RunHegelTest(t.Name(), func() {
		result := Draw(gen)
		if result.A < 0 || result.A > 5 {
			panic(fmt.Sprintf("Tuples4.A: out of range [0,5]: %d", result.A))
		}
		_ = result.B // bool
		if len(result.C) == 0 {
			panic("Tuples4.C: expected non-empty string")
		}
		if result.D < 10 || result.D > 20 {
			panic(fmt.Sprintf("Tuples4.D: out of range [10,20]: %d", result.D))
		}
	}, WithTestCases(50))
}

// TestTuples2BasicOneTransformOneIdentity verifies that when one element has a user transform
// and the other has an identity transform, the tuple transform applies both correctly.
func TestTuples2BasicOneTransformOneIdentity(t *testing.T) {
	// g1: basic generator with user transform (doubles)
	// g2: basic generator with identity transform
	g1 := Map(Integers(0, 10), func(v int64) int64 {
		return v * 2
	})
	g2 := Integers(0, 5) // identity transform
	gen := Tuples2(g1, g2)
	if !gen.isBasic() {
		t.Fatal("expected basic generator")
	}
	if gen.transform == nil {
		t.Fatal("transform should not be nil (g1 has a user transform)")
	}
	// Apply: raw[0]=3 → 3*2=6; raw[1]=uint64(2) → toInt64=2
	raw := []any{uint64(3), uint64(2)}
	result := gen.transform(raw)
	if result.A != 6 {
		t.Errorf("position A: expected 6, got %d", result.A)
	}
	if result.B != 2 {
		t.Errorf("position B: expected 2, got %d", result.B)
	}
}

// =============================================================================
// Primitive generator schema unit tests
// =============================================================================

// =============================================================================
// filteredGenerator tests
// =============================================================================

// TestFilteredGeneratorIsNotBasic verifies that Filter returns a non-basic generator.
func TestFilteredGeneratorIsNotBasic(t *testing.T) {
	g := Filter(Integers(0, 10), func(v int64) bool { return true })
	if g.isBasic() {
		t.Error("filtered generator should not be basic")
	}
}

// TestFilteredGeneratorFromBasicIsNotBasic verifies that Filter on a basic generator
// returns a non-basic generator.
func TestFilteredGeneratorFromBasicIsNotBasic(t *testing.T) {
	g := Filter(Integers(0, 100), func(v int64) bool { return true })
	if g.isBasic() {
		t.Fatal("Filter on basic generator should return a non-basic generator")
	}
}

// TestFilteredGeneratorFilterChaining verifies that calling Filter on a filtered generator
// returns another non-basic generator.
func TestFilteredGeneratorFilterChaining(t *testing.T) {
	g := Filter(
		Filter(Integers(0, 100), func(v int64) bool { return true }),
		func(v int64) bool { return true },
	)
	if g.isBasic() {
		t.Fatal("chained Filter should return a non-basic generator")
	}
}

// TestFilteredGeneratorMapMethod verifies that calling Map on a filtered generator
// returns a non-basic generator.
func TestFilteredGeneratorMapMethod(t *testing.T) {
	g := Filter(Integers(0, 100), func(v int64) bool { return true })
	mapped := Map(g, func(v int64) int64 { return v })
	if mapped.isBasic() {
		t.Fatal("Map on filtered generator should return a non-basic generator")
	}
}

// TestFilteredGeneratorPredicatePasses verifies that when the predicate passes on the
// first attempt, the value is returned immediately (only one FILTER span pair sent).
func TestFilteredGeneratorPredicatePasses(t *testing.T) {
	var gotVal int64
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

		// start_span
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck
		// generate
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(42)) //nolint:errcheck
		// stop_span (discard=false)
		spID, spPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded2, _ := DecodeCBOR(spPayload)
		m2, _ := extractDict(decoded2)
		discard, _ := m2[any("discard")].(bool)
		if discard {
			t.Error("stop_span should have discard=false when predicate passes")
		}
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck
		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("filter_passes", func() {
		g := Filter(
			newBasicGenerator(map[string]any{"type": "integer"}, toInt64, true),
			func(v int64) bool { return true },
		)
		gotVal = Draw(g)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 42 {
		t.Errorf("expected 42, got %d", gotVal)
	}
}

// TestFilteredGeneratorAllAttemptsFailRejectsCase verifies that when all 3 attempts fail,
// exactly 3 start_span/generate/stop_span(discard=true) cycles are sent, then
// Assume(false) causes the case to be sent mark_complete with status="INVALID".
func TestFilteredGeneratorAllAttemptsFailRejectsCase(t *testing.T) {
	var spanCount int
	var mcStatus string
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

		// 3 failed attempts: each produces start_span, generate, stop_span(discard=true)
		for i := 0; i < maxFilterAttempts; i++ {
			// start_span
			ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
			caseCh.SendReplyValue(ssID, nil) //nolint:errcheck
			// generate
			genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
			caseCh.SendReplyValue(genID, int64(0)) //nolint:errcheck
			// stop_span
			spID, spPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
			decoded2, _ := DecodeCBOR(spPayload)
			m2, _ := extractDict(decoded2)
			discard, _ := m2[any("discard")].(bool)
			if !discard {
				t.Errorf("attempt %d: stop_span should have discard=true when predicate fails", i)
			}
			caseCh.SendReplyValue(spID, nil) //nolint:errcheck
			spanCount++
		}

		// Assume(false) panics with assumeRejected → runner sends mark_complete with "INVALID".
		mcID, mcPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded3, _ := DecodeCBOR(mcPayload)
		m3, _ := extractDict(decoded3)
		mcStatus, _ = extractString(m3[any("status")])
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("filter_exhaust", func() {
		g := Filter(
			newBasicGenerator(map[string]any{"type": "integer"}, toInt64, true),
			func(v int64) bool { return false }, // always reject
		)
		Draw(g)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if spanCount != maxFilterAttempts {
		t.Errorf("expected %d span pairs, got %d", maxFilterAttempts, spanCount)
	}
	if mcStatus != "INVALID" {
		t.Errorf("mark_complete status: expected 'INVALID', got %q", mcStatus)
	}
}

// TestFilteredGeneratorPartialAttemptsSucceed verifies that when the first 2 attempts
// fail but the 3rd passes, 2 discard spans and 1 keep span are sent.
func TestFilteredGeneratorPartialAttemptsSucceed(t *testing.T) {
	attemptNum := 0
	var gotVal int64

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

		// 2 failed attempts (returns 0), then 1 successful (returns 7)
		for i := 0; i < 3; i++ {
			ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
			caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

			genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
			val := int64(0)
			if i == 2 {
				val = int64(7)
			}
			caseCh.SendReplyValue(genID, val) //nolint:errcheck

			spID, spPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
			decoded2, _ := DecodeCBOR(spPayload)
			m2, _ := extractDict(decoded2)
			discard, _ := m2[any("discard")].(bool)
			if i < 2 && !discard {
				t.Errorf("attempt %d: expected discard=true for failed attempt", i)
			}
			if i == 2 && discard {
				t.Errorf("attempt %d: expected discard=false for successful attempt", i)
			}
			caseCh.SendReplyValue(spID, nil) //nolint:errcheck
		}

		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("filter_partial", func() {
		g := Filter(
			newBasicGenerator(map[string]any{"type": "integer"}, toInt64, true),
			func(v int64) bool {
				attemptNum++
				return v > 0
			},
		)
		gotVal = Draw(g)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 7 {
		t.Errorf("expected 7, got %d", gotVal)
	}
	if attemptNum != 3 {
		t.Errorf("expected predicate called 3 times, called %d times", attemptNum)
	}
}

// TestFilteredGeneratorE2EAlwaysPasses verifies an e2e filter with a predicate
// that values greater than 50.
func TestFilteredGeneratorE2EAlwaysPasses(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		gen := Filter(Integers(0, 100), func(v int64) bool {
			return v > 50
		})
		n := Draw(gen)
		if n <= 50 {
			panic(fmt.Sprintf("filter(>50): expected n>50, got %d", n))
		}
	}, WithTestCases(50))
}

// TestFilteredGeneratorE2EEvenNumbers verifies filter for even numbers.
func TestFilteredGeneratorE2EEvenNumbers(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		gen := Filter(Integers(0, 10), func(v int64) bool {
			return v%2 == 0
		})
		n := Draw(gen)
		if n%2 != 0 {
			panic(fmt.Sprintf("filter(even): expected even, got %d", n))
		}
	}, WithTestCases(50))
}

// TestFilterOnNonBasicGenerators verifies that Filter works on non-basic generators.
func TestFilterOnNonBasicGenerators(t *testing.T) {
	// Filter on a mapped non-basic generator
	nonBasic := Filter(Integers(0, 5), func(v int64) bool { return true })
	mg := Map(nonBasic, func(v int64) int64 { return v })
	fg := Filter(mg, func(v int64) bool { return true })
	if fg.isBasic() {
		t.Error("Filter on mapped non-basic generator should return non-basic")
	}
	// Filter on a non-basic list generator
	cl := Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 3})
	fg2 := Filter(cl, func(v []int64) bool { return true })
	if fg2.isBasic() {
		t.Error("Filter on non-basic list generator should return non-basic")
	}
	// Filter on a non-basic dict generator
	cd := Dicts(nonBasic, Integers(0, 5), DictOptions{MinSize: 0})
	fg3 := Filter(cd, func(v map[int64]int64) bool { return true })
	if fg3.isBasic() {
		t.Error("Filter on non-basic dict generator should return non-basic")
	}
	// Filter on a non-basic oneOf generator
	co := OneOf(nonBasic, Filter(Integers(6, 10), func(v int64) bool { return true }))
	fg4 := Filter(co, func(v int64) bool { return true })
	if fg4.isBasic() {
		t.Error("Filter on non-basic oneOf generator should return non-basic")
	}
	// Filter on a non-basic tuple generator
	ct := Tuples2(nonBasic, Integers(0, 5))
	fg5 := Filter(ct, func(v Tuple2[int64, int64]) bool { return true })
	if fg5.isBasic() {
		t.Error("Filter on non-basic tuple generator should return non-basic")
	}
	// Filter on a FlatMap generator
	fm := FlatMap(Integers(0, 5), func(v int64) *Generator[int64] { return Integers(0, 5) })
	fg6 := Filter(fm, func(v int64) bool { return true })
	if fg6.isBasic() {
		t.Error("Filter on FlatMap generator should return non-basic")
	}
}

// TestBooleansSchema verifies that Booleans produces a schema with type=boolean and p field.
func TestBooleansSchema(t *testing.T) {
	g := Booleans(0.5)
	if !g.isBasic() {
		t.Fatal("Booleans should return a basic generator")
	}
	if g.schema["type"] != "boolean" {
		t.Errorf("type: expected 'boolean', got %v", g.schema["type"])
	}
	p, ok := g.schema["p"].(float64)
	if !ok {
		t.Fatalf("p field should be float64, got %T", g.schema["p"])
	}
	if p != 0.5 {
		t.Errorf("p: expected 0.5, got %v", p)
	}
}

// TestBooleansP1Schema verifies that Booleans(1.0) stores p=1.0.
func TestBooleansP1Schema(t *testing.T) {
	g := Booleans(1.0)
	if g.schema["p"] != 1.0 {
		t.Errorf("p: expected 1.0, got %v", g.schema["p"])
	}
}

// TestTextSchema verifies that Text produces the correct schema structure.
func TestTextSchema(t *testing.T) {
	g := Text(3, 10)
	if !g.isBasic() {
		t.Fatal("Text should return a basic generator")
	}
	if g.schema["type"] != "string" {
		t.Errorf("type: expected 'string', got %v", g.schema["type"])
	}
	minSize, _ := extractInt(g.schema["min_size"])
	if minSize != 3 {
		t.Errorf("min_size: expected 3, got %d", minSize)
	}
	maxSize, _ := extractInt(g.schema["max_size"])
	if maxSize != 10 {
		t.Errorf("max_size: expected 10, got %d", maxSize)
	}
	// Identity transform (no user transform).
	if !g.identityTransform {
		t.Error("Text should have identity transform")
	}
}

// TestTextSchemaNoMax verifies that Text with maxSize<0 omits max_size from schema.
func TestTextSchemaNoMax(t *testing.T) {
	g := Text(0, -1)
	if _, hasMax := g.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when maxSize < 0")
	}
	minSize, _ := extractInt(g.schema["min_size"])
	if minSize != 0 {
		t.Errorf("min_size: expected 0, got %d", minSize)
	}
}

// TestBinarySchema verifies that Binary produces the correct schema structure.
func TestBinarySchema(t *testing.T) {
	g := Binary(1, 20)
	if !g.isBasic() {
		t.Fatal("Binary should return a basic generator")
	}
	if g.schema["type"] != "binary" {
		t.Errorf("type: expected 'binary', got %v", g.schema["type"])
	}
	minSize, _ := extractInt(g.schema["min_size"])
	if minSize != 1 {
		t.Errorf("min_size: expected 1, got %d", minSize)
	}
	maxSize, _ := extractInt(g.schema["max_size"])
	if maxSize != 20 {
		t.Errorf("max_size: expected 20, got %d", maxSize)
	}
	// Identity transform (no user transform).
	if !g.identityTransform {
		t.Error("Binary should have identity transform")
	}
}

// TestBinarySchemaNoMax verifies that Binary with maxSize<0 omits max_size from schema.
func TestBinarySchemaNoMax(t *testing.T) {
	g := Binary(0, -1)
	if _, hasMax := g.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when maxSize < 0")
	}
}

// TestIntegersFromSchema verifies that IntegersFrom produces the correct schema.
func TestIntegersFromSchema(t *testing.T) {
	minV := int64(-10)
	maxV := int64(10)
	g := IntegersFrom(&minV, &maxV)
	if !g.isBasic() {
		t.Fatal("IntegersFrom should return a basic generator")
	}
	if g.schema["type"] != "integer" {
		t.Errorf("type: expected 'integer', got %v", g.schema["type"])
	}
	minVal, _ := extractInt(g.schema["min_value"])
	maxVal, _ := extractInt(g.schema["max_value"])
	if minVal != -10 {
		t.Errorf("min_value: expected -10, got %d", minVal)
	}
	if maxVal != 10 {
		t.Errorf("max_value: expected 10, got %d", maxVal)
	}
}

// TestIntegersFromSchemaOnlyMin verifies that IntegersFrom with only a min bound omits max_value.
func TestIntegersFromSchemaOnlyMin(t *testing.T) {
	minV := int64(5)
	g := IntegersFrom(&minV, nil)
	if _, hasMax := g.schema["max_value"]; hasMax {
		t.Error("max_value should not be present when maxVal is nil")
	}
	minVal, _ := extractInt(g.schema["min_value"])
	if minVal != 5 {
		t.Errorf("min_value: expected 5, got %d", minVal)
	}
}

// TestIntegersFromSchemaOnlyMax verifies that IntegersFrom with only a max bound omits min_value.
func TestIntegersFromSchemaOnlyMax(t *testing.T) {
	maxV := int64(99)
	g := IntegersFrom(nil, &maxV)
	if _, hasMin := g.schema["min_value"]; hasMin {
		t.Error("min_value should not be present when minVal is nil")
	}
	maxVal, _ := extractInt(g.schema["max_value"])
	if maxVal != 99 {
		t.Errorf("max_value: expected 99, got %d", maxVal)
	}
}

// TestFloatsSchemaWithBounds verifies that Floats with explicit bounds sets all schema fields.
func TestFloatsSchemaWithBounds(t *testing.T) {
	minV := 0.0
	maxV := 1.0
	falseV := false
	g := Floats(&minV, &maxV, &falseV, &falseV, false, false)
	if !g.isBasic() {
		t.Fatal("Floats should return a basic generator")
	}
	if g.schema["type"] != "float" {
		t.Errorf("type: expected 'float', got %v", g.schema["type"])
	}
	if g.schema["allow_nan"] != false {
		t.Errorf("allow_nan: expected false, got %v", g.schema["allow_nan"])
	}
	if g.schema["allow_infinity"] != false {
		t.Errorf("allow_infinity: expected false, got %v", g.schema["allow_infinity"])
	}
	if g.schema["exclude_min"] != false {
		t.Errorf("exclude_min: expected false, got %v", g.schema["exclude_min"])
	}
	if g.schema["exclude_max"] != false {
		t.Errorf("exclude_max: expected false, got %v", g.schema["exclude_max"])
	}
	minVal, _ := g.schema["min_value"].(float64)
	maxVal, _ := g.schema["max_value"].(float64)
	if minVal != 0.0 {
		t.Errorf("min_value: expected 0.0, got %v", minVal)
	}
	if maxVal != 1.0 {
		t.Errorf("max_value: expected 1.0, got %v", maxVal)
	}
	width, _ := extractInt(g.schema["width"])
	if width != 64 {
		t.Errorf("width: expected 64, got %d", width)
	}
}

// TestFloatsSchemaUnbounded verifies that Floats with no bounds defaults allow_nan=true, allow_infinity=true.
func TestFloatsSchemaUnbounded(t *testing.T) {
	g := Floats(nil, nil, nil, nil, false, false)
	if g.schema["allow_nan"] != true {
		t.Errorf("allow_nan: expected true (no bounds), got %v", g.schema["allow_nan"])
	}
	if g.schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true (no bounds), got %v", g.schema["allow_infinity"])
	}
	if _, hasMin := g.schema["min_value"]; hasMin {
		t.Error("min_value should not be present when minVal is nil")
	}
	if _, hasMax := g.schema["max_value"]; hasMax {
		t.Error("max_value should not be present when maxVal is nil")
	}
}

// TestFloatsSchemaOnlyMin verifies Floats with only min bound: allow_nan=false, allow_infinity=true.
func TestFloatsSchemaOnlyMin(t *testing.T) {
	minV := 0.0
	g := Floats(&minV, nil, nil, nil, false, false)
	// has_min=true, has_max=false → allow_nan=false, allow_infinity=true
	if g.schema["allow_nan"] != false {
		t.Errorf("allow_nan: expected false when min set, got %v", g.schema["allow_nan"])
	}
	if g.schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true when only min set, got %v", g.schema["allow_infinity"])
	}
}

// TestFloatsSchemaOnlyMax verifies Floats with only max bound: allow_nan=false, allow_infinity=true.
func TestFloatsSchemaOnlyMax(t *testing.T) {
	maxV := 1.0
	g := Floats(nil, &maxV, nil, nil, false, false)
	// has_min=false, has_max=true → allow_nan=false, allow_infinity=true
	if g.schema["allow_nan"] != false {
		t.Errorf("allow_nan: expected false when max set, got %v", g.schema["allow_nan"])
	}
	if g.schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true when only max set, got %v", g.schema["allow_infinity"])
	}
}

// TestFloatsSchemaExcludeBounds verifies that excludeMin/excludeMax are stored correctly.
func TestFloatsSchemaExcludeBounds(t *testing.T) {
	minV := 0.0
	maxV := 1.0
	falseV := false
	g := Floats(&minV, &maxV, &falseV, &falseV, true, true)
	if g.schema["exclude_min"] != true {
		t.Errorf("exclude_min: expected true, got %v", g.schema["exclude_min"])
	}
	if g.schema["exclude_max"] != true {
		t.Errorf("exclude_max: expected true, got %v", g.schema["exclude_max"])
	}
}

// =============================================================================
// FlatMappedGenerator tests
// =============================================================================

// TestFlatMappedGeneratorIsNotBasic verifies that FlatMap returns a non-basic generator.
func TestFlatMappedGeneratorIsNotBasic(t *testing.T) {
	gen := FlatMap(Integers(1, 5), func(v int64) *Generator[int64] {
		return Integers(0, 10)
	})
	if gen.isBasic() {
		t.Error("FlatMap should return a non-basic generator")
	}
}

// TestFlatMappedGeneratorIsNotBasicUnbounded verifies that FlatMap returns a non-basic generator.
func TestFlatMappedGeneratorIsNotBasicUnbounded(t *testing.T) {
	gen := FlatMap(IntegersUnbounded(), func(v int64) *Generator[int64] {
		return IntegersUnbounded()
	})
	if gen.isBasic() {
		t.Error("FlatMap should return a non-basic generator")
	}
}

// TestFlatMappedGeneratorMapReturnsNonBasic verifies that Map on FlatMap result returns a non-basic generator.
func TestFlatMappedGeneratorMapReturnsNonBasic(t *testing.T) {
	gen := FlatMap(Integers(1, 5), func(v int64) *Generator[int64] {
		return Integers(0, 10)
	})
	mapped := Map(gen, func(v int64) int64 { return v })
	if mapped.isBasic() {
		t.Fatal("Map on FlatMap result should return a non-basic generator")
	}
}

// TestFlatMappedGeneratorGenerate verifies the low-level protocol:
// start_span(11), source generate, second generate, stop_span, mark_complete.
func TestFlatMappedGeneratorGenerate(t *testing.T) {
	var cmds []string
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

		// Expect: start_span(11), generate(source), generate(second), stop_span, mark_complete
		for i := 0; i < 5; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := DecodeCBOR(pl)
			mp, _ := extractDict(dec)
			cmd, _ := extractString(mp[any("command")])
			cmds = append(cmds, cmd)
			switch cmd {
			case "generate":
				caseCh.SendReplyValue(mid, int64(7)) //nolint:errcheck
			default:
				caseCh.SendReplyValue(mid, nil) //nolint:errcheck
			}
		}

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotVal int64
	err := cli.runTest("flatmap_protocol", func() {
		gen := FlatMap(
			Integers(0, 100),
			func(v int64) *Generator[int64] { return Integers(0, 100) },
		)
		gotVal = Draw(gen)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 7 {
		t.Errorf("expected 7, got %d", gotVal)
	}
	// Verify protocol order: start_span, generate, generate, stop_span, mark_complete
	if len(cmds) < 4 {
		t.Fatalf("expected at least 4 commands, got %v", cmds)
	}
	if cmds[0] != "start_span" {
		t.Errorf("cmds[0]: expected start_span, got %s", cmds[0])
	}
	if cmds[1] != "generate" {
		t.Errorf("cmds[1]: expected generate (source), got %s", cmds[1])
	}
	if cmds[2] != "generate" {
		t.Errorf("cmds[2]: expected generate (second), got %s", cmds[2])
	}
	if cmds[3] != "stop_span" {
		t.Errorf("cmds[3]: expected stop_span, got %s", cmds[3])
	}
}

// TestFlatMappedGeneratorStartSpanLabel verifies that the FLAT_MAP span uses label 11.
func TestFlatMappedGeneratorStartSpanLabel(t *testing.T) {
	var gotLabel int64
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

		for i := 0; i < 5; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := DecodeCBOR(pl)
			mp, _ := extractDict(dec)
			cmd, _ := extractString(mp[any("command")])
			if cmd == "start_span" {
				gotLabel, _ = extractInt(mp[any("label")])
			}
			switch cmd {
			case "generate":
				caseCh.SendReplyValue(mid, int64(3)) //nolint:errcheck
			default:
				caseCh.SendReplyValue(mid, nil) //nolint:errcheck
			}
		}

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("flatmap_label", func() {
		gen := FlatMap(Integers(0, 10), func(v int64) *Generator[int64] { return Integers(0, 10) })
		_ = Draw(gen)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotLabel != int64(LabelFlatMap) {
		t.Errorf("start_span label: expected %d (LabelFlatMap), got %d", LabelFlatMap, gotLabel)
	}
}

// TestFlatMappedGeneratorE2E verifies that flat_map produces a dependent value.
// FlatMap(Integers(1,5), n => Text(n, n)) always produces text of length in [1,5].
func TestFlatMappedGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	gen := FlatMap(Integers(1, 5), func(n int64) *Generator[string] {
		return Text(int(n), int(n)) // exact length = n
	})
	RunHegelTest(t.Name(), func() {
		s := Draw(gen)
		count := len([]rune(s))
		// n is in [1,5], so text length is in [1,5].
		if count < 1 || count > 5 {
			panic(fmt.Sprintf("flat_map text length %d out of [1,5]", count))
		}
	}, WithTestCases(50))
}

// TestFlatMappedGeneratorDependency verifies that the second generation genuinely depends
// on the first generated value. We generate n in [2,4] and a list of exactly n elements.
// Every list must have length in [2,4] and all elements must be in [0,100].
func TestFlatMappedGeneratorDependency(t *testing.T) {
	hegelBinPath(t)
	gen := FlatMap(Integers(2, 4), func(n int64) *Generator[[]int64] {
		sz := int(n)
		return Lists(Integers(0, 100), ListsOptions{MinSize: sz, MaxSize: sz})
	})
	RunHegelTest(t.Name(), func() {
		slice := Draw(gen)
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

// --- Tuples3/4 non-basic drawFn protocol tests ---

func TestTuples3NonBasicProtocol(t *testing.T) {
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	gen := Tuples3(nonBasic, Integers(0, 10), Integers(0, 10))

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

		// Protocol: start_span(TUPLE), start_span(FILTER), generate, stop_span(FILTER),
		//           generate, generate, stop_span(TUPLE), mark_complete
		for i := 0; i < 8; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := DecodeCBOR(pl)
			mp, _ := extractDict(dec)
			cmd, _ := extractString(mp[any("command")])
			switch {
			case cmd == "generate":
				caseCh.SendReplyValue(mid, int64(7)) //nolint:errcheck
			default:
				caseCh.SendReplyValue(mid, nil) //nolint:errcheck
			}
		}

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var got Tuple3[int64, int64, int64]
	err := cli.runTest("tuples3_nonbasic", func() {
		got = Draw(gen)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if got.A != 7 || got.B != 7 || got.C != 7 {
		t.Errorf("expected all 7, got %+v", got)
	}
}

func TestTuples4NonBasicProtocol(t *testing.T) {
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	gen := Tuples4(nonBasic, Integers(0, 10), Integers(0, 10), Integers(0, 10))

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

		// Protocol: start_span(TUPLE), start_span(FILTER), generate, stop_span(FILTER),
		//           generate, generate, generate, stop_span(TUPLE), mark_complete
		for i := 0; i < 9; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := DecodeCBOR(pl)
			mp, _ := extractDict(dec)
			cmd, _ := extractString(mp[any("command")])
			switch {
			case cmd == "generate":
				caseCh.SendReplyValue(mid, int64(5)) //nolint:errcheck
			default:
				caseCh.SendReplyValue(mid, nil) //nolint:errcheck
			}
		}

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var got Tuple4[int64, int64, int64, int64]
	err := cli.runTest("tuples4_nonbasic", func() {
		got = Draw(gen)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if got.A != 5 || got.B != 5 || got.C != 5 || got.D != 5 {
		t.Errorf("expected all 5, got %+v", got)
	}
}
