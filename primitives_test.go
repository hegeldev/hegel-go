package hegel

// primitives_test.go contains e2e integration tests for the primitive
// generator functions: Integers, Floats, Booleans, Text, Binary.
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

func TestIntegersFullRangeE2E(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
		// Full-range integers: just verify the draw completes without error.
		_ = Draw[int](s, Integers[int](math.MinInt, math.MaxInt))
	}, stderrNoteFn, []Option{WithTestCases(20)}); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_WithBounds(t *testing.T) {
	hegelBinPath(t)
	falseBool := false
	if _err := runHegel(func(s *TestCase) {
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
	if _err := runHegel(func(s *TestCase) {
		// Unbounded floats may produce NaN or Inf -- any float64 is valid.
		_ = Draw[float64](s, Floats(nil, nil, nil, nil, false, false))
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}

func TestFloatsE2E_OnlyMin(t *testing.T) {
	hegelBinPath(t)
	if _err := runHegel(func(s *TestCase) {
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
	if _err := runHegel(func(s *TestCase) {
		b := Draw[bool](s, Booleans())
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
	if _err := runHegel(func(s *TestCase) {
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
	if _err := runHegel(func(s *TestCase) {
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
	if _err := runHegel(func(s *TestCase) {
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
	if _err := runHegel(func(s *TestCase) {
		_ = Draw[[]byte](s, Binary(0, -1))
	}, stderrNoteFn, []Option{WithTestCases(50)}); _err != nil {
		panic(_err)
	}
}
