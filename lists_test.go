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

// TestListsBasicElementSchema verifies that Lists on a BasicGenerator (no transform)
// produces a BasicGenerator with the correct list schema and no transform.
func TestListsBasicElementSchema(t *testing.T) {
	elem := Integers(0, 100)
	gen := Lists(elem, ListsOptions{MinSize: 2, MaxSize: 10})
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("Lists(basic) should return *BasicGenerator, got %T", gen)
	}
	if bg.schema["type"] != "list" {
		t.Errorf("schema type: expected 'list', got %v", bg.schema["type"])
	}
	elemSchema, ok := bg.schema["elements"].(map[string]any)
	if !ok {
		t.Fatalf("schema elements: expected map[string]any, got %T", bg.schema["elements"])
	}
	if elemSchema["type"] != "integer" {
		t.Errorf("elements type: expected 'integer', got %v", elemSchema["type"])
	}
	minV, _ := ExtractInt(bg.schema["min_size"])
	if minV != 2 {
		t.Errorf("min_size: expected 2, got %d", minV)
	}
	maxV, _ := ExtractInt(bg.schema["max_size"])
	if maxV != 10 {
		t.Errorf("max_size: expected 10, got %d", maxV)
	}
	if bg.transform != nil {
		t.Error("transform should be nil for basic element with no transform")
	}
}

// TestListsBasicElementNoMaxSchema verifies that when MaxSize < 0, max_size is omitted.
func TestListsBasicElementNoMaxSchema(t *testing.T) {
	elem := Integers(0, 100)
	gen := Lists(elem, ListsOptions{MinSize: 0, MaxSize: -1})
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("Lists(basic, no max) should return *BasicGenerator, got %T", gen)
	}
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when MaxSize < 0")
	}
}

// TestListsBasicElementWithTransformSchema verifies that Lists on a BasicGenerator with
// a transform applies the transform element-wise in the list transform.
func TestListsBasicElementWithTransformSchema(t *testing.T) {
	// Integers(0, 100) mapped to double: BasicGenerator with transform.
	elem := Integers(0, 100).Map(func(v any) any {
		n, _ := ExtractInt(v)
		return n * 2
	})
	gen := Lists(elem, ListsOptions{MinSize: 0, MaxSize: 5})
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("Lists(basic with transform) should return *BasicGenerator, got %T", gen)
	}
	if bg.schema["type"] != "list" {
		t.Errorf("schema type: expected 'list', got %v", bg.schema["type"])
	}
	if bg.transform == nil {
		t.Fatal("transform should not be nil for element with transform")
	}
	// Apply the transform to a raw []any and verify element-wise doubling.
	raw := []any{uint64(3), uint64(7), uint64(0)}
	result := bg.transform(raw)
	resultSlice, ok := result.([]any)
	if !ok {
		t.Fatalf("transform result should be []any, got %T", result)
	}
	if len(resultSlice) != 3 {
		t.Fatalf("transform result length: expected 3, got %d", len(resultSlice))
	}
	for i, want := range []int64{6, 14, 0} {
		got, _ := ExtractInt(resultSlice[i])
		if got != want {
			t.Errorf("transform result[%d]: expected %d, got %d", i, want, got)
		}
	}
}

// TestListsBasicElementWithTransformNonSlicePassthrough verifies that the list transform
// passes through non-slice values unchanged (defensive path in transform).
func TestListsBasicElementWithTransformNonSlicePassthrough(t *testing.T) {
	elem := Integers(0, 10).Map(func(v any) any { return v })
	gen := Lists(elem, ListsOptions{MinSize: 0, MaxSize: 5})
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("expected *BasicGenerator, got %T", gen)
	}
	// Pass a non-slice value to the transform — should be returned as-is.
	result := bg.transform("not-a-slice")
	if result != "not-a-slice" {
		t.Errorf("non-slice passthrough: expected 'not-a-slice', got %v", result)
	}
}

// TestListsNonBasicElementReturnsComposite verifies that Lists on a non-basic generator
// returns a compositeListGenerator (not a BasicGenerator).
func TestListsNonBasicElementReturnsComposite(t *testing.T) {
	// mappedGenerator is non-basic.
	inner := Integers(0, 10)
	nonBasic := &mappedGenerator{inner: inner, fn: func(v any) any { return v }}
	gen := Lists(nonBasic, ListsOptions{MinSize: 1, MaxSize: 3})
	if _, ok := gen.(*compositeListGenerator); !ok {
		t.Fatalf("Lists(non-basic) should return *compositeListGenerator, got %T", gen)
	}
	if gen.AsBasic() != nil {
		t.Error("compositeListGenerator.AsBasic() should return nil")
	}
}

// TestCompositeListGeneratorMap verifies that mapping a compositeListGenerator
// returns a mappedGenerator.
func TestCompositeListGeneratorMap(t *testing.T) {
	inner := Integers(0, 10)
	nonBasic := &mappedGenerator{inner: inner, fn: func(v any) any { return v }}
	gen := Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 3})
	mapped := gen.Map(func(v any) any { return v })
	if _, ok := mapped.(*mappedGenerator); !ok {
		t.Fatalf("Map on compositeListGenerator should return *mappedGenerator, got %T", mapped)
	}
}

// TestListsNegativeMinSizeClampedToZero verifies that a negative MinSize is clamped to 0.
func TestListsNegativeMinSizeClampedToZero(t *testing.T) {
	elem := Integers(0, 100)
	gen := Lists(elem, ListsOptions{MinSize: -5, MaxSize: 10})
	bg, ok := gen.(*BasicGenerator)
	if !ok {
		t.Fatalf("expected *BasicGenerator, got %T", gen)
	}
	minV, _ := ExtractInt(bg.schema["min_size"])
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
	// Non-basic generator: mappedGenerator wrapping integers
	inner := Integers(0, 10)
	nonBasic := &mappedGenerator{inner: inner, fn: func(v any) any {
		n, _ := ExtractInt(v)
		return n * 2
	}}
	gen := Lists(nonBasic, ListsOptions{MinSize: 1, MaxSize: 3})

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

		// Expect: start_span(list), new_collection, more→true, start_span(mapped), generate, stop_span, more→false, stop_span, mark_complete

		// start_span for list
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// new_collection
		ncID, ncPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		dec, _ := decodeCBOR(ncPayload)
		ncm, _ := ExtractDict(dec)
		cmd, _ := ExtractString(ncm[any("command")])
		if cmd != "new_collection" {
			t.Errorf("expected new_collection, got %s", cmd)
		}
		caseCh.SendReplyValue(ncID, "coll_proto") //nolint:errcheck

		// collection_more → true
		m1ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(m1ID, true) //nolint:errcheck

		// start_span for mappedGenerator
		mssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mssID, nil) //nolint:errcheck

		// generate (element)
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(3)) //nolint:errcheck

		// stop_span for mappedGenerator
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
	var gotResult []any
	err := cli.runTest("composite_list_proto", func() {
		result := Draw(gen)
		gotResult, _ = result.([]any)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if len(gotResult) != 1 {
		t.Fatalf("expected 1 element, got %d", len(gotResult))
	}
	v, _ := ExtractInt(gotResult[0])
	if v != 6 { // 3 * 2 = 6
		t.Errorf("expected 6 (3*2), got %d", v)
	}
}

// TestCompositeListGeneratorEmptyList tests that a composite list with no elements
// returns an empty (but non-nil via append behavior) slice.
func TestCompositeListGeneratorEmptyList(t *testing.T) {
	inner := Integers(0, 10)
	nonBasic := &mappedGenerator{inner: inner, fn: func(v any) any { return v }}
	gen := Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 3})

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
		slice, _ := result.([]any)
		gotLen = len(slice)
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
		slice, ok := xs.([]any)
		if !ok {
			panic(fmt.Sprintf("Lists: expected []any, got %T", xs))
		}
		for _, x := range slice {
			v, _ := ExtractInt(x)
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
		slice, ok := xs.([]any)
		if !ok {
			panic(fmt.Sprintf("Lists: expected []any, got %T", xs))
		}
		if len(slice) < 3 || len(slice) > 5 {
			panic(fmt.Sprintf("Lists: length %d out of [3, 5]", len(slice)))
		}
	}, WithTestCases(50))
}

// TestListsNonBasicElementE2E verifies that Lists with a non-basic element generator
// (filtered integers) always produces elements satisfying the filter condition.
func TestListsNonBasicElementE2E(t *testing.T) {
	hegelBinPath(t)
	// Filtered generator: integers in [0,100] keeping only even ones.
	filtered := Integers(0, 100).Map(func(v any) any {
		n, _ := ExtractInt(v)
		// Only keep even — transform to ensure "even" semantics (round to nearest even).
		return (n / 2) * 2
	})
	nonBasic := &mappedGenerator{inner: filtered, fn: func(v any) any { return v }}

	RunHegelTest("lists_non_basic_e2e", func() {
		xs := Draw(Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 5}))
		slice, ok := xs.([]any)
		if !ok {
			panic(fmt.Sprintf("Lists(non-basic): expected []any, got %T", xs))
		}
		for _, x := range slice {
			n, _ := ExtractInt(x)
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
		outerSlice, ok := outer.([]any)
		if !ok {
			panic(fmt.Sprintf("nested Lists: expected []any outer, got %T", outer))
		}
		for i, inner := range outerSlice {
			innerSlice, ok := inner.([]any)
			if !ok {
				panic(fmt.Sprintf("nested Lists[%d]: expected []any inner, got %T", i, inner))
			}
			for j, b := range innerSlice {
				if _, ok := b.(bool); !ok {
					panic(fmt.Sprintf("nested Lists[%d][%d]: expected bool, got %T", i, j, b))
				}
			}
		}
	}, WithTestCases(50))
}

// TestListsBasicWithTransformE2E verifies that Lists on a BasicGenerator with a transform
// applies the transform element-wise to the result.
func TestListsBasicWithTransformE2E(t *testing.T) {
	hegelBinPath(t)
	// Map Integers(0,10) → double. Lists wraps this into a list schema with element transform.
	doubled := Integers(0, 10).Map(func(v any) any {
		n, _ := ExtractInt(v)
		return n * 2
	})
	RunHegelTest("lists_basic_transform_e2e", func() {
		xs := Draw(Lists(doubled, ListsOptions{MinSize: 0, MaxSize: 5}))
		slice, ok := xs.([]any)
		if !ok {
			panic(fmt.Sprintf("Lists(basic+transform): expected []any, got %T", xs))
		}
		for _, x := range slice {
			n, _ := ExtractInt(x)
			if n%2 != 0 || n < 0 || n > 20 {
				panic(fmt.Sprintf("Lists(basic+transform): element %d should be even in [0,20]", n))
			}
		}
	}, WithTestCases(50))
}
