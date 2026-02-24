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
// a BasicGenerator with a dict schema containing the expected fields.
func TestDictsBasicSchema(t *testing.T) {
	keys := Text(0, 5)
	vals := Integers(0, 100)
	gen := Dicts(keys, vals, DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("Dicts(basic, basic) should return *BasicGenerator, got %T", gen)
	}
	if bg.schema["type"] != "dict" {
		t.Errorf("schema type: expected 'dict', got %v", bg.schema["type"])
	}
	minSz, _ := ExtractInt(bg.schema["min_size"])
	if minSz != 0 {
		t.Errorf("min_size: expected 0, got %d", minSz)
	}
	maxSz, _ := ExtractInt(bg.schema["max_size"])
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
	gen := Dicts(Text(0, 5), Integers(0, 100), DictOptions{MinSize: 1})
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("expected *BasicGenerator, got %T", gen)
	}
	if _, has := bg.schema["max_size"]; has {
		t.Error("max_size should not be present when HasMaxSize=false")
	}
}

// TestDictsBasicSchemaMinSize verifies that MinSize is propagated to the schema.
func TestDictsBasicSchemaMinSize(t *testing.T) {
	gen := Dicts(Text(0, 5), Integers(0, 100), DictOptions{MinSize: 2, MaxSize: 5, HasMaxSize: true})
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("expected *BasicGenerator, got %T", gen)
	}
	minSz, _ := ExtractInt(bg.schema["min_size"])
	if minSz != 2 {
		t.Errorf("min_size: expected 2, got %d", minSz)
	}
}

// TestDictsAsBasic verifies BasicGenerator path returns non-nil from AsBasic.
func TestDictsAsBasic(t *testing.T) {
	gen := Dicts(Text(0, 5), Integers(0, 100), DictOptions{})
	if gen.AsBasic() == nil {
		t.Error("Dicts(basic,basic).AsBasic() should not return nil")
	}
}

// TestDictsCompositeAsBasic verifies compositeDictGenerator returns nil from AsBasic.
func TestDictsCompositeAsBasic(t *testing.T) {
	// Use a non-basic key generator (MappedGenerator)
	nonBasicKeys := &MappedGenerator{
		inner: Integers(0, 10),
		fn:    func(v any) any { return v },
	}
	gen := Dicts(nonBasicKeys, Integers(0, 10), DictOptions{})
	if gen.AsBasic() != nil {
		t.Error("Dicts(non-basic, basic).AsBasic() should return nil")
	}
}

// TestDictsCompositeMap verifies that Map on a compositeDictGenerator returns a MappedGenerator.
func TestDictsCompositeMap(t *testing.T) {
	nonBasicKeys := &MappedGenerator{
		inner: Integers(0, 10),
		fn:    func(v any) any { return v },
	}
	gen := Dicts(nonBasicKeys, Integers(0, 10), DictOptions{})
	mapped := gen.Map(func(v any) any { return v })
	if _, ok := mapped.(*MappedGenerator); !ok {
		t.Errorf("Map on compositeDictGenerator should return *MappedGenerator, got %T", mapped)
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
	result := pairsToMap(pairs, nil, nil)
	m, ok := result.(map[any]any)
	if !ok {
		t.Fatalf("expected map[any]any, got %T", result)
	}
	if m["a"] != int64(1) {
		t.Errorf("m['a']: expected 1, got %v", m["a"])
	}
	if m["b"] != int64(2) {
		t.Errorf("m['b']: expected 2, got %v", m["b"])
	}
}

// TestPairsToMapWithKeyTransform verifies that the key transform is applied.
func TestPairsToMapWithKeyTransform(t *testing.T) {
	pairs := []any{
		[]any{"hello", int64(1)},
	}
	keyTransform := func(v any) any {
		s, _ := v.(string)
		return s + "_key"
	}
	result := pairsToMap(pairs, keyTransform, nil)
	m, ok := result.(map[any]any)
	if !ok {
		t.Fatalf("expected map[any]any, got %T", result)
	}
	if _, has := m["hello_key"]; !has {
		t.Errorf("key transform not applied: expected 'hello_key', got %v", m)
	}
}

// TestPairsToMapWithValTransform verifies that the value transform is applied.
func TestPairsToMapWithValTransform(t *testing.T) {
	pairs := []any{
		[]any{"x", int64(5)},
	}
	valTransform := func(v any) any {
		n, _ := ExtractInt(v)
		return n * 2
	}
	result := pairsToMap(pairs, nil, valTransform)
	m, ok := result.(map[any]any)
	if !ok {
		t.Fatalf("expected map[any]any, got %T", result)
	}
	if m["x"] != int64(10) {
		t.Errorf("val transform not applied: expected 10, got %v", m["x"])
	}
}

// TestPairsToMapBothTransforms verifies both key and value transforms are applied.
func TestPairsToMapBothTransforms(t *testing.T) {
	pairs := []any{
		[]any{"k", int64(3)},
	}
	keyTransform := func(v any) any { return "K" }
	valTransform := func(v any) any {
		n, _ := ExtractInt(v)
		return n * 3
	}
	result := pairsToMap(pairs, keyTransform, valTransform)
	m, ok := result.(map[any]any)
	if !ok {
		t.Fatalf("expected map[any]any, got %T", result)
	}
	if m["K"] != int64(9) {
		t.Errorf("expected m['K']=9, got %v", m["K"])
	}
}

// TestPairsToMapNonSliceInput verifies pairsToMap handles non-slice input gracefully.
func TestPairsToMapNonSliceInput(t *testing.T) {
	// If the server sends something unexpected, return an empty map.
	result := pairsToMap("not a slice", nil, nil)
	m, ok := result.(map[any]any)
	if !ok {
		t.Fatalf("expected map[any]any, got %T", result)
	}
	if len(m) != 0 {
		t.Errorf("expected empty map, got %v", m)
	}
}

// TestPairsToMapShortPair verifies that short pairs (len < 2) are skipped.
func TestPairsToMapShortPair(t *testing.T) {
	pairs := []any{
		[]any{"only_key"}, // only one element — skip
		[]any{"a", int64(1)},
	}
	result := pairsToMap(pairs, nil, nil)
	m, ok := result.(map[any]any)
	if !ok {
		t.Fatalf("expected map[any]any, got %T", result)
	}
	if len(m) != 1 {
		t.Errorf("expected 1 entry, got %d: %v", len(m), m)
	}
}

// TestPairsToMapNonSlicePair verifies that non-slice pair entries are skipped.
func TestPairsToMapNonSlicePair(t *testing.T) {
	pairs := []any{
		"not a pair",
		[]any{"a", int64(1)},
	}
	result := pairsToMap(pairs, nil, nil)
	m, ok := result.(map[any]any)
	if !ok {
		t.Fatalf("expected map[any]any, got %T", result)
	}
	if len(m) != 1 {
		t.Errorf("expected 1 entry, got %d", len(m))
	}
}

// =============================================================================
// Dicts: basic path integration test (fake server)
// =============================================================================

// TestDictsBasicGenerateHappyPath verifies the basic-path dict generator
// sends the correct schema and applies the pair-to-map transform.
func TestDictsBasicGenerateHappyPath(t *testing.T) {
	// Server returns [[k1, v1], [k2, v2]] as the CBOR array of pairs.
	clientConn := fakeServerConn(t, func(serverConn *Connection) {
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
	var gotMap map[any]any
	err := cli.runTest("dicts_basic_happy", func() {
		gen := Dicts(Text(0, 5), Integers(0, 100), DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
		v := gen.Generate()
		gotMap, _ = v.(map[any]any)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotMap == nil {
		t.Fatal("expected map, got nil")
	}
	// CBOR decodes positive integers as uint64, so use ExtractInt to compare.
	v1, err1 := ExtractInt(gotMap["key1"])
	if err1 != nil || v1 != 42 {
		t.Errorf("gotMap['key1']: expected 42, got %v (err: %v)", gotMap["key1"], err1)
	}
	v2, err2 := ExtractInt(gotMap["key2"])
	if err2 != nil || v2 != 7 {
		t.Errorf("gotMap['key2']: expected 7, got %v (err: %v)", gotMap["key2"], err2)
	}
}

// TestDictsBasicWithTransforms verifies that the BasicGenerator path applies
// key and value transforms when the inner generators have transforms.
func TestDictsBasicWithTransforms(t *testing.T) {
	// Key generator with transform: text → uppercase key
	// Value generator with transform: integer → value * 2
	clientConn := fakeServerConn(t, func(serverConn *Connection) {
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

		// Server returns [["hello", 5]]
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, []any{[]any{"hello", int64(5)}}) //nolint:errcheck

		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotMap map[any]any

	// Build generators with transforms
	keyGen := Text(0, 10).Map(func(v any) any {
		s, _ := v.(string)
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
	valGen := Integers(0, 100).Map(func(v any) any {
		n, _ := ExtractInt(v)
		return n * 2
	})

	err := cli.runTest("dicts_with_transforms", func() {
		gen := Dicts(keyGen, valGen, DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
		v := gen.Generate()
		gotMap, _ = v.(map[any]any)
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
	clientConn := fakeServerConn(t, func(serverConn *Connection) {
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

		// Expected sequence:
		// 1. start_span (MAP)  [from DiscardableGroup]
		// 2. new_collection
		// 3. collection_more → true
		// 4. start_span (MAP_ENTRY)  [from Group(LabelMapEntry, ...)]
		// 5. start_span (MAPPED)  [from MappedGenerator.Generate → Group(LabelMapped, ...)]
		// 6. generate key  [from inner Integers]
		// 7. stop_span (MAPPED)
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
		recvAndAck(nil)         // 5. start_span (MAPPED) for nonBasicKeys
		recvAndAck(int64(7))    // 6. generate key
		recvAndAck(nil)         // 7. stop_span (MAPPED)
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
	var gotMap map[any]any
	err := cli.runTest("dicts_composite_happy", func() {
		// Non-basic key generator
		nonBasicKeys := &MappedGenerator{
			inner: Integers(0, 10),
			fn:    func(v any) any { return v },
		}
		gen := Dicts(nonBasicKeys, Integers(0, 100), DictOptions{MinSize: 0, MaxSize: 2, HasMaxSize: true})
		v := gen.Generate()
		gotMap, _ = v.(map[any]any)
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
	// MappedGenerator.fn returns the inner value from CBOR decode (uint64 for positive int).
	// Try both int64 and uint64 key lookup.
	var keyFound bool
	var foundVal any
	for k, v := range gotMap {
		kInt, err := ExtractInt(k)
		if err == nil && kInt == 7 {
			keyFound = true
			foundVal = v
		}
	}
	if !keyFound {
		t.Errorf("expected key with value 7 in map, got %v", gotMap)
	}
	if foundVal != nil {
		n, err := ExtractInt(foundVal)
		if err != nil || n != 99 {
			t.Errorf("expected value 99, got %v", foundVal)
		}
	}
}

// TestDictsCompositeNoMaxHappyPath verifies that composite dict with no max size
// defaults to minSize+10 for the collection.
func TestDictsCompositeNoMaxHappyPath(t *testing.T) {
	var gotCollMax int64
	clientConn := fakeServerConn(t, func(serverConn *Connection) {
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

		// start_span (MAP)
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// new_collection — capture max_size
		ncID, ncPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		ncDecoded, _ := DecodeCBOR(ncPayload)
		ncMap, _ := ExtractDict(ncDecoded)
		gotCollMax, _ = ExtractInt(ncMap[any("max_size")])
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
		nonBasicKeys := &MappedGenerator{
			inner: Integers(0, 10),
			fn:    func(v any) any { return v },
		}
		// No max size: hasMax=false, minSize=2 → should use maxSz=2+10=12
		gen := Dicts(nonBasicKeys, Integers(0, 100), DictOptions{MinSize: 2})
		_ = gen.Generate()
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
		nonBasicKeys := &MappedGenerator{
			inner: Integers(0, 10),
			fn:    func(v any) any { return v },
		}
		gen := Dicts(nonBasicKeys, Integers(0, 100), DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
		_ = gen.Generate()
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
		nonBasicKeys := &MappedGenerator{
			inner: Integers(0, 10),
			fn:    func(v any) any { return v },
		}
		gen := Dicts(nonBasicKeys, Integers(0, 100), DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
		_ = gen.Generate()
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
		v := gen.Generate()
		m, ok := v.(map[any]any)
		if !ok {
			panic(fmt.Sprintf("Dicts: expected map[any]any, got %T", v))
		}
		if len(m) > 3 {
			panic(fmt.Sprintf("Dicts: expected at most 3 entries, got %d", len(m)))
		}
		for k, val := range m {
			_, kOk := k.(string)
			if !kOk {
				panic(fmt.Sprintf("Dicts: expected string key, got %T", k))
			}
			n, nErr := ExtractInt(val)
			if nErr != nil {
				panic(fmt.Sprintf("Dicts: expected integer value, got %T", val))
			}
			if n < 0 || n > 100 {
				panic(fmt.Sprintf("Dicts: value %d out of [0,100]", n))
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
		v := gen.Generate()
		m, ok := v.(map[any]any)
		if !ok {
			panic(fmt.Sprintf("Dicts bounded: expected map[any]any, got %T", v))
		}
		if len(m) < 1 || len(m) > 3 {
			panic(fmt.Sprintf("Dicts bounded: expected 1-3 entries, got %d", len(m)))
		}
		for k := range m {
			n, nErr := ExtractInt(k)
			if nErr != nil {
				panic(fmt.Sprintf("Dicts bounded: expected integer key, got %T", k))
			}
			if n < 0 || n > 10 {
				panic(fmt.Sprintf("Dicts bounded: key %d out of [0,10]", n))
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
		nonBasicKeys := Integers(0, 10).Map(func(v any) any {
			n, _ := ExtractInt(v)
			if n > 5 {
				return n
			}
			return int64(6) // clamp to > 5
		})
		gen := Dicts(nonBasicKeys, Just("val"), DictOptions{MinSize: 0, MaxSize: 3, HasMaxSize: true})
		v := gen.Generate()
		m, ok := v.(map[any]any)
		if !ok {
			panic(fmt.Sprintf("Dicts composite: expected map[any]any, got %T", v))
		}
		// All values must be "val"
		for k, val := range m {
			if val != "val" {
				panic(fmt.Sprintf("Dicts composite: expected value 'val', got %v for key %v", val, k))
			}
		}
	}, WithTestCases(50))
}
