package libhegel

import (
	"strings"
	"testing"
)

// TestOutputCallbackReceivesEngineOutput drives a verbose run with a non-nil
// [OutputFn] and asserts the engine forwards its log lines through the callback.
// This is the only mode in which libhegel invokes the callback (single-test-case
// and replay modes emit nothing), so it covers both outputCallback and
// freeOutputFn's AddCleanup arm end-to-end.
func TestOutputCallbackReceivesEngineOutput(t *testing.T) {
	ctx := NewContext()
	s := ctx.SettingsNew()
	s.TestCases(ctx, 3)
	s.Database(ctx, "")
	s.Verbosity(ctx, VERBOSITY_VERBOSE)

	var sb strings.Builder
	run, err := s.RunStart(ctx, &sb)
	if err != nil {
		t.Fatalf("RunStart: %v", err)
	}
	for {
		tc, err := run.NextTestCase(ctx)
		if err != nil {
			t.Fatalf("NextTestCase: %v", err)
		}
		if tc == nil {
			break
		}
		if _, err := tc.GenerateInteger(ctx, 0, 100); err != nil {
			t.Fatalf("GenerateInteger: %v", err)
		}
		if err := tc.MarkComplete(ctx, STATUS_VALID, ""); err != nil {
			t.Fatalf("MarkComplete: %v", err)
		}
	}
	if _, err := run.RunResult(ctx); err != nil {
		t.Fatalf("RunResult: %v", err)
	}

	if !strings.Contains(sb.String(), "phase") {
		t.Errorf("expected verbose phase output via callback, got %q", sb.String())
	}
}

// TestFreeOutputFnFailedCall covers freeOutputFn's ptr==nil arm: when a non-nil
// output writer is supplied but the underlying C call fails (nil result
// pointer), the handle must be deleted immediately and the call must return the
// error without panicking on runtime.AddCleanup(nil, ...).
func TestFreeOutputFnFailedCall(t *testing.T) {
	lib := Stub(t, uintptr(0), E_INTERNAL, "boom") // run_start: placeholder handle, error, diagnostic
	s := &Settings{syms: lib.syms, raw: 1}

	run, err := s.RunStart(lib, &strings.Builder{})
	if err == nil || run != nil {
		t.Fatalf("expected error and nil run, got run=%v err=%v", run, err)
	}
	if !strings.Contains(err.Error(), "boom") {
		t.Errorf("expected diagnostic in error, got %q", err)
	}
}
