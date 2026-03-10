package hegel

// filter_test.go tests the Filter function and filteredGenerator type.

import (
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// Filter function unit tests — verify return types
// =============================================================================

// TestBasicGeneratorFilterReturnsfilteredGenerator verifies that calling Filter
// on a basicGenerator returns a *filteredGenerator.
func TestBasicGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	g := Integers[int](0, 100)
	filtered := Filter(g, func(v int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[int]); !ok {
		t.Fatalf("Filter(basicGenerator) should return *filteredGenerator[int], got %T", filtered)
	}
}

// TestMappedGeneratorFilterReturnsfilteredGenerator verifies that calling Filter
// on a mappedGenerator returns a *filteredGenerator.
func TestMappedGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	inner := Integers[int](0, 100)
	mapped := Map(inner, func(v int) int { return v })
	filtered := Filter(mapped, func(v int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[int]); !ok {
		t.Fatalf("Filter(mappedGenerator) should return *filteredGenerator[int], got %T", filtered)
	}
}

// TestFilteredGeneratorFilterChainsfilteredGenerators verifies that calling Filter
// on a filteredGenerator returns another *filteredGenerator (chained filtering).
func TestFilteredGeneratorFilterChainsfilteredGenerators(t *testing.T) {
	g := Integers[int](0, 100)
	fg := Filter(g, func(v int) bool { return true })
	fg2 := Filter(fg, func(v int) bool { return true })
	if _, ok := fg2.(*filteredGenerator[int]); !ok {
		t.Fatalf("Filter(filteredGenerator) should return *filteredGenerator[int], got %T", fg2)
	}
}

// TestFilteredGeneratorMapReturnsmappedGenerator verifies that calling Map
// on a filteredGenerator returns a *mappedGenerator.
func TestFilteredGeneratorMapReturnsmappedGenerator(t *testing.T) {
	g := Integers[int](0, 100)
	fg := Filter(g, func(v int) bool { return true })
	mapped := Map(fg, func(v int) int { return v })
	if _, ok := mapped.(*mappedGenerator[int, int]); !ok {
		t.Fatalf("Map(filteredGenerator) should return *mappedGenerator, got %T", mapped)
	}
}

// =============================================================================
// Filter on composite generators — verify return types
// =============================================================================

// TestCompositeListGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a compositeListGenerator returns a *filteredGenerator.
func TestCompositeListGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	// compositeListGenerator is produced when elements are non-basic.
	// Filter produces a filteredGenerator (non-basic), forcing Lists into composite path.
	nonBasic := Filter(Integers[int](0, 10), func(v int) bool { return true })
	listGen := Lists(nonBasic, ListMaxSize(5))
	filtered := Filter(listGen, func(v []int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[[]int]); !ok {
		t.Fatalf("Filter(compositeListGenerator) should return *filteredGenerator, got %T", filtered)
	}
}

// TestCompositeDictGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a compositeDictGenerator returns a *filteredGenerator.
func TestCompositeDictGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	// compositeDictGenerator is produced when key or value is non-basic.
	// Filter produces a filteredGenerator (non-basic), forcing Dicts into composite path.
	nonBasic := Filter(Integers[int](0, 10), func(v int) bool { return true })
	dictGen := Dicts(nonBasic, Integers[int](0, 100))
	filtered := Filter(dictGen, func(v map[int]int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[map[int]int]); !ok {
		t.Fatalf("Filter(compositeDictGenerator) should return *filteredGenerator, got %T", filtered)
	}
}

// TestCompositeOneOfGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a compositeOneOfGenerator returns a *filteredGenerator.
func TestCompositeOneOfGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	// compositeOneOfGenerator is produced when any branch is non-basic.
	// Filter produces a filteredGenerator (non-basic), forcing OneOf into composite path.
	nonBasic := Filter(Integers[int](0, 10), func(v int) bool { return true })
	oneOf := OneOf[int](nonBasic, Integers[int](0, 5))
	filtered := Filter(oneOf, func(v int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[int]); !ok {
		t.Fatalf("Filter(compositeOneOfGenerator) should return *filteredGenerator, got %T", filtered)
	}
}

// TestFlatMappedGeneratorFilterReturnsfilteredGenerator verifies that calling
// Filter on a flatMappedGenerator returns a *filteredGenerator.
func TestFlatMappedGeneratorFilterReturnsfilteredGenerator(t *testing.T) {
	flatGen := FlatMap(Integers[int](1, 5), func(v int) Generator[int] {
		return Integers[int](0, v)
	})
	filtered := Filter(flatGen, func(v int) bool { return true })
	if _, ok := filtered.(*filteredGenerator[int]); !ok {
		t.Fatalf("Filter(flatMappedGenerator) should return *filteredGenerator, got %T", filtered)
	}
}

// =============================================================================
// filteredGenerator.draw tests using real hegel binary
// =============================================================================

// TestFilteredGeneratorGeneratePredicatePassesFirstTry verifies that when the
// predicate passes on the first attempt, the value is returned immediately.
func TestFilteredGeneratorGeneratePredicatePassesFirstTry(t *testing.T) {
	hegelBinPath(t)
	// Filter that always passes: every value is accepted on first try.
	gen := Filter(Integers[int](0, 100), func(v int) bool { return true })
	if _err := runHegel(func(s *TestCase) {
		n := gen.draw(s)
		if n < 0 || n > 100 {
			panic(fmt.Sprintf("Filter: expected [0,100], got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// TestFilteredGeneratorGenerateWithRealPredicate verifies that Filter correctly
// filters values: only even numbers should pass.
func TestFilteredGeneratorGenerateWithRealPredicate(t *testing.T) {
	hegelBinPath(t)
	// Filter integers [0,50] keeping only even ones.
	gen := Filter(Integers[int](0, 50), func(v int) bool {
		return v%2 == 0
	})
	if _err := runHegel(func(s *TestCase) {
		n := gen.draw(s)
		if n%2 != 0 {
			panic(fmt.Sprintf("Filter even: expected even number, got %d", n))
		}
		if n < 0 || n > 50 {
			panic(fmt.Sprintf("Filter even: expected [0,50], got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
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
		mMC, _ := extractCBORDict(decMC)
		status, _ := extractCBORString(mMC[any("status")])
		gotInvalidStatus = (status == "INVALID")
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest(func(s *TestCase) {
		inner := &basicGenerator[int64]{
			schema:    schema,
			transform: func(v any) int64 { return extractInt(v) },
		}
		fg := &filteredGenerator[int64]{
			source: inner,
			predicate: func(v int64) bool {
				return false // always reject
			},
		}
		fg.draw(s) // should call Assume(false) after 3 attempts
	}, runOptions{testCases: 1}, stderrNoteFn)
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
	gen := Filter(
		Filter(Integers[int](0, 100), func(v int) bool { return v%2 == 0 }),
		func(v int) bool { return v%4 == 0 },
	)
	if _err := runHegel(func(s *TestCase) {
		n := gen.draw(s)
		if n%4 != 0 {
			panic(fmt.Sprintf("chained filter: expected multiple of 4, got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// TestFilteredGeneratorGenerateThenMap verifies that Filter followed by Map
// correctly applies the predicate first and then the transform.
func TestFilteredGeneratorGenerateThenMap(t *testing.T) {
	hegelBinPath(t)
	// Filter odd numbers from [1,20], then multiply by 10.
	gen := Map(
		Filter(Integers[int](1, 20), func(v int) bool { return v%2 != 0 }),
		func(v int) int { return v * 10 },
	)
	if _, ok := gen.(*mappedGenerator[int, int]); !ok {
		t.Fatalf("Map(Filter(...)) should return *mappedGenerator, got %T", gen)
	}
	if _err := runHegel(func(s *TestCase) {
		n := gen.draw(s)
		// result must be odd*10, so divisible by 10 but result/10 must be odd
		quotient := n / 10
		if quotient*10 != n {
			panic(fmt.Sprintf("filter+map: expected multiple of 10, got %d", n))
		}
		if quotient%2 == 0 {
			panic(fmt.Sprintf("filter+map: expected odd*10, got %d (quotient=%d is even)", n, quotient))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// Unit test for filteredGenerator.draw using fake server
// =============================================================================

// TestFilteredGeneratorGenerateUnitPredicatePasses exercises filteredGenerator.draw
// in the case where the predicate passes on the first try, using a fake server.
// This covers the predicate-passes branch: startSpan → generate → predicate=true → stopSpan(false) → return.
func TestFilteredGeneratorGenerateUnitPredicatePasses(t *testing.T) {
	schema := map[string]any{"type": "integer"}
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

		// filteredGenerator.draw: start_span(labelFilter)
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
	err := cli.runTest(func(s *TestCase) {
		inner := &basicGenerator[int64]{
			schema:    schema,
			transform: func(v any) int64 { return extractInt(v) },
		}
		fg := &filteredGenerator[int64]{
			source:    inner,
			predicate: func(v int64) bool { return true },
		}
		gotVal = fg.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
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
		m1, _ := extractCBORDict(dec1)
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
		m2, _ := extractCBORDict(dec2)
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
	err := cli.runTest(func(s *TestCase) {
		inner := &basicGenerator[int64]{
			schema:    schema,
			transform: func(v any) int64 { return extractInt(v) },
		}
		fg := &filteredGenerator[int64]{
			source: inner,
			predicate: func(v int64) bool {
				return v%2 == 0 // only even
			},
		}
		gotVal = fg.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 4 {
		t.Errorf("expected 4, got %d", gotVal)
	}
}
