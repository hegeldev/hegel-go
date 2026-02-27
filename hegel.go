// Package hegel provides a Go SDK for the Hegel property-based testing framework.
//
// Hegel is a universal property-based testing framework backed by Hypothesis.
// This SDK communicates with the hegel binary (a Python subprocess) via Unix
// sockets using a custom binary protocol, enabling property-based testing in Go.
//
// # Quick Start
//
// Run a property test with [RunHegelTest]:
//
//	func TestMyProperty(t *testing.T) {
//	    hegel.RunHegelTest("my_property", func() {
//	        n, _ := hegel.ExtractInt(hegel.Integers(0, 100).Generate())
//	        if n < 0 || n > 100 {
//	            panic("out of range")
//	        }
//	    }, hegel.WithTestCases(50))
//	}
//
// Inside the test body, use the composable [Generator] types returned by functions
// such as [Integers], [Booleans], [Text], [Lists], and [OneOf].
//
// Use [Assume] to filter invalid inputs, [Note] to attach debug messages that
// appear only on the minimal failing example, and [Target] to guide Hegel
// toward interesting boundary values.
//
// # Architecture
//
// The SDK is structured in layers:
//
//  1. Wire protocol (readPacket, writePacket) — 20-byte header, CBOR payload, CRC32
//  2. Connection and channels (connection, channel) — Unix socket multiplexing
//  3. Test runner ([RunHegelTest], [RunHegelTestE]) — subprocess lifecycle, test loop
//  4. Generators ([Generator], [BasicGenerator], [Lists], [Dicts], …) — value generation
//
// See the README and docs/getting-started.md for a full tutorial.
package hegel

// Version returns the current version of the Hegel Go SDK.
func Version() string {
	return "0.1.0"
}
