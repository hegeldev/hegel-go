package hegel

import (
	"fmt"
	"reflect"
	"strings"

	"hegel.dev/go/hegel/internal/libhegel"
)

// statefulMaxSteps caps the number of rule invocations per test case.
const statefulMaxSteps = 50

// stateMachine drives a user-supplied struct's Rule-prefixed and
// Invariant-prefixed methods as a property-tested state machine.
//
// By convention, rules may mutate the machine but invariants must not;
// the framework cannot enforce this, and a mutating invariant will
// produce misleading test runs.
type stateMachine struct {
	rules      []stateMachineRule
	invariants []stateMachineRule
}

// stateMachineRule is a discovered rule or invariant: the method name and
// a bound function value with the receiver pre-applied at discovery time.
type stateMachineRule struct {
	name string
	fn   func(TestCase)
}

// newStateMachine inspects machine's method set and returns a runner.
func newStateMachine[M any, T interface{ *M }](machine T) (*stateMachine, error) {
	if machine == nil {
		return nil, fmt.Errorf("state machine pointer must not be nil")
	}
	sm := &stateMachine{}

	rt := reflect.TypeOf(machine)
	rv := reflect.ValueOf(machine)
	tcType := reflect.TypeFor[TestCase]()

	for i := range rt.NumMethod() {
		m := rt.Method(i)
		name := m.Name
		mt := m.Type

		takesTestCase := false
		for j := 1; j < mt.NumIn(); j++ {
			if mt.In(j) == tcType {
				takesTestCase = true
				break
			}
		}

		isRule := strings.HasPrefix(name, "Rule")
		isInvariant := strings.HasPrefix(name, "Invariant")

		if !isRule && !isInvariant {
			if takesTestCase {
				return nil, fmt.Errorf("method %s takes TestCase but is not prefixed with Rule or Invariant", name)
			}
			continue
		}

		fn, ok := rv.Method(i).Interface().(func(TestCase))
		if !ok {
			return nil, fmt.Errorf("method %s: rules and invariants must have signature func(TestCase) with no return", name)
		}

		r := stateMachineRule{name: name, fn: fn}
		if isRule {
			sm.rules = append(sm.rules, r)
		} else {
			sm.invariants = append(sm.invariants, r)
		}
	}

	if len(sm.rules) == 0 {
		return nil, fmt.Errorf("state machine has no rules; at least one method must be prefixed with Rule")
	}

	return sm, nil
}

// names returns the method names of the given rules in registration order,
// matching the rule indices the engine draws via [TestCase.stateMachineNextRule].
func names(rules []stateMachineRule) []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.name
	}
	return out
}

// Run drives a state machine.
//
// It registers the machine with the engine, runs every invariant once, then
// draws a step count and for each step asks the engine which rule to run next
// (the engine owns rule selection, including swarm testing) and invokes it.
// After each successful rule all invariants are re-run.
//
// Rules that reject the current pre-state via [TestCase.Assume] are
// skipped and another rule is drawn, up to a retry budget.
func (sm *stateMachine) Run(tc TestCase) {
	machine, err := tc.stateMachineNew(names(sm.rules), names(sm.invariants))
	if err != nil {
		tc.abort(err)
	}

	tc.Note("Initial invariant check.")
	for _, inv := range sm.invariants {
		if _, err := callRule(tc, inv.fn); err != nil {
			tc.abort(err)
		}
	}

	// The engine drives execution in rounds: stateMachineNextGroup is asked at
	// every join point — including before the first rule — whether another
	// round should run, and each round's rule stream is pulled via
	// stateMachineNextRule until the engine signals the join point. With a
	// single group and concurrency 1 this runs the familiar sequential loop;
	// the engine owns the overall step budget.
	step := 0
	for {
		group, err := tc.stateMachineNextGroup(machine)
		if err != nil {
			tc.abort(err)
		}
		// StateMachineDone (-1) from next_group terminates the whole machine.
		if group == libhegel.StateMachineDone {
			break
		}
		for {
			idx, err := tc.stateMachineNextRule(machine)
			if err != nil {
				tc.abort(err)
			}
			// StateMachineDone (-1) from next_rule ends this round's rule
			// stream: rejoin and ask for the next group.
			if idx == libhegel.StateMachineDone {
				break
			}
			step++
			rule := sm.rules[idx]
			tc.Note(fmt.Sprintf("Step %d: %s", step, rule.name))

			ok, err := callRule(tc, rule.fn)
			if err != nil { // coverage-ignore
				tc.abort(err)
			}
			if !ok {
				continue
			}
			for _, inv := range sm.invariants {
				if _, err := callRule(tc, inv.fn); err != nil { // coverage-ignore
					tc.abort(err)
				}
			}
		}
	}
}

// callRule brackets fn(tc) in a labelStateful span and recovers from
// [TestCase.Assume] rejections so the caller can try a different rule.
// It returns true if fn ran to completion, false if it rejected via
// Assume. Other panics propagate to the caller.
func callRule(tc TestCase, fn func(TestCase)) (bool, error) {
	return withSpan(tc, libhegel.LABEL_STATEFUL_RULE, func() (bool, error) {
		defer func() {
			if tc.getStatus() == libhegel.STATUS_INVALID {
				tc.setStatus(libhegel.STATUS_VALID)
			}

			if tc.getStatus() != libhegel.STATUS_VALID {
				tc.abort(nil)
			}
		}()
		defer tc.recoverAbort()
		fn(tc)
		return true, nil
	})
}

// RunStateful enables model-based testing.
//
// machine is a pointer to a struct which implements a state-machine in terms
// of rules and invariants.
//
// Methods whose name starts with "Rule" and whose signature is
// func(TestCase) are registered as rules. Methods whose name
// starts with "Invariant" with the same signature are registered as
// invariants.
//
// It panics if a method takes TestCase but is not prefixed
// with Rule or Invariant, if a Rule- or Invariant-prefixed method has
// the wrong signature, or if the machine has no rules.
func RunStateful[M any, T interface{ *M }](tc TestCase, machine T) {
	sm, err := newStateMachine(machine)
	if err != nil {
		panic(err)
	}
	sm.Run(tc)
}
