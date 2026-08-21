package hegel

import (
	"io"
	"sync/atomic"
	"testing"
)

// --- Unit tests: --single-test-case flag wiring ---

func TestSingleTestCaseFlagSetOption(t *testing.T) {
	t.Parallel()
	f := &singleTestCaseFlag{}
	if !f.IsBoolFlag() {
		t.Error("IsBoolFlag should be true")
	}
	if err := f.Set("true"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	if f.String() != "true" {
		t.Errorf("String=%q want %q", f.String(), "true")
	}
}

func TestSingleTestCaseFlagBadValue(t *testing.T) {
	t.Parallel()
	f := &singleTestCaseFlag{}
	if err := f.Set("maybe"); err == nil {
		t.Fatal("expected error for non-bool value")
	}
}

func TestSingleTestCaseFlagFalseIsNoop(t *testing.T) {
	t.Parallel()
	f := &singleTestCaseFlag{}
	if err := f.Set("false"); err != nil {
		t.Fatalf("Set: %v", err)
	}
	f.Option()(&runOptions{})
}

// --- Integration tests: real hegel binary ---

func TestSingleTestCaseRunsExactlyOnce(t *testing.T) {
	t.Parallel()
	var count atomic.Int32
	err := run(func(s TestCase) {
		count.Add(1)
		_ = Draw[bool](s, Booleans())
	}, WithSingleTestCase())
	if err != nil {
		t.Fatalf("runHegel: %v", err)
	}
	if got := count.Load(); got != 1 {
		t.Errorf("expected exactly 1 test case, got %d", got)
	}
}

func TestSingleTestCasePermissiveOptions(t *testing.T) {
	t.Parallel()
	// Combining WithSingleTestCase with other options is permissive — the
	// extra options simply don't go on the wire.
	err := run(func(s TestCase) {
		_ = Draw[bool](s, Booleans())
	},
		WithSingleTestCase(),
		WithTestCases(50),
		WithDerandomize(true),
		WithDatabase(""),
		SuppressHealthCheck(FilterTooMuch),
	)
	if err != nil {
		t.Fatalf("runHegel: %v", err)
	}
}

// --- Stateful unbounded steps ---

type unboundedMachine struct{ n int }

func (m *unboundedMachine) RuleStep(_ TestCase) { m.n++ }
func (m *unboundedMachine) InvariantBounded(tc TestCase) {
	// Threshold well above statefulMaxSteps (50), so a successful failure
	// proves the cap was lifted in single-test-case mode.
	if m.n >= 200 {
		tc.FailNow()
	}
}

func TestSingleTestCaseStatefulRunsBeyondDefaultCap(t *testing.T) {
	t.Parallel()
	m := &unboundedMachine{}
	err := run(func(s TestCase) {
		RunStateful(s, m)
	}, WithSingleTestCase())
	if err == nil {
		t.Fatalf("expected invariant to fail; counter reached %d", m.n)
	}
	if m.n <= statefulMaxSteps {
		t.Errorf("expected >%d steps, got %d (cap not lifted?)", statefulMaxSteps, m.n)
	}
}

// --- Workload --single-test-case ---

func TestWorkloadSingleTestCaseFlag(t *testing.T) {
	t.Parallel()
	var count atomic.Int32
	err := workload(
		[]string{"prog", "--single-test-case"},
		io.Discard,
		io.Discard,
		func(s TestCase) {
			count.Add(1)
			_ = Draw[bool](s, Booleans())
		},
		nil,
	)
	if err != nil {
		t.Fatalf("workload: %v", err)
	}
	if got := count.Load(); got != 1 {
		t.Errorf("expected exactly 1 test case, got %d", got)
	}
}
