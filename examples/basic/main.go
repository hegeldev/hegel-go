// basic demonstrates property testing with primitive generators in Hegel.
//
// It tests three simple mathematical properties — commutativity of addition,
// identity element of multiplication, and integer bounds — using GenerateInt
// and GenerateBool. Run it with: go run ./examples/basic
package main

import (
	"fmt"

	hegel "github.com/antithesishq/hegel-go"
)

func main() {
	// Property 1: addition is commutative.
	hegel.RunHegelTest("add_commutative", func() {
		a := hegel.GenerateInt(-1_000_000, 1_000_000)
		b := hegel.GenerateInt(-1_000_000, 1_000_000)
		if a+b != b+a {
			panic(fmt.Sprintf("add not commutative: %d + %d ≠ %d + %d", a, b, b, a))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ addition is commutative")

	// Property 2: multiplying by one is identity.
	hegel.RunHegelTest("mul_identity", func() {
		n := hegel.GenerateInt(-1_000_000, 1_000_000)
		if n*1 != n {
			panic(fmt.Sprintf("n*1 != n: %d", n))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ n*1 == n")

	// Property 3: integer bounds are respected.
	const lo, hi = -500, 500
	hegel.RunHegelTest("integer_bounds", func() {
		n := hegel.GenerateInt(lo, hi)
		if n < lo || n > hi {
			panic(fmt.Sprintf("out of range: %d ∉ [%d, %d]", n, lo, hi))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ integers respect bounds")

	// Property 4: OR with false is identity (b || false == b).
	hegel.RunHegelTest("bool_or_false", func() {
		b := hegel.GenerateBool()
		//nolint:gosimple // property test: explicitly checking the identity law
		if (b || false) != b {
			panic(fmt.Sprintf("b || false != b for b=%v", b))
		}
	}, hegel.WithTestCases(50))
	fmt.Println("✅ b || false == b")

	// Property 5: Assume filters out unwanted cases.
	hegel.RunHegelTest("division_remainder", func() {
		n := hegel.GenerateInt(-1000, 1000)
		d := hegel.GenerateInt(-1000, 1000)
		hegel.Assume(d != 0)
		// Euclidean division invariant.
		q, r := n/d, n%d
		if n != q*d+r {
			panic(fmt.Sprintf("%d ≠ %d*%d + %d", n, q, d, r))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("✅ integer division satisfies n == q*d + r")
}
