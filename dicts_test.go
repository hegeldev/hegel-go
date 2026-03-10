package hegel

// dicts_test.go tests the Dicts generator: schema structure, transform, basic/composite paths,
// StopTest handling, and e2e integration against the real hegel binary.

import (
	"fmt"
	"testing"
	"time"
	"unicode/utf8"
)

// =============================================================================
// Dicts: schema unit tests (no server)
// =============================================================================

// TestDictsBasicSchema verifies that Dicts with two basic generators produces
// a basicGenerator with a dict schema containing the expected fields.
func TestDictsBasicSchema(t *testing.T) {
	keys := Text(0, 5)
	vals := Integers[int64](0, 100)
	gen := Dicts(keys, vals, DictMaxSize(3))
	bg, ok := gen.(*basicGenerator[map[string]int64])
	if !ok {
		t.Fatalf("Dicts(basic, basic) should return *basicGenerator[map[string]int64], got %T", gen)
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

// TestDictsBasicSchemaNoMaxSize verifies that when HasMaxSize=false, max_size is omitted.
func TestDictsBasicSchemaNoMaxSize(t *testing.T) {
	gen := Dicts(Text(0, 5), Integers[int64](0, 100), DictMinSize(1))
	bg, ok := gen.(*basicGenerator[map[string]int64])
	if !ok {
		t.Fatalf("expected *basicGenerator[map[string]int64], got %T", gen)
	}
	if _, has := bg.schema["max_size"]; has {
		t.Error("max_size should not be present when HasMaxSize=false")
	}
}

// TestDictsBasicSchemaMinSize verifies that MinSize is propagated to the schema.
func TestDictsBasicSchemaMinSize(t *testing.T) {
	gen := Dicts(Text(0, 5), Integers[int64](0, 100), DictMinSize(2), DictMaxSize(5))
	bg, ok := gen.(*basicGenerator[map[string]int64])
	if !ok {
		t.Fatalf("expected *basicGenerator[map[string]int64], got %T", gen)
	}
	minSz, _ := extractCBORInt(bg.schema["min_size"])
	if minSz != 2 {
		t.Errorf("min_size: expected 2, got %d", minSz)
	}
}

// TestDictsBasicIsBasicGenerator verifies basicGenerator path via type assertion.
func TestDictsBasicIsBasicGenerator(t *testing.T) {
	gen := Dicts(Text(0, 5), Integers[int64](0, 100))
	if _, ok := gen.(*basicGenerator[map[string]int64]); !ok {
		t.Errorf("Dicts(basic,basic) should be *basicGenerator[map[string]int64], got %T", gen)
	}
}

// TestDictsCompositeIsNotBasicGenerator verifies compositeDictGenerator is not a basicGenerator.
func TestDictsCompositeIsNotBasicGenerator(t *testing.T) {
	// Use a non-basic key generator (mappedGenerator wrapping a basic generator)
	nonBasicKeys := &mappedGenerator[int64, int64]{
		inner: Integers[int64](0, 10),
		fn:    func(v int64) int64 { return v },
	}
	gen := Dicts(nonBasicKeys, Integers[int64](0, 10))
	if _, ok := gen.(*basicGenerator[map[int64]int64]); ok {
		t.Error("Dicts(non-basic, basic) should not be *basicGenerator")
	}
}

// TestDictsCompositeMap verifies that Map on a compositeDictGenerator returns a mappedGenerator.
func TestDictsCompositeMap(t *testing.T) {
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
	// If the server sends something unexpected, return an empty map.
	result := pairsToMap[string, int64]("not a slice", nil, nil)
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

// TestPairsToMapShortPair verifies that short pairs (len < 2) are skipped.
func TestPairsToMapShortPair(t *testing.T) {
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
// Dicts: basic path integration test (fake server)
// =============================================================================

// TestDictsBasicGenerateHappyPath verifies the basic-path dict generator
// sends the correct schema and applies the pair-to-map transform.
func TestDictsBasicGenerateHappyPath(t *testing.T) {
	// Server returns [[k1, v1], [k2, v2]] as the CBOR array of pairs.
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

		// Respond to generate with a pair list: [["key1", 42], ["key2", 7]]
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		pairList := []any{
			[]any{"key1", int64(42)},
			[]any{"key2", int64(7)},
		}
		caseCh.SendReplyValue(genID, pairList) //nolint:errcheck

		// Wait for mark_complete.
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotMap map[string]int64
	err := cli.runTest(func(s *TestCase) {
		gen := Dicts(Text(0, 5), Integers[int64](0, 100), DictMaxSize(3))
		gotMap = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotMap == nil {
		t.Fatal("expected map, got nil")
	}
	if gotMap["key1"] != 42 {
		t.Errorf("gotMap['key1']: expected 42, got %v", gotMap["key1"])
	}
	if gotMap["key2"] != 7 {
		t.Errorf("gotMap['key2']: expected 7, got %v", gotMap["key2"])
	}
}

// TestDictsBasicWithTransforms verifies that the basicGenerator path applies
// key and value transforms when the inner generators have transforms.
func TestDictsBasicWithTransforms(t *testing.T) {
	// Key generator with transform: text -> uppercase key
	// Value generator with transform: integer -> value * 2
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

		// Server returns [["hello", 5]]
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, []any{[]any{"hello", int64(5)}}) //nolint:errcheck

		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotMap map[string]int64

	// Build generators with transforms
	keyGen := Map(Text(0, 10), func(s string) string {
		result := ""
		for _, ch := range s {
			if ch >= 'a' && ch <= 'z' {
				result += string(rune(ch - 32))
			} else {
				result += string(ch)
			}
		}
		return result
	})
	valGen := Map(Integers[int64](0, 100), func(n int64) int64 {
		return n * 2
	})

	err := cli.runTest(func(s *TestCase) {
		gen := Dicts(keyGen, valGen, DictMaxSize(3))
		gotMap = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotMap == nil {
		t.Fatal("expected map, got nil")
	}
	// "hello" -> "HELLO" (uppercase), 5 -> 10 (*2)
	if gotMap["HELLO"] != int64(10) {
		t.Errorf("expected gotMap['HELLO']=10, got %v", gotMap["HELLO"])
	}
}

// =============================================================================
// Dicts: composite path integration test (fake server)
// =============================================================================

// TestDictsCompositeGenerateHappyPath verifies the composite dict generator
// sends new_collection, collection_more, and MapEntry spans correctly.
func TestDictsCompositeGenerateHappyPath(t *testing.T) {
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

		// Expected sequence:
		// 1. start_span (MAP)  [from discardableGroup]
		// 2. new_collection
		// 3. collection_more -> true
		// 4. start_span (MAP_ENTRY)  [from group(labelMapEntry, ...)]
		// 5. start_span (MAPPED)  [from mappedGenerator.draw -> group(labelMapped, ...)]
		// 6. generate key  [from inner Integers]
		// 7. stop_span (MAPPED)
		// 8. generate value  [from Integers[int64](0,100), no span]
		// 9. stop_span (MAP_ENTRY)
		// 10. collection_more -> false
		// 11. stop_span (MAP)
		// 12. mark_complete

		recvAndAck := func(reply any) {
			id, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
			caseCh.SendReplyValue(id, reply) //nolint:errcheck
		}

		recvAndAck(nil)         // 1. start_span (MAP)
		recvAndAck("coll_dc_1") // 2. new_collection -> server assigns name
		recvAndAck(true)        // 3. collection_more -> true (one element)
		recvAndAck(nil)         // 4. start_span (MAP_ENTRY)
		recvAndAck(nil)         // 5. start_span (MAPPED) for nonBasicKeys
		recvAndAck(int64(7))    // 6. generate key
		recvAndAck(nil)         // 7. stop_span (MAPPED)
		recvAndAck(int64(99))   // 8. generate value
		recvAndAck(nil)         // 9. stop_span (MAP_ENTRY)
		recvAndAck(false)       // 10. collection_more -> false
		recvAndAck(nil)         // 11. stop_span (MAP)

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotMap map[int64]int64
	err := cli.runTest(func(s *TestCase) {
		// Non-basic key generator (directly constructed mappedGenerator)
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 10),
			fn:    func(v int64) int64 { return v },
		}
		gen := Dicts(nonBasicKeys, Integers[int64](0, 100), DictMaxSize(2))
		gotMap = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotMap == nil {
		t.Fatal("expected map, got nil")
	}
	if len(gotMap) != 1 {
		t.Errorf("expected 1 entry, got %d", len(gotMap))
	}
	if gotMap[7] != 99 {
		t.Errorf("expected gotMap[7]=99, got %v", gotMap[7])
	}
}

// TestDictsCompositeNoMaxHappyPath verifies that composite dict with no max size
// defaults to minSize+10 for the collection.
func TestDictsCompositeNoMaxHappyPath(t *testing.T) {
	var gotCollMax int64
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

		// start_span (MAP)
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// new_collection -- capture max_size
		ncID, ncPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		ncDecoded, _ := decodeCBOR(ncPayload)
		ncMap, _ := extractCBORDict(ncDecoded)
		gotCollMax, _ = extractCBORInt(ncMap[any("max_size")])
		caseCh.SendReplyValue(ncID, "coll_no_max") //nolint:errcheck

		// collection_more -> false (empty)
		moreID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(moreID, false) //nolint:errcheck

		// stop_span (MAP)
		spID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck

		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest(func(s *TestCase) {
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 10),
			fn:    func(v int64) int64 { return v },
		}
		// No max size: hasMax=false, minSize=2 -> should use maxSz=2+10=12
		gen := Dicts(nonBasicKeys, Integers[int64](0, 100), DictMinSize(2))
		_ = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotCollMax != 12 {
		t.Errorf("composite dict no-max: expected maxSz=12 (minSize+10), got %d", gotCollMax)
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
	err := runHegel(func(s *TestCase) {
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 10),
			fn:    func(v int64) int64 { return v },
		}
		gen := Dicts(nonBasicKeys, Integers[int64](0, 100), DictMaxSize(3))
		_ = gen.draw(s)
	}, stderrNoteFn, nil)
	// StopTest causes test to be skipped or aborted, not fail
	_ = err
}

// TestDictsStopTestOnCollectionMore verifies that StopTest during collection_more
// aborts the test cleanly.
func TestDictsStopTestOnCollectionMore(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_collection_more")
	err := runHegel(func(s *TestCase) {
		nonBasicKeys := &mappedGenerator[int64, int64]{
			inner: Integers[int64](0, 10),
			fn:    func(v int64) int64 { return v },
		}
		gen := Dicts(nonBasicKeys, Integers[int64](0, 100), DictMaxSize(3))
		_ = gen.draw(s)
	}, stderrNoteFn, nil)
	_ = err
}

// =============================================================================
// Dicts: E2E tests against real hegel binary
// =============================================================================

// TestDictsBasicE2E verifies the basic Dicts generator produces maps with
// string keys and integer values within bounds.
func TestDictsBasicE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		gen := Dicts(Text(0, 5), Integers[int](0, 100), DictMaxSize(3))
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
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestDictsBasicWithBoundsE2E verifies that Dicts with min_size/max_size constraints
// produces maps with the right number of entries.
func TestDictsBasicWithBoundsE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		gen := Dicts(Integers[int](0, 10), Booleans(), DictMinSize(1), DictMaxSize(3))
		m := gen.draw(s)
		if len(m) < 1 || len(m) > 3 {
			panic(fmt.Sprintf("Dicts bounded: expected 1-3 entries, got %d", len(m)))
		}
		for k := range m {
			if k < 0 || k > 10 {
				panic(fmt.Sprintf("Dicts bounded: key %d out of [0,10]", k))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestDictsCompositeE2E verifies the composite Dicts generator (non-basic keys)
// produces valid maps.
func TestDictsCompositeE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
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
		gen := Dicts(nonBasicKeys, Just("val"), DictMaxSize(3))
		m := gen.draw(s)
		// All values must be "val"
		for k, val := range m {
			if val != "val" {
				panic(fmt.Sprintf("Dicts composite: expected value 'val', got %v for key %v", val, k))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}
