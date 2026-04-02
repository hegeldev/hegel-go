package hegel_test

import (
	"fmt"
	"math"
	"testing"

	"hegel.dev/go/hegel"
)

// ExampleCase demonstrates a basic property test using Case.
// This example also appears in README.md.
func ExampleCase() {
	t := &testing.T{} // in real code, use the *testing.T from your test function
	t.Run("integers", hegel.Case(func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(0, 200))
		if n >= 50 {
			ht.Fatalf("n=%d is too large", n)
		}
	}))
}

func ExampleCase_additionIdentity() {
	t := &testing.T{}
	t.Run("addition_identity", hegel.Case(func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
		if n+0 != n { // adding zero should never change a number
			ht.Fatal("addition identity failed")
		}
	}))
}

func ExampleCase_integersBelowFifty() {
	t := &testing.T{}
	t.Run("integers_below_50", hegel.Case(func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
		if n >= 50 {
			ht.Fatalf("n=%d is too large", n)
		}
	}))
}

func ExampleCase_boundedIntegers() {
	t := &testing.T{}
	t.Run("bounded_integers_below_50", hegel.Case(func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(0, 49))
		if n >= 50 {
			ht.Fatalf("n=%d is too large", n)
		}
	}))
}

func ExampleCase_appendIncreasesLength() {
	t := &testing.T{}
	t.Run("append_increases_length", hegel.Case(func(ht *hegel.T) {
		slice := hegel.Draw(ht, hegel.Lists(hegel.Integers(math.MinInt, math.MaxInt)))
		initialLength := len(slice)
		slice = append(slice, hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt)))
		if len(slice) <= initialLength {
			ht.Fatal("length did not increase")
		}
	}))
}

func ExampleCase_person() {
	type Person struct {
		Age  int
		Name string
	}
	t := &testing.T{}
	t.Run("person", hegel.Case(func(ht *hegel.T) {
		person := Person{
			Age:  hegel.Draw(ht, hegel.Integers(0, 120)),
			Name: hegel.Draw(ht, hegel.Text(1, 50)),
		}
		_ = person // use person in your test
	}))
}

func ExampleCase_personWithLicense() {
	type Person struct {
		Age            int
		Name           string
		DrivingLicense bool
	}
	t := &testing.T{}
	t.Run("person_with_license", hegel.Case(func(ht *hegel.T) {
		age := hegel.Draw(ht, hegel.Integers(0, 120))
		name := hegel.Draw(ht, hegel.Text(1, 50))
		drivingLicense := false
		if age >= 18 {
			drivingLicense = hegel.Draw(ht, hegel.Booleans())
		}
		person := Person{Age: age, Name: name, DrivingLicense: drivingLicense}
		_ = person // use person in your test
	}))
}

func ExampleCase_note() {
	t := &testing.T{}
	t.Run("with_notes", hegel.Case(func(ht *hegel.T) {
		x := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
		y := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
		ht.Note(fmt.Sprintf("x + y = %d, y + x = %d", x+y, y+x))
		if x+y != y+x {
			ht.Fatal("addition is not commutative")
		}
	}))
}

func ExampleCase_withTestCases() {
	t := &testing.T{}
	t.Run("many_cases", hegel.Case(func(ht *hegel.T) {
		n := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
		if n+0 != n {
			ht.Fatal("addition identity failed")
		}
	}, hegel.WithTestCases(500)))
}
