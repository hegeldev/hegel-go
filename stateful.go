package hegel

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"reflect"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

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
	rules                []stateMachineRule
	invariants           []stateMachineRule
	maxConcurrency       int
	configuredRuleGroups []stateMachineRuleGroup
	ruleGroups           []int64
}

type stateMachineRuleGroup struct {
	name  string
	rules []string
}

// StateMachineOption configures a state machine run.
type StateMachineOption func(*stateMachine)

// WithConcurrency allows state machine rules to run concurrently. The engine
// chooses a worker count between 1 and [runtime.GOMAXPROCS] for each test case,
// so concurrent execution is not guaranteed for every test case.
//
// Workers invoke rule methods concurrently on the same machine value. Any
// state read or mutated by rules must therefore be safe for concurrent access,
// for example through mutexes or atomics. Rules must not depend on their
// relative ordering. Each worker receives its own TestCase, so the TestCase
// passed to one rule must not be retained or shared with another worker.
//
// Execution proceeds in rounds. After every worker has finished its rules for
// a round, all invariants run before the next round begins.
// Invariants are never executed concurrently.
func WithConcurrency() StateMachineOption {
	return WithBoundedConcurrency(runtime.GOMAXPROCS(0))
}

// WithBoundedConcurrency is like [WithConcurrency], but uses n instead of
// [runtime.GOMAXPROCS] as the maximum worker count.
//
// n is an upper bound, not a guarantee that every test case runs n rules at
// once.
func WithBoundedConcurrency(n int) StateMachineOption {
	return func(sm *stateMachine) {
		sm.maxConcurrency = n
	}
}

// WithRuleGroup assigns rules to a named concurrency group. Rules in the same
// group may run concurrently with each other; rules in different groups never
// overlap. Pass the full method names, including the Rule prefix. Calls using
// the same group name add rules to that group.
//
// Each call creates a distinct group. Rules not mentioned by any call share
// one implicit default group, so they may overlap with each other but not with
// rules in an explicit group. A rule may belong to only one group.
//
// RunStateful panics if a group is empty, names an unknown rule or invariant,
// or names a rule already assigned by another WithRuleGroup option.
func WithRuleGroup(name string, rules ...string) StateMachineOption {
	ruleNames := slices.Clone(rules)
	return func(sm *stateMachine) {
		for i := range sm.configuredRuleGroups {
			if sm.configuredRuleGroups[i].name == name {
				sm.configuredRuleGroups[i].rules = append(sm.configuredRuleGroups[i].rules, ruleNames...)
				return
			}
		}
		sm.configuredRuleGroups = append(sm.configuredRuleGroups, stateMachineRuleGroup{name: name, rules: ruleNames})
	}
}

// stateMachineRule is a discovered rule or invariant: the method name and
// a bound function value with the receiver pre-applied at discovery time.
type stateMachineRule struct {
	name string
	fn   func(TestCase)
}

// newStateMachine inspects machine's method set and returns a runner.
func newStateMachine[M any, T interface{ *M }](machine T, opts ...StateMachineOption) (*stateMachine, error) {
	if machine == nil {
		return nil, fmt.Errorf("state machine pointer must not be nil")
	}
	sm := &stateMachine{maxConcurrency: 1}
	for _, opt := range opts {
		opt(sm)
	}
	if sm.maxConcurrency < 1 {
		return nil, fmt.Errorf("state machine maximum concurrency must be positive")
	}

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
	ruleIndices := make(map[string]int, len(sm.rules))
	for i, rule := range sm.rules {
		ruleIndices[rule.name] = i
	}
	invariants := make(map[string]struct{}, len(sm.invariants))
	for _, invariant := range sm.invariants {
		invariants[invariant.name] = struct{}{}
	}

	// Group 0 is the "catch all".
	sm.ruleGroups = make([]int64, len(sm.rules))
	assigned := make(map[string]int)
	for groupIndex, group := range sm.configuredRuleGroups {
		if len(group.rules) == 0 {
			return nil, fmt.Errorf("state machine rule group %d is empty", groupIndex+1)
		}
		for _, name := range group.rules {
			ruleIndex, ok := ruleIndices[name]
			if !ok {
				if _, ok := invariants[name]; ok {
					return nil, fmt.Errorf("state machine rule group names invariant %q; only rules can be grouped", name)
				}
				return nil, fmt.Errorf("state machine rule group references unknown rule %q; available rules: %s", name, strings.Join(names(sm.rules), ", "))
			}
			if previousGroup, ok := assigned[name]; ok {
				return nil, fmt.Errorf("state machine rule %q belongs to both group %d and group %d", name, previousGroup, groupIndex+1)
			}
			assigned[name] = groupIndex + 1
			sm.ruleGroups[ruleIndex] = int64(groupIndex + 1)
		}
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
	machine, concurrency, err := tc.stateMachineNew(names(sm.rules), sm.ruleGroups, names(sm.invariants), sm.maxConcurrency)
	if err != nil {
		tc.abort(err)
	}

	type stateMachineWorker struct {
		tc     TestCase
		output *bytes.Buffer
	}
	workers := make([]stateMachineWorker, 0, concurrency)
	start := time.Now()
	for worker := range concurrency {
		clone, err := tc.clone()
		if err != nil {
			tc.abort(err)
		}
		var output *bytes.Buffer
		if concurrency > 1 && clone.output() != nil {
			output = new(bytes.Buffer)
			clone.setOutput(&workerOutput{worker: worker, start: start, out: output, lineStart: true})
		}
		workers = append(workers, stateMachineWorker{tc: clone, output: output})
	}

	if sm.maxConcurrency != 1 {
		tc.log("Concurrency level: %d", concurrency)
	}
	tc.log("Initial invariant check.")
	for _, inv := range sm.invariants {
		if _, err := invokeRule(tc, inv.fn); err != nil {
			tc.abort(err)
		}
	}

	workersGroup := make(workgroup, concurrency)

	// The engine drives execution in rounds: stateMachineNextGroup is asked at
	// every join point — including before the first rule — whether another
	// round should run, and each round's rule stream is pulled via
	// stateMachineNextRule until the engine signals the join point. With a
	// single group and concurrency 1 this runs the familiar sequential loop;
	// the engine owns the overall step budget.
	for round := 1; ; round++ {
		group, err := tc.stateMachineNextGroup(machine)
		if err != nil {
			tc.abort(err)
		}
		if group == libhegel.StateMachineDone {
			break
		}
		groupName := "<anonymous>"
		if group != 0 {
			groupName = sm.configuredRuleGroups[int(group)-1].name
		}
		tc.log("---------------- Round %d: group %q ----------------", round, groupName)

		for i, worker := range workers {
			workersGroup.Go(i, func() error {
				for {
					idx, err := worker.tc.stateMachineNextRule(machine, int64(i))
					if err != nil {
						return err
					}
					if idx == libhegel.StateMachineDone {
						return nil
					}
					rule := sm.rules[idx]
					worker.tc.log("Rule: %s", rule.name)

					rejected, err := invokeRule(worker.tc, rule.fn)
					if err != nil { // coverage-ignore
						return err
					}
					if rejected {
						if err := worker.tc.stateMachineRuleRejected(machine, int64(i)); err != nil {
							return err
						}
						worker.tc.log("Rule stopped early due to violated assumption.")
					}
				}
			})
		}
		errs := workersGroup.Wait()
		for _, worker := range workers {
			if worker.output != nil && worker.output.Len() != 0 {
				tc.log("%s", strings.TrimSuffix(worker.output.String(), "\n"))
				worker.output.Reset()
			}
		}
		if len(errs) != 0 {
			dropped := errs[1:]
			sort.Slice(dropped, func(i, j int) bool {
				return dropped[i].worker < dropped[j].worker
			})
			for _, err := range dropped {
				var outcome *invocationError
				if errors.As(err.err, &outcome) {
					location := findCallerInPCs(outcome.pcs, isNotHegelFrame)
					tc.log("Dropped concurrent %s from worker %d at %s: %s", outcome.kind, err.worker, location, outcome)
				}
			}
			tc.abort(errs[0])
		}
		for _, inv := range sm.invariants {
			if _, err := invokeRule(tc, inv.fn); err != nil { // coverage-ignore
				tc.abort(err)
			}
		}
	}
}

// invokeRule gives each rule or invariant its own abort boundary. Assume
// rejects only that invocation; all other outcomes propagate to the outer test
// case, which owns completion of the native stream.
func invokeRule(tc TestCase, fn testBody) (bool, error) {
	err := tc.invoke(fn)
	if errors.Is(err, libhegel.E_ASSUME) {
		return true, nil
	}
	return false, err
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
// Rules run sequentially by default. Pass [WithConcurrency] or
// [WithBoundedConcurrency] to allow rules to run concurrently, and
// [WithRuleGroup] to restrict which rules may overlap.
//
// It panics if a method takes TestCase but is not prefixed
// with Rule or Invariant, if a Rule- or Invariant-prefixed method has
// the wrong signature, or if the machine has no rules.
func RunStateful[M any, T interface{ *M }](tc TestCase, machine T, opts ...StateMachineOption) {
	sm, err := newStateMachine(machine, opts...)
	if err != nil {
		panic(err)
	}
	sm.Run(tc)
}

type workerResult struct {
	worker int
	err    error
}

type workerOutput struct {
	worker    int64
	start     time.Time
	out       io.Writer
	lineStart bool
}

func (w *workerOutput) Write(p []byte) (int, error) {
	written := 0
	for len(p) != 0 {
		if w.lineStart {
			elapsed := time.Since(w.start).Seconds() * 1000
			if _, err := fmt.Fprintf(w.out, "[worker %d +%.3fms] ", w.worker, elapsed); err != nil {
				return written, err
			}
			w.lineStart = false
		}
		end := bytes.IndexByte(p, '\n') + 1
		if end == 0 {
			end = len(p)
		}
		n, err := w.out.Write(p[:end])
		written += n
		if err != nil {
			return written, err
		}
		if n != end {
			return written, io.ErrShortWrite
		}
		w.lineStart = p[end-1] == '\n'
		p = p[end:]
	}
	return written, nil
}

type workerError struct {
	worker int
	err    error
}

func (e *workerError) Error() string { return e.err.Error() }
func (e *workerError) Unwrap() error { return e.err }

type workgroup chan workerResult

func (g workgroup) Go(worker int, fn func() error) {
	go func() {
		g <- workerResult{worker: worker, err: fn()}
	}()
}

func (g workgroup) Wait() []*workerError {
	priority := func(err error) int {
		var userFailure *invocationError
		switch {
		case errors.As(err, &userFailure):
			return 3
		case errors.Is(err, libhegel.E_ASSUME):
			return 2
		case errors.Is(err, libhegel.E_STOP_TEST):
			return 1
		default:
			return 0
		}
	}

	results := make([]workerResult, 0, cap(g))
	for range cap(g) {
		result := <-g
		if result.err != nil {
			results = append(results, result)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		left, right := results[i], results[j]
		leftPriority, rightPriority := priority(left.err), priority(right.err)
		if leftPriority != rightPriority {
			return leftPriority < rightPriority
		}
		return left.worker < right.worker
	})
	errs := make([]*workerError, len(results))
	for i, result := range results {
		errs[i] = &workerError{worker: result.worker, err: result.err}
	}
	return errs
}
