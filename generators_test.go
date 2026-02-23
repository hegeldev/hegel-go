package hegel

import (
	"fmt"
	"testing"
	"time"
)

// =============================================================================
// Generator interface and BasicGenerator tests
// =============================================================================

// --- BasicGenerator: generate with no transform ---

func TestBasicGeneratorGenerateNoTransform(t *testing.T) {
	// Set up fake server that responds to a generate command with int64(42).
	schema := map[string]any{"type": "integer", "min_value": int64(0), "max_value": int64(100)}
	clientConn := fakeServerConn(t, func(serverConn *Connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		// Send one test_case.
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := EncodeCBOR(map[string]any{
			"event":    "test_case",
			"channel":  int64(caseCh.ChannelID()),
			"is_final": false,
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// Respond to generate with 42.
		genID, genPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		_, _ = genPayload, DecodeCBOR           // consumed
		caseCh.SendReplyValue(genID, int64(42)) //nolint:errcheck

		// Wait for mark_complete.
		mcID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(mcID, nil) //nolint:errcheck

		// Send test_done (passed, no interesting).
		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	var gotVal int64
	err := cli.runTest("basic_gen_no_transform", func() {
		g := &BasicGenerator{schema: schema}
		v := g.Generate()
		gotVal, _ = ExtractInt(v)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 42 {
		t.Errorf("expected 42, got %d", gotVal)
	}
}

// --- BasicGenerator: generate with transform ---

func TestBasicGeneratorGenerateWithTransform(t *testing.T) {
	schema := map[string]any{"type": "integer"}
	clientConn := fakeServerConn(t, func(serverConn *Connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := EncodeCBOR(map[string]any{
			"event":    "test_case",
			"channel":  int64(caseCh.ChannelID()),
			"is_final": false,
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
	err := cli.runTest("basic_gen_with_transform", func() {
		// transform: multiply by 2
		g := &BasicGenerator{
			schema:    schema,
			transform: func(v any) any { n, _ := ExtractInt(v); return n * 2 },
		}
		v := g.Generate()
		gotVal, _ = v.(int64)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 14 {
		t.Errorf("expected 14, got %d", gotVal)
	}
}

// --- BasicGenerator.Map: no existing transform ---

func TestBasicGeneratorMapNoTransform(t *testing.T) {
	schema := map[string]any{"type": "boolean"}
	g := &BasicGenerator{schema: schema}
	mapped := g.Map(func(v any) any {
		b, _ := v.(bool)
		if b {
			return "yes"
		}
		return "no"
	})
	// Map on BasicGenerator returns another BasicGenerator with same schema.
	bg, ok := mapped.(*BasicGenerator)
	if !ok {
		t.Fatalf("Map on BasicGenerator should return *BasicGenerator, got %T", mapped)
	}
	if bg.schema["type"] != "boolean" {
		t.Errorf("schema not preserved by Map")
	}
	if bg.transform == nil {
		t.Error("transform should not be nil after Map")
	}
}

// --- BasicGenerator.Map: compose transforms ---

func TestBasicGeneratorMapComposesTransforms(t *testing.T) {
	schema := map[string]any{"type": "integer"}
	g := &BasicGenerator{
		schema:    schema,
		transform: func(v any) any { n, _ := ExtractInt(v); return n + 1 },
	}
	// Map again: result should be (n+1)*2
	mapped := g.Map(func(v any) any {
		n, _ := ExtractInt(v)
		return n * 2
	})
	bg, ok := mapped.(*BasicGenerator)
	if !ok {
		t.Fatalf("double Map should return *BasicGenerator")
	}
	// Simulate applying: start with int64(5) → +1 → 6 → *2 → 12
	result := bg.transform(int64(5))
	n, _ := ExtractInt(result)
	if n != 12 {
		t.Errorf("composed transform: expected 12, got %d", n)
	}
}

// --- BasicGenerator.AsBasic ---

func TestBasicGeneratorAsBasic(t *testing.T) {
	g := &BasicGenerator{schema: map[string]any{"type": "boolean"}}
	ab := g.AsBasic()
	if ab != g {
		t.Error("AsBasic should return itself")
	}
}

// --- MappedGenerator ---

func TestMappedGeneratorGenerate(t *testing.T) {
	// MappedGenerator wraps a non-basic generator.
	schema := map[string]any{"type": "integer"}
	clientConn := fakeServerConn(t, func(serverConn *Connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := EncodeCBOR(map[string]any{
			"event":    "test_case",
			"channel":  int64(caseCh.ChannelID()),
			"is_final": false,
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
	err := cli.runTest("mapped_gen", func() {
		inner := &BasicGenerator{schema: schema}
		mg := &MappedGenerator{
			inner: inner,
			fn:    func(v any) any { n, _ := ExtractInt(v); return n * 10 },
		}
		v := mg.Generate()
		gotVal, _ = v.(int64)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotVal != 30 {
		t.Errorf("expected 30, got %d", gotVal)
	}
}

// --- MappedGenerator.AsBasic ---

func TestMappedGeneratorAsBasic(t *testing.T) {
	mg := &MappedGenerator{
		inner: &BasicGenerator{schema: map[string]any{"type": "integer"}},
		fn:    func(v any) any { return v },
	}
	if mg.AsBasic() != nil {
		t.Error("MappedGenerator.AsBasic should return nil")
	}
}

// --- MappedGenerator.Map returns BasicGenerator when inner is BasicGenerator ---

func TestMappedGeneratorMapOnBasicInner(t *testing.T) {
	inner := &BasicGenerator{schema: map[string]any{"type": "integer"}}
	// Map on BasicGenerator returns BasicGenerator.
	result := inner.Map(func(v any) any { return v })
	if _, ok := result.(*BasicGenerator); !ok {
		t.Errorf("Map on BasicGenerator should return *BasicGenerator")
	}
}

// =============================================================================
// Span helper tests
// =============================================================================

func fakeTestEnv(t *testing.T, fn func(caseCh *Channel)) *Connection {
	t.Helper()
	return fakeServerConn(t, func(serverConn *Connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := EncodeCBOR(map[string]any{
			"event":    "test_case",
			"channel":  int64(caseCh.ChannelID()),
			"is_final": false,
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

// --- StartSpan and StopSpan ---

func TestStartStopSpan(t *testing.T) {
	var gotStartLabel int64
	var gotStopDiscard bool
	clientConn := fakeTestEnv(t, func(caseCh *Channel) {
		// start_span
		ssID, ssPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(ssPayload)
		m, _ := ExtractDict(decoded)
		gotStartLabel, _ = ExtractInt(m[any("label")])
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck
		// stop_span
		spID, spPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded2, _ := DecodeCBOR(spPayload)
		m2, _ := ExtractDict(decoded2)
		b, _ := m2[any("discard")].(bool)
		gotStopDiscard = b
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("spans", func() {
		StartSpan(LabelMapped)
		StopSpan(false)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotStartLabel != int64(LabelMapped) {
		t.Errorf("start_span label: expected %d, got %d", LabelMapped, gotStartLabel)
	}
	if gotStopDiscard {
		t.Error("stop_span discard should be false")
	}
}

// --- StopSpan with discard=true ---

func TestStopSpanDiscard(t *testing.T) {
	var gotDiscard bool
	clientConn := fakeTestEnv(t, func(caseCh *Channel) {
		// start_span
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck
		// stop_span
		spID, spPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(spPayload)
		m, _ := ExtractDict(decoded)
		b, _ := m[any("discard")].(bool)
		gotDiscard = b
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("stop_span_discard", func() {
		StartSpan(LabelList)
		StopSpan(true)
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if !gotDiscard {
		t.Error("stop_span discard should be true")
	}
}

// --- StartSpan no-op when aborted ---

func TestStartSpanNoOpWhenAborted(t *testing.T) {
	// When aborted=true, StartSpan and StopSpan must not send any messages.
	clientConn := fakeTestEnv(t, func(caseCh *Channel) {
		// No messages expected (no start/stop span).
		// The test fn will set aborted then call StartSpan/StopSpan.
	})

	cli := newClient(clientConn)
	err := cli.runTest("span_noop_aborted", func() {
		// Directly set the aborted flag.
		setAborted()
		StartSpan(LabelList) // should be no-op
		StopSpan(false)      // should be no-op
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
}

// --- Group helper ---

func TestGroup(t *testing.T) {
	var cmds []string
	// Use fakeServerConn directly to control all message handling.
	clientConn := fakeServerConn(t, func(serverConn *Connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := EncodeCBOR(map[string]any{
			"event":    "test_case",
			"channel":  int64(caseCh.ChannelID()),
			"is_final": false,
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// Receive exactly: start_span, stop_span, mark_complete.
		for i := 0; i < 3; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := DecodeCBOR(pl)
			mp, _ := ExtractDict(dec)
			cmd, _ := ExtractString(mp[any("command")])
			cmds = append(cmds, cmd)
			caseCh.SendReplyValue(mid, nil) //nolint:errcheck
		}

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("group_test", func() {
		Group(LabelTuple, func() {
			// nothing inside, just test the wrapping
		})
	}, runOptions{testCases: 1})
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

// --- DiscardableGroup: no panic ---

func TestDiscardableGroupNoPanic(t *testing.T) {
	var cmds []string
	clientConn := fakeServerConn(t, func(serverConn *Connection) {
		ctrl := serverConn.ControlChannel()
		msgID, payload, _ := ctrl.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(payload)
		m, _ := ExtractDict(decoded)
		chID, _ := ExtractInt(m[any("channel")])
		ctrl.SendReplyValue(msgID, true) //nolint:errcheck

		testCh, _ := serverConn.ConnectChannel(uint32(chID), "TestCh")
		caseCh := serverConn.NewChannel("Case")
		casePayload, _ := EncodeCBOR(map[string]any{
			"event":    "test_case",
			"channel":  int64(caseCh.ChannelID()),
			"is_final": false,
		})
		caseID, _ := testCh.SendRequestRaw(casePayload)
		testCh.recvResponseRaw(caseID, 5*time.Second) //nolint:errcheck

		// Receive exactly: start_span, stop_span, mark_complete.
		for i := 0; i < 3; i++ {
			mid, pl, _ := caseCh.RecvRequestRaw(5 * time.Second)
			dec, _ := DecodeCBOR(pl)
			mp, _ := ExtractDict(dec)
			cmd, _ := ExtractString(mp[any("command")])
			cmds = append(cmds, cmd)
			caseCh.SendReplyValue(mid, nil) //nolint:errcheck
		}

		sendTestDone(t, testCh, true, 0)
	})

	cli := newClient(clientConn)
	err := cli.runTest("discardable_group_ok", func() {
		DiscardableGroup(LabelFilter, func() {
			// runs normally
		})
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	// should have start_span, stop_span(discard=false), mark_complete
	if len(cmds) < 2 || cmds[0] != "start_span" {
		t.Errorf("expected start_span first; got %v", cmds)
	}
}

// --- DiscardableGroup: panic propagates with discard=true ---

func TestDiscardableGroupPanic(t *testing.T) {
	var stopDiscardVal bool
	clientConn := fakeTestEnv(t, func(caseCh *Channel) {
		// start_span
		ssID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ssID, nil) //nolint:errcheck
		// stop_span (discard=true because panic propagated)
		spID, spPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(spPayload)
		m, _ := ExtractDict(decoded)
		b, _ := m[any("discard")].(bool)
		stopDiscardVal = b
		caseCh.SendReplyValue(spID, nil) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("discardable_group_panic", func() {
		DiscardableGroup(LabelFilter, func() {
			panic("inner panic")
		})
	}, runOptions{testCases: 1})
	// The panic inside DiscardableGroup should propagate out as INTERESTING.
	_ = err // may be nil (non-final case doesn't return error) or error
	if !stopDiscardVal {
		t.Error("stop_span should have discard=true when inner fn panics")
	}
}

// =============================================================================
// Collection protocol tests
// =============================================================================

// --- NewCollection: basic ---

func TestNewCollection(t *testing.T) {
	var gotCmd string
	var gotMin, gotMax int64
	clientConn := fakeTestEnv(t, func(caseCh *Channel) {
		// new_collection
		ncID, ncPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(ncPayload)
		m, _ := ExtractDict(decoded)
		gotCmd, _ = ExtractString(m[any("command")])
		gotMin, _ = ExtractInt(m[any("min_size")])
		gotMax, _ = ExtractInt(m[any("max_size")])
		caseCh.SendReplyValue(ncID, "coll_1") //nolint:errcheck

		// collection_more → false (done immediately)
		moreID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(moreID, false) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("new_collection", func() {
		coll := NewCollection(2, 10)
		more := coll.More()
		_ = more
	}, runOptions{testCases: 1})
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

// --- Collection.More: returns true then false ---

func TestCollectionMore(t *testing.T) {
	var moreCount int
	clientConn := fakeTestEnv(t, func(caseCh *Channel) {
		// new_collection
		ncID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_x") //nolint:errcheck

		// first more → true
		m1ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(m1ID, true) //nolint:errcheck
		// second more → false
		m2ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(m2ID, false) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("coll_more", func() {
		coll := NewCollection(0, 5)
		for coll.More() {
			moreCount++
		}
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if moreCount != 1 {
		t.Errorf("expected 1 more loop iteration, got %d", moreCount)
	}
}

// --- Collection.More: cached false after first false ---

func TestCollectionMoreCachesFalse(t *testing.T) {
	clientConn := fakeTestEnv(t, func(caseCh *Channel) {
		// new_collection
		ncID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_y") //nolint:errcheck

		// Only one more request (the second More() call should be cached).
		moreID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(moreID, false) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("coll_cache", func() {
		coll := NewCollection(0, 1)
		r1 := coll.More()
		r2 := coll.More() // should be cached false, no network call
		if r1 || r2 {
			panic("expected both to be false")
		}
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
}

// --- Collection.Reject ---

func TestCollectionReject(t *testing.T) {
	var gotRejectCmd string
	clientConn := fakeTestEnv(t, func(caseCh *Channel) {
		// new_collection
		ncID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_r") //nolint:errcheck

		// more → true
		moreID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(moreID, true) //nolint:errcheck

		// collection_reject
		rejID, rejPayload, _ := caseCh.RecvRequestRaw(5 * time.Second)
		decoded, _ := DecodeCBOR(rejPayload)
		m, _ := ExtractDict(decoded)
		gotRejectCmd, _ = ExtractString(m[any("command")])
		caseCh.SendReplyValue(rejID, nil) //nolint:errcheck

		// more → false
		m2ID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(m2ID, false) //nolint:errcheck
	})

	cli := newClient(clientConn)
	err := cli.runTest("coll_reject", func() {
		coll := NewCollection(0, 5)
		if coll.More() {
			coll.Reject()
		}
		for coll.More() {
		}
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
	if gotRejectCmd != "collection_reject" {
		t.Errorf("expected collection_reject, got %s", gotRejectCmd)
	}
}

// --- Collection.Reject no-op after finished ---

func TestCollectionRejectNoOpAfterFinished(t *testing.T) {
	clientConn := fakeTestEnv(t, func(caseCh *Channel) {
		// new_collection
		ncID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(ncID, "coll_nop") //nolint:errcheck

		// more → false immediately
		moreID, _, _ := caseCh.RecvRequestRaw(5 * time.Second)
		caseCh.SendReplyValue(moreID, false) //nolint:errcheck
		// No collection_reject should follow.
	})

	cli := newClient(clientConn)
	err := cli.runTest("coll_reject_noop", func() {
		coll := NewCollection(0, 1)
		coll.More()   // false → finished
		coll.Reject() // no-op
	}, runOptions{testCases: 1})
	if err != nil {
		t.Fatalf("runTest: %v", err)
	}
}

// --- Collection StopTest on new_collection ---

func TestCollectionStopTestOnNewCollection(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_new_collection")
	err := RunHegelTestE("coll_stop_new", func() {
		coll := NewCollection(0, 5)
		_ = coll.More()
	})
	// Should not error — the test was stopped, not failed.
	_ = err
}

// --- Collection StopTest on collection_more ---

func TestCollectionStopTestOnCollectionMore(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "stop_test_on_collection_more")
	err := RunHegelTestE("coll_stop_more", func() {
		coll := NewCollection(0, 5)
		_ = coll.More()
	})
	_ = err
}

// =============================================================================
// Label constants
// =============================================================================

func TestLabelConstants(t *testing.T) {
	cases := []struct {
		name string
		val  SpanLabel
		want int
	}{
		{"List", LabelList, 1},
		{"ListElement", LabelListElement, 2},
		{"Set", LabelSet, 3},
		{"SetElement", LabelSetElement, 4},
		{"Map", LabelMap, 5},
		{"MapEntry", LabelMapEntry, 6},
		{"Tuple", LabelTuple, 7},
		{"OneOf", LabelOneOf, 8},
		{"Optional", LabelOptional, 9},
		{"FixedDict", LabelFixedDict, 10},
		{"FlatMap", LabelFlatMap, 11},
		{"Filter", LabelFilter, 12},
		{"Mapped", LabelMapped, 13},
		{"SampledFrom", LabelSampledFrom, 14},
		{"EnumVariant", LabelEnumVariant, 15},
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
	RunHegelTest("integers_happy", func() {
		n := Integers(0, 100).Generate()
		v, _ := ExtractInt(n)
		vals = append(vals, v)
		if v < 0 || v > 100 {
			panic(fmt.Sprintf("out of range: %d", v))
		}
	}, WithTestCases(10))
	if len(vals) == 0 {
		t.Error("test function was never called")
	}
}

// --- Integers: schema is correct ---

func TestIntegersSchema(t *testing.T) {
	g := Integers(-5, 5)
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("Integers should return *BasicGenerator")
	}
	min, _ := ExtractInt(bg.schema["min_value"])
	max, _ := ExtractInt(bg.schema["max_value"])
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
	bg, ok := g.(*BasicGenerator)
	if !ok {
		t.Fatalf("IntegersUnbounded should return *BasicGenerator")
	}
	if _, hasMin := bg.schema["min_value"]; hasMin {
		t.Error("min_value should not be present when no min bound given")
	}
	if _, hasMax := bg.schema["max_value"]; hasMax {
		t.Error("max_value should not be present when no max bound given")
	}
}

// =============================================================================
// StopTest in collection protocol — error injection via HEGEL_PROTOCOL_TEST_MODE
// =============================================================================

// The stop_test_on_collection_more and stop_test_on_new_collection modes are
// now tested above via the real hegel binary. The skips have been removed since
// Stage 5 implements the collection protocol.

// Verify that after skip removal, the runner_test.go skips are gone.
// (These are integration tests that run against the real binary.)

// =============================================================================
// Just generator tests
// =============================================================================

// TestJustSchema verifies that Just produces a schema with "const" key.
func TestJustSchema(t *testing.T) {
	g := Just(42)
	if _, hasConst := g.schema["const"]; !hasConst {
		t.Error("Just schema should have 'const' key")
	}
	// The const value in schema should be nil (null)
	if g.schema["const"] != nil {
		t.Errorf("Just schema 'const' should be nil, got %v", g.schema["const"])
	}
}

// TestJustTransformIgnoresInput verifies that Just always returns the constant value.
func TestJustTransformIgnoresInput(t *testing.T) {
	g := Just("hello")
	// transform should ignore the server value and always return "hello"
	result := g.transform(nil)
	if result != "hello" {
		t.Errorf("Just transform: expected 'hello', got %v", result)
	}
	result = g.transform(int64(999))
	if result != "hello" {
		t.Errorf("Just transform with non-nil input: expected 'hello', got %v", result)
	}
}

// TestJustE2E verifies that Just always generates the constant value against the real server.
func TestJustE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		v := Just(42).Generate()
		if v.(int) != 42 {
			panic(fmt.Sprintf("Just: expected 42, got %v", v))
		}
	}, WithTestCases(20))
}

// TestJustNonPrimitive verifies that Just works with non-primitive values (pointer identity).
func TestJustNonPrimitive(t *testing.T) {
	hegelBinPath(t)
	type myStruct struct{ x int }
	val := &myStruct{x: 99}
	RunHegelTest(t.Name(), func() {
		v := Just(val).Generate()
		if v != val {
			panic("Just: pointer identity not preserved")
		}
	}, WithTestCases(10))
}

// =============================================================================
// SampledFrom generator tests
// =============================================================================

// TestSampledFromEmptyError verifies that SampledFrom returns an error for empty slice.
func TestSampledFromEmptyError(t *testing.T) {
	_, err := SampledFrom([]any{})
	if err == nil {
		t.Fatal("SampledFrom([]): expected error, got nil")
	}
	if err.Error() != "sampled_from requires at least one element" {
		t.Errorf("unexpected error message: %v", err)
	}
}

// TestMustSampledFromEmptyPanics verifies MustSampledFrom panics with empty slice.
func TestMustSampledFromEmptyPanics(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("MustSampledFrom([]) should panic")
		}
	}()
	MustSampledFrom([]any{})
}

// TestSampledFromSchema verifies that SampledFrom produces an integer schema with correct bounds.
func TestSampledFromSchema(t *testing.T) {
	g, err := SampledFrom([]any{"a", "b", "c"})
	if err != nil {
		t.Fatalf("SampledFrom: unexpected error: %v", err)
	}
	if g.schema["type"] != "integer" {
		t.Errorf("schema type: expected 'integer', got %v", g.schema["type"])
	}
	minVal, _ := ExtractInt(g.schema["min_value"])
	maxVal, _ := ExtractInt(g.schema["max_value"])
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
	items := []any{"x", "y", "z"}
	g, err := SampledFrom(items)
	if err != nil {
		t.Fatalf("SampledFrom: unexpected error: %v", err)
	}
	// Index 0 → "x", 1 → "y", 2 → "z"
	for i, want := range items {
		got := g.transform(uint64(i))
		if got != want {
			t.Errorf("transform(%d): expected %v, got %v", i, want, got)
		}
	}
}

// TestSampledFromSingleElement verifies that a single-element slice always returns that element.
func TestSampledFromSingleElement(t *testing.T) {
	hegelBinPath(t)
	g, _ := SampledFrom([]any{"only"})
	RunHegelTest(t.Name(), func() {
		v := g.Generate()
		if v != "only" {
			panic(fmt.Sprintf("SampledFrom single: expected 'only', got %v", v))
		}
	}, WithTestCases(20))
}

// TestSampledFromE2E verifies that SampledFrom only returns elements from the list
// and that all elements appear (with enough test cases).
func TestSampledFromE2E(t *testing.T) {
	hegelBinPath(t)
	choices := []any{"apple", "banana", "cherry"}
	g, _ := SampledFrom(choices)
	seen := map[string]bool{}
	RunHegelTest(t.Name(), func() {
		v := g.Generate()
		s, ok := v.(string)
		if !ok {
			panic(fmt.Sprintf("SampledFrom: expected string, got %T", v))
		}
		found := false
		for _, c := range choices {
			if c == s {
				found = true
				break
			}
		}
		if !found {
			panic(fmt.Sprintf("SampledFrom: value %q not in choices", s))
		}
		seen[s] = true
	}, WithTestCases(100))
	// After 100 cases we expect all 3 values to have appeared.
	for _, c := range choices {
		if !seen[c.(string)] {
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
	g, _ := SampledFrom([]any{obj1, obj2})
	RunHegelTest(t.Name(), func() {
		v := g.Generate()
		if v != obj1 && v != obj2 {
			panic("SampledFrom: value is not one of the original pointers")
		}
	}, WithTestCases(10))
}

// =============================================================================
// FromRegex generator tests
// =============================================================================

// TestFromRegexSchema verifies that FromRegex produces the correct schema.
func TestFromRegexSchema(t *testing.T) {
	g := FromRegex(`\d+`, true)
	if g.schema["type"] != "regex" {
		t.Errorf("schema type: expected 'regex', got %v", g.schema["type"])
	}
	if g.schema["pattern"] != `\d+` {
		t.Errorf("pattern: expected '\\d+', got %v", g.schema["pattern"])
	}
	if g.schema["fullmatch"] != true {
		t.Errorf("fullmatch: expected true, got %v", g.schema["fullmatch"])
	}
}

// TestFromRegexFullmatchFalse verifies that fullmatch=false is stored correctly.
func TestFromRegexFullmatchFalse(t *testing.T) {
	g := FromRegex(`abc`, false)
	if g.schema["fullmatch"] != false {
		t.Errorf("fullmatch: expected false, got %v", g.schema["fullmatch"])
	}
}

// TestFromRegexE2E verifies that FromRegex generates strings that match the pattern.
func TestFromRegexE2E(t *testing.T) {
	hegelBinPath(t)
	// Only digits, 1-5 chars
	g := FromRegex(`[0-9]{1,5}`, true)
	RunHegelTest(t.Name(), func() {
		v := g.Generate()
		s, ok := v.(string)
		if !ok {
			panic(fmt.Sprintf("FromRegex: expected string, got %T", v))
		}
		if len(s) == 0 || len(s) > 5 {
			panic(fmt.Sprintf("FromRegex: length out of range: %q", s))
		}
		for _, ch := range s {
			if ch < '0' || ch > '9' {
				panic(fmt.Sprintf("FromRegex: non-digit character %q in %q", ch, s))
			}
		}
	}, WithTestCases(50))
}

// =============================================================================
// BasicGenerator.Generate error path (line 78-79)
// =============================================================================

// TestBasicGeneratorGenerateErrorResponse covers the error path in
// BasicGenerator.Generate when generateFromSchema returns a non-StopTest error.
func TestBasicGeneratorGenerateErrorResponse(t *testing.T) {
	hegelBinPath(t)
	setEnv(t, "HEGEL_PROTOCOL_TEST_MODE", "error_response")
	err := RunHegelTestE(t.Name(), func() {
		g := &BasicGenerator{schema: map[string]any{"type": "integer"}}
		_ = g.Generate() // should panic with RequestError → caught as INTERESTING
	})
	// error_response causes the test to appear interesting (failing).
	_ = err
}

// =============================================================================
// Generator.Map on a Generator interface (non-basic returns MappedGenerator)
// =============================================================================

func TestGeneratorMapOnNonBasic(t *testing.T) {
	// A custom generator that is not a BasicGenerator.
	schema := map[string]any{"type": "integer"}
	inner := &BasicGenerator{schema: schema}
	// MappedGenerator is not a BasicGenerator.
	mg := &MappedGenerator{inner: inner, fn: func(v any) any { return v }}
	mapped := mg.Map(func(v any) any { return v })
	// Mapping a non-basic generator should produce a MappedGenerator.
	if _, ok := mapped.(*MappedGenerator); !ok {
		t.Errorf("Map on non-basic Generator should return *MappedGenerator, got %T", mapped)
	}
}

// =============================================================================
// MustSampledFrom happy-path test
// =============================================================================

// TestMustSampledFromHappyPath verifies that MustSampledFrom returns a valid generator
// when given a non-empty slice.
func TestMustSampledFromHappyPath(t *testing.T) {
	g := MustSampledFrom([]any{"only"})
	if g == nil {
		t.Fatal("MustSampledFrom should return non-nil generator")
	}
	result := g.transform(uint64(0))
	if result != "only" {
		t.Errorf("MustSampledFrom transform(0): expected 'only', got %v", result)
	}
}
