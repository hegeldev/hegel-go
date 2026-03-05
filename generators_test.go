package hegel

import (
	"fmt"
	"math/big"
	"testing"
	"time"
)

// =============================================================================
// Generator interface and basicGenerator tests
// =============================================================================

// --- basicGenerator: generate with no transform ---

func TestBasicGeneratorGenerateNoTransform(t *testing.T) {
	// Set up fake server that responds to a generate command with "hello".
	// We use string type because CBOR strings decode directly to Go strings,
	// so v.(T) works without a transform. (Integers decode as uint64, not int64.)
	schema := map[string]any{"type": "string"}
	clientConn := fakeServerConn(t, func(serverConn *connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(payload)
		m, _ := extractCBORDict(decoded)
		chID, _ := extractCBORInt(m[any("channel_id")])
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

		// Respond to generate with "hello".
		genID, genPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		_, _ = genPayload, decodeCBOR         // consumed
		caseCh.SendReplyValue(genID, "hello") //nolint:errcheck

		// Wait for mark_complete.
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		// Send test_done (passed, no interesting).
		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotVal string
	err := cli.runTest("basic_gen_no_transform", func(s *TestCase) {
		// No transform: the raw CBOR string "hello" is returned as-is via v.(T).
		g := &basicGenerator[string]{schema: schema}
		gotVal = g.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != "hello" {
		t.Errorf("expected %q, got %q", "hello", gotVal)
	}
}

// --- basicGenerator: generate with transform ---

func TestBasicGeneratorGenerateWithTransform(t *testing.T) {
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

		// Respond to generate with 7.
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(7)) //nolint:errcheck

		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotVal int64
	err := cli.runTest("basic_gen_with_transform", func(s *TestCase) {
		// transform: multiply by 2
		g := &basicGenerator[int64]{
			schema:    schema,
			transform: func(v any) int64 { return extractInt(v) * 2 },
		}
		gotVal = g.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 14 {
		t.Errorf("expected 14, got %d", gotVal)
	}
}

// --- Map free function on basicGenerator: no existing transform ---

func TestBasicGeneratorMapNoTransform(t *testing.T) {
	schema := map[string]any{"type": "boolean"}
	g := &basicGenerator[bool]{schema: schema}
	mapped := Map[bool, string](g, func(v bool) string {
		if v {
			return "yes"
		}
		return "no"
	})
	// Map on basicGenerator returns another basicGenerator with same schema.
	bg, ok := mapped.(*basicGenerator[string])
	if !ok {
		t.Fatalf("Map on basicGenerator should return *basicGenerator[string], got %T", mapped)
	}
	if bg.schema["type"] != "boolean" {
		t.Errorf("schema not preserved by Map")
	}
	if bg.transform == nil {
		t.Error("transform should not be nil after Map")
	}
}

// --- Map free function on basicGenerator: compose transforms ---

func TestBasicGeneratorMapComposesTransforms(t *testing.T) {
	schema := map[string]any{"type": "integer"}
	g := &basicGenerator[int64]{
		schema:    schema,
		transform: func(v any) int64 { return extractInt(v) + 1 },
	}
	// Map again: result should be (n+1)*2
	mapped := Map[int64, int64](g, func(v int64) int64 {
		return v * 2
	})
	bg, ok := mapped.(*basicGenerator[int64])
	if !ok {
		t.Fatalf("double Map should return *basicGenerator[int64]")
	}
	// Simulate applying: start with int64(5) -> +1 -> 6 -> *2 -> 12
	result := bg.transform(int64(5))
	if result != 12 {
		t.Errorf("composed transform: expected 12, got %d", result)
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
	err := cli.runTest("mapped_gen", func(s *TestCase) {
		inner := &basicGenerator[int64]{schema: schema, transform: func(v any) int64 { return extractInt(v) }}
		mg := &mappedGenerator[int64, int64]{
			inner: inner,
			fn:    func(v int64) int64 { return v * 10 },
		}
		gotVal = mg.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 30 {
		t.Errorf("expected 30, got %d", gotVal)
	}
}

// --- Map on basicGenerator inner returns basicGenerator ---

func TestMappedGeneratorMapOnBasicInner(t *testing.T) {
	inner := &basicGenerator[int64]{schema: map[string]any{"type": "integer"}, transform: func(v any) int64 { return extractInt(v) }}
	// Map on basicGenerator returns basicGenerator.
	result := Map[int64, int64](inner, func(v int64) int64 { return v })
	if _, ok := result.(*basicGenerator[int64]); !ok {
		t.Errorf("Map on basicGenerator should return *basicGenerator[int64]")
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

		fn(caseCh)

		// Wait for mark_complete.
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})
}

// --- startSpan and stopSpan ---

func TestStartStopSpan(t *testing.T) {
	var gotStartLabel int64
	var gotStopDiscard bool
	clientConn := fakeTestEnv(t, func(caseCh *channel) {
		// start_span
		ssID, ssPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(ssPayload)
		m, _ := extractCBORDict(decoded)
		gotStartLabel, _ = extractCBORInt(m[any("label")])
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck
		// stop_span
		spID, spPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded2, _ := decodeCBOR(spPayload)
		m2, _ := extractCBORDict(decoded2)
		b, _ := m2[any("discard")].(bool)
		gotStopDiscard = b
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("spans", func(s *TestCase) {
		startSpan(s, labelMapped)
		stopSpan(s, false)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotStartLabel != int64(labelMapped) {
		t.Errorf("start_span label: expected %d, got %d", labelMapped, gotStartLabel)
	}
	if gotStopDiscard {
		t.Error("stop_span discard should be false")
	}
}

// --- stopSpan with discard=true ---

func TestStopSpanDiscard(t *testing.T) {
	var gotDiscard bool
	clientConn := fakeTestEnv(t, func(caseCh *channel) {
		// start_span
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck
		// stop_span
		spID, spPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(spPayload)
		m, _ := extractCBORDict(decoded)
		b, _ := m[any("discard")].(bool)
		gotDiscard = b
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("stop_span_discard", func(s *TestCase) {
		startSpan(s, labelList)
		stopSpan(s, true)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if !gotDiscard {
		t.Error("stop_span discard should be true")
	}
}

// --- startSpan no-op when aborted ---

func TestStartSpanNoOpWhenAborted(t *testing.T) {
	// When aborted=true, startSpan and stopSpan must not send any messages.
	clientConn := fakeTestEnv(t, func(caseCh *channel) {
		// No messages expected (no start/stop span).
		// The test fn will set aborted then call startSpan/stopSpan.
	})

	cli := newClient(clientConn)
	err := cli.runTest("span_noop_aborted", func(s *TestCase) {
		// Directly set the aborted flag.
		s.aborted = true
		startSpan(s, labelList) // should be no-op
		stopSpan(s, false)      // should be no-op
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
}

// --- Group helper ---

func TestGroup(t *testing.T) {
	var cmds []string
	// Use fakeServerConn directly to control all message handling.
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

		// Receive exactly: start_span, stop_span, mark_complete.
		for i := 0; i < 3; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := decodeCBOR(pl)
			mp, _ := extractCBORDict(dec)
			cmd, _ := extractCBORString(mp[any("command")])
			cmds = append(cmds, cmd)
			caseCh.SendReplyValue(mid, nil) //nolint:errcheck
		}

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("group_test", func(s *TestCase) {
		group(s, labelMapped, func() {
			// nothing inside, just test the wrapping
		})
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if len(cmds) < 3 {
		t.Fatalf("expected at least start_span, stop_span, mark_complete; got %v", cmds)
	}
	if cmds[0] != "start_span" {
		t.Errorf("first cmd: expected start_span, got %s", cmds[0])
	}
	if cmds[1] != "stop_span" {
		t.Errorf("second cmd: expected stop_span, got %s", cmds[1])
	}
}

// --- discardableGroup: no panic ---

func TestDiscardableGroupNoPanic(t *testing.T) {
	var cmds []string
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

		// Receive exactly: start_span, stop_span, mark_complete.
		for i := 0; i < 3; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := decodeCBOR(pl)
			mp, _ := extractCBORDict(dec)
			cmd, _ := extractCBORString(mp[any("command")])
			cmds = append(cmds, cmd)
			caseCh.SendReplyValue(mid, nil) //nolint:errcheck
		}

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("discardable_group_ok", func(s *TestCase) {
		discardableGroup(s, labelFilter, func() {
			// runs normally
		})
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	// should have start_span, stop_span(discard=false), mark_complete
	if len(cmds) < 2 || cmds[0] != "start_span" {
		t.Errorf("expected start_span first; got %v", cmds)
	}
}

// --- discardableGroup: panic propagates with discard=true ---

func TestDiscardableGroupPanic(t *testing.T) {
	var stopDiscardVal bool
	clientConn := fakeTestEnv(t, func(caseCh *channel) {
		// start_span
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck
		// stop_span (discard=true because panic propagated)
		spID, spPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(spPayload)
		m, _ := extractCBORDict(decoded)
		b, _ := m[any("discard")].(bool)
		stopDiscardVal = b
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("discardable_group_panic", func(s *TestCase) {
		discardableGroup(s, labelFilter, func() {
			panic("inner panic")
		})
	}, runOptions{testCases: 1}, stderrNoteFn)
	// The panic inside discardableGroup should propagate out as INTERESTING.
	_ = err // may be nil (non-final case doesn't return error) or error
	if !stopDiscardVal {
		t.Error("stop_span should have discard=true when inner fn panics")
	}
}

// =============================================================================
// collection protocol tests
// =============================================================================

// --- newCollection: basic ---

func TestNewCollection(t *testing.T) {
	var gotCmd string
	var gotMin, gotMax int64
	clientConn := fakeTestEnv(t, func(caseCh *channel) {
		// new_collection
		ncID, ncPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(ncPayload)
		m, _ := extractCBORDict(decoded)
		gotCmd, _ = extractCBORString(m[any("command")])
		gotMin, _ = extractCBORInt(m[any("min_size")])
		gotMax, _ = extractCBORInt(m[any("max_size")])
		caseCh.SendReplyValue(ncID, "coll_1") //nolint:errcheck

		// collection_more -> false (done immediately)
		moreID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(moreID, false) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("new_collection", func(s *TestCase) {
		coll := newCollection(s, 2, 10)
		more := coll.More(s)
		_ = more
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotCmd != "new_collection" {
		t.Errorf("expected new_collection, got %s", gotCmd)
	}
	if gotMin != 2 {
		t.Errorf("expected min_size=2, got %d", gotMin)
	}
	if gotMax != 10 {
		t.Errorf("expected max_size=10, got %d", gotMax)
	}
}

// --- collection.More: returns true then false ---

func TestCollectionMore(t *testing.T) {
	var moreCount int
	clientConn := fakeTestEnv(t, func(caseCh *channel) {
		// new_collection
		ncID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_x") //nolint:errcheck

		// first more -> true
		m1ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(m1ID, true) //nolint:errcheck
		// second more -> false
		m2ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(m2ID, false) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("coll_more", func(s *TestCase) {
		coll := newCollection(s, 0, 5)
		for coll.More(s) {
			moreCount++
		}
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if moreCount != 1 {
		t.Errorf("expected 1 more loop iteration, got %d", moreCount)
	}
}

// --- collection.More: cached false after first false ---

func TestCollectionMoreCachesFalse(t *testing.T) {
	clientConn := fakeTestEnv(t, func(caseCh *channel) {
		// new_collection
		ncID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_y") //nolint:errcheck

		// Only one more request (the second More() call should be cached).
		moreID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(moreID, false) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("coll_cache", func(s *TestCase) {
		coll := newCollection(s, 0, 1)
		r1 := coll.More(s)
		r2 := coll.More(s) // should be cached false, no network call
		if r1 || r2 {
			panic("expected both to be false")
		}
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
}

// --- collection.Reject ---

func TestCollectionReject(t *testing.T) {
	var gotRejectCmd string
	clientConn := fakeTestEnv(t, func(caseCh *channel) {
		// new_collection
		ncID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_r") //nolint:errcheck

		// more -> true
		moreID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(moreID, true) //nolint:errcheck

		// collection_reject
		rejID, rejPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded, _ := decodeCBOR(rejPayload)
		m, _ := extractCBORDict(decoded)
		gotRejectCmd, _ = extractCBORString(m[any("command")])
		caseCh.SendReplyValue(rejID, nil) //nolint:errcheck

		// more -> false
		m2ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(m2ID, false) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("coll_reject", func(s *TestCase) {
		coll := newCollection(s, 0, 5)
		if coll.More(s) {
			coll.Reject(s)
		}
		for coll.More(s) {
		}
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotRejectCmd != "collection_reject" {
		t.Errorf("expected collection_reject, got %s", gotRejectCmd)
	}
}

// --- collection.Reject no-op after finished ---

func TestCollectionRejectNoOpAfterFinished(t *testing.T) {
	clientConn := fakeTestEnv(t, func(caseCh *channel) {
		// new_collection
		ncID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_nop") //nolint:errcheck

		// more -> false immediately
		moreID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(moreID, false) //nolint:errcheck
		// No collection_reject should follow.
	})

	cli := newClient(clientConn)
	err := cli.runTest("coll_reject_noop", func(s *TestCase) {
		coll := newCollection(s, 0, 1)
		coll.More(s)   // false -> finished
		coll.Reject(s) // no-op
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
}

// --- collection StopTest on new_collection ---

func TestCollectionStopTestOnNewCollection(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_new_collection")
	err := runHegel("coll_stop_new", func(s *TestCase) {
		coll := newCollection(s, 0, 5)
		_ = coll.More(s)
	}, stderrNoteFn, nil)
	// Should not error -- the test was stopped, not failed.
	_ = err
}

// --- collection StopTest on collection_more ---

func TestCollectionStopTestOnCollectionMore(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_collection_more")
	err := runHegel("coll_stop_more", func(s *TestCase) {
		coll := newCollection(s, 0, 5)
		_ = coll.More(s)
	}, stderrNoteFn, nil)
	_ = err
}

// =============================================================================
// Label constants
// =============================================================================

func TestLabelConstants(t *testing.T) {
	cases := []struct {
		name string
		val  spanLabel
		want int
	}{
		{"List", labelList, 1},
		{"ListElement", labelListElement, 2},
		{"Set", labelSet, 3},
		{"SetElement", labelSetElement, 4},
		{"Map", labelMap, 5},
		{"MapEntry", labelMapEntry, 6},
		{"Tuple", labelTuple, 7},
		{"OneOf", labelOneOf, 8},
		{"Optional", labelOptional, 9},
		{"FixedDict", labelFixedDict, 10},
		{"flatMap", labelFlatMap, 11},
		{"Filter", labelFilter, 12},
		{"Mapped", labelMapped, 13},
		{"SampledFrom", labelSampledFrom, 14},
		{"EnumVariant", labelEnumVariant, 15},
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
	if _err := runHegel("integers_happy", func(s *TestCase) {
		v := Draw[int64](s, Integers(0, 100))
		vals = append(vals, v)
		if v < 0 || v > 100 {
			panic(fmt.Sprintf("out of range: %d", v))
		}
	}, stderrNoteFn, []Option{WithTestCases(10)}); _err != nil {
		panic(_err)
	}
	if len(vals) == 0 {
		t.Error("test function was never called")
	}
}

// --- Integers: schema is correct ---

func TestIntegersSchema(t *testing.T) {
	g := Integers(-5, 5)
	bg, ok := g.(*basicGenerator[int64])
	if !ok {
		t.Fatalf("Integers should return *basicGenerator[int64]")
	}
	min := bg.schema["min_value"].(int64)
	max := bg.schema["max_value"].(int64)
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
	bg, ok := g.(*basicGenerator[int64])
	if !ok {
		t.Fatalf("IntegersUnbounded should return *basicGenerator[int64]")
	}
	if _, hasMin := bg.schema["min_value"]; hasMin {
		t.Error("min_value should not be present when no min bound given")
	}
	if _, hasMax := bg.schema["max_value"]; hasMax {
		t.Error("max_value should not be present when no max bound given")
	}
}

// =============================================================================
// Just generator tests
// =============================================================================

// TestJustSchema verifies that Just produces a schema with "const" key.
func TestJustSchema(t *testing.T) {
	g := Just(42)
	bg := g.(*basicGenerator[int])
	if _, hasConst := bg.schema["const"]; !hasConst {
		t.Error("Just schema should have 'const' key")
	}
	// The const value in schema should be nil (null)
	if bg.schema["const"] != nil {
		t.Errorf("Just schema 'const' should be nil, got %v", bg.schema["const"])
	}
}

// TestJustTransformIgnoresInput verifies that Just always returns the constant value.
func TestJustTransformIgnoresInput(t *testing.T) {
	g := Just("hello")
	bg := g.(*basicGenerator[string])
	// transform should ignore the server value and always return "hello"
	result := bg.transform(nil)
	if result != "hello" {
		t.Errorf("Just transform: expected 'hello', got %v", result)
	}
	result = bg.transform(int64(999))
	if result != "hello" {
		t.Errorf("Just transform with non-nil input: expected 'hello', got %v", result)
	}
}

// TestJustE2E verifies that Just always generates the constant value against the real server.
func TestJustE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[int](s, Just(42))
		if v != 42 {
			panic(fmt.Sprintf("Just: expected 42, got %v", v))
		}
	}, stderrNoteFn, []Option{WithTestCases(20)}); _err != nil {
		panic(_err)
	}
}

// TestJustNonPrimitive verifies that Just works with non-primitive values (pointer identity).
func TestJustNonPrimitive(t *testing.T) {
	hegelBinPath(t)
	type myStruct struct{ x int }
	val := &myStruct{x: 99}
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[*myStruct](s, Just(val))
		if v != val {
			panic("Just: pointer identity not preserved")
		}
	}, stderrNoteFn, []Option{WithTestCases(10)}); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// SampledFrom generator tests
// =============================================================================

// TestSampledFromEmptyPanics verifies that SampledFrom panics for empty slice.
func TestSampledFromEmptyPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("SampledFrom([]) should panic")
		}
	}()
	SampledFrom([]string{})
}

// TestSampledFromSchema verifies that SampledFrom produces an integer schema with correct bounds.
func TestSampledFromSchema(t *testing.T) {
	g := SampledFrom([]string{"a", "b", "c"})
	bg := g.(*basicGenerator[string])
	if bg.schema["type"] != "integer" {
		t.Errorf("schema type: expected 'integer', got %v", bg.schema["type"])
	}
	minVal := bg.schema["min_value"].(int64)
	maxVal := bg.schema["max_value"].(int64)
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
	items := []string{"x", "y", "z"}
	g := SampledFrom(items)
	bg := g.(*basicGenerator[string])
	// Index 0 -> "x", 1 -> "y", 2 -> "z"
	for i, want := range items {
		got := bg.transform(uint64(i))
		if got != want {
			t.Errorf("transform(%d): expected %v, got %v", i, want, got)
		}
	}
}

// TestSampledFromSingleElement verifies that a single-element slice always returns that element.
func TestSampledFromSingleElement(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[string](s, SampledFrom([]string{"only"}))
		if v != "only" {
			panic(fmt.Sprintf("SampledFrom single: expected 'only', got %v", v))
		}
	}, stderrNoteFn, []Option{WithTestCases(20)}); _err != nil {
		panic(_err)
	}
}

// TestSampledFromE2E verifies that SampledFrom only returns elements from the list
// and that all elements appear (with enough test cases).
func TestSampledFromE2E(t *testing.T) {
	hegelBinPath(t)
	choices := []string{"apple", "banana", "cherry"}
	seen := map[string]bool{}
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[string](s, SampledFrom(choices))
		found := false
		for _, c := range choices {
			if c == v {
				found = true
				break
			}
		}
		if !found {
			panic(fmt.Sprintf("SampledFrom: value %q not in choices", v))
		}
		seen[v] = true
	}, stderrNoteFn, []Option{WithTestCases(100)}); _err != nil {
		panic(_err)
	}
	// After 100 cases we expect all 3 values to have appeared.
	for _, c := range choices {
		if !seen[c] {
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
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[*myStruct](s, SampledFrom([]*myStruct{obj1, obj2}))
		if v != obj1 && v != obj2 {
			panic("SampledFrom: value is not one of the original pointers")
		}
	}, stderrNoteFn, []Option{WithTestCases(10)}); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// FromRegex generator tests
// =============================================================================

// TestFromRegexSchema verifies that FromRegex produces the correct schema.
func TestFromRegexSchema(t *testing.T) {
	g := FromRegex(`\d+`, true)
	bg := g.(*basicGenerator[string])
	if bg.schema["type"] != "regex" {
		t.Errorf("schema type: expected 'regex', got %v", bg.schema["type"])
	}
	if bg.schema["pattern"] != `\d+` {
		t.Errorf("pattern: expected '\\d+', got %v", bg.schema["pattern"])
	}
	if bg.schema["fullmatch"] != true {
		t.Errorf("fullmatch: expected true, got %v", bg.schema["fullmatch"])
	}
}

// TestFromRegexFullmatchFalse verifies that fullmatch=false is stored correctly.
func TestFromRegexFullmatchFalse(t *testing.T) {
	g := FromRegex(`abc`, false)
	bg := g.(*basicGenerator[string])
	if bg.schema["fullmatch"] != false {
		t.Errorf("fullmatch: expected false, got %v", bg.schema["fullmatch"])
	}
}

// TestFromRegexE2E verifies that FromRegex generates strings that match the pattern.
func TestFromRegexE2E(t *testing.T) {
	hegelBinPath(t)
	// Only digits, 1-5 chars
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[string](s, FromRegex(`[0-9]{1,5}`, true))
		if len(v) == 0 || len(v) > 5 {
			panic(fmt.Sprintf("FromRegex: length out of range: %q", v))
		}
		for _, ch := range v {
			if ch < '0' || ch > '9' {
				panic(fmt.Sprintf("FromRegex: non-digit character %q in %q", ch, v))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// basicGenerator.draw error path (line 78-79)
// =============================================================================

// TestBasicGeneratorGenerateErrorResponse covers the error path in
// basicGenerator.draw when generateFromSchema returns a non-StopTest error.
func TestBasicGeneratorGenerateErrorResponse(t *testing.T) {
	hegelBinPath(t)
	t.Setenv("HEGEL_PROTOCOL_TEST_MODE", "error_response")
	err := runHegel(t.Name(), func(s *TestCase) {
		g := &basicGenerator[int64]{schema: map[string]any{"type": "integer"}, transform: func(v any) int64 { return extractInt(v) }}
		_ = g.draw(s) // should panic with requestError -> caught as INTERESTING
	}, stderrNoteFn, nil)
	// error_response causes the test to appear interesting (failing).
	_ = err
}

// =============================================================================
// Map on a Generator interface (non-basic returns mappedGenerator)
// =============================================================================

func TestGeneratorMapOnNonBasic(t *testing.T) {
	// A custom generator that is not a basicGenerator.
	schema := map[string]any{"type": "integer"}
	inner := &basicGenerator[int64]{schema: schema, transform: func(v any) int64 { return extractInt(v) }}
	// mappedGenerator is not a basicGenerator.
	mg := &mappedGenerator[int64, int64]{inner: inner, fn: func(v int64) int64 { return v }}
	mapped := Map[int64, int64](mg, func(v int64) int64 { return v })
	// Mapping a non-basic generator should produce a mappedGenerator.
	if _, ok := mapped.(*mappedGenerator[int64, int64]); !ok {
		t.Errorf("Map on non-basic Generator should return *mappedGenerator, got %T", mapped)
	}
}

// =============================================================================
// Map generator E2E tests
// =============================================================================

// TestMapBasicGeneratorE2E verifies that mapping Integers(0,100) by doubling
// always produces even values in [0, 200], and the result is still a basicGenerator.
func TestMapBasicGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	gen := Map[int64, int64](Integers(0, 100), func(v int64) int64 {
		return v * 2
	})
	// Map on basic generator must preserve basicGenerator type.
	if _, ok := gen.(*basicGenerator[int64]); !ok {
		t.Fatalf("Map on basicGenerator should return *basicGenerator[int64], got %T", gen)
	}
	if _err := runHegel(t.Name(), func(s *TestCase) {
		n := Draw[int64](s, gen)
		if n%2 != 0 {
			panic(fmt.Sprintf("map(x*2): expected even number, got %d", n))
		}
		if n < 0 || n > 200 {
			panic(fmt.Sprintf("map(x*2): expected [0,200], got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestMapChainedBasicGeneratorE2E verifies that chaining two maps on a basicGenerator
// preserves the basicGenerator type and composes the transforms correctly.
// Integers(0,100).Map(x+1).Map(x*2): result must be even, in [2, 202].
func TestMapChainedBasicGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	gen := Map[int64, int64](
		Map[int64, int64](Integers(0, 100), func(v int64) int64 { return v + 1 }),
		func(v int64) int64 { return v * 2 },
	)
	// Both chained maps should still return a basicGenerator (schema preserved).
	if _, ok := gen.(*basicGenerator[int64]); !ok {
		t.Fatalf("chained Map on basicGenerator should return *basicGenerator[int64], got %T", gen)
	}
	if _err := runHegel(t.Name(), func(s *TestCase) {
		n := Draw[int64](s, gen)
		// (x+1)*2 is always even. x in [0,100] -> result in [2, 202].
		if n%2 != 0 {
			panic(fmt.Sprintf("map(x+1).map(x*2): expected even, got %d", n))
		}
		if n < 2 || n > 202 {
			panic(fmt.Sprintf("map(x+1).map(x*2): expected [2,202], got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestMapNonBasicGeneratorE2E verifies that mapping a mappedGenerator (non-basic)
// wraps it in a MAPPED span and applies the transform correctly.
// The result must be a mappedGenerator (not basicGenerator).
func TestMapNonBasicGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	// Create a non-basic generator by wrapping a basicGenerator in mappedGenerator.
	inner := Integers(1, 5)
	nonBasic := &mappedGenerator[int64, int64]{
		inner: inner,
		fn:    func(v int64) int64 { return v }, // identity
	}
	gen := Map[int64, int64](nonBasic, func(v int64) int64 {
		return v * 3
	})
	if _, ok := gen.(*mappedGenerator[int64, int64]); !ok {
		t.Fatalf("Map on non-basic Generator should return *mappedGenerator, got %T", gen)
	}
	if _err := runHegel(t.Name(), func(s *TestCase) {
		n := Draw[int64](s, gen)
		// inner is Integers(1,5)*1, map(*3): result is in {3, 6, 9, 12, 15}
		if n < 3 || n > 15 || n%3 != 0 {
			panic(fmt.Sprintf("map(*3) on [1,5]: expected multiple of 3 in [3,15], got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestMapSchemaPreservedUnit verifies unit-level schema properties of Map on basicGenerator.
func TestMapSchemaPreservedUnit(t *testing.T) {
	base := Integers(0, 100)
	mapped := Map[int64, int64](base, func(v int64) int64 { return v })
	bg, ok := mapped.(*basicGenerator[int64])
	if !ok {
		t.Fatalf("Map on basicGenerator: expected *basicGenerator[int64], got %T", mapped)
	}
	if bg.schema["type"] != "integer" {
		t.Errorf("schema type: expected 'integer', got %v", bg.schema["type"])
	}
	if bg.transform == nil {
		t.Error("transform should not be nil after Map")
	}
	// Map on basicGenerator must preserve min/max bounds in the schema.
	minV := bg.schema["min_value"].(int64)
	maxV := bg.schema["max_value"].(int64)
	if minV != 0 {
		t.Errorf("min_value: expected 0, got %d", minV)
	}
	if maxV != 100 {
		t.Errorf("max_value: expected 100, got %d", maxV)
	}

	// Double Map on basicGenerator: schema still preserved, transforms compose correctly.
	doubled := Map[int64, int64](
		Map[int64, int64](base, func(v int64) int64 { return v + 10 }),
		func(v int64) int64 { return v * 2 },
	)
	bg2, ok := doubled.(*basicGenerator[int64])
	if !ok {
		t.Fatalf("double Map on basicGenerator: expected *basicGenerator[int64], got %T", doubled)
	}
	if bg2.schema["type"] != "integer" {
		t.Errorf("double map schema type: expected 'integer', got %v", bg2.schema["type"])
	}
	// Verify composition: input 5 -> +10 -> 15 -> *2 -> 30.
	result := bg2.transform(int64(5))
	if result != 30 {
		t.Errorf("double map compose: input 5, expected 30, got %d", result)
	}

	// Map on mappedGenerator: returns a mappedGenerator.
	mg := &mappedGenerator[int64, int64]{inner: base, fn: func(v int64) int64 { return v }}
	mappedMG := Map[int64, int64](mg, func(v int64) int64 { return v })
	if _, ok := mappedMG.(*mappedGenerator[int64, int64]); !ok {
		t.Errorf("mapping a mappedGenerator should produce *mappedGenerator, got %T", mappedMG)
	}
}

// =============================================================================
// Primitive generator schema unit tests
// =============================================================================

// =============================================================================
// filteredGenerator tests
// =============================================================================

// TestFilteredGeneratorFromBasicIsNotBasic verifies that Filter on a basicGenerator
// returns a filteredGenerator (not a basicGenerator).
func TestFilteredGeneratorFromBasicIsNotBasic(t *testing.T) {
	g := Filter[int64](Integers(0, 100), func(v int64) bool { return true })
	if _, ok := g.(*filteredGenerator[int64]); !ok {
		t.Fatalf("Filter on basicGenerator should return *filteredGenerator[int64], got %T", g)
	}
}

// TestFilteredGeneratorFilterMethod verifies that calling Filter on a filteredGenerator
// returns another filteredGenerator.
func TestFilteredGeneratorFilterMethod(t *testing.T) {
	g := Filter[int64](
		Filter[int64](Integers(0, 100), func(v int64) bool { return true }),
		func(v int64) bool { return true },
	)
	if _, ok := g.(*filteredGenerator[int64]); !ok {
		t.Fatalf("Filter on filteredGenerator should return *filteredGenerator[int64], got %T", g)
	}
}

// TestFilteredGeneratorMapMethod verifies that calling Map on a filteredGenerator
// returns a mappedGenerator.
func TestFilteredGeneratorMapMethod(t *testing.T) {
	g := Filter[int64](Integers(0, 100), func(v int64) bool { return true })
	mapped := Map[int64, int64](g, func(v int64) int64 { return v })
	if _, ok := mapped.(*mappedGenerator[int64, int64]); !ok {
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
		// generate
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, int64(42)) //nolint:errcheck
		// stop_span (discard=false)
		spID, spPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded2, _ := decodeCBOR(spPayload)
		m2, _ := extractCBORDict(decoded2)
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
	err := cli.runTest("filter_passes", func(s *TestCase) {
		g := &filteredGenerator[int64]{
			source:    &basicGenerator[int64]{schema: map[string]any{"type": "integer"}, transform: func(v any) int64 { return extractInt(v) }},
			predicate: func(v int64) bool { return true },
		}
		gotVal = g.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
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
			m2, _ := extractCBORDict(decoded2)
			discard, _ := m2[any("discard")].(bool)
			if !discard {
				t.Errorf("attempt %d: stop_span should have discard=true when predicate fails", i)
			}
			caseCh.SendReplyValue(spID, nil) //nolint:errcheck
			spanCount++
		}

		// Assume(false) panics with assumeRejected -> runner sends mark_complete with "INVALID".
		mcID, mcPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded3, _ := decodeCBOR(mcPayload)
		m3, _ := extractCBORDict(decoded3)
		mcStatus, _ = extractCBORString(m3[any("status")])
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("filter_exhaust", func(s *TestCase) {
		g := &filteredGenerator[int64]{
			source:    &basicGenerator[int64]{schema: map[string]any{"type": "integer"}, transform: func(v any) int64 { return extractInt(v) }},
			predicate: func(v int64) bool { return false }, // always reject
		}
		g.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
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
			m2, _ := extractCBORDict(decoded2)
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
	err := cli.runTest("filter_partial", func(s *TestCase) {
		g := &filteredGenerator[int64]{
			source: &basicGenerator[int64]{schema: map[string]any{"type": "integer"}, transform: func(v any) int64 { return extractInt(v) }},
			predicate: func(v int64) bool {
				attemptNum++
				return v > 0
			},
		}
		gotVal = g.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
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
	if _err := runHegel(t.Name(), func(s *TestCase) {
		gen := Filter[int64](Integers(0, 100), func(v int64) bool {
			return v > 50
		})
		n := Draw[int64](s, gen)
		if n <= 50 {
			panic(fmt.Sprintf("filter(>50): expected n>50, got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestFilteredGeneratorE2EEvenNumbers verifies filter for even numbers.
func TestFilteredGeneratorE2EEvenNumbers(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		gen := Filter[int64](Integers(0, 10), func(v int64) bool {
			return v%2 == 0
		})
		n := Draw[int64](s, gen)
		if n%2 != 0 {
			panic(fmt.Sprintf("filter(even): expected even, got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestFilterOnNonBasicGenerators verifies that Filter works on non-basic generators.
func TestFilterOnNonBasicGenerators(t *testing.T) {
	// mappedGenerator.Filter
	mg := &mappedGenerator[int64, int64]{inner: Integers(0, 5), fn: func(v int64) int64 { return v }}
	fg := Filter[int64](mg, func(v int64) bool { return true })
	if _, ok := fg.(*filteredGenerator[int64]); !ok {
		t.Errorf("Filter on mappedGenerator should return *filteredGenerator, got %T", fg)
	}
	// compositeListGenerator.Filter
	cl := &compositeListGenerator[int64]{elements: Integers(0, 5), minSize: 0, maxSize: 3}
	fg2 := Filter[[]int64](cl, func(v []int64) bool { return true })
	if _, ok := fg2.(*filteredGenerator[[]int64]); !ok {
		t.Errorf("Filter on compositeListGenerator should return *filteredGenerator, got %T", fg2)
	}
	// compositeDictGenerator.Filter
	cd := &compositeDictGenerator[int64, int64]{keys: Integers(0, 5), values: Integers(0, 5), minSize: 0}
	fg3 := Filter[map[int64]int64](cd, func(v map[int64]int64) bool { return true })
	if _, ok := fg3.(*filteredGenerator[map[int64]int64]); !ok {
		t.Errorf("Filter on compositeDictGenerator should return *filteredGenerator, got %T", fg3)
	}
	// compositeOneOfGenerator.Filter
	co := &compositeOneOfGenerator[int64]{generators: []Generator[int64]{Integers(0, 5), Integers(6, 10)}}
	fg4 := Filter[int64](co, func(v int64) bool { return true })
	if _, ok := fg4.(*filteredGenerator[int64]); !ok {
		t.Errorf("Filter on compositeOneOfGenerator should return *filteredGenerator, got %T", fg4)
	}
	// flatMappedGenerator.Filter
	fm := &flatMappedGenerator[int64, int64]{source: Integers(0, 5), f: func(v int64) Generator[int64] { return Integers(0, 5) }}
	fg5 := Filter[int64](fm, func(v int64) bool { return true })
	if _, ok := fg5.(*filteredGenerator[int64]); !ok {
		t.Errorf("Filter on flatMappedGenerator should return *filteredGenerator, got %T", fg5)
	}
}

// TestBooleansSchema verifies that Booleans produces a schema with type=boolean and p field.
func TestBooleansSchema(t *testing.T) {
	g := Booleans(0.5)
	bg, ok := g.(*basicGenerator[bool])
	if !ok {
		t.Fatalf("Booleans should return *basicGenerator[bool], got %T", g)
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
}

// TestBooleansP1Schema verifies that Booleans(1.0) stores p=1.0.
func TestBooleansP1Schema(t *testing.T) {
	g := Booleans(1.0)
	bg := g.(*basicGenerator[bool])
	if bg.schema["p"] != 1.0 {
		t.Errorf("p: expected 1.0, got %v", bg.schema["p"])
	}
}

// TestTextSchema verifies that Text produces the correct schema structure.
func TestTextSchema(t *testing.T) {
	g := Text(3, 10)
	bg, ok := g.(*basicGenerator[string])
	if !ok {
		t.Fatalf("Text should return *basicGenerator[string], got %T", g)
	}
	if bg.schema["type"] != "string" {
		t.Errorf("type: expected 'string', got %v", bg.schema["type"])
	}
	minSize := bg.schema["min_size"].(int64)
	if minSize != 3 {
		t.Errorf("min_size: expected 3, got %d", minSize)
	}
	maxSize := bg.schema["max_size"].(int64)
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
	bg := g.(*basicGenerator[string])
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when maxSize < 0")
	}
	minSize := bg.schema["min_size"].(int64)
	if minSize != 0 {
		t.Errorf("min_size: expected 0, got %d", minSize)
	}
}

// TestBinarySchema verifies that Binary produces the correct schema structure.
func TestBinarySchema(t *testing.T) {
	g := Binary(1, 20)
	bg, ok := g.(*basicGenerator[[]byte])
	if !ok {
		t.Fatalf("Binary should return *basicGenerator[[]byte], got %T", g)
	}
	if bg.schema["type"] != "binary" {
		t.Errorf("type: expected 'binary', got %v", bg.schema["type"])
	}
	minSize := bg.schema["min_size"].(int64)
	if minSize != 1 {
		t.Errorf("min_size: expected 1, got %d", minSize)
	}
	maxSize := bg.schema["max_size"].(int64)
	if maxSize != 20 {
		t.Errorf("max_size: expected 20, got %d", maxSize)
	}
	// No transform needed -- server returns []byte directly via CBOR byte strings.
	if bg.transform != nil {
		t.Error("Binary should have no transform")
	}
}

// TestBinarySchemaNoMax verifies that Binary with maxSize<0 omits max_size from schema.
func TestBinarySchemaNoMax(t *testing.T) {
	g := Binary(0, -1)
	bg := g.(*basicGenerator[[]byte])
	if _, hasMax := bg.schema["max_size"]; hasMax {
		t.Error("max_size should not be present when maxSize < 0")
	}
}

// TestIntegersFromSchema verifies that IntegersFrom produces the correct schema.
func TestIntegersFromSchema(t *testing.T) {
	minV := int64(-10)
	maxV := int64(10)
	g := IntegersFrom(&minV, &maxV)
	bg, ok := g.(*basicGenerator[int64])
	if !ok {
		t.Fatalf("IntegersFrom should return *basicGenerator[int64], got %T", g)
	}
	if bg.schema["type"] != "integer" {
		t.Errorf("type: expected 'integer', got %v", bg.schema["type"])
	}
	minVal := bg.schema["min_value"].(int64)
	maxVal := bg.schema["max_value"].(int64)
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
	bg := g.(*basicGenerator[int64])
	if _, hasMax := bg.schema["max_value"]; hasMax {
		t.Error("max_value should not be present when maxVal is nil")
	}
}

// TestIntegersFromSchemaOnlyMax verifies that IntegersFrom with only a max bound omits min_value.
func TestIntegersFromSchemaOnlyMax(t *testing.T) {
	maxV := int64(99)
	g := IntegersFrom(nil, &maxV)
	bg := g.(*basicGenerator[int64])
	if _, hasMin := bg.schema["min_value"]; hasMin {
		t.Error("min_value should not be present when minVal is nil")
	}
}

// TestFloatsSchemaWithBounds verifies that Floats with explicit bounds sets all schema fields.
func TestFloatsSchemaWithBounds(t *testing.T) {
	minV := 0.0
	maxV := 1.0
	falseV := false
	g := Floats(&minV, &maxV, &falseV, &falseV, false, false)
	bg, ok := g.(*basicGenerator[float64])
	if !ok {
		t.Fatalf("Floats should return *basicGenerator[float64], got %T", g)
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
}

// TestFloatsSchemaUnbounded verifies that Floats with no bounds defaults allow_nan=true, allow_infinity=true.
func TestFloatsSchemaUnbounded(t *testing.T) {
	g := Floats(nil, nil, nil, nil, false, false)
	bg := g.(*basicGenerator[float64])
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
	bg := g.(*basicGenerator[float64])
	// has_min=true, has_max=false -> allow_nan=false, allow_infinity=true
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
	bg := g.(*basicGenerator[float64])
	// has_min=false, has_max=true -> allow_nan=false, allow_infinity=true
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
	bg := g.(*basicGenerator[float64])
	if bg.schema["exclude_min"] != true {
		t.Errorf("exclude_min: expected true, got %v", bg.schema["exclude_min"])
	}
	if bg.schema["exclude_max"] != true {
		t.Errorf("exclude_max: expected true, got %v", bg.schema["exclude_max"])
	}
}

// =============================================================================
// flatMappedGenerator tests
// =============================================================================

// TestFlatMappedGeneratorIsNotBasic verifies that FlatMap returns a *flatMappedGenerator (not basicGenerator).
func TestFlatMappedGeneratorIsNotBasic(t *testing.T) {
	gen := FlatMap[int64, int64](IntegersUnbounded(), func(v int64) Generator[int64] {
		return IntegersUnbounded()
	})
	if _, ok := gen.(*flatMappedGenerator[int64, int64]); !ok {
		t.Fatalf("FlatMap should return *flatMappedGenerator, got %T", gen)
	}
	// flatMappedGenerator is never a basicGenerator.
	if _, ok := gen.(*basicGenerator[int64]); ok {
		t.Error("FlatMap result should not be a *basicGenerator")
	}
}

// TestFlatMappedGeneratorMapReturnsMapped verifies that Map on flatMappedGenerator returns a mappedGenerator.
func TestFlatMappedGeneratorMapReturnsMapped(t *testing.T) {
	gen := FlatMap[int64, int64](Integers(1, 5), func(v int64) Generator[int64] {
		return Integers(0, 10)
	})
	mapped := Map[int64, int64](gen, func(v int64) int64 { return v })
	if _, ok := mapped.(*mappedGenerator[int64, int64]); !ok {
		t.Fatalf("Map on flatMappedGenerator should return *mappedGenerator, got %T", mapped)
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

		// Expect: start_span(11), generate(source), generate(second), stop_span, mark_complete
		for i := 0; i < 5; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := decodeCBOR(pl)
			mp, _ := extractCBORDict(dec)
			cmd, _ := extractCBORString(mp[any("command")])
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
	err := cli.runTest("flatmap_protocol", func(s *TestCase) {
		gen := FlatMap[int64, int64](
			Integers(0, 100),
			func(v int64) Generator[int64] { return Integers(0, 100) },
		)
		gotVal = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
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

		for i := 0; i < 5; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := decodeCBOR(pl)
			mp, _ := extractCBORDict(dec)
			cmd, _ := extractCBORString(mp[any("command")])
			if cmd == "start_span" {
				gotLabel, _ = extractCBORInt(mp[any("label")])
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
	err := cli.runTest("flatmap_label", func(s *TestCase) {
		gen := FlatMap[int64, int64](Integers(0, 10), func(v int64) Generator[int64] { return Integers(0, 10) })
		_ = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotLabel != int64(labelFlatMap) {
		t.Errorf("start_span label: expected %d (labelFlatMap), got %d", labelFlatMap, gotLabel)
	}
}

// TestFlatMappedGeneratorE2E verifies that flat_map produces a dependent value.
// integers(1,5).flat_map(n => text(min=n, max=n)) always produces text of length in [1,5].
func TestFlatMappedGeneratorE2E(t *testing.T) {
	hegelBinPath(t)
	gen := FlatMap[int64, string](Integers(1, 5), func(v int64) Generator[string] {
		return Text(int(v), int(v)) // exact length = n
	})
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[string](s, gen)
		count := len([]rune(v))
		// n is in [1,5], so text length is in [1,5].
		if count < 1 || count > 5 {
			panic(fmt.Sprintf("flat_map text length %d out of [1,5]", count))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestFlatMappedGeneratorDependency verifies that the second generation genuinely depends
// on the first generated value. We generate n in [2,4] and a list of exactly n elements.
// Every list must have length in [2,4] and all elements must be in [0,100].
func TestFlatMappedGeneratorDependency(t *testing.T) {
	hegelBinPath(t)
	gen := FlatMap[int64, []int64](Integers(2, 4), func(v int64) Generator[[]int64] {
		sz := int(v)
		return Lists[int64](Integers(0, 100), ListsOptions{MinSize: sz, MaxSize: sz})
	})
	if _err := runHegel(t.Name(), func(s *TestCase) {
		slice := Draw[[]int64](s, gen)
		if len(slice) < 2 || len(slice) > 4 {
			panic(fmt.Sprintf("flat_map dependency: list length %d not in [2,4]", len(slice)))
		}
		for _, elem := range slice {
			if elem < 0 || elem > 100 {
				panic(fmt.Sprintf("flat_map dependency: element %d not in [0,100]", elem))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// =============================================================================
// isSchemaIdentity
// =============================================================================

func TestIsSchemaIdentityTrue(t *testing.T) {
	bg := &basicGenerator[string]{schema: map[string]any{"type": "string"}}
	if !bg.isSchemaIdentity() {
		t.Error("expected identity when transform is nil")
	}
}

func TestIsSchemaIdentityFalse(t *testing.T) {
	bg := &basicGenerator[string]{
		schema:    map[string]any{"type": "string"},
		transform: func(v any) string { return v.(string) },
	}
	if bg.isSchemaIdentity() {
		t.Error("expected non-identity when transform is set")
	}
}

// =============================================================================
// extractFloat — all branches
// =============================================================================

func TestExtractFloatFloat64(t *testing.T) {
	if extractFloat(float64(1.5)) != 1.5 {
		t.Error("float64 branch failed")
	}
}

func TestExtractFloatFloat32(t *testing.T) {
	if extractFloat(float32(1.5)) != float64(float32(1.5)) {
		t.Error("float32 branch failed")
	}
}

func TestExtractFloatInt64(t *testing.T) {
	if extractFloat(int64(42)) != 42.0 {
		t.Error("int64 branch failed")
	}
}

func TestExtractFloatUint64(t *testing.T) {
	if extractFloat(uint64(42)) != 42.0 {
		t.Error("uint64 branch failed")
	}
}

func TestExtractFloatPanicsOnInvalidType(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid type")
		}
	}()
	extractFloat("not a number")
}

// =============================================================================
// extractInt — uint64 branch
// =============================================================================

func TestExtractIntUint64(t *testing.T) {
	if extractInt(uint64(99)) != 99 {
		t.Error("uint64 branch failed")
	}
}

func TestExtractIntBigIntValue(t *testing.T) {
	v := *new(big.Int).SetInt64(456)
	if extractInt(v) != 456 {
		t.Error("big.Int value branch failed")
	}
}

func TestExtractIntBigIntPointer(t *testing.T) {
	v := new(big.Int).SetInt64(123)
	if extractInt(v) != 123 {
		t.Error("*big.Int branch failed")
	}
}

func TestExtractIntPanicsOnInvalidType(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for invalid type")
		}
	}()
	extractInt("not a number")
}

// =============================================================================
// IntegersUnbounded — transform via fake server
// =============================================================================

func TestIntegersUnboundedTransformFakeServer(t *testing.T) {
	gen := IntegersUnbounded()

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

		// generate: reply with uint64 to test the extractInt uint64 path
		genID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(genID, uint64(42)) //nolint:errcheck

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var got int64
	err := cli.runTest("integers_unbounded_transform", func(s *TestCase) {
		got = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
}

// =============================================================================
// IntegersFrom nil bounds — transform
// =============================================================================

func TestIntegersFromNilBoundsTransformFakeServer(t *testing.T) {
	gen := IntegersFrom(nil, nil)

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

		// generate: reply with int64
		genID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(genID, int64(-99)) //nolint:errcheck

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var got int64
	err := cli.runTest("integers_from_nil", func(s *TestCase) {
		got = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if got != -99 {
		t.Errorf("expected -99, got %d", got)
	}
}

// =============================================================================
// Floats — transform via fake server
// =============================================================================

func TestFloatsTransformFakeServer(t *testing.T) {
	gen := Floats(nil, nil, nil, nil, false, false)

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

		// generate: reply with float64
		genID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(genID, float64(3.14)) //nolint:errcheck

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var got float64
	err := cli.runTest("floats_transform", func(s *TestCase) {
		got = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if got != 3.14 {
		t.Errorf("expected 3.14, got %f", got)
	}
}

// =============================================================================
// Floats: schema check with only allowNaN set, allowInfinity nil
// =============================================================================

func TestFloatsSchemaExplicitNaNNilInf(t *testing.T) {
	nan := true
	minV := 0.0
	maxV := 1.0
	g := Floats(&minV, &maxV, &nan, nil, false, false)
	bg := g.(*basicGenerator[float64])
	if bg.schema["allow_nan"] != true {
		t.Errorf("allow_nan: expected true, got %v", bg.schema["allow_nan"])
	}
	if bg.schema["allow_infinity"] != false {
		t.Errorf("allow_infinity: expected false (default with both bounds), got %v", bg.schema["allow_infinity"])
	}
}

// =============================================================================
// Floats: schema check with allowNaN nil, allowInfinity set
// =============================================================================

func TestFloatsSchemaExplicitInfNilNaN(t *testing.T) {
	inf := true
	minV := 0.0
	maxV := 1.0
	g := Floats(&minV, &maxV, nil, &inf, false, false)
	bg := g.(*basicGenerator[float64])
	if bg.schema["allow_nan"] != false {
		t.Errorf("allow_nan: expected false (default with both bounds), got %v", bg.schema["allow_nan"])
	}
	if bg.schema["allow_infinity"] != true {
		t.Errorf("allow_infinity: expected true, got %v", bg.schema["allow_infinity"])
	}
}

// =============================================================================
// Optional generator — via fake server (covers optionalGenerator.draw)
// =============================================================================

func TestOptionalGeneratorDrawNilFakeServer(t *testing.T) {
	gen := Optional(Integers(0, 100))

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

		// start_span (ONE_OF)
		ssID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// generate (index): reply 0 = nil
		genID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(genID, int64(0)) //nolint:errcheck

		// stop_span (ONE_OF)
		spID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var result *int64
	err := cli.runTest("optional_nil", func(s *TestCase) {
		result = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil, got %v", *result)
	}
}

func TestOptionalGeneratorDrawValueFakeServer(t *testing.T) {
	gen := Optional(Integers(0, 100))

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

		// start_span (ONE_OF)
		ssID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// generate (index): reply 1 = non-nil
		genID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(genID, int64(1)) //nolint:errcheck

		// generate (the actual integer value)
		innerGenID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(innerGenID, int64(42)) //nolint:errcheck

		// stop_span (ONE_OF)
		spID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var result *int64
	err := cli.runTest("optional_value", func(s *TestCase) {
		result = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	if *result != 42 {
		t.Errorf("expected 42, got %d", *result)
	}
}

// =============================================================================
// newCollection: StopTest error path
// =============================================================================

func TestNewCollectionStopTestFakeServer(t *testing.T) {
	inner := Integers(0, 10)
	nonBasic := &mappedGenerator[int64, int64]{inner: inner, fn: func(v int64) int64 { return v }}
	gen := Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 5})

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

		// start_span for list
		ssID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// new_collection: reply with StopTest error
		ncID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyError(ncID, "data exhausted", "StopTest") //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("new_collection_stoptest", func(s *TestCase) {
		_ = gen.draw(s) // should trigger StopTest -> dataExhausted panic -> aborted
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
}

// =============================================================================
// collection.More: StopTest error path
// =============================================================================

func TestCollectionMoreStopTestFakeServer(t *testing.T) {
	inner := Integers(0, 10)
	nonBasic := &mappedGenerator[int64, int64]{inner: inner, fn: func(v int64) int64 { return v }}
	gen := Lists(nonBasic, ListsOptions{MinSize: 0, MaxSize: 5})

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

		// start_span for list
		ssID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck

		// new_collection: succeed
		ncID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_test") //nolint:errcheck

		// collection_more: reply with StopTest error
		moreID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyError(moreID, "data exhausted more", "StopTest") //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("collection_more_stoptest", func(s *TestCase) {
		_ = gen.draw(s) // should trigger StopTest during collection_more
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
}

// =============================================================================
// startSpan/stopSpan: aborted path (no-op)
// =============================================================================

func TestStartSpanAborted(t *testing.T) {
	s := &TestCase{aborted: true}
	// Should be a no-op, not panic.
	startSpan(s, labelOneOf)
}

func TestStopSpanAborted(t *testing.T) {
	s := &TestCase{aborted: true}
	// Should be a no-op, not panic.
	stopSpan(s, false)
}

// =============================================================================
// Reject: finished collection path
// =============================================================================

func TestRejectFinishedCollection(t *testing.T) {
	c := &collection{finished: true}
	s := &TestCase{}
	// Should be a no-op since finished = true.
	c.Reject(s)
}

// =============================================================================
// Lists: identity-transform path for basic elements (no user transform)
// =============================================================================

func TestListsIdentityTransformFakeServer(t *testing.T) {
	// Lists(Booleans(0.5)) on a basic generator with nil transform.
	// This hits the identity transform path in Lists (lines 616-629).
	gen := Lists(Booleans(0.5), ListsOptions{MinSize: 0, MaxSize: 3})

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

		// generate: reply with a list of booleans
		genID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(genID, []any{true, false}) //nolint:errcheck

		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(2 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var got []bool
	err := cli.runTest("lists_identity_transform", func(s *TestCase) {
		got = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 elements, got %d", len(got))
	}
	if got[0] != true || got[1] != false {
		t.Errorf("expected [true, false], got %v", got)
	}
}

// =============================================================================
// Lists: MaxSize >= 0, MinSize < 0 (clamping path) - schema check
// =============================================================================

func TestListsNegativeMinSizeSchema(t *testing.T) {
	gen := Lists(Integers(0, 10), ListsOptions{MinSize: -5, MaxSize: 10})
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
// compositeOneOfGenerator.draw — via fake server with uint64 index response
// =============================================================================

func TestCompositeOneOfDrawUint64Index(t *testing.T) {
	nonBasic := &mappedGenerator[int64, int64]{inner: Integers(0, 100), fn: func(v int64) int64 { return v }}
	gen := &compositeOneOfGenerator[int64]{generators: []Generator[int64]{nonBasic, Integers(0, 5)}}

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

		// start_span (ONE_OF)
		ssID1, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID1, nil) //nolint:errcheck
		// generate (index: pick branch 1 which is a basic Integers)
		genID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(genID, uint64(1)) //nolint:errcheck
		// generate (inner integer for branch 1)
		innerGenID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(innerGenID, int64(3)) //nolint:errcheck
		// stop_span (ONE_OF)
		spID1, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(spID1, nil) //nolint:errcheck
		// mark_complete
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var got int64
	err := cli.runTest("composite_oneof_uint64", func(s *TestCase) {
		got = gen.draw(s)
	}, runOptions{testCases: 1}, stderrNoteFn)
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if got != 3 {
		t.Errorf("expected 3, got %d", got)
	}
}
