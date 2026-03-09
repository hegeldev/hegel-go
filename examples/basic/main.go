// basic demonstrates property testing with primitive generators in Hegel.
//
// It tests three simple mathematical properties -- commutativity of addition,
// identity element of multiplication, and integer bounds -- using the Integers
// and Booleans generators. Run it with: go run ./examples/basic
package main

import (
	"fmt"

	hegel "github.com/antithesishq/hegel-go"
)

func main() {
	// Property 1: addition is commutative.
	hegel.MustRun("add_commutative", func(s *hegel.TestCase) {
		a := hegel.Draw(s, hegel.Integers[int](-1_000_000, 1_000_000))
		b := hegel.Draw(s, hegel.Integers[int](-1_000_000, 1_000_000))
		if a+b != b+a {
			panic(fmt.Sprintf("add not commutative: %d + %d != %d + %d", a, b, b, a))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("addition is commutative")

	// Property 2: multiplying by one is identity.
	hegel.MustRun("mul_identity", func(s *hegel.TestCase) {
		n := hegel.Draw(s, hegel.Integers[int](-1_000_000, 1_000_000))
		if n*1 != n {
			panic(fmt.Sprintf("n*1 != n: %d", n))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("n*1 == n")

	// Property 3: integer bounds are respected.
	const lo, hi = -500, 500
	hegel.MustRun("integer_bounds", func(s *hegel.TestCase) {
		n := hegel.Draw(s, hegel.Integers[int](lo, hi))
		if n < lo || n > hi {
			panic(fmt.Sprintf("out of range: %d not in [%d, %d]", n, lo, hi))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("integers respect bounds")

	// Property 4: OR with false is identity (b || false == b).
	hegel.MustRun("bool_or_false", func(s *hegel.TestCase) {
		b := hegel.Draw(s, hegel.Booleans(0.5))
		//nolint:gosimple // property test: explicitly checking the identity law
		if (b || false) != b {
			panic(fmt.Sprintf("b || false != b for b=%v", b))
		}
	}, hegel.WithTestCases(50))
	fmt.Println("b || false == b")

	// Property 5: Assume filters out unwanted cases.
	hegel.MustRun("division_remainder", func(s *hegel.TestCase) {
		n := hegel.Draw(s, hegel.Integers[int](-1000, 1000))
		d := hegel.Draw(s, hegel.Integers[int](-1000, 1000))
		s.Assume(d != 0)
		// Euclidean division invariant.
		q, r := n/d, n%d
		if n != q*d+r {
			panic(fmt.Sprintf("%d != %d*%d + %d", n, q, d, r))
		}
	}, hegel.WithTestCases(200))
	fmt.Println("integer division satisfies n == q*d + r")
}
