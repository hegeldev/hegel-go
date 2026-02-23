// Package hegel provides a property-based testing SDK powered by the Hegel framework.
//
// Hegel is a universal property-based testing framework backed by Hypothesis.
// This SDK communicates with the hegel binary (a Python server) via Unix sockets
// using a custom binary protocol, enabling property-based testing in Go.
//
// Usage:
//
//	import "github.com/antithesishq/hegel-go"
//
//	func TestMyProperty(t *testing.T) {
//	    hegel.RunTest(t, "my_property", func() {
//	        n := hegel.Generate(hegel.Integers())
//	        // assert properties about n
//	    })
//	}
package hegel

// Version returns the current version of the Hegel Go SDK.
func Version() string {
	return "0.1.0"
}
