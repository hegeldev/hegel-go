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

// TestListsBasicElementSchema verifies that Lists on a basicGenerator[int64] (no transform)
// produces a basicGenerator[[]int64] with the correct list schema.
func TestListsBasicElementSchema(t *testing.T) {
	elem := Integers(0, 100)
	gen := Lists(elem, ListsOptions{MinSize: 2, MaxSize: 10})
	bg, ok := gen.(*basicGenerator[[]int64])
	if !ok {
		t.Fatalf("Lists(basic) should return *basicGenerator[[]int64], got %T", gen)
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
	minV, _ := extractCBORInt(bg.schema["min_size"])
	if minV != 2 {
		t.Errorf("min_size: expected 2, got %d", minV)
	}
	maxV, _ := extractCBORInt(bg.schema["max_size"])
	if maxV != 10 {
		t.Errorf("max_size: expected 10, got %d", maxV)
	}
}

// TestListsBasicElementNoMaxSchema verifies that when MaxSize < 0, max_size is omitted.
func TestListsBasicElementNoMaxSchema(t *testing.T) {
	elem := Integers(0, 100)
	gen := Lists(elem, ListsOptions{MinSize: 0, MaxSize: -1})
	bg, ok := gen.(*basicGenerator[[]int64])
	if !ok {
		t.Fatalf("Lists(basic, no max) should return *basicGenerator[[]int64], got %T", gen)
	}
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when MaxSize < 0")
	}
}

// TestListsBasicElementWithTransformSchema verifies that Lists on a basicGenerator with
// a transform applies the transform element-wise in the list transform.
func TestListsBasicElementWithTransformSchema(t *testing.T) {
	// Integers(0, 100) mapped to double: basicGenerator with transform.
	elem := Map(Integers(0, 100), func(n int64) int64 {
		return n * 2
	})
	gen := Lists(elem, ListsOptions{MinSize: 0, MaxSize: 5})
	bg, ok := gen.(*basicGenerator[[]int64])
	if !ok {
		t.Fatalf("Lists(basic with transform) should return *basicGenerator[[]int64], got %T", gen)
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
	elem := Map(Integers(0, 10), func(n int64) int64 { return n })
	gen := Lists(elem, ListsOptions{MinSize: 0, MaxSize: 5})
	bg, ok := gen.(*basicGenerator[[]int64])
	if !ok {
		t.Fatalf("expected *basicGenerator[[]int64], got %T", gen)
	}
	// Pass a non-slice value to the transform -- should return nil.
	result := bg.transform("not-a-slice")
	if result != nil {
		t.Errorf("non-slice passthrough: expected nil, got %v", result)
	}
}

// TestListsBasicElementNoTransformNonSlicePassthrough verifies that the list transform
// for a basic element with no transform returns nil for non-slice values.
func TestListsBasicElementNoTransformNonSlicePassthrough(t *testing.T) {
	elem := Booleans(0.5)
	gen := Lists(elem, ListsOptions{MinSize: 0, MaxSize: 5})
	bg, ok := gen.(*basicGenerator[[]bool])
	if !ok {
		t.Fatalf("expected *basicGenerator[[]bool], got %T", gen)
	}
	// Pass a non-slice value to the transform -- should return nil.
	result := bg.transform("not-a-slice")
	if result != nil {
		t.Errorf("non-slice passthrough: expected nil, got %v", result)
	}
}

// TestListsNonBasicElementReturnsComposite verifies that Lists on a non-basic generator
// returns a compositeListGenerator (not a basicGenerator).
func TestListsNonBasicElementReturnsComposite(t *testing.T) {
	// mappedGenerator is non-basic.
	inner := Integers(0, 10)
	nonBasic := &mappedGenerator[int64, int64]{inner: inner, fn: func(v int64) int64 { return v }}
	gen := Lists(nonBasic, ListsOptions{MinSize: 1, MaxSize: 3})
	if _, ok := gen.(*compositeListGenerator[int64]); !ok {
		t.Fatalf("Lists(non-basic) should return *compositeListGenerator[int64], got %T", gen)
	}
}

// TestListsNegativeMinSizeClampedToZero verifies that a negative MinSize is clamped to 0.
func TestListsNegativeMinSizeClampedToZero(t *testing.T) {
	elem := Integers(0, 100)
	gen := Lists(elem, ListsOptions{MinSize: -5, MaxSize: 10})
	bg, ok := gen.(*basicGenerator[[]int64])
	if !ok {
		t.Fatalf("expected *basicGenerator[[]int64], got %T", gen)
	}
	minV, _ := extractCBORInt(bg.schema["min_size"])
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
	nonBasic := &mappedGenerator[int64, int64]{inner: inner, fn: func(v int64) int64 {
		return v * 2
	}}
	gen := Lists(nonBasic, ListsOptions{MinSize: 1, MaxSize: 3})

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

		// Expect: start_span(list), new_collection, more->true, start_span(mapped), generate, stop_span, more->false, stop_span, mark_complete

		// start_span for list
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// new_collection
		ncID, ncPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		dec, _ := decodeCBOR(ncPayload)
		ncm, _ := extractCBORDict(dec)
		cmd, _ := extractCBORString(ncm[any("command")])
		if cmd != "new_collection" {
			t.Errorf("expected new_collection, got %s", cmd)
		}
		caseCh.SendReplyValue(ncID, "coll_proto") //nolint:errcheck

		// collection_more -> true
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

		// collection_more -> false
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
	err := cli.runTest("composite_list_proto", func(s *TestCase) {
		gotResult = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
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
// returns an empty (but non-nil via append behavior) slice.
func TestCompositeListGeneratorEmptyList(t *testing.T) {
	inner := Integers(0, 10)
	nonBasic := &mappedGenerator[int64, int64]{inner: inner, fn: func(v int64) int64 { return v }}
	gen := Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 3})

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

		// start_span
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// new_collection
		ncID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_empty") //nolint:errcheck

		// collection_more -> false immediately
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
	err := cli.runTest("composite_list_empty", func(s *TestCase) {
		result := gen.draw(s)
		gotLen = len(result)
	}, runOptions{testCases: 1}, stderrNoteFn)
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
	if _err := runHegel("lists_basic_integers_e2e", func(s *TestCase) {
		xs := Lists(Integers(0, 100), ListsOptions{MinSize: 0, MaxSize: 10}).draw(s)
		for _, x := range xs {
			if x < 0 || x > 100 {
				panic(fmt.Sprintf("Lists: element %d out of range [0, 100]", x))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestListsWithSizeBoundsE2E verifies that Lists with min_size and max_size constraints
// always produces slices whose length is within the specified bounds.
func TestListsWithSizeBoundsE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel("lists_with_bounds_e2e", func(s *TestCase) {
		xs := Lists(Booleans(0.5), ListsOptions{MinSize: 3, MaxSize: 5}).draw(s)
		if len(xs) < 3 || len(xs) > 5 {
			panic(fmt.Sprintf("Lists: length %d out of [3, 5]", len(xs)))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestListsNonBasicElementE2E verifies that Lists with a non-basic element generator
// (mapped integers) always produces elements satisfying the mapped condition.
func TestListsNonBasicElementE2E(t *testing.T) {
	hegelBinPath(t)
	// Mapped generator: integers in [0,100] then round to nearest even.
	mapped := Map(Integers(0, 100), func(n int64) int64 {
		return (n / 2) * 2
	})
	nonBasic := &mappedGenerator[int64, int64]{inner: mapped, fn: func(v int64) int64 { return v }}

	if _err := runHegel("lists_non_basic_e2e", func(s *TestCase) {
		xs := Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 5}).draw(s)
		for _, x := range xs {
			if x%2 != 0 {
				panic(fmt.Sprintf("Lists(non-basic): expected even element, got %d", x))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestListsNestedE2E verifies that nested lists work correctly:
// Lists(Lists(Booleans)) produces a list of lists of booleans.
func TestListsNestedE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel("lists_nested_e2e", func(s *TestCase) {
		outer := Lists(Lists(Booleans(0.5), ListsOptions{MinSize: 0, MaxSize: 3}), ListsOptions{MinSize: 0, MaxSize: 3}).draw(s)
		for i, inner := range outer {
			for j, b := range inner {
				// b is already bool due to typed generators; verify it is true or false.
				if b != true && b != false {
					panic(fmt.Sprintf("nested Lists[%d][%d]: expected bool, got %v", i, j, b))
				}
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestListsBasicWithTransformE2E verifies that Lists on a basicGenerator with a transform
// applies the transform element-wise to the result.
func TestListsBasicWithTransformE2E(t *testing.T) {
	hegelBinPath(t)
	// Map Integers(0,10) -> double. Lists wraps this into a list schema with element transform.
	doubled := Map(Integers(0, 10), func(n int64) int64 {
		return n * 2
	})
	if _err := runHegel("lists_basic_transform_e2e", func(s *TestCase) {
		xs := Lists(doubled, ListsOptions{MinSize: 0, MaxSize: 5}).draw(s)
		for _, x := range xs {
			if x%2 != 0 || x < 0 || x > 20 {
				panic(fmt.Sprintf("Lists(basic+transform): element %d should be even in [0,20]", x))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}
