package libhegel

// coverage_test.go covers the primitive wrappers and string-generator
// constructors not reached by the other stub tests: the UUID / IPv6 draws, the
// error-return branches of the byte/string/integer-big draws, the empty-buffer
// sliceData branch, and each string-generator constructor's failure path.

import (
	"math"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestToTime verifies Date.ToTime and Datetime.ToTime map onto the expected
// UTC time.Time values.
func TestToTime(t *testing.T) {
	d := Date{Year: 2026, Month: 7, Day: 7}
	if got, want := d.ToTime(), time.Date(2026, 7, 7, 0, 0, 0, 0, time.UTC); !got.Equal(want) {
		t.Errorf("Date.ToTime() = %v, want %v", got, want)
	}
	dt := Datetime{Date: d, Time: Time{Hour: 13, Minute: 20, Second: 30, Microsecond: 123456}}
	if got, want := dt.ToTime(), time.Date(2026, 7, 7, 13, 20, 30, 123456000, time.UTC); !got.Equal(want) {
		t.Errorf("Datetime.ToTime() = %v, want %v", got, want)
	}
}

// TestBigIntRoundTrip verifies that NewBigInt and BigInt.Uint64 round-trip
// non-negative values, including those above math.MaxInt64 that require the
// arbitrary-precision draw path.
func TestBigIntRoundTrip(t *testing.T) {
	for _, v := range []uint64{0, 1, 42, 255, 256, math.MaxInt64, math.MaxInt64 + 1, math.MaxUint64} {
		le := NewBigInt(v)
		// Encoding of a non-negative value never has its top bit set.
		if le[len(le)-1]&0x80 != 0 {
			t.Errorf("NewBigInt(%d) = %v has sign bit set in MSB", v, le)
		}
		if got := le.Uint64(); got != v {
			t.Errorf("round-trip %d: encoded %v decoded %d", v, le, got)
		}
	}
}

// TestNewBigIntSigned checks the two's-complement encoding of signed values,
// exercising the negative (sign-extended) branch of NewBigInt.
func TestNewBigIntSigned(t *testing.T) {
	for _, tc := range []struct {
		v    int64
		want BigInt
	}{
		{0, BigInt{0x00}},
		{127, BigInt{0x7f}},
		{128, BigInt{0x80, 0x00}}, // positive: trailing sign byte
		{-1, BigInt{0xff}},
		{-128, BigInt{0x80}},
		{-129, BigInt{0x7f, 0xff}},
		{-256, BigInt{0x00, 0xff}},
		{math.MinInt64, BigInt{0, 0, 0, 0, 0, 0, 0, 0x80}},
	} {
		if got := NewBigInt(tc.v); !slices.Equal(got, tc.want) {
			t.Errorf("NewBigInt(%d) = %v, want %v", tc.v, got, tc.want)
		}
	}
}

// TestBigIntUint64Overflow verifies Uint64 panics on a value too large for a
// uint64 rather than silently truncating the extraneous high bytes.
func TestBigIntUint64Overflow(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Uint64 did not panic on an overflowing value")
		}
	}()
	// Nine value bytes: the ninth (index 8) is non-zero, so it cannot be
	// non-negative sign extension.
	_ = BigInt{0, 0, 0, 0, 0, 0, 0, 0, 1}.Uint64()
}

func TestStubUUIDAndIPv6(t *testing.T) {
	lib := Stub(t,
		make([]byte, 16), OK, // generate_uuid
		make([]byte, 16), OK, // generate_ipv6
	)
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}
	if _, err := tc.GenerateUUID(lib, 4, true); err != nil {
		t.Fatalf("GenerateUUID: %v", err)
	}
	if _, err := tc.GenerateIPv6(lib); err != nil {
		t.Fatalf("GenerateIPv6: %v", err)
	}
}

func TestStubGenerateBytesEmpty(t *testing.T) {
	// An empty result exercises sliceData's zero-length (NULL data) branch.
	lib := Stub(t, []byte{}, OK)
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}
	got, err := tc.GenerateBytes(lib, 0, 0)
	if err != nil {
		t.Fatalf("GenerateBytes: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty result, got %v", got)
	}
}

func TestStubPrimitiveErrors(t *testing.T) {
	lib := Stub(t,
		[]byte{0}, uint64(0), E_BACKEND, "big boom", // generate_integer_big
		[]byte{}, E_BACKEND, "bytes boom", // generate_bytes
		"", E_BACKEND, "string boom", // generate_string
	)
	tc := &TestCase{pointer: &pointer[testCaseT]{syms: lib.syms, raw: 1}}
	if _, err := tc.GenerateIntegerBig(lib, []byte{0}, []byte{0xff}); err == nil {
		t.Error("GenerateIntegerBig: expected error")
	}
	if _, err := tc.GenerateBytes(lib, 0, 8); err == nil {
		t.Error("GenerateBytes: expected error")
	}
	sg := &StringGenerator{syms: lib.syms, raw: 1}
	if _, err := tc.GenerateString(lib, sg); err == nil {
		t.Error("GenerateString: expected error")
	}
}

func TestStubStringGeneratorErrors(t *testing.T) {
	// Each constructor writes a zero handle then a failing Error; the wrapper
	// surfaces the diagnostic and a nil generator.
	lib := Stub(t,
		uintptr(0), E_INVALID_ARG, "text boom",
		uintptr(0), E_INVALID_ARG, "regex boom",
		uintptr(0), E_INVALID_ARG, "email boom",
		uintptr(0), E_INVALID_ARG, "url boom",
		uintptr(0), E_INVALID_ARG, "domain boom",
	)
	for _, tc := range []struct {
		name string
		call func() (*StringGenerator, error)
	}{
		{"text", func() (*StringGenerator, error) {
			return lib.StringGeneratorText(0, 8, "utf-8", 0, 0, nil, nil, nil, nil)
		}},
		{"regex", func() (*StringGenerator, error) { return lib.StringGeneratorRegex("(", true) }},
		{"email", lib.StringGeneratorEmail},
		{"url", lib.StringGeneratorURL},
		{"domain", func() (*StringGenerator, error) { return lib.StringGeneratorDomain(255) }},
	} {
		g, err := tc.call()
		if err == nil || g != nil {
			t.Errorf("%s: expected error and nil generator, got g=%v err=%v", tc.name, g, err)
		}
		if !strings.Contains(err.Error(), "boom") {
			t.Errorf("%s: expected diagnostic, got %v", tc.name, err)
		}
	}
}
