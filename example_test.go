package hegel_test

import (
	"fmt"
	"math"
	"testing"

	"hegel.dev/go/hegel"
)

func ExampleCase() {
	t := &testing.T{} // in real code, use the *testing.T from your test function
	t.Run("integers", hegel.Case(func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(0, 200))
		if n >= 50 {
			ht.Fatalf("n=%d is too large", n)
		}
	}))
}

func ExampleCase_withTestCases() {
	t := &testing.T{}
	t.Run("many", hegel.Case(func(ht *hegel.T) {
		a := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		b := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		if a+b != b+a {
			ht.Fatal("addition is not commutative")
		}
	}, hegel.WithTestCases(500)))
}

func ExampleRun() {
	err := hegel.Run(func(s *hegel.TestCase) {
		n := hegel.Draw(s, hegel.Integers(0, 100))
		if n < 0 || n > 100 {
			panic("out of range")
		}
	}, hegel.WithTestCases(50))
	_ = err
}

func ExampleDraw() {
	t := &testing.T{}
	t.Run("multiple_values", hegel.Case(func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
		s := hegel.Draw(ht, hegel.Text().MaxSize(50))
		_ = n // n is int
		_ = s // s is string
	}))
}

func ExampleFilter() {
	t := &testing.T{}
	t.Run("even_integers", hegel.Case(func(ht *hegel.T) {
		evenIntegers := hegel.Filter(hegel.Integers(math.MinInt, math.MaxInt), func(v int) bool {
			return v%2 == 0
		})
		n := hegel.Draw(ht, evenIntegers)
		if n%2 != 0 {
			ht.Fatalf("%d is not even", n)
		}
	}))
}

func ExampleCase_assume() {
	t := &testing.T{}
	t.Run("division", hegel.Case(func(ht *hegel.T) {
		n1 := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		n2 := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		ht.Assume(n2 != 0)
		q, r := n1/n2, n1%n2
		if n1 != q*n2+r {
			ht.Fatalf("%d != %d*%d + %d", n1, q, n2, r)
		}
	}))
}

func ExampleMap() {
	t := &testing.T{}
	t.Run("string_of_digits", hegel.Case(func(ht *hegel.T) {
		s := hegel.Draw(ht, hegel.Map(hegel.Integers(0, 100), func(n int) string {
			return fmt.Sprintf("%d", n)
		}))
		for _, c := range s {
			if c < '0' || c > '9' {
				ht.Fatalf("%q contains non-digit %c", s, c)
			}
		}
	}))
}

func ExampleDraw_dependentGeneration() {
	t := &testing.T{}
	t.Run("list_with_valid_index", hegel.Case(func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(1, 10))
		lst := hegel.Draw(ht, hegel.Lists(
			hegel.Integers(math.MinInt, math.MaxInt),
		).MinSize(int(n)).MaxSize(int(n)))
		index := hegel.Draw(ht, hegel.Integers(0, n-1))
		if index < 0 || index >= len(lst) {
			ht.Fatal("index out of range")
		}
	}))
}

func ExampleFlatMap() {
	t := &testing.T{}
	t.Run("flatmap_example", hegel.Case(func(ht *hegel.T) {
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
	}))
}

func ExampleCase_note() {
	t := &testing.T{}
	t.Run("commutativity", hegel.Case(func(ht *hegel.T) {
		x := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		y := hegel.Draw(ht, hegel.Integers(-1000, 1000))
		ht.Note(fmt.Sprintf("trying x=%d, y=%d", x, y))
		if x+y != y+x {
			ht.Fatal("addition is not commutative")
		}
	}))
}

func ExampleCase_target() {
	t := &testing.T{}
	t.Run("seek_large_values", hegel.Case(func(ht *hegel.T) {
		x := hegel.Draw(ht, hegel.Integers(0, 10000))
		ht.Target(float64(x), "maximize_x")
		if x > 9999 {
			ht.Fatalf("x=%d exceeds limit", x)
		}
	}, hegel.WithTestCases(1000)))
}
