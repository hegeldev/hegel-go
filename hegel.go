// Package hegel provides a Go SDK for the Hegel property-based testing framework.
//
// Hegel is a universal property-based testing framework backed by Hypothesis.
// This SDK communicates with the hegel binary.
//
// # Quick Start
//
// Run a property test with [Case] inside Go tests:
//
//	func TestMyProperty(t *testing.T) {
//	    t.Run("bounds", hegel.Case(func(ht *hegel.T) {
//	        n := hegel.Draw(ht, hegel.Integers[int](0, 100))
//	        if n < 0 || n > 100 {
//	            ht.Fatal("out of range")
//	        }
//	    }, hegel.WithTestCases(50)))
//	}
//
// Use the composable [Generator] types returned by functions such as [Integers],
// [Booleans], [Text], [Lists], and [OneOf].
//
// See the README and docs/getting-started.md for a full tutorial.
package hegel
