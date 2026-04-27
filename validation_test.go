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
