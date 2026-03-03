package hegel

// lists_test.go contains unit tests and e2e integration tests for the Lists generator.

import (
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// Lists generator unit tests
// =============================================================================

// TestListsBasicElementSchema verifies that Lists on a basic generator (no user transform)
// produces a basic Generator with the correct list schema.
func TestListsBasicElementSchema(t *testing.T) {
	elem := Integers(0, 100)
	gen := Lists(elem, ListsOptions{MinSize: 2, MaxSize: 10})
	if !gen.isBasic() {
		t.Fatal("Lists(basic) should return a basic generator")
	}
	if gen.schema["type"] != "list" {
		t.Errorf("schema type: expected 'list', got %v", gen.schema["type"])
	}
	elemSchema, ok := gen.schema["elements"].(map[string]any)
	if !ok {
		t.Fatalf("schema elements: expected map[string]any, got %T", gen.schema["elements"])
	}
	if elemSchema["type"] != "integer" {
		t.Errorf("elements type: expected 'integer', got %v", elemSchema["type"])
	}
	minV, _ := extractInt(gen.schema["min_size"])
	if minV != 2 {
		t.Errorf("min_size: expected 2, got %d", minV)
	}
	maxV, _ := extractInt(gen.schema["max_size"])
	if maxV != 10 {
		t.Errorf("max_size: expected 10, got %d", maxV)
	}
	if gen.transform == nil {
		t.Error("transform should not be nil for basic generator")
	}
}

// TestListsBasicElementNoMaxSchema verifies that when MaxSize < 0, max_size is omitted.
func TestListsBasicElementNoMaxSchema(t *testing.T) {
	elem := Integers(0, 100)
	gen := Lists(elem, ListsOptions{MinSize: 0, MaxSize: -1})
	if !gen.isBasic() {
		t.Fatal("Lists(basic, no max) should return a basic generator")
	}
	if _, hasMax := gen.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when MaxSize < 0")
	}
}

// TestListsBasicElementWithTransformSchema verifies that Lists on a basic generator with
// a transform applies the transform element-wise in the list transform.
func TestListsBasicElementWithTransformSchema(t *testing.T) {
	// Integers(0, 100) mapped to double: basic generator with transform.
	elem := Map(Integers(0, 100), func(v int64) int64 {
		return v * 2
	})
	gen := Lists(elem, ListsOptions{MinSize: 0, MaxSize: 5})
	if !gen.isBasic() {
		t.Fatal("Lists(basic with transform) should return a basic generator")
	}
	if gen.schema["type"] != "list" {
		t.Errorf("schema type: expected 'list', got %v", gen.schema["type"])
	}
	if gen.transform == nil {
		t.Fatal("transform should not be nil for element with transform")
	}
	// Apply the transform to a raw []any and verify element-wise doubling.
	raw := []any{uint64(3), uint64(7), uint64(0)}
	result := gen.transform(raw)
	if len(result) != 3 {
		t.Fatalf("transform result length: expected 3, got %d", len(result))
	}
	for i, want := range []int64{6, 14, 0} {
		if result[i] != want {
			t.Errorf("transform result[%d]: expected %d, got %d", i, want, result[i])
		}
	}
}

// TestListsBasicElementWithTransformNonSlicePassthrough verifies that the list transform
// returns nil for non-slice values (defensive path in transform).
func TestListsBasicElementWithTransformNonSlicePassthrough(t *testing.T) {
	elem := Map(Integers(0, 10), func(v int64) int64 { return v })
	gen := Lists(elem, ListsOptions{MinSize: 0, MaxSize: 5})
	if !gen.isBasic() {
		t.Fatal("expected basic generator")
	}
	// Pass a non-slice value to the transform — should return nil.
	result := gen.transform("not-a-slice")
	if result != nil {
		t.Errorf("non-slice passthrough: expected nil, got %v", result)
	}
}

// TestListsNonBasicElementReturnsComposite verifies that Lists on a non-basic generator
// returns a non-basic Generator.
func TestListsNonBasicElementReturnsComposite(t *testing.T) {
	// Filter makes a generator non-basic.
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	gen := Lists(nonBasic, ListsOptions{MinSize: 1, MaxSize: 3})
	if gen.isBasic() {
		t.Fatal("Lists(non-basic) should return a non-basic generator")
	}
}

// TestCompositeListGeneratorMap verifies that mapping a non-basic list generator
// returns a non-basic generator.
func TestCompositeListGeneratorMap(t *testing.T) {
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	gen := Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 3})
	mapped := Map(gen, func(v []int64) []int64 { return v })
	if mapped.isBasic() {
		t.Fatal("Map on non-basic list generator should return a non-basic generator")
	}
}

// TestListsNegativeMinSizeClampedToZero verifies that a negative MinSize is clamped to 0.
func TestListsNegativeMinSizeClampedToZero(t *testing.T) {
	elem := Integers(0, 100)
	gen := Lists(elem, ListsOptions{MinSize: -5, MaxSize: 10})
	if !gen.isBasic() {
		t.Fatal("expected basic generator")
	}
	minV, _ := extractInt(gen.schema["min_size"])
	if minV != 0 {
		t.Errorf("negative MinSize should be clamped to 0, got %d", minV)
	}
}

// =============================================================================
// compositeListGenerator direct protocol tests
// =============================================================================

// TestCompositeListGeneratorProtocol tests the collection protocol for composite lists
// using a fake server (no real hegel binary needed).
func TestCompositeListGeneratorProtocol(t *testing.T) {
	// Non-basic generator: Map on a filtered integers generator
	nonBasic := Map(Filter(Integers(0, 10), func(v int64) bool { return true }), func(v int64) int64 {
		return v * 2
	})
	gen := Lists(nonBasic, ListsOptions{MinSize: 1, MaxSize: 3})

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

		// Expect: start_span(list), new_collection, more→true, start_span(mapped), start_span(filter), generate, stop_span(filter), stop_span(mapped), more→false, stop_span(list), mark_complete

		// start_span for list
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// new_collection
		ncID, ncPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		dec, _ := DecodeCBOR(ncPayload)
		ncm, _ := extractDict(dec)
		cmd, _ := extractString(ncm[any("command")])
		if cmd != "new_collection" {
			t.Errorf("expected new_collection, got %s", cmd)
		}
		caseCh.SendReplyValue(ncID, "coll_proto") //nolint:errcheck

		// collection_more → true
		m1ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(m1ID, true) //nolint:errcheck

		// start_span for mapped (Map wraps Filter)
		mssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mssID, nil) //nolint:errcheck

		// start_span for filter (inner)
		fssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(fssID, nil) //nolint:errcheck

		// generate (element from inner Integers)
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(3)) //nolint:errcheck

		// stop_span for filter
		fspID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(fspID, nil) //nolint:errcheck

		// stop_span for mapped
		mspID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mspID, nil) //nolint:errcheck

		// collection_more → false
		m2ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(m2ID, false) //nolint:errcheck

		// stop_span for list
		spID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotResult []int64
	err := cli.runTest("composite_list_proto", func() {
		result := Draw(gen)
		gotResult = result
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if len(gotResult) != 1 {
		t.Fatalf("expected 1 element, got %d", len(gotResult))
	}
	if gotResult[0] != 6 { // 3 * 2 = 6
		t.Errorf("expected 6 (3*2), got %d", gotResult[0])
	}
}

// TestCompositeListGeneratorEmptyList tests that a composite list with no elements
// returns a nil slice.
func TestCompositeListGeneratorEmptyList(t *testing.T) {
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	gen := Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 3})

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

		// new_collection
		ncID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_empty") //nolint:errcheck

		// collection_more → false immediately
		moreID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(moreID, false) //nolint:errcheck

		// stop_span
		spID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotLen int = -1
	err := cli.runTest("composite_list_empty", func() {
		result := Draw(gen)
		gotLen = len(result)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotLen != 0 {
		t.Errorf("expected empty slice (len 0), got len %d", gotLen)
	}
}

// =============================================================================
// Lists e2e integration tests (real hegel binary)
// =============================================================================

// TestListsBasicIntegersE2E verifies that Lists(Integers(0,100)) always produces
// a list where every element is in [0, 100].
func TestListsBasicIntegersE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest("lists_basic_integers_e2e", func() {
		xs := Draw(Lists(Integers(0, 100), ListsOptions{MinSize: 0, MaxSize: 10}))
		for _, v := range xs {
			if v < 0 || v > 100 {
				panic(fmt.Sprintf("Lists: element %d out of range [0, 100]", v))
			}
		}
	}, WithTestCases(50))
}

// TestListsWithSizeBoundsE2E verifies that Lists with min_size and max_size constraints
// always produces slices whose length is within the specified bounds.
func TestListsWithSizeBoundsE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest("lists_with_bounds_e2e", func() {
		xs := Draw(Lists(Booleans(0.5), ListsOptions{MinSize: 3, MaxSize: 5}))
		if len(xs) < 3 || len(xs) > 5 {
			panic(fmt.Sprintf("Lists: length %d out of [3, 5]", len(xs)))
		}
	}, WithTestCases(50))
}

// TestListsNonBasicElementE2E verifies that Lists with a non-basic element generator
// (filtered integers) always produces elements satisfying the transform condition.
func TestListsNonBasicElementE2E(t *testing.T) {
	hegelBinPath(t)
	// Non-basic: Filter makes it composite, then Map to ensure even semantics.
	nonBasic := Map(Filter(Integers(0, 100), func(v int64) bool { return true }), func(n int64) int64 {
		return (n / 2) * 2
	})

	RunHegelTest("lists_non_basic_e2e", func() {
		xs := Draw(Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 5}))
		for _, n := range xs {
			if n%2 != 0 {
				panic(fmt.Sprintf("Lists(non-basic): expected even element, got %d", n))
			}
		}
	}, WithTestCases(50))
}

// TestListsNestedE2E verifies that nested lists work correctly:
// Lists(Lists(Booleans)) produces a list of lists of booleans.
func TestListsNestedE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest("lists_nested_e2e", func() {
		outer := Draw(Lists(Lists(Booleans(0.5), ListsOptions{MinSize: 0, MaxSize: 3}), ListsOptions{MinSize: 0, MaxSize: 3}))
		for i, inner := range outer {
			_ = i
			for j, b := range inner {
				_ = j
				_ = b // b is already bool, no assertion needed
			}
		}
	}, WithTestCases(50))
}

// TestListsBasicWithTransformE2E verifies that Lists on a basic generator with a transform
// applies the transform element-wise to the result.
func TestListsBasicWithTransformE2E(t *testing.T) {
	hegelBinPath(t)
	// Map Integers(0,10) → double. Lists wraps this into a list schema with element transform.
	doubled := Map(Integers(0, 10), func(v int64) int64 {
		return v * 2
	})
	RunHegelTest("lists_basic_transform_e2e", func() {
		xs := Draw(Lists(doubled, ListsOptions{MinSize: 0, MaxSize: 5}))
		for _, n := range xs {
			if n%2 != 0 || n < 0 || n > 20 {
				panic(fmt.Sprintf("Lists(basic+transform): element %d should be even in [0,20]", n))
			}
		}
	}, WithTestCases(50))
}
