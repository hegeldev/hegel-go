package libhegel

import (
	"testing"
)

// TestLibhegelEndToEnd exercises the full libhegel C API in a single test:
// create settings, start a run, drive 10 test cases, mark each complete, check
// the result. This is the lowest-level integration test — anything fancier
// goes through the testCaseRunner abstraction (added in a later task).
func TestLibhegelEndToEnd(t *testing.T) {
	lib, _ := GlobalHandle()

	settings := lib.SettingsNew()

	settings.TestCases(10)
	settings.Derandomize(true)
	settings.Seed(42, true)
	// Disable the database so the test is hermetic.
	settings.Database("")

	run, err := settings.RunStart()
	if err != nil {
		t.Fatal(err)
	}

	// Hand-encoded CBOR for {"type": "integer", "min_value": 0, "max_value": 100}.
	// Same as hegel-c/examples/echo.c.
	schema := []byte{
		0xA3,
		0x64, 't', 'y', 'p', 'e',
		0x67, 'i', 'n', 't', 'e', 'g', 'e', 'r',
		0x69, 'm', 'i', 'n', '_', 'v', 'a', 'l', 'u', 'e',
		0x00,
		0x69, 'm', 'a', 'x', '_', 'v', 'a', 'l', 'u', 'e',
		0x18, 0x64,
	}

	cases := 0
	for {
		tc, err := run.NextTestCase()
		if err != nil {
			t.Fatal(err)
		}
		if tc == nil {
			break
		}
		value, err := tc.Generate(schema)
		if err != nil {
			t.Fatalf("generate err=%d msg=%q", err, lib.lastErrorMessage())
		}
		// Decode the single-byte unsigned integer to sanity-check the protocol.
		if len(value) == 0 {
			t.Fatalf("generate returned empty value")
		}
		_ = value[0] // we don't enforce a value; just confirm it's readable.
		if rc := lib.markComplete(tc.ptr, STATUS_VALID, ""); rc != OK {
			t.Fatalf("mark_complete rc=%d msg=%q", rc, lib.lastErrorMessage())
		}
		cases++
	}
	if cases == 0 {
		t.Fatal("expected at least one test case from run_start")
	}

	result, err := run.RunResult()
	if err != nil {
		t.Fatal(err)
	}
	if lib.resultStatus(result.ptr) != RUN_STATUS_PASSED {
		t.Errorf("expected passing run, got failure count %d", lib.resultFailureCount(result.ptr))
	}
}
