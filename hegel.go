// Package hegel provides a Go SDK for the Hegel property-based testing framework.
//
// Hegel is a universal property-based testing framework backed by Hypothesis.
// This SDK communicates with the hegel binary (a Python subprocess) via Unix
// sockets using a custom binary protocol, enabling property-based testing in Go.
//
// # Quick Start
//
// Run a property test with [Case] inside Go tests:
//
//	func TestMyProperty(t *testing.T) {
//	    t.Run("bounds", hegel.Case(func(ht *hegel.T) {
//	        n := hegel.Draw(ht, hegel.Integers(0, 100))
//	        if n < 0 || n > 100 {
//	            ht.Fatal("out of range")
//	        }
//	    }, hegel.WithTestCases(50)))
//	}
//
// Or use [Run] in standalone binaries:
//
//	err := hegel.Run("my_property", func(s *hegel.TestCase) {
//	    n := hegel.Draw(s, hegel.Integers(0, 100))
//	    if n < 0 || n > 100 {
//	        panic("out of range")
//	    }
//	}, hegel.WithTestCases(50))
//
// Use the composable [Generator] types returned by functions such as [Integers],
// [Booleans], [Text], [Lists], and [OneOf].
//
// # Architecture
//
// The SDK is structured in layers:
//
//  1. Wire protocol (readPacket, writePacket) — 20-byte header, CBOR payload, CRC32
//  2. Connection and channels (connection, channel) — Unix socket multiplexing
//  3. Test runner ([Case], [Run]) — subprocess lifecycle, test loop
//  4. Generators ([Generator], [Lists], [Dicts], …) — value generation
//
// See the README and docs/getting-started.md for a full tutorial.
package hegel
