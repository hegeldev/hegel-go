package hegel

import (
	"fmt"
	"reflect"
	"strings"
	"sync"

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
	machine, concurrency, err := tc.stateMachineNew(names(sm.rules), names(sm.invariants))
	if err != nil {
		tc.abort(err)
	}

	tcs := make([]TestCase, 0, concurrency)
	for range concurrency {
		clone, err := tc.clone()
		if err != nil {
			tc.abort(err)
		}
		tcs = append(tcs, clone)
	}

	tc.Note("Initial invariant check.")
	for _, inv := range sm.invariants {
		if err := callRule(tc, inv.fn); err != nil {
			tc.abort(err)
		}
	}

	// The engine drives execution in rounds: stateMachineNextGroup is asked at
	// every join point — including before the first rule — whether another
	// round should run, and each round's rule stream is pulled via
	// stateMachineNextRule until the engine signals the join point. With a
	// single group and concurrency 1 this runs the familiar sequential loop;
	// the engine owns the overall step budget.
	var wg sync.WaitGroup
	for {
		group, err := tc.stateMachineNextGroup(machine)
		if err != nil {
			tc.abort(err)
		}
		if group == libhegel.StateMachineDone {
			break
		}
		for i, tc := range tcs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer tc.recoverAbort()

				for {
					idx, err := tc.stateMachineNextRule(machine, int64(i))
					if err != nil {
						tc.abort(err)
					}
					if idx == libhegel.StateMachineDone {
						break
					}
					rule := sm.rules[idx]

					if err := callRule(tc, rule.fn); err != nil { // coverage-ignore
						tc.abort(err)
					}
				}
			}()
		}
		wg.Wait()
		// TODO: Need to collect results from workers?
		for _, inv := range sm.invariants {
			if err := callRule(tc, inv.fn); err != nil { // coverage-ignore
				tc.abort(err)
			}
		}
	}
}

// callRule brackets fn(tc) in a labelStateful span and recovers from
// [TestCase.Assume] rejections so the caller can try a different rule.
// It returns true if fn ran to completion, false if it rejected via
// Assume. Other panics propagate to the caller.
func callRule(tc TestCase, fn func(TestCase)) error {
	_, err := withSpan(tc, libhegel.LABEL_STATEFUL, func() (struct{}, error) {
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
		return struct{}{}, nil
	})
	return err
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
