// embedded_test - Example of using Hegel in embedded mode
//
// Run with: go run ./cmd/embedded_test
//
// This example demonstrates:
// - Running property-based tests without an external hegel process
// - Using the Note() function for debug output
// - Configuring test options

package main

import (
	"fmt"

	hegel "github.com/antithesishq/hegel-go"
)

func main() {
	fmt.Println("Running embedded mode tests...")

	// Test 1: Addition is commutative
	hegel.Hegel(func() {
		x := hegel.Integers[int32]().Generate()
		y := hegel.Integers[int32]().Generate()
		hegel.Note(fmt.Sprintf("Testing commutativity: %d + %d", x, y))
		if x+y != y+x {
			panic("commutativity violated")
		}
	}, hegel.HegelOptions{})
	fmt.Println("Test 1 passed: addition is commutative")

	// Test 2: Slice length is within bounds
	hegel.Hegel(func() {
		s := hegel.Slices(hegel.Integers[int32]()).MinSize(1).MaxSize(10).Generate()
		hegel.Note(fmt.Sprintf("Testing slice length: %d", len(s)))
		if len(s) < 1 || len(s) > 10 {
			panic(fmt.Sprintf("slice length %d not in [1,10]", len(s)))
		}
	}, hegel.HegelOptions{})
	fmt.Println("Test 2 passed: slice length within bounds")

	// Test 3: String generation
	hegel.Hegel(func() {
		s := hegel.Text().MaxSize(50).Generate()
		hegel.Note(fmt.Sprintf("Generated string: %q", s))
		// UTF-8 bytes can exceed codepoint count, so check bytes generously
		if len(s) > 200 {
			panic(fmt.Sprintf("string too long: %d bytes", len(s)))
		}
	}, hegel.HegelOptions{})
	fmt.Println("Test 3 passed: string generation works")

	// Test 4: Filter generates valid values
	hegel.Hegel(func() {
		// Generate only even numbers
		gen := hegel.Filter(
			hegel.Integers[int32]().Min(0).Max(100),
			func(x int32) bool { return x%2 == 0 },
			10,
		)
		value := gen.Generate()
		hegel.Note(fmt.Sprintf("Generated even number: %d", value))
		if value%2 != 0 {
			panic(fmt.Sprintf("expected even number, got %d", value))
		}
	}, hegel.HegelOptions{})
	fmt.Println("Test 4 passed: filter generates valid values")

	fmt.Println("All tests passed!")
}
