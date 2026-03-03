package hegel

// filter_test.go tests the Filter free function and filtered generator behavior.

import (
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// Filter function unit tests — verify return types
// =============================================================================

// TestBasicGeneratorFilterReturnsfilteredGenerator verifies that calling Filter
// on a basic generator returns a non-basic generator.
func TestBasicGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	gen := Integers(0, 100)
	filtered := Filter(gen, func(v int64) bool { return true })
	if filtered.isBasic() {
		t.Error("Filter should return a non-basic generator")
	}
}

// TestMappedGeneratorFilterReturnsfilteredGenerator verifies that calling Filter
// on a mapped generator returns a non-basic generator.
func TestMappedGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	inner := Integers(0, 100)
	mapped := Map(inner, func(v int64) int64 { return v })
	filtered := Filter(mapped, func(v int64) bool { return true })
	if filtered.isBasic() {
		t.Error("Filter should return non-basic")
	}
}

// TestFilteredGeneratorAsBasicReturnsNil verifies that a filtered generator is not basic.
func TestFilteredGeneratorAsBasicReturnsNil(t *testing.T) {
	gen := Integers(0, 100)
	fg := Filter(gen, func(v int64) bool { return true })
	if fg.isBasic() {
		t.Error("filtered generator should not be basic")
	}
}

// TestFilteredGeneratorFilterChainsfilteredGenerators verifies that calling Filter
// on a filtered generator returns another non-basic generator (chained filtering).
func TestFilteredGeneratorFilterChainsfilteredGenerators(t *testing.T) {
	gen := Integers(0, 100)
	fg := Filter(gen, func(v int64) bool { return true })
	fg2 := Filter(fg, func(v int64) bool { return true })
	if fg2.isBasic() {
		t.Error("chained Filter should be non-basic")
	}
}

// TestFilteredGeneratorMapReturnsmappedGenerator verifies that calling Map
// on a filtered generator returns a non-basic generator.
func TestFilteredGeneratorMapReturnsmappedGenerator(t *testing.T) {
	gen := Integers(0, 100)
	fg := Filter(gen, func(v int64) bool { return true })
	mapped := Map(fg, func(v int64) int64 { return v })
	if mapped.isBasic() {
		t.Error("Map on filtered (non-basic) should be non-basic")
	}
}

// =============================================================================
// Filter function on composite generators — verify return types
// =============================================================================

// TestCompositeListGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a non-basic list generator returns a non-basic generator.
func TestCompositeListGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	// A non-basic list is produced when elements are non-basic.
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	listGen := Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 5})
	filtered := Filter(listGen, func(v []int64) bool { return true })
	if filtered.isBasic() {
		t.Error("Filter on non-basic list generator should return non-basic")
	}
}

// TestCompositeDictGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a non-basic dict generator returns a non-basic generator.
func TestCompositeDictGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	// A non-basic dict is produced when key or value is non-basic.
	nonBasicKeys := Filter(Integers(0, 10), func(v int64) bool { return true })
	dictGen := Dicts(nonBasicKeys, Integers(0, 100), DictOptions{MinSize: 0})
	filtered := Filter(dictGen, func(v map[int64]int64) bool { return true })
	if filtered.isBasic() {
		t.Error("Filter on non-basic dict generator should return non-basic")
	}
}

// TestCompositeOneOfGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a non-basic oneOf generator returns a non-basic generator.
func TestCompositeOneOfGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	// A non-basic oneOf is produced when any branch is non-basic.
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	oneOf := OneOf(nonBasic, Filter(Integers(0, 5), func(v int64) bool { return true }))
	filtered := Filter(oneOf, func(v int64) bool { return true })
	if filtered.isBasic() {
		t.Error("Filter on non-basic oneOf generator should return non-basic")
	}
}

// TestCompositeTupleGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a non-basic tuple generator returns a non-basic generator.
func TestCompositeTupleGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	// A non-basic tuple is produced when any element is non-basic.
	nonBasic := Filter(Integers(0, 10), func(v int64) bool { return true })
	tupleGen := Tuples2(nonBasic, Booleans(0.5))
	filtered := Filter(tupleGen, func(v Tuple2[int64, bool]) bool { return true })
	if filtered.isBasic() {
		t.Error("Filter on non-basic tuple generator should return non-basic")
	}
}

// TestFlatMappedGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a flat-mapped generator returns a non-basic generator.
func TestFlatMappedGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	flatGen := FlatMap(Integers(1, 5), func(v int64) *Generator[int64] {
		return Integers(0, v)
	})
	filtered := Filter(flatGen, func(v int64) bool { return true })
	if filtered.isBasic() {
		t.Error("Filter on flat-mapped generator should return non-basic")
	}
}

// =============================================================================
// filtered generator Generate tests using real hegel binary
// =============================================================================

// TestFilteredGeneratorGeneratePredicatePassesFirstTry verifies that when the
// predicate passes on the first attempt, the value is returned immediately.
func TestFilteredGeneratorGeneratePredicatePassesFirstTry(t *testing.T) {
	hegelBinPath(t)
	// Filter that always passes: every value is accepted on first try.
	gen := Filter(Integers(0, 100), func(v int64) bool { return true })
	RunHegelTest(t.Name(), func() {
		n := Draw(gen)
		if n < 0 || n > 100 {
			panic(fmt.Sprintf("Filter: expected [0,100], got %d", n))
		}
	}, WithTestCases(30))
}

// TestFilteredGeneratorGenerateWithRealPredicate verifies that Filter correctly
// filters values: only even numbers should pass.
func TestFilteredGeneratorGenerateWithRealPredicate(t *testing.T) {
	hegelBinPath(t)
	// Filter integers [0,50] keeping only even ones.
	gen := Filter(Integers(0, 50), func(n int64) bool { return n%2 == 0 })
	RunHegelTest(t.Name(), func() {
		n := Draw(gen)
		if n%2 != 0 {
			panic(fmt.Sprintf("Filter even: expected even number, got %d", n))
		}
		if n < 0 || n > 50 {
			panic(fmt.Sprintf("Filter even: expected [0,50], got %d", n))
		}
	}, WithTestCases(50))
}

// TestFilteredGeneratorGenerateAllFailsCallsAssume verifies that when the
// predicate never passes (all maxFilterAttempts = 3 attempts fail), Assume(false)
// is called to reject the test case. Uses a fake server to control exact responses.
func TestFilteredGeneratorGenerateAllFailsCallsAssume(t *testing.T) {
	schema := map[string]any{"type": "integer"}

	// Track whether Assume(false) was observed (marked by INVALID status in mark_complete).
	var gotInvalidStatus bool

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

		// maxFilterAttempts = 3: handle three start_span + generate + stop_span(discard=true) cycles.
		for i := 0; i < maxFilterAttempts; i++ {
			// start_span(LabelFilter)
			ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
			caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

			// generate → always return odd number (fails even predicate)
			genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
			caseCh.SendReplyValue(genID, int64(1)) //nolint:errcheck

			// stop_span(discard=true)
			spID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
			caseCh.SendReplyValue(spID, nil) //nolint:errcheck
		}

		// After all attempts fail, Assume(false) is called → mark_complete with status=INVALID
		mcID, mcPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decMC, _ := DecodeCBOR(mcPayload)
		mMC, _ := extractDict(decMC)
		status, _ := extractString(mMC[any("status")])
		gotInvalidStatus = (status == "INVALID")
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("filter_all_fail", func() {
		inner := newBasicGenerator(schema, toInt64, true)
		fg := Filter(inner, func(v int64) bool {
			return false // always reject
		})
		Draw(fg) // should call Assume(false) after 3 attempts
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if !gotInvalidStatus {
		t.Error("expected mark_complete with status=INVALID (Assume(false) not called)")
	}
}

// TestFilteredGeneratorGenerateChainedFilters verifies that chaining two Filter
// calls composes the predicates: both must be satisfied.
func TestFilteredGeneratorGenerateChainedFilters(t *testing.T) {
	hegelBinPath(t)
	// First filter: even numbers; second filter: divisible by 4.
	// Combined: only multiples of 4.
	gen := Filter(Filter(Integers(0, 100), func(n int64) bool { return n%2 == 0 }), func(n int64) bool { return n%4 == 0 })
	RunHegelTest(t.Name(), func() {
		n := Draw(gen)
		if n%4 != 0 {
			panic(fmt.Sprintf("chained filter: expected multiple of 4, got %d", n))
		}
	}, WithTestCases(30))
}

// TestFilteredGeneratorGenerateThenMap verifies that Filter followed by Map
// correctly applies the predicate first and then the transform.
func TestFilteredGeneratorGenerateThenMap(t *testing.T) {
	hegelBinPath(t)
	// Filter odd numbers from [1,20], then multiply by 10.
	gen := Map(Filter(Integers(1, 20), func(n int64) bool { return n%2 != 0 }), func(n int64) int64 { return n * 10 })
	if gen.isBasic() {
		t.Fatal("Map on filtered (non-basic) should be non-basic")
	}
	RunHegelTest(t.Name(), func() {
		n := Draw(gen)
		// After filtering odd [1,20] and multiplying by 10: values like 10,30,50,...190
		quotient := n / 10
		if quotient*10 != n {
			panic(fmt.Sprintf("filter+map: expected multiple of 10, got %d", n))
		}
		if quotient%2 == 0 {
			panic(fmt.Sprintf("filter+map: expected odd*10, got %d (quotient=%d is even)", n, quotient))
		}
	}, WithTestCases(30))
}

// =============================================================================
// Unit test for filtered generator Generate using fake server
// =============================================================================

// TestFilteredGeneratorGenerateUnitPredicatePasses exercises filtered generator Generate
// in the case where the predicate passes on the first try, using a fake server.
// This covers the predicate-passes branch: StartSpan → generate → predicate=true → StopSpan(false) → return.
func TestFilteredGeneratorGenerateUnitPredicatePasses(t *testing.T) {
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

		// filtered generator Generate: start_span(LabelFilter)
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// generate
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(42)) //nolint:errcheck

		// predicate passes → StopSpan(false)
		spID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotVal int64
	err := cli.runTest("filter_predicate_passes", func() {
		inner := newBasicGenerator(schema, toInt64, true)
		fg := Filter(inner, func(v int64) bool { return true })
		v := Draw(fg)
		gotVal = v
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 42 {
		t.Errorf("expected 42, got %d", gotVal)
	}
}

// TestFilteredGeneratorGenerateUnitPredicateFailsThenPasses exercises the
// case where the predicate fails on the first attempt but passes on the second.
// Protocol: start_span, generate(fail), stop_span(discard=true),
//
//	start_span, generate(pass), stop_span(discard=false).
func TestFilteredGeneratorGenerateUnitPredicateFailsThenPasses(t *testing.T) {
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

		// --- First attempt: predicate fails ---
		// start_span(LabelFilter)
		ss1ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ss1ID, nil) //nolint:errcheck
		// generate → value 1 (odd — fails even predicate)
		gen1ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(gen1ID, int64(1)) //nolint:errcheck
		// stop_span(discard=true)
		sp1ID, sp1Payload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		dec1, _ := DecodeCBOR(sp1Payload)
		m1, _ := extractDict(dec1)
		d1, _ := m1[any("discard")].(bool)
		if !d1 {
			t.Errorf("first stop_span: expected discard=true, got false")
		}
		caseCh.SendReplyValue(sp1ID, nil) //nolint:errcheck

		// --- Second attempt: predicate passes ---
		// start_span(LabelFilter)
		ss2ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ss2ID, nil) //nolint:errcheck
		// generate → value 4 (even — passes)
		gen2ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(gen2ID, int64(4)) //nolint:errcheck
		// stop_span(discard=false)
		sp2ID, sp2Payload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		dec2, _ := DecodeCBOR(sp2Payload)
		m2, _ := extractDict(dec2)
		d2, _ := m2[any("discard")].(bool)
		if d2 {
			t.Errorf("second stop_span: expected discard=false, got true")
		}
		caseCh.SendReplyValue(sp2ID, nil) //nolint:errcheck

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotVal int64
	err := cli.runTest("filter_fail_then_pass", func() {
		inner := newBasicGenerator(schema, toInt64, true)
		fg := Filter(inner, func(v int64) bool {
			return v%2 == 0 // only even
		})
		v := Draw(fg)
		gotVal = v
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 4 {
		t.Errorf("expected 4, got %d", gotVal)
	}
}
