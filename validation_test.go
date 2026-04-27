package hegel

import (
	"fmt"
	"strings"
	"testing"
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

func TestFloatsAllowNaNWithMin(t *testing.T) {
	_, _, err := Floats[float64]().Min(0.0).AllowNaN(true).asBasic()
	assertErrorContains(t, "allow_nan", err)
}

func TestFloatsAllowNaNWithMax(t *testing.T) {
	_, _, err := Floats[float64]().Max(10.0).AllowNaN(true).asBasic()
	assertErrorContains(t, "allow_nan", err)
}

func TestFloatsMinGreaterThanMax(t *testing.T) {
	_, _, err := Floats[float64]().Min(10.0).Max(5.0).asBasic()
	assertErrorContains(t, "max_value", err)
}

func TestFloatsAllowInfinityWithBothBounds(t *testing.T) {
	_, _, err := Floats[float64]().Min(0.0).Max(10.0).AllowInfinity(true).asBasic()
	assertErrorContains(t, "allow_infinity", err)
}

func TestTextMinSizeNegative(t *testing.T) {
	_, _, err := Text(-1, 10).asBasic()
	assertErrorContains(t, "min_size", err)
}

func TestTextMinGreaterThanMax(t *testing.T) {
	_, _, err := Text(10, 5).asBasic()
	assertErrorContains(t, "max_size", err)
}

func TestTextAlphabetWithCodecError(t *testing.T) {
	_, _, err := Text(0, 10).Alphabet("abc").Codec("ascii").asBasic()
	assertErrorContains(t, "cannot combine", err)
}

func TestTextAlphabetWithCategoriesError(t *testing.T) {
	_, _, err := Text(0, 10).Alphabet("abc").Categories([]string{"Lu"}).asBasic()
	assertErrorContains(t, "cannot combine", err)
}

func TestTextCategoriesIncludingCsError(t *testing.T) {
	_, _, err := Text(0, 10).Categories([]string{"L", "Cs"}).asBasic()
	assertErrorContains(t, "surrogate", err)
}

func TestTextCategoriesIncludingCSuperCatError(t *testing.T) {
	_, _, err := Text(0, 10).Categories([]string{"C"}).asBasic()
	assertErrorContains(t, "surrogate", err)
}

func TestCharactersCategoriesIncludingCsError(t *testing.T) {
	_, _, err := Characters().Categories([]string{"Cs"}).asBasic()
	assertErrorContains(t, "surrogate", err)
}

func TestCharactersCategoriesIncludingCSuperCatError(t *testing.T) {
	_, _, err := Characters().Categories([]string{"C"}).asBasic()
	assertErrorContains(t, "surrogate", err)
}

func TestBinaryMinSizeNegative(t *testing.T) {
	assertPanicsWithMessage(t, "min_size", func() { Binary(-1, 10) })
}

func TestBinaryMinGreaterThanMax(t *testing.T) {
	assertPanicsWithMessage(t, "max_size", func() { Binary(10, 5) })
}

func TestListsMinGreaterThanMax(t *testing.T) {
	_, _, err := Lists(Booleans()).MinSize(10).MaxSize(5).asBasic()
	assertErrorContains(t, "max_size", err)
}

func TestListsMinSizeNegative(t *testing.T) {
	_, _, err := Lists(Booleans()).MinSize(-1).asBasic()
	assertErrorContains(t, "min_size", err)
}

func TestListsMaxSizeNegative(t *testing.T) {
	_, _, err := Lists(Booleans()).MaxSize(-1).asBasic()
	assertErrorContains(t, "max_size", err)
}

func TestDictsMinSizeNegative(t *testing.T) {
	_, _, err := Dicts(Integers(0, 100), Integers(0, 100)).MinSize(-1).asBasic()
	assertErrorContains(t, "min_size", err)
}

func TestDictsMaxSizeNegative(t *testing.T) {
	_, _, err := Dicts(Integers(0, 100), Integers(0, 100)).MaxSize(-1).asBasic()
	assertErrorContains(t, "max_size", err)
}

func TestDictsMinGreaterThanMax(t *testing.T) {
	_, _, err := Dicts(Integers(0, 100), Integers(0, 100)).MinSize(10).MaxSize(5).asBasic()
	assertErrorContains(t, "max_size", err)
}

func TestDomainsTooSmallMaxLength(t *testing.T) {
	_, _, err := Domains().MaxLength(3).asBasic()
	assertErrorContains(t, "max_length", err)
}

func TestDomainsNonPositiveMaxLength(t *testing.T) {
	_, _, err := Domains().MaxLength(0).asBasic()
	assertErrorContains(t, "max_length", err)
}

func TestDomainsTooBigMaxLength(t *testing.T) {
	_, _, err := Domains().MaxLength(256).asBasic()
	assertErrorContains(t, "max_length", err)
}

func TestOneOfZeroGenerators(t *testing.T) {
	assertPanicsWithMessage(t, "OneOf", func() { OneOf[bool]() })
}

func TestOneOfSingleGeneratorNoPanic(t *testing.T) {
	// one generator should be accepted
	OneOf(Booleans())
}

// invalidFloats returns a Floats generator whose asBasic() always errors,
// for use as a malformed inner generator in error-propagation tests.
func invalidFloats() Generator[float64] {
	return Floats[float64]().Min(0.0).AllowNaN(true)
}

// --- inner-error propagation in asBasic ---

func TestListsInnerErrorPropagates(t *testing.T) {
	_, _, err := Lists(invalidFloats()).asBasic()
	assertErrorContains(t, "allow_nan", err)
}

func TestDictsKeyErrorPropagates(t *testing.T) {
	_, _, err := Dicts[float64, int](invalidFloats(), Integers(0, 1)).asBasic()
	assertErrorContains(t, "allow_nan", err)
}

func TestDictsValueErrorPropagates(t *testing.T) {
	_, _, err := Dicts[int, float64](Integers(0, 1), invalidFloats()).asBasic()
	assertErrorContains(t, "allow_nan", err)
}

func TestOneOfBranchErrorPropagates(t *testing.T) {
	_, _, err := OneOf(invalidFloats()).asBasic()
	assertErrorContains(t, "allow_nan", err)
}

func TestOptionalInnerErrorPropagates(t *testing.T) {
	g := Optional(invalidFloats()).(*optionalGenerator[float64])
	_, _, err := g.asBasic()
	assertErrorContains(t, "allow_nan", err)
}

// --- draw() panics when asBasic errors ---

func TestListsDrawInvalidConfigPanics(t *testing.T) {
	gen := Lists(Booleans()).MinSize(-1)
	assertPanicsWithMessage(t, "min_size", func() { gen.draw(nil) })
}

func TestDictsDrawInvalidConfigPanics(t *testing.T) {
	gen := Dicts(Integers(0, 1), Integers(0, 1)).MinSize(-1)
	assertPanicsWithMessage(t, "min_size", func() { gen.draw(nil) })
}

func TestOneOfDrawInvalidBranchPanics(t *testing.T) {
	gen := OneOf(invalidFloats()).(*oneOfGenerator[float64])
	assertPanicsWithMessage(t, "allow_nan", func() { gen.draw(nil) })
}

func TestOptionalDrawInvalidInnerPanics(t *testing.T) {
	gen := Optional(invalidFloats()).(*optionalGenerator[float64])
	assertPanicsWithMessage(t, "allow_nan", func() { gen.draw(nil) })
}

func TestMapInvalidSourcePanics(t *testing.T) {
	assertPanicsWithMessage(t, "allow_nan", func() {
		Map(invalidFloats(), func(v float64) float64 { return v })
	})
}

func TestFloatsDrawInvalidConfigPanics(t *testing.T) {
	gen := Floats[float64]().Min(10.0).Max(5.0)
	assertPanicsWithMessage(t, "max_value", func() { gen.draw(nil) })
}

func TestTextDrawInvalidConfigPanics(t *testing.T) {
	gen := Text(-1, 5)
	assertPanicsWithMessage(t, "min_size", func() { gen.draw(nil) })
}

func TestCharactersDrawInvalidConfigPanics(t *testing.T) {
	gen := Characters().Categories([]string{"Cs"})
	assertPanicsWithMessage(t, "surrogate", func() { gen.draw(nil) })
}

func TestDomainsDrawInvalidConfigPanics(t *testing.T) {
	gen := Domains().MaxLength(0)
	assertPanicsWithMessage(t, "max_length", func() { gen.draw(nil) })
}
