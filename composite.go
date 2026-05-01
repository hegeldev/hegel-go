package hegel

// labelComposite marks a user-defined composite generation span.
//
// Composites are imperative: the user's function may call Draw any number of
// times, in any order, possibly recursively. The server treats every composite
// span as opaque for shrinking purposes — the structure inside is whatever the
// user's function decides on this run.
const labelComposite spanLabel = 16

// compositeGenerator is a user-defined generator built from an imperative
// function that calls Draw on other generators.
type compositeGenerator[T any] struct {
	label string
	fn    func(*TestCase) T
}

// draw runs the user's function inside a COMPOSITE span. The label string is
// emitted via Note so it appears in replay output for shrunk failures, but the
// span itself uses the fixed labelComposite int so the server's shrinker
// treats every composite uniformly without protocol changes.
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
func (g *compositeGenerator[T]) draw(s *TestCase) T {
	startSpan(s, labelComposite)
	if g.label != "" {
		s.Note("composite: " + g.label)
	}
	result := g.fn(s)
	stopSpan(s, false)
	return result
}

// asBasic always returns not-basic. A composite is imperative by definition —
// it may issue an unbounded number of generate requests, branch on intermediate
// values, and recurse. None of that fits a single JSON schema.
//
//lint:ignore U1000 satisfies Generator interface; staticcheck misses generic dispatch
func (g *compositeGenerator[T]) asBasic() (*basicGenerator[T], bool, error) {
	return nil, false, nil
}

// Composite returns a Generator backed by an imperative function.
//
// Inside fn, call [Draw] on other generators to assemble the value. The
// function may call Draw any number of times, branch on intermediate results,
// and recurse — Hegel records each draw in a span tree so shrinking still
// works. The label string appears in replay output and aids debugging shrunk
// failures; it does not affect generation or shrinking.
//
// Composite is the analog of Hypothesis's @composite. Reach for it when the
// shape of generation is genuinely state-dependent — the number of draws
// depends on a previously drawn value, fields appear conditionally on a
// discriminator, or the structure recurses. For independent fixed-shape
// values, prefer the declarative combinators ([Lists], [Maps], [OneOf],
// [Optional]); they give the shrinker more structural visibility.
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
//	personGen := hegel.Composite("person", func(tc *hegel.TestCase) Person {
//	    age := hegel.Draw(tc, hegel.Integers(0, 120))
//	    name := hegel.Draw(tc, hegel.Text().MinSize(1).MaxSize(50))
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
func Composite[T any](label string, fn func(*TestCase) T) Generator[T] {
	if fn == nil {
		panic("Composite requires a non-nil function")
	}
	return &compositeGenerator[T]{label: label, fn: fn}
}
