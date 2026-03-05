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
	if _err := runHegel("integers_from_e2e", func(s *TestCase) {
		n := Draw[int64](s, IntegersFrom(&minV, &maxV))
		if n < 10 || n > 50 {
			panic("integers_from: out of range [10, 50]")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestIntegersFromUnboundedE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel("integers_from_unbounded_e2e", func(s *TestCase) {
		// Unbounded integers are converted to int64 by the generator's transform.
		// Just verify the draw completes without error.
		_ = Draw[int64](s, IntegersFrom(nil, nil))
	}, stderrNoteFn, []Option{WithTestCases(20)}); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_WithBounds(t *testing.T) {
	hegelBinPath(t)
	falseBool := false
	if _err := runHegel("floats_e2e_bounded", func(s *TestCase) {
		fv := Draw[float64](s, Floats(floatPtr(0.0), floatPtr(1.0), &falseBool, &falseBool, false, false))
		if math.IsNaN(fv) {
			panic("floats: NaN not allowed when allow_nan=false")
		}
		if math.IsInf(fv, 0) {
			panic("floats: Inf not allowed when allow_infinity=false")
		}
		if fv < 0.0 || fv > 1.0 {
			panic("floats: out of range [0.0, 1.0]")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_Unbounded(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel("floats_e2e_unbounded", func(s *TestCase) {
		// Unbounded floats may produce NaN or Inf -- any float64 is valid.
		_ = Draw[float64](s, Floats(nil, nil, nil, nil, false, false))
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_OnlyMin(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel("floats_e2e_only_min", func(s *TestCase) {
		fv := Draw[float64](s, Floats(floatPtr(0.0), nil, nil, nil, false, false))
		// allow_nan is false (has min), allow_infinity is true (no max)
		// Value should be >= 0.0 or Inf; NaN not allowed.
		if math.IsNaN(fv) {
			panic("floats: NaN not expected when min set")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestBooleansE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel("booleans_e2e", func(s *TestCase) {
		b := Draw[bool](s, Booleans(0.5))
		// A valid assertion: b is either true or false.
		if b != true && b != false {
			panic("booleans: expected bool")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestTextE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel("text_e2e", func(s *TestCase) {
		sv := Draw[string](s, Text(2, 8))
		count := utf8.RuneCountInString(sv)
		if count < 2 || count > 8 {
			panic("text: codepoint count out of range [2, 8]")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestTextE2E_Unbounded(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel("text_e2e_unbounded", func(s *TestCase) {
		sv := Draw[string](s, Text(0, -1))
		if !utf8.ValidString(sv) {
			panic("text: invalid UTF-8 string")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestBinaryE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel("binary_e2e", func(s *TestCase) {
		bv := Draw[[]byte](s, Binary(1, 10))
		if len(bv) < 1 || len(bv) > 10 {
			panic("binary: byte length out of range [1, 10]")
		}
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestBinaryE2E_Unbounded(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel("binary_e2e_unbounded", func(s *TestCase) {
		_ = Draw[[]byte](s, Binary(0, -1))
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}
