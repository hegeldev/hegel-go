package libhegel

import (
	"strings"
	"testing"
)

// TestStubMissingReturnPanics covers the underflow guard in Stub's retval
// closure: popping more return values than were supplied panics with an
// index-bearing message.
func TestStubMissingReturnPanics(t *testing.T) {
	lib := Stub() // no returns supplied
	defer func() {
		r := recover()
		msg, ok := r.(string)
		if !ok {
			t.Fatalf("expected string panic, got %v", r)
		}
		if !strings.Contains(msg, "missing 1'th return value") {
			t.Errorf("unexpected panic message: %q", msg)
		}
	}()
	lib.SettingsNew() // pops the (absent) first return => panic
}

// TestStubUnwiredSetters exercises the settings setters that the runner does
// not (yet) drive — Verbosity, ReportMultipleFailures and Phases — directly
// against a Stub so their plumbing is covered.
func TestStubUnwiredSetters(t *testing.T) {
	lib := Stub(uintptr(1)) // settings_new handle
	s := lib.SettingsNew()
	s.Verbosity(VERBOSITY_VERBOSE)
	s.ReportMultipleFailures(true)
	s.Phases(PHASE_GENERATE)
}
