package libhegel

import (
	"testing"
)

// TestLibhegelEndToEnd exercises the full libhegel C API in a single test:
// create settings, start a run, drive 10 test cases, mark each complete, check
// the result. This is the lowest-level integration test — anything fancier
// goes through the testCaseRunner abstraction (added in a later task).
func TestLibhegelEndToEnd(t *testing.T) {
	ctx := NewContext()

	settings := ctx.SettingsNew()

	settings.TestCases(ctx, 10)
	settings.Derandomize(ctx, true)
	settings.Seed(ctx, 42, true)
	// Disable the database so the test is hermetic.
	settings.Database(ctx, "")

	run, err := settings.RunStart(ctx, 0, 0)
	if err != nil {
		t.Fatal(err)
	}

	cases := 0
	for {
		tc, err := run.NextTestCase(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if tc == nil {
			break
		}
		// Draw an integer in [0, 100] to sanity-check the protocol.
		value, err := tc.GenerateInteger(ctx, 0, 100)
		if err != nil {
			t.Fatalf("generate err=%v", err)
		}
		if value < 0 || value > 100 {
			t.Fatalf("generate returned out-of-range value %d", value)
		}
		if err := tc.MarkComplete(ctx, STATUS_VALID, ""); err != nil {
			t.Fatalf("mark_complete err=%v ", err)
		}
		cases++
	}
	if cases == 0 {
		t.Fatal("expected at least one test case from run_start")
	}

	result, err := run.RunResult(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status(ctx) != RUN_STATUS_PASSED {
		t.Errorf("expected passing run, got failure count %d", result.FailureCount(ctx))
	}
}
