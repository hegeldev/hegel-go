// Hegel is a property-based testing library for Go. Hegel is based on
// [Hypothesis], using the [Hegel] protocol.
//
// # Getting started with Hegel for Go
//
// This guide walks you through the basics of installing Hegel and writing your first tests.
//
// # Install Hegel
//
// Add hegel to your module using go get:
//
//	go get hegel.dev/go/hegel@latest
//
// # Write your first test
//
// You're now ready to write your first test. Hegel integrates directly with
// go test via [Case], which returns a func(*testing.T) for use with t.Run:
//
//	func TestIntegerSelfEquality(t *testing.T) {
//		t.Run("integer_self_equality", hegel.Case(func(ht *hegel.T) {
//			n := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
//			if n != n {
//				ht.Fatal("integer was not equal to itself")
//			}
//		}))
//	}
//
// Now run the test using go test. You should see that this test passes.
//
// Let's look at what's happening in more detail. [Case] runs your test
// many times (100, by default). The test function receives a *[T],
// which is used with the [Draw] function for drawing different values.
// This test draws a random integer and checks that it should be equal
// to itself.
//
// Next, try a test that fails:
//
//	func TestIntegersAlwaysBelow50(t *testing.T) {
//		t.Run("integers_always_below_50", hegel.Case(func(ht *hegel.T) {
//			n := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
//			if n >= 50 {
//				ht.Fatalf("n=%d is too large", n)
//			}
//		}))
//	}
//
// This test asserts that any integer is less than 50, which is obviously
// incorrect. Hegel will find a test case that makes this assertion fail,
// and then shrink it to find the smallest counterexample — in this case,
// n = 50.
//
// To fix this test, you can constrain the integers you generate with the
// min and max arguments to [Integers]:
//
//	func TestBoundedIntegersAlwaysBelow50(t *testing.T) {
//		t.Run("bounded_integers_always_below_50", hegel.Case(func(ht *hegel.T) {
//			n := hegel.Draw(ht, hegel.Integers(0, 49))
//			if n >= 50 {
//				ht.Fatalf("n=%d is too large", n)
//			}
//		}))
//	}
//
// Run the test again. It should now pass.
//
// # Use generators
//
// Hegel provides a rich library of generators that you can use out of the
// box. There are primitive generators, such as [Integers], [Floats], and
// [Text], and combinators that allow you to make generators out of other
// generators, such as [Lists] and [Dicts].
//
// For example, you can use [Lists] to generate a slice of integers:
//
//	func TestAppendIncreasesLength(t *testing.T) {
//		t.Run("append_increases_length", hegel.Case(func(ht *hegel.T) {
//			slice := hegel.Draw(ht, hegel.Lists(hegel.Integers(math.MinInt, math.MaxInt)))
//			initialLength := len(slice)
//			slice = append(slice, hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt)))
//			if len(slice) <= initialLength {
//				ht.Fatal("length did not increase")
//			}
//		}))
//	}
//
// This test checks that appending an element to a random slice of integers
// should always increase its length.
//
// You can also build composite data by drawing multiple values. For
// example, say you have a Person struct that we want to generate.
// Because generation in Hegel is imperative, you build the struct by
// drawing its fields directly:
//
//	func TestPerson(t *testing.T) {
//		type Person struct {
//			Age  int
//			Name string
//		}
//		t.Run("person", hegel.Case(func(ht *hegel.T) {
//			person := Person{
//				Age:  hegel.Draw(ht, hegel.Integers(0, 120)),
//				Name: hegel.Draw(ht, hegel.Text(1, 50)),
//			}
//			_ = person // use person in your test
//		}))
//	}
//
// Note that you can feed the results of a [Draw] to subsequent calls.
// For example, say that you extend the Person struct to include a
// DrivingLicense boolean field:
//
//	func TestPersonWithLicense(t *testing.T) {
//		type Person struct {
//			Age            int
//			Name           string
//			DrivingLicense bool
//		}
//		t.Run("person_with_license", hegel.Case(func(ht *hegel.T) {
//			age := hegel.Draw(ht, hegel.Integers(0, 120))
//			name := hegel.Draw(ht, hegel.Text(1, 50))
//			drivingLicense := false
//			if age >= 18 {
//				drivingLicense = hegel.Draw(ht, hegel.Booleans())
//			}
//			person := Person{Age: age, Name: name, DrivingLicense: drivingLicense}
//			_ = person // use person in your test
//		}))
//	}
//
// # Debug your failing test cases
//
// Use the [TestCase.Note] method to attach debug information:
//
//	func TestWithNotes(t *testing.T) {
//		t.Run("with_notes", hegel.Case(func(ht *hegel.T) {
//			x := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
//			y := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
//			ht.Note(fmt.Sprintf("x + y = %d, y + x = %d", x+y, y+x))
//			if x+y != y+x {
//				ht.Fatal("addition is not commutative")
//			}
//		}))
//	}
//
// Notes only appear when Hegel replays the minimal failing example.
//
// # Change the number of test cases
//
// By default Hegel runs 100 test cases. To override this, pass
// [WithTestCases]:
//
//	func TestIntegersMany(t *testing.T) {
//		t.Run("integers_many", hegel.Case(func(ht *hegel.T) {
//			n := hegel.Draw(ht, hegel.Integers(math.MinInt, math.MaxInt))
//			if n != n {
//				ht.Fatal("integer was not equal to itself")
//			}
//		}, hegel.WithTestCases(500)))
//	}
//
// # Learning more
//
//   - Browse the function documentation for the full list of available
//     generators.
//   - See [WithTestCases] and other [Option] functions for more
//     configuration settings to customize how your test runs.
//
// [Hypothesis]: https://github.com/hypothesisworks/hypothesis
// [Hegel]: https://hegel.dev/
//
// [uv]: https://docs.astral.sh/uv/
package hegel
