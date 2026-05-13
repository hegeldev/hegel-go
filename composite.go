package hegel

// compositeGenerator is a Generator built from an imperative function that
// composes other generators via [Draw]. It has no schema and always falls
// back to compositional generation.
type compositeGenerator[T any] struct {
	fn func(TestCase) T
}

// draw invokes the composed function with the given test case.
//
// The user fn calls [Draw], which panics at the exported boundary with the
// sentinel error types. Those panics propagate up to the runner's recover,
// which handles them identically to errors returned via the Generator
// contract — no wrapping generator branches on error type. Letting them
// through preserves the original stack trace for non-sentinel panics
// (fatalSentinel from T.Fatal, user panics) so [extractPanicOrigin] can
// point at the user's code rather than this defer site.
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
func (g *compositeGenerator[T]) draw(tc TestCase) (T, error) {
	return g.fn(tc), nil
}

// asBasic always returns not-basic — composite generators have no schema.
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
func (g *compositeGenerator[T]) asBasic() (*basicGenerator[T], bool, error) {
	return nil, false, nil
}

// Composite returns a Generator backed by an imperative function.
//
// Inside fn, call [Draw] on other generators to assemble the value. The
// function may call Draw any number of times.
//
// Example: a generator for a Person whose driving license field only appears
// when age >= 18.
//
//	type Person struct {
//	    Name           string
//	    Age            int
//	    DrivingLicense bool
//	}
//
//	personGen := hegel.Composite(func(tc hegel.TestCase) Person {
//	    age := hegel.Draw(tc, hegel.Integers(0, 120))
//	    name := hegel.Draw(tc, hegel.Text())
//	    p := Person{Age: age, Name: name}
//	    if age >= 18 {
//	        p.DrivingLicense = hegel.Draw(tc, hegel.Booleans())
//	    }
//	    return p
//	})
//
//	hegel.Test(t, func(ht *hegel.T) {
//	    p := hegel.Draw(ht, personGen)
//	    // assert properties of p
//	})
func Composite[T any](fn func(TestCase) T) Generator[T] {
	return &compositeGenerator[T]{fn: fn}
}
