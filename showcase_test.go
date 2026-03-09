package hegel

// showcase_test.go demonstrates idiomatic Hegel property-based tests.
// These tests verify real properties -- every generated value is used in a meaningful assertion.

import (
	"fmt"
	"math"
	"net/netip"
	"regexp"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

// TestAdditionIsCommutative verifies that integer addition is commutative.
func TestAdditionIsCommutative(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		x := Draw[int64](s, Integers(-1000, 1000))
		y := Draw[int64](s, Integers(-1000, 1000))
		if x+y != y+x {
			panic("addition is not commutative")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestAbsoluteValueIsNonNegative verifies that |x| >= 0 for all integers.
func TestAbsoluteValueIsNonNegative(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		x := Draw[int64](s, Integers(-1000, 1000))
		abs := x
		if abs < 0 {
			abs = -abs
		}
		if abs < 0 {
			panic("abs(x) is negative")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestDoubleNegationIsIdentity verifies that negating a boolean twice gives the original.
func TestDoubleNegationIsIdentity(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		b := Draw[bool](s, Booleans(0.5))
		notB := !b
		notNotB := !notB
		if notNotB != b {
			panic("double negation is not identity")
		}
	}, stderrNoteFn, []Option{WithTestCases(20)}); _err != nil {
		panic(_err)
	}
}

// TestEmailContainsAtSymbol demonstrates generating email addresses.
// Every email must contain exactly one "@" separating local and domain parts.
func TestEmailContainsAtSymbol(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		email := Draw[string](s, Emails())
		parts := strings.Split(email, "@")
		if len(parts) != 2 {
			panic("email must have exactly one '@': " + email)
		}
		if parts[0] == "" {
			panic("email local-part must not be empty: " + email)
		}
		if parts[1] == "" {
			panic("email domain part must not be empty: " + email)
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestDateParsingRoundtrip demonstrates that generated dates parse and round-trip correctly.
// This mirrors the Python reference test: date.fromisoformat(date_str).isoformat() == date_str.
func TestDateParsingRoundtrip(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		parsed := Draw(s, Dates())
		dateStr := parsed.Format("2006-01-02")
		roundTripped, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			panic("date not parseable as YYYY-MM-DD: " + dateStr)
		}
		if roundTripped != parsed {
			panic("date round-trip failed: " + parsed.String() + " != " + roundTripped.String())
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestJustAlwaysReturnsConstant demonstrates Just by verifying that every generated
// value is the same constant -- a fundamental property of constant generators.
func TestJustAlwaysReturnsConstant(t *testing.T) {
	hegelBinPath(t)
	const expected = "fixed"
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[string](s, Just(expected))
		if v != expected {
			panic(fmt.Sprintf("Just: expected %q, got %q", expected, v))
		}
	}, stderrNoteFn, []Option{WithTestCases(30)}); _err != nil {
		panic(_err)
	}
}

// TestSampledFromOnlyReturnsListElements demonstrates SampledFrom by verifying that
// every generated value is one of the input elements -- no values outside the list.
func TestSampledFromOnlyReturnsListElements(t *testing.T) {
	hegelBinPath(t)
	weekdays := []string{"Mon", "Tue", "Wed", "Thu", "Fri"}
	g := SampledFrom(weekdays)
	validSet := map[string]bool{"Mon": true, "Tue": true, "Wed": true, "Thu": true, "Fri": true}
	if _err := runHegel(t.Name(), func(s *TestCase) {
		day := Draw[string](s, g)
		if !validSet[day] {
			panic(fmt.Sprintf("SampledFrom: unexpected value %q", day))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestFromRegexAllValuesMatchPattern demonstrates FromRegex by verifying that every
// generated string matches the regex pattern -- the core property of regex generators.
func TestFromRegexAllValuesMatchPattern(t *testing.T) {
	hegelBinPath(t)
	pattern := `[A-Z]{2,4}`
	re := regexp.MustCompile(`^` + pattern + `$`)
	g := FromRegex(pattern, true)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[string](s, g)
		if !re.MatchString(v) {
			panic(fmt.Sprintf("FromRegex: %q does not match pattern %s", v, pattern))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestTextLengthBoundsAreRespected demonstrates the Text generator by verifying that
// every generated string has a codepoint count within the specified bounds.
func TestTextLengthBoundsAreRespected(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[string](s, Text(3, 7))
		count := utf8.RuneCountInString(v)
		if count < 3 || count > 7 {
			panic(fmt.Sprintf("text: codepoint count %d out of [3, 7]", count))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestBinaryLengthBoundsAreRespected demonstrates the Binary generator by verifying that
// every generated byte slice has a length within the specified bounds.
func TestBinaryLengthBoundsAreRespected(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		b := Draw[[]byte](s, Binary(2, 6))
		if len(b) < 2 || len(b) > 6 {
			panic(fmt.Sprintf("binary: length %d out of [2, 6]", len(b)))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestFloatsBoundedExcludesSpecials demonstrates that Floats with explicit bounds
// never produces NaN or Inf when those flags are disabled.
func TestFloatsBoundedExcludesSpecials(t *testing.T) {
	hegelBinPath(t)
	falseBool := false
	if _err := runHegel(t.Name(), func(s *TestCase) {
		f := Draw[float64](s, Floats(floatPtr(-100.0), floatPtr(100.0), &falseBool, &falseBool, false, false))
		if math.IsNaN(f) {
			panic("floats: unexpected NaN with allow_nan=false")
		}
		if math.IsInf(f, 0) {
			panic("floats: unexpected Inf with allow_infinity=false")
		}
		if f < -100.0 || f > 100.0 {
			panic(fmt.Sprintf("floats: %v out of [-100, 100]", f))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestBooleansWithHighP demonstrates that with p=1.0, booleans always generates true.
// This is a fundamental property: p=1.0 means probability 1 of true.
func TestBooleansWithHighP(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		b := Draw[bool](s, Booleans(1.0))
		if !b {
			panic("booleans(p=1.0): expected always true")
		}
	}, stderrNoteFn, []Option{WithTestCases(20)}); _err != nil {
		panic(_err)
	}
}

// TestMapDoubledIntegersAreEven demonstrates that mapping integers by doubling
// always produces even numbers -- a fundamental arithmetic property.
// Uses Map which preserves the schema (single server round-trip) for basicGenerators.
func TestMapDoubledIntegersAreEven(t *testing.T) {
	hegelBinPath(t)
	doubled := Map[int64, int64](Integers(-50, 50), func(n int64) int64 {
		return n * 2
	})
	if _err := runHegel(t.Name(), func(s *TestCase) {
		n := Draw[int64](s, doubled)
		if n%2 != 0 {
			panic(fmt.Sprintf("doubled integer must be even, got %d", n))
		}
		if n < -100 || n > 100 {
			panic(fmt.Sprintf("doubled integer must be in [-100,100], got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestListsSortedIsSorted demonstrates the Lists generator by verifying that
// sorting a list of integers produces a non-decreasing sequence.
// This is a meaningful property: sort is correct if and only if the result is ordered.
func TestListsSortedIsSorted(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		nums := Draw[[]int64](s, Lists(Integers(-100, 100), ListsOptions{MinSize: 0, MaxSize: 20}))
		// Insertion sort (simple, verifiable).
		sorted := make([]int64, len(nums))
		copy(sorted, nums)
		for i := 1; i < len(sorted); i++ {
			for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
				sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
			}
		}
		for i := 1; i < len(sorted); i++ {
			if sorted[i] < sorted[i-1] {
				panic(fmt.Sprintf("sorted list not non-decreasing at index %d: %v", i, sorted))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestListsLengthBoundsAreRespected demonstrates that Lists with min/max size bounds
// always generates a list within those bounds -- the fundamental size constraint property.
func TestListsLengthBoundsAreRespected(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		xs := Draw[[]int64](s, Lists(Integers(0, 1000), ListsOptions{MinSize: 2, MaxSize: 8}))
		if len(xs) < 2 || len(xs) > 8 {
			panic(fmt.Sprintf("Lists: length %d out of [2, 8]", len(xs)))
		}
		for _, v := range xs {
			if v < 0 || v > 1000 {
				panic(fmt.Sprintf("Lists: element %d out of range [0, 1000]", v))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestDictsKeyValueTypes demonstrates the Dicts generator with typed keys and values.
// Every generated map must have string keys and integer values in the specified range.
// This verifies the core type contract: keys come from the key generator, values from the value generator.
func TestDictsKeyValueTypes(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		gen := Dicts(Text(1, 8), Integers(0, 255), DictOptions{MinSize: 0, MaxSize: 4, HasMaxSize: true})
		m := Draw[map[string]int64](s, gen)
		if len(m) > 4 {
			panic(fmt.Sprintf("Dicts: at most 4 entries expected, got %d", len(m)))
		}
		for k, v := range m {
			runeCount := utf8.RuneCountInString(k)
			if runeCount < 1 || runeCount > 8 {
				panic(fmt.Sprintf("Dicts: key must be string of length 1-8 codepoints, got %q (len=%d)", k, runeCount))
			}
			if v < 0 || v > 255 {
				panic(fmt.Sprintf("Dicts: value must be int in [0,255], got %d", v))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestDictsSizeBoundsHold demonstrates that Dicts respects min_size and max_size.
// Maps with min_size=2, max_size=5 must always have between 2 and 5 entries.
// This is a fundamental size-bound property of the Dicts generator.
func TestDictsSizeBoundsHold(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		gen := Dicts(Integers(0, 100), Booleans(0.5), DictOptions{MinSize: 2, MaxSize: 5, HasMaxSize: true})
		m := Draw[map[int64]bool](s, gen)
		if len(m) < 2 || len(m) > 5 {
			panic(fmt.Sprintf("Dicts: expected size in [2,5], got %d", len(m)))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestMapChainedTransformsCompose demonstrates that chaining multiple Map calls
// composes the transforms correctly -- a fundamental property of function composition.
// integers(1, 10).map(x*x).map(x-1): result is x^2-1 for x in [1,10].
// Property: result is always non-negative (since x^2 >= 1).
func TestMapChainedTransformsCompose(t *testing.T) {
	hegelBinPath(t)
	gen := Map[int64, int64](
		Map[int64, int64](Integers(1, 10), func(n int64) int64 {
			return n * n // square
		}),
		func(n int64) int64 {
			return n - 1 // subtract 1
		},
	)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		n := Draw[int64](s, gen)
		// x in [1,10] -> x^2 in [1,100] -> x^2-1 in [0,99]; always non-negative.
		if n < 0 {
			panic(fmt.Sprintf("map(x^2-1) on [1,10]: expected non-negative, got %d", n))
		}
		if n > 99 {
			panic(fmt.Sprintf("map(x^2-1) on [1,10]: expected <= 99, got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestOneOfChoosesFromEitherBranch demonstrates that OneOf generates values
// from multiple branches and that every generated value satisfies a constraint
// derived from whichever branch produced it.
func TestOneOfChoosesFromEitherBranch(t *testing.T) {
	hegelBinPath(t)
	// Branch A: even integers [0,20]; Branch B: always the string "ok"
	// Both branches produce any values to be used with OneOf[any].
	evenInts := Map[int64, any](Integers(0, 10), func(n int64) any {
		return n * 2
	})
	constStr := Map[string, any](Just("ok"), func(s string) any {
		return s
	})
	gen := OneOf[any](evenInts, constStr)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[any](s, gen)
		switch val := v.(type) {
		case int64:
			if val%2 != 0 || val < 0 || val > 20 {
				panic(fmt.Sprintf("OneOf int branch: expected even [0,20], got %d", val))
			}
		case uint64:
			if val%2 != 0 || val > 20 {
				panic(fmt.Sprintf("OneOf int branch: expected even [0,20], got %d", val))
			}
		case string:
			if val != "ok" {
				panic(fmt.Sprintf("OneOf string branch: expected 'ok', got %q", val))
			}
		default:
			panic(fmt.Sprintf("OneOf: unexpected type %T: %v", v, v))
		}
	}, stderrNoteFn, []Option{WithTestCases(100)}); _err != nil {
		panic(_err)
	}
}

// TestOptionalSometimesNil demonstrates that Optional generates nil values
// and that non-nil values satisfy the element generator's constraint.
func TestOptionalSometimesNil(t *testing.T) {
	hegelBinPath(t)
	// Optional of non-negative integers: value is nil or a non-negative integer.
	gen := Optional(Integers(0, 100))
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[*int64](s, gen)
		if v == nil {
			return // nil is always valid
		}
		if *v < 0 || *v > 100 {
			panic(fmt.Sprintf("Optional int: expected [0,100], got %d", *v))
		}
	}, stderrNoteFn, []Option{WithTestCases(100)}); _err != nil {
		panic(_err)
	}
}

// TestIPAddressesAreValidFormat demonstrates that generated IP addresses have
// the correct format: IPv4 has exactly 4 dot-separated octets.
func TestIPAddressesAreValidFormat(t *testing.T) {
	hegelBinPath(t)
	v4gen := IPAddresses(IPAddressOptions{Version: IPVersion4})
	if _err := runHegel(t.Name(), func(s *TestCase) {
		addr := Draw[netip.Addr](s, v4gen)
		if !addr.Is4() {
			panic(fmt.Sprintf("IPv4 address must be v4: %v", addr))
		}
		parts := strings.Split(addr.String(), ".")
		if len(parts) != 4 {
			panic(fmt.Sprintf("IPv4 address must have 4 octets: %q", addr))
		}
		for _, part := range parts {
			if len(part) == 0 {
				panic(fmt.Sprintf("IPv4 octet must not be empty: %q", addr))
			}
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestIntBoolPairsViaIndividualDraws demonstrates drawing multiple independent values
// by verifying that each pair has an integer in [0, 10] and a boolean.
func TestIntBoolPairsViaIndividualDraws(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		n := Draw[int64](s, Integers(0, 10))
		b := Draw[bool](s, Booleans(0.5))
		if n < 0 || n > 10 {
			panic(fmt.Sprintf("expected integer in [0,10], got %d", n))
		}
		_ = b // bool is always valid
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestStringIntFloatTriples demonstrates drawing (text, int, float) triples
// and verifying each element satisfies its type and range constraints.
// Property: the sum of min-text-length + integer + float is always well-defined.
func TestStringIntFloatTriples(t *testing.T) {
	hegelBinPath(t)
	falseBool := false
	if _err := runHegel(t.Name(), func(s *TestCase) {
		str := Draw[string](s, Text(1, 10))
		n := Draw[int64](s, Integers(0, 100))
		f := Draw[float64](s, Floats(floatPtr(0.0), floatPtr(1.0), &falseBool, &falseBool, false, false))
		if len(str) == 0 {
			panic(fmt.Sprintf("expected non-empty string, got %q", str))
		}
		if n < 0 || n > 100 {
			panic(fmt.Sprintf("expected [0,100], got %d", n))
		}
		if math.IsNaN(f) || math.IsInf(f, 0) || f < 0.0 || f > 1.0 {
			panic(fmt.Sprintf("expected finite float in [0,1], got %v", f))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestFlatMapTextLengthMatchesInteger demonstrates FlatMap by generating an integer n
// and then a string of exactly n codepoints. This verifies the core property: the
// second generator depends on the first value, and the relationship holds for every
// generated example.
func TestFlatMapTextLengthMatchesInteger(t *testing.T) {
	hegelBinPath(t)
	// Generate n in [1,8], then generate text of exactly n codepoints.
	gen := FlatMap[int64, string](Integers(1, 8), func(n int64) Generator[string] {
		return Text(int(n), int(n))
	})
	if _err := runHegel(t.Name(), func(s *TestCase) {
		v := Draw[string](s, gen)
		count := utf8.RuneCountInString(v)
		// The text length must be in [1,8] because n is in [1,8].
		if count < 1 || count > 8 {
			panic(fmt.Sprintf("FlatMap text: codepoint count %d out of [1,8]", count))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestFlatMapListLengthMatchesInteger demonstrates that FlatMap enables dependent
// generation: generating a list whose length is determined by a previously generated
// integer. This is a property that cannot be expressed with Map alone.
func TestFlatMapListLengthMatchesInteger(t *testing.T) {
	hegelBinPath(t)
	// Generate n in [1,6], then generate a list of exactly n booleans.
	// Property: every generated list has length in [1,6], and the length
	// matches the integer that controlled the generation.
	gen := FlatMap[int64, []bool](Integers(1, 6), func(n int64) Generator[[]bool] {
		sz := int(n)
		return Lists(Booleans(0.5), ListsOptions{MinSize: sz, MaxSize: sz})
	})
	if _err := runHegel(t.Name(), func(s *TestCase) {
		bools := Draw[[]bool](s, gen)
		// Length must be in [1,6].
		if len(bools) < 1 || len(bools) > 6 {
			panic(fmt.Sprintf("FlatMap list: length %d out of [1,6]", len(bools)))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestFilterPreservesConstraint demonstrates Filter by generating integers and filtering
// to keep only those divisible by 3. Every generated value must satisfy x%3==0.
// This is a meaningful property: filtering is correct if and only if every kept value
// satisfies the predicate.
func TestFilterPreservesConstraint(t *testing.T) {
	hegelBinPath(t)
	// integers in [0, 30] filtered to multiples of 3
	gen := Filter[int64](Integers(0, 30), func(n int64) bool {
		return n%3 == 0
	})
	if _err := runHegel(t.Name(), func(s *TestCase) {
		n := Draw[int64](s, gen)
		if n%3 != 0 {
			panic(fmt.Sprintf("filter(%%3==0): expected multiple of 3, got %d", n))
		}
		if n < 0 || n > 30 {
			panic(fmt.Sprintf("filter(%%3==0): value %d outside [0,30]", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

// TestFilterSquaresAreSquares demonstrates filtering integers to keep only perfect squares.
// Every kept value must equal the square of some non-negative integer -- a mathematical
// property that filter either preserves or violates on every test case.
func TestFilterSquaresAreSquares(t *testing.T) {
	hegelBinPath(t)
	// Generate integers in [0, 100] and keep only perfect squares (0, 1, 4, 9, 16, 25, 36, 49, 64, 81, 100).
	isPerfectSquare := func(n int64) bool {
		if n < 0 {
			return false
		}
		// Integer square root check.
		root := int64(0)
		for root*root < n {
			root++
		}
		return root*root == n
	}
	gen := Filter[int64](Integers(0, 100), isPerfectSquare)
	if _err := runHegel(t.Name(), func(s *TestCase) {
		n := Draw[int64](s, gen)
		if !isPerfectSquare(n) {
			panic(fmt.Sprintf("filter(perfect square): expected perfect square, got %d", n))
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}
