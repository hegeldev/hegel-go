package hegel

import (
	"fmt"
	"strings"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

func assertPanicsWithMessage(t *testing.T, substr string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatal("expected panic")
		}
		msg := fmt.Sprint(r)
		if !strings.Contains(msg, substr) {
			t.Fatalf("expected panic containing %q, got: %s", substr, msg)
		}
	}()
	f()
}

// assertErrorContains asserts that err is non-nil and its message contains substr.
func assertErrorContains(t *testing.T, substr string, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), substr) {
		t.Fatalf("expected error containing %q, got: %s", substr, err.Error())
	}
}

func TestIntegersMinGreaterThanMax(t *testing.T) {
	assertPanicsWithMessage(t, "max_value", func() { Integers(10, 5) })
}

func TestIntegersEqualMinMax(t *testing.T) {
	Integers(5, 5)
}

func TestIntegersFromMinGreaterThanMax(t *testing.T) {
	assertPanicsWithMessage(t, "max_value", func() { Integers[int64](10, 5) })
}

// Float allow_nan / allow_infinity validation happens in params(), before any
// engine call, so draw(nil) surfaces the error without touching the test case.
// (max_value < min_value is validated by the engine instead — see below.)

func TestFloatsAllowNaNWithMin(t *testing.T) {
	_, err := Floats[float64]().Min(0.0).AllowNaN(true).draw(nil)
	assertErrorContains(t, "allow_nan", err)
}

func TestFloatsAllowNaNWithMax(t *testing.T) {
	_, err := Floats[float64]().Max(10.0).AllowNaN(true).draw(nil)
	assertErrorContains(t, "allow_nan", err)
}

// max_value < min_value is validated by the engine, so this drives a real
// test case rather than draw(nil).
func TestFloatsMinGreaterThanMax(t *testing.T) {
	_, err := Floats[float64]().Min(10.0).Max(5.0).draw(newRealTestCase(t))
	assertErrorContains(t, "10", err)
}

func TestFloatsAllowInfinityWithBothBounds(t *testing.T) {
	_, err := Floats[float64]().Min(0.0).Max(10.0).AllowInfinity(true).draw(nil)
	assertErrorContains(t, "allow_infinity", err)
}

// Text / Characters validation happens in build(), which the draw acquires the
// engine for first, so these drive a stubbed test case (no engine ops are
// consumed because the error is raised before the string draw).

func TestTextMinSizeNegative(t *testing.T) {
	_, err := Text().MinSize(-1).MaxSize(10).draw(newStubTestCase(t))
	assertErrorContains(t, "min_size", err)
}

// max_size < min_size is validated by the engine (during string-generator
// construction), so this drives the real build path.
func TestTextMinGreaterThanMax(t *testing.T) {
	_, err := Text().MinSize(10).MaxSize(5).draw(newRealTestCase(t))
	assertErrorContains(t, "10", err)
}

func TestTextAlphabetWithCodecError(t *testing.T) {
	_, err := Text().MaxSize(10).Alphabet("abc").Codec("ascii").draw(newStubTestCase(t))
	assertErrorContains(t, "cannot combine", err)
}

func TestTextAlphabetWithCategoriesError(t *testing.T) {
	_, err := Text().MaxSize(10).Alphabet("abc").Categories([]string{"Lu"}).draw(newStubTestCase(t))
	assertErrorContains(t, "cannot combine", err)
}

func TestTextCategoriesIncludingCsError(t *testing.T) {
	_, err := Text().MaxSize(10).Categories([]string{"L", "Cs"}).draw(newStubTestCase(t))
	assertErrorContains(t, "surrogate", err)
}

func TestTextCategoriesIncludingCSuperCatError(t *testing.T) {
	_, err := Text().MaxSize(10).Categories([]string{"C"}).draw(newStubTestCase(t))
	assertErrorContains(t, "surrogate", err)
}

func TestCharactersCategoriesIncludingCsError(t *testing.T) {
	_, err := Characters().Categories([]string{"Cs"}).draw(newStubTestCase(t))
	assertErrorContains(t, "surrogate", err)
}

func TestCharactersCategoriesIncludingCSuperCatError(t *testing.T) {
	_, err := Characters().Categories([]string{"C"}).draw(newStubTestCase(t))
	assertErrorContains(t, "surrogate", err)
}

// An inverted codepoint range leaves no characters available, an error the
// engine only surfaces during string-generator construction (Characters is
// fixed-size, so it can't reach the engine via min>max the way Text does).
func TestCharactersInvertedCodepointRange(t *testing.T) {
	_, err := Characters().MinCodepoint(0x5A).MaxCodepoint(0x41).draw(newRealTestCase(t))
	assertErrorContains(t, "hegel_string_generator_text", err)
}

func TestBinaryMinSizeNegative(t *testing.T) {
	assertPanicsWithMessage(t, "min_size", func() { Binary(-1, 10) })
}

func TestBinaryMinGreaterThanMax(t *testing.T) {
	assertPanicsWithMessage(t, "max_size", func() { Binary(10, 5) })
}

// Lists / Maps validation happens in validate(), before withSpan / engine, so
// draw(nil) surfaces the error.

func TestListsMinGreaterThanMax(t *testing.T) {
	_, err := Lists(Booleans()).MinSize(10).MaxSize(5).draw(nil)
	assertErrorContains(t, "max_size", err)
}

func TestListsMinSizeNegative(t *testing.T) {
	_, err := Lists(Booleans()).MinSize(-1).draw(nil)
	assertErrorContains(t, "min_size", err)
}

func TestListsMaxSizeNegative(t *testing.T) {
	_, err := Lists(Booleans()).MaxSize(-1).draw(nil)
	assertErrorContains(t, "max_size", err)
}

func TestMapsMinSizeNegative(t *testing.T) {
	_, err := Maps(Integers(0, 100), Integers(0, 100)).MinSize(-1).draw(nil)
	assertErrorContains(t, "min_size", err)
}

func TestMapsMaxSizeNegative(t *testing.T) {
	_, err := Maps(Integers(0, 100), Integers(0, 100)).MaxSize(-1).draw(nil)
	assertErrorContains(t, "max_size", err)
}

func TestMapsMinGreaterThanMax(t *testing.T) {
	_, err := Maps(Integers(0, 100), Integers(0, 100)).MinSize(10).MaxSize(5).draw(nil)
	assertErrorContains(t, "max_size", err)
}

// Domains max_length validation is done by the engine, so these drive the real
// build path.

func TestDomainsTooSmallMaxLength(t *testing.T) {
	_, err := Domains().MaxLength(3).build(libhegel.NewContext())
	assertErrorContains(t, "3", err)
}

func TestDomainsNonPositiveMaxLength(t *testing.T) {
	_, err := Domains().MaxLength(0).build(libhegel.NewContext())
	assertErrorContains(t, "0", err)
}

func TestDomainsTooBigMaxLength(t *testing.T) {
	_, err := Domains().MaxLength(256).build(libhegel.NewContext())
	assertErrorContains(t, "256", err)
}

func TestOneOfZeroGenerators(t *testing.T) {
	assertPanicsWithMessage(t, "OneOf", func() { OneOf[bool]() })
}

func TestOneOfSingleGeneratorNoPanic(t *testing.T) {
	// one generator should be accepted
	OneOf(Booleans())
}

// invalidFloats returns a Floats generator whose draw always errors (invalid
// params), for use as a malformed inner generator in error-propagation tests.
func invalidFloats() Generator[float64] {
	return Floats[float64]().Min(0.0).AllowNaN(true)
}

// --- inner-error propagation from a nested generator's draw ---

func TestListsInnerErrorPropagates(t *testing.T) {
	// start_span(LIST), new_collection, collection_more=true, then the element
	// draw (invalidFloats) fails in params() before any engine call.
	tc := newStubTestCase(t, libhegel.OK, libhegel.Collection(0), libhegel.OK, true, libhegel.OK)
	_, err := Lists(invalidFloats()).draw(tc)
	assertErrorContains(t, "allow_nan", err)
}

func TestMapsKeyErrorPropagates(t *testing.T) {
	tc := newStubTestCase(t, libhegel.OK, libhegel.Collection(0), libhegel.OK, true, libhegel.OK)
	_, err := Maps[float64, int](invalidFloats(), Integers(0, 1)).draw(tc)
	assertErrorContains(t, "allow_nan", err)
}

func TestMapsValueErrorPropagates(t *testing.T) {
	// The key draw (Integers) succeeds before the value draw fails, so it
	// consumes one generate_integer output.
	tc := newStubTestCase(t, libhegel.OK, libhegel.Collection(0), libhegel.OK, true, libhegel.OK, int64(0), libhegel.OK)
	_, err := Maps[int, float64](Integers(0, 1), invalidFloats()).draw(tc)
	assertErrorContains(t, "allow_nan", err)
}

func TestOneOfBranchErrorPropagates(t *testing.T) {
	// start_span(ONE_OF), generate_integer (branch index 0), then the branch
	// draw fails in params().
	tc := newStubTestCase(t, libhegel.OK, int64(0), libhegel.OK)
	_, err := OneOf(invalidFloats()).draw(tc)
	assertErrorContains(t, "allow_nan", err)
}

func TestOptionalInnerErrorPropagates(t *testing.T) {
	// start_span(OPTIONAL), generate_integer=1 (draw the inner value), then the
	// inner draw fails in params().
	tc := newStubTestCase(t, libhegel.OK, int64(1), libhegel.OK)
	_, err := Optional(invalidFloats()).draw(tc)
	assertErrorContains(t, "allow_nan", err)
}

// --- draw() surfaces invalid configuration ---

func TestListsDrawInvalidConfigReturnsError(t *testing.T) {
	gen := Lists(Booleans()).MinSize(-1)
	_, err := gen.draw(nil)
	assertErrorContains(t, "min_size", err)
}

func TestMapsDrawInvalidConfigReturnsError(t *testing.T) {
	gen := Maps(Integers(0, 1), Integers(0, 1)).MinSize(-1)
	_, err := gen.draw(nil)
	assertErrorContains(t, "min_size", err)
}

// TestMapInvalidSourceReturnsErrorOnDraw verifies that Map no longer validates
// its source at construction (it wraps unconditionally); the invalid inner
// config surfaces when the mapped generator is drawn.
func TestMapInvalidSourceReturnsErrorOnDraw(t *testing.T) {
	gen := Map(invalidFloats(), func(v float64) float64 { return v })
	// start_span(MAPPED), then the inner draw fails in params().
	tc := newStubTestCase(t, libhegel.OK)
	_, err := gen.draw(tc)
	assertErrorContains(t, "allow_nan", err)
}

func TestFloatsDrawInvalidConfigReturnsError(t *testing.T) {
	gen := Floats[float64]().Min(10.0).Max(5.0)
	_, err := gen.draw(newRealTestCase(t))
	assertErrorContains(t, "10", err)
}

func TestTextDrawInvalidConfigReturnsError(t *testing.T) {
	gen := Text().MinSize(-1).MaxSize(5)
	_, err := gen.draw(newStubTestCase(t))
	assertErrorContains(t, "min_size", err)
}

func TestCharactersDrawInvalidConfigReturnsError(t *testing.T) {
	gen := Characters().Categories([]string{"Cs"})
	_, err := gen.draw(newStubTestCase(t))
	assertErrorContains(t, "surrogate", err)
}

func TestDomainsDrawInvalidConfigReturnsError(t *testing.T) {
	gen := Domains().MaxLength(0)
	_, err := gen.draw(newRealTestCase(t))
	assertErrorContains(t, "0", err)
}
