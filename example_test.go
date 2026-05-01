package hegel_test

import (
	"fmt"
	"math"
	"testing"

	"hegel.dev/go/hegel"
)

func ExampleTest() {
	t := &testing.T{} // in real code, use the *testing.T from your test function
	hegel.Test(t, func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(0, 200))
		if n >= 50 {
			ht.Fatalf("n=%d is too large", n)
		}
	})
}

func ExampleTest_withTestCases() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		a := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		b := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		if a+b != b+a {
			ht.Fatal("addition is not commutative")
		}
	}, hegel.WithTestCases(500))
}

func ExampleTest_withDatabase() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(0, 100))
		if n < 0 {
			ht.Fatal("negative integer should not be generated")
		}
	}, hegel.WithDatabase(hegel.Database("my_hegel_database")))
}

func ExampleTest_disableDatabase() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		_ = hegel.Draw(ht, hegel.Booleans())
	}, hegel.WithDatabase(hegel.DatabaseDisabled()))
}

func ExampleTest_withDerandomize() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		_ = hegel.Draw(ht, hegel.Integers(0, 100))
	}, hegel.WithDerandomize(true))
}

func ExampleDraw() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
		s := hegel.Draw(ht, hegel.Text().MaxSize(50))
		_ = n // n is int
		_ = s // s is string
	})
}

func ExampleFilter() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		evenIntegers := hegel.Filter(hegel.Integers(math.MinInt, math.MaxInt), func(v int) bool {
			return v%2 == 0
		})
		n := hegel.Draw(ht, evenIntegers)
		if n%2 != 0 {
			ht.Fatalf("%d is not even", n)
		}
	})
}

func ExampleTest_assume() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		n1 := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		n2 := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		ht.Assume(n2 != 0)
		q, r := n1/n2, n1%n2
		if n1 != q*n2+r {
			ht.Fatalf("%d != %d*%d + %d", n1, q, n2, r)
		}
	})
}

func ExampleMap() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		s := hegel.Draw(ht, hegel.Map(hegel.Integers(0, 100), func(n int) string {
			return fmt.Sprintf("%d", n)
		}))
		for _, c := range s {
			if c < '0' || c > '9' {
				ht.Fatalf("%q contains non-digit %c", s, c)
			}
		}
	})
}

func ExampleDraw_dependentGeneration() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(1, 10))
		lst := hegel.Draw(ht, hegel.Lists(
			hegel.Integers(math.MinInt, math.MaxInt),
		).MinSize(int(n)).MaxSize(int(n)))
		index := hegel.Draw(ht, hegel.Integers(0, n-1))
		if index < 0 || index >= len(lst) {
			ht.Fatal("index out of range")
		}
	})
}

func ExampleFlatMap() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		result := hegel.Draw(ht, hegel.FlatMap(
			hegel.Integers(1, 5),
			func(n int) hegel.Generator[[]int] {
				return hegel.Lists(
					hegel.Integers(math.MinInt, math.MaxInt),
				).MinSize(n).MaxSize(n)
			},
		))
		if len(result) < 1 || len(result) > 5 {
			ht.Fatalf("unexpected list length: %d", len(result))
		}
	})
}

func ExampleTest_note() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		x := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		y := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		ht.Note(fmt.Sprintf("trying x=%d, y=%d", x, y))
		if x+y != y+x {
			ht.Fatal("addition is not commutative")
		}
	})
}

func ExampleTest_target() {
	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		x := hegel.Draw(ht, hegel.Integers(0, 10000))
		ht.Target(float64(x), "maximize_x")
		if x > 9999 {
			ht.Fatalf("x=%d exceeds limit", x)
		}
	}, hegel.WithTestCases(1000))
}

func ExampleComposite() {
	type person struct {
		Name string
		Age  int
	}

	personGen := hegel.Composite("person", func(tc *hegel.TestCase) person {
		return person{
			Name: hegel.Draw(tc, hegel.Text().MinSize(1).MaxSize(50)),
			Age:  hegel.Draw(tc, hegel.Integers(0, 120)),
		}
	})

	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		p := hegel.Draw(ht, personGen)
		if p.Age < 0 || p.Age > 120 {
			ht.Fatalf("age out of range: %d", p.Age)
		}
	})
}

func ExampleComposite_dataDependentDrawCount() {
	// The number of element draws depends on n, drawn moments earlier.
	// FlatMap can express this for fixed depth; Composite scales to
	// arbitrary, schema-driven shapes.
	variableList := hegel.Composite("variable_list", func(tc *hegel.TestCase) []int {
		n := hegel.Draw(tc, hegel.Integers(0, 10))
		out := make([]int, n)
		for i := range n {
			out[i] = hegel.Draw(tc, hegel.Integers(0, 1000))
		}
		return out
	})

	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		v := hegel.Draw(ht, variableList)
		if len(v) > 10 {
			ht.Fatalf("length %d exceeds bound", len(v))
		}
	})
}

func ExampleComposite_recursive() {
	// A binary tree generator that references itself: each node may contain
	// subtrees produced by the same generator. The forward declaration lets
	// the closure capture nodeGen before it's assigned.
	//
	// Recursion is explicitly bounded by a depth counter held in the closure
	// — incremented before each recursive Draw and decremented after, so it
	// tracks the live recursion stack. This is the safest pattern; an
	// unbounded variant relying purely on Hegel's per-test-case data budget
	// is possible but harder to reason about.
	type Node struct {
		Value int
		Left  *Node
		Right *Node
	}

	const maxDepth = 5
	depth := 0
	var nodeGen hegel.Generator[*Node]
	nodeGen = hegel.Composite("node", func(tc *hegel.TestCase) *Node {
		n := &Node{Value: hegel.Draw(tc, hegel.Integers(0, 100))}
		if depth < maxDepth && hegel.Draw(tc, hegel.Booleans()) {
			depth++
			n.Left = hegel.Draw(tc, nodeGen)
			n.Right = hegel.Draw(tc, nodeGen)
			depth--
		}
		return n
	})

	t := &testing.T{}
	hegel.Test(t, func(ht *hegel.T) {
		_ = hegel.Draw(ht, nodeGen)
	})
}
