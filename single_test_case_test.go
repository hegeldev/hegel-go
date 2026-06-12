package hegel

import (
	"bytes"
	"fmt"
	"io"
	"strings"
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
	o := applyOpts([]Option{f.Option()})
	if !o.singleTestCase {
		t.Error("expected singleTestCase=true")
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
	o := applyOpts([]Option{f.Option()})
	if o.singleTestCase {
		t.Error("expected singleTestCase=false")
	}
}

// --- Integration tests: real hegel binary ---

func TestSingleTestCaseRunsExactlyOnce(t *testing.T) {
	t.Parallel()
	var count int32
	err := run(func(s TestCase) {
		atomic.AddInt32(&count, 1)
		_ = Draw[bool](s, Booleans())
	}, WithSingleTestCase())
	if err != nil {
		t.Fatalf("runHegel: %v", err)
	}
	if got := atomic.LoadInt32(&count); got != 1 {
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
		WithDatabase(DatabaseDisabled()),
		SuppressHealthCheck(FilterTooMuch),
	)
	if err != nil {
		t.Fatalf("runHegel: %v", err)
	}
}

// --- Stateful unbounded steps ---

type unboundedMachine struct{ n int }

func (m *unboundedMachine) RuleStep(_ TestCase) { m.n++ }
func (m *unboundedMachine) InvariantBounded(_ TestCase) {
	// Threshold well above statefulMaxSteps (50), so a successful failure
	// proves the cap was lifted in single-test-case mode.
	if m.n >= 200 {
		panic(fmt.Sprintf("counter reached %d", m.n))
	}
}

func TestSingleTestCaseStatefulRunsBeyondDefaultCap(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	m := &unboundedMachine{}
	err := run(func(s TestCase) {
		RunStateful(s, m)
	}, WithSingleTestCase(), withOutput(&buf))
	if err == nil {
		t.Fatalf("expected invariant to fail; counter reached %d", m.n)
	}
	if m.n <= statefulMaxSteps {
		t.Errorf("expected >%d steps, got %d (cap not lifted?)", statefulMaxSteps, m.n)
	}
	if !strings.Contains(buf.String(), "counter reached") {
		t.Errorf("expected invariant panic in output, got: %q", buf.String())
	}
}

// --- Workload --single-test-case ---

func TestWorkloadSingleTestCaseFlag(t *testing.T) {
	t.Parallel()
	var count int32
	err := workload(
		[]string{"prog", "--single-test-case"},
		io.Discard,
		io.Discard,
		func(s TestCase) {
			atomic.AddInt32(&count, 1)
			_ = Draw[bool](s, Booleans())
		},
		nil,
	)
	if err != nil {
		t.Fatalf("workload: %v", err)
	}
	if got := atomic.LoadInt32(&count); got != 1 {
		t.Errorf("expected exactly 1 test case, got %d", got)
	}
}
