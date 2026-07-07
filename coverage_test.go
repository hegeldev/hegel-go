package hegel

// coverage_test.go drives the error-return branches of the typed-primitive
// generators (draws whose libhegel call fails) plus the few builder/helper
// paths not exercised by the E2E or validation tests. Each generator is drawn
// against a Stub scripted to fail at the relevant primitive.

import (
	"math"
	"testing"

	"hegel.dev/go/hegel/internal/libhegel"
)

func TestIntegersBigPathError(t *testing.T) {
	t.Parallel()
	// uint64 bounds above math.MaxInt64 take the integer_big path; the stub
	// writes the (outBuf, len) placeholders before the failing Error.
	tc := newStubTestCase(t, []byte{0}, uint64(0), libhegel.E_BACKEND, "boom")
	if _, err := Integers[uint64](0, math.MaxUint64).draw(tc); err == nil {
		t.Fatal("expected integer_big draw error")
	}
}

func TestFloatExcludeBoundsBuilders(t *testing.T) {
	t.Parallel()
	g := Floats[float64]().Min(0).Max(1).AllowNaN(false).AllowInfinity(false).ExcludeMin().ExcludeMax()
	_, _, _, _, _, _, err := g.params()
	if err != nil {
		t.Fatalf("params: %v", err)
	}
	if !g.excludeMin || !g.excludeMax {
		t.Fatal("ExcludeMin/ExcludeMax not recorded")
	}
}

func TestTextArgsNilCategories(t *testing.T) {
	t.Parallel()
	cf := &characterFields{hasCategoriesSet: true, categories: nil}
	_, _, _, cats, excl, err := cf.textArgs()
	if err != nil {
		t.Fatalf("textArgs: %v", err)
	}
	if cats == nil || len(cats) != 0 || excl != nil {
		t.Fatalf("expected empty non-nil categories and nil exclude, got cats=%v excl=%v", cats, excl)
	}
}

func TestDomainsDrawError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, uintptr(0), libhegel.E_INVALID_ARG, "bad domain")
	if _, err := Domains().draw(tc); err == nil {
		t.Fatal("expected domain generator construction error")
	}
}

func TestFromRegexDrawError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, uintptr(0), libhegel.E_INVALID_ARG, "bad pattern")
	if _, err := FromRegex("(", true).draw(tc); err == nil {
		t.Fatal("expected regex generator construction error")
	}
}

func TestDatesDrawError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, libhegel.Date{}, libhegel.E_BACKEND, "boom")
	if _, err := Dates().draw(tc); err == nil {
		t.Fatal("expected date draw error")
	}
}

func TestDatetimesDrawError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, libhegel.Datetime{}, libhegel.E_BACKEND, "boom")
	if _, err := Datetimes().draw(tc); err == nil {
		t.Fatal("expected datetime draw error")
	}
}

func TestSampledFromDrawError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, int64(0), libhegel.E_BACKEND, "boom")
	if _, err := SampledFrom([]string{"a", "b"}).draw(tc); err == nil {
		t.Fatal("expected sampled_from index draw error")
	}
}

func TestOneOfIndexDrawError(t *testing.T) {
	t.Parallel()
	// start_span succeeds, then the branch-index integer draw fails.
	tc := newStubTestCase(t, libhegel.OK, int64(0), libhegel.E_BACKEND, "boom")
	if _, err := OneOf(Booleans(), Booleans()).draw(tc); err == nil {
		t.Fatal("expected one_of index draw error")
	}
}

func TestOptionalIndexDrawError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, libhegel.OK, int64(0), libhegel.E_BACKEND, "boom")
	if _, err := Optional(Booleans()).draw(tc); err == nil {
		t.Fatal("expected optional index draw error")
	}
}

func TestIPAddressesV4DrawError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, []byte{0, 0, 0, 0}, libhegel.E_BACKEND, "boom")
	if _, err := IPAddresses().IPv4().draw(tc); err == nil {
		t.Fatal("expected ipv4 draw error")
	}
}

func TestIPAddressesV6DrawError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, make([]byte, 16), libhegel.E_BACKEND, "boom")
	if _, err := IPAddresses().IPv6().draw(tc); err == nil {
		t.Fatal("expected ipv6 draw error")
	}
}

func TestIPAddressesDefaultIndexDrawError(t *testing.T) {
	t.Parallel()
	tc := newStubTestCase(t, libhegel.OK, int64(0), libhegel.E_BACKEND, "boom")
	if _, err := IPAddresses().draw(tc); err == nil {
		t.Fatal("expected default ip index draw error")
	}
}
