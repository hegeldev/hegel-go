package hegel_test

import (
	"hegel.dev/go/hegel"
)

// stateCounter is a tiny state machine:
// every test case starts with a fresh counter and Hegel drives a sequence
// of rule invocations against it. The invariant fires if the counter
// ever leaves the sensible range.
type stateCounter struct{ n int }

func (c *stateCounter) reset() { c.n = 0 }

// RuleIncrement bumps the counter up by one.
func (c *stateCounter) RuleIncrement(_ hegel.TestCase) { c.n++ }

// RuleDecrement bumps the counter down by one.
func (c *stateCounter) RuleDecrement(_ hegel.TestCase) { c.n-- }

// InvariantSensible panics if the counter has drifted out of range.
func (c *stateCounter) InvariantSensible(_ hegel.TestCase) {
	if c.n < -10000 || c.n > 10000 {
		panic("counter out of sensible range")
	}
}

func ExampleRunStateful() {
	var c stateCounter
	hegel.MustRun(func(tc hegel.TestCase) {
		// Reset model state before each case — in a real scenario this
		// might flush a database or other shared store.
		c.reset()

		hegel.RunStateful(tc, &c)
	})
	// Output:
}
