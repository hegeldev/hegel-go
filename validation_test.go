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
	assertPanicsWithMessage(t, "allow_nan", func() { Floats[float64]().Min(0.0).AllowNaN(true).buildSchema() })
}

func TestFloatsAllowNaNWithMax(t *testing.T) {
	assertPanicsWithMessage(t, "allow_nan", func() { Floats[float64]().Max(10.0).AllowNaN(true).buildSchema() })
}

func TestFloatsMinGreaterThanMax(t *testing.T) {
	assertPanicsWithMessage(t, "max_value", func() { Floats[float64]().Min(10.0).Max(5.0).buildSchema() })
}

func TestFloatsAllowInfinityWithBothBounds(t *testing.T) {
	assertPanicsWithMessage(t, "allow_infinity", func() { Floats[float64]().Min(0.0).Max(10.0).AllowInfinity(true).buildSchema() })
}

func TestTextMinSizeNegative(t *testing.T) {
	assertPanicsWithMessage(t, "min_size", func() { Text(-1, 10) })
}

func TestTextMinGreaterThanMax(t *testing.T) {
	assertPanicsWithMessage(t, "max_size", func() { Text(10, 5) })
}

func TestBinaryMinSizeNegative(t *testing.T) {
	assertPanicsWithMessage(t, "min_size", func() { Binary(-1, 10) })
}

func TestBinaryMinGreaterThanMax(t *testing.T) {
	assertPanicsWithMessage(t, "max_size", func() { Binary(10, 5) })
}

func TestListsMinGreaterThanMax(t *testing.T) {
	assertPanicsWithMessage(t, "max_size", func() { Lists(Booleans()).MinSize(10).MaxSize(5).buildGenerator() })
}

func TestListsMinSizeNegative(t *testing.T) {
	assertPanicsWithMessage(t, "min_size", func() { Lists(Booleans()).MinSize(-1).buildGenerator() })
}

func TestListsMaxSizeNegative(t *testing.T) {
	assertPanicsWithMessage(t, "max_size", func() { Lists(Booleans()).MaxSize(-1).buildGenerator() })
}

func TestDictsMinSizeNegative(t *testing.T) {
	assertPanicsWithMessage(t, "min_size", func() {
		Dicts(Integers(0, 100), Integers(0, 100)).MinSize(-1).buildGenerator()
	})
}

func TestDictsMaxSizeNegative(t *testing.T) {
	assertPanicsWithMessage(t, "max_size", func() {
		Dicts(Integers(0, 100), Integers(0, 100)).MaxSize(-1).buildGenerator()
	})
}

func TestDictsMinGreaterThanMax(t *testing.T) {
	assertPanicsWithMessage(t, "max_size", func() {
		Dicts(Integers(0, 100), Integers(0, 100)).MinSize(10).MaxSize(5).buildGenerator()
	})
}

func TestDomainsTooSmallMaxLength(t *testing.T) {
	assertPanicsWithMessage(t, "max_length", func() { Domains().MaxLength(3).buildSchema() })
}

func TestDomainsNonPositiveMaxLength(t *testing.T) {
	assertPanicsWithMessage(t, "max_length", func() { Domains().MaxLength(0).buildSchema() })
}

func TestDomainsTooBigMaxLength(t *testing.T) {
	assertPanicsWithMessage(t, "max_length", func() { Domains().MaxLength(256).buildSchema() })
}

func TestOneOfZeroGenerators(t *testing.T) {
	assertPanicsWithMessage(t, "OneOf", func() { OneOf[bool]() })
}

func TestOneOfSingleGeneratorNoPanic(t *testing.T) {
	// one generator should be accepted
	OneOf(Booleans())
}
