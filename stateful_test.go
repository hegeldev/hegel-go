package hegel

import (
	"errors"
	"fmt"
	"io"
	"runtime"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"hegel.dev/go/hegel/internal/libhegel"
)

type goodCounter struct{ n int }

func (c *goodCounter) RuleIncrement(_ TestCase) { c.n++ }
func (c *goodCounter) RuleDecrement(_ TestCase) { c.n-- }
func (c *goodCounter) InvariantSensible(_ TestCase) {
	if c.n < -10000 || c.n > 10000 {
		panic("counter out of sensible range")
	}
}

type singleRuleMachine struct{ n int }

func (m *singleRuleMachine) RuleStep(_ TestCase) { m.n++ }

type outputRuleMachine struct{}

func (*outputRuleMachine) RuleStep(TestCase) {}

type helperMachine struct{ n int }

func (m *helperMachine) RuleBump(_ TestCase) { m.n++ }
func (m *helperMachine) Helper() int         { return m.n } //nolint:unused

type noRulesMachine struct{}

func (noRulesMachine) InvariantTrue(_ TestCase) {}

type badRuleSig struct{}

func (badRuleSig) RuleBad(_ TestCase, _ string) {}

type badInvariantSig struct{}

func (badInvariantSig) RuleOK(_ TestCase)     {}
func (badInvariantSig) InvariantBad(_ string) {}

type strayTestCaseMachine struct{}

func (strayTestCaseMachine) RuleOK(_ TestCase)  {}
func (strayTestCaseMachine) DoStuff(_ TestCase) {}

// gatedMachine.RuleGated rejects via Assume until RuleOpen has run, so
// the test exercises that an Assume rejection retries with a different
// rule rather than killing the test case.
type gatedMachine struct {
	opened    bool
	gateCount int
}

func (m *gatedMachine) RuleOpen(_ TestCase) { m.opened = true }

func (m *gatedMachine) RuleGated(tc TestCase) {
	tc.Assume(m.opened)
	m.gateCount++
}

// ruleFailer.RuleFail marks the case failed via Fail (which sets INTERESTING
// without panicking), so callRule's deferred block observes a non-VALID status
// after the rule returns and aborts via abort(nil).
type ruleFailer struct{}

func (ruleFailer) RuleFail(tc TestCase) { tc.Fail() }

type invariantViolator struct{ n int }

func (m *invariantViolator) RuleStep(_ TestCase) { m.n++ }
func (m *invariantViolator) InvariantSmall(tc TestCase) {
	if m.n >= 3 {
		tc.FailNow()
	}
}

type groupedMachine struct{}

func (*groupedMachine) RuleAlpha(TestCase)   {}
func (*groupedMachine) RuleBeta(TestCase)    {}
func (*groupedMachine) RuleDelta(TestCase)   {}
func (*groupedMachine) RuleGamma(TestCase)   {}
func (*groupedMachine) InvariantOK(TestCase) {}

type rejectingMachine struct{}

func (*rejectingMachine) RuleReject(tc TestCase) { tc.Assume(false) }

type concurrentExecutionProbe struct {
	expectedWorkers        int64
	active                 atomic.Int64
	entered                atomic.Int64
	barrierTimedOut        atomic.Bool
	allEntered             chan struct{}
	invariantCalls         int
	invariantWhileRuleRuns bool
}

func (m *concurrentExecutionProbe) RuleOverlap(_ TestCase) {
	m.active.Add(1)
	defer m.active.Add(-1)
	if m.entered.Add(1) == m.expectedWorkers {
		close(m.allEntered)
	}
	select {
	case <-m.allEntered:
	case <-time.After(2 * time.Second):
		m.barrierTimedOut.Store(true)
	}
}

func (m *concurrentExecutionProbe) InvariantAtJoin(_ TestCase) {
	m.invariantCalls++
	if m.active.Load() != 0 {
		m.invariantWhileRuleRuns = true
	}
}

// concurrentTestCase replaces only the state-machine protocol. Embedding the
// interface keeps unrelated TestCase operations unavailable to this focused
// orchestration test.
type concurrentTestCase struct {
	TestCase
	shared   *concurrentTestCaseShared
	drewRule bool
	out      io.Writer
}

type concurrentTestCaseShared struct {
	selectedConcurrency     int64
	selectedGroup           libhegel.StateMachineGroup
	requestedMaxConcurrency int
	ruleGroups              []int64
	cloneCount              int64
	cloneErr                error
	ranGroup                bool
	spanStarts              atomic.Int64
	spanStops               atomic.Int64
	spanLabel               atomic.Int64
	spanDiscarded           atomic.Bool
	rejectedRules           int
	nextRuleErrors          map[int64]error
	reverseWorkerOrder      chan struct{}
}

func (tc *concurrentTestCase) Note(message string) {
	if tc.out != nil {
		fmt.Fprintln(tc.out, message)
	}
}

func (tc *concurrentTestCase) log(format string, args ...any) {
	if tc.out != nil {
		fmt.Fprintln(tc.out, fmt.Sprintf(format, args...))
	}
}

func (tc *concurrentTestCase) output() io.Writer { return tc.out }

func (tc *concurrentTestCase) setOutput(out io.Writer) { tc.out = out }

func (tc *concurrentTestCase) Assume(condition bool) {
	if !condition {
		panic(libhegel.E_ASSUME)
	}
}

func (tc *concurrentTestCase) abort(err error) { panic(err) }

func (tc *concurrentTestCase) invoke(fn testBody) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			if recovered == libhegel.E_ASSUME {
				err = libhegel.E_ASSUME
				return
			}
			panic(recovered)
		}
	}()
	fn(tc)
	return nil
}

func (tc *concurrentTestCase) startSpan(label libhegel.Label) error {
	tc.shared.spanStarts.Add(1)
	tc.shared.spanLabel.Store(int64(label))
	return nil
}

func (tc *concurrentTestCase) stopSpan(discard bool) error {
	tc.shared.spanStops.Add(1)
	if discard {
		tc.shared.spanDiscarded.Store(true)
	}
	return nil
}

func (tc *concurrentTestCase) clone() (TestCase, error) {
	tc.shared.cloneCount++
	if tc.shared.cloneErr != nil {
		return nil, tc.shared.cloneErr
	}
	return &concurrentTestCase{shared: tc.shared, out: tc.out}, nil
}

func (tc *concurrentTestCase) stateMachineNew(_ []string, ruleGroups []int64, _ []string, maxConcurrency int) (*libhegel.StateMachine, int64, error) {
	tc.shared.requestedMaxConcurrency = maxConcurrency
	tc.shared.ruleGroups = slices.Clone(ruleGroups)
	return new(libhegel.StateMachine), tc.shared.selectedConcurrency, nil
}

func (tc *concurrentTestCase) stateMachineNextGroup(*libhegel.StateMachine) (libhegel.StateMachineGroup, error) {
	if !tc.shared.ranGroup {
		tc.shared.ranGroup = true
		return tc.shared.selectedGroup, nil
	}
	return libhegel.StateMachineDone, nil
}

func (tc *concurrentTestCase) stateMachineNextRule(_ *libhegel.StateMachine, worker int64) (int64, error) {
	if err := tc.shared.nextRuleErrors[worker]; err != nil {
		return 0, err
	}
	if tc.drewRule {
		return libhegel.StateMachineDone, nil
	}
	if tc.shared.reverseWorkerOrder != nil {
		if worker == 0 {
			<-tc.shared.reverseWorkerOrder
		} else {
			close(tc.shared.reverseWorkerOrder)
		}
	}
	tc.drewRule = true
	return 0, nil
}

func (tc *concurrentTestCase) stateMachineRuleRejected(*libhegel.StateMachine, int64) error {
	tc.shared.rejectedRules++
	return nil
}

func TestNewStateMachineNilPointer(t *testing.T) {
	t.Parallel()
	_, err := newStateMachine((*goodCounter)(nil))
	assertErrorContains(t, "must not be nil", err)
}

func TestNewStateMachineValidationErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name        string
		newMachine  func() (*stateMachine, error)
		wantSubstrs []string
	}{
		{
			name:        "NoRules",
			newMachine:  func() (*stateMachine, error) { return newStateMachine(&noRulesMachine{}) },
			wantSubstrs: []string{"no rules"},
		},
		{
			name:        "BadRuleSignature",
			newMachine:  func() (*stateMachine, error) { return newStateMachine(&badRuleSig{}) },
			wantSubstrs: []string{"RuleBad", "func(TestCase)"},
		},
		{
			name:        "BadInvariantSignature",
			newMachine:  func() (*stateMachine, error) { return newStateMachine(&badInvariantSig{}) },
			wantSubstrs: []string{"InvariantBad"},
		},
		{
			name:        "StrayTestCaseMethod",
			newMachine:  func() (*stateMachine, error) { return newStateMachine(&strayTestCaseMachine{}) },
			wantSubstrs: []string{"DoStuff", "Rule or Invariant"},
		},
		{
			name:        "NonStructPointer",
			newMachine:  func() (*stateMachine, error) { n := 0; return newStateMachine(&n) },
			wantSubstrs: []string{"no rules"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := tc.newMachine()
			for _, s := range tc.wantSubstrs {
				assertErrorContains(t, s, err)
			}
		})
	}
}

func TestNewStateMachineHelperIgnored(t *testing.T) {
	t.Parallel()
	sm, err := newStateMachine(&helperMachine{})
	if err != nil {
		t.Fatalf("helper-only method should be ignored, got: %v", err)
	}
	if len(sm.rules) != 1 {
		t.Errorf("expected 1 rule, got %d", len(sm.rules))
	}
	if len(sm.invariants) != 0 {
		t.Errorf("expected 0 invariants, got %d", len(sm.invariants))
	}
}

func TestNewStateMachineDiscoversRulesAndInvariants(t *testing.T) {
	t.Parallel()
	sm, err := newStateMachine(&goodCounter{})
	if err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if len(sm.rules) != 2 {
		t.Errorf("expected 2 rules, got %d", len(sm.rules))
	}
	if len(sm.invariants) != 1 {
		t.Errorf("expected 1 invariant, got %d", len(sm.invariants))
	}
}

func TestNewStateMachineConcurrency(t *testing.T) {
	t.Parallel()

	t.Run("Default", func(t *testing.T) {
		sm, err := newStateMachine(&goodCounter{})
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if sm.maxConcurrency != 1 {
			t.Errorf("expected maximum concurrency 1, got %d", sm.maxConcurrency)
		}
	})

	t.Run("Explicit", func(t *testing.T) {
		sm, err := newStateMachine(&goodCounter{}, WithBoundedConcurrency(3))
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if sm.maxConcurrency != 3 {
			t.Errorf("expected maximum concurrency 3, got %d", sm.maxConcurrency)
		}
	})

	t.Run("GOMAXPROCS", func(t *testing.T) {
		sm, err := newStateMachine(&goodCounter{}, WithConcurrency())
		if err != nil {
			t.Fatalf("unexpected validation error: %v", err)
		}
		if want := runtime.GOMAXPROCS(0); sm.maxConcurrency != want {
			t.Errorf("expected maximum concurrency %d, got %d", want, sm.maxConcurrency)
		}
	})

	for _, n := range []int{0, -1} {
		t.Run(fmt.Sprintf("Invalid/%d", n), func(t *testing.T) {
			_, err := newStateMachine(&goodCounter{}, WithBoundedConcurrency(n))
			assertErrorContains(t, "must be positive", err)
		})
	}
}

func TestNewStateMachineRuleGroups(t *testing.T) {
	t.Parallel()

	sm, err := newStateMachine(
		&groupedMachine{},
		WithRuleGroup("alpha", "RuleAlpha"),
		WithRuleGroup("alpha", "RuleGamma"),
		WithRuleGroup("beta", "RuleBeta"),
	)
	if err != nil {
		t.Fatalf("newStateMachine: %v", err)
	}

	// Reflection discovers methods alphabetically. Explicit groups are numbered
	// in option order; the unmentioned RuleDelta remains in default group zero.
	if want := []int64{1, 2, 0, 1}; !slices.Equal(sm.ruleGroups, want) {
		t.Errorf("rule groups = %v, want %v", sm.ruleGroups, want)
	}
}

func TestNewStateMachineRuleGroupValidation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		opts []StateMachineOption
		want string
	}{
		{name: "Empty", opts: []StateMachineOption{WithRuleGroup("empty")}, want: "group 1 is empty"},
		{name: "Unknown", opts: []StateMachineOption{WithRuleGroup("missing", "RuleMissing")}, want: `unknown rule "RuleMissing"`},
		{name: "Invariant", opts: []StateMachineOption{WithRuleGroup("invariant", "InvariantOK")}, want: `names invariant "InvariantOK"`},
		{name: "Duplicate", opts: []StateMachineOption{WithRuleGroup("alpha", "RuleAlpha"), WithRuleGroup("beta", "RuleAlpha")}, want: `belongs to both group 1 and group 2`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := newStateMachine(&groupedMachine{}, tt.opts...)
			assertErrorContains(t, tt.want, err)
		})
	}
}

func TestStateMachineRunsSequentiallyByDefault(t *testing.T) {
	t.Parallel()

	probe := &concurrentExecutionProbe{
		expectedWorkers: 1,
		allEntered:      make(chan struct{}),
	}
	sm, err := newStateMachine(probe)
	if err != nil {
		t.Fatalf("newStateMachine: %v", err)
	}
	shared := &concurrentTestCaseShared{selectedConcurrency: 1}
	sm.Run(&concurrentTestCase{shared: shared})

	if shared.requestedMaxConcurrency != 1 {
		t.Errorf("requested maximum concurrency = %d, want 1", shared.requestedMaxConcurrency)
	}
	if shared.cloneCount != 1 {
		t.Errorf("worker clones = %d, want 1", shared.cloneCount)
	}
	if got := probe.entered.Load(); got != 1 {
		t.Errorf("workers entering rule = %d, want 1", got)
	}
	if starts, stops := shared.spanStarts.Load(), shared.spanStops.Load(); starts != 0 || stops != 0 {
		t.Errorf("stateful spans = %d starts, %d stops; want none", starts, stops)
	}
}

func TestStateMachineAbortsWhenWorkerCloneFails(t *testing.T) {
	t.Parallel()

	want := errors.New("clone failed")
	sm, err := newStateMachine(&goodCounter{})
	if err != nil {
		t.Fatalf("newStateMachine: %v", err)
	}
	shared := &concurrentTestCaseShared{selectedConcurrency: 1, cloneErr: want}

	defer expectErrorPanic(t, want)
	sm.Run(&concurrentTestCase{shared: shared})
}

func TestStateMachineReportsRejectedRule(t *testing.T) {
	t.Parallel()

	sm, err := newStateMachine(&rejectingMachine{})
	if err != nil {
		t.Fatalf("newStateMachine: %v", err)
	}
	var out strings.Builder
	shared := &concurrentTestCaseShared{selectedConcurrency: 1}
	sm.Run(&concurrentTestCase{shared: shared, out: &out})

	if shared.rejectedRules != 1 {
		t.Errorf("rejected rules = %d, want 1", shared.rejectedRules)
	}
	if !strings.Contains(out.String(), "Rule stopped early due to violated assumption.\n") {
		t.Errorf("output did not report the rejected rule:\n%s", out.String())
	}
}

func TestStateMachineRunsEngineSelectedWorkersConcurrently(t *testing.T) {
	t.Parallel()

	const selectedConcurrency = int64(2)
	probe := &concurrentExecutionProbe{
		expectedWorkers: selectedConcurrency,
		allEntered:      make(chan struct{}),
	}
	sm, err := newStateMachine(probe, WithBoundedConcurrency(4), WithRuleGroup("overlap", "RuleOverlap"))
	if err != nil {
		t.Fatalf("newStateMachine: %v", err)
	}
	shared := &concurrentTestCaseShared{selectedConcurrency: selectedConcurrency, selectedGroup: 1}
	sm.Run(&concurrentTestCase{shared: shared})

	if shared.requestedMaxConcurrency != 4 {
		t.Errorf("requested maximum concurrency = %d, want 4", shared.requestedMaxConcurrency)
	}
	if want := []int64{1}; !slices.Equal(shared.ruleGroups, want) {
		t.Errorf("rule groups passed to engine = %v, want %v", shared.ruleGroups, want)
	}
	if shared.cloneCount != selectedConcurrency {
		t.Errorf("worker clones = %d, want engine-selected count %d", shared.cloneCount, selectedConcurrency)
	}
	if got := probe.entered.Load(); got != selectedConcurrency {
		t.Errorf("workers entering rule = %d, want %d", got, selectedConcurrency)
	}
	if probe.barrierTimedOut.Load() {
		t.Error("a worker did not enter the rule while the other worker was active")
	}
	if probe.invariantWhileRuleRuns {
		t.Error("invariant ran before all workers reached the join point")
	}
	if got := probe.invariantCalls; got != 2 {
		t.Errorf("invariant calls = %d, want initial and post-round checks", got)
	}
	if starts, stops := shared.spanStarts.Load(), shared.spanStops.Load(); starts != 0 || stops != 0 {
		t.Errorf("stateful spans = %d starts, %d stops; want none", starts, stops)
	}
}

func TestStateMachineGroupsRoundOutputByWorker(t *testing.T) {
	t.Parallel()

	sm, err := newStateMachine(&outputRuleMachine{}, WithBoundedConcurrency(2))
	if err != nil {
		t.Fatalf("newStateMachine: %v", err)
	}
	var out strings.Builder
	shared := &concurrentTestCaseShared{
		selectedConcurrency: 2,
		reverseWorkerOrder:  make(chan struct{}),
	}
	sm.Run(&concurrentTestCase{shared: shared, out: &out})
	worker0 := strings.Index(out.String(), "[worker 0 +")
	worker1 := strings.Index(out.String(), "[worker 1 +")
	if worker0 == -1 || worker1 == -1 || worker0 > worker1 {
		t.Fatalf("worker output is not grouped in index order:\n%s", out.String())
	}
}

func TestStateMachineOmitsWorkerMetadataAtConcurrencyOne(t *testing.T) {
	t.Parallel()

	sm, err := newStateMachine(&outputRuleMachine{})
	if err != nil {
		t.Fatalf("newStateMachine: %v", err)
	}
	var out strings.Builder
	shared := &concurrentTestCaseShared{selectedConcurrency: 1}
	sm.Run(&concurrentTestCase{shared: shared, out: &out})
	if got := out.String(); strings.Contains(got, "Concurrency level:") || strings.Contains(got, "[worker") {
		t.Fatalf("sequential output contains concurrency metadata:\n%s", got)
	}
}

func TestWorkerOutputPrefixesEachLine(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		writes []string
		lines  []string
	}{
		{"multiple and fragmented lines", []string{"first\nsecond", " half\n"}, []string{"first", "second half"}},
		{"fragmented line", []string{"fir", "st\n"}, []string{"first"}},
		{"separate newline write", []string{"first", "\n"}, []string{"first"}},
		{"empty write", []string{""}, nil},
		{"blank line", []string{"\n"}, []string{""}},
		{"consecutive blank lines", []string{"\n\n"}, []string{"", ""}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var out strings.Builder
			w := &workerOutput{worker: 3, start: time.Now(), out: &out, lineStart: true}
			for _, input := range test.writes {
				n, err := w.Write([]byte(input))
				if err != nil || n != len(input) {
					t.Fatalf("Write(%q) = (%d, %v), want (%d, nil)", input, n, err, len(input))
				}
			}

			got := out.String()
			if len(test.lines) == 0 {
				if got != "" {
					t.Fatalf("worker output = %q, want none", got)
				}
				return
			}
			physicalLines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
			if len(physicalLines) != len(test.lines) {
				t.Fatalf("worker output = %q, want %d lines", got, len(test.lines))
			}
			for i, line := range physicalLines {
				if !strings.HasPrefix(line, "[worker 3 +") {
					t.Fatalf("line %d = %q, want worker prefix", i, line)
				}
				_, content, ok := strings.Cut(line, "ms] ")
				if !ok || content != test.lines[i] {
					t.Fatalf("line %d = %q, want content %q", i, line, test.lines[i])
				}
			}
		})
	}
}

type failingWriter struct{}

var errWriteFailed = errors.New("write failed")

func (failingWriter) Write([]byte) (int, error) { return 0, errWriteFailed }

type shortWriter struct{}

func (shortWriter) Write(p []byte) (int, error) { return len(p) - 1, nil }

func TestWorkerOutputPropagatesWriteFailures(t *testing.T) {
	tests := []struct {
		name      string
		out       io.Writer
		lineStart bool
		want      error
	}{
		{"prefix failure", failingWriter{}, true, errWriteFailed},
		{"body failure", failingWriter{}, false, errWriteFailed},
		{"short body write", shortWriter{}, false, io.ErrShortWrite},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			w := &workerOutput{worker: 1, start: time.Now(), out: test.out, lineStart: test.lineStart}
			_, err := w.Write([]byte("line\n"))
			if !errors.Is(err, test.want) {
				t.Fatalf("Write error = %v, want %v", err, test.want)
			}
		})
	}
}

func TestWorkgroupWaitReturnsAllErrorsInPriorityOrder(t *testing.T) {
	t.Parallel()

	control := errors.New("control")
	overrun := libhegel.E_STOP_TEST
	invalid := libhegel.E_ASSUME
	user0 := interestingInvocationError(errors.New("user zero"))
	user1 := interestingInvocationError(errors.New("user one"))
	g := make(workgroup, 6)
	g.Go(4, func() error { return user1 })
	g.Go(3, func() error { return invalid })
	g.Go(2, func() error { return overrun })
	g.Go(5, func() error { return nil })
	g.Go(1, func() error { return control })
	g.Go(0, func() error { return user0 })

	errs := g.Wait()
	want := []struct {
		worker int
		err    error
	}{{1, control}, {2, overrun}, {3, invalid}, {0, user0}, {4, user1}}
	if len(errs) != len(want) {
		t.Fatalf("Wait returned %d errors, want %d", len(errs), len(want))
	}
	for i, expected := range want {
		var worker *workerError
		if !errors.As(errs[i], &worker) || worker.worker != expected.worker || !errors.Is(errs[i], expected.err) {
			t.Errorf("error %d = %#v, want worker %d error %v", i, errs[i], expected.worker, expected.err)
		}
	}
}

func TestStateMachineLogsDroppedWorkerErrors(t *testing.T) {
	t.Parallel()

	firstPanic := interestingInvocationError(errors.New("first panic"))
	secondFailure := failureInvocationError("second failure")
	primary := errors.New("primary framework error")
	sm, err := newStateMachine(&outputRuleMachine{}, WithBoundedConcurrency(4))
	if err != nil {
		t.Fatalf("newStateMachine: %v", err)
	}
	var out strings.Builder
	shared := &concurrentTestCaseShared{
		selectedConcurrency: 4,
		nextRuleErrors: map[int64]error{
			0: primary,
			1: errors.New("secondary framework error"),
			2: firstPanic,
			3: secondFailure,
		},
	}
	defer func() {
		var worker *workerError
		if got := recover(); !errors.As(got.(error), &worker) || worker.worker != 0 {
			t.Fatalf("panic = %#v, want worker 0 framework error", got)
		}
		text := out.String()
		if strings.Contains(text, "secondary framework error") {
			t.Fatalf("secondary framework error was logged:\n%s", text)
		}
		if !strings.Contains(text, "Dropped concurrent panic from worker 2") || !strings.Contains(text, ": first panic") ||
			!strings.Contains(text, "Dropped concurrent failure from worker 3") || !strings.Contains(text, ": second failure") {
			t.Fatalf("dropped errors not logged:\n%s", text)
		}
	}()
	sm.Run(&concurrentTestCase{shared: shared, out: &out})
}

func TestStateMachineSelectsWorkerError(t *testing.T) {
	t.Parallel()

	userFailure := interestingInvocationError(errors.New("user failure"))
	invalid := libhegel.E_ASSUME
	overrun := libhegel.E_STOP_TEST
	control := errors.New("control")
	lowest := errors.New("worker zero")

	tests := []struct {
		name   string
		errs   map[int64]error
		want   error
		worker int
	}{
		{"lowest worker breaks ties", map[int64]error{0: lowest, 1: errors.New("worker one")}, lowest, 0},
		{"invalid beats user failure", map[int64]error{0: userFailure, 1: invalid}, invalid, 1},
		{"overrun beats invalid", map[int64]error{0: invalid, 1: overrun}, overrun, 1},
		{"control beats overrun", map[int64]error{0: overrun, 1: control}, control, 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			sm, err := newStateMachine(&singleRuleMachine{}, WithBoundedConcurrency(2))
			if err != nil {
				t.Fatalf("newStateMachine: %v", err)
			}
			shared := &concurrentTestCaseShared{
				selectedConcurrency: 2,
				nextRuleErrors:      test.errs,
			}
			defer func() {
				got, ok := recover().(*workerError)
				if !ok || got.worker != test.worker || !errors.Is(got, test.want) {
					t.Fatalf("panic = %#v, want worker %d error %v", got, test.worker, test.want)
				}
			}()
			sm.Run(&concurrentTestCase{shared: shared})
		})
	}
}

func TestRunStatefulSingleRule(t *testing.T) {
	t.Parallel()
	Test(t, func(ht *T) {
		RunStateful(ht, &singleRuleMachine{})
	})
}

func TestRunStatefulPanicsOnValidationError(t *testing.T) {
	t.Parallel()
	assertPanicsWithMessage(t, "no rules", func() {
		RunStateful(&testCase{}, &noRulesMachine{})
	})
}

func TestRunStatefulAssumeRejectionRetries(t *testing.T) {
	t.Parallel()
	totalGateRuns := 0
	// A single test case only increments gateCount when swarm testing leaves
	// both rules enabled (probability 1/3: the engine draws a per-case
	// disabling probability p uniform on [0,1) and E[(1-p)^2] = 1/3) AND the
	// engine draws a step budget of at least two (RuleGated can only succeed
	// after a successful RuleOpen step). Measured over 3000 test cases, a
	// case has gateCount == 0 with probability q = 0.648, so with 10 test
	// cases this test failed at q^10 ~ 1.3% per run. 100 test cases push the
	// false-failure probability down to q^100 ~ 2e-19.
	err := Run(func(tc TestCase) {
		m := &gatedMachine{}
		RunStateful(tc, m)
		totalGateRuns += m.gateCount
	}, WithTestCases(100))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if totalGateRuns == 0 {
		t.Error("RuleGated never succeeded; assume retry appears broken")
	}
}

// TestRunStatefulNextRuleErrorAborts covers the error branch after
// stateMachineNextRule in Run: when the engine fails to draw the next rule (for
// example E_STOP_TEST once its per-test-case choice budget is exhausted), Run
// aborts the test case. The engine no longer surfaces this through the
// integration tests, so it is injected via a stubbed test case.
func TestRunStatefulNextRuleErrorAborts(t *testing.T) {
	t.Parallel()
	sm, err := newStateMachine(&singleRuleMachine{})
	if err != nil {
		t.Fatalf("newStateMachine: %v", err)
	}
	tc := newStubTestCase(t,
		uintptr(2), int64(1), libhegel.OK, // new_state_machine
		uintptr(3), libhegel.OK, // test_case_clone
		uintptr(4),                                 // context_new for the cloned test case
		libhegel.StateMachineGroup(0), libhegel.OK, // state_machine_next_group
		int64(0), libhegel.E_STOP_TEST, "overrun", // state_machine_next_rule + last-error message
	)
	defer expectErrorPanic(t, libhegel.E_STOP_TEST)
	sm.Run(tc)
}

func TestRunStatefulInvariantViolationFails(t *testing.T) {
	t.Parallel()
	err := run(func(tc TestCase) {
		RunStateful(tc, &invariantViolator{})
	}, WithTestCases(10))
	if err == nil {
		t.Fatal("Expected error")
	}
}

// TestRunStatefulRuleFailureAborts covers callRule's deferred abort(nil) when a
// rule leaves the case in a non-VALID status (via Fail) rather than panicking.
func TestRunStatefulRuleFailureAborts(t *testing.T) {
	t.Parallel()
	err := run(func(tc TestCase) {
		RunStateful(tc, &ruleFailer{})
	}, WithTestCases(10))
	if err == nil {
		t.Fatal("Expected error")
	}
}
