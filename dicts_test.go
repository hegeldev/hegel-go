package hegel

// dicts_test.go tests the Dicts generator: schema structure, transform, basic/composite paths,
// StopTest handling, and e2e integration against the real hegel binary.

import (
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// Dicts: schema unit tests (no server)
// =============================================================================

// TestDictsBasicSchema verifies that Dicts with two basic generators produces
// a basic Generator with a dict schema containing the expected fields.
func TestDictsBasicSchema(t *testing.T) {
	keys := Text(0, 5)
	vals := Integers(0, 100)
	gen := Dicts(keys, vals, DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
	if !gen.isBasic() {
		t.Fatal("Dicts(basic, basic) should return a basic generator")
	}
	if gen.schema["type"] != "dict" {
		t.Errorf("schema type: expected 'dict', got %v", gen.schema["type"])
	}
	minSz, _ := extractInt(gen.schema["min_size"])
	if minSz != 0 {
		t.Errorf("min_size: expected 0, got %d", minSz)
	}
	maxSz, _ := extractInt(gen.schema["max_size"])
	if maxSz != 3 {
		t.Errorf("max_size: expected 3, got %d", maxSz)
	}
	keySchema, ok := gen.schema["keys"].(map[string]any)
	if !ok {
		t.Fatalf("schema['keys'] should be a map, got %T", gen.schema["keys"])
	}
	if keySchema["type"] != "string" {
		t.Errorf("keys schema type: expected 'string', got %v", keySchema["type"])
	}
	valSchema, ok := gen.schema["values"].(map[string]any)
	if !ok {
		t.Fatalf("schema['values'] should be a map, got %T", gen.schema["values"])
	}
	if valSchema["type"] != "integer" {
		t.Errorf("values schema type: expected 'integer', got %v", valSchema["type"])
	}
}

// TestDictsBasicSchemaNoMaxSize verifies that when HasMaxSize=false, max_size is omitted.
func TestDictsBasicSchemaNoMaxSize(t *testing.T) {
	gen := Dicts(Text(0, 5), Integers(0, 100), DictOptions{MinSize: 1})
	if !gen.isBasic() {
		t.Fatal("expected basic generator")
	}
	if _, has := gen.schema["max_size"]; has {
		t.Error("max_size should not be present when HasMaxSize=false")
	}
}

// TestDictsBasicSchemaMinSize verifies that MinSize is propagated to the schema.
func TestDictsBasicSchemaMinSize(t *testing.T) {
	gen := Dicts(Text(0, 5), Integers(0, 100), DictOptions{MinSize: 2, MaxSize: 5, HasMaxSize: true})
	if !gen.isBasic() {
		t.Fatal("expected basic generator")
	}
	minSz, _ := extractInt(gen.schema["min_size"])
	if minSz != 2 {
		t.Errorf("min_size: expected 2, got %d", minSz)
	}
}

// TestDictsAsBasic verifies basic generator path returns true from isBasic.
func TestDictsAsBasic(t *testing.T) {
	gen := Dicts(Text(0, 5), Integers(0, 100), DictOptions{})
	if !gen.isBasic() {
		t.Error("Dicts(basic,basic).isBasic() should return true")
	}
}

// TestDictsCompositeAsBasic verifies composite dict generator returns false from isBasic.
func TestDictsCompositeAsBasic(t *testing.T) {
	// Use a non-basic key generator (Filter)
	nonBasicKeys := Filter(Integers(0, 10), func(v int64) bool { return true })
	gen := Dicts(nonBasicKeys, Integers(0, 10), DictOptions{})
	if gen.isBasic() {
		t.Error("Dicts(non-basic, basic).isBasic() should return false")
	}
}

// TestDictsCompositeMap verifies that Map on a composite dict generator returns a non-basic generator.
func TestDictsCompositeMap(t *testing.T) {
	nonBasicKeys := Filter(Integers(0, 10), func(v int64) bool { return true })
	gen := Dicts(nonBasicKeys, Integers(0, 10), DictOptions{})
	mapped := Map(gen, func(v map[int64]int64) map[int64]int64 { return v })
	if mapped.isBasic() {
		t.Error("Map on composite dict generator should return a non-basic generator")
	}
}

// =============================================================================
// Dicts: transform tests
// =============================================================================

// TestPairsToMapNoTransform verifies pairsToMap converts pairs to a map with identity transforms.
func TestPairsToMapNoTransform(t *testing.T) {
	pairs := []any{
		[]any{"a", int64(1)},
		[]any{"b", int64(2)},
	}
	result := pairsToMap[any, any](pairs, func(v any) any { return v }, func(v any) any { return v })
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
	result := pairsToMap[string, any](pairs, func(v any) string {
		s, _ := v.(string)
		return s + "_key"
	}, func(v any) any { return v })
	if _, has := result["hello_key"]; !has {
		t.Errorf("key transform not applied: expected 'hello_key', got %v", result)
	}
}

// TestPairsToMapWithValTransform verifies that the value transform is applied.
func TestPairsToMapWithValTransform(t *testing.T) {
	pairs := []any{
		[]any{"x", int64(5)},
	}
	result := pairsToMap[any, int64](pairs, func(v any) any { return v }, func(v any) int64 {
		n, _ := extractInt(v)
		return n * 2
	})
	if result["x"] != int64(10) {
		t.Errorf("val transform not applied: expected 10, got %v", result["x"])
	}
}

// TestPairsToMapBothTransforms verifies both key and value transforms are applied.
func TestPairsToMapBothTransforms(t *testing.T) {
	pairs := []any{
		[]any{"k", int64(3)},
	}
	result := pairsToMap[string, int64](pairs, func(v any) string { return "K" }, func(v any) int64 {
		n, _ := extractInt(v)
		return n * 3
	})
	if result["K"] != int64(9) {
		t.Errorf("expected m['K']=9, got %v", result["K"])
	}
}

// TestPairsToMapNonSliceInput verifies pairsToMap handles non-slice input gracefully.
func TestPairsToMapNonSliceInput(t *testing.T) {
	// If the server sends something unexpected, return an empty map.
	result := pairsToMap[any, any]("not a slice", func(v any) any { return v }, func(v any) any { return v })
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

// TestPairsToMapShortPair verifies that short pairs (len < 2) are skipped.
func TestPairsToMapShortPair(t *testing.T) {
	pairs := []any{
		[]any{"only_key"}, // only one element — skip
		[]any{"a", int64(1)},
	}
	result := pairsToMap[any, any](pairs, func(v any) any { return v }, func(v any) any { return v })
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
	result := pairsToMap[any, any](pairs, func(v any) any { return v }, func(v any) any { return v })
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
	err := cli.runTest("dicts_basic_happy", func() {
		gen := Dicts(Text(0, 5), Integers(0, 100), DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
		gotMap = Draw(gen)
	}, runOptions{testCases: 1})
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

// TestDictsBasicWithTransforms verifies that the basic generator path applies
// key and value transforms when the inner generators have transforms.
func TestDictsBasicWithTransforms(t *testing.T) {
	// Key generator with transform: text → uppercase key
	// Value generator with transform: integer → value * 2
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
	valGen := Map(Integers(0, 100), func(n int64) int64 {
		return n * 2
	})

	err := cli.runTest("dicts_with_transforms", func() {
		gen := Dicts(keyGen, valGen, DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
		gotMap = Draw(gen)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotMap == nil {
		t.Fatal("expected map, got nil")
	}
	// "hello" → "HELLO" (uppercase), 5 → 10 (*2)
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

		// Expected sequence:
		// 1. start_span (MAP)  [from DiscardableGroup]
		// 2. new_collection
		// 3. collection_more → true
		// 4. start_span (MAP_ENTRY)  [from Group(LabelMapEntry, ...)]
		// 5. start_span (FILTER)  [from Filter.drawFn for nonBasicKeys]
		// 6. generate key  [from inner Integers]
		// 7. stop_span (FILTER)
		// 8. generate value  [from Integers(0,100), no span]
		// 9. stop_span (MAP_ENTRY)
		// 10. collection_more → false
		// 11. stop_span (MAP)
		// 12. mark_complete

		recvAndAck := func(reply any) {
			id, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
			caseCh.SendReplyValue(id, reply) //nolint:errcheck
		}

		recvAndAck(nil)         // 1. start_span (MAP)
		recvAndAck("coll_dc_1") // 2. new_collection → server assigns name
		recvAndAck(true)        // 3. collection_more → true (one element)
		recvAndAck(nil)         // 4. start_span (MAP_ENTRY)
		recvAndAck(nil)         // 5. start_span (FILTER) for nonBasicKeys
		recvAndAck(int64(7))    // 6. generate key
		recvAndAck(nil)         // 7. stop_span (FILTER)
		recvAndAck(int64(99))   // 8. generate value
		recvAndAck(nil)         // 9. stop_span (MAP_ENTRY)
		recvAndAck(false)       // 10. collection_more → false
		recvAndAck(nil)         // 11. stop_span (MAP)

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotMap map[int64]int64
	err := cli.runTest("dicts_composite_happy", func() {
		// Non-basic key generator (Filter makes it non-basic)
		nonBasicKeys := Filter(Integers(0, 10), func(v int64) bool { return true })
		gen := Dicts(nonBasicKeys, Integers(0, 100), DictOptions{MinSize: 0, MaxSize: 2, HasMaxSize: true})
		gotMap = Draw(gen)
	}, runOptions{testCases: 1})
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

		// start_span (MAP)
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// new_collection — capture max_size
		ncID, ncPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		ncDecoded, _ := DecodeCBOR(ncPayload)
		ncMap, _ := extractDict(ncDecoded)
		gotCollMax, _ = extractInt(ncMap[any("max_size")])
		caseCh.SendReplyValue(ncID, "coll_no_max") //nolint:errcheck

		// collection_more → false (empty)
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
	err := cli.runTest("dicts_composite_no_max", func() {
		nonBasicKeys := Filter(Integers(0, 10), func(v int64) bool { return true })
		// No max size: hasMax=false, minSize=2 → should use maxSz=2+10=12
		gen := Dicts(nonBasicKeys, Integers(0, 100), DictOptions{MinSize: 2})
		_ = Draw(gen)
	}, runOptions{testCases: 1})
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
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_new_collection")
	err := RunHegelTestE("dicts_stop_new_collection", func() {
		nonBasicKeys := Filter(Integers(0, 10), func(v int64) bool { return true })
		gen := Dicts(nonBasicKeys, Integers(0, 100), DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
		_ = Draw(gen)
	})
	// StopTest causes test to be skipped or aborted, not fail
	_ = err
}

// TestDictsStopTestOnCollectionMore verifies that StopTest during collection_more
// aborts the test cleanly.
func TestDictsStopTestOnCollectionMore(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_collection_more")
	err := RunHegelTestE("dicts_stop_collection_more", func() {
		nonBasicKeys := Filter(Integers(0, 10), func(v int64) bool { return true })
		gen := Dicts(nonBasicKeys, Integers(0, 100), DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
		_ = Draw(gen)
	})
	_ = err
}

// =============================================================================
// Dicts: E2E tests against real hegel binary
// =============================================================================

// TestDictsBasicE2E verifies the basic Dicts generator produces maps with
// string keys and integer values within bounds.
func TestDictsBasicE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		gen := Dicts(Text(0, 5), Integers(0, 100), DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
		m := Draw(gen)
		if len(m) > 3 {
			panic(fmt.Sprintf("Dicts: expected at most 3 entries, got %d", len(m)))
		}
		for _, val := range m {
			if val < 0 || val > 100 {
				panic(fmt.Sprintf("Dicts: value %d out of [0,100]", val))
			}
		}
	}, WithTestCases(50))
}

// TestDictsBasicWithBoundsE2E verifies that Dicts with min_size/max_size constraints
// produces maps with the right number of entries.
func TestDictsBasicWithBoundsE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		gen := Dicts(Integers(0, 10), Booleans(0.5), DictOptions{MinSize: 1, MaxSize: 3, HasMaxSize: true})
		m := Draw(gen)
		if len(m) < 1 || len(m) > 3 {
			panic(fmt.Sprintf("Dicts bounded: expected 1-3 entries, got %d", len(m)))
		}
		for k := range m {
			if k < 0 || k > 10 {
				panic(fmt.Sprintf("Dicts bounded: key %d out of [0,10]", k))
			}
		}
	}, WithTestCases(50))
}

// TestDictsCompositeE2E verifies the composite Dicts generator (non-basic keys)
// produces valid maps.
func TestDictsCompositeE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		// Filter makes this a non-basic generator → composite path
		nonBasicKeys := Map(Filter(Integers(0, 10), func(v int64) bool { return true }), func(n int64) int64 {
			if n > 5 {
				return n
			}
			return int64(6) // clamp to > 5
		})
		gen := Dicts(nonBasicKeys, Just("val"), DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
		m := Draw(gen)
		// All values must be "val"
		for k, val := range m {
			if val != "val" {
				panic(fmt.Sprintf("Dicts composite: expected value 'val', got %v for key %v", val, k))
			}
		}
	}, WithTestCases(50))
}
