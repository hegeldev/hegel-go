package hegel

// showcase_test.go demonstrates idiomatic Hegel property-based tests.
// These tests verify real properties — every generated value is used in a meaningful assertion.

import "testing"

// TestAdditionIsCommutative verifies that integer addition is commutative.
func TestAdditionIsCommutative(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		x := GenerateInt(-1000, 1000)
		y := GenerateInt(-1000, 1000)
		if x+y != y+x {
			panic("addition is not commutative")
		}
	}, WithTestCases(50))
}

// TestAbsoluteValueIsNonNegative verifies that |x| >= 0 for all integers.
func TestAbsoluteValueIsNonNegative(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		x := GenerateInt(-1000, 1000)
		abs := x
		if abs < 0 {
			abs = -abs
		}
		if abs < 0 {
			panic("abs(x) is negative")
		}
	}, WithTestCases(50))
}

// TestDoubleNegationIsIdentity verifies that negating a boolean twice gives the original.
func TestDoubleNegationIsIdentity(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest(t.Name(), func() {
		b := GenerateBool()
		notB := !b
		notNotB := !notB
		if notNotB != b {
			panic("double negation is not identity")
		}
	}, WithTestCases(20))
}
