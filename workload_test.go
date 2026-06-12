package hegel

import (
	"bytes"
	"io"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
)

// applyOpts builds a runOptions by applying all opts in order.
func applyOpts(opts []Option) runOptions {
	var o runOptions
	for _, opt := range opts {
		opt(&o)
	}
	return o
}

// captureWorkloadExit replaces workloadExit with one that records the exit
// code into the returned pointer, and registers a t.Cleanup to restore it.
func captureWorkloadExit(t *testing.T) *int {
	t.Helper()
	var code int
	orig := workloadExit
	workloadExit = func(c int) { code = c }
	t.Cleanup(func() { workloadExit = orig })
	return &code
}

// withArgs replaces os.Args for the duration of the test.
func withArgs(t *testing.T, args []string) {
	t.Helper()
	orig := os.Args
	os.Args = args
	t.Cleanup(func() { os.Args = orig })
}

// --- Per-flag round-trip tests ---

func TestTestCasesFlagSetOption(t *testing.T) {
	t.Parallel()
	f := &testCasesFlag{}
	if err := f.Set("7"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if f.String() != "7" {
		t.Errorf("String=%q want %q", f.String(), "7")
	}
	o := applyOpts([]Option{f.Option()})
	if o.testCases != 7 {
		t.Errorf("testCases=%d want 7", o.testCases)
	}
}

func TestTestCasesFlagBadValue(t *testing.T) {
	t.Parallel()
	f := &testCasesFlag{}
	if err := f.Set("not-an-int"); err == nil {
		t.Fatal("expected error for non-int value")
	}
}

func TestDerandomizeFlagSetOption(t *testing.T) {
	t.Parallel()
	f := &derandomizeFlag{}
	if !f.IsBoolFlag() {
		t.Error("IsBoolFlag should be true")
	}
	if err := f.Set("true"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if f.String() != "true" {
		t.Errorf("String=%q want %q", f.String(), "true")
	}
	o := applyOpts([]Option{f.Option()})
	if !o.derandomize {
		t.Error("expected derandomize=true")
	}
}

func TestDerandomizeFlagBadValue(t *testing.T) {
	t.Parallel()
	f := &derandomizeFlag{}
	if err := f.Set("maybe"); err == nil {
		t.Fatal("expected error for non-bool value")
	}
}

func TestDatabaseFlagPath(t *testing.T) {
	t.Parallel()
	f := &databaseFlag{}
	if err := f.Set("/tmp/db"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if f.String() != "/tmp/db" {
		t.Errorf("String=%q want %q", f.String(), "/tmp/db")
	}
	o := applyOpts([]Option{f.Option()})
	want := Database("/tmp/db")
	if o.database != want {
		t.Errorf("database=%+v want %+v", o.database, want)
	}
}

func TestDatabaseFlagDisabledViaEmpty(t *testing.T) {
	t.Parallel()
	f := &databaseFlag{}
	if err := f.Set(""); err != nil {
		t.Fatalf("Set: %v", err)
	}
	o := applyOpts([]Option{f.Option()})
	if o.database != DatabaseDisabled() {
		t.Errorf("database=%+v want DatabaseDisabled()", o.database)
	}
}

func TestSuppressHealthCheckFlagRepeated(t *testing.T) {
	t.Parallel()
	f := &suppressHealthCheckFlag{}
	if err := f.Set("filter_too_much"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if err := f.Set("too_slow"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if f.String() != "filter_too_much,too_slow" {
		t.Errorf("String=%q", f.String())
	}
	o := applyOpts([]Option{f.Option()})
	want := []HealthCheck{FilterTooMuch, TooSlow}
	if !reflect.DeepEqual(o.suppressHealthCheck, want) {
		t.Errorf("suppressHealthCheck=%v want %v", o.suppressHealthCheck, want)
	}
}

func TestSuppressHealthCheckFlagUnknown(t *testing.T) {
	t.Parallel()
	f := &suppressHealthCheckFlag{}
	err := f.Set("bogus")
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("expected error mentioning %q, got %v", "bogus", err)
	}
}

func TestParseHealthCheckRoundTrip(t *testing.T) {
	t.Parallel()
	for _, hc := range AllHealthChecks() {
		got, err := parseHealthCheck(hc.String())
		if err != nil {
			t.Errorf("parseHealthCheck(%q): %v", hc.String(), err)
			continue
		}
		if got != hc {
			t.Errorf("parseHealthCheck(%q)=%v want %v", hc.String(), got, hc)
		}
	}
}

// --- workload() (lowercase) ---

func TestWorkloadParsesFlags(t *testing.T) {
	t.Parallel()
	err := workload(
		[]string{
			"prog",
			"--test-cases=3",
			"--derandomize",
			"--database=/tmp/x",
			"--suppress-health-check=filter_too_much",
		},
		io.Discard,
		io.Discard,
		func(TestCase) {}, nil,
	)
	if err != nil {
		t.Fatalf("workload: %v", err)
	}
}

func TestWorkloadHelpReturnsNil(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := workload([]string{"myprog", "--help"}, io.Discard, &buf, func(TestCase) {}, nil)
	if err != nil {
		t.Errorf("expected nil error on --help, got %v", err)
	}
	if !strings.Contains(buf.String(), "Usage of myprog") {
		t.Errorf("expected help to mention %q, got %q", "Usage of myprog", buf.String())
	}
	if !strings.Contains(buf.String(), "test-cases") {
		t.Errorf("expected help to list --test-cases, got %q", buf.String())
	}
}

func TestWorkloadBadFlagReturnsError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	err := workload([]string{"prog", "--bogus"}, io.Discard, &buf, func(TestCase) {}, nil)
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	// flag.ContinueOnError prints the error and usage to fs.Output().
	if !strings.Contains(buf.String(), "bogus") {
		t.Errorf("expected stderr to mention %q, got %q", "bogus", buf.String())
	}
}

func TestWorkloadBadHealthCheckReturnsError(t *testing.T) {
	t.Parallel()
	err := workload(
		[]string{"prog", "--suppress-health-check=nonsense"},
		io.Discard,
		io.Discard,
		func(TestCase) {}, nil,
	)
	if err == nil || !strings.Contains(err.Error(), "nonsense") {
		t.Errorf("expected error mentioning %q, got %v", "nonsense", err)
	}
}

// --- Layering: Workload default < opts < CLI ---

func TestWorkloadLayeringCLIOverrides(t *testing.T) {
	t.Parallel()
	tcFlag := &testCasesFlag{}
	if err := tcFlag.Set("7"); err != nil {
		t.Fatal(err)
	}
	userOpts := []Option{WithTestCases(99)}
	all := []Option{WithTestCases(math.MaxInt)}
	all = append(all, userOpts...)
	all = append(all, tcFlag.Option())
	o := applyOpts(all)
	if o.testCases != 7 {
		t.Errorf("CLI should win: testCases=%d want 7", o.testCases)
	}
}

func TestWorkloadLayeringOptsOverridesDefault(t *testing.T) {
	t.Parallel()
	userOpts := []Option{WithTestCases(13)}
	all := append([]Option{WithTestCases(math.MaxInt)}, userOpts...)
	o := applyOpts(all)
	if o.testCases != 13 {
		t.Errorf("opts should beat default: testCases=%d want 13", o.testCases)
	}
}

func TestWorkloadLayeringDefaultAlone(t *testing.T) {
	t.Parallel()
	all := []Option{WithTestCases(math.MaxInt)}
	o := applyOpts(all)
	if o.testCases != math.MaxInt {
		t.Errorf("default: testCases=%d want MaxInt", o.testCases)
	}
}

// --- Workload() (uppercase): success and exit paths ---

func TestWorkloadPublicSuccess(t *testing.T) {
	withArgs(t, []string{"prog", "--single-test-case"})
	code := captureWorkloadExit(t)

	Workload(func(tc TestCase) {
		_ = Draw[bool](tc, Booleans())
	})
	if *code != 0 {
		t.Errorf("expected exit code 0 (workloadExit not called); got %d", *code)
	}
}

func TestWorkloadPublicExitsOnFlagError(t *testing.T) {
	withArgs(t, []string{"prog", "--bogus"})
	code := captureWorkloadExit(t)

	Workload(func(TestCase) {})
	if *code != 1 {
		t.Errorf("expected exit code 1, got %d", *code)
	}
}
