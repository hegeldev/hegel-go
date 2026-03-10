package hegel

import (
	"fmt"
	"strings"
	"testing"
)

func ptr[T any](v T) *T { return &v }

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
	assertPanicsWithMessage(t, "allow_nan", func() { Floats(ptr(0.0), nil, ptr(true), nil, false, false) })
}

func TestFloatsAllowNaNWithMax(t *testing.T) {
	assertPanicsWithMessage(t, "allow_nan", func() { Floats(nil, ptr(10.0), ptr(true), nil, false, false) })
}

func TestFloatsMinGreaterThanMax(t *testing.T) {
	assertPanicsWithMessage(t, "max_value", func() { Floats(ptr(10.0), ptr(5.0), nil, nil, false, false) })
}

func TestFloatsAllowInfinityWithBothBounds(t *testing.T) {
	assertPanicsWithMessage(t, "allow_infinity", func() { Floats(ptr(0.0), ptr(10.0), nil, ptr(true), false, false) })
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
	assertPanicsWithMessage(t, "max_size", func() { Lists(Booleans(), ListMinSize(10), ListMaxSize(5)) })
}

func TestDictsMinSizeNegative(t *testing.T) {
	assertPanicsWithMessage(t, "min_size", func() { Dicts(Integers(0, 100), Integers(0, 100), DictMinSize(-1)) })
}

func TestDictsMinGreaterThanMax(t *testing.T) {
	assertPanicsWithMessage(t, "max_size", func() {
		Dicts(Integers(0, 100), Integers(0, 100), DictMinSize(10), DictMaxSize(5))
	})
}

func TestDomainsTooSmallMaxLength(t *testing.T) {
	// MaxLength <= 0 uses the default (255), so we need a value in [1, 3] to trigger the panic
	assertPanicsWithMessage(t, "max_length", func() { Domains(DomainMaxLength(3)) })
}

func TestDomainsTooBigMaxLength(t *testing.T) {
	assertPanicsWithMessage(t, "max_length", func() { Domains(DomainMaxLength(256)) })
}

func TestIPAddressesDefaultNoPanic(t *testing.T) {
	// no option means "both", should not panic
	IPAddresses()
}

func TestIPAddressesVersion4NoPanic(t *testing.T) {
	IPAddresses(IPv4())
}

func TestIPAddressesVersion6NoPanic(t *testing.T) {
	IPAddresses(IPv6())
}

func TestOneOfZeroGenerators(t *testing.T) {
	assertPanicsWithMessage(t, "OneOf", func() { OneOf[bool]() })
}

func TestOneOfSingleGeneratorNoPanic(t *testing.T) {
	// one generator should be accepted
	OneOf(Booleans())
}
