package hegel

import (
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// Generator interface and BasicGenerator tests
// =============================================================================

// --- BasicGenerator: generate with no transform ---

func TestBasicGeneratorGenerateNoTransform(t *testing.T) {
	// Set up fake server that responds to a generate command with int64(42).
	schema := map[string]any{"type": "integer", "min_value": int64(0), "max_value": int64(100)}
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel_id")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		// Send one test_case.
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := encodeCBOR(map[string]any{
			"event":      "test_case",
			"channel_id": int64(caseCh.ChannelID()),
			"is_final":   false,
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// Respond to generate with 42.
		genID, genPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		_, _ = genPayload, decodeCBOR           // consumed
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
		g := &BasicGenerator{schema: schema}
		v := Draw(g)
		gotVal, _ = ExtractInt(v)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 42 {
		t.Errorf("expected 42, got %d", gotVal)
	}
}

// --- BasicGenerator: generate with transform ---

func TestBasicGeneratorGenerateWithTransform(t *testing.T) {
	schema := map[string]any{"type": "integer"}
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel_id")])
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
		g := &BasicGenerator{
			schema:    schema,
			transform: func(v any) any { n, _ := ExtractInt(v); return n * 2 },
		}
		v := Draw(g)
		gotVal, _ = v.(int64)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 14 {
		t.Errorf("expected 14, got %d", gotVal)
	}
}

// --- BasicGenerator.Map: no existing transform ---

func TestBasicGeneratorMapNoTransform(t *testing.T) {
	schema := map[string]any{"type": "boolean"}
	g := &BasicGenerator{schema: schema}
	mapped := g.Map(func(v any) any {
		b, _ := v.(bool)
		if b {
			return "yes"
		}
		return "no"
	})
	// Map on BasicGenerator returns another BasicGenerator with same schema.
	bg, ok := mapped.(*BasicGenerator)
	if !ok {
		t.Fatalf("Map on BasicGenerator should return *BasicGenerator, got %T", mapped)
	}
	if bg.schema["type"] != "boolean" {
		t.Errorf("schema not preserved by Map")
	}
	if bg.transform == nil {
		t.Error("transform should not be nil after Map")
	}
}

// --- BasicGenerator.Map: compose transforms ---

func TestBasicGeneratorMapComposesTransforms(t *testing.T) {
	schema := map[string]any{"type": "integer"}
	g := &BasicGenerator{
		schema:    schema,
		transform: func(v any) any { n, _ := ExtractInt(v); return n + 1 },
	}
	// Map again: result should be (n+1)*2
	mapped := g.Map(func(v any) any {
		n, _ := ExtractInt(v)
		return n * 2
	})
	bg, ok := mapped.(*BasicGenerator)
	if !ok {
		t.Fatalf("double Map should return *BasicGenerator")
	}
	// Simulate applying: start with int64(5) → +1 → 6 → *2 → 12
	result := bg.transform(int64(5))
	n, _ := ExtractInt(result)
	if n != 12 {
		t.Errorf("composed transform: expected 12, got %d", n)
	}
}

// --- BasicGenerator.AsBasic ---

func TestBasicGeneratorAsBasic(t *testing.T) {
	g := &BasicGenerator{schema: map[string]any{"type": "boolean"}}
	ab := g.AsBasic()
	if ab != g {
		t.Error("AsBasic should return itself")
	}
}

// --- mappedGenerator ---

func TestMappedGeneratorGenerate(t *testing.T) {
	// mappedGenerator wraps a non-basic generator.
	schema := map[string]any{"type": "integer"}
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel_id")])
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

		// start_span, generate, stop_span.
		// start_span
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck
		// generate
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(3)) //nolint:errcheck
		// stop_span
		spID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck

		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotVal int64
	err := cli.runTest("mapped_gen", func() {
		inner := &BasicGenerator{schema: schema}
		mg := &mappedGenerator{
			inner: inner,
			fn:    func(v any) any { n, _ := ExtractInt(v); return n * 10 },
		}
		v := Draw(mg)
		gotVal, _ = v.(int64)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 30 {
		t.Errorf("expected 30, got %d", gotVal)
	}
}

// --- mappedGenerator.AsBasic ---

func TestMappedGeneratorAsBasic(t *testing.T) {
	mg := &mappedGenerator{
		inner: &BasicGenerator{schema: map[string]any{"type": "integer"}},
		fn:    func(v any) any { return v },
	}
	if mg.AsBasic() != nil {
		t.Error("mappedGenerator.AsBasic should return nil")
	}
}

// --- mappedGenerator.Map returns BasicGenerator when inner is BasicGenerator ---

func TestMappedGeneratorMapOnBasicInner(t *testing.T) {
	inner := &BasicGenerator{schema: map[string]any{"type": "integer"}}
	// Map on BasicGenerator returns BasicGenerator.
	result := inner.Map(func(v any) any { return v })
	if _, ok := result.(*BasicGenerator); !ok {
		t.Errorf("Map on BasicGenerator should return *BasicGenerator")
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
		decoded, _ := decodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel_id")])
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

		fn(caseCh)

		// Wait for mark_complete.
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})
}

// --- collection.more: cached false after first false ---

func TestCollectionMoreCachesFalse(t *testing.T) {
	clientConn := fakeTestEnv(t, func(caseCh *channel) {
		// new_collection
		ncID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_y") //nolint:errcheck

		// Only one more request — the second more() call should be cached.
		moreID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(moreID, false) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("coll_cache", func() {
		data := getState()
		coll := newCollection(0, 1, data)
		r1 := coll.more(data)
		r2 := coll.more(data) // should be cached false, no network call
		if r1 || r2 {
			panic("expected both to be false")
		}
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
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
		n := Draw(Integers(0, 100))
		v, _ := ExtractInt(n)
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
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("Integers should return *BasicGenerator")
	}
	min, _ := ExtractInt(bg.schema["min_value"])
	max, _ := ExtractInt(bg.schema["max_value"])
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

// --- Integers: nil min/max omitted from schema ---

func TestIntegersNoBounds(t *testing.T) {
	g := IntegersUnbounded()
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("IntegersUnbounded should return *BasicGenerator")
	}
	if _, hasMin := bg.schema["min_value"]; hasMin {
		t.Error("min_value should not be present when no min bound given")
	}
	if _, hasMax := bg.schema["max_value"]; hasMax {
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
		if v.(int) != 42 {
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
	_, err := SampledFrom([]any{})
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
	MustSampledFrom([]any{})
}

// TestSampledFromSchema verifies that SampledFrom produces an integer schema with correct bounds.
func TestSampledFromSchema(t *testing.T) {
	g, err := SampledFrom([]any{"a", "b", "c"})
	if err != nil {
		t.Fatalf("SampledFrom: unexpected error: %v", err)
	}
	if g.schema["type"] != "integer" {
		t.Errorf("schema type: expected 'integer', got %v", g.schema["type"])
	}
	minVal, _ := ExtractInt(g.schema["min_value"])
	maxVal, _ := ExtractInt(g.schema["max_value"])
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
	items := []any{"x", "y", "z"}
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
	g, _ := SampledFrom([]any{"only"})
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
	choices := []any{"apple", "banana", "cherry"}
	g, _ := SampledFrom(choices)
	seen := map[string]bool{}
	RunHegelTest(t.Name(), func() {
		v := Draw(g)
		s, ok := v.(string)
		if !ok {
			panic(fmt.Sprintf("SampledFrom: expected string, got %T", v))
		}
		found := false
		for _, c := range choices {
			if c == s {
				found = true
				break
			}
		}
		if !found {
			panic(fmt.Sprintf("SampledFrom: value %q not in choices", s))
		}
		seen[s] = true
	}, WithTestCases(100))
	// After 100 cases we expect all 3 values to have appeared.
	for _, c := range choices {
		if !seen[c.(string)] {
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
	g, _ := SampledFrom([]any{obj1, obj2})
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
		v := Draw(g)
		s, ok := v.(string)
		if !ok {
			panic(fmt.Sprintf("FromRegex: expected string, got %T", v))
		}
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
// BasicGenerator.Generate error path (line 78-79)
// =============================================================================

// TestBasicGeneratorGenerateErrorResponse covers the error path in
// BasicGenerator.Generate when generateFromSchema returns a non-StopTest error.
func TestBasicGeneratorGenerateErrorResponse(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "error_response")
	err := RunHegelTestE(t.Name(), func() {
		g := &BasicGenerator{schema: map[string]any{"type": "integer"}}
		_ = Draw(g) // should panic with requestError → caught as INTERESTING
	})
	// error_response causes the test to appear interesting (failing).
	_ = err
}

// =============================================================================
// Generator.Map on a Generator interface (non-basic returns mappedGenerator)
// =============================================================================

func TestGeneratorMapOnNonBasic(t *testing.T) {
	// A custom generator that is not a BasicGenerator.
	schema := map[string]any{"type": "integer"}
	inner := &BasicGenerator{schema: schema}
	// mappedGenerator is not a BasicGenerator.
	mg := &mappedGenerator{inner: inner, fn: func(v any) any { return v }}
	mapped := mg.Map(func(v any) any { return v })
	// Mapping a non-basic generator should produce a mappedGenerator.
	if _, ok := mapped.(*mappedGenerator); !ok {
		t.Errorf("Map on non-basic Generator should return *mappedGenerator, got %T", mapped)
	}
}

// =============================================================================
// MustSampledFrom happy-path test
// =============================================================================

// TestMustSampledFromHappyPath verifies that MustSampledFrom returns a valid generator
// when given a non-empty slice.
func TestMustSampledFromHappyPath(t *testing.T) {
	g := MustSampledFrom([]any{"only"})
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
// always produces even values in [0, 200], and the result is still a BasicGenerator.
func TestMapBasicGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	gen := Integers(0, 100).Map(func(v any) any {
		n, _ := ExtractInt(v)
		return n * 2
	})
	// Map on basic generator must preserve BasicGenerator type.
	if _, ok := gen.(*BasicGenerator); !ok {
		t.Fatalf("Map on BasicGenerator should return *BasicGenerator, got %T", gen)
	}
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		n, _ := ExtractInt(v)
		if n%2 != 0 {
			panic(fmt.Sprintf("map(x*2): expected even number, got %d", n))
		}
		if n < 0 || n > 200 {
			panic(fmt.Sprintf("map(x*2): expected [0,200], got %d", n))
		}
	}, WithTestCases(50))
}

// TestMapChainedBasicGeneratorE2E verifies that chaining two maps on a BasicGenerator
// preserves the BasicGenerator type and composes the transforms correctly.
// Integers(0,100).Map(x+1).Map(x*2): result must be even, in [2, 202].
func TestMapChainedBasicGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	gen := Integers(0, 100).
		Map(func(v any) any {
			n, _ := ExtractInt(v)
			return n + 1
		}).
		Map(func(v any) any {
			n, _ := ExtractInt(v)
			return n * 2
		})
	// Both chained maps should still return a BasicGenerator (schema preserved).
	if _, ok := gen.(*BasicGenerator); !ok {
		t.Fatalf("chained Map on BasicGenerator should return *BasicGenerator, got %T", gen)
	}
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		n, _ := ExtractInt(v)
		// (x+1)*2 is always even. x in [0,100] → result in [2, 202].
		if n%2 != 0 {
			panic(fmt.Sprintf("map(x+1).map(x*2): expected even, got %d", n))
		}
		if n < 2 || n > 202 {
			panic(fmt.Sprintf("map(x+1).map(x*2): expected [2,202], got %d", n))
		}
	}, WithTestCases(50))
}

// TestMapNonBasicGeneratorE2E verifies that mapping a mappedGenerator (non-basic)
// wraps it in a MAPPED span and applies the transform correctly.
// The result must be a mappedGenerator (not BasicGenerator).
func TestMapNonBasicGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	// Create a non-basic generator by wrapping a BasicGenerator in mappedGenerator.
	inner := Integers(1, 5)
	nonBasic := &mappedGenerator{
		inner: inner,
		fn:    func(v any) any { return v }, // identity
	}
	gen := nonBasic.Map(func(v any) any {
		n, _ := ExtractInt(v)
		return n * 3
	})
	if _, ok := gen.(*mappedGenerator); !ok {
		t.Fatalf("Map on non-basic Generator should return *mappedGenerator, got %T", gen)
	}
	if gen.AsBasic() != nil {
		t.Error("mappedGenerator.AsBasic() should return nil")
	}
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		n, _ := ExtractInt(v)
		// inner is Integers(1,5)*1, map(*3): result is in {3, 6, 9, 12, 15}
		if n < 3 || n > 15 || n%3 != 0 {
			panic(fmt.Sprintf("map(*3) on [1,5]: expected multiple of 3 in [3,15], got %d", n))
		}
	}, WithTestCases(50))
}

// TestMapSchemaPreservedUnit verifies unit-level schema properties of Map on BasicGenerator.
func TestMapSchemaPreservedUnit(t *testing.T) {
	base := Integers(0, 100)
	mapped := base.Map(func(v any) any { return v })
	bg, ok := mapped.(*BasicGenerator)
	if !ok {
		t.Fatalf("Map on BasicGenerator: expected *BasicGenerator, got %T", mapped)
	}
	if bg.schema["type"] != "integer" {
		t.Errorf("schema type: expected 'integer', got %v", bg.schema["type"])
	}
	if bg.transform == nil {
		t.Error("transform should not be nil after Map")
	}
	// Map on BasicGenerator must preserve min/max bounds in the schema.
	minV, _ := ExtractInt(bg.schema["min_value"])
	maxV, _ := ExtractInt(bg.schema["max_value"])
	if minV != 0 {
		t.Errorf("min_value: expected 0, got %d", minV)
	}
	if maxV != 100 {
		t.Errorf("max_value: expected 100, got %d", maxV)
	}

	// Double Map on BasicGenerator: schema still preserved, transforms compose correctly.
	doubled := base.
		Map(func(v any) any { n, _ := ExtractInt(v); return n + 10 }).
		Map(func(v any) any { n, _ := ExtractInt(v); return n * 2 })
	bg2, ok := doubled.(*BasicGenerator)
	if !ok {
		t.Fatalf("double Map on BasicGenerator: expected *BasicGenerator, got %T", doubled)
	}
	if bg2.schema["type"] != "integer" {
		t.Errorf("double map schema type: expected 'integer', got %v", bg2.schema["type"])
	}
	// Verify composition: input 5 → +10 → 15 → *2 → 30.
	result := bg2.transform(int64(5))
	n, _ := ExtractInt(result)
	if n != 30 {
		t.Errorf("double map compose: input 5, expected 30, got %d", n)
	}

	// Map on mappedGenerator: AsBasic() returns nil.
	mg := &mappedGenerator{inner: base, fn: func(v any) any { return v }}
	mappedMG := mg.Map(func(v any) any { return v })
	if mappedMG.AsBasic() != nil {
		t.Error("mapping a mappedGenerator should produce AsBasic()=nil")
	}
}

// =============================================================================
// Tuple generator tests
// =============================================================================

// TestTuples2AllBasicNoTransform verifies that Tuples2 of two basic (no-transform)
// generators returns a BasicGenerator with schema type=tuple and two elements.
func TestTuples2AllBasicNoTransform(t *testing.T) {
	g1 := Integers(0, 10)
	g2 := Booleans(0.5)
	gen := Tuples2(g1, g2)
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("Tuples2 of basic generators should return *BasicGenerator, got %T", gen)
	}
	if bg.schema["type"] != "tuple" {
		t.Errorf("schema type: expected 'tuple', got %v", bg.schema["type"])
	}
	elements, ok := bg.schema["elements"].([]any)
	if !ok {
		t.Fatalf("schema 'elements' should be []any, got %T", bg.schema["elements"])
	}
	if len(elements) != 2 {
		t.Errorf("expected 2 elements, got %d", len(elements))
	}
	// No transform for no-transform elements.
	if bg.transform != nil {
		t.Error("transform should be nil when no elements have transforms")
	}
	// AsBasic returns itself.
	if bg.AsBasic() != bg {
		t.Error("AsBasic should return itself")
	}
}

// TestTuples2AllBasicWithTransforms verifies that Tuples2 of mapped basic generators
// returns a BasicGenerator with the raw schemas (not transformed), and the combined
// transform applies per-position transforms.
func TestTuples2AllBasicWithTransforms(t *testing.T) {
	g1 := Integers(0, 10).Map(func(v any) any {
		n, _ := ExtractInt(v)
		return n * 2
	})
	g2 := Just(5).Map(func(v any) any {
		n, _ := v.(int)
		return n + 1
	})
	// Both g1 and g2 are *BasicGenerator with transforms.
	if _, ok := g1.(*BasicGenerator); !ok {
		t.Fatalf("g1 should be *BasicGenerator, got %T", g1)
	}
	if _, ok := g2.(*BasicGenerator); !ok {
		t.Fatalf("g2 should be *BasicGenerator, got %T", g2)
	}
	gen := Tuples2(g1, g2)
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("Tuples2 of mapped basic generators should return *BasicGenerator, got %T", gen)
	}
	if bg.schema["type"] != "tuple" {
		t.Errorf("schema type: expected 'tuple', got %v", bg.schema["type"])
	}
	if bg.transform == nil {
		t.Error("transform should not be nil when elements have transforms")
	}
	// Verify per-position transform: input [4, nil] → [4*2=8, 5+1=6]
	// (g2 = Just(5).Map(x+1): raw value from server is nil (const), transform of Just gives 5,
	// then +1 gives 6)
	raw := []any{uint64(4), nil}
	result, ok := bg.transform(raw).([]any)
	if !ok {
		t.Fatalf("transform result should be []any, got %T", bg.transform(raw))
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	// position 0: 4*2=8
	n0, _ := ExtractInt(result[0])
	if n0 != 8 {
		t.Errorf("position 0: expected 8, got %d", n0)
	}
	// position 1: Just(5).transform(nil)=5, then +1=6
	n1, _ := result[1].(int)
	if n1 != 6 {
		t.Errorf("position 1: expected 6, got %d", n1)
	}
}

// TestTuples2MixedBasicNonBasic verifies that Tuples2 with a non-basic element
// returns a compositeTupleGenerator, not a BasicGenerator.
func TestTuples2MixedBasicNonBasic(t *testing.T) {
	// Create a non-basic generator by wrapping in mappedGenerator.
	nonBasic := &mappedGenerator{
		inner: Integers(0, 10),
		fn:    func(v any) any { return v },
	}
	g2 := Booleans(0.5)
	gen := Tuples2(nonBasic, g2)
	if _, ok := gen.(*compositeTupleGenerator); !ok {
		t.Fatalf("Tuples2 with non-basic element should return *compositeTupleGenerator, got %T", gen)
	}
	if gen.AsBasic() != nil {
		t.Error("compositeTupleGenerator.AsBasic() should return nil")
	}
}

// TestTuples3AllBasic verifies that Tuples3 of all-basic generators returns a BasicGenerator
// with schema type=tuple and three elements.
func TestTuples3AllBasic(t *testing.T) {
	gen := Tuples3(Integers(0, 5), Booleans(0.5), Text(1, 5))
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("Tuples3 of basic generators should return *BasicGenerator, got %T", gen)
	}
	if bg.schema["type"] != "tuple" {
		t.Errorf("schema type: expected 'tuple', got %v", bg.schema["type"])
	}
	elements, ok := bg.schema["elements"].([]any)
	if !ok {
		t.Fatalf("schema 'elements' should be []any, got %T", bg.schema["elements"])
	}
	if len(elements) != 3 {
		t.Errorf("expected 3 elements, got %d", len(elements))
	}
}

// TestTuples3WithNonBasic verifies that Tuples3 falls back to compositeTupleGenerator
// when any element is non-basic.
func TestTuples3WithNonBasic(t *testing.T) {
	nonBasic := &mappedGenerator{inner: Integers(0, 5), fn: func(v any) any { return v }}
	gen := Tuples3(nonBasic, Booleans(0.5), Text(1, 5))
	if _, ok := gen.(*compositeTupleGenerator); !ok {
		t.Fatalf("Tuples3 with non-basic should return *compositeTupleGenerator, got %T", gen)
	}
}

// TestTuples4AllBasic verifies that Tuples4 of all-basic generators returns a BasicGenerator
// with schema type=tuple and four elements.
func TestTuples4AllBasic(t *testing.T) {
	gen := Tuples4(Integers(0, 5), Booleans(0.5), Text(1, 5), IntegersUnbounded())
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("Tuples4 of basic generators should return *BasicGenerator, got %T", gen)
	}
	elements, ok := bg.schema["elements"].([]any)
	if !ok {
		t.Fatalf("schema 'elements' should be []any, got %T", bg.schema["elements"])
	}
	if len(elements) != 4 {
		t.Errorf("expected 4 elements, got %d", len(elements))
	}
}

// TestTuples4WithNonBasic verifies that Tuples4 falls back to compositeTupleGenerator
// when any element is non-basic.
func TestTuples4WithNonBasic(t *testing.T) {
	nonBasic := &mappedGenerator{inner: Integers(0, 5), fn: func(v any) any { return v }}
	gen := Tuples4(Integers(0, 5), Booleans(0.5), nonBasic, Text(1, 5))
	if _, ok := gen.(*compositeTupleGenerator); !ok {
		t.Fatalf("Tuples4 with non-basic should return *compositeTupleGenerator, got %T", gen)
	}
}

// TestCompositeTupleGeneratorMap verifies that Map on compositeTupleGenerator returns
// a mappedGenerator.
func TestCompositeTupleGeneratorMap(t *testing.T) {
	nonBasic := &mappedGenerator{inner: Integers(0, 5), fn: func(v any) any { return v }}
	comp := Tuples2(nonBasic, Booleans(0.5))
	mapped := comp.Map(func(v any) any { return v })
	if _, ok := mapped.(*mappedGenerator); !ok {
		t.Fatalf("Map on compositeTupleGenerator should return *mappedGenerator, got %T", mapped)
	}
}

// TestTuples2AllBasicNoTransformE2E runs Tuples2(Integers, Booleans) against the real server.
func TestTuples2AllBasicNoTransformE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		gen := Tuples2(Integers(0, 10), Booleans(0.5))
		v := Draw(gen)
		result, ok := v.([]any)
		if !ok {
			panic(fmt.Sprintf("Tuples2: expected []any, got %T", v))
		}
		if len(result) != 2 {
			panic(fmt.Sprintf("Tuples2: expected len=2, got %d", len(result)))
		}
		n, _ := ExtractInt(result[0])
		if n < 0 || n > 10 {
			panic(fmt.Sprintf("Tuples2[0]: out of range [0,10]: %d", n))
		}
		_, ok = result[1].(bool)
		if !ok {
			panic(fmt.Sprintf("Tuples2[1]: expected bool, got %T", result[1]))
		}
	}, WithTestCases(50))
}

// TestTuples2WithTransformsE2E runs Tuples2 with mapped elements against the real server.
// Property: position 0 is always even; position 1 is always 6.
func TestTuples2WithTransformsE2E(t *testing.T) {
	hegelBinPath(t)
	g1 := Integers(0, 10).Map(func(v any) any {
		n, _ := ExtractInt(v)
		return n * 2
	})
	g2 := Just(5).Map(func(v any) any {
		n := v.(int)
		return n + 1
	})
	gen := Tuples2(g1, g2)
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		result, ok := v.([]any)
		if !ok {
			panic(fmt.Sprintf("Tuples2 mapped: expected []any, got %T", v))
		}
		if len(result) != 2 {
			panic(fmt.Sprintf("Tuples2 mapped: expected len=2, got %d", len(result)))
		}
		n0, _ := ExtractInt(result[0])
		if n0%2 != 0 || n0 < 0 || n0 > 20 {
			panic(fmt.Sprintf("Tuples2 mapped[0]: expected even in [0,20], got %d", n0))
		}
		n1 := result[1].(int)
		if n1 != 6 {
			panic(fmt.Sprintf("Tuples2 mapped[1]: expected 6, got %d", n1))
		}
	}, WithTestCases(50))
}

// TestTuples3E2E runs Tuples3(Text, Integers, Floats) against the real server.
func TestTuples3E2E(t *testing.T) {
	hegelBinPath(t)
	falseBool := false
	gen := Tuples3(Text(1, 5), Integers(0, 5), Floats(floatPtr(0.0), floatPtr(1.0), &falseBool, &falseBool, false, false))
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		result, ok := v.([]any)
		if !ok {
			panic(fmt.Sprintf("Tuples3: expected []any, got %T", v))
		}
		if len(result) != 3 {
			panic(fmt.Sprintf("Tuples3: expected len=3, got %d", len(result)))
		}
		_, ok = result[0].(string)
		if !ok {
			panic(fmt.Sprintf("Tuples3[0]: expected string, got %T", result[0]))
		}
		n, _ := ExtractInt(result[1])
		if n < 0 || n > 5 {
			panic(fmt.Sprintf("Tuples3[1]: expected [0,5], got %d", n))
		}
		f, ok := result[2].(float64)
		if !ok {
			panic(fmt.Sprintf("Tuples3[2]: expected float64, got %T", result[2]))
		}
		if f < 0.0 || f > 1.0 {
			panic(fmt.Sprintf("Tuples3[2]: expected [0,1], got %v", f))
		}
	}, WithTestCases(50))
}

// TestTuples2NonBasicE2E runs a compositeTupleGenerator against the real server.
// Uses a filtered generator so the first element is always generated via span protocol.
func TestTuples2NonBasicE2E(t *testing.T) {
	hegelBinPath(t)
	// mappedGenerator wrapping Integers(0,10) is non-basic.
	nonBasic := &mappedGenerator{
		inner: Integers(0, 10),
		fn:    func(v any) any { n, _ := ExtractInt(v); return n + 100 },
	}
	gen := Tuples2(nonBasic, Booleans(0.5))
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		result, ok := v.([]any)
		if !ok {
			panic(fmt.Sprintf("Tuples2 non-basic: expected []any, got %T", v))
		}
		if len(result) != 2 {
			panic(fmt.Sprintf("Tuples2 non-basic: expected len=2, got %d", len(result)))
		}
		n, _ := ExtractInt(result[0])
		if n < 100 || n > 110 {
			panic(fmt.Sprintf("Tuples2 non-basic[0]: expected [100,110], got %d", n))
		}
		_, ok = result[1].(bool)
		if !ok {
			panic(fmt.Sprintf("Tuples2 non-basic[1]: expected bool, got %T", result[1]))
		}
	}, WithTestCases(50))
}

// TestTuples4E2E runs Tuples4 of all-basic generators against the real server.
func TestTuples4E2E(t *testing.T) {
	hegelBinPath(t)
	gen := Tuples4(Integers(0, 5), Booleans(0.5), Text(1, 3), Integers(10, 20))
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		result, ok := v.([]any)
		if !ok {
			panic(fmt.Sprintf("Tuples4: expected []any, got %T", v))
		}
		if len(result) != 4 {
			panic(fmt.Sprintf("Tuples4: expected len=4, got %d", len(result)))
		}
		n0, _ := ExtractInt(result[0])
		if n0 < 0 || n0 > 5 {
			panic(fmt.Sprintf("Tuples4[0]: out of range [0,5]: %d", n0))
		}
		_, ok = result[1].(bool)
		if !ok {
			panic(fmt.Sprintf("Tuples4[1]: expected bool, got %T", result[1]))
		}
		_, ok = result[2].(string)
		if !ok {
			panic(fmt.Sprintf("Tuples4[2]: expected string, got %T", result[2]))
		}
		n3, _ := ExtractInt(result[3])
		if n3 < 10 || n3 > 20 {
			panic(fmt.Sprintf("Tuples4[3]: out of range [10,20]: %d", n3))
		}
	}, WithTestCases(50))
}

// TestTuples2BasicOneTransformOneNil verifies that when one element has a transform
// and the other does not (nil), the nil position passes the raw value through.
func TestTuples2BasicOneTransformOneNil(t *testing.T) {
	// g1: BasicGenerator with a transform (doubles)
	// g2: BasicGenerator without a transform (identity)
	g1 := Integers(0, 10).Map(func(v any) any {
		n, _ := ExtractInt(v)
		return n * 2
	})
	g2 := Integers(0, 5) // no transform
	gen := Tuples2(g1, g2)
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("expected *BasicGenerator, got %T", gen)
	}
	if bg.transform == nil {
		t.Fatal("transform should not be nil (g1 has a transform)")
	}
	// Apply: raw[0]=3 → 3*2=6; raw[1]=uint64(2) → pass-through=uint64(2)
	raw := []any{uint64(3), uint64(2)}
	result, ok := bg.transform(raw).([]any)
	if !ok {
		t.Fatalf("transform result should be []any, got %T", bg.transform(raw))
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	n0, _ := ExtractInt(result[0])
	if n0 != 6 {
		t.Errorf("position 0: expected 6, got %d", n0)
	}
	n1, _ := ExtractInt(result[1])
	if n1 != 2 {
		t.Errorf("position 1: expected 2 (pass-through), got %d", n1)
	}
}

// =============================================================================
// Primitive generator schema unit tests
// =============================================================================

// =============================================================================
// filteredGenerator tests
// =============================================================================

// TestFilteredGeneratorAsBasic verifies that filteredGenerator.AsBasic returns nil.
func TestFilteredGeneratorAsBasic(t *testing.T) {
	g := Integers(0, 10).Filter(func(v any) bool { return true })
	if g.AsBasic() != nil {
		t.Error("filteredGenerator.AsBasic() should return nil")
	}
}

// TestFilteredGeneratorFromBasicIsNotBasic verifies that Filter on a BasicGenerator
// returns a filteredGenerator (not a BasicGenerator).
func TestFilteredGeneratorFromBasicIsNotBasic(t *testing.T) {
	g := Integers(0, 100).Filter(func(v any) bool { return true })
	if _, ok := g.(*filteredGenerator); !ok {
		t.Fatalf("Filter on BasicGenerator should return *filteredGenerator, got %T", g)
	}
}

// TestFilteredGeneratorFilterMethod verifies that calling Filter on a filteredGenerator
// returns another filteredGenerator.
func TestFilteredGeneratorFilterMethod(t *testing.T) {
	g := Integers(0, 100).
		Filter(func(v any) bool { return true }).
		Filter(func(v any) bool { return true })
	if _, ok := g.(*filteredGenerator); !ok {
		t.Fatalf("Filter on filteredGenerator should return *filteredGenerator, got %T", g)
	}
}

// TestFilteredGeneratorMapMethod verifies that calling Map on a filteredGenerator
// returns a mappedGenerator.
func TestFilteredGeneratorMapMethod(t *testing.T) {
	g := Integers(0, 100).Filter(func(v any) bool { return true })
	mapped := g.Map(func(v any) any { return v })
	if _, ok := mapped.(*mappedGenerator); !ok {
		t.Fatalf("Map on filteredGenerator should return *mappedGenerator, got %T", mapped)
	}
}

// TestFilteredGeneratorPredicatePasses verifies that when the predicate passes on the
// first attempt, the value is returned immediately (only one FILTER span pair sent).
func TestFilteredGeneratorPredicatePasses(t *testing.T) {
	var gotVal int64
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel_id")])
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

		// start_span
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck
		// generate
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(42)) //nolint:errcheck
		// stop_span (discard=false)
		spID, spPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded2, _ := decodeCBOR(spPayload)
		m2, _ := ExtractDict(decoded2)
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
		g := &filteredGenerator{
			source:    &BasicGenerator{schema: map[string]any{"type": "integer"}},
			predicate: func(v any) bool { return true },
		}
		v := Draw(g)
		gotVal, _ = ExtractInt(v)
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
		decoded, _ := decodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel_id")])
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
			decoded2, _ := decodeCBOR(spPayload)
			m2, _ := ExtractDict(decoded2)
			discard, _ := m2[any("discard")].(bool)
			if !discard {
				t.Errorf("attempt %d: stop_span should have discard=true when predicate fails", i)
			}
			caseCh.SendReplyValue(spID, nil) //nolint:errcheck
			spanCount++
		}

		// Assume(false) panics with assumeRejected → runner sends mark_complete with "INVALID".
		mcID, mcPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded3, _ := decodeCBOR(mcPayload)
		m3, _ := ExtractDict(decoded3)
		mcStatus, _ = ExtractString(m3[any("status")])
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("filter_exhaust", func() {
		g := &filteredGenerator{
			source:    &BasicGenerator{schema: map[string]any{"type": "integer"}},
			predicate: func(v any) bool { return false }, // always reject
		}
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
		decoded, _ := decodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel_id")])
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
			decoded2, _ := decodeCBOR(spPayload)
			m2, _ := ExtractDict(decoded2)
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
		g := &filteredGenerator{
			source: &BasicGenerator{schema: map[string]any{"type": "integer"}},
			predicate: func(v any) bool {
				attemptNum++
				n, _ := ExtractInt(v)
				return n > 0
			},
		}
		v := Draw(g)
		gotVal, _ = ExtractInt(v)
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
		gen := Integers(0, 100).Filter(func(v any) bool {
			n, _ := ExtractInt(v)
			return n > 50
		})
		v := Draw(gen)
		n, _ := ExtractInt(v)
		if n <= 50 {
			panic(fmt.Sprintf("filter(>50): expected n>50, got %d", n))
		}
	}, WithTestCases(50))
}

// TestFilteredGeneratorE2EEvenNumbers verifies filter for even numbers.
func TestFilteredGeneratorE2EEvenNumbers(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		gen := Integers(0, 10).Filter(func(v any) bool {
			n, _ := ExtractInt(v)
			return n%2 == 0
		})
		v := Draw(gen)
		n, _ := ExtractInt(v)
		if n%2 != 0 {
			panic(fmt.Sprintf("filter(even): expected even, got %d", n))
		}
	}, WithTestCases(50))
}

// TestFilterOnNonBasicGenerators verifies that Filter works on non-basic generators.
func TestFilterOnNonBasicGenerators(t *testing.T) {
	// mappedGenerator.Filter
	mg := &mappedGenerator{inner: Integers(0, 5), fn: func(v any) any { return v }}
	fg := mg.Filter(func(v any) bool { return true })
	if _, ok := fg.(*filteredGenerator); !ok {
		t.Errorf("Filter on mappedGenerator should return *filteredGenerator, got %T", fg)
	}
	// compositeListGenerator.Filter
	cl := &compositeListGenerator{elements: Integers(0, 5), minSize: 0, maxSize: 3}
	fg2 := cl.Filter(func(v any) bool { return true })
	if _, ok := fg2.(*filteredGenerator); !ok {
		t.Errorf("Filter on compositeListGenerator should return *filteredGenerator, got %T", fg2)
	}
	// compositeDictGenerator.Filter
	cd := &compositeDictGenerator{keys: Integers(0, 5), values: Integers(0, 5), minSize: 0}
	fg3 := cd.Filter(func(v any) bool { return true })
	if _, ok := fg3.(*filteredGenerator); !ok {
		t.Errorf("Filter on compositeDictGenerator should return *filteredGenerator, got %T", fg3)
	}
	// compositeOneOfGenerator.Filter
	co := &compositeOneOfGenerator{generators: []Generator{Integers(0, 5), Integers(6, 10)}}
	fg4 := co.Filter(func(v any) bool { return true })
	if _, ok := fg4.(*filteredGenerator); !ok {
		t.Errorf("Filter on compositeOneOfGenerator should return *filteredGenerator, got %T", fg4)
	}
	// compositeTupleGenerator.Filter
	ct := &compositeTupleGenerator{elements: []Generator{Integers(0, 5)}}
	fg5 := ct.Filter(func(v any) bool { return true })
	if _, ok := fg5.(*filteredGenerator); !ok {
		t.Errorf("Filter on compositeTupleGenerator should return *filteredGenerator, got %T", fg5)
	}
	// FlatMappedGenerator.Filter
	fm := &FlatMappedGenerator{source: Integers(0, 5), f: func(v any) Generator { return Integers(0, 5) }}
	fg6 := fm.Filter(func(v any) bool { return true })
	if _, ok := fg6.(*filteredGenerator); !ok {
		t.Errorf("Filter on FlatMappedGenerator should return *filteredGenerator, got %T", fg6)
	}
}

// TestBooleansSchema verifies that Booleans produces a schema with type=boolean and p field.
func TestBooleansSchema(t *testing.T) {
	g := Booleans(0.5)
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("Booleans should return *BasicGenerator, got %T", g)
	}
	if bg.schema["type"] != "boolean" {
		t.Errorf("type: expected 'boolean', got %v", bg.schema["type"])
	}
	p, ok := bg.schema["p"].(float64)
	if !ok {
		t.Fatalf("p field should be float64, got %T", bg.schema["p"])
	}
	if p != 0.5 {
		t.Errorf("p: expected 0.5, got %v", p)
	}
	// AsBasic returns itself.
	if bg.AsBasic() != bg {
		t.Error("AsBasic should return itself")
	}
}

// TestBooleansP1Schema verifies that Booleans(1.0) stores p=1.0.
func TestBooleansP1Schema(t *testing.T) {
	g := Booleans(1.0)
	bg := g.(*BasicGenerator)
	if bg.schema["p"] != 1.0 {
		t.Errorf("p: expected 1.0, got %v", bg.schema["p"])
	}
}

// TestTextSchema verifies that Text produces the correct schema structure.
func TestTextSchema(t *testing.T) {
	g := Text(3, 10)
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("Text should return *BasicGenerator, got %T", g)
	}
	if bg.schema["type"] != "string" {
		t.Errorf("type: expected 'string', got %v", bg.schema["type"])
	}
	minSize, _ := ExtractInt(bg.schema["min_size"])
	if minSize != 3 {
		t.Errorf("min_size: expected 3, got %d", minSize)
	}
	maxSize, _ := ExtractInt(bg.schema["max_size"])
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
	g := Text(0, -1)
	bg := g.(*BasicGenerator)
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when maxSize < 0")
	}
	minSize, _ := ExtractInt(bg.schema["min_size"])
	if minSize != 0 {
		t.Errorf("min_size: expected 0, got %d", minSize)
	}
}

// TestBinarySchema verifies that Binary produces the correct schema structure.
func TestBinarySchema(t *testing.T) {
	g := Binary(1, 20)
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("Binary should return *BasicGenerator, got %T", g)
	}
	if bg.schema["type"] != "binary" {
		t.Errorf("type: expected 'binary', got %v", bg.schema["type"])
	}
	minSize, _ := ExtractInt(bg.schema["min_size"])
	if minSize != 1 {
		t.Errorf("min_size: expected 1, got %d", minSize)
	}
	maxSize, _ := ExtractInt(bg.schema["max_size"])
	if maxSize != 20 {
		t.Errorf("max_size: expected 20, got %d", maxSize)
	}
	// No transform needed — server returns []byte directly via CBOR byte strings.
	if bg.transform != nil {
		t.Error("Binary should have no transform")
	}
	// AsBasic returns itself.
	if bg.AsBasic() != bg {
		t.Error("AsBasic should return itself")
	}
}

// TestBinarySchemaNoMax verifies that Binary with maxSize<0 omits max_size from schema.
func TestBinarySchemaNoMax(t *testing.T) {
	g := Binary(0, -1)
	bg := g.(*BasicGenerator)
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when maxSize < 0")
	}
}

// TestIntegersFromSchema verifies that IntegersFrom produces the correct schema.
func TestIntegersFromSchema(t *testing.T) {
	minV := int64(-10)
	maxV := int64(10)
	g := IntegersFrom(&minV, &maxV)
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("IntegersFrom should return *BasicGenerator, got %T", g)
	}
	if bg.schema["type"] != "integer" {
		t.Errorf("type: expected 'integer', got %v", bg.schema["type"])
	}
	minVal, _ := ExtractInt(bg.schema["min_value"])
	maxVal, _ := ExtractInt(bg.schema["max_value"])
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
	bg := g.(*BasicGenerator)
	if _, hasMax := bg.schema["max_value"]; hasMax {
		t.Error("max_value should not be present when maxVal is nil")
	}
	minVal, _ := ExtractInt(bg.schema["min_value"])
	if minVal != 5 {
		t.Errorf("min_value: expected 5, got %d", minVal)
	}
}

// TestIntegersFromSchemaOnlyMax verifies that IntegersFrom with only a max bound omits min_value.
func TestIntegersFromSchemaOnlyMax(t *testing.T) {
	maxV := int64(99)
	g := IntegersFrom(nil, &maxV)
	bg := g.(*BasicGenerator)
	if _, hasMin := bg.schema["min_value"]; hasMin {
		t.Error("min_value should not be present when minVal is nil")
	}
	maxVal, _ := ExtractInt(bg.schema["max_value"])
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
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("Floats should return *BasicGenerator, got %T", g)
	}
	if bg.schema["type"] != "float" {
		t.Errorf("type: expected 'float', got %v", bg.schema["type"])
	}
	if bg.schema["allow_nan"] != false {
		t.Errorf("allow_nan: expected false, got %v", bg.schema["allow_nan"])
	}
	if bg.schema["allow_infinity"] != false {
		t.Errorf("allow_infinity: expected false, got %v", bg.schema["allow_infinity"])
	}
	if bg.schema["exclude_min"] != false {
		t.Errorf("exclude_min: expected false, got %v", bg.schema["exclude_min"])
	}
	if bg.schema["exclude_max"] != false {
		t.Errorf("exclude_max: expected false, got %v", bg.schema["exclude_max"])
	}
	minVal, _ := bg.schema["min_value"].(float64)
	maxVal, _ := bg.schema["max_value"].(float64)
	if minVal != 0.0 {
		t.Errorf("min_value: expected 0.0, got %v", minVal)
	}
	if maxVal != 1.0 {
		t.Errorf("max_value: expected 1.0, got %v", maxVal)
	}
	width, _ := ExtractInt(bg.schema["width"])
	if width != 64 {
		t.Errorf("width: expected 64, got %d", width)
	}
}

// TestFloatsSchemaUnbounded verifies that Floats with no bounds defaults allow_nan=true, allow_infinity=true.
func TestFloatsSchemaUnbounded(t *testing.T) {
	g := Floats(nil, nil, nil, nil, false, false)
	bg := g.(*BasicGenerator)
	if bg.schema["allow_nan"] != true {
		t.Errorf("allow_nan: expected true (no bounds), got %v", bg.schema["allow_nan"])
	}
	if bg.schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true (no bounds), got %v", bg.schema["allow_infinity"])
	}
	if _, hasMin := bg.schema["min_value"]; hasMin {
		t.Error("min_value should not be present when minVal is nil")
	}
	if _, hasMax := bg.schema["max_value"]; hasMax {
		t.Error("max_value should not be present when maxVal is nil")
	}
}

// TestFloatsSchemaOnlyMin verifies Floats with only min bound: allow_nan=false, allow_infinity=true.
func TestFloatsSchemaOnlyMin(t *testing.T) {
	minV := 0.0
	g := Floats(&minV, nil, nil, nil, false, false)
	bg := g.(*BasicGenerator)
	// has_min=true, has_max=false → allow_nan=false, allow_infinity=true
	if bg.schema["allow_nan"] != false {
		t.Errorf("allow_nan: expected false when min set, got %v", bg.schema["allow_nan"])
	}
	if bg.schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true when only min set, got %v", bg.schema["allow_infinity"])
	}
}

// TestFloatsSchemaOnlyMax verifies Floats with only max bound: allow_nan=false, allow_infinity=true.
func TestFloatsSchemaOnlyMax(t *testing.T) {
	maxV := 1.0
	g := Floats(nil, &maxV, nil, nil, false, false)
	bg := g.(*BasicGenerator)
	// has_min=false, has_max=true → allow_nan=false, allow_infinity=true
	if bg.schema["allow_nan"] != false {
		t.Errorf("allow_nan: expected false when max set, got %v", bg.schema["allow_nan"])
	}
	if bg.schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true when only max set, got %v", bg.schema["allow_infinity"])
	}
}

// TestFloatsSchemaExcludeBounds verifies that excludeMin/excludeMax are stored correctly.
func TestFloatsSchemaExcludeBounds(t *testing.T) {
	minV := 0.0
	maxV := 1.0
	falseV := false
	g := Floats(&minV, &maxV, &falseV, &falseV, true, true)
	bg := g.(*BasicGenerator)
	if bg.schema["exclude_min"] != true {
		t.Errorf("exclude_min: expected true, got %v", bg.schema["exclude_min"])
	}
	if bg.schema["exclude_max"] != true {
		t.Errorf("exclude_max: expected true, got %v", bg.schema["exclude_max"])
	}
}

// =============================================================================
// FlatMappedGenerator tests
// =============================================================================

// TestFlatMappedGeneratorAsBasicReturnsNil verifies that FlatMappedGenerator.AsBasic() returns nil.
func TestFlatMappedGeneratorAsBasicReturnsNil(t *testing.T) {
	gen := FlatMap(Integers(1, 5), func(v any) Generator {
		return Integers(0, 10)
	})
	if gen.AsBasic() != nil {
		t.Error("FlatMappedGenerator.AsBasic() should return nil")
	}
}

// TestFlatMappedGeneratorIsNotBasic verifies that FlatMap returns a *FlatMappedGenerator (not BasicGenerator).
func TestFlatMappedGeneratorIsNotBasic(t *testing.T) {
	gen := FlatMap(IntegersUnbounded(), func(v any) Generator {
		return IntegersUnbounded()
	})
	if _, ok := gen.(*FlatMappedGenerator); !ok {
		t.Fatalf("FlatMap should return *FlatMappedGenerator, got %T", gen)
	}
	// FlatMappedGenerator is never a BasicGenerator.
	if _, ok := gen.(*BasicGenerator); ok {
		t.Error("FlatMap result should not be a *BasicGenerator")
	}
}

// TestFlatMappedGeneratorMapReturnsMapped verifies that Map on FlatMappedGenerator returns a mappedGenerator.
func TestFlatMappedGeneratorMapReturnsMapped(t *testing.T) {
	gen := FlatMap(Integers(1, 5), func(v any) Generator {
		return Integers(0, 10)
	})
	mapped := gen.Map(func(v any) any { return v })
	if _, ok := mapped.(*mappedGenerator); !ok {
		t.Fatalf("Map on FlatMappedGenerator should return *mappedGenerator, got %T", mapped)
	}
}

// TestFlatMappedGeneratorGenerate verifies the low-level protocol:
// start_span(11), source generate, second generate, stop_span, mark_complete.
func TestFlatMappedGeneratorGenerate(t *testing.T) {
	var cmds []string
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel_id")])
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

		// Expect: start_span(11), generate(source), generate(second), stop_span, mark_complete
		for i := 0; i < 5; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := decodeCBOR(pl)
			mp, _ := ExtractDict(dec)
			cmd, _ := ExtractString(mp[any("command")])
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
			func(v any) Generator { return Integers(0, 100) },
		)
		v := Draw(gen)
		gotVal, _ = ExtractInt(v)
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
		decoded, _ := decodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel_id")])
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

		for i := 0; i < 5; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := decodeCBOR(pl)
			mp, _ := ExtractDict(dec)
			cmd, _ := ExtractString(mp[any("command")])
			if cmd == "start_span" {
				gotLabel, _ = ExtractInt(mp[any("label")])
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
		gen := FlatMap(Integers(0, 10), func(v any) Generator { return Integers(0, 10) })
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
// integers(1,5).flat_map(n => text(min=n, max=n)) always produces text of length in [1,5].
func TestFlatMappedGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	gen := FlatMap(Integers(1, 5), func(v any) Generator {
		n, _ := ExtractInt(v)
		return Text(int(n), int(n)) // exact length = n
	})
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		s, ok := v.(string)
		if !ok {
			panic(fmt.Sprintf("flat_map: expected string, got %T", v))
		}
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
	gen := FlatMap(Integers(2, 4), func(v any) Generator {
		n, _ := ExtractInt(v)
		sz := int(n)
		return Lists(Integers(0, 100), ListsOptions{MinSize: sz, MaxSize: sz})
	})
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		slice, ok := v.([]any)
		if !ok {
			panic(fmt.Sprintf("flat_map dependency: expected []any, got %T", v))
		}
		if len(slice) < 2 || len(slice) > 4 {
			panic(fmt.Sprintf("flat_map dependency: list length %d not in [2,4]", len(slice)))
		}
		for _, elem := range slice {
			n, err := ExtractInt(elem)
			if err != nil || n < 0 || n > 100 {
				panic(fmt.Sprintf("flat_map dependency: element %v not in [0,100]", elem))
			}
		}
	}, WithTestCases(50))
}
