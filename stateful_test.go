package hegel

import (
	"testing"

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

type assumeRejecting struct{}

func (assumeRejecting) RuleReject(tc TestCase) { tc.Assume(false) }

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

func TestRunStatefulRuleAssumeFalse(t *testing.T) {
	t.Parallel()
	Test(t, func(ht *T) {
		RunStateful(ht, &assumeRejecting{})
	})
}

func TestRunStatefulAssumeRejectionRetries(t *testing.T) {
	t.Parallel()
	totalGateRuns := 0
	err := Run(func(tc TestCase) {
		m := &gatedMachine{}
		RunStateful(tc, m)
		totalGateRuns += m.gateCount
	}, WithTestCases(10))
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
		uintptr(1), libhegel.OK, // new_state_machine
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
