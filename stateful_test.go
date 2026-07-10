package hegel

import (
	"testing"
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
