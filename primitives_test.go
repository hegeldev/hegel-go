package hegel

// primitives_test.go contains e2e integration tests for the primitive
// generator functions: IntegersFrom, Floats, Booleans, Text, Binary.
// Schema unit tests live in generators_test.go.

import (
	"math"
	"testing"
	"unicode/utf8"
)

// floatPtr returns a pointer to f, for use with Floats().
func floatPtr(f float64) *float64 { return &f }

// =============================================================================
// Integration / e2e tests (run against real hegel binary, 50 test cases each)
// =============================================================================

func TestIntegersFromE2E(t *testing.T) {
	hegelBinPath(t)
	minV := int64(10)
	maxV := int64(50)
	RunHegelTest("integers_from_e2e", func() {
		v := Draw(IntegersFrom(&minV, &maxV))
		if v < 10 || v > 50 {
			panic("integers_from: out of range [10, 50]")
		}
	}, WithTestCases(50))
}

func TestIntegersFromUnboundedE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest("integers_from_unbounded_e2e", func() {
		v := Draw(IntegersFrom(nil, nil))
		// Unbounded integers return int64 directly — any value is valid.
		_ = v
	}, WithTestCases(20))
}

func TestFloatsE2E_WithBounds(t *testing.T) {
	hegelBinPath(t)
	falseBool := false
	RunHegelTest("floats_e2e_bounded", func() {
		fv := Draw(Floats(floatPtr(0.0), floatPtr(1.0), &falseBool, &falseBool, false, false))
		if math.IsNaN(fv) {
			panic("floats: NaN not allowed when allow_nan=false")
		}
		if math.IsInf(fv, 0) {
			panic("floats: Inf not allowed when allow_infinity=false")
		}
		if fv < 0.0 || fv > 1.0 {
			panic("floats: out of range [0.0, 1.0]")
		}
	}, WithTestCases(50))
}

func TestFloatsE2E_Unbounded(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest("floats_e2e_unbounded", func() {
		fv := Draw(Floats(nil, nil, nil, nil, false, false))
		// Unbounded floats may produce NaN or Inf — any float64 is valid.
		_ = fv
	}, WithTestCases(50))
}

func TestFloatsE2E_OnlyMin(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest("floats_e2e_only_min", func() {
		fv := Draw(Floats(floatPtr(0.0), nil, nil, nil, false, false))
		// allow_nan is false (has min), allow_infinity is true (no max)
		// Value should be >= 0.0 or Inf; NaN not allowed.
		if math.IsNaN(fv) {
			panic("floats: NaN not expected when min set")
		}
	}, WithTestCases(50))
}

func TestBooleansE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest("booleans_e2e", func() {
		b := Draw(Booleans(0.5))
		// Draw returns bool directly — any value is valid.
		_ = b
	}, WithTestCases(50))
}

func TestTextE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest("text_e2e", func() {
		sv := Draw(Text(2, 8))
		count := utf8.RuneCountInString(sv)
		if count < 2 || count > 8 {
			panic("text: codepoint count out of range [2, 8]")
		}
	}, WithTestCases(50))
}

func TestTextE2E_Unbounded(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest("text_e2e_unbounded", func() {
		sv := Draw(Text(0, -1))
		if !utf8.ValidString(sv) {
			panic("text: invalid UTF-8 string")
		}
	}, WithTestCases(50))
}

func TestBinaryE2E(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest("binary_e2e", func() {
		bv := Draw(Binary(1, 10))
		if len(bv) < 1 || len(bv) > 10 {
			panic("binary: byte length out of range [1, 10]")
		}
	}, WithTestCases(50))
}

func TestBinaryE2E_Unbounded(t *testing.T) {
	hegelBinPath(t)
	RunHegelTest("binary_e2e_unbounded", func() {
		bv := Draw(Binary(0, -1))
		// Draw returns []byte directly — any value is valid.
		_ = bv
	}, WithTestCases(50))
}
