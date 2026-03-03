package hegel

// filter_test.go tests the Filter method and filteredGenerator type.

import (
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// Filter method unit tests — verify return types
// =============================================================================

// TestBasicGeneratorFilterReturnsfilteredGenerator verifies that calling Filter
// on a basicGenerator returns a *filteredGenerator.
func TestBasicGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	g := Integers(0, 100)
	filtered := g.Filter(func(v any) bool { return true })
	if _, ok := filtered.(*filteredGenerator); !ok {
		t.Fatalf("basicGenerator.Filter should return *filteredGenerator, got %T", filtered)
	}
	// filteredGenerator is not a basic generator.
	if filtered.AsBasic() != nil {
		t.Error("filteredGenerator.AsBasic() should return nil")
	}
}

// TestMappedGeneratorFilterReturnsfilteredGenerator verifies that calling Filter
// on a mappedGenerator returns a *filteredGenerator.
func TestMappedGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	inner := Integers(0, 100)
	mapped := inner.Map(func(v any) any { return v })
	filtered := mapped.Filter(func(v any) bool { return true })
	if _, ok := filtered.(*filteredGenerator); !ok {
		t.Fatalf("mappedGenerator.Filter should return *filteredGenerator, got %T", filtered)
	}
	if filtered.AsBasic() != nil {
		t.Error("filteredGenerator.AsBasic() should return nil")
	}
}

// TestFilteredGeneratorAsBasicReturnsNil verifies that filteredGenerator.AsBasic returns nil.
func TestFilteredGeneratorAsBasicReturnsNil(t *testing.T) {
	g := Integers(0, 100)
	fg := g.Filter(func(v any) bool { return true })
	if fg.AsBasic() != nil {
		t.Error("filteredGenerator.AsBasic() should return nil")
	}
}

// TestFilteredGeneratorFilterChainsfilteredGenerators verifies that calling Filter
// on a filteredGenerator returns another *filteredGenerator (chained filtering).
func TestFilteredGeneratorFilterChainsfilteredGenerators(t *testing.T) {
	g := Integers(0, 100)
	fg := g.Filter(func(v any) bool { return true })
	fg2 := fg.Filter(func(v any) bool { return true })
	if _, ok := fg2.(*filteredGenerator); !ok {
		t.Fatalf("filteredGenerator.Filter should return *filteredGenerator, got %T", fg2)
	}
	if fg2.AsBasic() != nil {
		t.Error("chained filteredGenerator.AsBasic() should return nil")
	}
}

// TestFilteredGeneratorMapReturnsmappedGenerator verifies that calling Map
// on a filteredGenerator returns a *mappedGenerator.
func TestFilteredGeneratorMapReturnsmappedGenerator(t *testing.T) {
	g := Integers(0, 100)
	fg := g.Filter(func(v any) bool { return true })
	mapped := fg.Map(func(v any) any { return v })
	if _, ok := mapped.(*mappedGenerator); !ok {
		t.Fatalf("filteredGenerator.Map should return *mappedGenerator, got %T", mapped)
	}
	if mapped.AsBasic() != nil {
		t.Error("mappedGenerator from filteredGenerator.Map should have AsBasic()=nil")
	}
}

// =============================================================================
// Filter method on composite generators — verify return types
// =============================================================================

// TestCompositeListGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a compositeListGenerator returns a *filteredGenerator.
func TestCompositeListGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	// compositeListGenerator is produced when elements are non-basic.
	nonBasic := &mappedGenerator{inner: Integers(0, 10), fn: func(v any) any { return v }}
	listGen := Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 5})
	filtered := listGen.Filter(func(v any) bool { return true })
	if _, ok := filtered.(*filteredGenerator); !ok {
		t.Fatalf("compositeListGenerator.Filter should return *filteredGenerator, got %T", filtered)
	}
}

// TestCompositeDictGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a compositeDictGenerator returns a *filteredGenerator.
func TestCompositeDictGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	// compositeDictGenerator is produced when key or value is non-basic.
	nonBasic := &mappedGenerator{inner: Integers(0, 10), fn: func(v any) any { return v }}
	dictGen := Dicts(nonBasic, Integers(0, 100), DictOptions{MinSize: 0})
	filtered := dictGen.Filter(func(v any) bool { return true })
	if _, ok := filtered.(*filteredGenerator); !ok {
		t.Fatalf("compositeDictGenerator.Filter should return *filteredGenerator, got %T", filtered)
	}
}

// TestCompositeOneOfGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a compositeOneOfGenerator returns a *filteredGenerator.
func TestCompositeOneOfGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	// compositeOneOfGenerator is produced when any branch is non-basic.
	nonBasic := &mappedGenerator{inner: Integers(0, 10), fn: func(v any) any { return v }}
	oneOf := OneOf(nonBasic, Integers(0, 5))
	filtered := oneOf.Filter(func(v any) bool { return true })
	if _, ok := filtered.(*filteredGenerator); !ok {
		t.Fatalf("compositeOneOfGenerator.Filter should return *filteredGenerator, got %T", filtered)
	}
}

// TestCompositeTupleGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a compositeTupleGenerator returns a *filteredGenerator.
func TestCompositeTupleGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	// compositeTupleGenerator is produced when any element is non-basic.
	nonBasic := &mappedGenerator{inner: Integers(0, 10), fn: func(v any) any { return v }}
	tupleGen := Tuples2(nonBasic, Booleans(0.5))
	filtered := tupleGen.Filter(func(v any) bool { return true })
	if _, ok := filtered.(*filteredGenerator); !ok {
		t.Fatalf("compositeTupleGenerator.Filter should return *filteredGenerator, got %T", filtered)
	}
}

// TestFlatMappedGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a flatMappedGenerator returns a *filteredGenerator.
func TestFlatMappedGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	flatGen := flatMap(Integers(1, 5), func(v any) Generator {
		n, _ := ExtractInt(v)
		return Integers(0, n)
	})
	filtered := flatGen.Filter(func(v any) bool { return true })
	if _, ok := filtered.(*filteredGenerator); !ok {
		t.Fatalf("flatMappedGenerator.Filter should return *filteredGenerator, got %T", filtered)
	}
}

// =============================================================================
// filteredGenerator.Generate tests using real hegel binary
// =============================================================================

// TestFilteredGeneratorGeneratePredicatePassesFirstTry verifies that when the
// predicate passes on the first attempt, the value is returned immediately.
func TestFilteredGeneratorGeneratePredicatePassesFirstTry(t *testing.T) {
	hegelBinPath(t)
	// Filter that always passes: every value is accepted on first try.
	gen := Integers(0, 100).Filter(func(v any) bool { return true })
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		n, err := ExtractInt(v)
		if err != nil {
			panic(fmt.Sprintf("Filter: expected int, got %T: %v", v, v))
		}
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
	gen := Integers(0, 50).Filter(func(v any) bool {
		n, _ := ExtractInt(v)
		return n%2 == 0
	})
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		n, err := ExtractInt(v)
		if err != nil {
			panic(fmt.Sprintf("Filter even: expected int, got %T: %v", v, v))
		}
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

		// maxFilterAttempts = 3: handle three start_span + generate + stop_span(discard=true) cycles.
		for i := 0; i < maxFilterAttempts; i++ {
			// start_span(labelFilter)
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
		decMC, _ := decodeCBOR(mcPayload)
		mMC, _ := ExtractDict(decMC)
		status, _ := ExtractString(mMC[any("status")])
		gotInvalidStatus = (status == "INVALID")
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("filter_all_fail", func() {
		inner := &basicGenerator{schema: schema}
		fg := &filteredGenerator{
			source: inner,
			predicate: func(v any) bool {
				return false // always reject
			},
		}
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
	gen := Integers(0, 100).
		Filter(func(v any) bool {
			n, _ := ExtractInt(v)
			return n%2 == 0
		}).
		Filter(func(v any) bool {
			n, _ := ExtractInt(v)
			return n%4 == 0
		})
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		n, err := ExtractInt(v)
		if err != nil {
			panic(fmt.Sprintf("chained filter: expected int, got %T: %v", v, v))
		}
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
	gen := Integers(1, 20).
		Filter(func(v any) bool {
			n, _ := ExtractInt(v)
			return n%2 != 0
		}).
		Map(func(v any) any {
			n, _ := ExtractInt(v)
			return n * 10
		})
	if _, ok := gen.(*mappedGenerator); !ok {
		t.Fatalf("Filter.Map should return *mappedGenerator, got %T", gen)
	}
	RunHegelTest(t.Name(), func() {
		v := Draw(gen)
		n, _ := ExtractInt(v)
		// After filtering odd [1,20] and multiplying by 10: values like 10,30,50,...190
		if n%20 != 10 && n%20 != 30 && n%2 != 0 {
			// result must be odd*10, so divisible by 10 but result/10 must be odd
		}
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
// Unit test for filteredGenerator.Generate using fake server
// =============================================================================

// TestFilteredGeneratorGenerateUnitPredicatePasses exercises filteredGenerator.Generate
// in the case where the predicate passes on the first try, using a fake server.
// This covers the predicate-passes branch: startSpan → generate → predicate=true → stopSpan(false) → return.
func TestFilteredGeneratorGenerateUnitPredicatePasses(t *testing.T) {
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

		// filteredGenerator.Generate: start_span(labelFilter)
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// generate
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(42)) //nolint:errcheck

		// predicate passes → stopSpan(false)
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
		inner := &basicGenerator{schema: schema}
		fg := &filteredGenerator{
			source:    inner,
			predicate: func(v any) bool { return true },
		}
		v := Draw(fg)
		gotVal, _ = ExtractInt(v)
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

		// --- First attempt: predicate fails ---
		// start_span(labelFilter)
		ss1ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ss1ID, nil) //nolint:errcheck
		// generate → value 1 (odd — fails even predicate)
		gen1ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(gen1ID, int64(1)) //nolint:errcheck
		// stop_span(discard=true)
		sp1ID, sp1Payload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		dec1, _ := decodeCBOR(sp1Payload)
		m1, _ := ExtractDict(dec1)
		d1, _ := m1[any("discard")].(bool)
		if !d1 {
			t.Errorf("first stop_span: expected discard=true, got false")
		}
		caseCh.SendReplyValue(sp1ID, nil) //nolint:errcheck

		// --- Second attempt: predicate passes ---
		// start_span(labelFilter)
		ss2ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ss2ID, nil) //nolint:errcheck
		// generate → value 4 (even — passes)
		gen2ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(gen2ID, int64(4)) //nolint:errcheck
		// stop_span(discard=false)
		sp2ID, sp2Payload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		dec2, _ := decodeCBOR(sp2Payload)
		m2, _ := ExtractDict(dec2)
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
		inner := &basicGenerator{schema: schema}
		fg := &filteredGenerator{
			source: inner,
			predicate: func(v any) bool {
				n, _ := ExtractInt(v)
				return n%2 == 0 // only even
			},
		}
		v := Draw(fg)
		gotVal, _ = ExtractInt(v)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 4 {
		t.Errorf("expected 4, got %d", gotVal)
	}
}
